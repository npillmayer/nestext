package nestext

import (
	"fmt"
	"io"
	"reflect"
	"strings"
)

// TODO unify stack to a single type (now duplicates for top-level parser and inline-parser).

// Parse reads a NestedText input source and outputs a resulting hierarchy of values.
// Values are stored as strings, []interface{} or map[string]interface{} respectively.
//
// If a non-nil error is returned, it will be of type NestedTextError.
//
func Parse(r io.Reader, opts ...Option) (interface{}, error) {
	p := newParser()
	for _, opt := range opts {
		if err := opt(p); err != nil {
			return nil, err
		}
	}
	return p.Parse(r)
}

// === Parser Options ========================================================

// Option is a type to influence the behaviour of the parsing process.
// Multiple options may be passed to `Parse(…)`.
type Option _Option

type _Option func(*nestedTextParser) error // internal synonym to hide unterlying type of options.

// TopLevel determines the top-level type of the return value from parsing.
// Possible values are "list" and "dict". "list" will force the result to be an
// []interface{} (of possibly one item), while "dict" will force the result to be of
// type map[string]interface.
//
// For "dict", if the result is not a dict naturally, it will be wrapped in a map with a single
// key = "dict". However, if the dict-option is given with a suffix (separated by '.'), the suffix
// string will be used as the top-level key. In this case, even naturally parsed dicts will be
// wrapped into a map with a single key (= the suffix to "dict.").
//
// Use as:
//     nestext.Parse(reader, nestext.TopLevel("dict.config"))
//
// This will result in a return-value of map[string]interface{} with a single entry
// map["config"] = …
//
// The default is for the parsing-result to be of the natural type corresponding to the
// top-level item of the input source.
// Option-strings other than "list" and "dict"/"dict.<suffix>" will result in an error
// returned by Parse(…).
//
func TopLevel(top string) Option {
	return func(p *nestedTextParser) (err error) {
		switch top {
		case "dict":
			p.toplevel = "dict"
		case "list":
			p.toplevel = "list"
		default:
			if strings.HasPrefix(top, "dict.") {
				p.toplevel = top[5:]
			} else {
				return MakeNestedTextError(ErrCodeUsage, `option TopLevel( "list" | "dict"(".<suffix>")? )`)
			}
		}
		return nil
	}
}

// TODO
//
// Default behaviour should be to strip LTR and RTL legacy control characters.
func KeepLegacyBidi(keep bool) Option {
	return func(p *nestedTextParser) (err error) {
		return nil
	}
}

// === Top level parser ======================================================

type nestedTextParser struct {
	sc       *scanner           // line level scanner
	token    *parserToken       // the current token from the scanner
	inline   *inlineItemParser  // sub-parser for inline lists/dicts
	stack    []inlineStackEntry // result stack
	toplevel string             // type of top-level item
}

func newParser() *nestedTextParser {
	p := &nestedTextParser{
		inline: newInlineParser(),
		stack:  make([]inlineStackEntry, 0, 10),
	}
	return p
}

func (p *nestedTextParser) Parse(r io.Reader) (result interface{}, err error) {
	p.sc, err = newScanner(r)
	if err != nil {
		return
	}
	result, err = p.parseDocument()
	if err == nil {
		result = p.wrapResult(result)
	}
	return
}

func (p *nestedTextParser) parseDocument() (result interface{}, err error) {
	// initial token from scanner is a health check for the input source
	fmt.Println("# requesting document root from scanner")
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return nil, p.token.Error
	}
	fmt.Printf("# document root = %s\n", p.token)
	if p.token.TokenType == eof || p.token.TokenType == emptyDocument {
		return nil, nil
	}
	// read the first item line
	fmt.Println("# parsers starts requesting lines")
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return nil, p.token.Error
	}
	result, err = p.parseAny(0)
	fmt.Printf("final token = %s\n", p.token)
	fmt.Printf("final line = %d\n", p.sc.Buf.CurrentLine)
	if err == nil && p.token.TokenType != eof { // TODO this test is not sufficient
		err = makeParsingError(p.token, ErrCodeFormat,
			"unused content following valid input")
	}
	return
}

func (p *nestedTextParser) parseAny(indent int) (result interface{}, err error) {
	fmt.Printf("# parseAny(%d)\n", indent)
	if p.token.Indent < indent {
		fmt.Println("# <-outdent")
		return nil, nil
	}
	switch p.token.TokenType {
	case stringMultiline:
		result, err = p.parseMultiString(p.token.Indent)
	case inlineList:
		result, err = p.inline.parse(_S2, p.token.Content[0])
		fmt.Printf("sub list = %v\n", result)
	case inlineDict:
		result, err = p.inline.parse(_S1, p.token.Content[0])
		fmt.Printf("sub dict = %v\n", result)
	case listItem, listItemMultiline:
		result, err = p.parseList(indent)
	case inlineDictKeyValue, inlineDictKey, dictKeyMultiline:
		result, err = p.parseDict(indent)
	default:
		panic(fmt.Sprintf("unknown item type: %d/%s", p.token.TokenType, p.token.TokenType))
	}
	return
}

func (p *nestedTextParser) parseList(indent int) (result interface{}, err error) {
	fmt.Printf("# parseList(%d)\n", indent)
	p.pushNonterm(false)
	_, err = p.parseListItems(p.token.Indent)
	if err != nil {
		return nil, err
	}
	result, err = p.tos().ReduceToItem()
	p.pop()
	return
}

func (p *nestedTextParser) parseListItems(indent int) (result interface{}, err error) {
	fmt.Printf("# parseListItems(%d)\n", indent)
	var value interface{}
	for p.token.TokenType == listItem || p.token.TokenType == listItemMultiline {
		if p.token.TokenType == listItem {
			value, err = p.parseListItem(indent)
		} else {
			value, err = p.parseListItemMultiline(indent)
		}
		if value != nil && err == nil {
			p.push(nil, value)
		}
	}
	fmt.Printf("# <-- parseListItems\n")
	return p.tos().Values, err
}

func (p *nestedTextParser) parseListItem(indent int) (result interface{}, err error) {
	fmt.Printf("# parseListItem(%d)\n", indent)
	if p.token.Indent != indent {
		return nil, nil
	}
	value := p.token.Content[0]
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return nil, p.token.Error
	}
	fmt.Printf("# <-- parseListItem\n")
	return value, err
}

func (p *nestedTextParser) parseListItemMultiline(indent int) (result interface{}, err error) {
	fmt.Printf("# --> parseListItemMultiline(%d)..", indent)
	if p.token.Indent != indent {
		fmt.Printf(".X\n")
		return nil, nil
	}
	fmt.Printf(".go\n")
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return nil, p.token.Error
	}
	if p.token.Indent <= indent {
		return "", nil
	}
	result, err = p.parseAny(p.token.Indent)
	fmt.Println("# <-- parseListItemMultiline")
	return
}

func (p *nestedTextParser) parseDict(indent int) (result interface{}, err error) {
	fmt.Printf("# parseDict(%d)\n", indent)
	p.pushNonterm(true)
	_, err = p.parseDictKeyValuePairs(p.token.Indent)
	if err != nil {
		return nil, err
	}
	result, err = p.tos().ReduceToItem()
	p.pop()
	return
}

type keyValuePair struct {
	key   *string
	value interface{}
}

func (p *nestedTextParser) parseDictKeyValuePairs(indent int) (result interface{}, err error) {
	fmt.Printf("# parseDictKeyValuePairs(%d)\n", indent)
	var kv keyValuePair
	for p.token.TokenType == inlineDictKeyValue || p.token.TokenType == inlineDictKey ||
		p.token.TokenType == dictKeyMultiline {
		//
		switch p.token.TokenType {
		case inlineDictKeyValue:
			kv, err = p.parseDictKeyValuePair(indent)
		case inlineDictKey:
			kv, err = p.parseDictKeyAnyValuePair(indent)
		case dictKeyMultiline:
			kv, err = p.parseDictKeyValuePairWithMultilineKey(indent)
		}
		if kv.value != nil {
			if err == nil {
				p.push(kv.key, kv.value)
			}
		} else {
			break
		}
	}
	fmt.Println("# <-- parseDictKeyValuePairs")
	return p.tos().Keys, err
}

func (p *nestedTextParser) parseDictKeyValuePair(indent int) (kv keyValuePair, err error) {
	fmt.Printf("# parseDictKeyValuePair(%d)..", indent)
	if p.token.Indent != indent {
		fmt.Printf(".X\n")
		return
	}
	fmt.Printf(".go\n")
	key := p.token.Content[0]
	value := p.token.Content[1]
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return kv, p.token.Error
	}
	fmt.Println("# <-- parseDictKeyValuePair")
	return keyValuePair{key: &key, value: value}, err
}

func (p *nestedTextParser) parseDictKeyAnyValuePair(indent int) (kv keyValuePair, err error) {
	fmt.Printf("# parseDictAnyKeyValuePair(%d)..", indent)
	if p.token.Indent != indent {
		fmt.Printf(".X\n")
		return
	}
	fmt.Printf(".go\n")
	kv.key = &p.token.Content[0]
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return kv, p.token.Error
	}
	if p.token.Indent <= indent {
		kv.value = ""
		return
	}
	kv.value, err = p.parseAny(p.token.Indent)
	fmt.Println("# <-- parseDictAnyKeyValuePair")
	return
}

func (p *nestedTextParser) parseDictKeyValuePairWithMultilineKey(indent int) (kv keyValuePair, err error) {
	fmt.Printf("# --> parseDictKeyValuePairWithMultilineKey(%d)\n", indent)
	if p.token.Indent != indent {
		fmt.Printf(".X\n")
		return
	}
	fmt.Printf(".go\n")
	builder := strings.Builder{}
	builder.WriteString(p.token.Content[0])
	for err == nil {
		p.token = p.sc.NextToken()
		if p.token.Error != nil {
			return kv, p.token.Error
		}
		if p.token.TokenType != dictKeyMultiline || p.token.Indent != indent {
			break
		}
		builder.WriteRune('\n')
		builder.WriteString(p.token.Content[0])
	}
	key := builder.String()
	kv.key = &key
	if p.token.Indent <= indent {
		return keyValuePair{key: &key, value: ""}, nil
	}
	kv.value, err = p.parseAny(p.token.Indent)
	fmt.Println("# <-- parseDictKeyValuePairWithMultilineKey")
	return
}

func (p *nestedTextParser) parseMultiString(indent int) (result interface{}, err error) {
	fmt.Printf("# --> parseMultiString(%d)..", indent)
	if p.token.Indent != indent {
		fmt.Printf(".X\n")
		return nil, nil
	}
	fmt.Printf(".go\n")
	builder := strings.Builder{}
	builder.WriteString(p.token.Content[0])
	for err == nil {
		p.token = p.sc.NextToken()
		if p.token.Error != nil {
			fmt.Printf("### err = %s\n", p.token.Error)
			return builder.String(), p.token.Error
		}
		if p.token.TokenType != stringMultiline || p.token.Indent != indent {
			fmt.Printf("# stop string at %s\n", p.token)
			break
		}
		builder.WriteRune('\n')
		builder.WriteString(p.token.Content[0])
		fmt.Printf("# string.append %q\n", p.token.Content[0])
	}
	fmt.Printf("### eof = %v, err = %v\n", p.sc.Buf.IsEof(), err)
	fmt.Println("# <-- parseMultiString")
	return builder.String(), nil
}

func (p *nestedTextParser) pushNonterm(isDict bool) {
	entry := inlineStackEntry{
		Values: make([]interface{}, 0, 16),
	}
	if isDict { // dict
		entry.Keys = make([]string, 0, 16)
	}
	p.stack = append(p.stack, entry)
}

func (p *nestedTextParser) tos() *inlineStackEntry {
	if len(p.stack) > 0 {
		return &p.stack[len(p.stack)-1]
	}
	return nil
}

func (p *nestedTextParser) pop() (tos *inlineStackEntry) {
	if len(p.stack) > 0 {
		tos = p.tos()
		p.stack = p.stack[:len(p.stack)-1]
	}
	return tos
}

func (p *nestedTextParser) push(s *string, val interface{}) bool {
	if val != nil {
		var key string
		if s == nil {
			key = "<nil>"
		} else {
			key = *s
		}
		fmt.Printf("# push( %q , %#v )\n", key, val)
	}
	tos := &p.stack[len(p.stack)-1]
	tos.Values = append(tos.Values, val)
	if s != nil {
		tos.Keys = append(tos.Keys, *s)
	}
	return true
}

// wrapResult wraps the result according to the TopLevel option.
func (p *nestedTextParser) wrapResult(result interface{}) interface{} {
	switch p.toplevel {
	case "":
		// do nothing
	case "list":
		v := reflect.ValueOf(result)
		if v.Kind() != reflect.Slice {
			result = []interface{}{result}
		}
	case "dict":
		v := reflect.ValueOf(result)
		if v.Kind() != reflect.Map {
			result = map[string]interface{}{
				"dict": result,
			}
		}
	default:
		result = map[string]interface{}{
			p.toplevel: result,
		}
	}
	return result
}

// === Inline item parser ====================================================
//
// Inline items are lists or dicts as one-liners. Examples would be
//
//     [ one, two three ]
//     { one:1, two:2, three:3 }
//
// or nested instances like
//
//     { one:1, two:2, three:3, all: [1, 2, 3] }
//
// We use a scannerless parser. It suffices to construct the prefix-automaton for inline lists and
// inline dicts (see file "automata.dot"). The parser traverses the states of the automaton,
// performing an optional action at each of the states encountered. This way, the inline parser will
// collect strings as keys and/or values.
//
// We manage a stack to be able to parse nested items. Whenever an automaton moves to a non-terminal
// state, we push a stack-entry onto the parser stack. This stack-entry will hold all the information
// gathered for an list/dict item. As soon as an accept-state is reached for a nested item, the
// stack-entry is reduced to a result type ([]interface{} or map[string]interface{}) and the stack-entry
// is popped. The result is appended to the newly uncovered TOS.
//
type inlineItemParser struct {
	Text         string             // current line of NestedText
	TextPosition int                // position of reader in string
	Marker       int                // positional marker for start of key or value
	Input        *strings.Reader    // reader for Text
	stack        []inlineStackEntry // parse stack
}

// newInlineParser creates a fresh inline parser instance.
func newInlineParser() *inlineItemParser {
	return &inlineItemParser{
		stack: make([]inlineStackEntry, 0, 10),
	}
}

func (p *inlineItemParser) parse(initial inlineParserState, input string) (result interface{}, err error) {
	p.Text = input
	p.Input = strings.NewReader(input)
	p.stack = p.stack[:0]
	p.TextPosition, p.Marker = 0, 0
	//
	p.pushNonterm(initial)
	var oldState, state inlineParserState = 0, initial
	for len(p.stack) > 0 {
		ch, w, err := p.Input.ReadRune()
		if err != nil {
			err = WrapError(ErrCodeIO, "I/O-error reading inline item", err)
			return nil, err
		}
		chType := inlineTokenFor(ch)
		oldState, state = state, inlineStateMachine[state][chType]
		fmt.Printf("state = %d, |stack| = %d\n", state, len(p.stack))
		if isErrorState(state) {
			break
		} else if isNonterm(state) {
			nonterm := state
			p.pushNonterm(state)
			state = inlineStateMachine[state][chType]
			fmt.Printf(". encountered non-terminal %d\n", nonterm)
			p.tos().NontermState = inlineStateMachine[oldState][_S(nonterm)]
			fmt.Printf(". jumping to %d\n", state)
			fmt.Printf(". will drop back to %d\n", p.tos().NontermState)
		}
		ok := inlineStateMachineActions[state](p, oldState, state, ch, w)
		fmt.Printf("> action for state %d => %v\n", state, ok)
		if !ok {
			state = e // flag error by setting error state
			break
		}
		if isAccept(state) {
			fmt.Printf(". accept %d\n", state)
			result, err = p.tos().ReduceToItem()
			if err != nil {
				p.tos().Error = err
				state = e
				break
			}
			state = p.tos().NontermState
			fmt.Printf(". continue after non-term at %d\n", state)
			p.pop()
			if len(p.stack) > 0 {
				p.push(p.tos().Key, result)
			}
		}
		p.TextPosition += w
	}
	if isErrorState(state) {
		err = p.stack[len(p.stack)-1].Error
	}
	return
}

// pushNonterm pushes a new (empyt) stack entry onto the parser stack. Depending on wether
// the non-terminal represents a list item or a dict item, the .Keys slice will be initialized.
// This function will be called for every non-terminal encounterd during the parse run, i.e.,
// for the initial non-terminal as well as for every nested one.
//
func (p *inlineItemParser) pushNonterm(state inlineParserState) {
	entry := inlineStackEntry{
		Values: make([]interface{}, 0, 16),
	}
	if state == _S1 { // dict
		entry.Keys = make([]string, 0, 16)
	}
	p.stack = append(p.stack, entry)
}

// The inline parser manages a stack, with a stack entry for every non-terminal. The bottom
// stack entry represents the outermost item. Each successive nested item will trigger a
// stack-enty to be pushed onto the parser stack.
// Stack entries collect the information for an item, either a list or a dict.
type inlineStackEntry struct {
	Values       []interface{}     // list of values, either list items or dict values
	Keys         []string          // list of keys, empty for list items
	Key          *string           // current key to set value for, if in a dict
	Error        error             // if error occured: remember it
	NontermState inlineParserState // sub-nonterm, or 0 for root entry
}

func (p *inlineItemParser) tos() *inlineStackEntry {
	if len(p.stack) > 0 {
		return &p.stack[len(p.stack)-1]
	}
	return nil
}

func (p *inlineItemParser) pop() (tos *inlineStackEntry) {
	if len(p.stack) > 0 {
		tos = p.tos()
		p.stack = p.stack[:len(p.stack)-1]
	}
	return tos
}

func (p *inlineItemParser) push(s *string, val interface{}) bool {
	if val != nil {
		fmt.Printf("# push( %#v )\n", val)
	}
	tos := &p.stack[len(p.stack)-1]
	tos.Values = append(tos.Values, val)
	if s != nil {
		tos.Keys = append(tos.Keys, *s)
	}
	return true
}

func (entry inlineStackEntry) ReduceToItem() (interface{}, error) {
	if entry.Keys == nil {
		fmt.Printf("# reduce to %+v of type %T\n", entry.Values, entry.Values)
		return entry.Values, nil
	}
	dict := make(map[string]interface{}, len(entry.Values))
	if len(entry.Keys) > 0 && len(entry.Values) != len(entry.Keys) {
		// error: mixed content
		panic("mixed content")
	}
	for i, key := range entry.Keys {
		dict[key] = entry.Values[i]
	}
	fmt.Printf("# reduce to %v of type %T\n", dict, dict)
	return dict, nil
}

// --- Inline parser table ---------------------------------------------------

// The parser is driven by a prefix-automaton, moving over states identified by
// type inlineParserState
type inlineParserState int8

// For a diagram of the automata, please refer to automata.dot.
// States 1..9 are unnamed.
const e inlineParserState = -1   // error state
const _S1 inlineParserState = 10 // non-terminal S1
const _S2 inlineParserState = 11 // non-terminal S2
const _A1 inlineParserState = 12 // acceptance state A1
const _A2 inlineParserState = 13 // acceptance state A2

// isErrorState is a predicate on parser states.
func isErrorState(state inlineParserState) bool {
	return state < 0
}

// isNonterm is a predicate on parser states.
func isNonterm(state inlineParserState) bool {
	return state == _S1 || state == _S2
}

// isAcceptState is a predicate on parser states.
func isAccept(state inlineParserState) bool {
	return state == _A1 || state == _A2
}

// isGhostState is a predicate on parser states. It returns true if state is a
// "ghost state" (dashed line in the automata.dot diagram) which follows the
// acceptance of a nested non-terminal.
func isGhost(state inlineParserState) bool {
	return state == 5 || state == 9
}

const chClassCnt = 10

// Character classes:
//   A  \n ,  :  [  ]  {  }  _S(S1) _S(S2)
var inlineStateMachine = [...][chClassCnt]inlineParserState{
	{e, e, e, e, 7, e, 1, e, e, e},       // state 0, initial
	{2, e, e, 3, e, e, e, _A1, e, e},     // state 1
	{2, e, e, 3, e, e, e, e, e, e},       // state 2
	{4, e, 6, e, _S2, e, _S1, _A1, 5, 5}, // state 3
	{4, e, 6, e, e, e, e, _A1, e, e},     // state 4
	{e, e, 6, e, e, e, e, _A1, e, e},     // state 5
	{2, e, e, 3, e, e, e, _A1, e, e},     // state 6
	{8, e, 7, 8, _S2, _A2, _S1, e, 9, 9}, // state 7
	{8, e, 7, 8, e, _A2, e, e, e, e},     // state 8
	{e, e, 7, e, e, _A2, e, e, e, e},     // state 9
	{e, e, e, e, e, e, 1, e, e, e},       // state S1
	{e, e, e, e, 7, e, e, e, e, e},       // state S2
	{e, e, e, e, e, e, e, e, e, e},       // state A1
	{e, e, e, e, e, e, e, e, e, e},       // state A2
}

// _S returns a non-terminal state as a pseudo character class.
// This is used to determine the "ghost state" which follows the acceptance of a nested
// non-terminal.
func _S(s inlineParserState) int {
	return int(s-_S1) + chClassCnt - 2
}

var inlineStateMachineActions = [...]func(p *inlineItemParser,
	from, to inlineParserState, ch rune, w int) bool{
	nop, // 0
	func(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool { // 1
		p.Marker = p.TextPosition + w // get ready for first key
		fmt.Printf("- Marker for key at %d\n", p.Marker)
		return true
	},
	nop, // 2
	func(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool { // 3
		key := p.Text[p.Marker:p.TextPosition]
		key = strings.TrimSpace(key)
		p.tos().Key = &key
		p.Marker = p.TextPosition + w // get ready for value
		fmt.Printf("- Marker for value at %d\n", p.Marker)
		return true
	},
	nop, // 4
	nop, // 5
	func(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool { // 6
		if p.Marker > 0 && !isGhost(from) {
			p.appendStringValue(false)
		}
		p.tos().Key = nil
		p.Marker = p.TextPosition + w // get ready for next key
		fmt.Printf("- Marker for key at %d\n", p.Marker)
		return true
	},
	func(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool { // 7
		if ch == ',' && p.Marker > 0 && !isGhost(from) {
			p.appendStringValue(false)
		}
		p.Marker = p.TextPosition + w // get ready for next item
		fmt.Printf("- Marker for item at %d\n", p.Marker)
		return true
	},
	nop,    // 8
	nop,    // 9
	nop,    // S1
	nop,    // S2
	accept, // A1
	accept, // A2
}

func (p *inlineItemParser) appendStringValue(isAccept bool) {
	value := p.Text[p.Marker:p.TextPosition]
	// From the spec:
	// Both inline lists and dictionaries may be empty, and represent the only way to
	// represent empty lists or empty dictionaries in NestedText. An empty dictionary
	// is represented with {} and an empty list with []. In both cases there must be
	// no space between the opening and closing delimiters. An inline list that contains
	// only white spaces, such as [ ], is treated as a list with a single empty string
	// (the whitespace is considered a string value, and string values have leading and
	// trailing spaces removed, resulting in an empty string value). If a list contains
	// multiple values, no white space is required to represent an empty string
	// Thus, [] represents an empty list, [ ] a list with a single empty string value,
	// and [,] a list with two empty string values.
	if !isAccept || len(value) > 0 || len(p.tos().Values) > 0 {
		value = strings.TrimSpace(value)
		p.push(p.tos().Key, value)
	}
}

// nop is a no-op state machine action.
func nop(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool {
	return true
}

func parserError(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool {
	return false
}

func accept(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool {
	fmt.Println("ACCEPT")
	if p.Marker > 0 && !isGhost(from) {
		p.appendStringValue(true)
	}
	return true
}

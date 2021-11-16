package nestext

import (
	"fmt"
	"io"
	"reflect"
	"strings"
)

// === Top-level API =========================================================

// Parse reads a NestedText input source and outputs a resulting hierarchy of values.
// Values are stored as strings, []interface{} or map[string]interface{} respectively.
// The concrete resulting top-level type depends on the top-level NestedText input type.
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

// --- Parser options --------------------------------------------------------

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
// key = "nestedtext". However, if the dict-option is given with a suffix (separated by '.'), the
// suffix string will be used as the top-level key. In this case, even naturally parsed dicts will
// be wrapped into a map with a single key (= the suffix to "dict.").
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

// KeepLegacyBidi requests the parser to keep Unicode LTR and RTL markers.
//
// Attention: This option is not yet functional!
func KeepLegacyBidi(keep bool) Option {
	// Default behaviour should be to strip LTR and RTL legacy control characters.
	// For security reasons applications should usually treat LTR/RTL cautiously when read
	// in from external sources. You can find various sources on the internet discussion
	// this problem, including a policy in place at GitHub.
	return func(p *nestedTextParser) (err error) {
		return nil
	}
}

// === Top level parser ======================================================

// nestedTextParser is a recursive-descend parser working on a grammar on input lines.
// The scanner is expected to return line by line wrapped into `parserToken`.
type nestedTextParser struct {
	sc       *scanner          // line level scanner
	token    *parserToken      // the current token from the scanner
	inline   *inlineItemParser // sub-parser for inline lists/dicts
	toplevel string            // type of top-level item
	stack    pstack            // parser stack
	//stack    []parserStackEntry // result stack
}

func newParser() *nestedTextParser {
	p := &nestedTextParser{
		inline: newInlineParser(),
		stack:  make([]parserStackEntry, 0, 10),
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
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return nil, p.token.Error
	}
	if p.token.TokenType == eof || p.token.TokenType == emptyDocument {
		return nil, nil
	}
	// read the first item line
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return nil, p.token.Error
	}
	result, err = p.parseAny(0)
	if err == nil && p.token.TokenType != eof { // TODO this test is not sufficient
		err = makeParsingError(p.token, ErrCodeFormat,
			"unused content following valid input")
	}
	return
}

func (p *nestedTextParser) parseAny(indent int) (result interface{}, err error) {
	if p.token.Indent < indent {
		return nil, nil
	}
	switch p.token.TokenType {
	case stringMultiline:
		result, err = p.parseMultiString(p.token.Indent)
	case inlineList:
		p.inline.LineNo = p.token.LineNo
		result, err = p.inline.parse(_S2, p.token.Content[0])
		if err == nil {
			if p.token = p.sc.NextToken(); p.token.Error != nil {
				return nil, p.token.Error
			}
		}
	case inlineDict:
		p.inline.LineNo = p.token.LineNo
		result, err = p.inline.parse(_S1, p.token.Content[0])
		if err == nil {
			if p.token = p.sc.NextToken(); p.token.Error != nil {
				return nil, p.token.Error
			}
		}
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
	p.pushNonterm(false)
	_, err = p.parseListItems(p.token.Indent)
	if err != nil {
		return nil, err
	}
	result, err = p.stack.tos().ReduceToItem()
	p.stack.pop()
	return
}

func (p *nestedTextParser) parseListItems(indent int) (result interface{}, err error) {
	var value interface{}
	for p.token.TokenType == listItem || p.token.TokenType == listItemMultiline {
		if p.token.TokenType == listItem {
			value, err = p.parseListItem(indent)
		} else {
			value, err = p.parseListItemMultiline(indent)
		}
		if value != nil && err == nil {
			p.stack.pushKV(nil, value)
		} else if err != nil {
			return
		} else if value == nil {
			break
		}
	}
	return p.stack.tos().Values, err
}

func (p *nestedTextParser) parseListItem(indent int) (result interface{}, err error) {
	if p.token.Indent > indent {
		return nil, MakeNestedTextError(ErrCodeFormat,
			"invalid indent: may only follow an item that does not already have a value")
	}
	if p.token.Indent < indent {
		return nil, nil
	}
	value := p.token.Content[0]
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return nil, p.token.Error
	}
	return value, err
}

func (p *nestedTextParser) parseListItemMultiline(indent int) (result interface{}, err error) {
	if p.token.Indent != indent {
		return nil, nil
	}
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return nil, p.token.Error
	}
	if p.token.Indent <= indent {
		return "", nil
	}
	result, err = p.parseAny(p.token.Indent)
	if p.token.Indent > indent {
		return nil, MakeNestedTextError(ErrCodeFormat,
			"invalid indent: may only follow an item that does not already have a value")
	}
	return
}

func (p *nestedTextParser) parseDict(indent int) (result interface{}, err error) {
	p.pushNonterm(true)
	_, err = p.parseDictKeyValuePairs(p.token.Indent)
	if err != nil {
		return nil, err
	}
	result, err = p.stack.tos().ReduceToItem()
	p.stack.pop()
	if p.token.Indent > indent {
		err = MakeNestedTextError(ErrCodeFormat, "partial dedent")
	}
	return
}

// keyValuePair is a helper type to hold dict key-values as return-type.
type keyValuePair struct {
	key   *string
	value interface{}
}

func (p *nestedTextParser) parseDictKeyValuePairs(indent int) (result interface{}, err error) {
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
			if err != nil {
				return
			}
			p.stack.pushKV(kv.key, kv.value)
		} else {
			break
		}
	}
	return p.stack.tos().Keys, err
}

func (p *nestedTextParser) parseDictKeyValuePair(indent int) (kv keyValuePair, err error) {
	if p.token.Indent != indent {
		return
	}
	key := p.token.Content[0]
	value := p.token.Content[1]
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return kv, p.token.Error
	}
	return keyValuePair{key: &key, value: value}, err
}

func (p *nestedTextParser) parseDictKeyAnyValuePair(indent int) (kv keyValuePair, err error) {
	if p.token.Indent != indent {
		return
	}
	kv.key = &p.token.Content[0]
	if p.token = p.sc.NextToken(); p.token.Error != nil {
		return kv, p.token.Error
	}
	if p.token.Indent <= indent {
		kv.value = ""
		return
	}
	kv.value, err = p.parseAny(p.token.Indent)
	return
}

func allowVoid(val []string, i int) string {
	if val == nil || len(val) <= i {
		return ""
	}
	return val[i]
}

func (p *nestedTextParser) parseDictKeyValuePairWithMultilineKey(indent int) (kv keyValuePair, err error) {
	if p.token.Indent != indent {
		return
	}
	builder := strings.Builder{}
	builder.WriteString(allowVoid(p.token.Content, 0))
	for err == nil {
		p.token = p.sc.NextToken()
		if p.token.Error != nil {
			return kv, p.token.Error
		}
		if p.token.TokenType != dictKeyMultiline || p.token.Indent != indent {
			break
		}
		builder.WriteRune('\n')
		builder.WriteString(allowVoid(p.token.Content, 0))
	}
	key := builder.String()
	kv.key = &key
	if p.token.Indent <= indent {
		return keyValuePair{key: &key, value: ""}, nil
	}
	kv.value, err = p.parseAny(p.token.Indent)
	return
}

func (p *nestedTextParser) parseMultiString(indent int) (result interface{}, err error) {
	if p.token.Indent != indent {
		return nil, nil
	}
	builder := strings.Builder{}
	builder.WriteString(allowVoid(p.token.Content, 0))
	for err == nil {
		p.token = p.sc.NextToken()
		if p.token.Error != nil {
			return builder.String(), p.token.Error
		}
		if p.token.TokenType != stringMultiline || p.token.Indent != indent {
			break
		}
		builder.WriteRune('\n')
		builder.WriteString(allowVoid(p.token.Content, 0))
	}
	return builder.String(), nil
}

func (p *nestedTextParser) pushNonterm(isDict bool) {
	entry := parserStackEntry{
		Values: make([]interface{}, 0, 16),
	}
	if isDict { // dict
		entry.Keys = make([]string, 0, 16)
	}
	p.stack.push(&entry)
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
				"nestedtext": result,
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
	Text         string          // current line of NestedText
	TextPosition int             // position of reader in string
	Marker       int             // positional marker for start of key or value
	Input        *strings.Reader // reader for Text
	LineNo       int             // current input line number
	stack        pstack          // parser stack
	//stack        []parserStackEntry // parse stack
}

// newInlineParser creates a fresh inline parser instance.
func newInlineParser() *inlineItemParser {
	return &inlineItemParser{
		stack: make([]parserStackEntry, 0, 10),
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
		if isErrorState(state) {
			break
		} else if isNonterm(state) {
			nonterm := state
			p.pushNonterm(state)
			state = inlineStateMachine[state][chType]
			p.stack.tos().NontermState = inlineStateMachine[oldState][_S(nonterm)]
		}
		ok := inlineStateMachineActions[state](p, oldState, state, ch, w)
		if !ok {
			state = e // flag error by setting error state
			break
		}
		if isAccept(state) {
			result, err = p.stack.tos().ReduceToItem()
			if err != nil {
				p.stack.tos().Error = err
				state = e
				break
			}
			state = p.stack.tos().NontermState
			p.stack.pop()
			if len(p.stack) > 0 {
				p.stack.pushKV(p.stack.tos().Key, result)
			}
		}
		p.TextPosition += w
	}
	if isErrorState(state) {
		if err = p.stack[len(p.stack)-1].Error; err == nil {
			t := parserToken{ColNo: p.TextPosition, LineNo: p.LineNo}
			err = makeParsingError(&t, ErrCodeFormat, "format error")
		}
	}
	return
}

// pushNonterm pushes a new (empyt) stack entry onto the parser stack. Depending on wether
// the non-terminal represents a list item or a dict item, the .Keys slice will be initialized.
// This function will be called for every non-terminal encounterd during the parse run, i.e.,
// for the initial non-terminal as well as for every nested one.
func (p *inlineItemParser) pushNonterm(state inlineParserState) {
	entry := parserStackEntry{
		Values: make([]interface{}, 0, 16),
	}
	if state == _S1 { // dict
		entry.Keys = make([]string, 0, 16)
	}
	p.stack.push(&entry)
}

// --- Inline parser table ---------------------------------------------------

// The parser is driven by a prefix-automaton, moving over states identified by
// type inlineParserState
type inlineParserState int8

// For a diagram of the automata, please refer to automata.dot.
// States 1..9 are unnamed.
const e inlineParserState = -1   // error state
const _S1 inlineParserState = 11 // non-terminal S1
const _S2 inlineParserState = 12 // non-terminal S2
const _A1 inlineParserState = 13 // acceptance state A1
const _A2 inlineParserState = 14 // acceptance state A2

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
	return state == 5 || state == 10
}

const chClassCnt = 11

// Character classes:
//   A  ws \n ,  :  [  ]  {  }  _S(S1) _S(S2)
var inlineStateMachine = [...][chClassCnt]inlineParserState{
	{e, e, e, e, e, 7, e, 1, e, e, e},         // state 0, initial
	{2, 2, e, e, 3, e, e, e, _A1, e, e},       // state 1
	{2, 2, e, e, 3, e, e, e, e, e, e},         // state 2
	{4, 3, e, 6, e, _S2, e, _S1, _A1, 5, 5},   // state 3
	{4, 4, e, 6, e, e, e, e, _A1, e, e},       // state 4
	{e, 5, e, 6, e, e, e, e, _A1, e, e},       // state 5
	{2, 6, e, e, 3, e, e, e, e, e, e},         // state 6
	{9, 8, e, 7, 9, _S2, _A2, _S1, e, 10, 10}, // state 7
	{9, 8, e, 7, 9, _S2, _A2, _S1, e, 10, 10}, // state 8
	{9, 9, e, 7, 9, e, _A2, e, e, e, e},       // state 9
	{e, 10, e, 7, e, e, _A2, e, e, e, e},      // state 10
	{e, e, e, e, e, e, e, 1, e, e, e},         // state S1
	{e, e, e, e, e, 7, e, e, e, e, e},         // state S2
	{e, e, e, e, e, e, e, e, e, e, e},         // state A1
	{e, e, e, e, e, e, e, e, e, e, e},         // state A2
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
		return true
	},
	nop, // 2
	func(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool { // 3
		if from != 3 {
			key := p.Text[p.Marker:p.TextPosition]
			key = strings.TrimSpace(key)
			p.stack.tos().Key = &key
			p.Marker = p.TextPosition + w // get ready for value
		}
		return true
	},
	nop, // 4
	nop, // 5
	func(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool { // 6
		if from != 6 {
			if p.Marker > 0 && !isGhost(from) {
				p.appendStringValue(false)
			}
			p.stack.tos().Key = nil
			p.Marker = p.TextPosition + w // get ready for next key
		}
		return true
	},
	func(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool { // 7
		if ch == ',' && p.Marker > 0 && !isGhost(from) {
			p.appendStringValue(false)
		}
		p.Marker = p.TextPosition + w // get ready for next item
		return true
	},
	nop, // 8
	nop, // 9
	nop, // 10
	func(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool { // S1
		if from == 3 || from == 8 {
			p.Marker = 0
		}
		return true
	},
	func(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool { // S2
		if from == 3 || from == 8 {
			p.Marker = 0
		}
		return true
	},
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
	if p.stack.tos().Key != nil {
		value = strings.TrimSpace(value)
		p.stack.pushKV(p.stack.tos().Key, value)
	} else if !isAccept || len(value) > 0 || len(p.stack.tos().Values) > 0 {
		value = strings.TrimSpace(value)
		p.stack.pushKV(p.stack.tos().Key, value)
	}
}

// nop is a no-op state machine action.
func nop(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool {
	return true
}

func accept(p *inlineItemParser, from, to inlineParserState, ch rune, w int) bool {
	if p.Marker > 0 && !isGhost(from) {
		p.appendStringValue(true)
	}
	return true
}

// === Parser stack ==========================================================

// The parser manages a stack, with a stack entry for every non-terminal. The bottom
// stack entry represents the outermost item. Each successive nested item will trigger a
// stack-enty to be pushed onto the parser stack.
// Stack entries collect the information for an item, either a list or a dict.

type pstack []parserStackEntry // parse stack = slice of stack entries

func (s pstack) tos() *parserStackEntry {
	if len(s) > 0 {
		return &s[len(s)-1]
	}
	return nil
}

func (s *pstack) pop() (tos *parserStackEntry) {
	if len(*s) > 0 {
		tos = s.tos()
		*s = (*s)[:len(*s)-1]
	}
	return tos
}

func (s *pstack) push(e *parserStackEntry) (tos *parserStackEntry) {
	if len(*s) > 0 {
		tos = s.tos()
	}
	*s = append(*s, *e)
	return tos
}

// pushKV will push a value and an option key onto the stack by appending it to the
// top-most stack entry.
// The containing stack-entry has to be provided by a non-term (pushNonterm).
func (s *pstack) pushKV(str *string, val interface{}) bool {
	// if val != nil && str != nil {
	// 	fmt.Printf("# push( %s, %#v )\n", *str, val)
	// } else if val != nil {
	// 	fmt.Printf("# push( %#v )\n", val)
	// }
	if s == nil || len(*s) == 0 {
		panic("use of un-initialized parser stack")
	}
	tos := &(*s)[len(*s)-1]
	tos.Values = append(tos.Values, val)
	if str != nil {
		if tos.Keys == nil {
			//panic("top-most stack entry should not contain keys")
			return false
		}
		tos.Keys = append(tos.Keys, *str)
	}
	return true
}

// The parser manages a stack, with a stack entry for every non-terminal. The bottom
// stack entry represents the outermost item. Each successive nested item will trigger a
// stack-enty to be pushed onto the parser stack.
// Stack entries collect the information for an item, either a list or a dict.
type parserStackEntry struct {
	Values       []interface{}     // list of values, either list items or dict values
	Keys         []string          // list of keys, empty for list items
	Key          *string           // current key to set value for, if in a dict
	Error        error             // if error occured: remember it
	NontermState inlineParserState // sub-nonterm, or 0 for root entry (used for inline-parser only)
}

func (entry parserStackEntry) ReduceToItem() (interface{}, error) {
	if entry.Keys == nil {
		return entry.Values, nil
	}
	dict := make(map[string]interface{}, len(entry.Values))
	if len(entry.Keys) > 0 && len(entry.Values) != len(entry.Keys) {
		// error: mixed content = uneven count of keys and values
		// fmt.Printf("@ entry.keys   = %#v\n", entry.Keys)
		// fmt.Printf("@ entry.values = %#v\n", entry.Values)
		panic(fmt.Sprintf("mixed item: number of keys (%d) not equal to number of values (%d)",
			len(entry.Keys), len(entry.Values)))
	}
	for i, key := range entry.Keys {
		dict[key] = entry.Values[i]
	}
	return dict, nil
}

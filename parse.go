package nestext

import (
	"fmt"
	"strings"
)

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

func (p *inlineItemParser) parse(input string) (result interface{}, err error) {
	p.Text = input
	p.Input = strings.NewReader(input)
	p.stack = p.stack[:0]
	p.pushNonterm(initial)
	fmt.Printf("|stack| = %d\n", len(p.stack))
	//
	var state inlineParserState = initial
	for {
		ch, w, err := p.Input.ReadRune()
		if err != nil {
			err = wrapError(ErrCodeIO, "I/O-error reading inline item", err)
			return nil, err
		}
		chtype := inlineTokenFor(ch)
		state = inlineStateMachine[state][chtype]
		if isErrorState(state) {
			break
		}
		fmt.Printf("state = %d\n", state)
		fmt.Printf("|stack| = %d\n", len(p.stack))
		ok := inlineStateMachineActions[state](p, ch, w)
		if !ok {
			state = e // flag error by setting error state
			break
		}
		if isAccept(state) {
			break
		}
		p.TextPosition += w
	}
	if isErrorState(state) {
		err = p.stack[len(p.stack)-1].Error
	} else {
		result, err = p.stack[len(p.stack)-1].ReduceToItem()
	}
	return
}

func (p *inlineItemParser) pushNonterm(state inlineParserState) {
	entry := inlineStackEntry{
		Values: make([]interface{}, 0, 16),
	}
	if state == S2 { // dict
		entry.Strings = make([]string, 0, 16)
	}
	p.stack = append(p.stack, entry)
}

type inlineStackEntry struct {
	Values  []interface{} // values
	Strings []string      // keys, emtpy for list items
	Error   error         // if error occured: remember it
}

func (entry inlineStackEntry) ReduceToItem() (interface{}, error) {
	if entry.Strings == nil {
		return entry.Values, nil
	}
	dict := make(map[string]interface{}, len(entry.Values))
	if len(entry.Values) != len(entry.Strings) {
		// error: mixed content
		panic("mixed content")
	}
	for i, key := range entry.Strings {
		dict[key] = entry.Values[i]
	}
	return dict, nil
}

func (p *inlineItemParser) push(s *string, val interface{}) bool {
	if val != nil {
		fmt.Printf("push( %#v )\n", val)
	}
	tos := &p.stack[len(p.stack)-1]
	tos.Values = append(tos.Values, val)
	if s != nil {
		tos.Strings = append(tos.Strings, *s)
	}
	return true
}

// --- Inline parser table ---------------------------------------------------

type inlineParserState int8

const initial inlineParserState = 0
const e inlineParserState = -1
const S1 inlineParserState = 10
const S2 inlineParserState = 11
const A1 inlineParserState = 12
const A2 inlineParserState = 13

func isErrorState(state inlineParserState) bool {
	return state < 0
}

func isNonterm(state inlineParserState) bool {
	return state == S1 || state == S2
}

func isAccept(state inlineParserState) bool {
	return state == A1 || state == A2
}

//   A  \n ,  :  [  ]  {  }  S1 S2 A1 A2
var inlineStateMachine = [...][8]inlineParserState{
	{e, e, e, e, 7, e, 1, e},    // state 0, initial
	{2, e, e, 3, e, e, e, A1},   // state 1
	{2, e, e, 3, e, e, e, e},    // state 2
	{4, e, 6, e, S2, e, S1, A1}, // state 3
	{4, e, 6, e, e, e, e, A1},   // state 4
	{e, e, 6, e, e, e, e, A1},   // state 5
	{2, e, e, 3, e, e, e, A1},   // state 6
	{8, e, 7, 8, S2, A2, S1, e}, // state 7
	{8, e, 7, 8, e, A2, e, e},   // state 8
	{e, e, 7, e, e, A2, e, e},   // state 9
	{9, 9, 9, 9, 9, 9, 9, 9},    // state S1
	{9, 9, 9, 9, 9, 9, 9, 9},    // state S2
	{e, e, e, e, e, e, e, e},    // state A1
	{e, e, e, e, e, e, e, e},    // state A2
}

var inlineStateMachineActions = [...]func(p *inlineItemParser, ch rune, w int) bool{
	nop, // 0
	nop, // 1
	nop, // 2
	nop, // 3
	nop, // 4
	nop, // 5
	nop, // 6
	func(p *inlineItemParser, ch rune, w int) bool { // 7
		if ch == ',' {
			value := p.Text[p.Marker:p.TextPosition]
			p.push(nil, value)
		}
		p.Marker = p.TextPosition + w
		fmt.Printf("- Marker at %d\n", p.Marker)
		return true
	},
	nop,    // 8
	nop,    // 9
	nop,    // S1
	nop,    // S2
	accept, // A1
	func(p *inlineItemParser, ch rune, w int) bool { // A2
		// state 9 has to set Marker = 0
		if p.Marker > 0 {
			value := p.Text[p.Marker:p.TextPosition]
			p.push(nil, value)
		}
		return true
	},
}

func nop(p *inlineItemParser, ch rune, w int) bool {
	return true
}

func startValue(p *inlineItemParser, ch rune, w int) bool {
	return true
}

func parserError(p *inlineItemParser, ch rune, w int) bool {
	return false
}

func accept(p *inlineItemParser, ch rune, w int) bool {
	fmt.Println("ACCEPT")
	return true
}

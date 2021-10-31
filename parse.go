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
type inlineParser struct {
	Text  string
	Input *strings.Reader
	stack []itemBuilder
}

// newInlineParser creates a fresh inline parser instance.
func newInlineParser() *inlineParser {
	return &inlineParser{
		stack: make([]itemBuilder, 0, 10),
	}
}

func (p *inlineParser) parse(input string) (result map[string]interface{}, err error) {
	p.Text = input
	p.Input = strings.NewReader(input)
	//
	var ch rune
	var state inlineParserState
	for !isErrorState(state) {
		ch, _, err = p.Input.ReadRune()
		if err != nil {
			err = wrapError(ErrCodeIO, "I/O-error reading inline item", err)
			return nil, err
		}
		chtype := inlineTokenFor(ch)
		state = inlineStateMachine[state][chtype]
		if !isErrorState(state) {
			ok := inlineStateMachineActions[state](p.stack[len(p.stack)-1])
			if !ok {
				state = e // jump to error state
			}
		}
	}
	result, err = p.stack[len(p.stack)-1].Item()
	return map[string]interface{}{}, nil
}

type itemBuilder interface {
	SetKey(string)
	AppendValue(interface{})
	Item() (interface{}, error)
	SetError(error)
}

type listBuilder struct {
	List []interface{}
	Err  error
}

func (lb *listBuilder) SetKey(string) {
	fmt.Println("ERROR: LIST MAY NOT HAVE KEYS")
}

func (lb *listBuilder) AppendValue(value interface{}) {
	lb.List = append(lb.List, value)
}

func (lb *listBuilder) Item() (interface{}, error) {
	return lb.List, lb.Err
}

func (lb *listBuilder) SetError(err error) {
	lb.Err = err
}

type dictBuilder struct {
	Key  string
	Dict map[string]interface{}
	Err  error
}

func (db *dictBuilder) SetKey(key string) {
	db.Key = key
}

func (db *dictBuilder) AppendValue(value interface{}) {
	db.Dict[db.Key] = value
}

func (db *dictBuilder) Item() (interface{}, error) {
	return db.Dict, db.Err
}

func (db *dictBuilder) SetError(err error) {
	db.Err = err
}

var _ itemBuilder = &listBuilder{}
var _ itemBuilder = &dictBuilder{}

// --- Inline parser table ---------------------------------------------------

type inlineParserState int8

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
	{e, e, e, e, 1, e, 7, e},    // state 0, initial
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

var inlineStateMachineActions = [...]func(b itemBuilder) bool{
	parseError,
	parseError,
	parseError,
	parseError,
	parseError,
	parseError,
	parseError,
	parseError,
	parseError,
	parseError,
	accept,
	accept,
}

func parseError(b itemBuilder) bool {
	return false
}

func accept(b itemBuilder) bool {
	return true
}

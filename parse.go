package nestext

import "strings"

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
}

func newInlineParser(input string) *inlineParser {
	ip := &inlineParser{
		Text:  input,
		Input: strings.NewReader(input),
	}
	return ip
}

type state int8

const e state = 0
const S1 state = 10
const S2 state = 11
const A1 state = 12
const A2 state = 13

//   A  \n ,  :  [  ]  {  }  S1 S2 A1 A2
var inlineStateMachine = [...][8]state{
	{e, e, e, e, e, e, e, e},    // state 0
	{2, e, e, 3, e, e, e, A1},   // state 1
	{2, e, e, 3, e, e, e, e},    // state 2
	{4, e, 6, e, S2, e, S1, A1}, // state 3
	{4, e, 6, e, e, e, e, A1},   // state 4
	{e, e, 6, e, e, e, e, A1},   // state 5
	{2, e, e, 3, e, e, e, A1},   // state 6
	{8, e, 7, 8, S2, A2, S1, e}, // state 7
	{8, e, 7, 8, e, A2, e, e},   // state 8
	{e, e, 7, e, e, A2, e, e},   // state 9
	{9, 9, 9, 9, 9, 9, 9, 9},    // stat9 S1
	{9, 9, 9, 9, 9, 9, 9, 9},    // stat9 S2
	{e, e, e, e, e, e, e, e},    // state A1
	{e, e, e, e, e, e, e, e},    // state A2
}

var inlineSMActions = [...]func() bool{
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

func parseError() bool {
	return false
}

func accept() bool {
	return true
}

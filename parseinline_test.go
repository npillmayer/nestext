package nestext

import (
	"errors"
	"fmt"
	"testing"
)

func TestInlineParseEOF(t *testing.T) {
	p := newInlineParser()
	_, err := p.parse(_S2, "")
	if err != nil {
		nterr := &NestedTextError{}
		if errors.As(err, nterr) {
			t.Logf("error code = %d, error = %q", nterr.Code, nterr.msg)
			t.Logf("   wrapped = %q", nterr.Unwrap().Error())
		} else {
			t.Errorf("expected empty input to result in I/O error; didn't:")
			t.Errorf("error = %q", err.Error())
		}
	} else {
		t.Fatal("expected empty input to result in I/O error; didn't")
	}
}

func TestInlineParseItemsTable(t *testing.T) {
	p := newInlineParser()
	inputs := []struct {
		text    string
		initial inlineParserState
		output  string
	}{
		{"[]", _S2, "[]"},
		{"[x]", _S2, "[x]"},
		{"[x,y]", _S2, "[x y]"},
		{"[[]]", _S2, "[[]]"},
		{"{}", _S1, "map[]"},
		{"{a:x}", _S1, "map[a:x]"},
		{"{a: [x]}", _S1, "map[a:[x]]"},
		{"{a:[x,y] }", _S1, "map[a:[x y]]"},
		{"{a: {b: x} }", _S1, "map[a:map[b:x]]"},
		{"{ a : { A : 0 } , b : { B : 1 } }   ", _S1, "map[a:map[A:0] b:map[B:1]]"},
		{"{a: {b:0, c:1}, d: {e:2, f:3}}", _S1, "map[a:map[b:0 c:1] d:map[e:2 f:3]]"},
		{"[[11, 12, 13], [21, 22, 23]]", _S2, "[[11 12 13] [21 22 23]]"},
	}
	for i, input := range inputs {
		r, err := p.parse(input.initial, input.text)
		if err != nil {
			t.Errorf(err.Error())
		}
		t.Logf("[%2d] result = %v of type %#T", i, r, r)
		if fmt.Sprintf("%v", r) != input.output {
			t.Errorf("[%2d] however, expected %q", i, input.output)
		}
		t.Logf("------------------------------------------")
	}
}

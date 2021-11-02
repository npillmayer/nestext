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
		//{"[]", _S2, "[]"},
		//{"[x]", _S2, "[x]"},
		{"[x,y]", _S2, "[x y]"},
		//{"[[]]", S2, "[[]]"},
		//{"{}", _S1, "map[]"},
		//{"{a:x}", _S1, "map[a:b]"},
		//{"{a:[x,y]}", _S1, "map[a:[x y]]"},
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

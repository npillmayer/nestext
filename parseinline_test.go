package nestext

import (
	"errors"
	"testing"
)

func TestInlineParseEOF(t *testing.T) {
	ip := newInlineParser()
	_, err := ip.parse("")
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

func TestInlineParseEmptyList(t *testing.T) {
	ip := newInlineParser()
	r, err := ip.parse("[]")
	if err != nil {
		t.Errorf(err.Error())
	}
	t.Logf("result = %v of type %#T", r, r)
}

func TestInlineParseListOfOne(t *testing.T) {
	ip := newInlineParser()
	r, err := ip.parse("[x]")
	if err != nil {
		t.Errorf(err.Error())
	}
	t.Logf("result = %v of type %#T", r, r)
}

func TestInlineParseListOfTwo(t *testing.T) {
	ip := newInlineParser()
	r, err := ip.parse("[x,y]")
	if err != nil {
		t.Errorf(err.Error())
	}
	t.Logf("result = %v of type %#T", r, r)
}

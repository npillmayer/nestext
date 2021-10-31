package nestext

import "testing"

func TestInlineParserCreate(t *testing.T) {
	//
	_ = newInlineParser()
}

func TestInlineParseEmpty(t *testing.T) {
	ip := newInlineParser()
	r, err := ip.parse("")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("parse of '' = %#v", r)
}

package nestext

import (
	"strings"
	"testing"
)

func TestLineBufferRemainder(t *testing.T) {
	inputDoc := strings.NewReader("Hello World\nHow are you?")
	buf := newLineBuffer(inputDoc)
	for i := 0; i < 6; i++ {
		buf.AdvanceCursor()
	}
	r := buf.ReadLineRemainder()
	t.Logf("remainder = '%s'", r)
	if r != "World" {
		t.Errorf("expected remainder to be 'World', is '%s'", r)
	}
	r = buf.ReadLineRemainder()
	t.Logf("remainder = '%s'", r)
	if r != "How are you?" {
		t.Errorf("expected remainder to be 'How are you?', is '%s'", r)
	}
}

func TestScannerCreate(t *testing.T) {
	r := strings.NewReader("")
	_, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
}

func TestScannerStart(t *testing.T) {
	r := strings.NewReader("# This is a comment to skip\n")
	sc, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
	if !sc.Buf.isEof {
		t.Errorf("expected scanner to be at EOF, isn't")
	}
}

func TestScannerTopLevelIndent(t *testing.T) {
	r := strings.NewReader("# This is a comment\n   debug: false\n")
	sc, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
	tok := sc.NextToken()
	if tok.Error == nil {
		t.Errorf("tok.Error to reflect error code ErrCodeFormatToplevelIndent")
	}
}

func TestScannerItem(t *testing.T) {
	r := strings.NewReader("# This is a comment\n- debug\n")
	sc, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
	tok := sc.NextToken()
	if tok.Error != nil {
		t.Errorf("top-level document root expected to parse without error; didn't: %v", tok.Error)
	}
	tok = sc.NextToken()
	t.Logf("token = %v", tok)
	if tok.TokenType != listKey {
		t.Errorf("item expected to be of type list item; is: %s", tok.TokenType)
	}
	if tok.Content != "debug" {
		t.Errorf("item expected to have value 'debug'; is: %s", tok.Content)
	}
}

func TestScannerItemIllegal(t *testing.T) {
	r := strings.NewReader("# This is a comment\n-debug\n")
	sc, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
	sc.NextToken()        // doc root
	tok := sc.NextToken() // item
	logTag(tok, t)
	if tok.Error == nil {
		t.Errorf("item expected to have error; hasn't")
	} else {
		t.Logf("Error caught: %v", tok.Error)
	}
}

func TestScannerLongItem(t *testing.T) {
	r := strings.NewReader("# This is a comment\n-\n > debug\n")
	sc, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
	tok := sc.NextToken()
	if tok.Error != nil {
		t.Errorf("top-level document root expected to parse without error; didn't: %v", tok.Error)
	}
	tok = sc.NextToken()
	logTag(tok, t)
	if tok.TokenType != listKeyMultiline {
		t.Errorf("item expected to be of type multiline list item; is: %s", tok.TokenType)
	}
}

func TestScannerMultilineString(t *testing.T) {
	r := strings.NewReader("> Hello\n> World!\n")
	sc, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
	sc.NextToken()        // doc root
	tok := sc.NextToken() // string
	logTag(tok, t)
	if tok.TokenType != stringMultiline {
		t.Errorf("item expected to be of type multiline string; is: %s", tok.TokenType)
	}
	tok = sc.NextToken() // string
	logTag(tok, t)
	if tok.TokenType != stringMultiline {
		t.Errorf("item expected to be of type multiline string; is: %s", tok.TokenType)
	}
}

func TestScannerMultilineKey(t *testing.T) {
	r := strings.NewReader(": Hello\n: World!\n")
	sc, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
	sc.NextToken()        // doc root
	tok := sc.NextToken() // key
	logTag(tok, t)
	if tok.TokenType != dictKeyMultiline {
		t.Errorf("item expected to be of type multiline key; is: %s", tok.TokenType)
	}
	tok = sc.NextToken() // key
	logTag(tok, t)
	if tok.TokenType != dictKeyMultiline {
		t.Errorf("item expected to be of type multiline key; is: %s", tok.TokenType)
	}
}

// ---------------------------------------------------------------------------

func logTag(tag *parserTag, t *testing.T) {
	t.Logf("token = %v", tag)
	if tag.Error != nil {
		t.Logf("      + error: %v", tag.Error)
	}
}

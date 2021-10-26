package nestext

import (
	"strings"
	"testing"
)

func TestScannerCreate(t *testing.T) {
	r := strings.NewReader("# Test\ndebug: false\n")
	_, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
}

func TestScannerStart(t *testing.T) {
	r := strings.NewReader("# Test\ndebug: false\n")
	sc, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
	tok := sc.NextToken()
	if tok.TokenType != docRoot {
		t.Errorf("tok.type = %d, expected %d", tok.TokenType, docRoot)
	}
}

func TestScannerIndent(t *testing.T) {
	r := strings.NewReader("# Test\n   debug: false\n")
	sc, err := newScanner(r)
	if err != nil {
		t.Fatal(err)
	}
	tok := sc.NextToken()
	if tok.TokenType != docRoot {
		t.Errorf("tok.type = %d, expected %d", tok.TokenType, docRoot)
	}
}

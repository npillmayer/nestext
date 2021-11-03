package nestext

import (
	"strings"
	"testing"
)

func TestParseSimple(t *testing.T) {
	input := strings.NewReader("[x, y]\n")
	p := NewNestedTextParser()
	_, err := p.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
}

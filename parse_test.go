package nestext

import (
	"strings"
	"testing"
)

func TestParseSimple(t *testing.T) {
	p := NewNestedTextParser()
	inputs := []struct {
		text   string
		output string
	}{
		{"> Hello\n> World!\n: key\n", "[]"},
		//{"[x]", "[x]"},
		//{"[x,y]", "[x y]"},
		//{"[[]]" "[[]]"},
		//{"{}", "map[]"},
		//{"{a:x}", "map[a:b]"},
		//{"{a:[x,y]}", "map[a:[x y]]"},
	}
	for i, input := range inputs {
		//
		buf := strings.NewReader(input.text)
		result, err := p.Parse(buf)
		if err != nil {
			t.Errorf("[%2d] %s\n", i, err.Error())
		} else {
			t.Logf("[%2d] ( %v ) of type %T\n", i, result, result)
		}
	}
}

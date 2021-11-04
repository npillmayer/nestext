package nestext

import (
	"strings"
	"testing"
)

func TestParseSimple(t *testing.T) {
	p := NewNestedTextParser()
	inputs := []struct {
		text    string
		correct bool
	}{
		{"> Hello\n> World!\n: key\n", false}, // extra ':' line
		/* 		{`# multi-line list item
		- Hello
		-
		  > World
		  > !
		`, true}, */
		/* 		{`# multi-line dict
		a: Hello
		b: World
		`, true}, */
		{`# multi-line dict
a:
  > Hello World!
b: How are you?
`, true},
		//{"[x]", "[x]"},
		//{"[x,y]", "[x y]"},
		//{"[[]]" "[[]]"},
		//{"{}", "map[]"},
		//{"{a:x}", "map[a:b]"},
		//{"{a:[x,y]}", "map[a:[x y]]"},
	}
	for i, input := range inputs {
		buf := strings.NewReader(input.text)
		result, err := p.Parse(buf)
		if err == nil && !input.correct {
			t.Errorf("[%2d] expected error to occur, didn't", i)
		} else if err == nil {
			t.Logf("[%2d] ( %v ) of type %T\n", i, result, result)
		} else if err != nil && input.correct {
			t.Errorf("[%2d] %s\n", i, err.Error())
		} else {
			t.Logf("[%2d] got expected error: %s", i, err.Error())
		}
	}
}

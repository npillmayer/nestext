package nestext

import (
	"strings"
	"testing"
)

func TestTableParse(t *testing.T) {
	p := NewNestedTextParser()
	t.Logf("============================================================")
	inputs := []struct {
		text    string
		correct bool
	}{
		{`# string
> Hello
> World
`, true},
		{`# string with error
> Hello
> World!
: key
`, false}, // extra ':' line
		{`# multi-line list item
- Hello
-
  > World
  > !
`, true},
		{`# dict
a: Hello
b: World
`, true},
		{`# multi-line dict
a:
  > Hello World!
b: How are you?
`, true},
		{`# multi-line dict
: A
: a
  > Hello World!
b: How are you?
`, true},
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
		t.Logf("------------------------------------------------------------")
	}
}

package nestext

import (
	"fmt"
	"log"
	"strings"
	"testing"
)

func TestParserUsageError(t *testing.T) {
	_, err := Parse(strings.NewReader(""), TopLevel("dict.config"))
	if err != nil {
		t.Logf("got error = %v", err)
		t.Error("expected top-level 'dict.config' to be ok; produced error")
	}
	_, err = Parse(nil, TopLevel("dict-config"))
	if err == nil {
		t.Error("expected top-level 'dict-config' to produce an error; didn't")
	} else {
		t.Logf("got expected error = %v", err)
	}
}

func TestParserStack(t *testing.T) {
	p := newParser()
	p.pushNonterm(false)
	if ok := p.stack.pushKV(nil, "one"); !ok {
		t.Fatal("pushing a value onto the stack failed")
	}
	two := "2"
	if ok := p.stack.pushKV(&two, "two"); ok {
		t.Fatal("pushing a key onto the stack should fail, didn't")
	}
}

func TestSimpleDict(t *testing.T) {
	input := `
# Example for a dict
a: Hello
b: World
`
	result, err := Parse(strings.NewReader(input))
	if err != nil {
		log.Fatal("parsing failed")
	}
	fmt.Printf("result = %#v\n", result)
}

func TestTableParse(t *testing.T) {
	p := newParser()
	t.Logf("============================================================")
	inputs := []struct {
		text    string
		correct bool
	}{
		{`# strange key with :
-:: x
`, true},
		{`# string
> Hello
> World
`, true},
		{`# string with error
> Hello
> World!
: key
`, false}, // extra ':' line
		{`# inline dict with inline list
{a: [x]}
`, true},
		{`# inline dict in list
-
  {a: 0}
`, true},
		{`# inline dict with comma
{a: x, }
`, false},
		{`# empty dict entry
:
  >
`, true},
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
		{`# dict indent error
- Hello
  - World!
`, false},
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

func TestParseForExample(t *testing.T) {
	address := `
name: Katheryn McDaniel
address:
  > 138 Almond Street
  > Topeka, Kansas 20697
phone:
  cell: 1-210-555-5297
  home: 1-210-555-8470
    # Katheryn prefers that we always call her on her cell phone.
email: KateMcD@aol.com
additional roles:
  - board member
`
	result, err := Parse(strings.NewReader(address))
	if err != nil {
		t.Error(err)
	}
	dump(" ", result.(map[string]interface{}))
}

// ----------------------------------------------------------------------

func dump(space string, v interface{}) {
	fmt.Print(space)
	_dump(space, v)
}

func _dump(space string, v interface{}) {
	if m, ok := v.(map[string]interface{}); ok {
		fmt.Printf("{\n")
		for k, v := range m {
			fmt.Printf(space+"    "+"\"%v\": ", k)
			if s, ok := v.(string); ok {
				fmt.Printf("\"%v\":\n", s)
			} else {
				_dump(space+"    ", v)
			}
		}
		fmt.Printf(space + "}\n")
	} else if s, ok := v.(string); ok {
		fmt.Printf("%s\"%v\"\n", space, s)
	} else if l, ok := v.([]interface{}); ok {
		fmt.Printf("[\n")
		for _, lv := range l {
			_dump(space+"    ", lv)
		}
		fmt.Printf(space + "]\n")
	} else {
		fmt.Printf("%v%v\n", space, v)
	}
}

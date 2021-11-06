package encode

import (
	"os"
	"strings"
	"testing"
)

func TestEncodeOptions(t *testing.T) {
	Encode(nil, os.Stdout, IndentBy(5), InlineLimited(80))
}

func TestEncodeSimpleString(t *testing.T) {
	expect(t, 2, "Hello\nWorld")
}

func TestEncodeSimpleStringList(t *testing.T) {
	expect(t, 1, []string{"Hello", "World"})
}

func TestEncodeStringListWithComma(t *testing.T) {
	expect(t, 2, []string{"Hello", "Wo,rld"})
}

func TestEncodeSimpleNumberList(t *testing.T) {
	expect(t, 3, []interface{}{1, 2, 3})
}

func TestEncodeConcreteNumberList(t *testing.T) {
	expect(t, 1, []int{1, 2, 3})
}

func TestEncodeStringListWithLongString(t *testing.T) {
	expect(t, 6, []string{"Hello", "World", "How\nare\nyou?"})
}

func TestEncodeListOfObjects(t *testing.T) {
	expect(t, 2, []interface{}{4.1, 7.2})
}

func TestEncodeDict(t *testing.T) {
	expect(t, 4, map[string]string{"Hello": "World", "How": "are\nyou?"})
}

// ----------------------------------------------------------------------

func expect(t *testing.T, linecnt int, tree interface{}) {
	out := &strings.Builder{}
	Encode(tree, out)
	s := out.String()
	t.Logf("encoded:\n%s", s)
	n := strings.Count(s, "\n")
	if n != linecnt {
		t.Errorf("expected output to have %d lines, has %d", linecnt, n)
	}
}

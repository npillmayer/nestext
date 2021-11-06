package ntenc

import (
	"io"
	"strings"
	"testing"
)

func TestEncodeOptions(t *testing.T) {
	n, err := Encode("X", io.Discard, IndentBy(5), InlineLimited(80))
	if err != nil {
		t.Error(err)
	}
	if n != 4 { // "> X\n"
		t.Errorf("expected encoding to be of length 4, is %d", n)
	}
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
	expect(t, 4, map[string]string{"World": "Hello!", "How": "are\nyou?"})
}

func TestEncodeMultilineKeys(t *testing.T) {
	expect(t, 4, map[string]string{"Hello": "World", "How\nare": "you?"})
}

func TestEncodeNested(t *testing.T) {
	expect(t, 6, map[string]interface{}{
		"Key1": "Value1",
		"Key2": map[string]interface{}{
			"B": 2,
			"A": "a long\nstring",
		}})
}

func TestEncodeStruct(t *testing.T) {
	_, err := Encode(struct{ a int }{a: 1}, io.Discard)
	t.Logf("error for struct = %v", err)
	if err == nil {
		t.Error("expected encoding of struct to fail with error, didn't")
	}
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

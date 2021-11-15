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
	expect(t, "Hello\nWorld", `> Hello
> World
`)
}

func TestEncodeSimpleStringList(t *testing.T) {
	expect(t, []string{"Hello", "World"}, "[Hello, World]\n")
}

func TestEncodeStringListWithComma(t *testing.T) {
	expect(t, []string{"Hello", "Wo,rld"}, `- Hello
- Wo,rld
`)
}

func TestEncodeSimpleNumberList(t *testing.T) {
	expect(t, []interface{}{1, 2, 3}, `- 1
- 2
- 3
`)
}

func TestEncodeConcreteNumberList(t *testing.T) {
	expect(t, []int{1, 2, 3}, `[1, 2, 3]
`)
}

func TestEncodeStringListWithLongString(t *testing.T) {
	expect(t, []string{"Hello", "World", "How\nare\nyou?"}, `- Hello
- World
-
  > How
  > are
  > you?
`)
}

func TestEncodeListOfObjects(t *testing.T) {
	expect(t, []interface{}{4.1, 7.2}, `- 4.1
- 7.2
`)
}

func TestEncodeDict(t *testing.T) {
	expect(t, map[string]string{"World": "Hello!", "How": "are\nyou?"}, `How:
  > are
  > you?
World: Hello!
`)
}

func TestEncodeMultilineKeys(t *testing.T) {
	expect(t, map[string]string{"Hello": "World", "How\nare": "you?"}, `Hello: World
: How
: are
  > you?
`)
}

func TestEncodeNested(t *testing.T) {
	expect(t, map[string]interface{}{
		"Key1": "Value1",
		"Key2": map[string]interface{}{
			"B": 2,
			"A": "a long\nstring",
		}}, `Key1: Value1
Key2:
  A:
    > a long
    > string
  B: 2
`)
}

func TestEncodeStruct(t *testing.T) {
	_, err := Encode(struct{ a int }{a: 1}, io.Discard)
	t.Logf("error for struct = %v", err)
	if err == nil {
		t.Error("expected encoding of struct to fail with error, didn't")
	}
}

// ----------------------------------------------------------------------

func expect(t *testing.T, tree interface{}, target string) {
	out := &strings.Builder{}
	Encode(tree, out)
	str := out.String()
	t.Logf("encoded:\n%s", str)
	S := strings.Split(str, "\n")
	T := strings.Split(target, "\n")
	if len(S) != len(T) {
		t.Errorf("expected output to have %d lines, has %d", len(T), len(S))
	}
	t.Logf("S = %#v", S)
	t.Logf("T = %#v", T)
	for i, s := range S {
		if i >= len(T) {
			break
		}
		if T[i] != s {
			t.Errorf("%q != %q", s, T[i])
		}
	}
}

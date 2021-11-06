package encode

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	nestext "com.pillmayer.nestext"
)

// InlineLimit is the threshold above which lists and dicts are not encoded as inline lists/dicts.
const DefaultInlineLimit = 128

// MaxIndent is the maximum number of spaces used as indent.
// Indentation may be in the range of 1…MaxIndent.
const MaxIndent = 16

// --- Encoding ---------------------------------------------------------

// Encode encodes its argument `tree`, which has to be a string or a nested data-structure of
// `map[string]interface{}` and `[]interface{}`, as a byte stream in NestedText format.
//
// Encode won't handle structs, channels nor unsafe types.
//
func Encode(tree interface{}, w io.Writer, opts ...EncoderOption) (int, error) {
	enc := &encoder{indentSize: 2, inlineLimit: DefaultInlineLimit}
	for _, opt := range opts {
		opt(enc)
	}
	fmt.Printf("encoder = %#v\n", enc)
	return enc.encode(0, tree, w)
}

type encoder struct {
	indentSize  int
	inlineLimit int
}

func (enc *encoder) encode(indent int, tree interface{}, w io.Writer) (int, error) {
	if !isEncodable(tree) {
		return 0, nestext.MakeNestedTextError(nestext.ErrCodeSchema,
			fmt.Sprintf("unable to encode type %T", tree))
	}
	switch t := tree.(type) {
	case string:
		if strings.IndexByte(t, '\n') == -1 {
			enc.indent(w, indent)
			w.Write([]byte("> "))
			w.Write([]byte(t))
			w.Write([]byte{'\n'})
		} else {
			S := strings.Split(t, "\n")
			for _, s := range S {
				enc.indent(w, indent)
				w.Write([]byte("> "))
				w.Write([]byte(s))
				w.Write([]byte{'\n'})
			}
		}
	case []string:
		if len(t) <= 5 { // max of 5 is completely arbitrary
			l := 0
			inlineable := true
			S := make([][]byte, len(t))
			for i, item := range t {
				l += len(item)
				ok, s := isInlineable(item)
				inlineable = inlineable && ok
				if !inlineable || l > enc.inlineLimit {
					break
				}
				S[i] = s
			}
			if inlineable && l <= enc.inlineLimit {
				w.Write([]byte{'['})
				for i, item := range t {
					if i > 0 {
						w.Write([]byte{',', ' '})
					}
					w.Write([]byte(item))
				}
				w.Write([]byte{']', '\n'})
				break
			}
		}
		for _, s := range t {
			enc.indent(w, indent)
			w.Write([]byte{'-'})
			if strings.IndexByte(s, '\n') == -1 {
				w.Write([]byte{' '})
				w.Write([]byte(s))
				w.Write([]byte{'\n'})
			} else {
				w.Write([]byte{'\n'})
				enc.encode(indent+1, s, w)
			}
		}
	case []int:
		if len(t) <= 10 { // max of 10 is completely arbitrary
			w.Write([]byte{'['})
			for i, n := range t {
				if i > 0 {
					w.Write([]byte{',', ' '})
				}
				w.Write([]byte(strconv.Itoa(n)))
			}
			w.Write([]byte{']', '\n'})
			break
		}
		for _, n := range t {
			enc.indent(w, indent)
			w.Write([]byte("- "))
			w.Write([]byte(strconv.Itoa(n)))
			w.Write([]byte{'\n'})
		}
	case []interface{}:
		for _, item := range t {
			enc.indent(w, indent)
			w.Write([]byte("-"))
			if ok, itemAsBytes := isInlineable(item); ok {
				w.Write([]byte{' '})
				w.Write(itemAsBytes)
				w.Write([]byte{'\n'})
			} else {
				w.Write([]byte{'\n'})
				enc.encode(indent+1, item, w)
			}
		}
	default:
		enc.encodeReflected(indent, tree, w)
	}
	return 0, nil
}

func (enc *encoder) encodeReflected(indent int, tree interface{}, w io.Writer) (int, error) {
	v := reflect.ValueOf(tree)
	switch v.Kind() {
	case reflect.Slice:
		l := v.Len()
		fmt.Println("-------------------------------")
		for i := 0; i < l; i++ {
			item := v.Index(i).Interface()
			enc.indent(w, indent)
			w.Write([]byte{'-'})
			if ok, itemAsBytes := isInlineable(item); ok {
				w.Write([]byte{' '})
				w.Write(itemAsBytes)
				w.Write([]byte{'\n'})
			} else {
				w.Write([]byte{'\n'})
				enc.encode(indent+1, item, w)
			}
		}
	case reflect.Map:
		//l := v.Len()
		fmt.Println("-------------------------------")
		iter := v.MapRange()
		for iter.Next() {
			k := iter.Key()
			if k.Kind() != reflect.String {
				return 0, nestext.MakeNestedTextError(nestext.ErrCodeSchema,
					"map key is not a string; can only keys of type string")
			}
			key := k.Interface().(string)
			item := iter.Value().Interface()
			if ok, keyAsBytes := isInlineable(key); ok {
				enc.indent(w, indent)
				w.Write(keyAsBytes)
				w.Write([]byte{':'})
				if ok, itemAsBytes := isInlineable(item); ok {
					w.Write([]byte{' '})
					w.Write(itemAsBytes)
					w.Write([]byte{'\n'})
				} else {
					w.Write([]byte{'\n'})
					enc.encode(indent+1, item, w)
				}
			} else {
				panic("mulit-line keys not yet supported")
			}
		}
	default:
		panic(fmt.Sprintf("unsupported type: %T", tree))
	}
	return 0, nil
}

func isEncodable(item interface{}) bool {
	switch reflect.ValueOf(item).Kind() {
	case reflect.Chan, reflect.Func, reflect.Invalid, reflect.Uintptr, reflect.UnsafePointer:
		return false
	}
	return true
}

func isInlineable(item interface{}) (bool, []byte) {
	switch reflect.ValueOf(item).Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.Struct:
		return false, nil
	case reflect.String:
		if strings.ContainsAny(item.(string), "\n,:[]{}") {
			return false, nil
		}
		return true, []byte(item.(string))
	default:
	}
	v := fmt.Sprintf("%v", item)
	if strings.IndexByte(v, '\n') == -1 {
		return true, []byte(v)
	}
	return false, nil
}

// used for indentation
var spaces = [MaxIndent]byte{
	' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ',
}

func (enc *encoder) indent(w io.Writer, indent int) (int, error) {
	for i := 0; i < indent; i++ {
		w.Write(spaces[:enc.indentSize])
	}
	return enc.indentSize * indent, nil
}

// --- Encoding options -------------------------------------------------

// EncoderOption is a type to influence the behaviour of the encoding process.
// Multiple options may be passed to `Encode(…)`.
type EncoderOption _EncoderOption

type _EncoderOption func(*encoder) // internal synonym to hide unterlying type of options.

// IndentBy sets the number of spaces per indentation level. The default is 2.
// Allowed values are 1…MaxIndent
//
// Use as:
//     ntenc.Encode(mydata, w, ntenc.IndentBy(4))
//
func IndentBy(indentSize int) EncoderOption {
	return func(enc *encoder) {
		if indentSize < 1 {
			indentSize = 1
		} else if indentSize > MaxIndent {
			indentSize = MaxIndent
		}
		enc.indentSize = indentSize
	}
}

// InlineLimited sets the threshold above which lists and dicts are never inlined.
// If set to a small number, inlining is suppressed.
//
// Defaults to `DefaultInlineLimit`; may not exceed 2048.
//
// Use as:
//     ntenc.Encode(mydata, w, ntenc.InlineLimited(100))
//
func InlineLimited(limit int) EncoderOption {
	return func(enc *encoder) {
		if limit > 2048 {
			limit = 2048
		}
		enc.inlineLimit = limit
	}
}

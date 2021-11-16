// Package ntenc implements encoding of configuration data into NestedText format.
// Configuration data is a tree of map[string]interface{}, []interface{} and strings.
// It may not contain structs, channels nor unsafe types.
//
// This package is the counterpart to the NestedText parser (located in the base package
// of module `nestext`).
//
package ntenc

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/npillmayer/nestext"
)

// InlineLimit is the threshold above which lists and dicts are not encoded as inline lists/dicts.
const DefaultInlineLimit = 128

// MaxIndent is the maximum number of spaces used as indent.
// Indentation may be in the range of 1…MaxIndent.
const MaxIndent = 16

// --- Encoding ---------------------------------------------------------

// Encode encodes its argument `tree`, which has to be a string or a nested data-structure of
// `map[string]interface{}` and `[]interface{}`, as a byte stream in NestedText format.
// It returns the number of bytes written and possibly an error (of type nestext.NestedTextError).
//
// Map entries are sorted alphabetically by key.
//
// Encode won't handle structs, channels nor unsafe types.
//
func Encode(tree interface{}, w io.Writer, opts ...EncoderOption) (int, error) {
	enc := &encoder{indentSize: 2, inlineLimit: DefaultInlineLimit}
	for _, opt := range opts {
		opt(enc)
	}
	return enc.encode(0, tree, w, 0, nil)
}

type encoder struct {
	indentSize  int
	inlineLimit int
}

// encode is the top level function to encode data into NestedText format.
// It will be called recursively and therefore carries the current indentation depth
// as a parameter.
func (enc *encoder) encode(indent int, tree interface{}, w io.Writer, bcnt int, err error) (int, error) {
	if !isEncodable(tree) {
		return 0, nestext.MakeNestedTextError(nestext.ErrCodeSchema,
			fmt.Sprintf("unable to encode type %T", tree))
	}
	switch t := tree.(type) {
	// We first try a couple of standard-cases without relying on reflection
	case string:
		if ok, s := isInlineable(asString, t); ok {
			bcnt, err = enc.indent(w, bcnt, err, indent)
			bcnt, err = wr(w, bcnt, err, []byte("> "))
			bcnt, err = wr(w, bcnt, err, s)
			bcnt, err = wr(w, bcnt, err, []byte{'\n'})
		} else {
			S := strings.Split(t, "\n")
			for _, s := range S {
				bcnt, err = enc.indent(w, bcnt, err, indent)
				bcnt, err = wr(w, bcnt, err, []byte{'>', ' '})
				bcnt, err = wr(w, bcnt, err, []byte(s))
				bcnt, err = wr(w, bcnt, err, []byte{'\n'})
			}
		}
	case []string:
		if len(t) <= 5 { // max of 5 is completely arbitrary
			l := 0
			inlineable := true
			S := make([][]byte, len(t))
			for i, item := range t { // measure all list items
				l += len(item)
				ok, s := isInlineable(asList, item)
				inlineable = inlineable && ok
				if !inlineable || l > enc.inlineLimit {
					break // stop trying if not suited for inlining
				}
				S[i] = s
			}
			// if the complete array fits into one line, output "[ a, b, … ]"
			if inlineable && l <= enc.inlineLimit {
				bcnt, err = wr(w, bcnt, err, []byte{'['})
				for i, item := range t {
					if i > 0 {
						bcnt, err = wr(w, bcnt, err, []byte{',', ' '})
					}
					bcnt, err = wr(w, bcnt, err, []byte(item))
				}
				bcnt, err = wr(w, bcnt, err, []byte{']', '\n'})
				break
			}
		}
		// general case: list item with '-' as tag
		for _, s := range t {
			bcnt, err = enc.indent(w, bcnt, err, indent)
			bcnt, err = wr(w, bcnt, err, []byte{'-'})
			if strings.IndexByte(s, '\n') == -1 { // no newlines in string
				bcnt, err = wr(w, bcnt, err, []byte{' '})
				bcnt, err = wr(w, bcnt, err, []byte(s))
				bcnt, err = wr(w, bcnt, err, []byte{'\n'})
			} else { // contains newlines => item is multi-line string
				bcnt, err = wr(w, bcnt, err, []byte{'\n'})
				bcnt, err = enc.encode(indent+1, s, w, bcnt, err)
			}
		}
	case []int:
		if len(t) <= 10 { // max of 10 is completely arbitrary
			bcnt, err = wr(w, bcnt, err, []byte{'['})
			for i, n := range t {
				if i > 0 {
					bcnt, err = wr(w, bcnt, err, []byte{',', ' '})
				}
				bcnt, err = wr(w, bcnt, err, []byte(strconv.Itoa(n)))
			}
			bcnt, err = wr(w, bcnt, err, []byte{']', '\n'})
			break
		}
		for _, n := range t {
			bcnt, err = enc.indent(w, bcnt, err, indent)
			bcnt, err = wr(w, bcnt, err, []byte("- "))
			bcnt, err = wr(w, bcnt, err, []byte(strconv.Itoa(n)))
			bcnt, err = wr(w, bcnt, err, []byte{'\n'})
		}
	case []interface{}:
		for _, item := range t {
			bcnt, err = enc.indent(w, bcnt, err, indent)
			bcnt, err = wr(w, bcnt, err, []byte("-"))
			if ok, itemAsBytes := isInlineable(asList, item); ok {
				bcnt, err = wr(w, bcnt, err, []byte{' '})
				bcnt, err = wr(w, bcnt, err, itemAsBytes)
				bcnt, err = wr(w, bcnt, err, []byte{'\n'})
			} else {
				bcnt, err = wr(w, bcnt, err, []byte{'\n'})
				bcnt, err = enc.encode(indent+1, item, w, bcnt, err)
			}
		}
	default:
		bcnt, err = enc.encodeReflected(indent, tree, w, bcnt, err)
	}
	return bcnt, err
}

// encodeReflected encodes container types slice and map. As the name suggests,
// we use reflection to get detailled type information.
// The code here is not difficult in structure, but rather simply tedious for all the
// special cases.
func (enc *encoder) encodeReflected(indent int, tree interface{}, w io.Writer, bcnt int, err error) (int, error) {
	v := reflect.ValueOf(tree)
	switch v.Kind() {
	case reflect.Slice:
		l := v.Len()
		for i := 0; i < l; i++ {
			item := v.Index(i).Interface()
			bcnt, err = enc.indent(w, bcnt, err, indent)
			bcnt, err = wr(w, bcnt, err, []byte{'-'})
			if ok, itemAsBytes := isInlineable(asList, item); ok {
				bcnt, err = wr(w, bcnt, err, []byte{' '})
				bcnt, err = wr(w, bcnt, err, itemAsBytes)
				bcnt, err = wr(w, bcnt, err, []byte{'\n'})
			} else {
				bcnt, err = wr(w, bcnt, err, []byte{'\n'})
				bcnt, err = enc.encode(indent+1, item, w, bcnt, err)
			}
		}
	case reflect.Map:
		keys := v.MapKeys()
		// special case: empty map
		if len(keys) == 0 {
			return wr(w, bcnt, err, []byte("{}\n"))
		}
		// first sort items alphabetically by key
		sort.Slice(keys, func(i, j int) bool {
			return keys[i].String() < keys[j].String()
		})
		// for i, k := range keys {
		// 	fmt.Printf("@@@ [%d] keys = %#v\n", i, k.String())
		// }
		for _, k := range keys {
			if k.Kind() != reflect.String {
				return 0, nestext.MakeNestedTextError(nestext.ErrCodeSchema,
					"map key is not a string; can only keys of type string")
			}
			key := k.Interface().(string)
			item := v.MapIndex(k).Interface()
			if ok, keyAsBytes := isInlineable(asKey, key); ok {
				bcnt, err = enc.indent(w, bcnt, err, indent)
				bcnt, err = wr(w, bcnt, err, keyAsBytes)
				bcnt, err = wr(w, bcnt, err, []byte{':'})
				if ok, itemAsBytes := isInlineable(asString, item); ok {
					bcnt, err = wr(w, bcnt, err, []byte{' '})
					bcnt, err = wr(w, bcnt, err, itemAsBytes)
					bcnt, err = wr(w, bcnt, err, []byte{'\n'})
				} else {
					bcnt, err = wr(w, bcnt, err, []byte{'\n'})
					bcnt, err = encodeIfNotEmpty(enc, item, w, indent, bcnt, err)
					//bcnt, err = enc.encode(indent+1, item, w, bcnt, err)
				}
			} else { // output key as a multi-line key
				S := strings.Split(key, "\n")
				for _, s := range S {
					bcnt, err = enc.indent(w, bcnt, err, indent)
					if s == "" {
						bcnt, err = wr(w, bcnt, err, []byte(":"))
					} else {
						bcnt, err = wr(w, bcnt, err, []byte(": "))
						bcnt, err = wr(w, bcnt, err, []byte(s))
					}
					bcnt, err = wr(w, bcnt, err, []byte{'\n'})
				}
				bcnt, err = encodeIfNotEmpty(enc, item, w, indent, bcnt, err)
				//bcnt, err = enc.encode(indent+1, item, w, bcnt, err)
			}
		}
	default:
		err = nestext.MakeNestedTextError(nestext.ErrCodeSchema,
			fmt.Sprintf("unable to encode type %T", tree))
	}
	return bcnt, err
}

func encodeIfNotEmpty(enc *encoder, item interface{}, w io.Writer, indent, bcnt int, err error) (int, error) {
	if err != nil {
		return bcnt, err
	}
	if s, ok := item.(string); ok {
		if s == "" {
			return bcnt, err
		}
	}
	return enc.encode(indent+1, item, w, bcnt, err)
}

func isEncodable(item interface{}) bool {
	switch reflect.ValueOf(item).Kind() {
	case reflect.Chan, reflect.Func, reflect.Invalid, reflect.Uintptr, reflect.UnsafePointer:
		return false
	case reflect.Struct: // maybe we'll support this one day
		return false
	}
	return true
}

// item categories
const (
	asKey int = iota
	asString
	asList
	asDict
)

// itemPattern holds a string (list of characters) per item category which are
// forbidden for this item.
var itemPattern = []string{
	":\n",    // Key
	"\n",     // String
	"[],\n",  // List
	"{},:\n", // Dict
}

func isInlineable(what int, item interface{}) (bool, []byte) {
	switch reflect.ValueOf(item).Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.Struct:
		return false, nil
	case reflect.String:
		if item.(string) == "" {
			return false, nil
		}
		if strings.ContainsAny(item.(string), itemPattern[what]) {
			return false, nil
		}
		return true, []byte(item.(string))
	default:
		v := fmt.Sprintf("%v", item)
		if strings.ContainsAny(v, itemPattern[what]) {
			return false, nil
		}
		return true, []byte(v)
	}
}

// used for indentation
var spaces = [MaxIndent]byte{
	' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ',
}

// indent writes the correct amount of spaces for the current indentation level.
func (enc *encoder) indent(w io.Writer, bcnt int, err error, indent int) (int, error) {
	c := 0
	for i := 0; i < indent; i++ {
		c, err = wr(w, 0, err, spaces[:enc.indentSize])
		bcnt += c
	}
	return bcnt, err
}

// wr is a wrapper around w.Write(…). We wrap is to suppress the call if err is non-nil
// and to add up the count of written bytes.
func wr(w io.Writer, bcnt int, err error, data []byte) (int, error) {
	if err != nil {
		return bcnt, err
	}
	c, err := w.Write(data)
	if err != nil {
		err = nestext.WrapError(nestext.ErrCodeIO, "write error during encoding", err)
	}
	return bcnt + c, err
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

package nestext

import (
	"fmt"
	"io"
	"strings"
)

// TODO: set new ScanLines function which will break on 'CR' without following 'LF' (see spec)

// We will be using two different scanners:
//
// - a line-level scanner, concerned with recognizing lines of input as tokens.
//   These are to be consumed by a parser for the overall NestedText-grammar
// - an inline-scanner, concerned with recognizing words from inline-items' content.
//   These will be used to parse inline-items like "{ key1:value1, key2:value2 }"

// --- Line level scanner ----------------------------------------------------

// scanner is a type for a line-level scanner.
//
// Our line-level scanner will operate by calling scanning steps in a chain, iteratively.
// Each step function tests for valid lookahead and then possibly branches out to a
// subsequent step function. Step functions may consume input characters ("match(…)").
//
type scanner struct {
	Buf       *lineBuffer // line buffer abstracts away properties of input readers
	Step      scannerStep // the next scanner step to execute in a chain
	LastError error       // last error, if any
}

// We're buiding up a scanner from chains of scanner step functions.
// Tokens may be modified by a step function.
// A scanner step will return the next step in the chain, or nil to stop/accept.
//
type scannerStep func(*parserToken) (*parserToken, scannerStep)

// newScanner creates a scanner for an input reader.
func newScanner(inputReader io.Reader) (*scanner, error) {
	if inputReader == nil {
		return nil, makeParsingError(nil, ErrCodeFormatNoInput, "no input present")
	}
	buf := newLineBuffer(inputReader)
	sc := &scanner{Buf: buf}
	sc.Step = sc.ScanFileStart
	return sc, nil
}

// NextToken will be called by the parser to receive the next line-level token. A token
// subsumes the properties of a line of NestedText input (excluding inline-items such
// as "{ key:val, key:val }" ).
//
// NextToken ususally will iterate over a chain of step functions until it reaches an
// accepting state. Acceptance is signalled by getting a nil-step return value from a
// step function, meaning there is no further step applicable in this chain.
//
// If a step function returns an error-signalling token, the chaining stops as well.
//
func (sc *scanner) NextToken() *parserToken {
	token := newParserToken(sc.Buf.CurrentLine, int(sc.Buf.Cursor))
	if sc.Buf.IsEof() {
		token.TokenType = eof
		return token
	}
	if sc.Step == nil {
		sc.Step = sc.ScanItem
	}
	for sc.Step != nil {
		token, sc.Step = sc.Step(token)
		if token.Error != nil {
			sc.LastError = token.Error
			sc.Buf.AdvanceLine()
			break
		}
		if sc.Buf.Line.Size() == 0 {
			//fmt.Printf("===> line empty\n")
			break
		}
	}
	//fmt.Printf("# new %s\n", token)
	return token
}

// ScanFileStart matches a valid start of a NestedText document input. This is always the
// first step function to call.
//
//    file start:
//      -> EOF:   emptyDocument
//      -> other: docRoot
//
func (sc *scanner) ScanFileStart(token *parserToken) (*parserToken, scannerStep) {
	token.TokenType = emptyDocument
	if sc.Buf == nil {
		token.Error = makeParsingError(token, ErrCodeFormatNoInput, "no valid input document")
		return token, nil
	}
	if sc.Buf.IsEof() {
		return token, nil
	}
	token.TokenType = docRoot
	token.Indent = 0
	if sc.Buf.Lookahead == ' ' {
		// From the spec: There is no indentation on the top-level object.
		token.Error = makeParsingError(token, ErrCodeFormatToplevelIndent, "top-level item must not be indented")
	}
	return token, nil
}

// StepItem is a step function to start recognizing a line-level item.
func (sc *scanner) ScanItem(token *parserToken) (*parserToken, scannerStep) {
	//fmt.Println("---> ScanItem")
	if sc.Buf.Lookahead == ' ' {
		return token, sc.ScanIndentation
	}
	return token, sc.ScanItemBody
}

// ScanIndentation is a step function to recognize the indentation part of an item.
func (sc *scanner) ScanIndentation(token *parserToken) (*parserToken, scannerStep) {
	if sc.Buf.Lookahead == ' ' {
		sc.Buf.match(singleRune(' '))
		token.Indent++
		return token, sc.ScanIndentation
	}
	return token, sc.ScanItemBody
}

// ScanItemBody is a step function to recognize the main part of an item, starting at
// the item's tag (e.g., ':', '>', etc.). The only exception are inline keys and inline key-value-pairs,
// which start with the key's string.
//
func (sc *scanner) ScanItemBody(token *parserToken) (*parserToken, scannerStep) {
	//fmt.Printf("---> ScanItemBody, LA = '%#U'\n", sc.Buf.Lookahead)
	switch sc.Buf.Lookahead {
	case '-': // list value, either single-line or multi-line. From the spec:
		// If the first non-space character on a line is a dash followed immediately by a space (-␣) or
		// a line break, the line is a list item.
		sc.Buf.match(singleRune('-'))
		switch sc.Buf.Lookahead {
		case ' ', '\n': // yes, this is a valid list tag
			return sc.recognizeItemTag('-', listItem, listItemMultiline, token), nil
		default: // rare case: '-' as start of a dict key
			return token, sc.ScanInlineKey
		}
	case '>': // multi-line string. From the spec:
		// If the first non-space character on a line is a greater-than symbol followed immediately by
		// a space (>␣) or a line break, the line is a string item.
		sc.Buf.match(singleRune('>'))
		switch sc.Buf.Lookahead {
		case ' ', '\n': // yes, this is a valid string tag
			return sc.recognizeItemTag('>', stringMultiline, stringMultiline, token), nil
		default: // rare case: '>' as start of a dict key
			return token, sc.ScanInlineKey
		}
	case ':': // multi-line key. From the spec:
		// If the first non-space character on a line is a colon followed immediately by a space (:␣) or
		// a line break, the line is a key item.
		sc.Buf.match(singleRune(':'))
		switch sc.Buf.Lookahead {
		case ' ', '\n': // yes, this is a valid dict-key tag
			return sc.recognizeItemTag(':', dictKeyMultiline, dictKeyMultiline, token), nil
		default: // rare case: ':' as start of a dict-key
			return token, sc.ScanInlineKey
		}
	case '[': // single-line list
		return sc.recognizeInlineItem(inlineList, token), nil
	case '{': // single-line dictionary
		return sc.recognizeInlineItem(inlineDict, token), nil
	default: // should be dictionary key
	}
	return token, sc.ScanInlineKey // 'epsilon-transition' to inline-key-value rules
}

// ScanInlineKey is a step function to recognize an inline key, optionally followed by an inline
// value.
func (sc *scanner) ScanInlineKey(token *parserToken) (*parserToken, scannerStep) {
	switch sc.Buf.Lookahead { // consume characters; stop on ': ', ':\n' or EOL
	case ':':
		//fmt.Printf("@ LA = %#U, line = %q, at %d\n", sc.Buf.Lookahead, sc.Buf.Text, sc.Buf.Cursor)
		sc.Buf.match(singleRune(':'))
		//fmt.Printf("LA = %#U, line = %q, at %d\n", sc.Buf.Lookahead, sc.Buf.Text, sc.Buf.Cursor)
		switch sc.Buf.Lookahead {
		case ' ': // yes, this is a valid dict-key tag
			//fmt.Printf("LA = %#U, line = %q, at %d\n", sc.Buf.Lookahead, sc.Buf.Text, sc.Buf.Cursor)
			//fmt.Println("should fork --->")
			// remove trailing whitespace from key (=> Content[0])
			key := sc.Buf.Text[token.Indent : sc.Buf.ByteCursor-2]
			token.Content = append(token.Content, strings.TrimSpace(key))
			token = sc.recognizeItemTag(':', inlineDictKeyValue, inlineDictKey, token)
		case eolMarker: // yes, this is a valid dict-key tag
			// remove trailing whitespace from key (=> Content[0])
			key := sc.Buf.Text[token.Indent : sc.Buf.ByteCursor-1]
			token.Content = append(token.Content, strings.TrimSpace(key))
			token = sc.recognizeItemTag(':', inlineDictKeyValue, inlineDictKey, token)
		default: // rare case: ':' inside a dict key
			//sc.Buf.match(anything())
			//panic(fmt.Sprintf(":: found: LA=%#U\n", sc.Buf.Lookahead))
			return token, sc.ScanInlineKey
		}
	case eolMarker: // Error: premature end of line
		key := sc.Buf.Text[token.Indent : sc.Buf.ByteCursor-1]
		token.Error = makeParsingError(token, ErrCodeFormatIllegalTag,
			fmt.Sprintf("dict key item %q not properly terminated by ':'", key))
		//fmt.Printf("LA = %#U, line = %q, at %d\n", sc.Buf.Lookahead, sc.Buf.Text, sc.Buf.Cursor)
	default: // recognize everything as either part of the key or trailing whitespace
		sc.Buf.match(anything())
		return token, sc.ScanInlineKey
	}
	return token, nil
}

// recognizeItemTag continues after a valid item tag has been discovered. It will
// match the second character of the tag (either a space or a newline) and,
// depending on this character, select the continuation call.
//
func (sc *scanner) recognizeItemTag(tag rune, single, multi parserTokenType, token *parserToken) *parserToken {
	//fmt.Printf("forked: LA = %#U, line = %q, at %d\n", sc.Buf.Lookahead, sc.Buf.Text, sc.Buf.Cursor)
	// sc.Buf.match(singleRune(tag)) // changed: now already match by calling party
	if sc.Buf.Lookahead != ' ' && sc.Buf.Lookahead != eolMarker {
		token.Error = makeParsingError(token, ErrCodeFormatIllegalTag,
			fmt.Sprintf("item tag %q followed by illegal character %#U", tag, sc.Buf.Lookahead))
		return token
	}
	if sc.Buf.Lookahead == ' ' {
		sc.Buf.match(singleRune(' '))
		token.TokenType = single
		token.Content = append(token.Content, sc.Buf.ReadLineRemainder())
		return token
	}
	sc.Buf.match(singleRune(eolMarker))
	token.TokenType = multi
	return token
}

func (sc *scanner) recognizeInlineItem(toktype parserTokenType, token *parserToken) *parserToken {
	trimmed := strings.TrimSpace(sc.Buf.Text)
	closing := trimmed[len(trimmed)-1]
	//closing := sc.Buf.Text[len(sc.Buf.Text)-1]
	if !isMatchingBracket(sc.Buf.Lookahead, rune(closing)) {
		token.Error = makeParsingError(token, ErrCodeFormatIllegalTag,
			fmt.Sprintf("inline-item does not match opening tag: %#U vs %#U",
				sc.Buf.Lookahead, rune(closing)))
	}
	token.TokenType = toktype
	token.Content = append(token.Content, sc.Buf.ReadLineRemainder())
	return token
}

func isMatchingBracket(open, close rune) bool {
	if open == '[' {
		return close == ']'
	}
	if open == '{' {
		return close == '}'
	}
	return false
}

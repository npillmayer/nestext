package nestext

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// TODO: set new ScanLines function which will break on 'CR' without following 'LF'

// --- Input text buffer -----------------------------------------------------

// lineBuffer is an abstraction of a NestedText document source.
// The scanner will use a lineBuffer for input.
type lineBuffer struct {
	Lookahead   rune           // the next UTF-8 character
	Cursor      int64          // position of lookahead in character count
	ByteCursor  int64          // position of lookahead in byte count
	CurrentLine int            // current line number, starting at 1 (= next "expected line")
	Input       *bufio.Scanner // we use this to break up input into lines
	Text        string         // holds a copy of Input
	Line        strings.Reader // reader on Text
	isEof       bool           // is this buffer done reading?
	LastError   error          // last error, if any (except EOF errors)
}

const eolMarker = '\n'

var errAtEof error = errors.New("EOF")

func newLineBuffer(inputDoc io.Reader) *lineBuffer {
	input := bufio.NewScanner(inputDoc)
	buf := &lineBuffer{Input: input}
	err := buf.AdvanceLine()
	if err != errAtEof {
		buf.LastError = err
	}
	return buf
}

func (buf *lineBuffer) IsEof() bool {
	return buf.isEof && buf.ByteCursor >= buf.Line.Size()
}

func (buf *lineBuffer) AdvanceCursor() error {
	if buf.isEof {
		return errAtEof
	}
	if buf.ByteCursor >= buf.Line.Size() { // at end of line, set lookahead to eolMarker
		buf.Lookahead = eolMarker
	} else {
		r, err := buf.readRune()
		if err != nil {
			return err
		}
		buf.Lookahead = r
	}
	return nil
}

func (buf *lineBuffer) readRune() (rune, error) {
	r, runeLen, readerErr := buf.Line.ReadRune()
	if readerErr != nil {
		return 0, wrapError(ErrCodeIO, "I/O error while reading input character", readerErr)
	}
	buf.ByteCursor += int64(runeLen)
	buf.Cursor++
	return r, nil
}

// AdvanceLine will advance the input buffer to the next line. Will return atEof if EOF has been
// encountered.
//
// Blank lines and comment lines are skipped. This may be a somewhat questionable decision in terms
// of separation of concerns, as empty lines and comments are artifacts for which the scanner should
// take care of. However, it makes implemeting the scanner rules much more convenient
//
// Lookahead will be set to first rune (UFT-8 character) of the resulting current line.
// Line-count and cursor are updated.
//
func (buf *lineBuffer) AdvanceLine() error {
	buf.Cursor = 0
	buf.ByteCursor = 0
	// iterate over the lines of the input document until valid line found or EOF
	for !buf.isEof {
		buf.CurrentLine++
		fmt.Printf("===> reading line #%d\n", buf.CurrentLine)
		if !buf.Input.Scan() { // could not read a new line: either I/O-error or EOF
			if err := buf.Input.Err(); err != nil {
				return wrapError(ErrCodeIO, "I/O error while reading input", err)
			}
			fmt.Println("===> EOF !")
			buf.isEof = true
			buf.Line = *strings.NewReader("")
			return errAtEof
		}
		buf.Text = buf.Input.Text()
		if !buf.IsIgnoredLine() {
			buf.Line = *strings.NewReader(buf.Text)
			break
		}
	}
	buf.Line = *strings.NewReader(buf.Text)
	return buf.AdvanceCursor()
}

var blankPattern *regexp.Regexp
var commentPattern *regexp.Regexp

// IsIgnoredLine is a predicate for the current line of input. From the spec:
// Blank lines are lines that are empty or consist only of white space characters (spaces or tabs).
// Comments are lines that have # as the first non-white-space character on the line.
func (buf *lineBuffer) IsIgnoredLine() bool {
	if blankPattern == nil {
		blankPattern = regexp.MustCompile(`^\s*$`)
		commentPattern = regexp.MustCompile(`^\s*#`)
	}
	if blankPattern.MatchString(buf.Text) || commentPattern.MatchString(buf.Text) {
		return true
	}
	return false
}

// ReadRemainder returns the remainder of the current line of input text.
// This is a frequent operation for NestedText items.
func (buf *lineBuffer) ReadLineRemainder() string {
	var s string
	if buf.IsEof() {
		s = ""
	} else if buf.ByteCursor == buf.Line.Size() {
		s = string(buf.Lookahead)
	} else if buf.ByteCursor > buf.Line.Size() {
		s = ""
	} else {
		s = string(buf.Lookahead) + buf.Text[buf.ByteCursor:buf.Line.Size()]
	}
	buf.LastError = buf.AdvanceLine()
	return s
}

// The scanner has to match UTF-8 characters (runes) from the input. Matching is done using
// predicate functions (instead of direct comparison).
//
// singleRune returns a predicate to match a single rune
func singleRune(r rune) func(rune) bool {
	return func(arg rune) bool {
		return arg == r
	}
}

// anyRuneOf retuns a predicate to match a single rune out of a set of runes.
func anyRuneOf(runes ...rune) func(rune) bool {
	return func(arg rune) bool {
		for _, r := range runes {
			if arg == r {
				return true
			}
		}
		return false
	}
}

func (buf *lineBuffer) match(predicate func(rune) bool) bool {
	if buf.IsEof() || buf.LastError != nil {
		return false
	}
	if !predicate(buf.Lookahead) {
		return false
	}
	var err error
	if buf.Lookahead == eolMarker {
		err = buf.AdvanceLine()
	} else {
		err = buf.AdvanceCursor()
	}
	if err != nil && err != errAtEof {
		buf.LastError = err
		return false
	}
	return true
}

// --- Scanner ---------------------------------------------------------------

type scanner struct {
	Buf       *lineBuffer
	Step      scannerStep
	LastError error
	Levels    []int // stack of indents
}

// We're buiding up a scanner from chains of scanner step functions.
type scannerStep func(*parserToken) (*parserToken, scannerStep)

// newScanner creates a scanner for an input reader.
func newScanner(inputReader io.Reader) (*scanner, error) {
	if inputReader == nil {
		return nil, makeNestedTextError(nil, ErrCodeFormatNoInput, "no input present")
	}
	buf := newLineBuffer(inputReader)
	sc := &scanner{Buf: buf}
	sc.Step = sc.ScanFileStart
	return sc, nil
}

func (sc *scanner) NextToken() *parserToken {
	token := &parserToken{LineNo: sc.Buf.CurrentLine, ColNo: int(sc.Buf.Cursor)}
	if sc.Step == nil {
		sc.Step = sc.ScanItem
	}
	for sc.Step != nil {
		token, sc.Step = sc.Step(token)
		if token.Error != nil {
			sc.LastError = token.Error
			break
		}
	}
	return token
}

func (sc *scanner) fastPath(rule scannerStep, callback func(*parserToken)) *parserToken {
	token := &parserToken{LineNo: sc.Buf.CurrentLine, ColNo: int(sc.Buf.Cursor)}
	for rule != nil {
		token, rule = rule(token)
		if token.Error != nil {
			break
		}
	}
	return token
}

// scanFileStart matches a valid start of a NestedText document input.
//
//    file start:
//      -> EOF:   emptyDocument
//      -> other: docRoot
//
func (sc *scanner) ScanFileStart(token *parserToken) (*parserToken, scannerStep) {
	token.TokenType = emptyDocument
	if sc.Buf == nil {
		token.Error = makeNestedTextError(token, ErrCodeFormatNoInput, "no valid input document")
		return token, nil
	}
	if sc.Buf.IsEof() {
		return token, nil
	}
	token.TokenType = docRoot
	token.Indent = 0
	if sc.Buf.Lookahead == ' ' {
		// From the spec: There is no indentation on the top-level object.
		token.Error = makeNestedTextError(token, ErrCodeFormatToplevelIndent, "top-level item must not be indented")
	}
	return token, nil
}

func (sc *scanner) ScanItem(token *parserToken) (*parserToken, scannerStep) {
	fmt.Println("---> ScanItem")
	if sc.Buf.Lookahead == ' ' {
		return token, sc.ScanIndentation
	}
	return token, sc.ScanItemBody
}

func (sc *scanner) ScanIndentation(token *parserToken) (*parserToken, scannerStep) {
	if sc.Buf.Lookahead == ' ' {
		sc.Buf.match(singleRune(' '))
		token.Indent++
		return token, sc.ScanIndentation
	}
	return token, sc.ScanItemBody
}

func (sc *scanner) ScanItemBody(token *parserToken) (*parserToken, scannerStep) {
	fmt.Printf("---> ScanItemBody, LA = '%#U'\n", sc.Buf.Lookahead)
	switch sc.Buf.Lookahead {
	case '-': // list value, either single-line or multi-line. From the spec:
		// If the first non-space character on a line is a dash followed immediately by a space (-␣) or
		// a line break, the line is a list item.
		return sc.recognizeItemTag('-', listItem, listItemMultiline, token), nil
	case '>': // multi-line string. From the spec:
		// If the first non-space character on a line is a greater-than symbol followed immediately by
		// a space (>␣) or a line break, the line is a string item.
		return sc.recognizeItemTag('>', stringMultiline, stringMultiline, token), nil
	case ':': // multi-line key. From the spec:
		// If the first non-space character on a line is a colon followed immediately by a space (:␣) or
		// a line break, the line is a key item.
		return sc.recognizeItemTag(':', dictKeyMultiline, dictKeyMultiline, token), nil
	case '[': // single-line list
		return sc.recognizeInlineItem(inlineList, token), nil
	case '{': // single-line dictionary
		return sc.recognizeInlineItem(inlineDict, token), nil
	default: // should be dictionary key
	}
	return token, sc.ScanInlineKey // 'epsilon-transition' to inline-key-value rules
}

func (sc *scanner) ScanInlineKey(token *parserToken) (*parserToken, scannerStep) {
	switch sc.Buf.Lookahead {
	case ':':
		sc.Buf.match(singleRune(':'))
	case eolMarker:
	default: // recognize everything as either part of the key or trailing whitespace
	}
	return token, nil
}

func (sc *scanner) recognizeItemTag(tag rune, single, multi parserTokenType, token *parserToken) *parserToken {
	sc.Buf.match(singleRune(tag))
	if sc.Buf.Lookahead == ' ' {
		sc.Buf.match(singleRune(' '))
		token.TokenType = single
		token.Content = sc.Buf.ReadLineRemainder()
		return token
	}
	if sc.Buf.Lookahead != eolMarker {
		token.Error = makeNestedTextError(token, ErrCodeFormatIllegalTag,
			fmt.Sprintf("item tag %q followed by illegal character %#U", tag, sc.Buf.Lookahead))
		return token
	}
	sc.Buf.match(singleRune(eolMarker))
	token.TokenType = multi
	return token
}

func (sc *scanner) recognizeInlineItem(toktype parserTokenType, token *parserToken) *parserToken {
	closing := sc.Buf.Text[len(sc.Buf.Text)-1]
	if rune(closing) != sc.Buf.Lookahead {
		token.Error = makeNestedTextError(token, ErrCodeFormatIllegalTag,
			"inline-item does not match opening tag")
	}
	token.TokenType = toktype
	token.Content = sc.Buf.ReadLineRemainder()
	return token
}

// --- Helpers ---------------------------------------------------------------

func makeNestedTextError(token *parserToken, code int, errMsg string) *NestedTextError {
	err := &NestedTextError{
		Code: code,
		msg:  errMsg,
	}
	if token != nil {
		err.Line = token.LineNo
		err.Column = token.ColNo
	}
	return err
}

func wrapError(code int, errMsg string, err error) *NestedTextError {
	e := makeNestedTextError(nil, code, errMsg)
	e.wrappedError = err
	return e
}

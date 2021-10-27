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
// TODO: empty lines are allowed to contain whitespace

// --- Error type ------------------------------------------------------------

// NestedTextError is a custom error type for working with NestedText instances.
type NestedTextError struct {
	Code         int // error code
	Line, Column int // error position
	msg          string
	wrappedError error
}

const (
	NoError       = 0
	ErrCodeIO     = 10  // will wrap an underlying I/O error
	ErrCodeSchema = 100 // schema violation; may wrap an underlying error
	// all NestedText format errors have code >= ErrCodeFormat
	ErrCodeFormat               = 200 + iota // NestedText format error
	ErrCodeFormatNoInput                     // NestedText format error: no input present
	ErrCodeFormatToplevelIndent              // NestedText format error: top-level item was indented
)

// Error produces an error message from a NestedText error.
func (e *NestedTextError) Error() string {
	return fmt.Sprintf("[%d,%d] %s", e.Line, e.Column, e.msg)
}

// Unwrap returns an optionally present underlying error condition, e.g., an I/O-Error.
func (e NestedTextError) Unwrap() error {
	return e.wrappedError
}

// --- Enums -----------------------------------------------------------------

type ParserTokenType int8

const (
	undefined ParserTokenType = iota
	eof
	docRoot
	emptyDocument
	atomString
	atomKey
	atomValue
	inlineString
	inlineList
	inlineDict
	multiString
	multiList
	multiKey
)

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
		if !buf.Input.Scan() { // could not read a new line: either I/O-error or EOF
			if err := buf.Input.Err(); err != nil {
				return wrapError(ErrCodeIO, "I/O error while reading input", err)
			}
			buf.isEof = true
			buf.Line = *strings.NewReader("")
			return errAtEof
		}
		buf.Text = buf.Input.Text()
		if buf.IsIgnoredLine() {
			continue
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
	if buf.isEof || buf.LastError != nil {
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
	if err != errAtEof {
		buf.LastError = err
		return false
	}
	return true
}

// --- Scanner ---------------------------------------------------------------

type parserTag struct {
	LineNo    int
	ColNo     int
	TokenType ParserTokenType
	Indent    int
	Content   string
	Error     error
}

type scanner struct {
	Buf       *lineBuffer
	Step      scannerStep
	LastError error
	Levels    []int // stack of indents
}

// We're buiding up a scanner from chains of scanner step functions.
type scannerStep func(*parserTag, *lineBuffer) (*parserTag, scannerStep, *lineBuffer)

// newScanner creates a scanner for an input reader.
func newScanner(inputReader io.Reader) (*scanner, error) {
	if inputReader == nil {
		return nil, makeNestedTextError(nil, ErrCodeFormatNoInput, "no input present")
	}
	buf := newLineBuffer(inputReader)
	return &scanner{Buf: buf, Step: scanFileStart}, nil
}

func (sc *scanner) NextToken() *parserTag {
	tag := &parserTag{}
	for sc.Step != nil {
		tag, sc.Step, sc.Buf = sc.Step(tag, sc.Buf)
		if tag.Error != nil {
			sc.LastError = tag.Error
			break
		}
	}
	sc.Step = scanItem // prepare next run
	return tag
}

// scanFileStart matches a valid start of a NestedText document input.
//
//    file start:
//      -> EOF:   empty document
//      -> other: docRoot
//
func scanFileStart(tag *parserTag, input *lineBuffer) (*parserTag, scannerStep, *lineBuffer) {
	tag.TokenType = emptyDocument
	if input == nil {
		tag.Error = makeNestedTextError(tag, ErrCodeFormatNoInput, "no valid input document")
		return tag, nil, input
	}
	if input.isEof {
		return tag, nil, input
	}
	tag.TokenType = docRoot
	scanIndent(tag, input)
	if tag.Indent > 0 {
		tag.Error = makeNestedTextError(tag, ErrCodeFormatToplevelIndent, "top-level item must not be indented")
	}
	return tag, nil, input
}

func scanItem(tag *parserTag, input *lineBuffer) (*parserTag, scannerStep, *lineBuffer) {
	return scanIndent(tag, input)
}

// scanIndent scans intentation at the start of a line of input. From the spec:
// Leading spaces on a line represents indentation. Only ASCII spaces are allowed in the indentation.
// Specifically, tabs and the various Unicode spaces are not allowed.
func scanIndent(tag *parserTag, input *lineBuffer) (*parserTag, scannerStep, *lineBuffer) {
	matched := input.match(singleRune(' '))
	for matched && input.LastError == nil {
		tag.Indent++
		matched = input.match(singleRune(' '))
	}
	if input.LastError != nil {
		tag.Error = input.LastError
	}
	return tag, nil, input
}

func makeNestedTextError(tag *parserTag, code int, errMsg string) *NestedTextError {
	err := &NestedTextError{
		Code: code,
		msg:  errMsg,
	}
	if tag != nil {
		err.Line = tag.LineNo
		err.Column = tag.ColNo
	}
	return err
}

func wrapError(code int, errMsg string, err error) *NestedTextError {
	e := makeNestedTextError(nil, code, errMsg)
	e.wrappedError = err
	return e
}

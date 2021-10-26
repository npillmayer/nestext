package nestext

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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
	ErrCodeFormat        = 200 + iota // NestedText format error
	ErrCodeFormatNoInput              // NestedText format error: no input present
)

// Error produces an error message from a NestedText error.
func (e *NestedTextError) Error() string {
	return fmt.Sprintf("[%d,%d] %s", e.Line, e.Column, e.msg)
}

// Unwrap returns an optionally present underlying error condition, e.g., an I/O-Error.
func (e NestedTextError) Unwrap() error {
	return e.wrappedError
}

// ---------------------------------------------------------------------------

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

type parserTag struct {
	LineNo    int
	ColNo     int
	TokenType ParserTokenType
	Indent    int
	Content   string
	Error     error
}

// --- Document buffer -------------------------------------------------------

// lineBuffer is an abstraction of a NestedText document source.
type lineBuffer struct {
	Lookahead  rune
	ByteCursor int64
	Cursor     int64
	Input      *bufio.Scanner
	Line       strings.Reader
	isEof      bool
}

const eolMarker = '\n'

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

// AdvanceLine will advance the input buffer to the next line.
// Will return atEof if EOF has been encountered.
// Lookahead will be set to first rune of line, or `eolMarker` in the case of an empty line.
func (buf *lineBuffer) AdvanceLine() error {
	if buf.isEof {
		return errAtEof
	}
	buf.Cursor = 0
	buf.ByteCursor = 0
	if !buf.Input.Scan() {
		if err := buf.Input.Err(); err != nil {
			return wrapError(ErrCodeIO, "I/O error while reading input", err)
		}
		buf.isEof = true
		buf.Line = *strings.NewReader("")
		return nil
	}
	buf.Line = *strings.NewReader(buf.Input.Text())
	return buf.AdvanceCursor()
}

var errAtEof error = errors.New("EOF")

// --- Scanner ---------------------------------------------------------------

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
	input := bufio.NewScanner(inputReader)
	buf := &lineBuffer{
		Input: input,
	}
	if !buf.Input.Scan() {
		if err := buf.Input.Err(); err != nil {
			return nil, wrapError(ErrCodeIO, "cannot read input", err)
		}
		buf.isEof = true // empty input is a valid NestedText document
	}
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
	if input == nil {
		return &parserTag{TokenType: emptyDocument}, nil, nil
	}
	// skip empty lines and comments at start of document
	for input.Lookahead == '#' || input.Lookahead == eolMarker {
		if err := input.AdvanceLine(); err != nil {
			return &parserTag{TokenType: emptyDocument, Error: err}, nil, input
		}
		if input.isEof {
			return &parserTag{TokenType: emptyDocument}, nil, input
		}
	}
	tag.TokenType = docRoot
	return tag, nil, input
}

func scanItem(tag *parserTag, input *lineBuffer) (*parserTag, scannerStep, *lineBuffer) {
	return scanIndent(tag, input)
}

func scanIndent(tag *parserTag, input *lineBuffer) (*parserTag, scannerStep, *lineBuffer) {
	matched, err := match(' ', input)
	for matched && err == nil {
		tag.Indent++
		matched, err = match(' ', input)
	}
	if err != nil {
		tag.Error = err
		return tag, nil, input
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

// TODO make this a member of lineBuffer
// TODO make variadic list of LA options (alternatives)
func match(lookahead rune, input *lineBuffer) (bool, error) {
	if input.isEof {
		return false, nil
	}
	if lookahead != input.Lookahead {
		return false, nil
	}
	var err error
	if input.Lookahead == eolMarker {
		err = input.AdvanceLine()
	} else {
		err = input.AdvanceCursor()
	}
	return true, err
}

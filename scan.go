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
type scannerStep func(*parserTag) (*parserTag, scannerStep)

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

func (sc *scanner) NextToken() *parserTag {
	tag := &parserTag{LineNo: sc.Buf.CurrentLine, ColNo: int(sc.Buf.Cursor)}
	if sc.Step == nil {
		sc.Step = sc.ScanItem
	}
	for sc.Step != nil {
		tag, sc.Step = sc.Step(tag)
		if tag.Error != nil {
			sc.LastError = tag.Error
			break
		}
	}
	return tag
}

func (sc *scanner) fastPath(rule scannerStep, callback func(*parserTag)) *parserTag {
	tag := &parserTag{LineNo: sc.Buf.CurrentLine, ColNo: int(sc.Buf.Cursor)}
	for rule != nil {
		tag, rule = rule(tag)
		if tag.Error != nil {
			break
		}
	}
	return tag
}

// scanFileStart matches a valid start of a NestedText document input.
//
//    file start:
//      -> EOF:   emptyDocument
//      -> other: docRoot
//
func (sc *scanner) ScanFileStart(tag *parserTag) (*parserTag, scannerStep) {
	tag.TokenType = emptyDocument
	if sc.Buf == nil {
		tag.Error = makeNestedTextError(tag, ErrCodeFormatNoInput, "no valid input document")
		return tag, nil
	}
	if sc.Buf.IsEof() {
		return tag, nil
	}
	tag.TokenType = docRoot
	tag.Indent = 0
	if sc.Buf.Lookahead == ' ' {
		// From the spec: There is no indentation on the top-level object.
		tag.Error = makeNestedTextError(tag, ErrCodeFormatToplevelIndent, "top-level item must not be indented")
	}
	return tag, nil
}

func (sc *scanner) ScanItem(tag *parserTag) (*parserTag, scannerStep) {
	fmt.Println("---> ScanItem")
	if sc.Buf.Lookahead == ' ' {
		return tag, sc.ScanIndentation
	}
	return tag, sc.ScanItemBody
}

func (sc *scanner) ScanIndentation(tag *parserTag) (*parserTag, scannerStep) {
	if sc.Buf.Lookahead == ' ' {
		sc.Buf.match(singleRune(' '))
		tag.Indent++
		return tag, sc.ScanIndentation
	}
	return tag, sc.ScanItemBody
}

func (sc *scanner) ScanItemBody(tag *parserTag) (*parserTag, scannerStep) {
	fmt.Printf("---> ScanItemBody, LA = '%#U'", sc.Buf.Lookahead)
	switch sc.Buf.Lookahead {
	case '-': // list value, either single-line or multi-line
		// From the spec:
		// If the first non-space character on a line is a dash followed immediately by a space (-␣) or
		// a line break, the line is a list item.
		sc.Buf.match(singleRune('-'))
		if sc.Buf.Lookahead == ' ' {
			sc.Buf.match(singleRune(' '))
			return tag, sc.ScanListItem
		}
		if sc.Buf.Lookahead != eolMarker {
			tag.Error = makeNestedTextError(tag, ErrCodeFormatIllegalTag,
				"list-item tag ('-') followed by illegal character")
			return tag, nil
		}
		sc.Buf.match(singleRune(eolMarker))
		tag.TokenType = listKeyMultiline
		return tag, nil
	case '>': // multi-line string
		fmt.Println("---> multiline string")
		// From the spec:
		// If the first non-space character on a line is a greater-than symbol followed immediately by
		// a space (>␣) or a line break, the line is a string item.
		sc.Buf.match(singleRune('>'))
		if sc.Buf.Lookahead == ' ' {
			sc.Buf.match(singleRune(' '))
		} else if sc.Buf.Lookahead != eolMarker {
			tag.Error = makeNestedTextError(tag, ErrCodeFormatIllegalTag,
				"string tag ('>') followed by illegal character")
			return tag, nil
		}
		tag.Content = sc.Buf.ReadLineRemainder()
		tag.TokenType = stringMultiline
		return tag, nil
	case ':': // multi-line key
		fmt.Println("---> multiline key")
		// From the spec:
		// If the first non-space character on a line is a colon followed immediately by a space (:␣) or
		// a line break, the line is a key item.
		sc.Buf.match(singleRune(':'))
		if sc.Buf.Lookahead == ' ' {
			sc.Buf.match(singleRune(' '))
		} else if sc.Buf.Lookahead != eolMarker {
			tag.Error = makeNestedTextError(tag, ErrCodeFormatIllegalTag,
				"key tag (':') followed by illegal character")
			return tag, nil
		}
		tag.Content = sc.Buf.ReadLineRemainder()
		tag.TokenType = dictKeyMultiline
		return tag, nil
	case '[': // single-line list
		return tag, sc.ScanSingleLineList
	case '{': // single-line dictionary
	default: // should be dictionary key
	}
	return tag, nil
}

func (sc *scanner) ScanListItem(tag *parserTag) (*parserTag, scannerStep) {
	fmt.Println("---> ScanListItem")
	tag.TokenType = listKey
	tag.Content = sc.Buf.ReadLineRemainder()
	return tag, nil
}

func (sc *scanner) ScanSingleLineList(tag *parserTag) (*parserTag, scannerStep) {
	return tag, nil
}

// ScanIndent scans intentation at the start of a line of input. From the spec:
// Leading spaces on a line represents indentation. Only ASCII spaces are allowed in the indentation.
// Specifically, tabs and the various Unicode spaces are not allowed.
func (sc *scanner) ScanIndent(tag *parserTag) (*parserTag, scannerStep) {
	fmt.Printf("===> scanIndent('%s')\n", sc.Buf.Text)
	matched := sc.Buf.match(singleRune(' '))
	for matched && sc.Buf.LastError == nil {
		tag.Indent++
		matched = sc.Buf.match(singleRune(' '))
	}
	if sc.Buf.LastError != nil {
		tag.Error = sc.Buf.LastError
	}
	return tag, nil
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

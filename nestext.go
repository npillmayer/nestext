// Package nestext provides tools for processing NestedText, a human friendly data format.
// For more information on NestedText see
// https://nestedtext.org .
//
// To get a feel for the NestedText format, take a look at the following example
// (shortended version from the NestedText site):
/*
   # Contact information for our officers

   president:
      name: Katheryn McDaniel
      address:
         > 138 Almond Street
         > Topeka, Kansas 20697
      phone:
         cell: 1-210-555-5297
         home: 1-210-555-8470
            # Katheryn prefers that we always call her on her cell phone.
      email: KateMcD@aol.com
      additional roles:
         - board member

   vice president:
      name: Margaret Hodge
      …
*/
// NestedText is somewhat reminiscent of YAML, without the complexity of the latter and
// without the sometimes confusing details of interpretation.
// NestedText does not interpret any data types (unlike YAML), nor does it impose a schema.
// All of that has to be done by the application.
//
// Parsing NestedText
//
// (TODO)
//
// Example Application Usage
//
// (TODO)
//
package nestext

import "fmt"

// --- Error type ------------------------------------------------------------

// NestedTextError is a custom error type for working with NestedText instances.
type NestedTextError struct {
	Code         int // error code
	Line, Column int // error position
	msg          string
	wrappedError error
}

// We use a custom error type which contains a numeric error code.
const (
	NoError       = 0
	ErrCodeIO     = 10  // error will wrap an underlying I/O error
	ErrCodeSchema = 100 // schema violation; error may wrap an underlying error

	// all errors rooted in format violations have code >= ErrCodeFormat
	ErrCodeFormat               = 200 + iota // NestedText format error
	ErrCodeFormatNoInput                     // NestedText format error: no input present
	ErrCodeFormatToplevelIndent              // NestedText format error: top-level item was indented
	ErrCodeFormatIllegalTag                  // NestedText format error: tag not recognized
)

// Error produces an error message from a NestedText error.
func (e NestedTextError) Error() string {
	return fmt.Sprintf("[%d,%d] %s", e.Line, e.Column, e.msg)
}

// Unwrap returns an optionally present underlying error condition, e.g., an I/O-Error.
func (e NestedTextError) Unwrap() error {
	return e.wrappedError
}

// MakeNestedTextError creates a NestedTextError with a given error code and message.
func MakeNestedTextError(code int, errMsg string) NestedTextError {
	err := NestedTextError{
		Code: code,
		msg:  errMsg,
	}
	return err
}

// --- Parser token type -----------------------------------------------------

// parserToken is a type for communicating between the line-level scanner and the parser.
// The scanner will read lines and wrap the content into parser tags, i.e., tokens for the
// parser to perform its operations on.
type parserToken struct {
	LineNo, ColNo int             // start of the tag within the input source
	TokenType     parserTokenType // type of token
	Indent        int             // amount of indent of this line
	Content       []string        // UTF-8 content of the line (without indent and item tag)
	Error         error           // error condition, if any
}

//go:generate stringer -type=parserTokenType
type parserTokenType int8

const (
	undefined parserTokenType = iota
	eof
	emptyDocument
	docRoot
	listItem
	listItemMultiline
	stringMultiline
	dictKeyMultiline
	inlineList
	inlineDict
	inlineDictKeyValue
	inlineDictKey
)

// newParserToken creates a parser token initialized with line and column index.
func newParserToken(line, col int) *parserToken {
	return &parserToken{
		LineNo:  line,
		ColNo:   col,
		Content: []string{},
	}
}

func (token *parserToken) String() string {
	return fmt.Sprintf("token[at(%d,%d) ind=%d type=%s %#v]", token.LineNo, token.ColNo, token.Indent,
		token.TokenType, token.Content)
}

// --- Inline token type -----------------------------------------------------

//go:generate stringer -type=inlineTokenType
type inlineTokenType int8

const (
	character inlineTokenType = iota
	newline
	comma
	colon
	listOpen
	listClose
	dictOpen
	dictClose
)

var inlineTokenMap = map[rune]inlineTokenType{
	'\n': newline,
	',':  comma,
	':':  colon,
	'[':  listOpen,
	']':  listClose,
	'{':  dictOpen,
	'}':  dictClose,
}

func inlineTokenFor(r rune) inlineTokenType {
	if t, ok := inlineTokenMap[r]; ok {
		return t
	}
	return character
}

// --- Error helpers ---------------------------------------------------------

func makeParsingError(token *parserToken, code int, errMsg string) NestedTextError {
	err := NestedTextError{
		Code: code,
		msg:  errMsg,
	}
	if token != nil {
		err.Line = token.LineNo
		err.Column = token.ColNo
	}
	return err
}

func wrapError(code int, errMsg string, err error) NestedTextError {
	e := makeParsingError(nil, code, errMsg)
	e.wrappedError = err
	return e
}

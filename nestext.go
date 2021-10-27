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
func (e *NestedTextError) Error() string {
	return fmt.Sprintf("[%d,%d] %s", e.Line, e.Column, e.msg)
}

// Unwrap returns an optionally present underlying error condition, e.g., an I/O-Error.
func (e NestedTextError) Unwrap() error {
	return e.wrappedError
}

// --- Parser tag type -------------------------------------------------------

type parserTokenType int8

//go:generate stringer -type=parserTokenType
const (
	undefined parserTokenType = iota
	eof
	emptyDocument
	docRoot
	listKey
	listKeyMultiline
	stringMultiline
	dictKeyMultiline
)

// parserTag is a type for communicating between the scanner and the parser.
// The scanner will read lines and wrap the content into parser tags, i.e., tokens for the
// parser to perform its operations on.
type parserTag struct {
	LineNo, ColNo int             // start of the tag within the input source
	TokenType     parserTokenType // type of token
	Indent        int             // amount of indent of this line
	Content       string          // UTF-8 content of the line (without indent and item tag)
	Error         error           // error condition, if any
}

func (tag *parserTag) String() string {
	return fmt.Sprintf("tag[(%d,%d) i=%d type=%s '%s']", tag.LineNo, tag.ColNo, tag.Indent, tag.TokenType,
		tag.Content)
}

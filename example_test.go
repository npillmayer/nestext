package nestext_test

import (
	"fmt"
	"strings"

	"github.com/npillmayer/nestext"
)

func ExampleParse() {
	address := `
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
`

	// parse address input and print out e-mail item
	result, err := nestext.Parse(strings.NewReader(address))
	if err != nil {
		panic(err)
	}
	m := result.(map[string]interface{})
	fmt.Printf("Katheryn's e-mail: %v\n", m["email"])
	// Output:
	// Katheryn's e-mail: KateMcD@aol.com
}

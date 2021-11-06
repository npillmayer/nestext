package ntenc_test

import (
	"fmt"
	"os"

	"com.pillmayer.nestext/ntenc"
)

func ExampleEncode() {
	var config = map[string]interface{}{
		"timeout": 20,
		"ports":   []interface{}{6483, 8020, 9332},
	}

	n, err := ntenc.Encode(config, os.Stdout)
	fmt.Println("------------------------------")
	fmt.Printf("%d bytes written, error: %v", n, err != nil)

	// Output:
	// ports:
	//   - 6483
	//   - 8020
	//   - 9332
	// timeout: 20
	// ------------------------------
	// 46 bytes written, error: false
}

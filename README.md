# nestext
Processing NestedText ([NestedText: A Human Friendly Data Format](https://nestedtext.org/)) in Go.

A description of NestedText by the authors:

> NestedText is a file format for holding structured data that is to be entered, edited,
> or viewed by people. It allows data to be organized into a nested collection of dictionaries,
> lists, and strings without the need for quoting or escaping. In this way it is similar to JSON,
> YAML and TOML, but without the complexity and risk of YAML and without the syntactic clutter of
> JSON and TOML. NestedText is both simple and natural.

### Decoding

`Parse(…)` is the top-level API:
   
```go
input := `
# Example for a NestedText dict
a: Hello
b: World
`

result, err := nestext.Parse(strings.NewReader(input))
if err != nil {
    log.Fatal("parsing failed")
}
fmt.Printf("result = %#v\n", result)
```

will yield:

    result = map[string]interface {}{"a":"Hello", "b":"World"}

Clients may use tools like `mitchellh/mapstructure` or `knadh/koanf` for further processing.

### Encoding

Sub-package `ntenc` provides an encoder-API:

```go
	var config = map[string]interface{}{
		"timeout": 20,
		"ports":   []interface{}{6483, 8020, 9332},
	}

	n, err := ntenc.Encode(config, os.Stdout)
	fmt.Println("------------------------------")
	fmt.Printf("%d bytes written, error: %v", n, err != nil)
```

will yield:

    Output:
    ports:
      - 6483
      - 8020
      - 9332
    timeout: 20
    ------------------------------
    46 bytes written, error: false


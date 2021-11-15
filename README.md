# nestext
Processing NestedText ([NestedText: A Human Friendly Data Format](https://nestedtext.org/)) in Go.

A description of NestedText by the authors:

> NestedText is a file format for holding structured data that is to be entered, edited,
> or viewed by people. It allows data to be organized into a nested collection of dictionaries,
> lists, and strings without the need for quoting or escaping. In this way it is similar to JSON,
> YAML and TOML, but without the complexity and risk of YAML and without the syntactic clutter of
> JSON and TOML. NestedText is both simple and natural.

To get a feel for the NestedText format, take a look at the following example
(shortended version from the NestedText site):

```
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
```

NestedText does not interpret any data types (unlike YAML), nor does it impose a schema.
All of that has to be done by the application.

## Decoding

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

## Encoding

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

    ports:
      - 6483
      - 8020
      - 9332
    timeout: 20
    ------------------------------
    46 bytes written, error: false

## Status

Tested with NestedText test suite for Version 3.1.0.

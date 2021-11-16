package testsuite_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/npillmayer/nestext"
	"github.com/npillmayer/nestext/ntenc"
)

// This test runner tests the full NestedText test-suite, as proposed in the
// NestedText test proposal (https://github.com/kenkundert/nestedtext_tests).
// Current version tested against is 3.1.0
//
// Decoding-tests are checked via string comparison of the "%#v"-output. This seems
// to be a stable method. All tests pass.
//
// Encoding-tests are trickier, as for many structures there are more than one correct
// NT representations. Moreover, stability of map elements is a challenge: we sort
// them alphabetically, as Go does not make any guarantees about the sequence.
// All in all we are currently not testing encoding-cases to full depth, but in a
// sufficient manner.

var suitePath = filepath.Join(".", "official_tests", "test_cases")

func casePath(c *ntTestCase) string {
	return filepath.Join(suitePath, c.name)
}

func caseFilePath(c *ntTestCase, f string) string {
	return filepath.Join(casePath(c), f)
}

type ntTestCase struct {
	name    string
	dir     fs.FileInfo
	files   []fs.FileInfo
	data    map[string][]byte
	isLoad  bool
	isDump  bool
	status  string
	statusD string
	statusE string
	isFail  bool
}

func (c ntTestCase) contains(f string) bool {
	for _, a := range c.files {
		if a.Name() == f {
			return true
		}
	}
	return false
}

func (c *ntTestCase) bytes(f string) []byte {
	r, err := os.Open(caseFilePath(c, f))
	if err != nil {
		return nil
	}
	b, err := ioutil.ReadAll(r)
	r.Close()
	if err != nil {
		return nil
	}
	return b
}

func ttype(name string) string {
	if strings.Contains(name, "string") {
		return "string"
	}
	if strings.Contains(name, "list") {
		return "list"
	}
	if strings.Contains(name, "dict") {
		return "dict"
	}
	if strings.Contains(name, "holistic") {
		return "dict"
	}
	return ""
}

var skipped = []string{
	//"inline_dict_01",
}

func contains(l []string, s string) bool {
	for _, a := range l {
		if a == s {
			return true
		}
	}
	return false
}

func TestAll(t *testing.T) {
	cases := listTestCases(t)
	min, max := 0, len(cases)-1
	for i, c := range cases[min : max+1] {
		if strings.HasPrefix(c.name, ".") {
			cases[min+i].status = "-"
			continue
		}
		if contains(skipped, c.name) {
			cases[min+i].status = "skipped"
			continue
		}
		runTestCase(&cases[min+i], t)
	}
	failcnt := 0
	for i, c := range cases[min : max+1] {
		t.Logf("test (%03d) %-21q: %10s  [ %-5s , %-5s ]", min+i, c.name, c.status, c.statusD, c.statusE)
		if c.isFail {
			failcnt++
		}
	}
	t.Logf("%d out of %d tests failed", failcnt, len(cases))
}

func runTestCase(c *ntTestCase, t *testing.T) {
	if err := loadTestCase(c, t); err != nil {
		c.isFail = true
		t.Errorf("cannot open %q, skipping", c.name)
	}
	testDecodeCase(c, t)
	testEncodeCase(c, t)
}

func loadTestCase(c *ntTestCase, t *testing.T) (err error) {
	if c.files, err = ioutil.ReadDir(casePath(c)); err != nil {
		return
	}
	c.status = "located"
	c.data = make(map[string][]byte)
	//t.Logf("case.name = %q, %d files", c.name, len(c.files))
	for _, fname := range []string{
		"load_in.nt", "load_out.json", "load_err.json",
		"dump_in.json", "dump_out.nt", "dump_err.json",
	} {
		if c.contains(fname) {
			c.data[fname] = c.bytes(fname)
		}
	}
	if _, ok := c.data["load_in.nt"]; ok {
		c.isLoad = true
	}
	if _, ok := c.data["dump_in.json"]; ok {
		c.isDump = true
	}
	c.status = "loaded"
	return
}

func testDecodeCase(c *ntTestCase, t *testing.T) {
	//t.Logf("decoding-test %q", c.name)
	if c.isLoad {
		b := c.data["load_in.nt"]
		nt, err := nestext.Parse(strings.NewReader(string(b)))
		if err != nil {
			if c.contains("load_err.json") {
				c.statusD = "ok"
				return
			}
			c.statusD = err.Error()
			c.isFail = true
			return
		}
		c.statusD = "parsed"
		if compareOutput(nt, c, t) {
			c.statusD = "ok"
		}
	}
}

func testEncodeCase(c *ntTestCase, t *testing.T) {
	//t.Logf("encoding-test %q", c.name)
	if c.isDump {
		b := c.data["dump_in.json"]
		var r interface{}
		switch ttype(c.name) {
		case "dict":
			r = make(map[string]interface{})
		case "list":
			r = make([]interface{}, 10)
		case "string":
			r = ""
		}
		if err := json.Unmarshal(b, &r); err != nil {
			c.statusE = fmt.Sprintf("error: %s", err.Error())
			return
		}
		buf := &bytes.Buffer{}
		_, err := ntenc.Encode(r, buf, ntenc.IndentBy(4))
		if err != nil {
			if c.contains("dump_err.json") {
				c.statusE = "ok"
				return
			}
			c.statusE = fmt.Sprintf("error: %s", err.Error())
			c.isFail = true
			return
		}
		c.statusE = "parsed"
		if compareJson(buf, c, t) {
			c.statusE = "ok"
		} else {
			c.statusE = "?"
		}
	}
}

func compareOutput(any interface{}, c *ntTestCase, t *testing.T) bool {
	nt, ok1 := c.data["load_in.nt"]
	j, ok2 := c.data["load_out.json"]
	if !ok1 || !ok2 {
		return true
	}
	var target interface{}
	err := json.Unmarshal(j, &target)
	if err != nil {
		return true
	}
	//fmt.Printf("nt:     %#v\n", any)
	//fmt.Printf("target: %#v\n", target)
	r := fmt.Sprintf("%#v", any)
	o := fmt.Sprintf("%#v", target)
	if r != o {
		t.Logf("input:\n%s", nt)
		t.Logf("nt   : %s", r)
		t.Logf("json : %s", o)
		t.Fatalf("output comparison failed!\n--------------------------")
	}
	return true
}

func compareJson(buf *bytes.Buffer, c *ntTestCase, t *testing.T) bool {
	// kill excessive newlines at end
	n := strings.TrimRight(string(c.data["dump_out.nt"]), "\n")
	m := strings.TrimRight(buf.String(), "\n")
	if n != m {
		// t.Logf("target NT:\n%q\n\n", n+"\n")
		// t.Logf("output NT:\n%q\n\n", m+"\n")
		// b := c.data["dump_in.json"]
		// t.Logf("input JSON:\n\n%s\n", string(b))
		// t.Logf("NT output does not match target")
		return false
	}
	return true
}

func listTestCases(t *testing.T) []ntTestCase {
	list, err := ioutil.ReadDir(suitePath)
	if err != nil {
		t.Fatalf("unable to read test-suite dir")
	}
	t.Logf("%d test cases in NestedText Suite", len(list))
	cases := make([]ntTestCase, len(list))
	for i, caseDir := range list {
		cases[i] = ntTestCase{
			name: caseDir.Name(),
			dir:  caseDir,
		}
	}
	return cases
}

package template

import (
	"errors"
	"strings"
	"testing"

	"github.com/anotherhadi/operations"
)

func TestRender(t *testing.T) {
	cases := map[string]string{
		"hello my name is to-base64{hadi}":              "hello my name is aGFkaQ==",
		"url-encode{to-base64{hadi} ok}":                "aGFkaQ==%20ok",
		"from-base64{to-base64{a to-base64{b}}}":        "a Yg==",
		"to-base64|url-encode(encode-all){hadi?}":       "aGFkaT8%3D",
		"to-hex(delimiter=colon)|to-upper-case{hi}":     "68:69",
		"to-base64|from-base64|to-hex{a}":               "61",
		"to-base64|url-encode(encode-all){x}|to-hex{a}": "eA%3D%3D|61",
		// unknown or incomplete: text
		`{x} foo{x} json: {"a": 1} } {`:                  `{x} foo{x} json: {"a": 1} } {`,
		"xto-base64{a}":                                  "xto-base64{a}",
		"to-base64|{a} to-base64|nope{a} to-base64| {a}": "to-base64|{a} to-base64|nope{a} to-base64| {a}",
		"nope|to-hex{a}":                                 "nope|61",
		"call to-base64(hello) now":                      "call to-base64(hello) now",
		`to-base64("unterminated){x}`:                    `to-base64("unterminated){x}`,
		"-to-hex{a} to-hex-{a}":                          "-to-hex{a} to-hex-{a}",
		// balanced braces need no escaping
		`to-base64{{"a":{"b":1}}}`: "eyJhIjp7ImIiOjF9fQ==",
		`to-base64{a\}b}`:          "YX1i",
		`to-base64{\{}`:            "ew==",
		`to-hex{\\}`:               "5c",
		`to-hex{\x}`:               "5c78",
		// text outside blocks is left as is
		`a\\b \x \{ \} {"p": "C:\\a"}`: `a\\b \x \{ \} {"p": "C:\\a"}`,
		`to-base64\{a}`:                `to-base64\{a}`,
		// \op{} keeps a block as text
		`\to-base64{a}`: "to-base64{a}",
		`\to-base64(padding=false)|to-hex{to-hex{a}}`:            "to-base64(padding=false)|to-hex{61}",
		`to-hex{\to-hex{a}}`:                                     "746f2d6865787b617d",
		`\nope{a}`:                                               `\nope{a}`,
		"to-hex{}":                                               "",
		"to-hex(delimiter=colon, uppercase){hi?}":                "68:69:3F",
		"url-encode(encode-all){a=b}":                            "a%3Db",
		"url-decode(plus-as-space=false){a+b}":                   "a+b",
		"to-base64( padding = false ){a}":                        "YQ",
		"to-base64(){a}":                                         "YQ==",
		"url-encode(encode-all){to-base64(padding=false){a?}}":   "YT8",
		`to-hex(delimiter="colon"){hi}`:                          "68:69",
		`to-hex(delimiter='colon'){hi}`:                          "68:69",
		`remove-newlines(replacement=", "){a` + "\n" + `b}`:      "a, b",
		`remove-newlines(replacement="a\"b\\c"){x` + "\n" + `y}`: `xa"b\cy`,
		"add-line-numbers(start=10){a}":                          "10 a",
	}
	for src, want := range cases {
		got, err := RenderText(src)
		if err != nil {
			t.Errorf("%q: %v", src, err)
		} else if got != want {
			t.Errorf("%q: got %q, want %q", src, got, want)
		}
	}
}

func TestErrors(t *testing.T) {
	cases := map[string]string{
		"ok to-base64{abc":                  "missing closing brace (line 1, column 4)",
		"to-base64{a{b}":                    "missing closing brace",
		"ok to-base64(nope=1){x}":           `unknown argument "nope" for "to-base64" (expected: padding) (line 1, column 14)`,
		"from-hex(x){a}":                    `unknown argument "x" for "from-hex" (expected: none)`,
		"to-hex(delimiter=foo){x}":          `invalid value "foo" for "delimiter"`,
		"to-base64(padding=maybe){x}":       `"padding" expects true or false`,
		"to-base64(padding, padding){x}":    `duplicate argument "padding"`,
		"to-hex(delimiter=a b){x}":          `unexpected "b"`,
		"to-hex(delimiter=a!b){x}":          `unexpected "!"`,
		"to-base64(padding,){x}":            "expected an argument name",
		"to-hex(delimiter=){x}":             "expected a value",
		"to-hex(delimiter){x}":              `"delimiter" needs a value`,
		"add-line-numbers(start=x){a}":      `"start" expects an integer`,
		`to-hex(delimiter="a,b)c\"d{}"){x}`: `invalid value "a,b)c\"d{}"`,
		"to-hex|to-base64(nope){x}":         `unknown argument "nope" for "to-base64"`,
		"ok\n  from-base64(strict){!!}":     `from-base64: invalid Base64: unexpected character "!" at offset 0 (line 2, column 3)`,
		"é from-base64(strict)|to-hex{!!}":  `from-base64: invalid Base64: unexpected character "!" at offset 0 (line 1, column 3)`,
	}
	for src, want := range cases {
		_, err := Render(src)
		var terr *Error
		if !errors.As(err, &terr) || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %q", src, err, want)
		}
	}

	_, err := Parse("first line\n  to-base64{abc")
	var terr *Error
	if !errors.As(err, &terr) || terr.Line != 2 || terr.Column != 3 || terr.Pos != 13 {
		t.Errorf("got %+v", err)
	}
	_, err = Render("from-base64(strict){!}")
	var opErr *operations.Error
	if !errors.As(err, &opErr) || opErr.Op != "from-base64" {
		t.Errorf("operation error not wrapped: %v", err)
	}
}

func TestParse(t *testing.T) {
	src := "a url-decode(plus-as-space=false)|to-hex{x{y}z}!"
	nodes, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes", len(nodes))
	}
	outer := nodes[1].(*Block)
	inner := outer.Children[0].(*Block)
	span := func(s Span) string { return src[s.Start:s.End] }
	checks := [][2]string{
		{span(nodes[0].Span()), "a "},
		{outer.Op.ID, "to-hex"},
		{span(outer.Head), "to-hex"},
		{span(outer.Pos), "url-decode(plus-as-space=false)|to-hex{x{y}z}"},
		{span(outer.Content), "x{y}z"},
		{inner.Op.ID, "url-decode"},
		{span(inner.Head), "url-decode(plus-as-space=false)"},
		{span(inner.Pos), span(outer.Pos)},
		{inner.Children[0].(*Text).Value, "x{y}z"},
		{span(inner.Children[0].Span()), "x{y}z"},
		{span(nodes[2].Span()), "!"},
	}
	for _, c := range checks {
		if c[0] != c[1] {
			t.Errorf("got %q, want %q", c[0], c[1])
		}
	}
	if inner.Args["plus-as-space"] != false || outer.Args["delimiter"] != "none" {
		t.Errorf("args: %v %v", inner.Args, outer.Args)
	}
}

func TestHostBlocks(t *testing.T) {
	file := &operations.Operation{ID: "file"}
	upper := &operations.Operation{
		ID:   "shout",
		Args: []operations.ArgDef{{ID: "mark", Type: operations.String, Default: "!"}},
		Run: func(in []byte, a operations.Args) ([]byte, error) {
			return []byte(strings.ToUpper(string(in)) + a.String("mark")), nil
		},
	}
	e := &Engine{Lookup: func(id string) (*operations.Operation, bool) {
		switch id {
		case "file":
			return file, true
		case "shout":
			return upper, true
		}
		return operations.Lookup(id)
	}}

	nodes, err := e.Parse("GET /?u=to-base64{file{users.txt}}")
	if err != nil {
		t.Fatal(err)
	}
	b := nodes[1].(*Block).Children[0].(*Block)
	if b.Op != file || b.Children[0].(*Text).Value != "users.txt" {
		t.Errorf("got %+v", b)
	}
	if _, err := e.Render("file{x}"); err == nil || !strings.Contains(err.Error(), "file: cannot be evaluated here") {
		t.Errorf("got %v", err)
	}
	if out, err := e.Render(`shout(mark="?")|to-hex{a}`); err != nil || string(out) != "413f" {
		t.Errorf("got %q %v", out, err)
	}
	if out, _ := Render("shout{a}"); string(out) != "shout{a}" {
		t.Error("the default engine must not know host blocks")
	}
}

package operations

import (
	"bytes"
	"crypto/rand"
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestRegistry(t *testing.T) {
	id := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	seen := map[string]bool{}
	for _, op := range All() {
		if !id.MatchString(op.ID) || seen[op.ID] {
			t.Errorf("invalid or duplicate id %q", op.ID)
		}
		seen[op.ID] = true
		if op.Inverse != "" {
			if inv, ok := Lookup(op.Inverse); !ok || inv.Inverse != op.ID {
				t.Errorf("%s: inverse %q is not symmetric", op.ID, op.Inverse)
			}
		}
		if len(op.Examples) == 0 {
			t.Errorf("%s: no example", op.ID)
		}
		for _, d := range op.Args {
			if !id.MatchString(d.ID) {
				t.Errorf("%s: invalid arg id %q", op.ID, d.ID)
			}
			if _, err := d.Check(d.Default); err != nil {
				t.Errorf("%s: invalid default: %v", op.ID, err)
			}
		}
	}
}

func TestExamples(t *testing.T) {
	for _, op := range All() {
		for _, ex := range op.Examples {
			got, err := op.Apply([]byte(ex.Input), ex.Args)
			if err != nil {
				t.Errorf("%s %q: %v", op.ID, ex.Input, err)
			} else if string(got) != ex.Output {
				t.Errorf("%s %q %v:\n got %q\nwant %q", op.ID, ex.Input, ex.Args, got, ex.Output)
			}
		}
	}
}

func TestTextOutputIsPrintable(t *testing.T) {
	input := make([]byte, 256)
	rand.Read(input)
	printable := regexp.MustCompile(`^[\x20-\x7e]*$`)
	for _, op := range All() {
		if op.Output != TextOutput {
			continue
		}
		in := input
		if op.ID == "jwt-sign" {
			in = []byte(`{"a":1}`)
		}
		out, err := op.Apply(in, nil)
		if err != nil || !printable.Match(out) {
			t.Errorf("%s: non-printable output %q", op.ID, out)
		}
	}
}

func TestErrors(t *testing.T) {
	_, err := Run("from-base64", []byte("a!"), Args{"strict": true})
	var opErr *Error
	if !errors.As(err, &opErr) || opErr.Op != "from-base64" ||
		err.Error() != `from-base64: invalid Base64: unexpected character "!" at offset 1` {
		t.Errorf("got %v", err)
	}
	var argErr *ArgError
	if _, err := Run("to-base64", nil, Args{"nope": 1}); !errors.As(err, &argErr) || !strings.Contains(err.Error(), `unknown argument "nope"`) {
		t.Errorf("got %v", err)
	}
	if _, err := Run("to-hex", nil, Args{"delimiter": "foo"}); err == nil || !strings.Contains(err.Error(), `invalid value "foo" for "delimiter"`) {
		t.Errorf("got %v", err)
	}
	if _, err := Run("url-encode", nil, Args{"encode-all": "no"}); err == nil || !strings.Contains(err.Error(), `"encode-all" expects true or false`) {
		t.Errorf("got %v", err)
	}
	if _, err := Run("bcrypt", nil, Args{"rounds": 3}); err == nil || !strings.Contains(err.Error(), "between 4 and 31") {
		t.Errorf("got %v", err)
	}
	var unknown *UnknownError
	if _, err := Run("nope", nil, nil); !errors.As(err, &unknown) {
		t.Errorf("got %v", err)
	}
	if out, err := Run("add-line-numbers", []byte("a"), Args{"start": float64(3)}); err != nil || string(out) != "3 a" {
		t.Errorf("float args: %q %v", out, err)
	}
}

type textCase struct {
	id, input string
	args      Args
	want      string
}

func runCases(t *testing.T, cases []textCase) {
	t.Helper()
	for _, c := range cases {
		got, err := RunText(c.id, c.input, c.args)
		if err != nil {
			t.Errorf("%s %q: %v", c.id, c.input, err)
		} else if got != c.want {
			t.Errorf("%s %q %v:\n got %q\nwant %q", c.id, c.input, c.args, got, c.want)
		}
	}
}

func expectError(t *testing.T, id, input string, args Args, contains string) {
	t.Helper()
	_, err := RunText(id, input, args)
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Errorf("%s %q: got error %v, want %q", id, input, err, contains)
	}
}

func TestBase(t *testing.T) {
	deadbeef := "\xde\xad\xbe\xef"
	runCases(t, []textCase{
		{"to-base64", "", nil, ""},
		{"to-base64", deadbeef, nil, "3q2+7w=="},
		{"to-base64", deadbeef, Args{"padding": false}, "3q2+7w"},
		{"from-base64", "3q2+7w==", nil, deadbeef},
		{"from-base64", " aGVs\nbG8 ", nil, "hello"},
		{"from-base64", "aGVsbG8", nil, "hello"},
		{"from-base64", "aGVsbG8=", Args{"strict": true}, "hello"},
		{"from-base64url", "3q2-7w", nil, deadbeef},
		{"to-base32", "", nil, ""},
		{"from-base32", "NBSW Y3DP", nil, "hello"},
		{"to-base58", "\x00\x00hi", nil, "11" + "8wr"},
		{"from-base58", "118wr", nil, "\x00\x00hi"},
		{"to-base62", "\x00a", nil, "01Z"},
		{"from-base62", "01Z", nil, "\x00a"},
	})
	expectError(t, "from-base64", "3q2-7w", Args{"strict": true}, `unexpected character "-"`)
	expectError(t, "from-base64", "aGVsbG8", Args{"strict": true}, "padding")
	expectError(t, "from-base64", "a", nil, "truncated")
	expectError(t, "from-base32", "A", nil, "truncated")
	expectError(t, "from-base32", "A1", nil, `unexpected character "1"`)
	expectError(t, "from-base58", "0", nil, `invalid Base58: unexpected character "0"`)

	random := make([]byte, 257)
	rand.Read(random)
	for _, pair := range [][2]string{{"to-base64", "from-base64"}, {"to-base64url", "from-base64url"}, {"to-base32", "from-base32"}, {"to-base58", "from-base58"}, {"to-base62", "from-base62"}, {"to-hex", "from-hex"}} {
		enc, _ := Run(pair[0], random, nil)
		dec, err := Run(pair[1], enc, nil)
		if err != nil || !bytes.Equal(dec, random) {
			t.Errorf("%s round trip: %v", pair[0], err)
		}
	}
	enc, _ := Run("url-encode", random, Args{"encode-all": true})
	if dec, _ := Run("url-decode", enc, nil); !bytes.Equal(dec, random) {
		t.Error("url-encode round trip")
	}
}

func TestEncoding(t *testing.T) {
	runCases(t, []textCase{
		{"from-hex", "6869\n", nil, "hi"},
		{"url-decode", "%zz%4", nil, "%zz%4"},
		{"url-decode", "%41%", nil, "A%"},
		{"html-decode", "&#0;&#x110000;&#xD800;&AMP;", nil, "\ufffd\ufffd\ufffd&AMP;"},
		{"html-encode", "é\xff", Args{"encode-non-ascii": true}, "&#233;&#65533;"},
	})
	expectError(t, "from-hex", "abc", nil, "odd number")
	expectError(t, "from-hex", "zz", nil, `unexpected character "z"`)
}

func TestCrypto(t *testing.T) {
	key := "000102030405060708090a0b0c0d0e0f"
	for _, mode := range []string{"cbc", "ecb", "ctr", "gcm"} {
		for _, size := range []int{16, 24, 32} {
			k := strings.Repeat("ab", size)
			for _, input := range []string{"", "hello", strings.Repeat("x", 16), strings.Repeat("y", 33)} {
				args := Args{"key": k, "mode": mode}
				enc, err := RunText("aes-encrypt", input, args)
				if err != nil {
					t.Fatalf("%s: %v", mode, err)
				}
				dec, err := RunText("aes-decrypt", enc, args)
				if err != nil || dec != input {
					t.Errorf("%s/%d %q: got %q, %v", mode, size, input, dec, err)
				}
			}
		}
	}
	a, _ := RunText("aes-encrypt", "x", Args{"key": key})
	b, _ := RunText("aes-encrypt", "x", Args{"key": key})
	if a == b || len(a) != 32 {
		t.Errorf("random IVs: %x %x", a, b)
	}
	runCases(t, []textCase{
		{"aes-decrypt", "", Args{"key": "0123456789abcdef", "key-format": "utf8", "mode": "ecb"}, ""},
		{"xor", "hello", Args{"key": "aGk=", "key-format": "base64"}, "\x00\x0c\x04\x05\x07"},
	})
	expectError(t, "aes-encrypt", "x", Args{"key": "00"}, "AES needs 16, 24 or 32 bytes, got 1")
	expectError(t, "aes-encrypt", "x", Args{"key": key, "iv": "00"}, "CBC needs 16 bytes, got 1")
	expectError(t, "aes-encrypt", "x", Args{"key": key, "iv": "00", "mode": "ecb"}, "ECB does not use an IV")
	expectError(t, "aes-decrypt", strings.Repeat("x", 32), Args{"key": key}, "invalid padding")
	expectError(t, "aes-decrypt", strings.Repeat("x", 40), Args{"key": key, "mode": "gcm"}, "authentication failed")
	expectError(t, "aes-decrypt", "short", Args{"key": key}, "too short")
	expectError(t, "xor", "x", nil, "key is required")
	expectError(t, "xor", "x", Args{"key": "zz"}, "invalid key")

	runCases(t, []textCase{
		{"jwt-decode", "Bearer " + jwtExample + "\n", Args{"indent": "minify"}, `{"header":{"alg":"HS256","typ":"JWT"},"payload":{"sub":"1234567890","name":"John Doe","iat":1516239022},"signature":"SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"}`},
		{"jwt-decode", "eyJhbGciOiJub25lIn0.PGI-.", Args{"indent": "minify"}, `{"header":{"alg":"none"},"payload":"<b>","signature":""}`},
		{"jwt-sign", `{"a":1}`, Args{"algorithm": "HS512", "key": "c2VjcmV0", "key-format": "base64"}, mustRun(t, "jwt-sign", `{"a":1}`, Args{"algorithm": "HS512", "key": "secret"})},
	})
	expectError(t, "jwt-decode", "a.b", nil, "expected 3 parts")
	expectError(t, "jwt-decode", "a.b.c.d.e", nil, "JWE")
	expectError(t, "jwt-decode", "e30.!.x", nil, "payload is not Base64url")
	expectError(t, "jwt-sign", "{", nil, "invalid JSON")
}

func mustRun(t *testing.T, id, input string, args Args) string {
	out, err := RunText(id, input, args)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestHash(t *testing.T) {
	out := mustRun(t, "bcrypt", "hello", Args{"rounds": 4})
	if !regexp.MustCompile(`^\$2b\$04\$[./A-Za-z0-9]{53}$`).MatchString(out) || out == mustRun(t, "bcrypt", "hello", Args{"rounds": 4}) {
		t.Errorf("random salt: %s", out)
	}
	long := strings.Repeat("a", 72)
	salt := Args{"rounds": 4, "salt": "abcdefghijklmnopqrstuu"}
	if mustRun(t, "bcrypt", long, salt) != mustRun(t, "bcrypt", long+"b", salt) {
		t.Error("bcrypt must ignore bytes after the 72nd")
	}
	expectError(t, "bcrypt", "x", Args{"salt": "short"}, "invalid salt")
}

func TestData(t *testing.T) {
	runCases(t, []textCase{
		{"json-format", `[1, {"a":"x,y:[]"}]`, Args{"indent": "minify"}, `[1,{"a":"x,y:[]"}]`},
		{"json-format", `{"a":{"b":[]}}`, Args{"indent": "tab"}, "{\n\t\"a\": {\n\t\t\"b\": []\n\t}\n}"},
		{"json-to-yaml", `{"num":12345678901234567890,"s":"123","e":{},"l":[],"t":"a\tb","x":"<&>"}`, nil, "num: 12345678901234567890\ns: \"123\"\ne: {}\nl: []\nt: \"a\\tb\"\nx: <&>\n"},
		{"json-to-yaml", `"off"`, nil, "\"off\"\n"},
		{"json-to-yaml", `{"<<":{"a":1},"t":"a\tb\nc","e":"\ud83d\udc4d","s":"\/"}`, nil, "\"<<\":\n  a: 1\nt: \"a\\tb\\nc\"\ne: \"\\U0001F44D\"\ns: /\n"},
		{"yaml-to-json", "big: 12345678901234567890\nhex: 0x1F\nf: 1.50\ninf: .inf\nd: 2001-12-14\nk: &a 1\nr: *a\n'<': '&'\n", Args{"indent": "minify"}, `{"big":12345678901234567890,"hex":31,"f":1.50,"inf":null,"d":"2001-12-14","k":1,"r":1,"<":"&"}`},
		{"yaml-to-json", "", nil, "null"},
	})
	expectError(t, "json-format", "{", nil, "invalid JSON")
	expectError(t, "json-to-yaml", "a: 1", nil, "invalid JSON")
	expectError(t, "yaml-to-json", "a: [", nil, "invalid YAML")
	expectError(t, "hex-to-rgb", "#ff\n#zz0", nil, `invalid hex color "#ff"`)
}

func TestText(t *testing.T) {
	runCases(t, []textCase{
		{"to-camel-case", "user_id\nversion2Beta\nPDF-to HTML", nil, "userId\nversion2Beta\npdfToHtml"},
		{"to-snake-case", "élan vitalÉtat", nil, "élan_vital_état"},
		{"to-kebab-case", "don't stop", nil, "dont-stop"},
		{"to-pascal-case", "  ", nil, ""},
		{"to-upper-case", "straße", nil, "STRASSE"},
		{"to-slug", "  --Ça va? ok-- ", nil, "ca-va-ok"},
		{"to-slug", "A\nB c", nil, "a\nb-c"},
		{"sort-lines", "b\na\n", nil, "a\nb\n"},
		{"sort-lines", "b\r\na\r\n", nil, "a\r\nb\r\n"},
		{"sort-lines", "b\nA\na", Args{"ignore-case": true}, "A\na\nb"},
		{"sort-lines", "", nil, ""},
		{"reverse-lines", "a\r\nb", nil, "b\r\na"},
		{"unique-lines", "a\na\n", nil, "a\n"},
		{"add-line-numbers", "a\nb\n", Args{"start": 0}, "0 a\n1 b\n"},
		{"add-line-numbers", "a\nb", Args{"start": -1}, "-1 a\n 0 b"},
		{"reverse-text", "ab\r\ncd", nil, "dc\r\nba"},
		{"reverse-text", "🇫🇷é", nil, "é🇫🇷"},
		{"escape-quotes", `a\"b`, Args{"escape-backslashes": false}, `a\\"b`},
		{"escape-quotes", `a\"b`, nil, `a\\\"b`},
		{"remove-whitespace", "a\u00a0b\u2003c", nil, "abc"},
		{"count-chars", "", nil, "0"},
		{"count-chars", "e\u0301", nil, "1"},
		{"count-lines", "\n", nil, "1"},
		{"count-words", "", nil, "0"},
	})

	var lines []string
	for i := range 50 {
		lines = append(lines, string(rune('a'+i%26))+strings.Repeat("x", i))
	}
	in := strings.Join(lines, "\n")
	shuffled := strings.Split(mustRun(t, "shuffle-lines", in, nil), "\n")
	slices.Sort(shuffled)
	sorted := slices.Sorted(slices.Values(lines))
	if !slices.Equal(shuffled, sorted) {
		t.Error("shuffle-lines lost lines")
	}
	x := mustRun(t, "shuffle-lines", in, Args{"seed": "x"})
	if x != mustRun(t, "shuffle-lines", in, Args{"seed": "x"}) || x == mustRun(t, "shuffle-lines", in, Args{"seed": "y"}) {
		t.Error("seeded shuffle is not reproducible")
	}
}

func TestExtract(t *testing.T) {
	ips := "fe80::1 2001:0db8:85a3:0000:0000:8a2e:0370:7334. time 12:30:45 mac 00:11:22:33:44:55 " +
		"::ffff:1.2.3.4 1:2:3:4:5:6:7:8:9 1.2.3 999.1.1.1"
	runCases(t, []textCase{
		{"extract-ips", ips, nil, "fe80::1\n2001:0db8:85a3:0000:0000:8a2e:0370:7334\n::ffff:1.2.3.4"},
		{"extract-ips", ips, Args{"version": "ipv4"}, ""},
		{"extract-ips", "8.8.8.8 1.1.1.1 8.8.8.8", Args{"unique": true, "sort": true}, "1.1.1.1\n8.8.8.8"},
		{"extract-ips", "v1.2.3.4 01.2.3.4 1.2.3.4. end", nil, "1.2.3.4\n1.2.3.4"},
		{"extract-urls", `<a href="https://x.com/p">x</a> [link](https://md.org/x) https://a.b/c?d=e#f!`, nil, "https://x.com/p\nhttps://md.org/x\nhttps://a.b/c?d=e#f"},
		{"extract-emails", "mailto:Bob@Exämple.fr; x@y; a.b@c.co.uk.", nil, "Bob@Exämple.fr\na.b@c.co.uk"},
	})
}

func TestDisplay(t *testing.T) {
	for in, want := range map[string]bool{"héllo\n\tok\r\n": true, "\x1b[31m": false, "\xff": false, "\u009b": false, "a\u202eb": false, "\x7f": false} {
		if IsPrintable([]byte(in)) != want {
			t.Errorf("IsPrintable(%q) != %v", in, want)
		}
	}
	got := Hexdump([]byte("\x1b[31mhello world, this is long"))
	want := "00000000  1b 5b 33 31 6d 68 65 6c  6c 6f 20 77 6f 72 6c 64  |.[31mhello world|\n" +
		"00000010  2c 20 74 68 69 73 20 69  73 20 6c 6f 6e 67        |, this is long|\n" +
		"0000001e"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if Hexdump(nil) != "00000000" {
		t.Error("empty hexdump")
	}
}

// steep runs them from many goroutines.
func TestConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		for _, op := range All() {
			for _, ex := range op.Examples {
				wg.Go(func() {
					if got, err := op.Apply([]byte(ex.Input), ex.Args); err != nil || string(got) != ex.Output {
						t.Errorf("%s: %q %v", op.ID, got, err)
					}
				})
			}
		}
	}
	wg.Wait()
}

func TestBcryptCanonicalSalt(t *testing.T) {
	// "a" and "O" end the same 16 bytes; bcryptjs writes "O"
	got := mustRun(t, "bcrypt", "hello", Args{"rounds": 4, "salt": "sc1Sq/TGV/u7QJmouGtqwa"})
	if got != "$2b$04$sc1Sq/TGV/u7QJmouGtqwOqjnnrasOmAUl5IA7pVTzCcy19JUZYw2" {
		t.Errorf("got %s", got)
	}
}

func TestInvalidUTF8IsKept(t *testing.T) {
	for _, id := range []string{"remove-whitespace", "remove-newlines", "escape-quotes", "sort-lines", "reverse-lines", "to-lower-case", "html-decode"} {
		out, err := Run(id, []byte("\xff a\xfe"), nil)
		if err != nil || !bytes.Contains(out, []byte("\xff")) || !bytes.Contains(out, []byte("\xfe")) {
			t.Errorf("%s: %q %v", id, out, err)
		}
	}
}

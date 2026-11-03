package operations

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func encodingOps() []*Operation {
	return []*Operation{
		toBase64, fromBase64, toBase64url, fromBase64url,
		toBase32, fromBase32, toBase58, fromBase58, toBase62, fromBase62,
		toHex, fromHex, urlEncode, urlDecode, htmlEncode, htmlDecode,
	}
}

var strictArg = boolArg("strict", "Strict mode", false).
	describe("Fail on non-alphabet characters and invalid padding instead of ignoring them.")

var toBase64 = &Operation{
	ID:          "to-base64",
	Name:        "To Base64",
	Description: `Encodes raw bytes as Base64 text. e.g. "hello" becomes "aGVsbG8=".`,
	Category:    Encoding,
	Aliases:     []string{"b64", "base64 encode"},
	Inverse:     "from-base64",
	InfoURL:     "https://wikipedia.org/wiki/Base64",
	Args:        []ArgDef{boolArg("padding", "Padding (=)", true)},
	Examples: []Example{
		{Input: "hello", Output: "aGVsbG8="},
		{Input: "hello", Args: Args{"padding": false}, Output: "aGVsbG8"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return encodeBase64(in, base64.StdEncoding, a.Bool("padding")), nil
	},
}

var fromBase64 = &Operation{
	ID:          "from-base64",
	Name:        "From Base64",
	Description: `Decodes Base64 text into raw bytes. e.g. "aGVsbG8=" becomes "hello".`,
	Category:    Encoding,
	Output:      BinaryOutput,
	Aliases:     []string{"b64", "base64 decode"},
	Inverse:     "to-base64",
	InfoURL:     "https://wikipedia.org/wiki/Base64",
	Args:        []ArgDef{strictArg},
	Examples: []Example{
		{Input: "aGVsbG8=", Output: "hello"},
		{Input: "aGVs\nbG8", Output: "hello"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return decodeBase64(in, base64.StdEncoding, "+/", a.Bool("strict"))
	},
}

var toBase64url = &Operation{
	ID:          "to-base64url",
	Name:        "To Base64url",
	Description: `Encodes raw bytes as URL-safe Base64 (- and _ instead of + and /), without padding by default. e.g. "ÿÿ" becomes "w7_Dvw".`,
	Category:    Encoding,
	Aliases:     []string{"b64url", "base64url encode"},
	Inverse:     "from-base64url",
	InfoURL:     "https://datatracker.ietf.org/doc/html/rfc4648#section-5",
	Args:        []ArgDef{boolArg("padding", "Padding (=)", false)},
	Examples: []Example{
		{Input: "ÿÿ", Output: "w7_Dvw"},
		{Input: "ÿÿ", Args: Args{"padding": true}, Output: "w7_Dvw=="},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return encodeBase64(in, base64.URLEncoding, a.Bool("padding")), nil
	},
}

var fromBase64url = &Operation{
	ID:          "from-base64url",
	Name:        "From Base64url",
	Description: `Decodes URL-safe Base64 (- and _ instead of + and /) into raw bytes. e.g. "w7_Dvw" becomes "ÿÿ".`,
	Category:    Encoding,
	Output:      BinaryOutput,
	Aliases:     []string{"b64url", "base64url decode", "jwt"},
	Inverse:     "to-base64url",
	InfoURL:     "https://datatracker.ietf.org/doc/html/rfc4648#section-5",
	Args:        []ArgDef{strictArg},
	Examples: []Example{
		{Input: "w7_Dvw", Output: "ÿÿ"},
		{Input: "w7_Dvw==", Output: "ÿÿ"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return decodeBase64(in, base64.URLEncoding, "-_", a.Bool("strict"))
	},
}

func encodeBase64(in []byte, enc *base64.Encoding, padding bool) []byte {
	if !padding {
		enc = enc.WithPadding(base64.NoPadding)
	}
	out := make([]byte, enc.EncodedLen(len(in)))
	enc.Encode(out, in)
	return out
}

func isAlnumOr(c byte, extra string) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte(extra, c) >= 0
}

// Unless strict, other characters and padding are ignored.
func decodeBase64(in []byte, enc *base64.Encoding, extra string, strict bool) ([]byte, error) {
	if strict {
		out, err := enc.Strict().DecodeString(string(in))
		if err == nil {
			return out, nil
		}
		unpadded := bytes.TrimRight(in, "=")
		for i, c := range unpadded {
			if !isAlnumOr(c, extra) && c != '\n' && c != '\r' {
				return nil, fmt.Errorf("invalid Base64: unexpected character %q at offset %d", string(c), i)
			}
		}
		return nil, errors.New("invalid Base64: missing or incorrect padding")
	}
	clean := make([]byte, 0, len(in))
	for _, c := range in {
		if isAlnumOr(c, extra) {
			clean = append(clean, c)
		}
	}
	out, err := enc.WithPadding(base64.NoPadding).DecodeString(string(clean))
	if err != nil {
		return nil, errors.New("invalid Base64: truncated input (the last group has a single character)")
	}
	return out, nil
}

var base32Alphabet = selectArg("alphabet", "Alphabet", "base32",
	Option{"base32", "Standard (RFC 4648): A-Z2-7="},
	Option{"base32hex", "Extended hex (RFC 4648 §7): 0-9A-V="},
)

var base32Chars = map[string]string{
	"base32":    "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567",
	"base32hex": "0123456789ABCDEFGHIJKLMNOPQRSTUV",
}

var toBase32 = &Operation{
	ID:          "to-base32",
	Name:        "To Base32",
	Description: `Encodes raw bytes as Base32 text. e.g. "hello" becomes "NBSWY3DP".`,
	Category:    Encoding,
	Aliases:     []string{"b32", "base32 encode"},
	Inverse:     "from-base32",
	InfoURL:     "https://wikipedia.org/wiki/Base32",
	Args:        []ArgDef{base32Alphabet, boolArg("padding", "Padding (=)", true)},
	Examples: []Example{
		{Input: "hello", Output: "NBSWY3DP"},
		{Input: "hi", Output: "NBUQ===="},
		{Input: "hi", Args: Args{"alphabet": "base32hex", "padding": false}, Output: "D1KG"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		enc := base32.NewEncoding(base32Chars[a.String("alphabet")])
		if !a.Bool("padding") {
			enc = enc.WithPadding(base32.NoPadding)
		}
		return []byte(enc.EncodeToString(in)), nil
	},
}

var fromBase32 = &Operation{
	ID:          "from-base32",
	Name:        "From Base32",
	Description: `Decodes Base32 text into raw bytes, ignoring case, whitespace and padding. e.g. "NBSWY3DP" becomes "hello".`,
	Category:    Encoding,
	Output:      BinaryOutput,
	Aliases:     []string{"b32", "base32 decode"},
	Inverse:     "to-base32",
	InfoURL:     "https://wikipedia.org/wiki/Base32",
	Args:        []ArgDef{base32Alphabet},
	Examples: []Example{
		{Input: "NBSWY3DP", Output: "hello"},
		{Input: "nbuq====", Output: "hi"},
		{Input: "D1KG", Args: Args{"alphabet": "base32hex"}, Output: "hi"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		chars := base32Chars[a.String("alphabet")]
		text := strings.ToUpper(strings.TrimRight(removeSpaces(string(in)), "="))
		switch utf8.RuneCountInString(text) % 8 {
		case 1, 3, 6:
			return nil, errors.New("invalid Base32: truncated input (the last group has an impossible length)")
		}
		out := make([]byte, 0, len(text)*5/8)
		buffer, bits := 0, 0
		for _, r := range text {
			v := strings.IndexRune(chars, r)
			if v < 0 {
				return nil, fmt.Errorf("invalid Base32: unexpected character %q", string(r))
			}
			buffer = (buffer<<5 | v) & 0xfff
			bits += 5
			if bits >= 8 {
				bits -= 8
				out = append(out, byte(buffer>>bits))
			}
		}
		return out, nil
	},
}

func removeSpaces(s string) string {
	return strings.Join(strings.Fields(s), "")
}

var base58Alphabet = selectArg("alphabet", "Alphabet", "bitcoin",
	Option{"bitcoin", "Bitcoin: 1-9A-HJ-NP-Za-km-z"},
	Option{"flickr", "Flickr: 1-9a-km-zA-HJ-NP-Z"},
	Option{"ripple", "Ripple: rpshnaf39wBUDNEGHJKLM4PQRST7VWXYZ2bcdeCg65jkm8oFqi1tuvAxyz"},
)

var base58Chars = map[string]string{
	"bitcoin": "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz",
	"flickr":  "123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ",
	"ripple":  "rpshnaf39wBUDNEGHJKLM4PQRST7VWXYZ2bcdeCg65jkm8oFqi1tuvAxyz",
}

var toBase58 = &Operation{
	ID:          "to-base58",
	Name:        "To Base58",
	Description: `Encodes raw bytes as Base58 text, as used by Bitcoin addresses. Leading zero bytes become "1". e.g. "hello" becomes "Cn8eVZg".`,
	Category:    Encoding,
	Aliases:     []string{"b58", "base58 encode", "bitcoin"},
	Inverse:     "from-base58",
	InfoURL:     "https://wikipedia.org/wiki/Binary-to-text_encoding#Base58",
	Args:        []ArgDef{base58Alphabet},
	Examples: []Example{
		{Input: "hello", Output: "Cn8eVZg"},
		{Input: "Hello World!", Output: "2NEpo7TZRRrLZSi2U"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return encodeBaseX(in, base58Chars[a.String("alphabet")]), nil
	},
}

var fromBase58 = &Operation{
	ID:          "from-base58",
	Name:        "From Base58",
	Description: `Decodes Base58 text into raw bytes, ignoring whitespace. e.g. "Cn8eVZg" becomes "hello".`,
	Category:    Encoding,
	Output:      BinaryOutput,
	Aliases:     []string{"b58", "base58 decode", "bitcoin"},
	Inverse:     "to-base58",
	InfoURL:     "https://wikipedia.org/wiki/Binary-to-text_encoding#Base58",
	Args:        []ArgDef{base58Alphabet},
	Examples: []Example{
		{Input: "Cn8eVZg", Output: "hello"},
		{Input: "2NEpo7TZRRrLZSi2U", Output: "Hello World!"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return decodeBaseX(removeSpaces(string(in)), base58Chars[a.String("alphabet")], "Base58")
	},
}

var base62Alphabet = selectArg("alphabet", "Alphabet", "0-9A-Za-z",
	Option{"0-9A-Za-z", "Digits, uppercase, lowercase (GMP)"},
	Option{"0-9a-zA-Z", "Digits, lowercase, uppercase"},
)

var base62Chars = map[string]string{
	"0-9A-Za-z": "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz",
	"0-9a-zA-Z": "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ",
}

var toBase62 = &Operation{
	ID:          "to-base62",
	Name:        "To Base62",
	Description: `Encodes raw bytes as Base62 text (letters and digits only). Leading zero bytes become "0". e.g. "hello" becomes "7tQLFHz".`,
	Category:    Encoding,
	Aliases:     []string{"b62", "base62 encode"},
	Inverse:     "from-base62",
	InfoURL:     "https://wikipedia.org/wiki/Base62",
	Args:        []ArgDef{base62Alphabet},
	Examples: []Example{
		{Input: "hello", Output: "7tQLFHz"},
		{Input: "hello", Args: Args{"alphabet": "0-9a-zA-Z"}, Output: "7TqlfhZ"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return encodeBaseX(in, base62Chars[a.String("alphabet")]), nil
	},
}

var fromBase62 = &Operation{
	ID:          "from-base62",
	Name:        "From Base62",
	Description: `Decodes Base62 text into raw bytes, ignoring whitespace. e.g. "7tQLFHz" becomes "hello".`,
	Category:    Encoding,
	Output:      BinaryOutput,
	Aliases:     []string{"b62", "base62 decode"},
	Inverse:     "to-base62",
	InfoURL:     "https://wikipedia.org/wiki/Base62",
	Args:        []ArgDef{base62Alphabet},
	Examples: []Example{
		{Input: "7tQLFHz", Output: "hello"},
		{Input: "7TqlfhZ", Args: Args{"alphabet": "0-9a-zA-Z"}, Output: "hello"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return decodeBaseX(removeSpaces(string(in)), base62Chars[a.String("alphabet")], "Base62")
	},
}

// Leading zero bytes become the first character, to survive a round trip.
func encodeBaseX(in []byte, alphabet string) []byte {
	base := len(alphabet)
	zeros := 0
	for zeros < len(in) && in[zeros] == 0 {
		zeros++
	}
	var digits []int // little-endian
	for _, b := range in[zeros:] {
		carry := int(b)
		for j := range digits {
			carry += digits[j] << 8
			digits[j] = carry % base
			carry /= base
		}
		for carry > 0 {
			digits = append(digits, carry%base)
			carry /= base
		}
	}
	out := bytes.Repeat([]byte{alphabet[0]}, zeros)
	for i := len(digits) - 1; i >= 0; i-- {
		out = append(out, alphabet[digits[i]])
	}
	return out
}

func decodeBaseX(text, alphabet, name string) ([]byte, error) {
	base := len(alphabet)
	zeros := 0
	for zeros < len(text) && text[zeros] == alphabet[0] {
		zeros++
	}
	var num []byte // little-endian
	for _, r := range text[zeros:] {
		carry := strings.IndexRune(alphabet, r)
		if carry < 0 {
			return nil, fmt.Errorf("invalid %s: unexpected character %q", name, string(r))
		}
		for j := range num {
			carry += int(num[j]) * base
			num[j] = byte(carry)
			carry >>= 8
		}
		for carry > 0 {
			num = append(num, byte(carry))
			carry >>= 8
		}
	}
	out := make([]byte, zeros, zeros+len(num))
	for i := len(num) - 1; i >= 0; i-- {
		out = append(out, num[i])
	}
	return out, nil
}

var hexFormats = map[string]struct{ prefix, separator string }{
	"none":   {"", ""},
	"space":  {"", " "},
	"colon":  {"", ":"},
	"comma":  {"", ","},
	"escape": {`\x`, ""},
	"0x":     {"0x", ", "},
}

var toHex = &Operation{
	ID:          "to-hex",
	Name:        "To Hex",
	Description: `Encodes raw bytes as hexadecimal text. e.g. "hello" becomes "68656c6c6f".`,
	Category:    Encoding,
	Aliases:     []string{"hex encode", "base16", "xxd"},
	Inverse:     "from-hex",
	InfoURL:     "https://wikipedia.org/wiki/Hexadecimal",
	Args: []ArgDef{
		selectArg("delimiter", "Delimiter", "none",
			Option{"none", "None: 68656c6c6f"},
			Option{"space", "Space: 68 65 6c 6c 6f"},
			Option{"colon", "Colon: 68:65:6c:6c:6f"},
			Option{"comma", "Comma: 68,65,6c,6c,6f"},
			Option{"escape", `Escape sequences: \x68\x65\x6c\x6c\x6f`},
			Option{"0x", "C array: 0x68, 0x65, 0x6c, 0x6c, 0x6f"},
		),
		boolArg("uppercase", "Uppercase", false),
	},
	Examples: []Example{
		{Input: "hello", Output: "68656c6c6f"},
		{Input: "hello", Args: Args{"delimiter": "colon", "uppercase": true}, Output: "68:65:6C:6C:6F"},
		{Input: "hi", Args: Args{"delimiter": "escape"}, Output: `\x68\x69`},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		f := hexFormats[a.String("delimiter")]
		digits := "0123456789abcdef"
		if a.Bool("uppercase") {
			digits = "0123456789ABCDEF"
		}
		var out []byte
		for i, b := range in {
			if i > 0 {
				out = append(out, f.separator...)
			}
			out = append(out, f.prefix...)
			out = append(out, digits[b>>4], digits[b&15])
		}
		return out, nil
	},
}

var hexIgnored = regexp.MustCompile(`(?i)0x|\\x|[\s:,;.%-]`)

var fromHex = &Operation{
	ID:          "from-hex",
	Name:        "From Hex",
	Description: `Decodes hexadecimal text into raw bytes, ignoring case, whitespace, separators (: , ; . % -) and 0x or \x prefixes. e.g. "68656c6c6f" becomes "hello".`,
	Category:    Encoding,
	Output:      BinaryOutput,
	Aliases:     []string{"hex decode", "base16", "unhex", "xxd -r"},
	Inverse:     "to-hex",
	InfoURL:     "https://wikipedia.org/wiki/Hexadecimal",
	Examples: []Example{
		{Input: "68656c6c6f", Output: "hello"},
		{Input: "68:65:6C:6C:6F", Output: "hello"},
		{Input: "0x68, 0x69", Output: "hi"},
		{Input: `\x68\x69`, Output: "hi"},
	},
	Run: func(in []byte, _ Args) ([]byte, error) {
		digits := hexIgnored.ReplaceAll(in, nil)
		for _, r := range string(digits) {
			if !isHexDigit(r) {
				return nil, fmt.Errorf("invalid hex: unexpected character %q", string(r))
			}
		}
		if len(digits)%2 != 0 {
			return nil, errors.New("invalid hex: odd number of digits")
		}
		out := make([]byte, len(digits)/2)
		_, err := hex.Decode(out, digits)
		return out, err
	},
}

func isHexDigit(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
}

func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c|0x20 >= 'a' && c|0x20 <= 'f':
		return int(c|0x20-'a') + 10
	}
	return -1
}

// RFC 3986 unreserved, and those also kept by JavaScript's encodeURI.
const (
	urlUnreserved = "-_.~"
	urlURISafe    = "-_.~!*'();/?:@&=+$,#"
)

var urlEncode = &Operation{
	ID:          "url-encode",
	Name:        "URL Encode",
	Description: `Encodes bytes as %XX sequences (percent-encoding). e.g. "a b" becomes "a%20b".`,
	Category:    Encoding,
	Aliases:     []string{"percent encode", "urlencode"},
	Inverse:     "url-decode",
	InfoURL:     "https://wikipedia.org/wiki/Percent-encoding",
	Args: []ArgDef{
		boolArg("encode-all", "Encode all special chars", false).
			describe("Also encode reserved characters such as / ? & = (like encodeURIComponent)."),
	},
	Examples: []Example{
		{Input: "a b&c=é", Output: "a%20b&c=%C3%A9"},
		{Input: "a b&c=é", Args: Args{"encode-all": true}, Output: "a%20b%26c%3D%C3%A9"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		keep := urlURISafe
		if a.Bool("encode-all") {
			keep = urlUnreserved
		}
		var out []byte
		for _, c := range in {
			if c < 0x80 && isAlnumOr(c, keep) {
				out = append(out, c)
			} else {
				out = fmt.Appendf(out, "%%%02X", c)
			}
		}
		return out, nil
	},
}

var urlDecode = &Operation{
	ID:          "url-decode",
	Name:        "URL Decode",
	Description: "Decodes %XX sequences (percent-encoding) into bytes. Invalid sequences are left as is.",
	Category:    Encoding,
	Output:      BinaryOutput,
	Aliases:     []string{"percent decode", "urldecode"},
	Inverse:     "url-encode",
	InfoURL:     "https://wikipedia.org/wiki/Percent-encoding",
	Args: []ArgDef{
		boolArg("plus-as-space", `Treat "+" as space`, true).
			describe("HTML form behavior (application/x-www-form-urlencoded)."),
	},
	Examples: []Example{
		{Input: "a%20b%26c%3D%C3%A9", Output: "a b&c=é"},
		{Input: "a+b", Output: "a b"},
		{Input: "a+b", Args: Args{"plus-as-space": false}, Output: "a+b"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		plus := a.Bool("plus-as-space")
		out := make([]byte, 0, len(in))
		for i := 0; i < len(in); i++ {
			c := in[i]
			if c == '%' && i+2 < len(in) {
				if hi, lo := hexValue(in[i+1]), hexValue(in[i+2]); hi >= 0 && lo >= 0 {
					out = append(out, byte(hi<<4|lo))
					i += 2
					continue
				}
			}
			if c == '+' && plus {
				c = ' '
			}
			out = append(out, c)
		}
		return out, nil
	},
}

var htmlSpecial = map[rune]string{'&': "&amp;", '<': "&lt;", '>': "&gt;", '"': "&quot;", '\'': "&#39;"}

var htmlEncode = &Operation{
	ID:          "html-encode",
	Name:        "HTML Encode",
	Description: `Escapes & < > " and ' as HTML entities. e.g. "<b>" becomes "&lt;b&gt;".`,
	Category:    Encoding,
	Output:      BinaryOutput,
	Aliases:     []string{"html escape", "html entities"},
	Inverse:     "html-decode",
	InfoURL:     "https://wikipedia.org/wiki/Character_encodings_in_HTML",
	Args: []ArgDef{
		boolArg("encode-non-ascii", "Encode non-ASCII characters", false).
			describe("Also write characters outside ASCII as numeric entities, e.g. é as &#233;."),
	},
	Examples: []Example{
		{Input: `<a href="x">Tom & Jerry</a>`, Output: "&lt;a href=&quot;x&quot;&gt;Tom &amp; Jerry&lt;/a&gt;"},
		{Input: "café €", Args: Args{"encode-non-ascii": true}, Output: "caf&#233; &#8364;"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		nonASCII := a.Bool("encode-non-ascii")
		var out []byte
		for len(in) > 0 {
			r, size := utf8.DecodeRune(in)
			switch {
			case htmlSpecial[r] != "":
				out = append(out, htmlSpecial[r]...)
			case nonASCII && in[0] >= 0x80:
				out = fmt.Appendf(out, "&#%d;", r)
			default:
				out = append(out, in[:size]...)
			}
			in = in[size:]
		}
		return out, nil
	},
}

var htmlEntity = regexp.MustCompile(`(?i)&(?:#(\d+)|#x([0-9a-f]+)|([a-z][a-z0-9]*));`)

var htmlDecode = &Operation{
	ID:          "html-decode",
	Name:        "HTML Decode",
	Description: `Decodes HTML entities: numeric (&#233; &#xE9;) and named HTML 4 ones (&eacute;, &amp;). Unknown entities are left as is. e.g. "&lt;b&gt;" becomes "<b>".`,
	Category:    Encoding,
	Output:      BinaryOutput,
	Aliases:     []string{"html unescape", "html entities"},
	Inverse:     "html-encode",
	InfoURL:     "https://wikipedia.org/wiki/Character_encodings_in_HTML",
	Examples: []Example{
		{Input: "&lt;a href=&quot;x&quot;&gt;Tom &amp; Jerry&lt;/a&gt;", Output: `<a href="x">Tom & Jerry</a>`},
		{Input: "caf&eacute; &#8364; &#x263A; &unknown;", Output: "café € ☺ &unknown;"},
	},
	Run: func(in []byte, _ Args) ([]byte, error) {
		return htmlEntity.ReplaceAllFunc(in, func(entity []byte) []byte {
			m := htmlEntity.FindSubmatch(entity)
			switch {
			case m[1] != nil:
				return codePoint(strconv.ParseUint(string(m[1]), 10, 32))
			case m[2] != nil:
				return codePoint(strconv.ParseUint(string(m[2]), 16, 32))
			}
			if r, ok := namedEntities[string(m[3])]; ok {
				return utf8.AppendRune(nil, r)
			}
			return entity
		}), nil
	},
}

// Invalid code points become U+FFFD, as in browsers.
func codePoint(n uint64, err error) []byte {
	r := rune(n)
	if err != nil || n == 0 || n > utf8.MaxRune || !utf8.ValidRune(r) {
		r = utf8.RuneError
	}
	return utf8.AppendRune(nil, r)
}

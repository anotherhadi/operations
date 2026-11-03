package operations

import (
	"math/rand/v2"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
	"golang.org/x/text/cases"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

func textOps() []*Operation {
	return []*Operation{
		toLowerCase, toUpperCase, toTitleCase,
		caseOp("to-camel-case", "To camelCase", `Joins the words with the first one in lowercase and the others capitalized. e.g. "hello world" becomes "helloWorld".`,
			[]string{"camelcase", "lower camel case"}, camelCase, Example{Input: "Hello big_world\nXMLHttpRequest", Output: "helloBigWorld\nxmlHttpRequest"}),
		caseOp("to-pascal-case", "To PascalCase", `Joins the capitalized words. e.g. "hello world" becomes "HelloWorld".`,
			[]string{"pascalcase", "upper camel case"}, pascalCase, Example{Input: "hello big_world", Output: "HelloBigWorld"}),
		caseOp("to-snake-case", "To snake_case", `Joins the lowercase words with underscores. e.g. "helloWorld" becomes "hello_world".`,
			[]string{"snakecase", "underscore"}, joinLower("_"), Example{Input: "Hello bigWorld", Output: "hello_big_world"}),
		caseOp("to-kebab-case", "To kebab-case", `Joins the lowercase words with hyphens. e.g. "helloWorld" becomes "hello-world".`,
			[]string{"kebabcase", "dash case", "hyphen"}, joinLower("-"), Example{Input: "Hello bigWorld", Output: "hello-big-world"}),
		toSlug,
		sortLines, shuffleLines, uniqueLines, reverseLines, addLineNumbers,
		reverseText, removeNewlines, removeWhitespace, escapeQuotes,
		countChars, countLines, countWords,
	}
}

// A final line ending does not start a new line: "a\nb\n" gives [a b].
func splitLines(text string) (lines []string, eol string, trailing bool) {
	eol = "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	if text == "" {
		return nil, eol, false
	}
	body, trailing := strings.CutSuffix(text, eol)
	return strings.Split(body, eol), eol, trailing
}

func mapLines(text string, f func([]string) []string) string {
	lines, eol, trailing := splitLines(text)
	if len(lines) == 0 {
		return text
	}
	out := strings.Join(f(lines), eol)
	if trailing {
		out += eol
	}
	return out
}

func eachLine(f func(string) string) func([]byte, Args) ([]byte, error) {
	return func(in []byte, _ Args) ([]byte, error) {
		return []byte(mapLines(string(in), func(lines []string) []string {
			for i, l := range lines {
				lines[i] = f(l)
			}
			return lines
		})), nil
	}
}

// Casers are not safe for concurrent use.
func upper(s string) string { return cases.Upper(language.Und).String(s) }
func lower(s string) string { return cases.Lower(language.Und).String(s) }

func capitalize(word string) string {
	r, size := utf8.DecodeRuneInString(word)
	return upper(string(r)) + lower(word[size:])
}

func isLowerish(r rune) bool {
	return unicode.IsLower(r) || unicode.Is(unicode.Lo, r) || unicode.IsMark(r)
}

// "XMLHttpRequest v2" gives XML, Http, Request, v, 2.
func words(line string) []string {
	rs := []rune(strings.NewReplacer("'", "", "’", "").Replace(line))
	var out []string
	run := func(i int, ok func(rune) bool) int {
		for i < len(rs) && ok(rs[i]) {
			i++
		}
		return i
	}
	for i := 0; i < len(rs); {
		r := rs[i]
		var end int
		switch {
		case unicode.IsUpper(r):
			uppers := run(i, unicode.IsUpper)
			switch n := uppers - i; {
			case n >= 2 && uppers < len(rs) && unicode.IsLower(rs[uppers]):
				// the last capital starts the next word
				end = uppers - 1
			case n == 1 && uppers < len(rs) && isLowerish(rs[uppers]):
				end = run(uppers, isLowerish)
			default:
				end = run(uppers, unicode.IsMark)
			}
		case isLowerish(r):
			end = run(i, isLowerish)
		case unicode.IsTitle(r):
			end = run(i+1, func(r rune) bool { return unicode.IsLower(r) || unicode.IsMark(r) })
		case unicode.IsNumber(r):
			end = run(i, unicode.IsNumber)
		default:
			i++
			continue
		}
		out = append(out, string(rs[i:end]))
		i = end
	}
	return out
}

func camelCase(ws []string) string {
	for i, w := range ws {
		if i == 0 {
			ws[i] = lower(w)
		} else {
			ws[i] = capitalize(w)
		}
	}
	return strings.Join(ws, "")
}

func pascalCase(ws []string) string {
	for i, w := range ws {
		ws[i] = capitalize(w)
	}
	return strings.Join(ws, "")
}

func joinLower(sep string) func([]string) string {
	return func(ws []string) string { return lower(strings.Join(ws, sep)) }
}

func caseOp(id, name, description string, aliases []string, join func([]string) string, example Example) *Operation {
	return &Operation{
		ID:          id,
		Name:        name,
		Description: description + " Each line is converted separately.",
		Category:    Text,
		Output:      BinaryOutput,
		Aliases:     aliases,
		Examples:    []Example{example},
		Run:         eachLine(func(line string) string { return join(words(line)) }),
	}
}

var toLowerCase = &Operation{
	ID:          "to-lower-case",
	Name:        "To lower case",
	Description: `Converts the text to lowercase. e.g. "HÉLLO" becomes "héllo".`,
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"lowercase"},
	Examples:    []Example{{Input: "HÉLLO World", Output: "héllo world"}},
	Run:         func(in []byte, _ Args) ([]byte, error) { return []byte(lower(string(in))), nil },
}

var toUpperCase = &Operation{
	ID:          "to-upper-case",
	Name:        "To UPPER CASE",
	Description: `Converts the text to uppercase. e.g. "héllo" becomes "HÉLLO".`,
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"uppercase", "capitals"},
	Examples:    []Example{{Input: "héllo world", Output: "HÉLLO WORLD"}},
	Run:         func(in []byte, _ Args) ([]byte, error) { return []byte(upper(string(in))), nil },
}

var titleWord = regexp.MustCompile(`[\pL\pN][\pL\pN\pM'’]*`)

var toTitleCase = &Operation{
	ID:          "to-title-case",
	Name:        "To Title Case",
	Description: `Capitalizes the first letter of every word and lowercases the others. e.g. "hello WORLD" becomes "Hello World".`,
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"capitalize", "titlecase"},
	Examples:    []Example{{Input: "hello WORLD, don't panic", Output: "Hello World, Don't Panic"}},
	Run: func(in []byte, _ Args) ([]byte, error) {
		return []byte(titleWord.ReplaceAllStringFunc(string(in), capitalize)), nil
	},
}

// NFKD does not reduce these to ASCII.
var transliterate = strings.NewReplacer("ß", "ss", "æ", "ae", "œ", "oe", "ø", "o", "đ", "d", "ł", "l", "þ", "th", "ð", "d")

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

var toSlug = &Operation{
	ID:          "to-slug",
	Name:        "To slug",
	Description: `Makes a URL-friendly slug: removes accents, lowercases, and replaces everything but letters and digits with a separator. e.g. "Héllo, World!" becomes "hello-world". Each line is converted separately.`,
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"slugify", "permalink"},
	Args:        []ArgDef{stringArg("separator", "Separator", "-")},
	Examples: []Example{
		{Input: "Héllo, World!", Output: "hello-world"},
		{Input: "Straße & Œuvre", Args: Args{"separator": "_"}, Output: "strasse_oeuvre"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return eachLine(func(line string) string {
			s := norm.NFKD.String(transliterate.Replace(lower(line)))
			s = strings.Map(func(r rune) rune {
				if unicode.IsMark(r) {
					return -1
				}
				return r
			}, s)
			return strings.ReplaceAll(strings.TrimSpace(nonSlug.ReplaceAllString(s, " ")), " ", a.String("separator"))
		})(in, a)
	},
}

var ignoreCaseArg = boolArg("ignore-case", "Ignore case", false)

func linesOp(f func(lines []string, a Args) []string) func([]byte, Args) ([]byte, error) {
	return func(in []byte, a Args) ([]byte, error) {
		return []byte(mapLines(string(in), func(lines []string) []string { return f(lines, a) })), nil
	}
}

var sortLines = &Operation{
	ID:          "sort-lines",
	Name:        "Sort lines",
	Description: "Sorts the lines by Unicode code point, like `LC_ALL=C sort`, or in natural order.",
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"sort", "order"},
	Args: []ArgDef{
		boolArg("reverse", "Reverse", false).describe("Sort in descending order."),
		ignoreCaseArg,
		boolArg("natural", "Natural order", false).
			describe(`Compare numbers by value ("2" before "10") and letters alphabetically ("a" before "B").`),
	},
	Examples: []Example{
		{Input: "b\nB\na\n10\n2", Output: "10\n2\nB\na\nb"},
		{Input: "b\nB\na\n10\n2", Args: Args{"natural": true, "ignore-case": true}, Output: "2\n10\na\nb\nB"},
		{Input: "a\nc\nb", Args: Args{"reverse": true}, Output: "c\nb\na"},
	},
	Run: linesOp(func(lines []string, a Args) []string {
		compare := strings.Compare
		switch {
		case a.Bool("natural"):
			opts := []collate.Option{collate.Numeric}
			if a.Bool("ignore-case") {
				opts = append(opts, collate.IgnoreCase)
			}
			compare = collate.New(language.English, opts...).CompareString
		case a.Bool("ignore-case"):
			compare = func(x, y string) int { return strings.Compare(lower(x), lower(y)) }
		}
		if a.Bool("reverse") {
			slices.SortStableFunc(lines, func(x, y string) int { return compare(y, x) })
		} else {
			slices.SortStableFunc(lines, compare)
		}
		return lines
	}),
}

// Seeded with the FNV-1a hash of the seed. Not secure.
type mulberry32 uint32

func newMulberry32(seed string) *mulberry32 {
	state := mulberry32(0x811c9dc5)
	for _, b := range []byte(seed) {
		state = (state ^ mulberry32(b)) * 0x01000193
	}
	return &state
}

func (m *mulberry32) below(n int) int {
	*m += 0x6d2b79f5
	s := uint32(*m)
	t := (s ^ s>>15) * (1 | s)
	t = (t + (t^t>>7)*(61|t)) ^ t
	return int(uint64(t^t>>14) * uint64(n) >> 32)
}

var shuffleLines = &Operation{
	ID:          "shuffle-lines",
	Name:        "Shuffle lines",
	Description: "Puts the lines in a random order.",
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"shuf", "randomize"},
	Args: []ArgDef{
		stringArg("seed", "Seed", "").
			describe("Gives the same order every time for the same seed and input. Leave empty for a random order."),
	},
	Examples: []Example{{Input: "a\nb\nc\nd", Args: Args{"seed": "42"}, Output: "c\na\nd\nb"}},
	Run: linesOp(func(lines []string, a Args) []string {
		swap := func(i, j int) { lines[i], lines[j] = lines[j], lines[i] }
		if seed := a.String("seed"); seed != "" {
			r := newMulberry32(seed)
			for i := len(lines) - 1; i > 0; i-- {
				swap(i, r.below(i+1))
			}
		} else {
			rand.Shuffle(len(lines), swap)
		}
		return lines
	}),
}

var uniqueLines = &Operation{
	ID:          "unique-lines",
	Name:        "Unique lines",
	Description: "Removes duplicate lines, keeping the first occurrence of each, even when duplicates are not adjacent.",
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"uniq", "dedupe", "remove duplicates"},
	Args:        []ArgDef{ignoreCaseArg},
	Examples: []Example{
		{Input: "a\nb\na\nc\nb", Output: "a\nb\nc"},
		{Input: "a\nA\nb", Args: Args{"ignore-case": true}, Output: "a\nb"},
	},
	Run: linesOp(func(lines []string, a Args) []string {
		seen := map[string]bool{}
		return slices.DeleteFunc(lines, func(line string) bool {
			if a.Bool("ignore-case") {
				line = lower(line)
			}
			dup := seen[line]
			seen[line] = true
			return dup
		})
	}),
}

var reverseLines = &Operation{
	ID:          "reverse-lines",
	Name:        "Reverse lines",
	Description: "Reverses the order of the lines, like `tac`.",
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"tac"},
	Examples:    []Example{{Input: "a\nb\nc\n", Output: "c\nb\na\n"}},
	Run: linesOp(func(lines []string, _ Args) []string {
		slices.Reverse(lines)
		return lines
	}),
}

var addLineNumbers = &Operation{
	ID:          "add-line-numbers",
	Name:        "Add line numbers",
	Description: "Prefixes every line with its number, right-aligned, like `cat -n`.",
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"line numberer", "nl", "cat -n"},
	Args: []ArgDef{
		intArg("start", "First number", 1),
		stringArg("separator", "Separator", " ").describe("Placed between the number and the line."),
	},
	Examples: []Example{
		{Input: "a\nb", Output: "1 a\n2 b"},
		{Input: "a\nb\nc", Args: Args{"start": 9, "separator": ": "}, Output: " 9: a\n10: b\n11: c"},
	},
	Run: linesOp(func(lines []string, a Args) []string {
		start := a.Int("start")
		width := max(len(strconv.Itoa(start)), len(strconv.Itoa(start+len(lines)-1)))
		for i, line := range lines {
			n := strconv.Itoa(start + i)
			lines[i] = strings.Repeat(" ", width-len(n)) + n + a.String("separator") + line
		}
		return lines
	}),
}

var reverseText = &Operation{
	ID:          "reverse-text",
	Name:        "Reverse text",
	Description: `Reverses the order of the characters. Accented letters and emoji stay intact. e.g. "héllo" becomes "olléh".`,
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"reverse string", "mirror"},
	Examples: []Example{
		{Input: "héllo", Output: "olléh"},
		{Input: "a👍🏽b", Output: "b👍🏽a"},
	},
	Run: func(in []byte, _ Args) ([]byte, error) {
		var graphemes [][]byte
		g := uniseg.NewGraphemes(string(in))
		for g.Next() {
			graphemes = append(graphemes, g.Bytes())
		}
		slices.Reverse(graphemes)
		return slices.Concat(graphemes...), nil
	},
}

var newline = regexp.MustCompile(`\r\n|\r|\n`)

var removeNewlines = &Operation{
	ID:          "remove-newlines",
	Name:        "Remove newlines",
	Description: "Removes every line break (LF, CRLF and CR), or replaces it with the given text.",
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"join lines", "single line", "strip newlines"},
	Args:        []ArgDef{stringArg("replacement", "Replacement", "")},
	Examples: []Example{
		{Input: "a\nb\r\nc", Output: "abc"},
		{Input: "a\nb\nc", Args: Args{"replacement": ", "}, Output: "a, b, c"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return newline.ReplaceAllLiteral(in, []byte(a.String("replacement"))), nil
	},
}

var removeWhitespace = &Operation{
	ID:          "remove-whitespace",
	Name:        "Remove whitespace",
	Description: "Removes all spaces, tabs, line breaks and other Unicode whitespace.",
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"remove spaces", "strip whitespace", "trim"},
	Args:        []ArgDef{boolArg("keep-newlines", "Keep line breaks", false)},
	Examples: []Example{
		{Input: "a b\tc\nd", Output: "abcd"},
		{Input: "a b\nc d", Args: Args{"keep-newlines": true}, Output: "ab\ncd"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		keep := a.Bool("keep-newlines")
		out := make([]byte, 0, len(in))
		for len(in) > 0 {
			r, size := utf8.DecodeRune(in)
			if !unicode.IsSpace(r) || keep && (r == '\n' || r == '\r') {
				out = append(out, in[:size]...)
			}
			in = in[size:]
		}
		return out, nil
	},
}

var quoteChars = map[string]string{"double": `"`, "single": "'", "both": `"'`}

var escapeQuotes = &Operation{
	ID:          "escape-quotes",
	Name:        "Escape quotes",
	Description: `Puts a backslash before quotes and backslashes. e.g. say "hi" becomes say \"hi\".`,
	Category:    Text,
	Output:      BinaryOutput,
	Aliases:     []string{"escape string", "addslashes"},
	Args: []ArgDef{
		selectArg("quotes", "Quotes", "double",
			Option{"double", `Double quotes (")`},
			Option{"single", "Single quotes (')"},
			Option{"both", "Both"},
		),
		boolArg("escape-backslashes", "Escape backslashes", true),
	},
	Examples: []Example{
		{Input: `say "hi" \o/`, Output: `say \"hi\" \\o/`},
		{Input: `it's "ok"`, Args: Args{"quotes": "single"}, Output: `it\'s "ok"`},
		{Input: `'a' "b" \`, Args: Args{"quotes": "both", "escape-backslashes": false}, Output: `\'a\' \"b\" \`},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		special := quoteChars[a.String("quotes")]
		if a.Bool("escape-backslashes") {
			special += `\`
		}
		var out []byte
		for _, c := range in {
			if strings.IndexByte(special, c) >= 0 {
				out = append(out, '\\')
			}
			out = append(out, c)
		}
		return out, nil
	},
}

func number(n int) ([]byte, error) { return strconv.AppendInt(nil, int64(n), 10), nil }

var countChars = &Operation{
	ID:          "count-chars",
	Name:        "Count characters",
	Description: "Counts the characters of the input, line breaks included, like `wc -m`.",
	Category:    Text,
	Aliases:     []string{"length", "wc -m", "char count"},
	Args: []ArgDef{
		selectArg("unit", "Unit", "graphemes",
			Option{"graphemes", "Characters as displayed (é and 👍🏽 count as 1)"},
			Option{"codepoints", "Unicode code points"},
			Option{"bytes", "Bytes"},
		),
	},
	Examples: []Example{
		{Input: "héllo 👍🏽", Output: "7"},
		{Input: "héllo 👍🏽", Args: Args{"unit": "codepoints"}, Output: "8"},
		{Input: "héllo 👍🏽", Args: Args{"unit": "bytes"}, Output: "15"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		switch a.String("unit") {
		case "bytes":
			return number(len(in))
		case "codepoints":
			return number(utf8.RuneCount(in))
		}
		return number(uniseg.GraphemeClusterCount(string(in)))
	},
}

var countLines = &Operation{
	ID:          "count-lines",
	Name:        "Count lines",
	Description: "Counts the lines of the input. Unlike `wc -l`, a last line without a line break is counted.",
	Category:    Text,
	Aliases:     []string{"wc -l", "line count"},
	Examples: []Example{
		{Input: "a\nb\nc", Output: "3"},
		{Input: "a\nb\n", Output: "2"},
		{Input: "", Output: "0"},
	},
	Run: func(in []byte, _ Args) ([]byte, error) {
		lines, _, _ := splitLines(string(in))
		return number(len(lines))
	},
}

var countWords = &Operation{
	ID:          "count-words",
	Name:        "Count words",
	Description: "Counts the words of the input, separated by whitespace, like `wc -w`.",
	Category:    Text,
	Aliases:     []string{"wc -w", "word count"},
	Examples:    []Example{{Input: "  hello big\n world ", Output: "3"}},
	Run: func(in []byte, _ Args) ([]byte, error) {
		return number(len(strings.Fields(string(in))))
	},
}

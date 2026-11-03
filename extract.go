package operations

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

func extractOps() []*Operation {
	return []*Operation{extractIPs, extractEmails, extractURLs}
}

var listArgs = []ArgDef{
	boolArg("unique", "Unique", false).describe("Only list each value once."),
	boolArg("sort", "Sort", false),
}

type match struct {
	start, end int
	value      string
}

func list(found []match, a Args) []byte {
	slices.SortStableFunc(found, func(x, y match) int { return x.start - y.start })
	values := make([]string, len(found))
	for i, m := range found {
		values[i] = m.value
	}
	if a.Bool("unique") {
		seen := map[string]bool{}
		values = slices.DeleteFunc(values, func(v string) bool {
			dup := seen[v]
			seen[v] = true
			return dup
		})
	}
	if a.Bool("sort") {
		slices.Sort(values)
	}
	return []byte(strings.Join(values, "\n"))
}

func findAll(re *regexp.Regexp, text string) []match {
	var out []match
	for _, loc := range re.FindAllStringIndex(text, -1) {
		out = append(out, match{loc[0], loc[1], text[loc[0]:loc[1]]})
	}
	return out
}

const octet = `(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)`

var (
	ipv4      = regexp.MustCompile(octet + `(?:\.` + octet + `){3}`)
	ipv4Exact = regexp.MustCompile(`^` + octet + `(?:\.` + octet + `){3}$`)
	ipv6Run   = regexp.MustCompile(`[0-9A-Fa-f:.]+`)
	hexGroup  = regexp.MustCompile(`^[0-9A-Fa-f]{1,4}$`)
	wordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
)

func byteAt(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isWordByte(c byte) bool { return c != 0 && strings.IndexByte(wordChars, c) >= 0 }

func isIPv6(s string) bool {
	halves := strings.Split(s, "::")
	if len(halves) > 2 {
		return false
	}
	groups := 0
	for h, half := range halves {
		if half == "" {
			continue
		}
		parts := strings.Split(half, ":")
		for i, part := range parts {
			switch {
			// ::ffff:1.2.3.4 counts as two groups
			case h == len(halves)-1 && i == len(parts)-1 && ipv4Exact.MatchString(part):
				groups += 2
			case hexGroup.MatchString(part):
				groups++
			default:
				return false
			}
		}
	}
	if len(halves) == 2 {
		return groups < 8
	}
	return groups == 8
}

var extractIPs = &Operation{
	ID:          "extract-ips",
	Name:        "Extract IP addresses",
	Description: "Lists the IPv4 and IPv6 addresses found in the text, one per line.",
	Category:    Extractors,
	Output:      BinaryOutput,
	Aliases:     []string{"ipv4", "ipv6", "find ip", "grep ip"},
	Args: append([]ArgDef{
		selectArg("version", "Version", "both",
			Option{"both", "IPv4 and IPv6"},
			Option{"ipv4", "IPv4 only"},
			Option{"ipv6", "IPv6 only"},
		),
	}, listArgs...),
	Examples: []Example{
		{Input: "from 10.0.0.1 to 2001:db8::1, not 1.2.3.4.5 or 256.1.1.1", Output: "10.0.0.1\n2001:db8::1"},
		{Input: "::1 ::ffff:192.168.1.1 8.8.8.8", Args: Args{"version": "ipv4"}, Output: "8.8.8.8"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		text := string(in)
		var v6 []match
		for _, m := range findAll(ipv6Run, text) {
			if isWordByte(byteAt(text, m.start-1)) || isWordByte(byteAt(text, m.end)) {
				continue
			}
			m.value = strings.TrimSuffix(m.value, ".")
			m.end = m.start + len(m.value)
			if isIPv6(m.value) {
				v6 = append(v6, m)
			}
		}
		var v4 []match
		for _, m := range findAll(ipv4, text) {
			// not part of a version number like 1.2.3.4.5
			before, after := byteAt(text, m.start-1), byteAt(text, m.end)
			if isDigit(before) || before == '.' || isDigit(after) || after == '.' && isDigit(byteAt(text, m.end+1)) {
				continue
			}
			if !slices.ContainsFunc(v6, func(v match) bool { return m.start >= v.start && m.start < v.end }) {
				v4 = append(v4, m)
			}
		}
		switch a.String("version") {
		case "ipv4":
			return list(v4, a), nil
		case "ipv6":
			return list(v6, a), nil
		}
		return list(append(v4, v6...), a), nil
	},
}

var email = regexp.MustCompile("[\\pL\\pN!#$%&'*+/=?^_`{|}~-]+(?:\\.[\\pL\\pN!#$%&'*+/=?^_`{|}~-]+)*@(?:[\\pL\\pN](?:[\\pL\\pN-]*[\\pL\\pN])?\\.)+\\pL[\\pL\\pN-]*[\\pL\\pN]")

var extractEmails = &Operation{
	ID:          "extract-emails",
	Name:        "Extract email addresses",
	Description: "Lists the email addresses found in the text, one per line.",
	Category:    Extractors,
	Output:      BinaryOutput,
	Aliases:     []string{"find email", "grep email"},
	Args:        listArgs,
	Examples: []Example{
		{Input: "Contact: <john.doe@example.com>, jane+tag@mail.example.org.", Output: "john.doe@example.com\njane+tag@mail.example.org"},
		{Input: "b@x.io a@x.io b@x.io", Args: Args{"unique": true, "sort": true}, Output: "a@x.io\nb@x.io"},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		return list(findAll(email, string(in)), a), nil
	},
}

var urlPattern = regexp.MustCompile("(?i)\\b[a-z][a-z0-9+.-]*://[^\\s<>\"'`]+")

// Drops what more likely ends the sentence than the URL.
func trimURL(url string) string {
	for {
		last, _ := utf8.DecodeLastRuneInString(url)
		switch {
		case strings.ContainsRune(".,;:!?*", last):
		case last == ')' && strings.Count(url, "(") < strings.Count(url, ")"):
		case last == ']' && strings.Count(url, "[") < strings.Count(url, "]"):
		default:
			return url
		}
		url = url[:len(url)-1]
	}
}

var extractURLs = &Operation{
	ID:          "extract-urls",
	Name:        "Extract URLs",
	Description: "Lists the URLs (scheme://) found in the text, one per line.",
	Category:    Extractors,
	Output:      BinaryOutput,
	Aliases:     []string{"find url", "grep url", "links"},
	Args:        listArgs,
	Examples: []Example{
		{
			Input:  "See https://example.com/a?b=1, (or http://wiki.org/A_(b)) and ftp://x.org/file.",
			Output: "https://example.com/a?b=1\nhttp://wiki.org/A_(b)\nftp://x.org/file",
		},
	},
	Run: func(in []byte, a Args) ([]byte, error) {
		urls := findAll(urlPattern, string(in))
		for i := range urls {
			urls[i].value = trimURL(urls[i].value)
		}
		return list(urls, a), nil
	},
}

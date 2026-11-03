package operations

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// IsPrintable reports whether b is UTF-8 text that cannot drive a terminal:
// no control characters (U+009B is a CSI on some terminals), no bidi overrides.
func IsPrintable(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
		case r < 0x20, r >= 0x7f && r <= 0x9f:
			return false
		case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			return false
		}
	}
	return true
}

// Hexdump formats b like hexdump -C.
func Hexdump(b []byte) string {
	var sb strings.Builder
	for off := 0; off < len(b); off += 16 {
		row := b[off:min(off+16, len(b))]
		fmt.Fprintf(&sb, "%08x  ", off)
		for i := range 16 {
			if i == 8 {
				sb.WriteByte(' ')
			}
			if i < len(row) {
				fmt.Fprintf(&sb, "%02x ", row[i])
			} else {
				sb.WriteString("   ")
			}
		}
		sb.WriteString(" |")
		for _, c := range row {
			if c >= 0x20 && c < 0x7f {
				sb.WriteByte(c)
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("|\n")
	}
	fmt.Fprintf(&sb, "%08x", len(b))
	return sb.String()
}

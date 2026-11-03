package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sectui/operations"
)

func usage() string {
	return "ops " + getVersion() + ` - CyberChef-like data transformations

Usage:
  ops <operation> [options] [input]   Apply an operation (input from stdin if omitted)
  ops render [text]                   Run the op{} blocks of a text
  ops list                            List available operations
  ops help [operation]                Show help
  ops completions <` + strings.Join(shells, "|") + `>     Print a shell completion script

Global options:
  --raw    Write binary output to the terminal as is (by default it is shown as a hexdump)
  --help   Show help

Examples:
  ops to-base64 hello
  echo -n aGVsbG8= | ops from-base64
  cat token.txt | ops from-base64 | ops url-encode --encode-all
  ops render "hello my name is to-base64{hadi}"`
}

const renderHelp = `ops render [--raw] [text]

Replaces every op{text} block of the text (or stdin) with its result.
Arguments go between parentheses, and | chains operations from left to right:

  to-hex(delimiter=colon){hadi}
  to-base64|url-encode(encode-all){hadi}    same as url-encode(encode-all){to-base64{hadi}}

Example:
  ops render "hello my name is to-base64{hadi}"`

// Booleans that default to true are turned off with --no-*.
func cliFlag(d operations.ArgDef) string {
	if d.Type == operations.Bool && d.Default == true {
		return "--no-" + d.ID
	}
	return "--" + d.ID
}

var shellSafe = regexp.MustCompile(`^[\w\-.,/=+:@%]+$`)

func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	if utf8.ValidString(s) && !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	// in binary data, valid multibyte characters are a coincidence
	binary := !utf8.ValidString(s)
	var b strings.Builder
	b.WriteString("$'")
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		switch {
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\\' || r == '\'':
			b.WriteString(`\` + string(r))
		case r < 0x20 || r == 0x7f || binary && s[0] >= 0x80:
			for i := range size {
				fmt.Fprintf(&b, `\x%02x`, s[i])
			}
		default:
			b.WriteString(s[:size])
		}
		s = s[size:]
	}
	b.WriteString("'")
	return b.String()
}

func cliExample(op *operations.Operation, ex operations.Example) string {
	parts := []string{"ops", op.ID}
	for _, d := range op.Args {
		v, ok := ex.Args[d.ID]
		switch {
		case !ok:
		case d.Type != operations.Bool:
			parts = append(parts, "--"+d.ID, shellQuote(fmt.Sprint(v)))
		case v != d.Default:
			parts = append(parts, cliFlag(d))
		}
	}
	return strings.Join(append(parts, shellQuote(ex.Input)), " ")
}

func describeArg(d operations.ArgDef) string {
	value := ""
	switch d.Type {
	case operations.Select:
		value = " <" + strings.Join(d.Values(), "|") + ">"
	case operations.String, operations.Int:
		value = " <" + d.Type.String() + ">"
	}
	details := []string{d.Name}
	if d.Description != "" {
		details = append(details, strings.TrimSuffix(d.Description, "."))
	}
	if d.Type != operations.Bool {
		details = append(details, fmt.Sprintf("default: %v", d.Default))
	}
	return "  " + cliFlag(d) + value + "\n      " + strings.Join(details, ". ")
}

func operationHelp(op *operations.Operation) string {
	lines := []string{op.ID + " - " + op.Name, "", op.Description}
	if len(op.Aliases) > 0 {
		lines = append(lines, "", "Aliases: "+strings.Join(op.Aliases, ", "))
	}
	if op.Inverse != "" {
		lines = append(lines, "Inverse: "+op.Inverse)
	}
	if len(op.Args) > 0 {
		lines = append(lines, "", "Options:")
		for _, d := range op.Args {
			lines = append(lines, describeArg(d))
		}
	}
	lines = append(lines, "", "Examples:")
	for _, ex := range op.Examples {
		lines = append(lines, "  "+cliExample(op, ex), "      -> "+strconv.Quote(ex.Output))
	}
	return strings.Join(lines, "\n")
}

func list() string {
	width := 0
	for _, op := range operations.All() {
		width = max(width, len(op.ID))
	}
	var groups []string
	for _, cat := range operations.Categories() {
		lines := []string{string(cat)}
		for _, op := range operations.All() {
			if op.Category == cat {
				lines = append(lines, fmt.Sprintf("  %-*s  %s", width, op.ID, op.Name))
			}
		}
		groups = append(groups, strings.Join(lines, "\n"))
	}
	return strings.Join(groups, "\n\n")
}

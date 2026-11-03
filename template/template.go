// Package template runs the op{text} blocks of a text.
//
//	to-hex{hi}                   6869
//	to-hex(delimiter=colon){hi}  68:69
//	to-base64|to-hex{a}          to-hex{to-base64{a}}
//
// Only known IDs at the start of a word are blocks, and text outside blocks
// is left as is. Inside a block, braces must be balanced or escaped (\{ \}
// \\). \op{x} keeps a block as text.
package template

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sectui/operations"
)

type Span struct{ Start, End int }

// Node is a *Text or a *Block.
type Node interface{ Span() Span }

type Text struct {
	Value string
	Pos   Span
}

type Block struct {
	Op       *operations.Operation
	Args     operations.Args
	Children []Node
	// Blocks of a chain share Pos and Content.
	Pos     Span
	Head    Span
	Content Span
}

func (t *Text) Span() Span  { return t.Pos }
func (b *Block) Span() Span { return b.Pos }

type Error struct {
	Msg string
	// Pos is in bytes, Column in characters.
	Pos, Line, Column int
	Err               error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s (line %d, column %d)", e.Msg, e.Line, e.Column)
}
func (e *Error) Unwrap() error { return e.Err }

func ErrorAt(src string, pos int, msg string) *Error { return newError(src, pos, nil, "%s", msg) }

func newError(src string, pos int, err error, format string, a ...any) *Error {
	before := src[:pos]
	lineStart := strings.LastIndexByte(before, '\n') + 1
	return &Error{
		Msg:    fmt.Sprintf(format, a...),
		Pos:    pos,
		Line:   strings.Count(before, "\n") + 1,
		Column: utf8.RuneCountInString(before[lineStart:]) + 1,
		Err:    err,
	}
}

type Engine struct {
	// Lookup lets a host add its own blocks, with a nil Run if it runs them
	// itself. Nil means operations.Lookup.
	Lookup func(id string) (*operations.Operation, bool)
}

var defaultEngine Engine

func Parse(src string) ([]Node, error)  { return defaultEngine.Parse(src) }
func Render(src string) ([]byte, error) { return defaultEngine.Render(src) }

func RenderText(src string) (string, error) {
	out, err := Render(src)
	return string(out), err
}

func (e *Engine) lookup(id string) (*operations.Operation, bool) {
	if e.Lookup != nil {
		return e.Lookup(id)
	}
	return operations.Lookup(id)
}

func (e *Engine) Parse(src string) ([]Node, error) {
	p := &parser{src: src, engine: e}
	return p.nodes(-1)
}

func (e *Engine) Render(src string) ([]byte, error) {
	nodes, err := e.Parse(src)
	if err != nil {
		return nil, err
	}
	return Eval(src, nodes)
}

// src is only used to locate errors.
func Eval(src string, nodes []Node) ([]byte, error) {
	var out []byte
	for _, n := range nodes {
		switch n := n.(type) {
		case *Text:
			out = append(out, n.Value...)
		case *Block:
			if n.Op.Run == nil {
				return nil, newError(src, n.Head.Start, nil, "%s: cannot be evaluated here", n.Op.ID)
			}
			input, err := Eval(src, n.Children)
			if err != nil {
				return nil, err
			}
			result, err := n.Op.Apply(input, n.Args)
			if err != nil {
				return nil, newError(src, n.Head.Start, err, "%s", err.Error())
			}
			out = append(out, result...)
		}
	}
	return out, nil
}

type parser struct {
	src    string
	engine *Engine
	i      int
}

func isIDChar(c byte) bool { return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' }

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// open is the start of the enclosing block, -1 at the top level.
func (p *parser) nodes(open int) ([]Node, error) {
	var nodes []Node
	var text strings.Builder
	textStart := p.i
	flush := func(end int) {
		if text.Len() > 0 {
			nodes = append(nodes, &Text{Value: text.String(), Pos: Span{textStart, end}})
			text.Reset()
		}
	}
	depth := 0
	for p.i < len(p.src) {
		c := p.src[p.i]
		if c == '\\' && p.i+1 < len(p.src) {
			if next := p.src[p.i+1]; open >= 0 && strings.IndexByte(`{}\`, next) >= 0 {
				text.WriteByte(next)
				p.i += 2
				continue
			}
			if _, content := p.heads(p.i + 1); content > 0 {
				text.WriteString(p.src[p.i+1 : content])
				p.i = content
				if open >= 0 {
					depth++
				}
				continue
			}
		}
		if c == '}' && open >= 0 {
			if depth == 0 {
				flush(p.i)
				return nodes, nil
			}
			depth--
		}
		if p.i == 0 || !isIDChar(p.src[p.i-1]) {
			start := p.i
			block, err := p.block()
			if err != nil {
				return nil, err
			}
			if block != nil {
				flush(start)
				nodes = append(nodes, block)
				textStart = p.i
				continue
			}
		}
		if c == '{' && open >= 0 {
			depth++
		}
		text.WriteByte(c)
		p.i++
	}
	if open >= 0 {
		return nil, newError(p.src, open, nil, "missing closing brace")
	}
	flush(p.i)
	return nodes, nil
}

type head struct {
	op        *operations.Operation
	span      Span
	argsStart int
	argsEnd   int // index of ")", or -1 without arguments
}

func (p *parser) block() (*Block, error) {
	heads, j := p.heads(p.i)
	if j < 0 {
		return nil, nil
	}

	args := make([]operations.Args, len(heads))
	for n, h := range heads {
		a, err := p.args(h)
		if err != nil {
			return nil, err
		}
		args[n] = a
	}

	start := p.i
	p.i = j
	children, err := p.nodes(start)
	if err != nil {
		return nil, err
	}
	pos, content := Span{start, p.i + 1}, Span{j, p.i}
	p.i++

	var block *Block
	for n, h := range heads {
		block = &Block{Op: h.op, Args: args[n], Children: children, Pos: pos, Head: h.span, Content: content}
		children = []Node{block}
	}
	return block, nil
}

// heads returns the index after the opening brace, or -1.
func (p *parser) heads(j int) ([]head, int) {
	var heads []head
	for {
		k := j
		for k < len(p.src) && isIDChar(p.src[k]) {
			k++
		}
		id := p.src[j:k]
		if id == "" || id[0] == '-' || id[len(id)-1] == '-' || strings.Contains(id, "--") {
			return nil, -1
		}
		op, ok := p.engine.lookup(id)
		if !ok {
			return nil, -1
		}
		h := head{op: op, argsEnd: -1}
		if k < len(p.src) && p.src[k] == '(' {
			h.argsStart = k + 1
			h.argsEnd = p.argsEnd(k + 1)
			if h.argsEnd < 0 {
				return nil, -1
			}
			k = h.argsEnd + 1
		}
		h.span = Span{j, k}
		heads = append(heads, h)
		if k < len(p.src) && p.src[k] == '{' {
			return heads, k + 1
		}
		if k < len(p.src) && p.src[k] == '|' {
			j = k + 1
			continue
		}
		return nil, -1
	}
}

func (p *parser) argsEnd(from int) int {
	var quote byte
	for j := from; j < len(p.src); j++ {
		c := p.src[j]
		switch {
		case quote != 0:
			if c == '\\' {
				j++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == ')':
			return j
		}
	}
	return -1
}

func isBareChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte("-_.+/=", c) >= 0
}

func (p *parser) args(h head) (operations.Args, error) {
	args := operations.Args{}
	for _, d := range h.op.Args {
		args[d.ID] = d.Default
	}
	if h.argsEnd < 0 {
		return args, nil
	}
	src, end := p.src, h.argsEnd
	j := h.argsStart
	seen := map[string]bool{}
	fail := func(at int, format string, a ...any) error {
		return newError(src, at, nil, format, a...)
	}
	skipSpaces := func() {
		for j < end && isSpace(src[j]) {
			j++
		}
	}

	skipSpaces()
	for j < end {
		nameStart := j
		for j < end && isIDChar(src[j]) {
			j++
		}
		name := src[nameStart:j]
		if name == "" {
			return nil, fail(j, "expected an argument name")
		}
		def, ok := h.op.Arg(name)
		if !ok {
			expected := make([]string, len(h.op.Args))
			for i, d := range h.op.Args {
				expected[i] = d.ID
			}
			if len(expected) == 0 {
				expected = []string{"none"}
			}
			return nil, fail(nameStart, "unknown argument %q for %q (expected: %s)", name, h.op.ID, strings.Join(expected, ", "))
		}
		if seen[name] {
			return nil, fail(nameStart, "duplicate argument %q", name)
		}
		seen[name] = true

		skipSpaces()
		raw, hasValue := "", false
		if j < end && src[j] == '=' {
			j++
			skipSpaces()
			var err error
			if raw, err = p.value(&j, end); err != nil {
				return nil, err
			}
			hasValue = true
		}
		skipSpaces()
		if j < end && src[j] != ',' {
			r, _ := utf8.DecodeRuneInString(src[j:])
			return nil, fail(j, "unexpected %q (use quotes for special characters)", string(r))
		}
		switch {
		case hasValue:
			v, err := def.Parse(raw)
			if err != nil {
				return nil, newError(src, nameStart, err, "%s", err.Error())
			}
			args[def.ID] = v
		case def.Type == operations.Bool:
			args[def.ID] = true
		default:
			return nil, fail(nameStart, "%q needs a value", name)
		}
		if j < end {
			j++
			skipSpaces()
			if j >= end {
				return nil, fail(j, "expected an argument name")
			}
		}
	}
	return args, nil
}

func (p *parser) value(j *int, end int) (string, error) {
	src := p.src
	if q := src[*j]; q == '"' || q == '\'' {
		var b strings.Builder
		start := *j
		for *j++; ; *j++ {
			if *j >= end {
				return "", newError(src, start, nil, "unterminated quote")
			}
			c := src[*j]
			if c == q {
				*j++
				return b.String(), nil
			}
			if c == '\\' && *j+1 < end && (src[*j+1] == q || src[*j+1] == '\\') {
				*j++
				c = src[*j]
			}
			b.WriteByte(c)
		}
	}
	start := *j
	for *j < end && isBareChar(src[*j]) {
		*j++
	}
	if *j == start {
		return "", newError(src, start, nil, "expected a value (use quotes for special characters)")
	}
	return src[start:*j], nil
}

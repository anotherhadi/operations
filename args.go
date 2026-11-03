package operations

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

type ArgType int

const (
	Bool ArgType = iota
	String
	Int
	Select
)

func (t ArgType) String() string {
	return [...]string{"boolean", "string", "number", "select"}[t]
}

type Option struct {
	Value string
	Label string
}

type ArgDef struct {
	ID          string
	Name        string
	Description string
	Type        ArgType
	// bool, string or int
	Default  any
	Options  []Option
	Min, Max *int
}

type Args map[string]any

func (a Args) Bool(id string) bool     { v, _ := a[id].(bool); return v }
func (a Args) String(id string) string { v, _ := a[id].(string); return v }
func (a Args) Int(id string) int       { v, _ := a[id].(int); return v }

func (d ArgDef) Values() []string {
	values := make([]string, len(d.Options))
	for i, o := range d.Options {
		values[i] = o.Value
	}
	return values
}

func (d ArgDef) Check(value any) (any, error) {
	switch d.Type {
	case Bool:
		if b, ok := value.(bool); ok {
			return b, nil
		}
		return nil, fmt.Errorf("%q expects true or false, got %s", d.ID, show(value))
	case String:
		if s, ok := value.(string); ok {
			return s, nil
		}
		return nil, fmt.Errorf("%q expects a string, got %s", d.ID, show(value))
	case Int:
		n, ok := toInt(value)
		if !ok {
			return nil, fmt.Errorf("%q expects an integer, got %s", d.ID, show(value))
		}
		if (d.Min != nil && n < *d.Min) || (d.Max != nil && n > *d.Max) {
			return nil, fmt.Errorf("%q must be between %s and %s, got %d", d.ID, bound(d.Min, "-∞"), bound(d.Max, "∞"), n)
		}
		return n, nil
	default:
		if s, ok := value.(string); ok && slices.Contains(d.Values(), s) {
			return s, nil
		}
		return nil, fmt.Errorf("invalid value %s for %q (expected: %s)", show(value), d.ID, strings.Join(d.Values(), ", "))
	}
}

// Parse reads a value typed in the CLI or a template.
func (d ArgDef) Parse(raw string) (any, error) {
	switch d.Type {
	case Bool:
		switch raw {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	case Int:
		if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			return d.Check(n)
		}
		return nil, fmt.Errorf("%q expects an integer, got %q", d.ID, raw)
	}
	return d.Check(raw)
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n == math.Trunc(n) && math.Abs(n) < 1<<53 {
			return int(n), true
		}
	}
	return 0, false
}

func show(v any) string {
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	return fmt.Sprint(v)
}

func bound(p *int, none string) string {
	if p == nil {
		return none
	}
	return strconv.Itoa(*p)
}

func (op *Operation) resolve(args Args) (Args, error) {
	for id := range args {
		if _, ok := op.Arg(id); !ok {
			return nil, &ArgError{Op: op.ID, Arg: id, Msg: fmt.Sprintf("unknown argument %q (expected: %s)", id, op.argList())}
		}
	}
	out := make(Args, len(op.Args))
	for _, d := range op.Args {
		v, ok := args[d.ID]
		if !ok {
			out[d.ID] = d.Default
			continue
		}
		v, err := d.Check(v)
		if err != nil {
			return nil, &ArgError{Op: op.ID, Arg: d.ID, Msg: err.Error()}
		}
		out[d.ID] = v
	}
	return out, nil
}

func (op *Operation) argList() string {
	if len(op.Args) == 0 {
		return "none"
	}
	ids := make([]string, len(op.Args))
	for i, a := range op.Args {
		ids[i] = a.ID
	}
	return strings.Join(ids, ", ")
}

func boolArg(id, name string, def bool) ArgDef {
	return ArgDef{ID: id, Name: name, Type: Bool, Default: def}
}

func stringArg(id, name, def string) ArgDef {
	return ArgDef{ID: id, Name: name, Type: String, Default: def}
}

func intArg(id, name string, def int) ArgDef {
	return ArgDef{ID: id, Name: name, Type: Int, Default: def}
}

func selectArg(id, name, def string, options ...Option) ArgDef {
	return ArgDef{ID: id, Name: name, Type: Select, Default: def, Options: options}
}

func (d ArgDef) describe(description string) ArgDef {
	d.Description = description
	return d
}

func (d ArgDef) between(min, max int) ArgDef {
	d.Min, d.Max = &min, &max
	return d
}

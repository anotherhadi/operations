// Command ops applies operations from the terminal.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"

	"github.com/spf13/pflag"

	"github.com/sectui/operations"
	"github.com/sectui/operations/template"
)

// Set with -ldflags "-X main.version=1.2.3".
var version string

func getVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return "dev"
}

type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return &usageError{fmt.Sprintf(format, a...)} }

type cli struct {
	stdin          io.Reader
	stdout, stderr io.Writer
	stdinTTY       bool
	stdoutTTY      bool
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func main() {
	// Get EPIPE when the reader goes away (`ops x | head`), and exit quietly.
	signal.Ignore(syscall.SIGPIPE)
	c := &cli{os.Stdin, os.Stdout, os.Stderr, isTerminal(os.Stdin), isTerminal(os.Stdout)}
	os.Exit(c.run(os.Args[1:]))
}

func (c *cli) run(argv []string) int {
	err := c.dispatch(argv)
	var usage *usageError
	switch {
	case err == nil, errors.Is(err, syscall.EPIPE):
		return 0
	case errors.As(err, &usage):
		fmt.Fprintln(c.stderr, "ops:", err)
		return 2
	default:
		fmt.Fprintln(c.stderr, "ops:", err)
		return 1
	}
}

func (c *cli) dispatch(argv []string) error {
	if len(argv) == 0 {
		return c.write([]byte(usage()), false)
	}
	command, rest := argv[0], argv[1:]
	switch command {
	case "-h", "--help":
		return c.write([]byte(usage()), false)
	case "-v", "--version":
		return c.write([]byte(getVersion()), false)
	case "list":
		return c.write([]byte(list()), false)
	case "help":
		if len(rest) == 0 {
			return c.write([]byte(usage()), false)
		}
		if rest[0] == "render" {
			return c.write([]byte(renderHelp), false)
		}
		op, err := lookup(rest[0])
		if err != nil {
			return err
		}
		return c.write([]byte(operationHelp(op)), false)
	case "completions":
		if len(rest) == 0 || !slices.Contains(shells, rest[0]) {
			return usagef("expected a shell: %s", strings.Join(shells, ", "))
		}
		return c.write([]byte(completions(rest[0])), false)
	case "render":
		fs, raw, help := newFlagSet("render")
		if err := parseFlags(fs, rest, "ops help render"); err != nil {
			return err
		}
		if *help {
			return c.write([]byte(renderHelp), false)
		}
		input, err := c.input(fs.Args())
		if err != nil {
			return err
		}
		out, err := template.Render(string(input))
		if err != nil {
			return err
		}
		return c.write(out, *raw)
	}

	op, err := lookup(command)
	if err != nil {
		return err
	}
	fs, raw, help := newFlagSet(op.ID)
	for _, d := range op.Args {
		if d.Type == operations.Bool {
			fs.Bool(cliFlag(d)[2:], false, d.Name)
		} else {
			fs.String(d.ID, "", d.Name)
		}
	}
	if err := parseFlags(fs, rest, "ops help "+op.ID); err != nil {
		return err
	}
	if *help {
		return c.write([]byte(operationHelp(op)), false)
	}
	args := operations.Args{}
	for _, d := range op.Args {
		name := cliFlag(d)[2:]
		if !fs.Changed(name) {
			continue
		}
		if d.Type == operations.Bool {
			v, _ := fs.GetBool(name)
			args[d.ID] = v != d.Default.(bool)
			continue
		}
		raw, _ := fs.GetString(name)
		v, err := d.Parse(raw)
		if err != nil {
			return usagef("--%s: %v", d.ID, err)
		}
		args[d.ID] = v
	}
	input, err := c.input(fs.Args())
	if err != nil {
		return err
	}
	out, err := op.Apply(input, args)
	if err != nil {
		return err
	}
	return c.write(out, *raw)
}

func lookup(id string) (*operations.Operation, error) {
	op, ok := operations.Lookup(id)
	if !ok {
		return nil, usagef("unknown operation %q (see: ops list)", id)
	}
	return op, nil
}

func newFlagSet(name string) (fs *pflag.FlagSet, raw, help *bool) {
	fs = pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	raw = fs.Bool("raw", false, "Write binary output to the terminal as is")
	help = fs.BoolP("help", "h", false, "Show help")
	return fs, raw, help
}

func parseFlags(fs *pflag.FlagSet, argv []string, hint string) error {
	if err := fs.Parse(argv); err != nil {
		return usagef("%v (see: %s)", err, hint)
	}
	return nil
}

func (c *cli) input(positionals []string) ([]byte, error) {
	if len(positionals) > 0 {
		return []byte(strings.Join(positionals, " ")), nil
	}
	if c.stdinTTY {
		return nil, usagef("no input: pass it as an argument or pipe it through stdin")
	}
	return io.ReadAll(c.stdin)
}

func (c *cli) write(out []byte, raw bool) error {
	if c.stdoutTTY {
		if !raw && !operations.IsPrintable(out) {
			fmt.Fprintln(c.stderr, "ops: output contains binary data or control characters, shown as a hexdump (use --raw to print it as is)")
			out = []byte(operations.Hexdump(out))
		}
		out = append(out, '\n')
	}
	_, err := c.stdout.Write(out)
	return err
}

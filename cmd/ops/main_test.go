package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type result struct {
	code           int
	stdout, stderr string
}

func runCLI(stdin string, tty bool, args ...string) result {
	var out, errOut bytes.Buffer
	c := &cli{stdin: strings.NewReader(stdin), stdout: &out, stderr: &errOut, stdinTTY: tty && stdin == "", stdoutTTY: tty}
	code := c.run(args)
	return result{code, out.String(), errOut.String()}
}

func TestCLI(t *testing.T) {
	cases := []struct {
		stdin string
		args  []string
		want  result
	}{
		{"", []string{"to-base64", "hello"}, result{0, "aGVsbG8=", ""}},
		{"", []string{"to-base64", "hello", "world"}, result{0, "aGVsbG8gd29ybGQ=", ""}},
		{"aGVsbG8=", []string{"from-base64"}, result{0, "hello", ""}},
		{"", []string{"to-base64", "--no-padding", "hello"}, result{0, "aGVsbG8", ""}},
		{"", []string{"to-hex", "--delimiter", "colon", "--uppercase", "hi"}, result{0, "68:69", ""}},
		{"", []string{"to-hex", "--delimiter=colon", "hi"}, result{0, "68:69", ""}},
		{"", []string{"to-base64", "--", "--help"}, result{0, "LS1oZWxw", ""}},
		{"", []string{"add-line-numbers", "--start", "5", "a"}, result{0, "5 a", ""}},
		{"", []string{"extract-ips", "--version", "ipv4", "::1 1.2.3.4"}, result{0, "1.2.3.4", ""}},
		{"", []string{"render", "to-hex(delimiter=colon){hi} to-base64|to-hex{a}"}, result{0, "68:69 59513d3d", ""}},
		{"x to-base64{y}", []string{"render"}, result{0, "x eQ==", ""}},
		{"", []string{"nope"}, result{2, "", "ops: unknown operation \"nope\" (see: ops list)\n"}},
		{"", []string{"to-hex", "--nope", "x"}, result{2, "", "ops: unknown flag: --nope (see: ops help to-hex)\n"}},
		{"", []string{"to-hex", "--delimiter", "foo", "x"}, result{2, "", "ops: --delimiter: invalid value \"foo\" for \"delimiter\" (expected: none, space, colon, comma, escape, 0x)\n"}},
		{"", []string{"bcrypt", "--rounds", "x", "x"}, result{2, "", "ops: --rounds: \"rounds\" expects an integer, got \"x\"\n"}},
		{"", []string{"from-base64", "--strict", "a!"}, result{1, "", "ops: from-base64: invalid Base64: unexpected character \"!\" at offset 1\n"}},
		{"", []string{"render", "to-base64{x"}, result{1, "", "ops: missing closing brace (line 1, column 1)\n"}},
		{"", []string{"completions", "nope"}, result{2, "", "ops: expected a shell: bash, zsh, fish\n"}},
	}
	for _, c := range cases {
		if got := runCLI(c.stdin, false, c.args...); got != c.want {
			t.Errorf("%q: got %+v, want %+v", c.args, got, c.want)
		}
	}
}

func TestTerminal(t *testing.T) {
	if got := runCLI("", true, "to-base64", "hi"); got != (result{0, "aGk=\n", ""}) {
		t.Errorf("got %+v", got)
	}
	got := runCLI("", true, "from-base64", "G1szMW0=")
	if got.code != 0 || !strings.HasPrefix(got.stdout, "00000000  1b 5b 33 31 6d") || !strings.Contains(got.stderr, "hexdump") {
		t.Errorf("got %+v", got)
	}
	if got := runCLI("", true, "from-base64", "--raw", "G1szMW0="); got.stdout != "\x1b[31m\n" {
		t.Errorf("got %+v", got)
	}
	if got := runCLI("", true, "to-base64"); got.code != 2 || !strings.Contains(got.stderr, "no input") {
		t.Errorf("got %+v", got)
	}
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}, {"help"}, {"help", "render"}, {"render", "--help"}, {"help", "to-hex"}, {"to-hex", "-h"}, {"list"}, {"--version"}} {
		if got := runCLI("", false, args...); got.code != 0 || got.stdout == "" {
			t.Errorf("%q: got %+v", args, got)
		}
	}
	help := runCLI("", false, "help", "to-base64").stdout
	for _, want := range []string{"to-base64 - To Base64", "--no-padding", "ops to-base64 --no-padding hello", `-> "aGVsbG8"`} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q:\n%s", want, help)
		}
	}
	if help := runCLI("", false, "help", "aes-decrypt").stdout; !strings.Contains(help, `$'\x18\x15G\xcejT8\xcb"$\x18V\xb3\\\xc4\x89'`) {
		t.Errorf("binary example not quoted:\n%s", help)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"hello":        "hello",
		"a b":          "'a b'",
		"it's":         `'it'\''s'`,
		"a\nb'":        `$'a\nb\''`,
		"\x00\xff":     `$'\x00\xff'`,
		"\xc4\x89\xff": `$'\xc4\x89\xff'`,
		"é\n":          `$'é\n'`,
		"é":            "'é'",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestCompletions(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		script := runCLI("", false, "completions", shell).stdout
		if !strings.Contains(script, "to-base64") || !strings.Contains(script, "base32hex") {
			t.Errorf("%s: incomplete script", shell)
		}
		if path, err := exec.LookPath(shell); err == nil {
			flag := "-n"
			if shell == "fish" {
				flag = "--no-execute"
			}
			cmd := exec.Command(path, flag)
			cmd.Stdin = strings.NewReader(script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s: syntax error: %v\n%s", shell, err, out)
			}
		}
	}
}

// The real binary: SIGPIPE handling is process-wide.
func TestBrokenPipe(t *testing.T) {
	bin := t.TempDir() + "/ops"
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	r, w, _ := os.Pipe()
	r.Close()
	cmd := exec.Command(bin, "to-hex")
	cmd.Stdin = strings.NewReader(strings.Repeat("x", 1<<20))
	cmd.Stdout = w
	err := cmd.Run()
	w.Close()
	if err != nil {
		t.Errorf("got %v, want a quiet exit 0", err)
	}
}

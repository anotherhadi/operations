package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sectui/operations"
)

var shells = []string{"bash", "zsh", "fish"}

var commands = [][2]string{
	{"list", "List available operations"},
	{"help", "Show help"},
	{"render", "Run the op{} blocks of a text"},
	{"completions", "Print a shell completion script"},
}

var globalFlags = [][2]string{
	{"--raw", "Write raw bytes even to a terminal"},
	{"--help", "Show help"},
}

func sq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

type flag struct {
	flag, description string
	// nil for booleans, empty for free values
	values []string
}

func flagsOf(op *operations.Operation) []flag {
	var out []flag
	for _, d := range op.Args {
		f := flag{flag: cliFlag(d), description: d.Name}
		switch d.Type {
		case operations.Select:
			f.values = d.Values()
		case operations.String, operations.Int:
			f.values = []string{}
		}
		out = append(out, f)
	}
	return out
}

func ids() []string {
	var out []string
	for _, op := range operations.All() {
		out = append(out, op.ID)
	}
	return out
}

func firsts(pairs [][2]string) []string {
	var out []string
	for _, p := range pairs {
		out = append(out, p[0])
	}
	return out
}

func completions(shell string) string {
	switch shell {
	case "bash":
		return bash()
	case "zsh":
		return zsh()
	}
	return fish()
}

func bash() string {
	var cases []string
	for _, op := range operations.All() {
		flags := flagsOf(op)
		var valueCases, all []string
		for _, f := range flags {
			all = append(all, f.flag)
			if f.values != nil {
				valueCases = append(valueCases, fmt.Sprintf(`        %s) COMPREPLY=($(compgen -W %s -- "$cur")); return ;;`, f.flag, sq(strings.Join(f.values, " "))))
			}
		}
		all = append(all, firsts(globalFlags)...)
		lines := []string{"    " + op.ID + ")"}
		if len(valueCases) > 0 {
			lines = append(lines, `      case "$prev" in`)
			lines = append(lines, valueCases...)
			lines = append(lines, "      esac")
		}
		lines = append(lines, fmt.Sprintf(`      [[ $cur == -* ]] && COMPREPLY=($(compgen -W %s -- "$cur"))`, sq(strings.Join(all, " "))), "      ;;")
		cases = append(cases, strings.Join(lines, "\n"))
	}
	allIDs := strings.Join(ids(), " ")
	return fmt.Sprintf(`# bash completion for ops
_ops() {
  local cur="${COMP_WORDS[COMP_CWORD]}" prev="${COMP_WORDS[COMP_CWORD-1]}"
  if (( COMP_CWORD == 1 )); then
    COMPREPLY=($(compgen -W %s -- "$cur"))
    return
  fi
  case "${COMP_WORDS[1]}" in
    help) (( COMP_CWORD == 2 )) && COMPREPLY=($(compgen -W %s -- "$cur")) ;;
    completions) (( COMP_CWORD == 2 )) && COMPREPLY=($(compgen -W %s -- "$cur")) ;;
    render) [[ $cur == -* ]] && COMPREPLY=($(compgen -W %s -- "$cur")) ;;
%s
  esac
}
complete -F _ops ops
`, sq(strings.Join(firsts(commands), " ")+" "+allIDs), sq(allIDs), sq(strings.Join(shells, " ")),
		sq(strings.Join(firsts(globalFlags), " ")), strings.Join(cases, "\n"))
}

var zshSpecial = regexp.MustCompile(`([:\[\]\\])`)

func zshEscape(s string) string { return zshSpecial.ReplaceAllString(s, `\$1`) }

func zshDescribe(items [][2]string) string {
	var out []string
	for _, it := range items {
		out = append(out, sq(zshEscape(it[0])+":"+zshEscape(it[1])))
	}
	return strings.Join(out, " ")
}

func zsh() string {
	var globals []string
	for _, g := range globalFlags {
		globals = append(globals, sq(g[0]+"["+zshEscape(g[1])+"]"))
	}
	globalSpecs := strings.Join(globals, " ")
	var opItems [][2]string
	var cases []string
	for _, op := range operations.All() {
		opItems = append(opItems, [2]string{op.ID, op.Name})
		var specs []string
		for _, f := range flagsOf(op) {
			action := ""
			switch {
			case f.values == nil:
			case len(f.values) > 0:
				action = ":value:(" + strings.Join(f.values, " ") + ")"
			default:
				action = ":value:"
			}
			specs = append(specs, sq(f.flag+"["+zshEscape(f.description)+"]"+action))
		}
		specs = append(specs, globalSpecs, sq("*:input:"))
		cases = append(cases, fmt.Sprintf("    %s) _arguments %s ;;", op.ID, strings.Join(specs, " ")))
	}
	return fmt.Sprintf(`#compdef ops
_ops() {
  if (( CURRENT == 2 )); then
    local -a commands=(%s)
    _describe command commands
    return
  fi
  local cmd=$words[2]
  shift words
  (( CURRENT-- ))
  case $cmd in
    help) (( CURRENT == 2 )) && { local -a ids=(%s); _describe operation ids } ;;
    completions) (( CURRENT == 2 )) && _values shell %s ;;
    render) _arguments %s %s ;;
%s
  esac
}
if [[ $funcstack[1] == _ops ]]; then _ops "$@"; else compdef _ops ops; fi
`, zshDescribe(append(append([][2]string{}, commands...), opItems...)), zshDescribe(opItems), strings.Join(shells, " "),
		globalSpecs, sq("*:text:"), strings.Join(cases, "\n"))
}

func fish() string {
	lines := []string{"# fish completion for ops", "complete -c ops -f"}
	for _, c := range commands {
		lines = append(lines, fmt.Sprintf("complete -c ops -n __fish_use_subcommand -a %s -d %s", c[0], sq(c[1])))
	}
	for _, op := range operations.All() {
		lines = append(lines, fmt.Sprintf("complete -c ops -n __fish_use_subcommand -a %s -d %s", op.ID, sq(op.Name)))
	}
	lines = append(lines,
		fmt.Sprintf("complete -c ops -n '__fish_seen_subcommand_from help' -a %s", sq(strings.Join(ids(), " "))),
		fmt.Sprintf("complete -c ops -n '__fish_seen_subcommand_from completions' -a %s", sq(strings.Join(shells, " "))),
	)
	withGlobals := "render " + strings.Join(ids(), " ")
	for _, g := range globalFlags {
		lines = append(lines, fmt.Sprintf("complete -c ops -n %s -l %s -d %s", sq("__fish_seen_subcommand_from "+withGlobals), g[0][2:], sq(g[1])))
	}
	for _, op := range operations.All() {
		for _, f := range flagsOf(op) {
			values := ""
			switch {
			case f.values == nil:
			case len(f.values) > 0:
				values = " -x -a " + sq(strings.Join(f.values, " "))
			default:
				values = " -x"
			}
			lines = append(lines, fmt.Sprintf("complete -c ops -n %s -l %s%s -d %s", sq("__fish_seen_subcommand_from "+op.ID), f.flag[2:], values, sq(f.description)))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

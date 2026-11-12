```
█▀█ █▀█ █▀▀ █▀█ ▄▀█ ▀█▀ █ █▀█ █▄ █ █▀▀
█▄█ █▀▀ ██▄ █▀▄ █▀█  █  █ █▄█ █ ▀█ ▄▄█
```

# Operations: quick and easy data transformations

Data transformations in the spirit of CyberChef: encodings, hashes, AES, JWT,
text and extractors. A Go library, the `ops` command, and an `op{text}` template
syntax. Part of the [foam](https://github.com/anotherhadi/foam) tools.

## Install

With go:

```sh
go install github.com/anotherhadi/operations/cmd/ops@latest
```

With Nix, from the [anotherhadi NUR](https://github.com/anotherhadi/nur):

```sh
nix run github:anotherhadi/nur#operations -- to-base64 hello
```

```nix
# flake: inputs.nur-anotherhadi.url = "github:anotherhadi/nur";
environment.systemPackages = [ nur-anotherhadi.packages.${system}.operations ];

# or through NUR
environment.systemPackages = [ pkgs.nur.repos.anotherhadi.operations ];
```

Each [GitHub release](https://github.com/anotherhadi/operations/releases) has an
archive per platform (Linux, macOS, Windows and FreeBSD, amd64 and arm64) with
the `ops` binary and its shell completions, and a `SHA256SUMS` file:

```sh
tar xzf ops_1.2.3_linux_amd64.tar.gz
sudo install ops /usr/local/bin/
ops --version   # 1.2.3
```

## CLI

```sh
ops to-base64 hello                                          # aGVsbG8=
cat token.txt | ops from-base64 | ops url-encode --encode-all
ops render 'hello to-base64|url-encode(encode-all){hadi?}'   # hello aGFkaT8%3D
ops list
ops help aes-encrypt
source <(ops completions zsh)
```

Input is the arguments, or stdin. On a terminal, binary output is shown as a
hexdump unless `--raw`; pipes get the bytes as they are. Exit codes: `0` ok, `1`
the operation failed, `2` usage error.

## Library

```go
import "github.com/anotherhadi/operations"

out, err := operations.Run("to-hex", []byte("hi"), operations.Args{"delimiter": "colon"}) // 68:69

for _, op := range operations.All() {
	fmt.Println(op.ID, op.Name, op.Category, op.Args)
}
```

- Argument IDs are kebab-case: `encode-all` in `Args`, `--encode-all` in the CLI
  and `encode-all` in templates.
- Omitted args take their default. A bad one gives an `*operations.ArgError`, a
  failure an `*operations.Error`.
- `IsPrintable` and `Hexdump` help to show a result safely.

## Templates

```go
import "github.com/anotherhadi/operations/template"

template.RenderText("hi to-base64{hadi}")           // hi aGFkaQ==
template.RenderText("to-hex(delimiter=colon){hi}")  // 68:69
template.RenderText("to-base64|url-encode{x}")      // same as url-encode{to-base64{x}}
```

- Blocks nest, take arguments in parentheses, and chain from left to right with
  `|`.
- Only known IDs at the start of a word are blocks, so `{x}`, `foo{x}` or JSON
  stay as they are. Text outside blocks is never changed.
- Inside a block, braces must be balanced (`to-base64{{"a":1}}` works) or
  escaped with `\{` and `\}`, and `\\` is a backslash.
- `\to-base64{a}` keeps the block as text.
- Values are bare (`[A-Za-z0-9-_.+/=]`) or quoted with `"` or `'`.
- `Parse` gives the nodes with their byte spans, for highlighting.
- `Engine{Lookup: f}` adds blocks of your own, like steep's `file{path}`. A
  block with a nil `Run` is left to the caller.

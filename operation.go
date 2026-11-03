// Package operations is a registry of CyberChef-like data transformations.
package operations

type Category string

const (
	Encoding   Category = "Encoding"
	Crypto     Category = "Crypto"
	Hashing    Category = "Hashing"
	DataFormat Category = "Data format"
	Text       Category = "Text"
	Extractors Category = "Extractors"
)

type Output int

const (
	TextOutput Output = iota
	BinaryOutput
)

type Example struct {
	Input  string
	Args   Args
	Output string
}

type Operation struct {
	ID          string
	Name        string
	Description string
	Category    Category
	Output      Output
	Aliases     []string
	Inverse     string
	InfoURL     string
	Args        []ArgDef
	Examples    []Example
	// Run gets every argument, defaults filled in.
	Run func(input []byte, args Args) ([]byte, error)
}

func (op *Operation) Arg(id string) (ArgDef, bool) {
	for _, a := range op.Args {
		if a.ID == id {
			return a, true
		}
	}
	return ArgDef{}, false
}

package operations

import "slices"

var registry = slices.Concat(
	encodingOps(),
	cryptoOps(),
	hashOps(),
	dataOps(),
	textOps(),
	extractOps(),
)

var byID = func() map[string]*Operation {
	m := make(map[string]*Operation, len(registry))
	for _, op := range registry {
		m[op.ID] = op
	}
	return m
}()

func All() []*Operation { return slices.Clone(registry) }

func Lookup(id string) (*Operation, bool) {
	op, ok := byID[id]
	return op, ok
}

func Categories() []Category {
	var cats []Category
	for _, op := range registry {
		if !slices.Contains(cats, op.Category) {
			cats = append(cats, op.Category)
		}
	}
	return cats
}

func Run(id string, input []byte, args Args) ([]byte, error) {
	op, ok := Lookup(id)
	if !ok {
		return nil, &UnknownError{ID: id}
	}
	return op.Apply(input, args)
}

func RunText(id, input string, args Args) (string, error) {
	out, err := Run(id, []byte(input), args)
	return string(out), err
}

func (op *Operation) Apply(input []byte, args Args) ([]byte, error) {
	resolved, err := op.resolve(args)
	if err != nil {
		return nil, err
	}
	out, err := op.Run(input, resolved)
	if err != nil {
		return nil, &Error{Op: op.ID, Err: err}
	}
	return out, nil
}

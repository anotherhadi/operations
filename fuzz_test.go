package operations

import "testing"

func FuzzOperations(f *testing.F) {
	f.Add(0, []byte("hello"), "", 0)
	f.Fuzz(func(t *testing.T, n int, input []byte, s string, i int) {
		ops := All()
		op := ops[uint(n)%uint(len(ops))]
		if op.ID == "bcrypt" {
			i = 4
		}
		args := Args{}
		for k, d := range op.Args {
			switch d.Type {
			case Bool:
				args[d.ID] = (i>>k)&1 == 1
			case Int:
				args[d.ID] = i
			case Select:
				args[d.ID] = d.Options[uint(i+k)%uint(len(d.Options))].Value
			default:
				args[d.ID] = s
			}
		}
		op.Apply(input, args)
	})
}

package template

import "testing"

func FuzzParse(f *testing.F) {
	for _, s := range []string{"to-hex{a}", "to-base64|to-hex(delimiter=colon){x{y}}", `\md5{a}`, "to-hex(", `to-hex(delimiter="a\"){`, "md5{{}}}{", "a|b{c}"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		nodes, err := Parse(src)
		if err != nil {
			if e, ok := err.(*Error); !ok || e.Pos < 0 || e.Pos > len(src) || e.Line < 1 || e.Column < 1 {
				t.Fatalf("%q: bad error %v", src, err)
			}
			return
		}
		var outer Span
		var check func([]Node, Span)
		check = func(nodes []Node, in Span) {
			for _, n := range nodes {
				s := n.Span()
				if _, ok := n.(*Block); ok && len(nodes) == 1 && s == outer {
					// the next operation of a chain shares its block's spans
					in = s
				}
				if s.Start < in.Start || s.End > in.End || s.Start > s.End {
					t.Fatalf("%q: span %v outside %v", src, s, in)
				}
				if b, ok := n.(*Block); ok {
					if b.Head.Start < s.Start || b.Content.End > s.End || src[b.Content.Start-1] != '{' || src[b.Content.End] != '}' {
						t.Fatalf("%q: bad block spans %+v", src, b)
					}
					outer = s
					check(b.Children, b.Content)
				}
			}
		}
		check(nodes, Span{0, len(src)})
		Eval(src, nodes)
	})
}

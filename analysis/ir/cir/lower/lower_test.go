package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cir"
	"github.com/wippyai/go-lua/analysis/ir/cir/lower"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/parse"
)

// lowerSource parses, binds, lowers, and prints src to the golden textual cir.
func lowerSource(t *testing.T, src string) string {
	t.Helper()
	stmts, err := parse.ParseString(src, "test")
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{
		Globals: []string{"type", "print", "pairs", "ipairs", "f", "g", "h", "obj", "t"},
	})
	res := lower.Chunk("main", stmts, bindings)
	return cir.Print(res.Body, res.Graph)
}

func TestGolden(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "assignment_binop",
			src:  "local c = a + b",
			want: `body main
b0: entry
b1: c = add a b
b2: exit
`,
		},
		{
			name: "if_type_number",
			src:  "local y\nif type(x) == \"number\" then y = x else y = 0 end",
			want: `body main
b0: entry
b1: y = nil
b2: branch type_eq x "number"  then b4 else b3
b3: y = 0
b4: y = x
b5: exit
`,
		},
		{
			name: "numeric_for",
			src:  "local s = 0\nfor i = 1, 10 do s = s + i end",
			want: `body main
b0: entry
b1: s = 0
b2: i = iterate.numeric [1, 10, 1]
b3: exit
b4: s = add s i
`,
		},
		{
			name: "direct_call",
			src:  "print(a, b)",
			want: `body main
b0: entry
b1: call print(a, b)
b2: exit
`,
		},
		{
			name: "cast",
			src:  "local y = x as string",
			want: `body main
b0: entry
b1: y = claim.cast x : string
b2: exit
`,
		},
		{
			name: "method_call_sugar",
			src:  "local n = obj:len()",
			want: `body main
b0: entry
b1: n = call obj:len()
b2: exit
`,
		},
		{
			name: "annotation_claim",
			src:  "local x: number = 1",
			want: `body main
b0: entry
b1: x = 1
    x = claim.annotation x : number
b2: exit
`,
		},
		{
			name: "member_and_index_write",
			src:  "obj.field = 5\nt[k] = v",
			want: `body main
b0: entry
b1: store.field obj.field = 5
b2: store.index t[k] = v
b3: exit
`,
		},
		{
			name: "multret_call_and_return",
			src:  "local a, b = f()\nreturn g(h())",
			want: `body main
b0: entry
b1: a, b = call f()
b2: %1 = call h() multret
    %0 = call g(%1...) multret
    return %0...
b3: exit
`,
		},
		{
			name: "nonnil_assert",
			src:  "local y = x!",
			want: `body main
b0: entry
b1: y = claim.assert x
b2: exit
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lowerSource(t, tc.src)
			if got != tc.want {
				t.Fatalf("cir mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", tc.name, got, tc.want)
			}
		})
	}
}

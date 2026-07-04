package cirlower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cir"
	"github.com/wippyai/go-lua/analysis/lua/cirlower"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/parse"
)

// lowerSource parses, binds, lowers, and prints src to the golden textual cir.
func lowerSource(t *testing.T, src string) string {
	t.Helper()
	return lowerSourceG(t, src, "type", "print", "pairs", "ipairs", "f", "g", "h", "obj", "t")
}

// lowerSourceG lowers src with an explicit set of recognized globals.
func lowerSourceG(t *testing.T, src string, globals ...string) string {
	t.Helper()
	stmts, err := parse.ParseString(src, "test")
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	res := cirlower.Chunk("main", stmts, bindings)
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

// TestGoldenExtended covers the constructs the prototype skipped: control-flow
// (repeat, break, goto/label), short-circuit and/or, closures and function
// definitions, table array+hash+spread constructors, channel-select, and the
// adversarial multret interactions (call in middle vs tail, multi-assign tail
// expansion).
func TestGoldenExtended(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		globals []string
		want    string
	}{
		{
			name: "repeat_until",
			src:  "local s = 0\nrepeat s = s + 1 until s > 10",
			want: `body main
b0: entry
b1: s = 0
b2: noop
b3: s = add s 1
b4: branch num_ge s 11  then b5 else b2
b5: exit
`,
		},
		{
			name:    "while_break",
			src:     "while cond do if x then break end end",
			globals: []string{"cond", "x"},
			want: `body main
b0: entry
b1: branch truthy cond  then b2 else b4
b2: branch truthy x  then b3 else b1
b3: noop
b4: exit
`,
		},
		{
			name:    "logical_and_or",
			src:     "local y = a and b or c",
			globals: []string{"a", "b", "c"},
			want: `body main
b0: entry
b1: %0 = and a b
    y = or %0 c
b2: exit
`,
		},
		{
			name: "local_function_closure",
			src:  "local function inc(n) return n + 1 end\nlocal r = inc(2)",
			want: `body main
b0: entry
b1: inc = closure main.fn0
b2: r = call inc(2)
b3: exit

body main.fn0
b0: entry
b1: %0 = add n 1
    return %0
b2: exit
`,
		},
		{
			name: "funcdef_member",
			src:  "function obj.m(v) return v end",
			want: `body main
b0: entry
b1: %0 = closure main.fn0
    store.field obj.m = %0
b2: exit

body main.fn0
b0: entry
b1: return v
b2: exit
`,
		},
		{
			name: "method_def",
			src:  "function obj:m(v) return v end",
			want: `body main
b0: entry
b1: %0 = closure main.fn0
    store.field obj.m = %0
b2: exit

body main.fn0
b0: entry
b1: return v
b2: exit
`,
		},
		{
			name: "goto_label_dead_code",
			src:  "goto done\nprint(1)\n::done::",
			want: `body main
b0: entry
b1: noop
b2: noop
b3: exit
`,
		},
		{
			name: "table_array_hash",
			src:  "local tbl = {10, x = 1, [\"k\"] = 2}",
			want: `body main
b0: entry
b1: tbl = table [10, 1, 2]
b2: exit
`,
		},
		{
			name: "table_spread_tail",
			src:  "local tbl = {1, 2, f()}",
			want: `body main
b0: entry
b1: %0 = call f() multret
    tbl = table [1, 2, %0]
b2: exit
`,
		},
		{
			name: "nested_closure",
			src:  "local mk = function() return function() return x end end",
			globals: []string{"x"},
			want: `body main
b0: entry
b1: mk = closure main.fn0
b2: exit

body main.fn0
b0: entry
b1: %0 = closure main.fn0.fn0
    return %0
b2: exit

body main.fn0.fn0
b0: entry
b1: return x
b2: exit
`,
		},
		{
			name:    "channel_select",
			src:     "type Message = {kind: string}\nlocal ch: Channel<Message>\nlocal r = channel.select { ch:case_receive(), default = true }",
			globals: []string{"channel"},
			want: `body main
b0: entry
b1: ch = nil
    ch = claim.annotation ch : Channel<{kind: string}>
b2: r = select [ch] default
b3: exit
`,
		},
		{
			name: "multret_call_in_middle_vs_tail",
			src:  "print(f(), g())",
			want: `body main
b0: entry
b1: %0 = call f()
    %1 = call g() multret
    call print(%0, %1...)
b2: exit
`,
		},
		{
			name: "multret_multi_assign_tail_expansion",
			src:  "local a, b, c = f(), g()",
			want: `body main
b0: entry
b1: a = call f()
    b, c = call g()
b2: exit
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			globals := tc.globals
			if globals == nil {
				globals = []string{"type", "print", "pairs", "ipairs", "f", "g", "h", "obj", "t"}
			}
			got := lowerSourceG(t, tc.src, globals...)
			if got != tc.want {
				t.Fatalf("cir mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", tc.name, got, tc.want)
			}
		})
	}
}

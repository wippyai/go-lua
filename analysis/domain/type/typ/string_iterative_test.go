package typ

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/annotation"
)

// TestTypeStringGoldenParity keeps the existing public product spellings
// stable while their String methods share the iterative product renderer.
func TestTypeStringGoldenParity(t *testing.T) {
	typeParam := &TypeParam{Name: "T", Constraint: String}
	generic := &Generic{Name: "Box", TypeParams: []*TypeParam{typeParam}}

	tests := []struct {
		name string
		typ  Type
		want string
	}{
		{"optional", &Optional{Inner: Number}, "number?"},
		{"optional_nil_root", &Optional{}, "nil?"},
		{"union", &Union{Members: []Type{Number, String}}, "number | string"},
		{"intersection", &Intersection{Members: []Type{Number, String}}, "number & string"},
		{"array", &Array{Element: Number}, "number[]"},
		{"map", &Map{Key: String, Value: Number}, "{[string]: number}"},
		{"readonly_map", &ReadonlyMap{Key: String, Value: Number}, "readonly {[string]: number}"},
		{"tuple", &Tuple{Elements: []Type{Number, String, Boolean}}, "(number, string, boolean)"},
		{"record", &Record{
			Fields:        []Field{{Name: "value", Type: Number, Optional: true, Readonly: true}},
			StaticMembers: []StaticMember{{Kind: StaticMemberStringIndex, Name: `x"y`, Type: String, Optional: true}},
			MapKey:        String,
			MapValue:      Boolean,
			Open:          true,
		}, `{readonly value?: number, ["x\"y"]?: string, [string]: boolean, ...}`},
		{"function", &Function{
			TypeParams: []*TypeParam{typeParam},
			Params:     []Param{{Name: "value", Type: Number, Optional: true}},
			Variadic:   String,
			Returns:    []Type{Boolean, Nil},
		}, "fun<T : string>(value: number?, ...string) -> (boolean, nil)"},
		{"interface", &Interface{Methods: []Method{{Name: "next", Type: &Function{Returns: []Type{String}}}}}, "interface { next: fun() -> string }"},
		{"meta", &Meta{Of: Number}, "typeof(number)"},
		{"generic", generic, "Box<T : string>"},
		{"instantiated", &Instantiated{Generic: generic, TypeArgs: []Type{Number}}, "Box<number>"},
		{"type_param", typeParam, "T : string"},
		{"annotated", &Annotated{
			Inner:       String,
			Annotations: []annotation.Annotation{{Name: "min", Arg: annotation.Int64Arg(7)}},
		}, "string @min(7)"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.typ.String(); got != test.want {
				t.Fatalf("String() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTypeStringIterativeTwelveThousandAcyclicProductRoot(t *testing.T) {
	const depth = 12_000
	deep := deepProductStringArrays(String, depth)
	want := "string" + strings.Repeat("[]", depth)

	if got := deep.String(); got != want {
		t.Fatalf("deep product String() differs: got length %d, want length %d", len(got), len(want))
	}

	if allocs := testing.AllocsPerRun(10, func() {
		if got := renderTypeString(deep); got != want {
			t.Fatal("deep product rendering lost deterministic output")
		}
	}); allocs > 64 {
		t.Fatalf("deep iterative product rendering allocated %.1f times/run, want bounded linear-buffer growth", allocs)
	}
}

func TestTypeStringIterativeTwelveThousandMixedRecursiveProductRoot(t *testing.T) {
	const depth = 12_000
	rec := NewRecursive("Node", func(self Type) Type {
		return &Record{Fields: []Field{{Name: "next", Type: self, Optional: true}}}
	})
	root := &Tuple{Elements: []Type{deepProductStringArrays(rec, depth)}}
	want := "(μNode. {next?: Node}" + strings.Repeat("[]", depth) + ")"

	if got := root.String(); got != want {
		t.Fatalf("mixed recursive/product String() differs: got length %d, want length %d", len(got), len(want))
	}

	if allocs := testing.AllocsPerRun(10, func() {
		if got := renderTypeString(root); got != want {
			t.Fatal("mixed recursive/product rendering lost deterministic output")
		}
	}); allocs > 64 {
		t.Fatalf("mixed iterative product rendering allocated %.1f times/run, want bounded linear-buffer growth", allocs)
	}
}

func deepProductStringArrays(leaf Type, depth int) Type {
	var out Type = leaf
	for range depth {
		out = &Array{Element: out}
	}
	return out
}

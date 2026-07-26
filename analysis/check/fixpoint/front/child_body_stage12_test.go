package front_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// TestStage2LexicalCatalogAndBoundaryMetadata is deliberately a green test:
// it asserts the metadata Stage 2 actually records, without asking the
// root-only evaluator to execute a child body.
func TestStage2LexicalCatalogAndBoundaryMetadata(t *testing.T) {
	source := `
local function outer(first, ...)
    local shared = 0
    local function middle(second)
        local function inner()
            shared = shared + first + second
            local bad: string = 1
            return network
        end
        return inner
    end
    return middle
end
`
	compilation, err := front.Compile(source)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(compilation.Catalog) != 4 {
		t.Fatalf("catalog bodies = %d, want root plus three lexical children", len(compilation.Catalog))
	}
	if !compilation.Body.Valid() || len(compilation.LexicalPath) != 0 {
		t.Fatalf("root lexical identity = %x / %v", compilation.Body, compilation.LexicalPath)
	}
	outer := onlyChild(t, compilation)
	middle := onlyChild(t, outer)
	inner := onlyChild(t, middle)
	for _, body := range []struct {
		name string
		body front.Compilation
		path []uint32
	}{
		{"outer", outer, []uint32{0}},
		{"middle", middle, []uint32{0, 0}},
		{"inner", inner, []uint32{0, 0, 0}},
	} {
		if !body.body.Body.Valid() || !reflect.DeepEqual(body.body.LexicalPath, body.path) {
			t.Fatalf("%s identity = %x / %v, want valid / %v", body.name, body.body.Body, body.body.LexicalPath, body.path)
		}
		entry, exists := compilation.Catalog[body.body.Body]
		if !exists || entry.Body != body.body.Body || !reflect.DeepEqual(entry.LexicalPath, body.path) {
			t.Fatalf("catalog[%s] = %#v, want matching body entry", body.name, entry)
		}
		for _, equation := range body.body.Artifact.Equations {
			if equation.Target.Body != body.body.Body || equation.Entry.Body != body.body.Body {
				t.Fatalf("%s equation escaped lexical body: %#v", body.name, equation)
			}
		}
	}
	if got := outer.Boundary.Parameters; len(got) != 2 || got[0].Name != "first" || got[0].Vararg || got[1].Name != "..." || !got[1].Vararg {
		t.Fatalf("outer boundary parameters = %#v", got)
	}
	if got := inner.Boundary.Captures; len(got) != 3 || got[0].Name != "shared" || !got[0].Mutable || got[1].Name != "first" || got[2].Name != "second" {
		t.Fatalf("inner boundary captures = %#v, want ordered mutable shared cell lens candidates", got)
	}
	if len(inner.QualifiedClaimSpans) != len(inner.ClaimSpans) || len(inner.QualifiedClaimSpans) == 0 {
		t.Fatalf("qualified child claim spans = %#v; legacy = %#v", inner.QualifiedClaimSpans, inner.ClaimSpans)
	}
	for key := range inner.QualifiedClaimSpans {
		if key.Body != inner.Body {
			t.Fatalf("child claim span key body = %x, want %x", key.Body, inner.Body)
		}
	}

	again, err := front.Compile(source)
	if err != nil {
		t.Fatalf("second Compile: %v", err)
	}
	if again.Body != compilation.Body || onlyChild(t, onlyChild(t, onlyChild(t, again))).Body != inner.Body {
		t.Fatal("lexical body identities are not stable across identical source admission")
	}
}

// TestCaptureMutabilityTracksEveryLexicalRebindForm keeps the front-owned T1
// theorem explicit.  The engine may trust Mutable only because the binder
// resolves these writes to their declaration symbols (rather than names).
func TestCaptureMutabilityTracksEveryLexicalRebindForm(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		mutable bool
	}{
		{"plain-assignment", `local x = 0; local function f() x = 1 end`, true},
		{"multi-assignment", `local x, y = 0, 0; local function f() x, y = 1, 2 end`, true},
		{"repeat-until", `local x = 0; local function f() repeat x = 1 until true end`, true},
		{"function-name-definition", `local x; local function f() function x() end end`, true},
		{"nested-closure-write", `local x = 0; local function f() local function g() x = 1 end; return g end`, true},
		// Loop declarations are new bindings. They must not turn an outer cell
		// mutable merely because their spelling is the same.
		{"numeric-for-shadow", `local x = 0; local function f() for x = 1, 2 do local y = x end; return x end`, false},
		{"generic-for-shadow", `local x = 0; local function f() for x in pairs({}) do local y = x end; return x end`, false},
		// Member and colon definitions mutate a table, not the lexical cell.
		{"member-definition", `local x = {}; local function f() function x.m() end; function x:m2() end end`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compilation, err := front.Compile(tc.source)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			var found *front.Compilation
			var visit func(front.Compilation)
			visit = func(body front.Compilation) {
				for i := range body.Boundary.Captures {
					if body.Boundary.Captures[i].Name == "x" {
						copy := body
						found = &copy
					}
				}
				for _, nested := range body.Nested {
					visit(nested)
				}
			}
			visit(compilation)
			if found == nil {
				t.Fatal("no child captured x")
			}
			for _, capture := range found.Boundary.Captures {
				if capture.Name == "x" && capture.Mutable != tc.mutable {
					t.Fatalf("capture Mutable = %v, want %v", capture.Mutable, tc.mutable)
				}
			}
		})
	}
}

func onlyChild(t *testing.T, compilation front.Compilation) front.Compilation {
	t.Helper()
	if len(compilation.Nested) != 1 {
		t.Fatalf("nested bodies = %d, want one: %#v", len(compilation.Nested), compilation)
	}
	return compilation.Nested[0]
}

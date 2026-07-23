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
	if got := inner.Boundary.DirectGlobals; len(got) != 1 {
		t.Fatalf("inner direct globals = %#v, want network", got)
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

func onlyChild(t *testing.T, compilation front.Compilation) front.Compilation {
	t.Helper()
	if len(compilation.Nested) != 1 {
		t.Fatalf("nested bodies = %d, want one: %#v", len(compilation.Nested), compilation)
	}
	return compilation.Nested[0]
}

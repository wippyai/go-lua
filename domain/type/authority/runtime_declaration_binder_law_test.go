package typeauthority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/runtimekind"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// RuntimeInput snapshots the admitted graph at its owner boundary. Later
// mutation of the caller's typ nodes cannot make the token bytes disagree
// with the graph SealRuntime consumes, so sealing needs no compensating
// decode/re-encode proof.
func TestRuntimeInputOwnsCanonicalGraph(t *testing.T) {
	authority := &Authority{linkID: identity.ContentID{1}, artifact: &artifactAuthority{}}
	if _, ok := authority.RuntimeInputForType(typ.NewTypeParam("open", nil)); ok {
		t.Fatal("RuntimeInput admitted a graph with a free formal")
	}
	source := typ.Func().Returns(typ.String).Build()
	input, ok := authority.RuntimeInputForType(source)
	if !ok {
		t.Fatal("mint RuntimeInput")
	}
	source.Returns[0] = typ.Number
	runtime, inners, err := SealRuntime(authority, []RuntimeInput{input})
	if err != nil || runtime == nil || len(inners) != 1 {
		t.Fatalf("SealRuntime: runtime=%v inners=%d err=%v", runtime != nil, len(inners), err)
	}
	wantID, wantOK := input.CanonicalIdentity()
	if !wantOK {
		t.Fatal("read owner-issued identity")
	}
	gotID, ok := runtime.CanonicalIdentity(inners[0])
	if !ok || gotID != wantID {
		t.Fatal("sealed Runtime followed caller mutation instead of the owner-issued snapshot")
	}
	if gotKinds, published := runtime.RuntimeKinds(inners[0]); !published || gotKinds != runtimekind.Bit(runtimekind.Function) {
		t.Fatalf("owner-issued runtime kinds = %d, published=%v; want Function", gotKinds, published)
	}
	foreign := RuntimeInner{owner: &Runtime{}, index: inners[0].index}
	if gotKinds, published := runtime.RuntimeKinds(foreign); published || gotKinds != 0 {
		t.Fatalf("foreign runtime kinds = %d, published=%v; want refusal", gotKinds, published)
	}
	if _, _, err := SealRuntime(authority, []RuntimeInput{{authority: authority}}); err == nil {
		t.Fatal("Runtime accepted a token without an owner-issued graph")
	}
}

// Runtime decomposes a declaration into structural rows, so a generic
// declaration's body becomes a row of its own that is open in the declaration's
// formals. Every self reference in that body re-enters the declaration, and the
// row still has to carry a canonical identity.
func TestRuntimeSealsSelfReferentialGenericDeclaration(t *testing.T) {
	formal := typ.NewTypeParam("T", nil)
	declaration := typ.NewGeneric("Container", []*typ.TypeParam{formal}, nil)
	declaration.SetBody(typetable.NewRecord().
		Field("_value", formal).
		Field("get", typ.Func().Param("self", typ.Instantiate(declaration, formal)).Returns(formal).Build()).
		Build())

	authority := &Authority{linkID: identity.ContentID{7}, artifact: &artifactAuthority{}}
	input, ok := authority.RuntimeInputForType(declaration)
	if !ok {
		t.Fatal("mint generic RuntimeInput")
	}
	runtime, inners, err := SealRuntime(authority, []RuntimeInput{input})
	if err != nil {
		t.Fatalf("SealRuntime: %v", err)
	}
	inner := inners[0]
	if _, ok := runtime.InnerAtIndex(inner.index); !ok {
		t.Fatal("sealed declaration is not an owned Runtime row")
	}
	var closed uint32
	for _, row := range runtime.rows {
		if !row.scopedID.Available() {
			closed++
		}
	}
	var sawOpen bool
	for index, row := range runtime.rows {
		if !row.scopedID.Available() {
			if row.rank != closed {
				t.Fatalf("closed Runtime rank = %d, want closed universe %d", row.rank, closed)
			}
			continue
		}
		sawOpen = true
		if row.rank != 0 {
			t.Fatalf("open Runtime rank = %d, want unavailable zero", row.rank)
		}
		open, ok := runtime.InnerAtIndex(uint32(index + 1))
		if !ok {
			t.Fatal("open Runtime row has no owner-fenced handle")
		}
		if got, published := runtime.RuntimeKinds(open); !published || got != runtimekind.All {
			t.Fatalf("open Runtime runtime kinds = %d, published=%v; want sound abstention %d", got, published, runtimekind.All)
		}
	}
	if !sawOpen {
		t.Fatal("generic declaration produced no scoped/open Runtime row")
	}
}

func TestRuntimeKindsPublishBottomAndRecursiveFixedPointAtOwnerSeal(t *testing.T) {
	recursive := typ.NewRecursive("Strings", func(self typ.Type) typ.Type {
		return typeexpr.Union(typ.String, self)
	})
	authority := &Authority{linkID: identity.ContentID{8}, artifact: &artifactAuthority{}}
	bottomInput, bottomOK := authority.RuntimeInputForType(typ.Never)
	cycleInput, cycleOK := authority.RuntimeInputForType(recursive)
	if !bottomOK || !cycleOK {
		t.Fatal("mint RuntimeInputs")
	}
	runtime, inners, err := SealRuntime(authority, []RuntimeInput{bottomInput, cycleInput})
	if err != nil {
		t.Fatalf("SealRuntime: %v", err)
	}
	bottom, cycle := inners[0], inners[1]
	if got, published := runtime.RuntimeKinds(bottom); !published || got != 0 {
		t.Fatalf("Never runtime kinds = %d, published=%v; want valid bottom", got, published)
	}
	if got, published := runtime.RuntimeKinds(cycle); !published || got != runtimekind.Bit(runtimekind.String) {
		t.Fatalf("recursive string runtime kinds = %d, published=%v; want String", got, published)
	}
}

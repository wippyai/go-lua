package typeauthority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestAuthoritySemanticIdentityIsSealedScalarProjection(t *testing.T) {
	referenceID := identity.ContentID{1}
	authority := &Authority{linkID: identity.ContentID{1}, artifact: &artifactAuthority{}}
	input, inputOK := authority.RuntimeInputForType(typ.String)
	if !inputOK {
		t.Fatal("mint RuntimeInput")
	}
	semanticID, semanticOK := input.CanonicalIdentity()
	if !semanticOK {
		t.Fatal("read RuntimeInput identity")
	}
	ref := StaticTypeRef{owner: identity.ContentID{9}, node: referenceID}
	authority.byReferenceID = map[identity.ContentID]Selector{referenceID: 1}
	projection := ReferenceProjection{owner: authority, ref: ref, semantic: semanticID, root: kind.String,
		may: runtimekind.Bit(runtimekind.String), name: "Text"}
	authority.entries = []entry{{ref: ref, projection: projection}}
	authority.byRef = map[StaticTypeRef]Selector{ref: 1}
	authority.runtimeInputs = []RuntimeInput{input}
	gotProjection, projected := authority.ProjectionByReferenceID(referenceID)
	got, ok := gotProjection.SemanticIdentity()
	if !projected || !ok || got != semanticID {
		t.Fatalf("SemanticIdentity() = (%x, %v), want (%x, true)", got, ok, semanticID)
	}
	if _, ok := authority.ProjectionByReferenceID(identity.ContentID{3}); ok {
		t.Fatal("semantic identity admitted a foreign reference")
	}
	closed, closedOK := projection.ClosedInput()
	closedID, closedIDOK := closed.CanonicalIdentity()
	if !closedOK || !closedIDOK || closedID != semanticID || projection.Open() {
		t.Fatal("closed projection did not carry its owner-issued Runtime input")
	}
	if _, _, err := SealRuntime(authority, []RuntimeInput{closed}); err != nil {
		t.Fatalf("SealRuntime: %v", err)
	}
	if _, retained := projection.ClosedInput(); retained {
		t.Fatal("closed projection retained its construction graph after Runtime seal")
	}
	if got, ok := projection.SemanticIdentity(); !ok || got != semanticID {
		t.Fatal("scalar projection became invalid when its construction graph was released")
	}
}

func TestStaticReferenceResolutionShapeLaw(t *testing.T) {
	tests := []struct {
		name       string
		resolution staticrefs.Resolution
		children   int
		edge       staticReferenceEdge
	}{
		{name: "unresolved leaf", resolution: staticrefs.Unresolved, children: 0, edge: staticReferenceUnresolved},
		{name: "unresolved target", resolution: staticrefs.Unresolved, children: 1},
		{name: "declaration target", resolution: staticrefs.Declaration, children: 1, edge: staticReferenceDeclaration},
		{name: "declaration missing", resolution: staticrefs.Declaration, children: 0},
		{name: "declaration extra", resolution: staticrefs.Declaration, children: 2},
		// A canonical reference names a declaration outside this Program by
		// path. The Program seal gives it no target and the artifact
		// publication gives it no child, so one child is a malformed row and
		// none is the declared shape.
		{name: "canonical leaf", resolution: staticrefs.CanonicalPath, children: 0, edge: staticReferenceCanonical},
		{name: "canonical target", resolution: staticrefs.CanonicalPath, children: 1},
		{name: "canonical extra", resolution: staticrefs.CanonicalPath, children: 2},
		{name: "unknown resolution", resolution: staticrefs.Resolution(0), children: 1},
		{name: "future resolution", resolution: staticrefs.Resolution(255), children: 1},
		{name: "negative cardinality", resolution: staticrefs.Declaration, children: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			edge := staticReferenceResolutionShape(test.resolution, test.children)
			if edge != test.edge {
				t.Fatalf("staticReferenceResolutionShape(%d, %d) = %v, want %v", test.resolution, test.children, edge, test.edge)
			}
		})
	}
}

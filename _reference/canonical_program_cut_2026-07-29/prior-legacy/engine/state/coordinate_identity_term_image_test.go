package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestCoordinateIdentityTermImageCanonicalUnionDelta(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	body := lexicalidentity.FunctionBody(namespace, 1)
	first := identity.FormalTerm(identity.NewFormalVar(identity.NewFormalSchemaID(body, 1), formal.Input))
	second := identity.FormalTerm(identity.NewFormalVar(identity.NewFormalSchemaID(body, 2), formal.Input))
	one := identity.ConcreteTerm(identity.ID{Kind: "test.image", Site: t.Name(), Index: 1})
	two := identity.ConcreteTerm(identity.ID{Kind: "test.image", Site: t.Name(), Index: 2})
	three := identity.ConcreteTerm(identity.ID{Kind: "test.image", Site: t.Name(), Index: 3})

	left, ok := NewCoordinateIdentityTermImage([]CoordinateIdentityTermBinding{
		{Source: second, Images: nil},
		{Source: first, Images: []identity.Term{two, one, one}},
	})
	if !ok {
		t.Fatal("left image")
	}
	right, ok := NewCoordinateIdentityTermImage([]CoordinateIdentityTermBinding{
		{Source: first, Images: []identity.Term{three, two}},
	})
	if !ok {
		t.Fatal("right image")
	}
	union, ok := left.Union(right)
	if !ok {
		t.Fatal("union")
	}
	want, ok := NewCoordinateIdentityTermImage([]CoordinateIdentityTermBinding{
		{Source: first, Images: []identity.Term{one, two, three}},
		{Source: second, Images: nil},
	})
	if !ok || !union.Equal(want) {
		t.Fatalf("union = %#v", union.Bindings())
	}
	delta, monotone := union.Delta(left)
	if !monotone {
		t.Fatal("union did not extend left")
	}
	deltaBindings := delta.Bindings()
	if len(deltaBindings) != 1 || deltaBindings[0].Source != first || !identityTermSlicesEqual(deltaBindings[0].Images, []identity.Term{three}) {
		t.Fatalf("delta = %#v", deltaBindings)
	}
	if _, monotone = left.Delta(union); monotone {
		t.Fatal("shrinking image accepted as monotone")
	}
	detached := union.Bindings()
	detached[0].Images[0] = identity.Term{}
	if !union.Equal(want) {
		t.Fatal("detached bindings mutated image")
	}
}

func TestCoordinateIdentityTermImageRejectsNonFormalSource(t *testing.T) {
	concrete := identity.ConcreteTerm(identity.ID{Kind: "test.image", Site: t.Name(), Index: 1})
	if image, ok := NewCoordinateIdentityTermImage([]CoordinateIdentityTermBinding{{Source: concrete, Images: []identity.Term{concrete}}}); ok || image != nil {
		t.Fatalf("non-formal source admitted: %#v", image)
	}
}

package keyspace

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestImportExistentialNamedRootIgnoresInternOrder(t *testing.T) {
	first, second, leftDestination, rightDestination := New(), New(), New(), New()
	first.FromPath(pathdom.Path{Root: "unrelated-first"})
	left := first.FromPath(pathdom.Path{Root: "captured-local"})
	second.FromPath(pathdom.Path{Root: "unrelated-second"})
	second.FromPath(pathdom.Path{Root: "another-unrelated"})
	right := second.FromPath(pathdom.Path{Root: "captured-local"})
	namespace := ExistentialNamespace{
		OwnerHi: 9, OwnerLo: 7, Point: 5, Partition: 3,
	}
	leftDestination.FromPath(pathdom.Path{Root: "destination-unrelated-left"})
	rightDestination.FromPath(pathdom.Path{Root: "destination-unrelated-right"})
	rightDestination.FromPath(pathdom.Path{Root: "destination-unrelated-extra"})
	leftExistential, leftOK := leftDestination.ImportExistential(first, left, namespace)
	rightExistential, rightOK := rightDestination.ImportExistential(second, right, namespace)
	if !leftOK || !rightOK || leftDestination.FormatReadOnly(leftExistential) != rightDestination.FormatReadOnly(rightExistential) {
		t.Fatalf("named existential depends on source intern order: %#v/%v %#v/%v", leftExistential, leftOK, rightExistential, rightOK)
	}
}

func TestBoundaryExistentialSpellingCannotCollideWithForgedUserRoot(t *testing.T) {
	source, destination := New(), New()
	root := source.FromPath(pathdom.Path{Root: "captured"})
	namespace := ExistentialNamespace{
		OwnerLo: 4, Point: 5, Partition: 6,
	}
	existential, ok := destination.ImportExistential(source, root, namespace)
	if !ok {
		t.Fatal("ImportExistential")
	}
	spelling := destination.FormatReadOnly(existential)
	forged := destination.FromPath(pathdom.Path{Root: string(spelling)})
	if forged == existential || destination.FormatReadOnly(forged) == spelling {
		t.Fatal("private existential collided with a forged ordinary named root")
	}
	roundTripExistential, ok := destination.FromStateKey(spelling)
	if !ok || roundTripExistential != existential {
		t.Fatal("existential state-key round trip failed")
	}
	roundTripForged, ok := destination.FromStateKey(destination.FormatReadOnly(forged))
	if !ok || roundTripForged != forged {
		t.Fatal("forged named-root round trip failed")
	}
}

func TestExistentialNamespaceRequiresLexicalOwner(t *testing.T) {
	if (ExistentialNamespace{Point: 1, Partition: 1}).Valid() {
		t.Fatal("ownerless namespace accepted")
	}
	if !(ExistentialNamespace{OwnerLo: 1}).Valid() {
		t.Fatal("owned lexical namespace rejected")
	}
}

func TestImportExistentialFormalRootIsStableAndFrameInjective(t *testing.T) {
	owner := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-existential")), 1)
	source := New()
	first, firstOK := source.InternFormalRoot(formal.NewRoot(owner, 1, formal.Input))
	second, secondOK := source.InternFormalRoot(formal.NewRoot(owner, 2, formal.Input))
	if !firstOK || !secondOK {
		t.Fatal("formal roots")
	}
	namespace := ExistentialNamespace{OwnerLo: 7, Point: 11, Partition: 2}
	left, right := New(), New()
	right.FromPath(pathdom.Path{Root: "unrelated"})
	leftFirst, ok := left.ImportExistential(source, first, namespace)
	if !ok {
		t.Fatal("left formal existential")
	}
	rightFirst, ok := right.ImportExistential(source, first, namespace)
	if !ok || left.FormatReadOnly(leftFirst) != right.FormatReadOnly(rightFirst) {
		t.Fatal("formal existential depends on destination interning order")
	}
	repeated, ok := left.ImportExistential(source, first, namespace)
	if !ok || repeated != leftFirst {
		t.Fatal("same formal root/frame did not reuse its existential")
	}
	otherRoot, ok := left.ImportExistential(source, second, namespace)
	if !ok || otherRoot == leftFirst {
		t.Fatal("distinct formal roots collided in one frame")
	}
	otherFrame, ok := left.ImportExistential(source, first, ExistentialNamespace{OwnerLo: 7, Point: 12, Partition: 2})
	if !ok || otherFrame == leftFirst {
		t.Fatal("same formal root collided across frames")
	}
}

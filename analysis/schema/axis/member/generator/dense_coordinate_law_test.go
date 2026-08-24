package generator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/memberroster"
)

// TestEveryAxisPublishesOneDenseCoordinateInItsOwnPackage is the generated
// half of the no-hand-export fence.
//
// The declaration states a width; this states where the type it stands for
// lands and what it is called. It lands in the axis's own package - the one
// its fact carrier is declared in, because a Factor is the pair of that fact
// and the coordinate indexing it - and it is called the same thing on every
// axis, because the spelling is not an owner's choice. An axis whose key
// carrier is borrowed from another axis therefore still publishes its own
// coordinate rather than adopting the lender's.
func TestEveryAxisPublishesOneDenseCoordinateInItsOwnPackage(t *testing.T) {
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		t.Run(source.Name, func(t *testing.T) {
			composed, composedOK := source.Compose()
			if !composedOK {
				t.Fatalf("member definition source %q does not compose", source.Name)
			}
			metadata, err := Resolve(composed)
			if err != nil {
				t.Fatalf("resolve %q: %v", source.Name, err)
			}
			if metadata.Key.Coordinate.Name != CoordinateType {
				t.Fatalf("coordinate name = %q, want the one spelling %q every axis publishes", metadata.Key.Coordinate.Name, CoordinateType)
			}
			if metadata.Key.Coordinate.PackagePath != metadata.FactType.PackagePath {
				t.Fatalf("coordinate lands in %q, want the axis's own package %q", metadata.Key.Coordinate.PackagePath, metadata.FactType.PackagePath)
			}
			if metadata.Key.Coordinate == metadata.Key.Dense {
				t.Fatal("the published coordinate is the declared width itself, so the axis has no nominal coordinate at all")
			}
			if metadata.Key.Dense.PackagePath != "" {
				t.Fatalf("declared dense width %v is qualified, so it is a hand-exported coordinate", metadata.Key.Dense)
			}
		})
	}
}

// TestTheGeneratedOwnerDeclaresTheCoordinateAndItsReadHandle states what the
// rendered artifact owes a family of another axis: the coordinate type itself,
// over the declared width, and one read handle already instantiated at this
// axis's coordinate and fact.
//
// The handle is the whole reason the type is exported. A cross-axis fold reads
// a Factor whose key and fact types it may not name; with the handle it names
// neither and still gets a read sealed at exactly those types, instead of an
// erased one it would have to reinterpret.
func TestTheGeneratedOwnerDeclaresTheCoordinateAndItsReadHandle(t *testing.T) {
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	for index := 0; index < roster.Count(); index++ {
		source, _ := roster.At(index)
		t.Run(source.Name, func(t *testing.T) {
			composed, composedOK := source.Compose()
			if !composedOK {
				t.Fatalf("member definition source %q does not compose", source.Name)
			}
			metadata, err := Resolve(composed)
			if err != nil {
				t.Fatalf("resolve %q: %v", source.Name, err)
			}
			artifact, err := Render(source.Package, composed)
			if err != nil {
				t.Fatalf("render %q: %v", source.Name, err)
			}
			rendered := string(artifact.Relations)
			declaration := fmt.Sprintf("type %s %s\n", CoordinateType, metadata.Key.Dense.Name)
			if !strings.Contains(rendered, declaration) {
				t.Fatalf("generated owner does not declare %q", strings.TrimSuffix(declaration, "\n"))
			}
			pair := fmt.Sprintf("[%s, %s]", CoordinateType, metadata.FactType.Name)
			handle := "func ForeignRead(foreign execution.ForeignFactor, coordinate execution.SelectedCoordinate, input uint16) (execution.ExactRead" + pair
			if !strings.Contains(rendered, handle) {
				t.Fatalf("generated owner publishes no read handle at %s", pair)
			}
			if !strings.Contains(rendered, "execution.ForeignExactRead"+pair) {
				t.Fatalf("the read handle of %q is not sealed at the axis's own types", source.Name)
			}
		})
	}
}

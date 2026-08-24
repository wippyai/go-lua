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

// renderedTypeArguments returns the instantiation that follows one prefix in
// the rendered artifact, so a law can compare the pair two handles are sealed
// at without restating how the generator spells a type.
func renderedTypeArguments(rendered, prefix string) (string, bool) {
	at := strings.Index(rendered, prefix)
	if at < 0 {
		return "", false
	}
	open := strings.Index(rendered[at:], "[")
	if open < 0 {
		return "", false
	}
	close := strings.Index(rendered[at+open:], "]")
	if close < 0 {
		return "", false
	}
	return rendered[at+open : at+open+close+1], true
}

// TestTheGeneratedOwnerDeclaresTheCoordinateAndItsReadHandle states what the
// rendered artifact owes a family of another axis: the coordinate type itself,
// over the declared width, and the two handles already instantiated at this
// axis's coordinate and fact - one to read a coordinate, one to enumerate the
// members of a dependent join.
//
// The handle is the whole reason the type is exported. A cross-axis fold reads
// a Factor whose key and fact types it may not name; with the handle it names
// neither and still gets a read sealed at exactly those types, instead of an
// erased one it would have to reinterpret.
//
// The pair is read out of the artifact rather than spelled here. An axis whose
// rendered owner sits in a package of its own qualifies its fact type, and how
// that type is written is the generator's business; what this law owes is that
// both handles are sealed at ONE pair, that the pair's coordinate is the type
// this file publishes, and that the pair names this axis's fact.
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
			pair, pairOK := renderedTypeArguments(rendered, "func ForeignRead(foreign execution.ForeignFactor, coordinate execution.SelectedCoordinate, input uint16) (execution.ExactRead")
			if !pairOK {
				t.Fatal("generated owner publishes no read handle")
			}
			if !strings.HasPrefix(pair, "["+CoordinateType+", ") {
				t.Fatalf("the read handle is sealed at %s, want this axis's own coordinate first", pair)
			}
			fact := strings.TrimSuffix(strings.TrimPrefix(pair, "["+CoordinateType+", "), "]")
			if fact != metadata.FactType.Name && !strings.HasSuffix(fact, "."+metadata.FactType.Name) {
				t.Fatalf("the read handle is sealed at fact %s, want this axis's own %s", fact, metadata.FactType.Name)
			}
			if !strings.Contains(rendered, "execution.ForeignExactRead"+pair) {
				t.Fatalf("the read handle of %q is not sealed at the axis's own types", source.Name)
			}
			selection := "func ForeignSelectedMember(foreign execution.ForeignFactor, dense uint32, tag uint64) (execution.RouteMember, bool)"
			if !strings.Contains(rendered, selection) || !strings.Contains(rendered, "execution.ForeignSelectedMember"+pair) {
				t.Fatalf("generated owner publishes no selection handle at %s", pair)
			}
		})
	}
}

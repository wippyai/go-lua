package composite

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/effect/factor"
	"github.com/wippyai/go-lua/domain/value"
)

// planedFamilyDeclarations is the independent statement of what each planed
// family declares. The composition below is asked to have sealed exactly these
// declarations, so a family that starts publishing without appearing here, or
// that seals a layout its own declaration does not reach, fails the law rather
// than acquiring a second authority over its wire.
func planedFamilyDeclarations(shapeOf func(schema.Key) query.Shape, vocabulary structure.Table) map[schema.Key]func() (*plane.Sealed, bool) {
	return map[schema.Key]func() (*plane.Sealed, bool){
		value.SummaryResultFamily: func() (*plane.Sealed, bool) {
			return plane.SealPublication(shapeOf(value.SummaryResultFamily), vocabulary, value.SummaryPublication())
		},
		factor.ExactResultFamily: func() (*plane.Sealed, bool) {
			return plane.SealPublication(shapeOf(factor.ExactResultFamily), vocabulary, factor.ExactPublication())
		},
	}
}

// TestEveryPublishedFamilySealsOneLayoutFromOneDeclaration states the
// declaration law. A family that publishes on the plane states one
// publication - row state vocabulary, columns, and the projection those
// columns are read out of - and the composition turns that one declaration
// into one layout. A family that publishes off the plane declares no columns
// and is sealed no layout at all, so no family ever holds two descriptions of
// its own wire.
func TestEveryPublishedFamilySealsOneLayoutFromOneDeclaration(t *testing.T) {
	state, failure := newCatalog()
	if state == nil || failure.Available() || !state.structureOK {
		t.Fatalf("the declaration table did not seal: %+v", failure)
	}
	shapeOf := func(family schema.Key) query.Shape {
		registration, found := queryRegistrationFor(state, family)
		if !found {
			t.Fatalf("no sealed registration for %q", family)
		}
		shape, shapeOK := registration.Shape()
		if !shapeOK {
			t.Fatalf("the registration of %q derives no published shape", family)
		}
		return shape
	}
	declarations := planedFamilyDeclarations(shapeOf, state.structure)

	planed := make(map[schema.Key]bool, len(declarations))
	digests := make(map[identity.ContentID]schema.Key, len(declarations))
	for position, registration := range state.queries {
		if registration == nil {
			t.Fatalf("sealed query row %d is absent", position)
		}
		family := registration.Key()
		publication := state.queryContributors[position].queryResultPublication
		layout, layoutOK := queryResultLayout(state, family)
		if !publication.planed() {
			if publication.states.Available() || len(publication.columns) != 0 {
				t.Fatalf("%q publishes off the plane yet states a row vocabulary or columns", family)
			}
			if layoutOK || layout.Available() {
				t.Fatalf("%q publishes off the plane yet the composition sealed it a layout", family)
			}
			continue
		}
		planed[family] = true
		if !layoutOK || !layout.Available() {
			t.Fatalf("%q declares a publication yet the composition sealed it no layout", family)
		}
		if prior, duplicate := digests[layout.Digest()]; duplicate {
			t.Fatalf("%q and %q publish under one layout digest", prior, family)
		}
		digests[layout.Digest()] = family

		declaration, declared := declarations[family]
		if !declared {
			t.Fatalf("%q publishes on the plane but states no declaration this law can seal independently", family)
		}
		independent, independentOK := declaration()
		if !independentOK || !independent.Available() {
			t.Fatalf("the declaration of %q does not seal on its own", family)
		}
		if independent.Digest() != layout.Digest() {
			t.Fatalf("%q: the composition sealed a layout its own declaration does not reach", family)
		}
		if independent.ColumnCount() != layout.ColumnCount() || len(publication.columns) != layout.ColumnCount() {
			t.Fatalf("%q: the sealed layout and the retained declaration disagree on column count", family)
		}
	}
	for family := range declarations {
		if !planed[family] {
			t.Fatalf("%q states a publication declaration the composition never sealed", family)
		}
	}
	if len(planed) == 0 {
		t.Fatal("no family publishes on the schema plane")
	}
}

// unplanedSelectedFamilies is the number of selected-point families whose
// answers are still detached by a codec of their own rather than by the plane
// driver. It is a ratchet: CX-10 moves Placement onto the plane and this
// becomes zero, and until then no further family may join it.
const unplanedSelectedFamilies = 1

// TestOnlyTheDeclaredRemainderDetachesItsOwnAnswer states that publishing is
// declared rather than authored. Every selected-point family is published by
// the plane driver from its own declaration, except the recorded remainder,
// and that remainder is on record here rather than discovered when a payload
// turns out to carry bytes nothing sealed.
func TestOnlyTheDeclaredRemainderDetachesItsOwnAnswer(t *testing.T) {
	state, failure := newCatalog()
	if state == nil || failure.Available() {
		t.Fatalf("the declaration table did not seal: %+v", failure)
	}
	remainder := 0
	for position, registration := range state.queries {
		if registration == nil {
			t.Fatalf("sealed query row %d is absent", position)
		}
		contributor := state.queryContributors[position]
		if registration.PopulationKind() != query.PopulationKindSelectedPoint {
			continue
		}
		if !contributor.resultComplete() {
			t.Fatalf("selected-point family %q carries no Result publication", registration.Key())
		}
		if !contributor.queryResultPublication.planed() {
			remainder++
		}
	}
	if remainder != unplanedSelectedFamilies {
		t.Fatalf("%d selected-point families detach their own answers, want the recorded remainder %d",
			remainder, unplanedSelectedFamilies)
	}
}

// domainPlaneWriters is the number of production domain packages that still
// open a plane writer of their own instead of declaring a publication. It is a
// ratchet over the census gap: the remaining walk belongs to a family that is
// not yet on the sealed query surface, and it may shrink but never grow.
const domainPlaneWriters = 1

// TestOnlyTheDriverOpensAPlaneWriter states that the payload of a published
// answer is written in exactly one place. A domain that opens a writer of its
// own is a second wire authority over the same sealed layout, so the set of
// them is bounded and reported rather than left to accumulate.
func TestOnlyTheDriverOpensAPlaneWriter(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	walked := 0
	writers := map[string]string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case entry.IsDir(), !strings.HasSuffix(path, ".go"), strings.HasSuffix(path, "_test.go"):
			return nil
		}
		file, parseErr := parser.ParseFile(set, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		walked++
		ast.Inspect(file, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector || selector.Sel.Name != "Begin" {
				return true
			}
			pkg, isIdent := selector.X.(*ast.Ident)
			if !isIdent || pkg.Name != "plane" {
				return true
			}
			relative, _ := filepath.Rel(root, path)
			writers[filepath.Dir(relative)] = relative + ":" + set.Position(call.Pos()).String()
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if walked == 0 {
		t.Fatal("the domain walk read no sources")
	}
	if len(writers) > domainPlaneWriters {
		t.Fatalf("%d domain packages open a plane writer of their own, want at most the recorded %d: %v",
			len(writers), domainPlaneWriters, writers)
	}
}

// TestPopulationDeclaresTheCapability states law (c) of the query surface: an
// execution lane is read off the sealed registration, never inferred from what
// a contributor happens to supply. A selected-point family is a Result lane
// and carries the complete Result capability; an observation family carries
// its typed producer and never acquires a fabricated selected-point admission.
func TestPopulationDeclaresTheCapability(t *testing.T) {
	state, failure := newCatalog()
	if state == nil || failure.Available() {
		t.Fatalf("the declaration table did not seal: %+v", failure)
	}
	observations := 0
	for position, registration := range state.queries {
		if registration == nil {
			t.Fatalf("sealed query row %d is absent", position)
		}
		contributor := state.queryContributors[position]
		envelope, envelopeOK := registration.ProducerEnvelope()
		if !envelopeOK || envelope.Population != registration.PopulationKind() {
			t.Fatalf("%q carries no authenticated population envelope", registration.Key())
		}
		if !contributor.producerComplete() {
			t.Fatalf("%q carries no typed producer", registration.Key())
		}
		switch registration.PopulationKind() {
		case query.PopulationKindSelectedPoint:
			if !contributor.resultComplete() {
				t.Fatalf("selected-point family %q carries no Result publication", registration.Key())
			}
		case query.PopulationKindObservation:
			observations++
			if contributor.resultComplete() {
				t.Fatalf("observation family %q acquired a Result publication lane", registration.Key())
			}
			if _, admitted := contributor.admit(nil, query.Cell{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, executioncontext.Context{}); admitted {
				t.Fatalf("observation family %q exposed a selected-point admission", registration.Key())
			}
		default:
			t.Fatalf("%q carries no declared population kind", registration.Key())
		}
	}
	if observations == 0 {
		t.Fatal("no family declares the observation population")
	}
}

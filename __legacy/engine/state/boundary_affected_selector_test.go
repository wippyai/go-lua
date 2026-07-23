package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestBoundaryAffectedSelectorClosedAtomAlgebra(t *testing.T) {
	keys := keyspace.New()
	path := keys.FromPath(pathdom.NewPath(symbol.ID(8801), "field"))
	id := identity.ConcreteTerm(identity.ID{Kind: "test", Site: t.Name(), Index: 1})
	slot := statekey.SymbolValue(8802)
	suffix := boundaryHeapSuffix{owner: id, suffix: path}

	builder := newBoundaryAffectedSelectorBuilder(keys)
	builder.anyPaths(path, path)
	builder.anyIdentities(id, id)
	builder.anySlots(slot, slot)
	builder.anyHeapSuffixes(suffix, suffix)
	selector, err := builder.seal()
	if err != nil || selector.incidenceCount() != 4 {
		t.Fatalf("selector incidences=%d err=%v, want four canonical atoms", selector.incidenceCount(), err)
	}
	if selector.affected(emptyBoundaryClosure()) {
		t.Fatal("selector affected an empty closure")
	}
	closures := []BoundaryClosure{emptyBoundaryClosure(), emptyBoundaryClosure(), emptyBoundaryClosure(), emptyBoundaryClosure()}
	closures[0].paths[path] = struct{}{}
	closures[1].identities[id] = struct{}{}
	closures[2].slots[slot] = struct{}{}
	closures[3].heapSuffixes[suffix] = struct{}{}
	for index, closure := range closures {
		if !selector.affected(closure) {
			t.Fatalf("selector missed atom family %d", index)
		}
	}

	alwaysBuilder := newBoundaryAffectedSelectorBuilder(keys)
	alwaysBuilder.always()
	always, err := alwaysBuilder.seal()
	if err != nil || !always.affected(emptyBoundaryClosure()) {
		t.Fatalf("Always affected=false err=%v", err)
	}
	neverBuilder := newBoundaryAffectedSelectorBuilder(keys)
	neverBuilder.never()
	never, err := neverBuilder.seal()
	if err != nil || never.affected(closures[0]) || never.incidenceCount() != 0 {
		t.Fatalf("Never affected=%t incidences=%d err=%v", never.affected(closures[0]), never.incidenceCount(), err)
	}
}

func TestRegisteredCoordinateAffectedSelectorsMatchBoundaryOwnership(t *testing.T) {
	keys := keyspace.New()
	left := keys.FromPath(pathdom.NewPath(symbol.ID(8811), "left"))
	right := keys.FromPath(pathdom.NewPath(symbol.ID(8812), "right"))
	id := identity.ConcreteTerm(identity.ID{Kind: "test", Site: t.Name(), Index: 2})

	tests := []struct {
		name      string
		ops       coordinateFamilyBoundaryOps
		key       coordinateKeyPayload
		incidence int
		hit       func(*BoundaryClosure)
	}{
		{
			name: "path-evidence", ops: pathEvidenceCoordinateFamilySpec.boundary,
			key: typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: pathevidence.BranchProofCoordinate(pathevidence.BranchProof{
				Kind: pathevidence.BranchProofPathEqual, Path: left, Other: right,
			})}, incidence: 2,
			hit: func(closure *BoundaryClosure) { closure.paths[right] = struct{}{} },
		},
		{name: "length-floor", ops: lenFloorCoordinateFamilySpec.boundary, key: wrapLenFloorCoordinateKey(left), incidence: 1,
			hit: func(closure *BoundaryClosure) { closure.paths[left] = struct{}{} }},
		{name: "difference-relation", ops: diffRelationCoordinateFamilySpec.boundary, key: wrapDiffRelationCoordinateKey(diffRelationCoordinateKey{
			coA: 1, a: diffRelationCoordinateOperand{path: left, kind: RelOperandValue},
			coB: -1, b: diffRelationCoordinateOperand{path: right, kind: RelOperandValue},
		}), incidence: 2, hit: func(closure *BoundaryClosure) { closure.paths[right] = struct{}{} }},
		{name: "heap", ops: heapCoordinateFamilySpec.boundary, key: wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateRoot, id: id}), incidence: 1,
			hit: func(closure *BoundaryClosure) { closure.identities[id] = struct{}{} }},
		{name: "placement", ops: placementCoordinateFamilySpec.boundary, key: wrapPlacementCoordinateKey(id), incidence: 1,
			hit: func(closure *BoundaryClosure) { closure.identities[id] = struct{}{} }},
		{name: "numeric-bound", ops: numBoundCoordinateBoundaryOps(), key: wrapNumBoundCoordinateKey(left), incidence: 1,
			hit: func(closure *BoundaryClosure) { closure.paths[left] = struct{}{} }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selector, err := test.ops.sealAffectedSelector(keys, test.key)
			if err != nil {
				t.Fatal(err)
			}
			if selector.incidenceCount() != test.incidence || selector.affected(emptyBoundaryClosure()) {
				t.Fatalf("incidences=%d affected(empty)=%t, want %d/false", selector.incidenceCount(), selector.affected(emptyBoundaryClosure()), test.incidence)
			}
			closure := emptyBoundaryClosure()
			test.hit(&closure)
			if !selector.affected(closure) {
				t.Fatal("registered selector missed the family ownership condition")
			}
		})
	}
}

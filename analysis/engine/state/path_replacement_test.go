package state

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPathReplacementInventoryIsRegistrationComplete(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	if got, want := productLaneIDs(domain.PathReplacementWriteLanes()), []LaneID{
		LanePathEvidence, LaneDynamicIndex, LaneHeapTableIdentity, LaneKeyMemberships,
		LaneTypestates, LaneLenFloors, LaneUserLattices,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("path-replacement writes = %v, want %v", got, want)
	}
	for _, lane := range domain.LaneInventory() {
		runtime, err := domain.validateLane(lane)
		if err != nil {
			t.Fatal(err)
		}
		if _, declared := pathReplacementBinding(runtime); !declared {
			t.Fatalf("lane %q has no path-replacement declaration", lane.ID())
		}
		if lane.ID() == LaneValues {
			binding, _ := pathReplacementBinding(runtime)
			if !binding.currentRead || !binding.write {
				t.Fatal("Values is not the eighth path-replacement participant")
			}
		}
	}
}

func TestConcretePathReplacementSnapshotsAliasesAndHeapOwners(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	target := mustStateKey(t, keys, pathdom.PathKey("sym401@1.child"))
	alias := mustStateKey(t, keys, pathdom.PathKey("sym402@1.alias"))
	childSuffix, _ := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	grandSuffix, _ := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "child"}, {Kind: segment.SegmentField, Name: "grand"}})
	id := identity.ID{Kind: "table", Site: "path-replacement", Index: 1}
	root := identityvalue.Present(reg, id)
	old := presentValue(reg)
	replacement := absentValue(reg)
	current := domain.Lattice().Bottom().
		WriteValue(reg, statekey.SymbolValue(401), root).
		WriteLocalPathKey(reg, target, old).
		WriteLocalPathKey(reg, alias, old).
		AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: target, Other: alias}).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root, StaticMembers: map[keyspace.Key]product.Value{childSuffix: old, grandSuffix: old}}))
	residual, values := DecomposeValueLane(domain.Lattice(), current)
	if direct := current.ReadValue(reg, statekey.SymbolValue(401)); product.Equal(reg, direct, product.Bottom(reg)) {
		t.Fatal("fixture root Value is bottom")
	}
	if _, present := values.Values[statekey.SymbolValue(401)]; !present {
		t.Fatalf("Values factor = %#v", values)
	}
	reads, readErr := domain.DecomposeLanes(residual, domain.PathReplacementReadLanes())
	if readErr != nil {
		t.Fatal(readErr)
	}
	rootKey, rootKeyOK := keys.StructuralRoot(target)
	if !rootKeyOK {
		t.Fatal("concrete structural root")
	}
	rootDependency, rootDependencyOK := pathevidence.PathValueDependency(keys, rootKey)
	if !rootDependencyOK {
		t.Fatal("concrete root dependency missing")
	}
	if concrete, _ := rootDependency.Concrete(); concrete != statekey.SymbolValue(401) {
		t.Fatalf("root dependency = %v", concrete)
	}
	if resolved, found := domain.resolvePathReplacementValue(keys, rootKey, concretePathReplacementValues{values: values}, reads); !found {
		t.Fatal("concrete root value did not resolve")
	} else if got, exact := identityvalue.ExactID(reg, resolved); !exact || got != id {
		t.Fatalf("concrete root = %#v/%v", got, exact)
	}
	tx, prepareErr := domain.PreparePathReplacement(PathReplacementConfig{Keys: keys, Target: target, Value: replacement}, concretePathReplacementValues{values: values}, reads)
	if prepareErr != nil {
		t.Fatal(prepareErr)
	}
	if len(tx.heapMutations) == 0 {
		t.Fatal("concrete heap-owner snapshot is empty")
	}
	got, applied, err := domain.ApplyConcretePathReplacement(PathReplacementConfig{Keys: keys, Target: target, Value: replacement}, current, current)
	if err != nil || !applied {
		t.Fatalf("apply: applied=%v err=%v", applied, err)
	}
	for _, key := range []keyspace.Key{target, alias} {
		if value := got.ReadLocalPathKey(reg, key); !product.Equal(reg, value, replacement) {
			t.Fatalf("alias %v = %#v, want replacement", key, value)
		}
	}
	object := got.ReadHeapTableObject(reg, id)
	if _, present := object.StaticMember(childSuffix); present {
		t.Fatal("heap child survived destructive replacement")
	}
	if _, present := object.StaticMember(grandSuffix); present {
		t.Fatal("heap descendant survived destructive replacement")
	}
}

type dependencyValueReader map[statekey.ValueDependency]product.Value

func (r dependencyValueReader) ReadPathReplacementValue(dependency statekey.ValueDependency) (product.Value, bool) {
	value, ok := r[dependency]
	return value, ok
}

func TestPathReplacementFormalRootPreservesNeutralDependencyAndHeapOwner(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 0x91
	rootSyntax := formal.NewRoot(owner, 1, formal.Input)
	root, ok := keys.InternFormalRoot(rootSyntax)
	if !ok {
		t.Fatal("formal root")
	}
	childSegment := segment.Segment{Kind: segment.SegmentField, Name: "child"}
	target, ok := keys.AppendSegment(root, childSegment)
	if !ok {
		t.Fatal("formal child")
	}
	target, ok = keys.AppendSegment(target, segment.Segment{Kind: segment.SegmentField, Name: "grand"})
	if !ok {
		t.Fatal("formal grandchild")
	}
	outer := identity.ID{Kind: "table", Site: "formal-path-replacement", Index: 1}
	inner := identity.ID{Kind: "table", Site: "formal-path-replacement", Index: 2}
	rootValue := identityvalue.Present(reg, outer)
	innerValue := identityvalue.Present(reg, inner)
	childSuffix, _ := keys.FromRootlessSuffix([]segment.Segment{childSegment})
	heapState := domain.Lattice().Bottom().
		WriteHeapTableObject(reg, outer, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: rootValue, StaticMembers: map[keyspace.Key]product.Value{childSuffix: innerValue}})).
		WriteHeapTableObject(reg, inner, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: innerValue}))
	residual, _ := DecomposeValueLane(domain.Lattice(), heapState)
	reads, err := domain.DecomposeLanes(residual, domain.PathReplacementReadLanes())
	if err != nil {
		t.Fatal(err)
	}
	dependency := statekey.FormalDependency(rootSyntax)
	derived, derivedOK := pathevidence.PathValueDependency(keys, root)
	if !derivedOK || derived != dependency {
		t.Fatalf("derived dependency = %#v/%v, want %#v", derived, derivedOK, dependency)
	}
	if resolved, found := domain.resolvePathReplacementValue(keys, root, dependencyValueReader{dependency: rootValue}, reads); !found {
		t.Fatal("formal root value did not resolve")
	} else if got, exact := identityvalue.ExactID(reg, resolved); !exact || got != outer {
		t.Fatalf("formal root resolved to %#v / %v", got, exact)
	}
	tx, err := domain.PreparePathReplacement(PathReplacementConfig{Keys: keys, Target: target, Value: product.Top()}, dependencyValueReader{dependency: rootValue}, reads)
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.rootWrites) != 1 {
		t.Fatalf("root writes = %#v", tx.rootWrites)
	}
	if got, formalRoot := tx.rootWrites[0].Formal(); !formalRoot || got != rootSyntax {
		t.Fatalf("root write lost formal identity: %#v", tx.rootWrites[0])
	}
	owners := map[identity.ID]bool{}
	for _, mutation := range tx.heapMutations {
		owners[mutation.Owner] = true
	}
	if !owners[outer] || !owners[inner] {
		t.Fatalf("formal heap owner mutations = %#v", tx.heapMutations)
	}
}

func TestPathReplacementSourceInsideTargetReadsPostInvalidationEvidence(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	target := mustStateKey(t, keys, pathdom.PathKey("sym501@1.target"))
	source := mustStateKey(t, keys, pathdom.PathKey("sym501@1.target.source"))
	sourceLeaf := mustStateKey(t, keys, pathdom.PathKey("sym501@1.target.source.leaf"))
	destinationLeaf := mustStateKey(t, keys, pathdom.PathKey("sym501@1.target.leaf"))
	root := presentValue(reg)
	copied := absentValue(reg)
	current := domain.Lattice().Bottom().WriteValue(reg, statekey.SymbolValue(501), root).
		WriteLocalPathKey(reg, target, root).WriteLocalPathStaticMember(sourceLeaf, copied)
	got, applied, err := domain.ApplyConcretePathReplacement(PathReplacementConfig{Keys: keys, Target: target, Source: source, HasSource: true, Value: root}, current, current)
	if err != nil || !applied {
		t.Fatalf("apply: %v/%v", applied, err)
	}
	if _, present := got.ReadLocalPathStaticMember(destinationLeaf); present {
		t.Fatal("source descendant was copied from pre-invalidation evidence")
	}
}

func TestPathReplacementPairedStaticPreservesExactHeapMemberOnly(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	target := mustStateKey(t, keys, pathdom.PathKey("sym601@1.child"))
	child, _ := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	grand, _ := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "child"}, {Kind: segment.SegmentField, Name: "grand"}})
	id := identity.ID{Kind: "table", Site: "paired-static", Index: 1}
	root := identityvalue.Present(reg, id)
	old := presentValue(reg)
	current := domain.Lattice().Bottom().WriteValue(reg, statekey.SymbolValue(601), root).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root, StaticMembers: map[keyspace.Key]product.Value{child: old, grand: old}}))
	got, applied, err := domain.ApplyConcretePathReplacement(PathReplacementConfig{Keys: keys, Target: target, Value: old, PairedStatic: true}, current, current)
	if err != nil || !applied {
		t.Fatalf("apply: %v/%v", applied, err)
	}
	object := got.ReadHeapTableObject(reg, id)
	if _, present := object.StaticMember(child); !present {
		t.Fatal("paired static replacement removed the exact heap member")
	}
	if _, present := object.StaticMember(grand); present {
		t.Fatal("paired static replacement retained a heap descendant")
	}
}

func TestPathReplacementUserAssignmentReadsPointEntry(t *testing.T) {
	const axisID userlattice.AxisID = "state.test.path-replacement"
	reg := userLatticeTestRegistry(t, userLatticeTestSpec(axisID, userlattice.CallBoundaryKeep))
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	targetState := pathaddr.StateKey("sym701@1.target")
	sourceState := pathaddr.StateKey("sym702@1.source")
	target, targetOK := keys.InternStateKey(targetState)
	source, sourceOK := keys.InternStateKey(sourceState)
	if !targetOK || !sourceOK {
		t.Fatal("user assignment keys")
	}
	pointEntry := domain.Lattice().Bottom().WriteUserElement(reg, keys, axisID, sourceState, "Tainted")
	current := domain.Lattice().Bottom().WriteUserElement(reg, keys, axisID, sourceState, "Sanitized")
	got, applied, err := domain.ApplyConcretePathReplacement(PathReplacementConfig{Keys: keys, Target: target, Value: product.Top(), UserSource: source, HasUserSource: true}, pointEntry, current)
	if err != nil || !applied {
		t.Fatalf("apply: %v/%v", applied, err)
	}
	if value, ok := got.ReadUserElement(reg, keys, axisID, targetState); !ok || value != "Tainted" {
		t.Fatalf("target user element = %q/%v, want point-entry Tainted", value, ok)
	}
}

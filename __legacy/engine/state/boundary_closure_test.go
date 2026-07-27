package state

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestLaneCatalogRejectsMissingBoundaryReachability(t *testing.T) {
	missing := valuesLaneSpec
	missing.id = "test.missing-boundary-reachability"
	build := missing.build
	missing.build = func(reg *axis.Registry, options DomainOptions) laneOps {
		ops := build(reg, options)
		ops.factor.boundaryReachability = nil
		return ops
	}
	defer func() {
		got := recover()
		if got == nil || !strings.Contains(got.(string), "has no typed boundary reachability program") {
			t.Fatalf("newLaneCatalog panic = %v, want missing boundary reachability", got)
		}
	}()
	_ = newLaneCatalog([]laneSpec{missing}).ProductDomain(standard.Registry())
}

func TestBoundaryClosureDispatchesEveryRegisteredLane(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	source := Domain(reg).Bottom()
	specs := append([]laneSpec(nil), defaultLaneCatalog.specs...)
	visits := make(map[LaneID]int, len(specs))
	for i := range specs {
		id := specs[i].id
		build := specs[i].build
		specs[i].build = func(reg *axis.Registry, options DomainOptions) laneOps {
			ops := build(reg, options)
			emit := ops.factor.boundaryReachability
			ops.factor.boundaryReachability = func(reg *axis.Registry, keys *keyspace.KeySpace, payload laneFactorPayload) (BoundaryReachabilityProgram, error) {
				visits[id]++
				return emit(reg, keys, payload)
			}
			return ops
		}
	}
	if _, err := buildBoundaryRootClosure(reg, keys, source, nil, specs); err != nil {
		t.Fatal(err)
	}
	if len(visits) != len(specs) {
		t.Fatalf("boundary reachability visited %d/%d lanes", len(visits), len(specs))
	}
	for _, spec := range specs {
		if visits[spec.id] == 0 {
			t.Fatalf("boundary reachability omitted lane %q", spec.id)
		}
	}
}

func TestBoundaryClosureNilInventoryUsesDefaultAndEmptyInventorySeedsOnly(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	root := keys.FromPath(pathdom.Path{Symbol: 31, Version: 1})
	alias := keys.FromPath(pathdom.Path{Symbol: 32, Version: 1})
	source := Domain(reg).Bottom().AddBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  root,
		Other: alias,
	})
	withDefault, err := buildBoundaryRootClosure(reg, keys, source, BoundaryRoots{{Path: root}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !withDefault.ContainsPath(alias) {
		t.Fatal("nil lane inventory did not select the default catalog")
	}
	seedOnly, err := buildBoundaryRootClosure(reg, keys, source, BoundaryRoots{{Path: root}}, []laneSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if seedOnly.ContainsPath(alias) {
		t.Fatal("empty selected inventory unexpectedly expanded path-evidence lane")
	}
}

func TestBoundaryClosureUsesSourceLaneSelection(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	root := keys.FromPath(pathdom.Path{Symbol: 33, Version: 1})
	alias := keys.FromPath(pathdom.Path{Symbol: 34, Version: 1})
	full := Domain(reg).Bottom().AddBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  root,
		Other: alias,
	})
	selected := NormalizeForDomain(DomainWithLanes(reg, []LaneID{LaneValues}), full)
	closure, err := BuildBoundaryRootClosure(reg, keys, selected, BoundaryRoots{{Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	if closure.ContainsPath(alias) {
		t.Fatal("disabled path-evidence lane contributed to boundary closure")
	}
}

func TestBoundaryClosureFollowsAliasesAndHeapIdentityToFixedPoint(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	rootPath := pathdom.Path{Symbol: symbol.ID(41), Version: 1}
	aliasPath := pathdom.Path{Symbol: symbol.ID(42), Version: 1}
	root := keys.FromPath(rootPath)
	alias := keys.FromPath(aliasPath)
	child := keys.FromPath(aliasPath.Field("child"))
	member, ok := heapidentity.StaticMemberSuffixKey(keys, []segment.Segment{{Kind: segment.SegmentField, Name: "leaf"}})
	if !ok {
		t.Fatal("static member key")
	}
	outerID := identity.ID{Kind: "lua.table", Site: "outer", Index: 1}
	innerID := identity.ID{Kind: "lua.table", Site: "inner", Index: 2}
	outer := identityvalue.Present(reg, outerID)
	inner := identityvalue.Present(reg, innerID)
	state := Domain(reg).Bottom().
		WriteLocalPathKey(reg, child, inner).
		WriteHeapTableObject(reg, outerID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          outer,
			StaticMembers: map[keyspace.Key]product.Value{member: inner},
		})).
		WriteHeapTableObject(reg, innerID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: inner}))
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: root, Other: alias}
	state = state.AddBranchProof(proof)
	if got := state.ReadLocalPathKey(reg, child); !product.Equal(reg, got, inner) {
		t.Fatalf("test setup path refinement missing: got=%v", got)
	}
	if !keys.HasPrefix(child, alias) {
		t.Fatalf("test setup child does not have alias prefix: child=%#v alias=%#v", child, alias)
	}

	closure, err := BuildBoundaryRootClosure(reg, keys, state, BoundaryRoots{{Path: root, Value: outer}})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []keyspace.Key{root, alias, child} {
		if !closure.ContainsPath(path) {
			got := make([]string, 0, len(closure.paths))
			for retained := range closure.paths {
				got = append(got, string(keys.FormatReadOnly(retained)))
			}
			t.Fatalf("closure omitted reachable path %q; retained=%v", keys.FormatReadOnly(path), got)
		}
	}
	if !closure.ContainsHeapSuffix(outerID, member) {
		t.Fatal("closure omitted owner-qualified heap member suffix")
	}
	if closure.ContainsPath(member) {
		t.Fatal("closure mixed a rootless heap suffix into absolute paths")
	}
	for _, id := range []identity.ID{outerID, innerID} {
		if !closure.ContainsIdentity(id) {
			t.Fatalf("closure omitted reachable identity %v", id)
		}
	}
}

func TestBoundaryClosureVisitsEveryFiniteHeapObjectFact(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	rootPath := keys.FromPath(pathdom.Path{Symbol: symbol.ID(51), Version: 1})
	staticSuffix, ok := heapidentity.StaticMemberSuffixKey(keys, []segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	if !ok {
		t.Fatal("static member suffix")
	}
	dynamicSuffix, ok := heapidentity.StaticMemberSuffixKey(keys, []segment.Segment{{Kind: segment.SegmentField, Name: "indexed"}})
	if !ok {
		t.Fatal("dynamic table suffix")
	}
	outerID := identity.ID{Kind: "lua.table", Site: "outer", Index: 1}
	staticID := identity.ID{Kind: "lua.table", Site: "static", Index: 2}
	keyID := identity.ID{Kind: "lua.table", Site: "key", Index: 3}
	valueID := identity.ID{Kind: "lua.table", Site: "value", Index: 4}
	outer := identityvalue.Present(reg, outerID)
	staticValue := identityvalue.Present(reg, staticID)
	keyValue := identityvalue.Present(reg, keyID)
	dynamicValue := identityvalue.Present(reg, valueID)
	factKey := dynamicindex.Key{Table: dynamicSuffix, Site: "closure"}
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          outer,
		StaticMembers: map[keyspace.Key]product.Value{staticSuffix: staticValue},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			factKey: {KeyValue: keyValue, Value: dynamicValue, Admission: dynamicindex.AdmissionAdmitted},
		},
	})
	world := Domain(reg).Bottom().WriteHeapTableObject(reg, outerID, object)

	closure, err := BuildBoundaryRootClosure(reg, keys, world, BoundaryRoots{{Path: rootPath, Value: outer}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []identity.ID{outerID, staticID, keyID, valueID} {
		if !closure.ContainsIdentity(id) {
			t.Fatalf("closure omitted finite heap fact identity %v", id)
		}
	}
	for _, suffix := range []keyspace.Key{staticSuffix, dynamicSuffix} {
		if !closure.ContainsHeapSuffix(outerID, suffix) {
			t.Fatalf("closure omitted owner-qualified heap suffix %q", keys.FormatReadOnly(suffix))
		}
	}
}

func TestRebaseBoundaryPathsClonesAliasedRootSubstitutionDeterministically(t *testing.T) {
	from, to := keyspace.New(), keyspace.New()
	formalPath := pathdom.Path{Symbol: 70, Version: 1}
	formal := from.FromPath(formalPath)
	leaf := from.FromPath(formalPath.Field("leaf"))
	first := to.FromPath(pathdom.Path{Symbol: 71, Version: 1})
	second := to.FromPath(pathdom.Path{Symbol: 72, Version: 1})
	bindings := boundaryPathMap{
		{from: formal, to: first},
		{from: formal, to: second},
	}
	for _, roots := range []boundaryPathMap{bindings, {bindings[1], bindings[0]}} {
		got, ok := rebaseBoundaryPaths(from, to, roots, leaf)
		if !ok || len(got) != 2 || !to.Less(got[0], got[1]) {
			t.Fatalf("aliased substitution = %#v/%v, want two canonical outputs", got, ok)
		}
	}
}

func TestBoundaryRootClosureFollowsRelationalAndStoreOperands(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	firstPath := pathdom.Path{Symbol: 81, Version: 1}
	secondPath := pathdom.Path{Symbol: 82, Version: 1}
	thirdPath := pathdom.Path{Symbol: 83, Version: 1}
	first := keys.FromPath(firstPath)
	second := keys.FromPath(secondPath)
	third := keys.FromPath(thirdPath)
	firstState := mustBoundaryAddress(t, keys.FormatReadOnly(first))
	secondState := mustBoundaryAddress(t, keys.FormatReadOnly(second))
	thirdState := mustBoundaryAddress(t, keys.FormatReadOnly(third))
	world := Domain(reg).Bottom().
		WriteDiffConstraint(RelValueOperand(firstState), RelValueOperand(secondState), 0).
		AddStoreRelation(StoreRelation{Source: secondState, Into: thirdState})
	closure, err := BuildBoundaryRootClosure(reg, keys, world, BoundaryRoots{{Path: first}})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []keyspace.Key{first, second, third} {
		if !closure.ContainsPath(path) {
			t.Fatalf("closure omitted connected lane operand %q", keys.FormatReadOnly(path))
		}
	}
}

func TestRebaseBoundaryPathUsesLongestRootAndPreservesAliasTargets(t *testing.T) {
	from, to := keyspace.New(), keyspace.New()
	formal := pathdom.Path{Symbol: symbol.ID(51), Version: 1}
	outer := from.FromPath(formal)
	inner := from.FromPath(formal.Field("inner"))
	leaf := from.FromPath(formal.Field("inner").Field("leaf"))
	actualPath := pathdom.Path{Symbol: symbol.ID(61), Version: 1}
	aliasPath := pathdom.Path{Symbol: symbol.ID(62), Version: 1}
	actual := to.FromPath(actualPath)
	alias := to.FromPath(aliasPath)

	rebased, ok := rebaseBoundaryPaths(from, to, boundaryPathMap{
		{from: outer, to: actual},
		{from: inner, to: alias},
	}, leaf)
	if !ok {
		t.Fatal("root substitution failed")
	}
	want := to.FromPath(aliasPath.Field("leaf"))
	if len(rebased) != 1 || rebased[0] != want {
		t.Fatalf("rebased path = %#v, want %q", rebased, to.FormatReadOnly(want))
	}
}

func TestRebaseBoundaryIdentityFailsClosedWithoutAtomicMapping(t *testing.T) {
	caller := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("boundary-identity")))
	callee := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("boundary-identity")), 1)
	template := identity.ManifestAllocationTemplate(callee, 1, 1)
	lens, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(callee, caller, 9, 0), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	actual := identity.BoundaryAllocation(template, caller, 9, 0)
	if got, ok := lens.RebaseAllocation(template); !ok || got != actual {
		t.Fatalf("mapped identity = %v/%v, want %v/true", got, ok, actual)
	}
	if got, ok := (*BoundaryAllocationAuthority)(nil).RebaseAllocation(template); ok || got != (identity.ID{}) {
		t.Fatalf("unmapped identity = %v/%v, want zero/false", got, ok)
	}
	stable := identity.ID{Kind: "lua.table", Site: "already-instantiated", Index: 4}
	if got, err := NewIdentitySubstitutionAuthority(identity.Substitution{}, lens).Image(identity.ConcreteTerm(stable)); err != nil {
		t.Fatal(err)
	} else if concrete, ok := got.ID(); !ok || concrete != stable {
		t.Fatalf("stable identity = %v/%v, want self/true", concrete, ok)
	}
}

func TestBoundaryAllocationAuthoritySeparatesRootAndApplyRoutes(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("boundary-route-authority"))
	target, caller := lexicalidentity.FunctionBody(namespace, 1), lexicalidentity.RootBody(namespace)
	template := identity.ManifestAllocationTemplate(target, 1, 1)
	root, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(target), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	apply, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(target, caller, 9, 0), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	rootID, rootOK := root.RebaseAllocation(template)
	applyID, applyOK := apply.RebaseAllocation(template)
	if !rootOK || !applyOK || rootID == applyID || !identity.IsRootBoundaryAllocation(rootID) || !identity.IsBoundaryAllocation(applyID) {
		t.Fatalf("root/apply identities = %#v/%v %#v/%v", rootID, rootOK, applyID, applyOK)
	}
	foreign := identity.ManifestAllocationTemplate(lexicalidentity.FunctionBody(namespace, 2), 1, 1)
	if authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(target), []identity.AllocationTemplate{foreign}); err == nil || authority != nil {
		t.Fatal("root route accepted a template owned by another lexical body")
	}
}

func mustBoundaryAddress(t *testing.T, path pathdom.PathKey) pathaddr.StateKey {
	t.Helper()
	key, ok := pathaddr.StateKeyFromPathKey(path)
	if !ok {
		t.Fatalf("StateKeyFromPathKey(%q)", path)
	}
	return key
}

package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

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

func TestRebaseBoundaryPathRejectsAmbiguousRootSubstitution(t *testing.T) {
	from, to := keyspace.New(), keyspace.New()
	formalPath := pathdom.Path{Symbol: 70, Version: 1}
	formal := from.FromPath(formalPath)
	leaf := from.FromPath(formalPath.Field("leaf"))
	first := to.FromPath(pathdom.Path{Symbol: 71, Version: 1})
	second := to.FromPath(pathdom.Path{Symbol: 72, Version: 1})
	bindings := BoundaryRootMap{
		{From: formal, To: first},
		{From: formal, To: second},
	}
	for _, roots := range []BoundaryRootMap{bindings, BoundaryRootMap{bindings[1], bindings[0]}} {
		if got, ok := RebaseBoundaryPath(from, to, roots, leaf); ok || got != (keyspace.Key{}) {
			t.Fatalf("ambiguous substitution = %#v/%v, want zero/false", got, ok)
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

	rebased, ok := RebaseBoundaryPath(from, to, BoundaryRootMap{
		{From: outer, To: actual},
		{From: inner, To: alias},
	}, leaf)
	if !ok {
		t.Fatal("root substitution failed")
	}
	want := to.FromPath(aliasPath.Field("leaf"))
	if rebased != want {
		t.Fatalf("rebased path = %q, want %q", to.FormatReadOnly(rebased), to.FormatReadOnly(want))
	}
}

func TestRebaseBoundaryIdentityFailsClosedWithoutAtomicMapping(t *testing.T) {
	template := identity.ID{Kind: "lua.table", Site: "template", Index: 1}
	actual := identity.ID{Kind: "lua.table", Site: "caller", Index: 9}
	if got, ok := RebaseBoundaryIdentity(BoundaryAllocationMap{template: actual}, template); !ok || got != actual {
		t.Fatalf("mapped identity = %v/%v, want %v/true", got, ok, actual)
	}
	if got, ok := RebaseBoundaryIdentity(nil, template); ok || got != (identity.ID{}) {
		t.Fatalf("unmapped identity = %v/%v, want zero/false", got, ok)
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

package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestBoundaryVersionBridgeMutationClearsStructuralDynamicValueTheorem(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	fromContainer := from.FromPath(pathdom.Path{Symbol: 9001})
	toContainer := to.FromPath(pathdom.Path{Symbol: 9101})
	toVisible := to.FromPath(pathdom.Path{Symbol: 9101, Version: 3})
	toTable := to.FromPath(pathdom.Path{Symbol: 9102})
	site := dynamicindex.Site("captured-overwrite")

	source := Domain(reg).Bottom().WriteEffectDelta(effectdelta.Key{
		Target: fromContainer,
		Site:   effectdelta.Site("captured-overwrite"),
		Kind:   effectdelta.Mutation,
	}, effectdelta.Top())
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Path: fromContainer}})
	if err != nil {
		t.Fatal(err)
	}
	if !artifact.closure.ContainsPath(fromContainer) {
		t.Fatal("mutation did not demand its certified structural companion")
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: toVisible},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	destination := Reachable(Domain(reg).Bottom()).AddDynamicIndexValueKeyMembership(
		toContainer, site, boundaryVersionStateKey(t, to, toTable),
	)
	applied, err := ApplyBoundary(reg, to, destination, rebased)
	if err != nil {
		t.Fatal(err)
	}
	if tables := applied.DynamicIndexValueKeyMembershipTables(toContainer, site); len(tables) != 0 {
		t.Fatalf("stale dynamic-value theorem survived mutation: %#v", tables)
	}
}

func TestBoundaryVersionBridgeRebasesDynamicMembershipBothEndpoints(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	fromTable := from.FromPath(pathdom.Path{Symbol: 8401})
	fromContainer := from.FromPath(pathdom.Path{Symbol: 8402, Version: 1})
	toTable := to.FromPath(pathdom.Path{Symbol: 8501, Version: 4})
	toContainer := to.FromPath(pathdom.Path{Symbol: 8502, Version: 2})
	unrelated := to.FromPath(pathdom.Path{Symbol: 8503, Version: 4})
	site := dynamicindex.Site("versioned-membership")

	source := Domain(reg).Bottom().AddDynamicIndexValueKeyMembership(
		fromContainer, site, boundaryVersionStateKey(t, from, fromTable),
	)
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
		{Path: fromTable},
		{Path: fromContainer},
	})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: toTable},
		{FromRoot: 1, ToRoot: 1, To: toContainer},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyBoundary(reg, to, Reachable(Domain(reg).Bottom()), rebased)
	if err != nil {
		t.Fatal(err)
	}
	tables := applied.DynamicIndexValueKeyMembershipTables(toContainer, site)
	want := boundaryVersionStateKey(t, to, toTable)
	if len(tables) != 1 || tables[0] != want {
		t.Fatalf("rebased dynamic membership tables = %#v, want [%q]", tables, want)
	}
	if wrong := boundaryVersionStateKey(t, to, unrelated); applied.HasPathKeyMembership(wrong, want) {
		t.Fatalf("unrelated versioned symbol %q acquired membership in %q", wrong, want)
	}
}

func TestBoundaryVersionBridgeRebasesClosedDynamicAllFromCertifiedResolverRoots(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	fromContainer := from.FromPath(pathdom.Path{Symbol: 8601})
	fromTable := from.FromPath(pathdom.Path{Symbol: 8602})
	fromVisibleContainer := from.FromPath(pathdom.Path{Symbol: 8601, Version: 1})
	fromVisibleTable := from.FromPath(pathdom.Path{Symbol: 8602, Version: 1})
	fromSibling := from.FromPath(pathdom.Path{Symbol: 8601, Version: 2}.Field("member"))
	toContainer := to.FromPath(pathdom.Path{Symbol: 8701})
	toTable := to.FromPath(pathdom.Path{Symbol: 8702})

	source := Domain(reg).Bottom().
		AddDynamicIndexAllValuesKeyMembership(fromContainer, boundaryVersionStateKey(t, from, fromTable)).
		WriteNumFloor(from, boundaryVersionStateKey(t, from, fromSibling), 99)
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
		{Path: fromVisibleContainer},
		{Path: fromVisibleTable},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !artifact.closure.ContainsPath(fromContainer) || !artifact.closure.ContainsPath(fromTable) {
		t.Fatal("closed dynamic theorem did not demand its structural companions")
	}
	if artifact.closure.ContainsPath(fromSibling) {
		t.Fatal("structural companion captured an uncertified sibling SSA version")
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: toContainer},
		{FromRoot: 1, ToRoot: 1, To: toTable},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyBoundary(reg, to, Reachable(Domain(reg).Bottom()), rebased)
	if err != nil {
		t.Fatal(err)
	}
	tables := applied.DynamicIndexAllValuesKeyMembershipTables(toContainer)
	want := boundaryVersionStateKey(t, to, toTable)
	if len(tables) != 1 || tables[0] != want {
		t.Fatalf("closed dynamic theorem = %#v, want [%q]", tables, want)
	}
}

func TestBoundaryVersionBridgeResolverOutputPreservesDestinationStructuralTheorem(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	fromContainer := from.FromPath(pathdom.Path{Symbol: 8801})
	fromTable := from.FromPath(pathdom.Path{Symbol: 8802})
	toContainer := to.FromPath(pathdom.Path{Symbol: 8901})
	toTable := to.FromPath(pathdom.Path{Symbol: 8902})
	toVisibleContainer := to.FromPath(pathdom.Path{Symbol: 8901, Version: 3})
	toVisibleTable := to.FromPath(pathdom.Path{Symbol: 8902, Version: 2})

	source := Domain(reg).Bottom().
		AddDynamicIndexAllValuesKeyMembership(fromContainer, boundaryVersionStateKey(t, from, fromTable))
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
		{Path: fromContainer},
		{Path: fromTable},
	})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: toVisibleContainer},
		{FromRoot: 1, ToRoot: 1, To: toVisibleTable},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	destination := Reachable(Domain(reg).Bottom()).
		AddDynamicIndexAllValuesKeyMembership(toContainer, boundaryVersionStateKey(t, to, toTable))
	applied, err := ApplyBoundary(reg, to, destination, rebased)
	if err != nil {
		t.Fatal(err)
	}
	want := boundaryVersionStateKey(t, to, toTable)
	if tables := applied.DynamicIndexAllValuesKeyMembershipTables(toContainer); len(tables) != 1 || tables[0] != want {
		t.Fatalf("closed dynamic output theorem = %#v, want [%q]", tables, want)
	}
	if tables := applied.DynamicIndexAllValuesKeyMembershipTables(toVisibleContainer); len(tables) != 0 {
		t.Fatalf("closed dynamic theorem leaked onto resolver representation: %#v", tables)
	}
}

func boundaryVersionStateKey(t *testing.T, keys *keyspace.KeySpace, value keyspace.Key) pathaddr.StateKey {
	t.Helper()
	out, ok := pathaddr.StateKeyFromPathKey(keys.FormatReadOnly(value))
	if !ok {
		t.Fatalf("state key for %q", keys.FormatReadOnly(value))
	}
	return out
}

// One lexical boundary root has two deliberately distinct State addresses:
// the version-insensitive structural root used for root facts and the exact
// resolver-selected SSA root used by point-local descendant facts.  The frame
// must seal both wires.  Equating every SSA version with the structural root
// would turn a strong update into an unsound wildcard update.
func TestBoundaryVersionBridgeTransportsOnlyCertifiedSSADescendants(t *testing.T) {
	reg := standard.Registry()
	calleeKeys, callerKeys := keyspace.New(), keyspace.New()
	calleeSymbol, callerSymbol := symbol.ID(8101), symbol.ID(8201)

	calleeStructural := calleeKeys.FromPath(pathdom.Path{Symbol: calleeSymbol})
	calleeVisible := calleeKeys.FromPath(pathdom.Path{Symbol: calleeSymbol, Version: 1})
	calleeMember := calleeKeys.FromPath(pathdom.Path{Symbol: calleeSymbol, Version: 1}.Field("member"))
	calleeWrongVersion := calleeKeys.FromPath(pathdom.Path{Symbol: calleeSymbol, Version: 2}.Field("member"))

	callerStructural := callerKeys.FromPath(pathdom.Path{Symbol: callerSymbol})
	callerVisible := callerKeys.FromPath(pathdom.Path{Symbol: callerSymbol, Version: 4})
	callerMember := callerKeys.FromPath(pathdom.Path{Symbol: callerSymbol, Version: 4}.Field("member"))
	callerSiblingVersion := callerKeys.FromPath(pathdom.Path{Symbol: callerSymbol, Version: 5}.Field("member"))

	source := Domain(reg).Bottom().
		WriteNumFloor(calleeKeys, boundaryVersionStateKey(t, calleeKeys, calleeStructural), 3).
		WriteNumFloor(calleeKeys, boundaryVersionStateKey(t, calleeKeys, calleeMember), 7).
		WriteNumFloor(calleeKeys, boundaryVersionStateKey(t, calleeKeys, calleeWrongVersion), 99)

	// The structural spelling does not confer authority over any resolver
	// version. The exact visible root must be present as a separate frame wire.
	structuralOnly, err := BuildBoundaryRootClosure(reg, calleeKeys, source, BoundaryRoots{{Path: calleeStructural}})
	if err != nil {
		t.Fatal(err)
	}
	if structuralOnly.ContainsPath(calleeMember) || structuralOnly.ContainsPath(calleeWrongVersion) {
		t.Fatal("structural boundary root captured an uncertified SSA version")
	}

	artifact, err := ProjectBoundary(reg, calleeKeys, source, BoundaryRoots{
		{Path: calleeStructural},
		{Path: calleeVisible},
	})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, callerKeys, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: callerStructural},
		{FromRoot: 1, ToRoot: 1, To: callerVisible},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	destination := Domain(reg).Bottom().
		WriteNumFloor(callerKeys, boundaryVersionStateKey(t, callerKeys, callerStructural), 1).
		WriteNumFloor(callerKeys, boundaryVersionStateKey(t, callerKeys, callerMember), 2).
		WriteNumFloor(callerKeys, boundaryVersionStateKey(t, callerKeys, callerSiblingVersion), 11)
	applied, err := ApplyBoundary(reg, callerKeys, destination, rebased)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := applied.ReadNumFloor(callerKeys, boundaryVersionStateKey(t, callerKeys, callerStructural)); !ok || got != 3 {
		t.Fatalf("structural root = %d/%t, want 3/true", got, ok)
	}
	if got, ok := applied.ReadNumFloor(callerKeys, boundaryVersionStateKey(t, callerKeys, callerMember)); !ok || got != 7 {
		t.Fatalf("certified SSA descendant = %d/%t, want 7/true", got, ok)
	}
	if got, ok := applied.ReadNumFloor(callerKeys, boundaryVersionStateKey(t, callerKeys, callerSiblingVersion)); !ok || got != 11 {
		t.Fatalf("sibling SSA version = %d/%t, want 11/true", got, ok)
	}
}

func TestBoundaryVersionBridgeRebasesSameSymbolAcrossKeySpaceOwnership(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	sym := symbol.ID(8301)

	fromStructural := from.FromPath(pathdom.Path{Symbol: sym})
	fromVisible := from.FromPath(pathdom.Path{Symbol: sym, Version: 1})
	fromMember := from.FromPath(pathdom.Path{Symbol: sym, Version: 1}.Field("member"))
	fromWrongVersion := from.FromPath(pathdom.Path{Symbol: sym, Version: 2}.Field("member"))
	toStructural := to.FromPath(pathdom.Path{Symbol: sym})
	toVisible := to.FromPath(pathdom.Path{Symbol: sym, Version: 1})
	toMember := to.FromPath(pathdom.Path{Symbol: sym, Version: 1}.Field("member"))
	toWrongVersion := to.FromPath(pathdom.Path{Symbol: sym, Version: 2}.Field("member"))

	source := Domain(reg).Bottom().
		WriteNumFloor(from, boundaryVersionStateKey(t, from, fromMember), 7).
		WriteNumFloor(from, boundaryVersionStateKey(t, from, fromWrongVersion), 99)
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
		{Path: fromStructural},
		{Path: fromVisible},
	})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: toStructural},
		{FromRoot: 1, ToRoot: 1, To: toVisible},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	destination := Domain(reg).Bottom().
		WriteNumFloor(to, boundaryVersionStateKey(t, to, toStructural), 1).
		WriteNumFloor(to, boundaryVersionStateKey(t, to, toWrongVersion), 11)
	applied, err := ApplyBoundary(reg, to, destination, rebased)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := applied.ReadNumFloor(to, boundaryVersionStateKey(t, to, toMember)); !ok || got != 7 {
		t.Fatalf("same-symbol certified descendant = %d/%t, want 7/true", got, ok)
	}
	if got, ok := applied.ReadNumFloor(to, boundaryVersionStateKey(t, to, toWrongVersion)); !ok || got != 11 {
		t.Fatalf("same-symbol sibling SSA version = %d/%t, want preserved 11/true", got, ok)
	}
}

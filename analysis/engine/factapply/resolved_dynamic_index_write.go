package factapply

import (
	"math"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ResolvedDynamicIndexWrite is the closed input to canonical dynamic-index
// publication. All resolver/Facts/source-provider decisions are made before
// this value exists; Apply only mutates State from these exact verdicts.
type ResolvedDynamicIndexWrite struct {
	data *resolvedDynamicIndexWriteData
}

type resolvedDynamicIndexWriteData struct {
	Table               ResolvedPathAddress
	Key                 dynamicindex.Key
	Fact                dynamicindex.Fact
	TableStateKeys      []pathaddr.StateKey
	AllValueTables      []pathaddr.StateKey
	PendingRestores     []state.PendingDynamicAllValueRestore
	KeyStateKey         pathaddr.StateKey
	HasKeyStateKey      bool
	SourceMemberships   []pathaddr.StateKey
	EqualitySource      ResolvedPathAddress
	EqualityTarget      ResolvedPathAddress
	HasEquality         bool
	StaticTarget        ResolvedPathAddress
	HasStaticTarget     bool
	AppendStateKey      pathaddr.StateKey
	AppendFloor         int64
	HasAppend           bool
	TableID             identity.ID
	TableOwnerPlacement placement.Value
	HasTableID          bool
	TableSymbol         symbol.ID
	DefinitelyPresent   bool
	DefinitelyAbsent    bool
	MayBeAbsent         bool
}

// ApplyResolvedDynamicIndexWrite performs the authoritative lane updates in
// their historical order. Invalid or foreign requests publish nothing.
func ApplyResolvedDynamicIndexWrite(reg *axis.Registry, ks *keyspace.KeySpace, out state.State, request ResolvedDynamicIndexWrite) (state.State, bool) {
	resolved := request.data
	if reg == nil || resolved == nil || !resolved.Table.belongsTo(ks) || resolved.Key.Table != resolved.Table.rootOrVisibleLocal ||
		(resolved.HasEquality && (!resolved.EqualitySource.belongsTo(ks) || !resolved.EqualityTarget.belongsTo(ks))) ||
		(resolved.HasStaticTarget && !resolved.StaticTarget.belongsTo(ks)) {
		return out, false
	}

	allValueTables := append([]pathaddr.StateKey(nil), resolved.AllValueTables...)
	out = out.ClearDynamicIndexValueKeyMembershipsForContainer(resolved.Key.Table)
	if resolved.MayBeAbsent {
		for _, tableKey := range resolved.TableStateKeys {
			out = out.ClearKeyMembershipsForPath(tableKey)
		}
		if resolved.TableSymbol != 0 {
			out = out.ClearKeyMembershipsForTableSymbol(ks, resolved.TableSymbol)
		}
	}
	for _, restore := range resolved.PendingRestores {
		out = out.AddPendingDynamicAllValueRestore(restore.Container, restore.Table, restore.Key)
	}
	out = out.WriteDynamicIndexFact(reg, resolved.Key, resolved.Fact)
	if resolved.DefinitelyPresent && resolved.HasKeyStateKey {
		out = out.AddPathKeyMembership(resolved.KeyStateKey, resolved.Table.rootOrVisible)
	}
	if resolved.DefinitelyPresent {
		for _, table := range resolved.SourceMemberships {
			out = out.AddDynamicIndexValueKeyMembership(resolved.Key.Table, resolved.Key.Site, table)
		}
	}
	if resolved.DefinitelyAbsent {
		for _, table := range allValueTables {
			out = out.AddDynamicIndexAllValuesKeyMembership(resolved.Key.Table, table)
		}
	} else {
		for _, table := range allValueTables {
			if stateKeyIn(resolved.SourceMemberships, table) {
				out = out.AddDynamicIndexAllValuesKeyMembership(resolved.Key.Table, table)
			}
		}
	}
	if resolved.DefinitelyAbsent && resolved.HasKeyStateKey {
		keys := append([]pathaddr.StateKey{resolved.KeyStateKey}, out.EquivalentStateKeys(ks, resolved.KeyStateKey)...)
		for _, key := range keys {
			for _, restore := range out.PendingDynamicAllValueRestores(resolved.Key.Table, key) {
				out = out.AddDynamicIndexAllValuesKeyMembership(restore.Container, restore.Table)
				out = out.ClearPendingDynamicAllValueRestore(restore)
			}
		}
	}
	if resolved.HasEquality && resolved.EqualityTarget.stateKey != resolved.EqualitySource.stateKey {
		out = out.AddBranchProof(pathevidence.BranchProof{
			Kind: pathevidence.BranchProofPathEqual, Path: resolved.EqualityTarget.local, Other: resolved.EqualitySource.local,
		}).CanonicalizeTypestateResources(ks)
	}
	if resolved.HasStaticTarget {
		edit := out.Edit(reg)
		edit.WriteLocalPathStaticMember(resolved.StaticTarget.local, resolved.Fact.Value)
		if canonical, ok := ks.FieldCanonical(resolved.StaticTarget.local); ok {
			edit.WriteLocalPathStaticMember(canonical, resolved.Fact.Value)
		}
		out = edit.Done()
	}
	if resolved.HasTableID {
		switch resolved.TableOwnerPlacement {
		case placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
			out = markReachableHeapValuePlacement(reg, out, resolved.Fact.Value, resolved.TableOwnerPlacement, map[identity.ID]struct{}{})
		}
	}
	if resolved.HasAppend && resolved.AppendFloor < math.MaxInt64 {
		out = out.WriteLenFloor(ks, resolved.AppendStateKey, resolved.AppendFloor)
	}
	if resolved.HasTableID {
		object := out.ReadHeapTableObject(reg, resolved.TableID)
		if !heapidentity.ObjectDomain(reg).Equal(object, heapidentity.BottomObject(reg)) {
			dynamic := object.DynamicIndexFacts()
			if dynamic == nil {
				dynamic = make(map[dynamicindex.Key]dynamicindex.Fact, 1)
			}
			if existing, ok := dynamic[resolved.Key]; ok {
				dynamic[resolved.Key] = dynamicindex.Domain(reg).Join(existing, resolved.Fact)
			} else {
				dynamic[resolved.Key] = resolved.Fact
			}
			out = out.WriteHeapTableObject(reg, resolved.TableID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: object.Root(), StaticMembers: object.StaticMembers(), DynamicIndexFacts: dynamic,
			}))
		}
	}
	return out, true
}

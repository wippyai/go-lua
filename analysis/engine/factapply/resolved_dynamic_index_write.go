package factapply

import (
	"context"
	"math"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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
	RestoreKeys         []pathaddr.StateKey
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
	Direct              bool
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

	if resolved.Direct {
		domain := state.RegisteredProductDomain(reg)
		plan, err := domain.PrepareDynamicIndexMembershipFactorPlan(ks, state.DynamicIndexMembershipFactorConfig{
			Key: resolved.Key, Fact: resolved.Fact,
			TableStateKeys: resolved.TableStateKeys, AllValueTables: resolved.AllValueTables,
			PendingRestores: resolved.PendingRestores, RestoreKeys: resolved.RestoreKeys,
			KeyStateKey: resolved.KeyStateKey, MembershipTable: resolved.Table.rootOrVisible,
			SourceMemberships: resolved.SourceMemberships, TableSymbol: resolved.TableSymbol,
			HasKeyStateKey: resolved.HasKeyStateKey, DefinitelyPresent: resolved.DefinitelyPresent,
			DefinitelyAbsent: resolved.DefinitelyAbsent, MayBeAbsent: resolved.MayBeAbsent,
		})
		if err != nil {
			return out, false
		}
		written, err := domain.ApplyDynamicIndexMembership(plan, out)
		if err != nil {
			return out, false
		}
		out = written
	}
	if resolved.HasEquality && resolved.EqualityTarget.stateKey != resolved.EqualitySource.stateKey {
		proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: resolved.EqualityTarget.local, Other: resolved.EqualitySource.local}
		domain := state.RegisteredProductDomain(reg)
		written, err := domain.ApplyPathEqualityProof(ks, proof, out)
		if err != nil {
			return out, false
		}
		out = written
	}
	if resolved.HasStaticTarget {
		domain := state.RegisteredProductDomain(reg)
		plan, err := domain.PrepareStaticMemberFactorPlan(ks, resolved.StaticTarget.local, resolved.Fact.Value)
		if err != nil {
			return out, false
		}
		written, err := domain.ApplyStaticMember(plan, out)
		if err != nil {
			return out, false
		}
		out = written
	}
	if resolved.HasTableID && resolved.Direct {
		switch resolved.TableOwnerPlacement {
		case placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
			domain := state.RegisteredProductDomain(reg)
			plan, err := domain.PreparePlacementReachabilityPlan(ks, []product.Value{resolved.Fact.Value}, resolved.TableOwnerPlacement)
			if err != nil {
				return out, false
			}
			written, err := domain.ApplyPlacementReachability(context.Background(), plan, out)
			if err != nil {
				return out, false
			}
			out = written
		}
	}
	if resolved.HasAppend && resolved.AppendFloor < math.MaxInt64 {
		path, interned := ks.InternStateKey(resolved.AppendStateKey)
		if !interned {
			return out, false
		}
		domain := state.RegisteredProductDomain(reg)
		plan, err := domain.PrepareLengthFloorFactorPlan(ks, path, resolved.AppendFloor)
		if err != nil {
			return out, false
		}
		written, err := domain.ApplyLengthFloor(plan, out)
		if err != nil {
			return out, false
		}
		out = written
	}
	if resolved.HasTableID {
		object, observed := observeIndexMutationHeapObject(reg, out, identity.ConcreteTerm(resolved.TableID))
		if !observed {
			return out, false
		}
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
			replacement := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: object.Root(), StaticMembers: object.StaticMembers(), DynamicIndexFacts: dynamic,
			})
			domain := state.RegisteredProductDomain(reg)
			plan, err := domain.PrepareObjectGraphReplacePlan(ks, []state.ObjectGraphMutation{{Identity: identity.ConcreteTerm(resolved.TableID), Object: replacement}})
			if err != nil {
				return out, false
			}
			written, err := domain.ApplyObjectGraphMutation(plan, out)
			if err != nil {
				return out, false
			}
			out = written
		}
	}
	return out, true
}

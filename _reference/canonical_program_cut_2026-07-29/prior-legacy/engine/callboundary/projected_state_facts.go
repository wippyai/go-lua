package callboundary

import (
	"fmt"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

type projectedStateFactOwnership uint8

const (
	projectedStateFactInvalid projectedStateFactOwnership = iota
	projectedStateFactState
	projectedStateFactMixed
	projectedStateFactSyntax
)

type projectedStateFactHandler struct {
	ownership projectedStateFactOwnership
	sources   []state.LaneID
	project   func(projectedStateFactContext, *NormalReturnFacts)
}

// Every NormalReturnFacts descriptor is classified here. Mixed lanes receive
// their State-owned evidence here and are supplemented by syntax/read-model
// projection at the caller; syntax lanes deliberately have no State handler.
var projectedStateFactHandlers = BindNormalReturnFactLanes("projected State normal-return", map[NormalReturnFactLaneID]projectedStateFactHandler{
	LanePathRefinements:          {projectedStateFactState, []state.LaneID{state.LanePathEvidence}, projectStatePathRefinements},
	LanePersistentPathWrites:     {projectedStateFactSyntax, nil, nil},
	LanePathStaticMembers:        {projectedStateFactState, []state.LaneID{state.LanePathEvidence}, projectStatePathStaticMembers},
	LanePathStaticMemberDeltas:   {projectedStateFactSyntax, nil, nil},
	LanePathPresenceImplications: {projectedStateFactState, []state.LaneID{state.LanePathEvidence}, projectStatePathPresenceImplications},
	LanePathInvalidations:        {projectedStateFactMixed, []state.LaneID{state.LaneDynamicIndex, state.LaneEffectDeltas}, projectStatePathInvalidations},
	LaneDynamicIndexFacts:        {projectedStateFactMixed, []state.LaneID{state.LaneDynamicIndex}, projectStateDynamicIndexFacts},
	LaneKeyMemberships:           {projectedStateFactState, []state.LaneID{state.LaneKeyMemberships}, projectStateKeyMemberships},
	LaneDynamicValueKeys:         {projectedStateFactMixed, []state.LaneID{state.LaneKeyMemberships}, projectStateDynamicValueKeys},
	LaneDynamicAllValues:         {projectedStateFactState, []state.LaneID{state.LaneKeyMemberships}, projectStateDynamicAllValues},
	LaneBranchProofs:             {projectedStateFactState, []state.LaneID{state.LanePathEvidence}, projectStateBranchProofs},
	LaneChannelSelects:           {projectedStateFactState, []state.LaneID{state.LaneChannelSelect}, projectStateChannelSelects},
	LaneFrozenTables:             {projectedStateFactState, []state.LaneID{state.LaneFrozenTables, state.LaneEffectDeltas, state.LanePathEvidence, state.LaneHeapTableIdentity}, projectStateFrozenTables},
	LaneEffectDeltas:             {projectedStateFactState, []state.LaneID{state.LaneEffectDeltas}, projectStateEffectDeltas},
	LaneEscapeEvents:             {projectedStateFactState, []state.LaneID{state.LaneEscapeEvents}, projectStateEscapeEvents},
	LaneStoreRelations:           {projectedStateFactMixed, []state.LaneID{state.LaneStoreRelations}, projectStateStoreRelations},
	LaneLifecycleFacts:           {projectedStateFactSyntax, nil, nil},
	LaneNumFloors:                {projectedStateFactState, []state.LaneID{state.LaneNumFloors}, projectStateNumFloors},
	LaneNumCeils:                 {projectedStateFactState, []state.LaneID{state.LaneNumCeils}, projectStateNumCeils},
	LaneRelConstraints:           {projectedStateFactState, []state.LaneID{state.LaneDiffRelations}, projectStateRelConstraints},
}, func(handler projectedStateFactHandler) bool {
	return handler.ownership >= projectedStateFactState && handler.ownership <= projectedStateFactSyntax &&
		(handler.ownership == projectedStateFactSyntax) == (handler.project == nil) &&
		(handler.project != nil || len(handler.sources) == 0)
})

// NormalReturnFactSourceLanes returns the descriptor-declared, first-use
// ordered source union for the State-owned normal-return projection. It is the
// sole dependency inventory used to prepare a sparse BoundaryFactorView.
func NormalReturnFactSourceLanes() []state.LaneID {
	seen := make(map[state.LaneID]struct{})
	var lanes []state.LaneID
	for _, binding := range projectedStateFactHandlers {
		for _, lane := range binding.Value.sources {
			if _, duplicate := seen[lane]; duplicate {
				continue
			}
			seen[lane] = struct{}{}
			lanes = append(lanes, lane)
		}
	}
	return lanes
}

// NormalReturnFactSource is the read-only semantic observation surface shared
// by State and BoundaryFactorView. Emitters are defined exactly once below;
// alternate carriers supply lane observations rather than alternate semantics.
type NormalReturnFactSource interface {
	ForEachPathRefinement(func(keyspace.Key, product.Value) bool)
	ForEachPathStaticMember(func(keyspace.Key, product.Value) bool)
	PathPresenceImplicationsSnapshot(*keyspace.KeySpace) pathevidence.PathPresenceImplicationsSnapshot
	DynamicIndexFactsSnapshot() state.DynamicIndexFactsSnapshot
	KeyMembershipsSnapshot() state.KeyMembershipsSnapshot
	BranchProofsSnapshot(*keyspace.KeySpace) pathevidence.BranchProofsSnapshot
	ChannelSelectFactsSnapshot() state.ChannelSelectFactsSnapshot
	FrozenTablesSnapshot() state.FrozenTablesSnapshot
	EffectDeltasSnapshot() state.EffectDeltasSnapshot
	EscapeEventsSnapshot() state.EscapeEventsSnapshot
	StoreRelationsSnapshot() state.StoreRelationsSnapshot
	NumFloorsSnapshot(*keyspace.KeySpace) state.NumFloorsSnapshot
	NumCeilsSnapshot(*keyspace.KeySpace) state.NumCeilsSnapshot
	RelConstraints() state.RelConstraintsSnapshot
	ReadLocalPathKey(*axis.Registry, keyspace.Key) product.Value
	HeapTableObjectsSnapshot() state.HeapTableObjectsSnapshot
}

type projectedStateFactContext struct {
	reg       *axis.Registry
	keys      *keyspace.KeySpace
	world     NormalReturnFactSource
	projector projectedStatePathProjector
}

// NormalReturnFactsFromProjectedState converts one already-joined and
// root-reachable final State into the State-owned portion of NormalReturnFacts.
// roots are the exact ordered BoundaryRoots returned by BoundaryArtifact;
// roots[:paramCount] become $N placeholders. Later roots remain visible only
// when they are explicit return-slot or concrete capture/global roots.
//
// Alternatives must be joined in the State lattice before ProjectBoundary and
// this conversion. Appending separately projected alternatives is unsound for
// must lanes.
func NormalReturnFactsFromProjectedState(
	reg *axis.Registry,
	keys *keyspace.KeySpace,
	world state.State,
	roots state.BoundaryRoots,
	paramCount int,
) (NormalReturnFacts, error) {
	return NormalReturnFactsFromProjectedSource(reg, keys, world, roots, paramCount)
}

// NormalReturnFactsFromProjectedSource applies the canonical normal-return
// emitter program to any exact carrier of the declared lane observations.
func NormalReturnFactsFromProjectedSource(
	reg *axis.Registry,
	keys *keyspace.KeySpace,
	world NormalReturnFactSource,
	roots state.BoundaryRoots,
	paramCount int,
) (NormalReturnFacts, error) {
	projector, err := newProjectedStatePathProjector(reg, keys, roots, paramCount)
	if err != nil {
		return NormalReturnFacts{}, err
	}
	ctx := projectedStateFactContext{reg: reg, keys: keys, world: world, projector: projector}
	var out NormalReturnFacts
	for _, binding := range projectedStateFactHandlers {
		if binding.Value.project != nil {
			binding.Value.project(ctx, &out)
		}
	}
	return out, nil
}

type projectedStatePathRoot struct {
	source keyspace.Key
	target pathdom.Path
	value  product.Value
}

type projectedStatePathProjector struct {
	keys  *keyspace.KeySpace
	roots []projectedStatePathRoot
}

func newProjectedStatePathProjector(reg *axis.Registry, keys *keyspace.KeySpace, roots state.BoundaryRoots, paramCount int) (projectedStatePathProjector, error) {
	if reg == nil || keys == nil || !keys.Valid() || paramCount < 0 || paramCount > len(roots) {
		return projectedStatePathProjector{}, fmt.Errorf("callboundary: projected State facts require registry, keyspace, and valid parameter arity")
	}
	out := projectedStatePathProjector{keys: keys, roots: make([]projectedStatePathRoot, 0, len(roots))}
	paramTargets := make(map[key.Value]pathdom.Path, paramCount)
	for index, root := range roots {
		if !product.BelongsToRegistry(reg, root.Value) {
			return projectedStatePathProjector{}, fmt.Errorf("callboundary: projected State root %d belongs to a foreign registry", index)
		}
		if root.Path.Kind == keyspace.KindInvalid {
			if index < paramCount && root.Slot != 0 {
				// A parameter value whose binding was overwritten has no
				// caller-visible structural path, but its tuple ordinal remains
				// exact and later explicitly linked roots may still use it.
				paramTargets[root.Slot] = pathdom.NewPlaceholder(index)
			}
			continue
		}
		if keys.FormatReadOnly(root.Path) == "" {
			return projectedStatePathProjector{}, fmt.Errorf("callboundary: projected State root %d belongs to a foreign keyspace", index)
		}
		source, concrete := keys.StatePath(root.Path)
		formalRoot, formalSource := keys.DescribeFormalRoot(root.Path)
		if !concrete && !formalSource {
			return projectedStatePathProjector{}, fmt.Errorf("callboundary: projected State root %d has no structural vocabulary", index)
		}
		var target pathdom.Path
		if formalSource && formalRoot.Vocabulary() == formal.Input && formalRoot.Ordinal() <= uint64(paramCount) {
			target = pathdom.NewPlaceholder(int(formalRoot.Ordinal() - 1))
			if base, ok := keys.StructuralRoot(root.Path); ok {
				if suffix, exact := keys.ExactRemainderAfterPrefix(root.Path, base); exact {
					target = target.AppendSegments(suffix)
				}
			}
			if root.Slot != 0 {
				paramTargets[root.Slot] = target
			}
		} else if formalSource && formalRoot.Vocabulary() == formal.Input {
			sym, exact := key.ParseSymbolValue(root.Slot)
			if !exact {
				return projectedStatePathProjector{}, fmt.Errorf("callboundary: non-parameter formal input root %d has no persistent symbol identity", index)
			}
			target = pathdom.NewPath(sym, "")
			if base, ok := keys.StructuralRoot(root.Path); ok {
				if suffix, exact := keys.ExactRemainderAfterPrefix(root.Path, base); exact {
					target = target.AppendSegments(suffix)
				}
			}
		} else if index < paramCount && concrete {
			target = pathdom.NewPlaceholder(index)
			if root.Slot != 0 {
				paramTargets[root.Slot] = target
			}
		} else if paramTarget, ok := paramTargets[root.Slot]; ok && root.Slot != 0 {
			// One parameter may have several exact structural State roots
			// (for example its stable root and initial resolver version). The
			// shared value slot is the authority that they denote one $N root.
			target = paramTarget
		} else if formalSource && formalRoot.Vocabulary() == formal.Output {
			target = pathdom.Path{Root: fmt.Sprintf("ret[%d]", formalRoot.Ordinal()-1)}
			if base, ok := keys.StructuralRoot(root.Path); ok {
				if suffix, exact := keys.ExactRemainderAfterPrefix(root.Path, base); exact {
					target = target.AppendSegments(suffix)
				}
			}
		} else if result, ok := key.ParseReturnSlot(root.Slot); ok {
			target = pathdom.Path{Root: fmt.Sprintf("ret[%d]", result)}
		} else if concrete && source.ReturnSlotIndex() >= 0 {
			target = source
		} else if concrete && source.Symbol != 0 {
			symbol, symbolRoot := key.ParseSymbolValue(root.Slot)
			if !symbolRoot || symbol != source.Symbol || len(source.Segments) != 0 {
				continue
			}
			target = source
		} else {
			continue
		}
		out.roots = append(out.roots, projectedStatePathRoot{source: root.Path, target: target, value: root.Value})
	}
	return out, nil
}

func (p projectedStatePathProjector) key(source keyspace.Key) (pathdom.Path, bool) {
	if p.keys == nil || p.keys.FormatReadOnly(source) == "" {
		return pathdom.Path{}, false
	}
	best := int(^uint(0) >> 1)
	var target pathdom.Path
	for _, root := range p.roots {
		var suffix []segment.Segment
		var ok bool
		if root.source.Kind == keyspace.KindUnversionedSym {
			ok = source == root.source
		} else {
			suffix, ok = p.keys.ExactRemainderAfterPrefix(source, root.source)
		}
		if ok && root.source.Kind == keyspace.KindUnversionedSym && len(suffix) != 0 {
			// Unversioned symbol roots are exact value-slot roots, not an
			// authority to absorb arbitrary resolver descendants.
			ok = false
		}
		if !ok || len(suffix) > best {
			continue
		}
		best = len(suffix)
		target = root.target.AppendSegments(suffix)
	}
	return target, best != int(^uint(0)>>1)
}

func (p projectedStatePathProjector) stateKey(source pathaddr.StateKey) (pathdom.Path, bool) {
	if source == "" || p.keys == nil {
		return pathdom.Path{}, false
	}
	key, ok := p.keys.InternStateKey(source)
	if !ok {
		return pathdom.Path{}, false
	}
	return p.key(key)
}

func projectStatePathRefinements(ctx projectedStateFactContext, out *NormalReturnFacts) {
	ctx.world.ForEachPathRefinement(func(source keyspace.Key, value product.Value) bool {
		value, useful := ProjectPathRefinementValue(ctx.reg, value)
		if !useful {
			return true
		}
		target, ok := ctx.projector.key(source)
		if ok {
			out.PathRefinements = append(out.PathRefinements, PathValueFact{Path: target, Value: value})
		}
		return true
	})
}

func projectStatePathStaticMembers(ctx projectedStateFactContext, out *NormalReturnFacts) {
	bottom := product.Bottom(ctx.reg)
	ctx.world.ForEachPathStaticMember(func(source keyspace.Key, value product.Value) bool {
		if product.Equal(ctx.reg, value, bottom) {
			return true
		}
		target, ok := ctx.projector.key(source)
		if ok {
			out.PathStaticMembers = append(out.PathStaticMembers, PathStaticMemberFact{Path: target, Value: value})
		}
		return true
	})
}

func projectStatePathPresenceImplications(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.PathPresenceImplicationsSnapshot(ctx.keys)
	if snapshot.Bottom {
		return
	}
	for _, implication := range snapshot.Implications {
		if implication.HasTriggerPathEqual {
			continue
		}
		trigger, ok := ctx.projector.key(implication.Trigger)
		if !ok {
			continue
		}
		target, ok := ctx.projector.key(implication.Target)
		if !ok {
			continue
		}
		fact := PathPresenceImplicationFact{
			Trigger: trigger, TriggerPresence: implication.TriggerPresence,
			HasTriggerValue: implication.HasTriggerValue,
			Target:          target, TargetPresence: implication.TargetPresence,
			HasTargetValue: implication.HasTargetValue,
		}
		if fact.HasTriggerValue {
			fact.TriggerValue = product.ProjectBoundary(ctx.reg, implication.TriggerValue)
		}
		if fact.HasTargetValue {
			fact.TargetValue = product.ProjectBoundary(ctx.reg, implication.TargetValue)
		}
		out.PathPresenceImplications = append(out.PathPresenceImplications, fact)
	}
}

func projectStatePathInvalidations(ctx projectedStateFactContext, out *NormalReturnFacts) {
	if snapshot := ctx.world.DynamicIndexFactsSnapshot(); !snapshot.Top {
		for source, fact := range snapshot.Facts {
			if dynamicindex.Domain(ctx.reg).Equal(fact, dynamicindex.Bottom(ctx.reg)) {
				continue
			}
			if target, ok := ctx.projector.key(source.Table); ok {
				out.PathInvalidations = append(out.PathInvalidations, PathInvalidationFact{Path: target})
			}
		}
	}
	if snapshot := ctx.world.EffectDeltasSnapshot(); !snapshot.Top {
		for source := range snapshot.Deltas {
			target, ok := ctx.projector.key(source.Target)
			if !ok || source.Kind != effectdelta.Mutation {
				continue
			}
			switch {
			case IsPathInvalidationEffectSite(source.Site):
				out.PathInvalidations = append(out.PathInvalidations, PathInvalidationFact{Path: target})
			case IsPathStructuralPreservingInvalidationEffectSite(source.Site):
				out.PathInvalidations = append(out.PathInvalidations, PathInvalidationFact{Path: target, PreserveStructuralWitness: true})
			}
		}
	}
}

func projectStateDynamicIndexFacts(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.DynamicIndexFactsSnapshot()
	if snapshot.Top {
		return
	}
	domain := dynamicindex.Domain(ctx.reg)
	for source, value := range snapshot.Facts {
		target, ok := ctx.projector.key(source.Table)
		if !ok || domain.Equal(value, dynamicindex.Bottom(ctx.reg)) || domain.Equal(value, dynamicindex.Top()) {
			continue
		}
		out.DynamicIndexFacts = append(out.DynamicIndexFacts, DynamicIndexFact{Table: target, Site: source.Site, Value: value})
	}
}

func projectStateKeyMemberships(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.KeyMembershipsSnapshot()
	if snapshot.Bottom || snapshot.Top {
		return
	}
	for _, membership := range snapshot.Memberships {
		if membership.Kind != state.KeyMembershipPath {
			continue
		}
		keyPath, keyOK := ctx.projector.stateKey(membership.Key)
		tablePath, tableOK := ctx.projector.stateKey(membership.Table)
		if keyOK && tableOK {
			out.KeyMemberships = append(out.KeyMemberships, KeyMembershipFact{Key: keyPath, Table: tablePath})
		}
	}
}

func projectStateDynamicValueKeys(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.KeyMembershipsSnapshot()
	if snapshot.Bottom || snapshot.Top {
		return
	}
	for _, membership := range snapshot.Memberships {
		if membership.Kind != state.KeyMembershipDynamicIndexValue {
			continue
		}
		container, containerOK := ctx.projector.key(membership.Container)
		table, tableOK := ctx.projector.stateKey(membership.Table)
		if containerOK && tableOK {
			out.DynamicValueKeys = append(out.DynamicValueKeys, DynamicValueKeyMembershipFact{Container: container, Site: membership.Site, Table: table})
		}
	}
}

func projectStateDynamicAllValues(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.KeyMembershipsSnapshot()
	if snapshot.Bottom || snapshot.Top {
		return
	}
	for _, membership := range snapshot.Memberships {
		if membership.Kind != state.KeyMembershipDynamicIndexAllValues {
			continue
		}
		container, containerOK := ctx.projector.key(membership.Container)
		table, tableOK := ctx.projector.stateKey(membership.Table)
		if containerOK && tableOK {
			out.DynamicAllValues = append(out.DynamicAllValues, DynamicAllValueKeyMembershipFact{Container: container, Table: table})
		}
	}
}

func projectStateBranchProofs(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.BranchProofsSnapshot(ctx.keys)
	if snapshot.Bottom || snapshot.Top {
		return
	}
	for _, source := range snapshot.Proofs {
		target, ok := ctx.projector.key(source.Path)
		if !ok {
			continue
		}
		fact := BranchProof{Kind: source.Kind, Path: target}
		switch source.Kind {
		case pathevidence.BranchProofPathPresence:
			if source.Presence.IsBottom() || source.Presence.IsTop() {
				continue
			}
			fact.Presence = source.Presence
		case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual, pathevidence.BranchProofIndexInRange:
			other, ok := ctx.projector.key(source.Other)
			if !ok {
				continue
			}
			fact.Other = other
		default:
			continue
		}
		out.BranchProofs = append(out.BranchProofs, fact)
	}
}

func projectStateChannelSelects(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.ChannelSelectFactsSnapshot()
	if snapshot.Bottom || snapshot.Top {
		return
	}
	for _, source := range snapshot.Facts {
		fact := ChannelSelectFact{Select: source.Select, Kind: source.Kind, Index: source.Index, HasDefault: source.HasDefault}
		var ok bool
		if source.Result != "" {
			fact.Result, ok = ctx.projector.stateKey(source.Result)
			if !ok {
				continue
			}
		}
		if source.Case != "" {
			fact.Case, ok = ctx.projector.stateKey(source.Case)
			if !ok {
				continue
			}
		}
		out.ChannelSelects = append(out.ChannelSelects, fact)
	}
}

func projectStateFrozenTables(ctx projectedStateFactContext, out *NormalReturnFacts) {
	paths := projectedFrozenTablePaths(ctx)
	if snapshot := ctx.world.FrozenTablesSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, id := range snapshot.Tables {
			for _, target := range paths[id] {
				out.FrozenTables = append(out.FrozenTables, FrozenTableFact{Target: target})
			}
		}
	}
	if snapshot := ctx.world.EffectDeltasSnapshot(); !snapshot.Top {
		for source := range snapshot.Deltas {
			if source.Kind == effectdelta.Freeze && IsFrozenTableEffectSite(source.Site) {
				if target, ok := ctx.projector.key(source.Target); ok {
					out.FrozenTables = append(out.FrozenTables, FrozenTableFact{Target: target})
				}
			}
		}
	}
}

func projectStateEffectDeltas(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.EffectDeltasSnapshot()
	if snapshot.Top {
		return
	}
	for source, value := range snapshot.Deltas {
		target, ok := ctx.projector.key(source.Target)
		if !ok || source.Kind == effectdelta.Freeze && IsFrozenTableEffectSite(source.Site) ||
			source.Kind == effectdelta.Mutation && (IsPathInvalidationEffectSite(source.Site) || IsPathStructuralPreservingInvalidationEffectSite(source.Site)) {
			continue
		}
		domain := effectdelta.Domain(ctx.reg)
		if domain.Equal(value, domain.Bottom()) || domain.Equal(value, effectdelta.Top()) {
			continue
		}
		out.EffectDeltas = append(out.EffectDeltas, EffectDelta{Target: target, Site: source.Site, Kind: source.Kind, Value: value})
	}
}

func projectStateEscapeEvents(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.EscapeEventsSnapshot()
	if snapshot.Bottom || snapshot.Top {
		return
	}
	for _, event := range snapshot.Facts {
		if target, ok := ctx.projector.stateKey(event.Target); ok {
			out.EscapeEvents = append(out.EscapeEvents, EscapeEventFact{Target: target, Kind: event.Kind, Recursive: event.Recursive})
		}
	}
}

func projectStateStoreRelations(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.StoreRelationsSnapshot()
	if snapshot.Bottom || snapshot.Top {
		return
	}
	for _, relation := range snapshot.Relations {
		source, sourceOK := ctx.projector.stateKey(relation.Source)
		into, intoOK := ctx.projector.stateKey(relation.Into)
		if sourceOK && intoOK {
			out.StoreRelations = append(out.StoreRelations, StoreRelationFact{Source: source, Into: into})
		}
	}
}

func projectStateNumFloors(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.NumFloorsSnapshot(ctx.keys)
	if snapshot.Bottom {
		return
	}
	for source, floor := range snapshot.Floors {
		if target, ok := ctx.projector.stateKey(source); ok {
			out.NumFloors = append(out.NumFloors, NumFloorFact{Path: target, Floor: floor})
		}
	}
}

func projectStateNumCeils(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.NumCeilsSnapshot(ctx.keys)
	if snapshot.Bottom {
		return
	}
	for source, ceil := range snapshot.Ceils {
		if target, ok := ctx.projector.stateKey(source); ok {
			out.NumCeils = append(out.NumCeils, NumCeilFact{Path: target, Ceil: ceil})
		}
	}
}

func projectStateRelConstraints(ctx projectedStateFactContext, out *NormalReturnFacts) {
	snapshot := ctx.world.RelConstraints()
	if snapshot.Bottom || snapshot.Top {
		return
	}
	for _, source := range snapshot.Constraints {
		a, aOK := projectedRelOperand(ctx.projector, source.A)
		c, cOK := projectedRelOperand(ctx.projector, source.C)
		if !aOK || !cOK {
			continue
		}
		fact := RelConstraintFact{CoA: source.CoA, A: a, C: c, K: source.K}
		if source.B.IsValid() && source.CoB != 0 {
			b, ok := projectedRelOperand(ctx.projector, source.B)
			if !ok {
				continue
			}
			fact.CoB, fact.B = source.CoB, b
		}
		out.RelConstraints = append(out.RelConstraints, fact)
	}
}

func projectedRelOperand(projector projectedStatePathProjector, source state.RelOperand) (RelOperand, bool) {
	target, ok := projector.stateKey(source.StateKey())
	return RelOperand{Path: target, IsLength: source.IsLength()}, ok
}

type projectedFrozenCandidate struct {
	id   identity.ID
	path pathdom.Path
	seen map[identity.ID]struct{}
}

func projectedFrozenTablePaths(ctx projectedStateFactContext) map[identity.ID][]pathdom.Path {
	out := make(map[identity.ID][]pathdom.Path)
	queue := make([]projectedFrozenCandidate, 0)
	addValue := func(target pathdom.Path, value product.Value, seen map[identity.ID]struct{}) {
		id, ok := product.Get(ctx.reg, value, identity.Key).ID()
		if !ok || id == (identity.ID{}) || !addProjectedFrozenPath(out, id, target) {
			return
		}
		nextSeen := make(map[identity.ID]struct{}, len(seen)+1)
		for prior := range seen {
			nextSeen[prior] = struct{}{}
		}
		nextSeen[id] = struct{}{}
		queue = append(queue, projectedFrozenCandidate{id: id, path: target, seen: nextSeen})
	}
	for _, root := range ctx.projector.roots {
		addValue(root.target, root.value, nil)
		addValue(root.target, ctx.world.ReadLocalPathKey(ctx.reg, root.source), nil)
	}
	ctx.world.ForEachPathRefinement(func(source keyspace.Key, value product.Value) bool {
		if target, ok := ctx.projector.key(source); ok {
			addValue(target, value, nil)
		}
		return true
	})
	ctx.world.ForEachPathStaticMember(func(source keyspace.Key, value product.Value) bool {
		if target, ok := ctx.projector.key(source); ok {
			addValue(target, value, nil)
		}
		return true
	})
	heap := ctx.world.HeapTableObjectsSnapshot()
	if heap.Top {
		return out
	}
	for len(queue) != 0 {
		candidate := queue[0]
		queue = queue[1:]
		object, ok := heap.Objects[candidate.id]
		if !ok {
			continue
		}
		for suffix, value := range object.StaticMembers() {
			child, ok := product.Get(ctx.reg, value, identity.Key).ID()
			if !ok || child == (identity.ID{}) {
				continue
			}
			if _, seen := candidate.seen[child]; seen {
				continue
			}
			segments, ok := ctx.keys.SuffixSegmentsView(suffix)
			if !ok {
				continue
			}
			addValue(candidate.path.AppendSegments(segments), value, candidate.seen)
		}
	}
	return out
}

func addProjectedFrozenPath(paths map[identity.ID][]pathdom.Path, id identity.ID, target pathdom.Path) bool {
	if id == (identity.ID{}) || target.IsEmpty() {
		return false
	}
	for _, existing := range paths[id] {
		if existing.Equal(target) {
			return false
		}
	}
	paths[id] = append(paths[id], target)
	return true
}

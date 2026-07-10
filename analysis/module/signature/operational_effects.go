package signature

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// OperationalEffects carries analyzed, call-boundary facts across module
// manifests. Unlike Effect rows, this is not handwritten contract vocabulary:
// it is a stable, param-relative serialization of facts the analyzer proved.
type OperationalEffects struct {
	// SuspensionKnown certifies that MaySuspend is an exhaustive suspension
	// classification. A missing certification remains conservative for legacy
	// manifests, whose operational effects predate this field.
	SuspensionKnown                 bool
	MaySuspend                      bool
	ReturnPresenceRelations         []ReturnPresenceRelation
	NormalReturnPresenceRefinements []PathPresenceRefinement
	NormalReturnTypeRefinements     []PathTypeRefinement
	PathPresenceImplications        []PathPresenceImplication
	PathStaticMembers               []PathStaticMemberFact
	PathStaticMemberDeltas          []PathStaticMemberDelta
	PathInvalidations               []PathInvalidation
	BranchProofs                    []BranchProof
	DynamicIndexFacts               []DynamicIndexFact
	KeyMemberships                  []KeyMembership
	DynamicValueKeys                []DynamicValueKeyMembership
	FrozenTables                    []FrozenTable
	EscapeEvents                    []EscapeEvent
	StoreRelations                  []StoreRelation
	ParamRelations                  []ParamRelation
	ReturnFlows                     []ReturnFlow
	LifecycleEffects                []LifecycleEffect
	TypestateRequirements           []TypestateRequirement
	ReturnAllocationTemplates       []ReturnAllocationTemplate
}

type ReturnPresenceRelation struct {
	TriggerIndex    int
	TriggerPresence presence.Value
	TargetIndex     int
	TargetPresence  presence.Value
}

type PathPresenceRefinement struct {
	Path     pathdom.Path
	Presence presence.Value
}

type PathTypeRefinement struct {
	Path      pathdom.Path
	Type      typ.Type
	Assertion assertion.Value
}

type PathPresenceImplication struct {
	Trigger         pathdom.Path
	TriggerPresence presence.Value
	TriggerType     typ.Type
	HasTriggerType  bool
	Target          pathdom.Path
	TargetPresence  presence.Value
}

type PathStaticMemberFact struct {
	Path pathdom.Path
	Type typ.Type
}

type PathStaticMemberDelta struct {
	Path     pathdom.Path
	Type     typ.Type
	Required bool
}

type PathInvalidation struct {
	Path                      pathdom.Path
	PreserveStructuralWitness bool
}

type BranchProofKind uint8

const (
	BranchProofPathPresence BranchProofKind = iota + 1
	BranchProofPathEqual
	BranchProofPathNotEqual
	BranchProofIndexInRange
)

type BranchProof struct {
	Kind     BranchProofKind
	Path     pathdom.Path
	Presence presence.Value
	Other    pathdom.Path
}

type DynamicIndexAdmission string

const (
	DynamicIndexAdmissionAdmitted DynamicIndexAdmission = "admitted"
	DynamicIndexAdmissionRejected DynamicIndexAdmission = "rejected"
	DynamicIndexAdmissionUnknown  DynamicIndexAdmission = "unknown"
)

type DynamicIndexOperand struct {
	Path pathdom.Path
	Type typ.Type
}

type DynamicIndexFact struct {
	Table       pathdom.Path
	Site        string
	KeyPresence presence.Value
	Key         DynamicIndexOperand
	Value       DynamicIndexOperand
	Admission   DynamicIndexAdmission
}

type KeyMembership struct {
	Key   pathdom.Path
	Table pathdom.Path
}

type DynamicValueKeyMembership struct {
	Container pathdom.Path
	Site      string
	Table     pathdom.Path
}

type FrozenTable struct {
	Target pathdom.Path
}

type EscapeKind uint8

const (
	EscapeNone EscapeKind = iota
	EscapeBorrow
	EscapeRetain
	EscapeStore
	EscapeSend
	EscapeExport
	EscapeOpaque
)

type EscapeEvent struct {
	Target    pathdom.Path
	Kind      EscapeKind
	Recursive bool
}

type StoreRelation struct {
	Source pathdom.Path
	Into   pathdom.Path
}

type PlacementConsequence string

const (
	PlacementConsequenceKeep       PlacementConsequence = "keep"
	PlacementConsequenceOwnedHeap  PlacementConsequence = "owned-heap"
	PlacementConsequenceSharedHeap PlacementConsequence = "shared-heap"
)

type ParamRelation struct {
	Param                int
	EscapeClass          EscapeKind
	PlacementConsequence PlacementConsequence
	ThroughReturn        bool
	StoredInto           int
	HasStoredInto        bool
}

type ReturnFlowKind uint8

const (
	ReturnFlowInvalid ReturnFlowKind = iota
	ReturnFlowParam
	ReturnFlowParamMember
)

// ReturnFlow is a manifest return-flow relation. ReturnFlowParam preserves an
// exact parameter identity.
type ReturnFlow struct {
	ReturnIndex int
	Kind        ReturnFlowKind
	Param       int
	Path        []segment.Segment
}

type LifecycleKind uint8

const (
	LifecycleNone LifecycleKind = iota
	LifecycleAcquire
	LifecycleTransition
	LifecycleEscape
)

type LifecycleEffect struct {
	Target     pathdom.Path
	Kind       LifecycleKind
	Protocol   typestate.Protocol
	From       typestate.State
	To         typestate.State
	Obligation typestate.Obligation
}

// TypestateRequirement declares a call-entry precondition for a protocol
// resource. Target is parameter-relative (including a method receiver at $0);
// it does not mutate the resource state.
type TypestateRequirement struct {
	Target   pathdom.Path
	Protocol typestate.Protocol
	State    typestate.State
}

type AllocationTemplateID string

type ReturnAllocationTemplate struct {
	ReturnIndex int
	Root        AllocationTemplateID
	Objects     []AllocationObjectTemplate
}

type AllocationObjectTemplate struct {
	ID             AllocationTemplateID
	Type           typ.Type
	StableShape    bool
	PrefixStable   bool
	StaticMembers  []AllocationStaticMemberTemplate
	DynamicEntries []AllocationDynamicEntryTemplate
}

type AllocationStaticMemberTemplate struct {
	Suffix []segment.Segment
	Value  AllocationTemplateID
}

type AllocationDynamicEntryTemplate struct {
	Key     AllocationTemplateID
	KeyType typ.Type
	Value   AllocationTemplateID
}

func (e OperationalEffects) IsEmpty() bool {
	for _, lane := range operationalEffectLanes {
		if !lane.empty(e) {
			return false
		}
	}
	return true
}

func (e OperationalEffects) Clone() OperationalEffects {
	var out OperationalEffects
	for _, lane := range operationalEffectLanes {
		lane.clone(e, &out)
	}
	return out
}

func (e OperationalEffects) Equals(other OperationalEffects) bool {
	for _, lane := range operationalEffectLanes {
		if !lane.equal(e, other) {
			return false
		}
	}
	return true
}

type operationalEffectLane struct {
	fieldName       string
	empty           func(OperationalEffects) bool
	clone           func(OperationalEffects, *OperationalEffects)
	equal           func(OperationalEffects, OperationalEffects) bool
	substituteTypes func(*OperationalEffects, []*typ.TypeParam, []typ.Type)
}

func operationalEffectSliceLane[T any](
	fieldName string,
	get func(OperationalEffects) []T,
	set func(*OperationalEffects, []T),
	clone func([]T) []T,
	equal func([]T, []T) bool,
	substituteTypes func(*OperationalEffects, []*typ.TypeParam, []typ.Type),
) operationalEffectLane {
	return operationalEffectLane{
		fieldName: fieldName,
		empty: func(e OperationalEffects) bool {
			return len(get(e)) == 0
		},
		clone: func(e OperationalEffects, out *OperationalEffects) {
			set(out, clone(get(e)))
		},
		equal: func(a, b OperationalEffects) bool {
			return equal(get(a), get(b))
		},
		substituteTypes: substituteTypes,
	}
}

func operationalEffectBoolLane(
	fieldName string,
	get func(OperationalEffects) bool,
	set func(*OperationalEffects, bool),
) operationalEffectLane {
	return operationalEffectBoolLaneWithEmptiness(fieldName, get, set, true)
}

// operationalEffectCertificationLane builds a bool lane that clones, compares,
// and round-trips normally but never marks the struct non-empty. A certification
// rider travels with proven facts; on its own it is not a fact worth emitting,
// so an operational-effects object carrying only a certification stays absent.
func operationalEffectCertificationLane(
	fieldName string,
	get func(OperationalEffects) bool,
	set func(*OperationalEffects, bool),
) operationalEffectLane {
	return operationalEffectBoolLaneWithEmptiness(fieldName, get, set, false)
}

func operationalEffectBoolLaneWithEmptiness(
	fieldName string,
	get func(OperationalEffects) bool,
	set func(*OperationalEffects, bool),
	countsForEmptiness bool,
) operationalEffectLane {
	return operationalEffectLane{
		fieldName: fieldName,
		empty: func(e OperationalEffects) bool {
			if !countsForEmptiness {
				return true
			}
			return !get(e)
		},
		clone: func(e OperationalEffects, out *OperationalEffects) {
			set(out, get(e))
		},
		equal: func(a, b OperationalEffects) bool {
			return get(a) == get(b)
		},
	}
}

var operationalEffectLanes = []operationalEffectLane{
	operationalEffectCertificationLane("SuspensionKnown",
		func(e OperationalEffects) bool { return e.SuspensionKnown },
		func(e *OperationalEffects, value bool) { e.SuspensionKnown = value }),
	operationalEffectBoolLane("MaySuspend",
		func(e OperationalEffects) bool { return e.MaySuspend },
		func(e *OperationalEffects, value bool) { e.MaySuspend = value }),
	operationalEffectSliceLane("ReturnPresenceRelations",
		func(e OperationalEffects) []ReturnPresenceRelation { return e.ReturnPresenceRelations },
		func(e *OperationalEffects, facts []ReturnPresenceRelation) { e.ReturnPresenceRelations = facts },
		cloneReturnPresenceRelations, equalReturnPresenceRelations, nil),
	operationalEffectSliceLane("NormalReturnPresenceRefinements",
		func(e OperationalEffects) []PathPresenceRefinement { return e.NormalReturnPresenceRefinements },
		func(e *OperationalEffects, facts []PathPresenceRefinement) { e.NormalReturnPresenceRefinements = facts },
		clonePathPresenceRefinements, equalPathPresenceRefinements, nil),
	operationalEffectSliceLane("NormalReturnTypeRefinements",
		func(e OperationalEffects) []PathTypeRefinement { return e.NormalReturnTypeRefinements },
		func(e *OperationalEffects, facts []PathTypeRefinement) { e.NormalReturnTypeRefinements = facts },
		clonePathTypeRefinements, equalPathTypeRefinements, substitutePathTypeRefinementTypes),
	operationalEffectSliceLane("PathPresenceImplications",
		func(e OperationalEffects) []PathPresenceImplication { return e.PathPresenceImplications },
		func(e *OperationalEffects, facts []PathPresenceImplication) { e.PathPresenceImplications = facts },
		clonePathPresenceImplications, equalPathPresenceImplications, substitutePathPresenceImplicationTypes),
	operationalEffectSliceLane("PathStaticMembers",
		func(e OperationalEffects) []PathStaticMemberFact { return e.PathStaticMembers },
		func(e *OperationalEffects, facts []PathStaticMemberFact) { e.PathStaticMembers = facts },
		clonePathStaticMemberFacts, equalPathStaticMemberFacts, substitutePathStaticMemberTypes),
	operationalEffectSliceLane("PathStaticMemberDeltas",
		func(e OperationalEffects) []PathStaticMemberDelta { return e.PathStaticMemberDeltas },
		func(e *OperationalEffects, facts []PathStaticMemberDelta) { e.PathStaticMemberDeltas = facts },
		clonePathStaticMemberDeltas, equalPathStaticMemberDeltas, substitutePathStaticMemberDeltaTypes),
	operationalEffectSliceLane("PathInvalidations",
		func(e OperationalEffects) []PathInvalidation { return e.PathInvalidations },
		func(e *OperationalEffects, facts []PathInvalidation) { e.PathInvalidations = facts },
		clonePathInvalidations, equalPathInvalidations, nil),
	operationalEffectSliceLane("BranchProofs",
		func(e OperationalEffects) []BranchProof { return e.BranchProofs },
		func(e *OperationalEffects, facts []BranchProof) { e.BranchProofs = facts },
		cloneBranchProofs, equalBranchProofs, nil),
	operationalEffectSliceLane("DynamicIndexFacts",
		func(e OperationalEffects) []DynamicIndexFact { return e.DynamicIndexFacts },
		func(e *OperationalEffects, facts []DynamicIndexFact) { e.DynamicIndexFacts = facts },
		cloneDynamicIndexFacts, equalDynamicIndexFacts, substituteDynamicIndexFactTypes),
	operationalEffectSliceLane("KeyMemberships",
		func(e OperationalEffects) []KeyMembership { return e.KeyMemberships },
		func(e *OperationalEffects, facts []KeyMembership) { e.KeyMemberships = facts },
		cloneKeyMemberships, equalKeyMemberships, nil),
	operationalEffectSliceLane("DynamicValueKeys",
		func(e OperationalEffects) []DynamicValueKeyMembership { return e.DynamicValueKeys },
		func(e *OperationalEffects, facts []DynamicValueKeyMembership) { e.DynamicValueKeys = facts },
		cloneDynamicValueKeyMemberships, equalDynamicValueKeyMemberships, nil),
	operationalEffectSliceLane("FrozenTables",
		func(e OperationalEffects) []FrozenTable { return e.FrozenTables },
		func(e *OperationalEffects, facts []FrozenTable) { e.FrozenTables = facts },
		cloneFrozenTables, equalFrozenTables, nil),
	operationalEffectSliceLane("EscapeEvents",
		func(e OperationalEffects) []EscapeEvent { return e.EscapeEvents },
		func(e *OperationalEffects, facts []EscapeEvent) { e.EscapeEvents = facts },
		cloneEscapeEvents, equalEscapeEvents, nil),
	operationalEffectSliceLane("StoreRelations",
		func(e OperationalEffects) []StoreRelation { return e.StoreRelations },
		func(e *OperationalEffects, facts []StoreRelation) { e.StoreRelations = facts },
		cloneStoreRelations, equalStoreRelations, nil),
	operationalEffectSliceLane("ParamRelations",
		func(e OperationalEffects) []ParamRelation { return e.ParamRelations },
		func(e *OperationalEffects, facts []ParamRelation) { e.ParamRelations = facts },
		cloneParamRelations, equalParamRelations, nil),
	operationalEffectSliceLane("ReturnFlows",
		func(e OperationalEffects) []ReturnFlow { return e.ReturnFlows },
		func(e *OperationalEffects, facts []ReturnFlow) { e.ReturnFlows = facts },
		cloneReturnFlows, equalReturnFlows, nil),
	operationalEffectSliceLane("LifecycleEffects",
		func(e OperationalEffects) []LifecycleEffect { return e.LifecycleEffects },
		func(e *OperationalEffects, facts []LifecycleEffect) { e.LifecycleEffects = facts },
		cloneLifecycleEffects, equalLifecycleEffects, nil),
	operationalEffectSliceLane("TypestateRequirements",
		func(e OperationalEffects) []TypestateRequirement { return e.TypestateRequirements },
		func(e *OperationalEffects, facts []TypestateRequirement) { e.TypestateRequirements = facts },
		cloneTypestateRequirements, equalTypestateRequirements, nil),
	operationalEffectSliceLane("ReturnAllocationTemplates",
		func(e OperationalEffects) []ReturnAllocationTemplate { return e.ReturnAllocationTemplates },
		func(e *OperationalEffects, facts []ReturnAllocationTemplate) { e.ReturnAllocationTemplates = facts },
		cloneReturnAllocationTemplates, equalReturnAllocationTemplates, substituteReturnAllocationTemplateTypes),
}

func cloneReturnPresenceRelations(in []ReturnPresenceRelation) []ReturnPresenceRelation {
	if len(in) == 0 {
		return nil
	}
	return append([]ReturnPresenceRelation(nil), in...)
}

func clonePathPresenceRefinements(in []PathPresenceRefinement) []PathPresenceRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathPresenceRefinement, len(in))
	for i, fact := range in {
		out[i] = PathPresenceRefinement{Path: fact.Path.Clone(), Presence: fact.Presence}
	}
	return out
}

func clonePathTypeRefinements(in []PathTypeRefinement) []PathTypeRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathTypeRefinement, len(in))
	for i, fact := range in {
		out[i] = PathTypeRefinement{Path: fact.Path.Clone(), Type: fact.Type, Assertion: normalizePathTypeAssertion(fact.Assertion)}
	}
	return out
}

func clonePathPresenceImplications(in []PathPresenceImplication) []PathPresenceImplication {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathPresenceImplication, len(in))
	for i, fact := range in {
		out[i] = PathPresenceImplication{
			Trigger:         fact.Trigger.Clone(),
			TriggerPresence: fact.TriggerPresence,
			TriggerType:     fact.TriggerType,
			HasTriggerType:  fact.HasTriggerType,
			Target:          fact.Target.Clone(),
			TargetPresence:  fact.TargetPresence,
		}
	}
	return out
}

func clonePathStaticMemberFacts(in []PathStaticMemberFact) []PathStaticMemberFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathStaticMemberFact, len(in))
	for i, fact := range in {
		out[i] = PathStaticMemberFact{Path: fact.Path.Clone(), Type: fact.Type}
	}
	return out
}

func clonePathStaticMemberDeltas(in []PathStaticMemberDelta) []PathStaticMemberDelta {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathStaticMemberDelta, len(in))
	for i, fact := range in {
		out[i] = PathStaticMemberDelta{
			Path:     fact.Path.Clone(),
			Type:     fact.Type,
			Required: fact.Required,
		}
	}
	return out
}

func clonePathInvalidations(in []PathInvalidation) []PathInvalidation {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathInvalidation, len(in))
	for i, fact := range in {
		out[i] = PathInvalidation{
			Path:                      fact.Path.Clone(),
			PreserveStructuralWitness: fact.PreserveStructuralWitness,
		}
	}
	return out
}

func cloneBranchProofs(in []BranchProof) []BranchProof {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchProof, len(in))
	for i, proof := range in {
		out[i] = BranchProof{
			Kind:     proof.Kind,
			Path:     proof.Path.Clone(),
			Presence: proof.Presence,
			Other:    proof.Other.Clone(),
		}
	}
	return out
}

func cloneDynamicIndexFacts(in []DynamicIndexFact) []DynamicIndexFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]DynamicIndexFact, len(in))
	for i, fact := range in {
		out[i] = DynamicIndexFact{
			Table:       fact.Table.Clone(),
			Site:        fact.Site,
			KeyPresence: fact.KeyPresence,
			Key:         cloneDynamicIndexOperand(fact.Key),
			Value:       cloneDynamicIndexOperand(fact.Value),
			Admission:   fact.Admission,
		}
	}
	return out
}

func cloneDynamicIndexOperand(in DynamicIndexOperand) DynamicIndexOperand {
	return DynamicIndexOperand{Path: in.Path.Clone(), Type: in.Type}
}

func cloneKeyMemberships(in []KeyMembership) []KeyMembership {
	if len(in) == 0 {
		return nil
	}
	out := make([]KeyMembership, len(in))
	for i, fact := range in {
		out[i] = KeyMembership{Key: fact.Key.Clone(), Table: fact.Table.Clone()}
	}
	return out
}

func cloneDynamicValueKeyMemberships(in []DynamicValueKeyMembership) []DynamicValueKeyMembership {
	if len(in) == 0 {
		return nil
	}
	out := make([]DynamicValueKeyMembership, len(in))
	for i, fact := range in {
		out[i] = DynamicValueKeyMembership{
			Container: fact.Container.Clone(),
			Site:      fact.Site,
			Table:     fact.Table.Clone(),
		}
	}
	return out
}

func cloneFrozenTables(in []FrozenTable) []FrozenTable {
	if len(in) == 0 {
		return nil
	}
	out := make([]FrozenTable, len(in))
	for i, fact := range in {
		out[i] = FrozenTable{Target: fact.Target.Clone()}
	}
	return out
}

func cloneEscapeEvents(in []EscapeEvent) []EscapeEvent {
	if len(in) == 0 {
		return nil
	}
	out := make([]EscapeEvent, len(in))
	for i, fact := range in {
		out[i] = EscapeEvent{Target: fact.Target.Clone(), Kind: fact.Kind, Recursive: fact.Recursive}
	}
	return out
}

func cloneStoreRelations(in []StoreRelation) []StoreRelation {
	if len(in) == 0 {
		return nil
	}
	out := make([]StoreRelation, len(in))
	for i, fact := range in {
		out[i] = StoreRelation{Source: fact.Source.Clone(), Into: fact.Into.Clone()}
	}
	return out
}

func cloneParamRelations(in []ParamRelation) []ParamRelation {
	if len(in) == 0 {
		return nil
	}
	return append([]ParamRelation(nil), in...)
}

func cloneReturnFlows(in []ReturnFlow) []ReturnFlow {
	if len(in) == 0 {
		return nil
	}
	out := make([]ReturnFlow, len(in))
	for i, flow := range in {
		out[i] = flow
		out[i].Path = append([]segment.Segment(nil), flow.Path...)
	}
	return out
}

func cloneLifecycleEffects(in []LifecycleEffect) []LifecycleEffect {
	if len(in) == 0 {
		return nil
	}
	out := make([]LifecycleEffect, len(in))
	for i, fact := range in {
		out[i] = fact
		out[i].Target = fact.Target.Clone()
	}
	return out
}

func cloneReturnAllocationTemplates(in []ReturnAllocationTemplate) []ReturnAllocationTemplate {
	if len(in) == 0 {
		return nil
	}
	out := make([]ReturnAllocationTemplate, len(in))
	for i, template := range in {
		out[i] = ReturnAllocationTemplate{
			ReturnIndex: template.ReturnIndex,
			Root:        template.Root,
			Objects:     cloneAllocationObjectTemplates(template.Objects),
		}
	}
	return out
}

func cloneAllocationObjectTemplates(in []AllocationObjectTemplate) []AllocationObjectTemplate {
	if len(in) == 0 {
		return nil
	}
	out := make([]AllocationObjectTemplate, len(in))
	for i, object := range in {
		out[i] = AllocationObjectTemplate{
			ID:             object.ID,
			Type:           object.Type,
			StableShape:    object.StableShape,
			PrefixStable:   object.PrefixStable,
			StaticMembers:  cloneAllocationStaticMemberTemplates(object.StaticMembers),
			DynamicEntries: cloneAllocationDynamicEntryTemplates(object.DynamicEntries),
		}
	}
	return out
}

func cloneAllocationStaticMemberTemplates(in []AllocationStaticMemberTemplate) []AllocationStaticMemberTemplate {
	if len(in) == 0 {
		return nil
	}
	out := make([]AllocationStaticMemberTemplate, len(in))
	for i, member := range in {
		out[i] = AllocationStaticMemberTemplate{
			Suffix: append([]segment.Segment(nil), member.Suffix...),
			Value:  member.Value,
		}
	}
	return out
}

func cloneAllocationDynamicEntryTemplates(in []AllocationDynamicEntryTemplate) []AllocationDynamicEntryTemplate {
	if len(in) == 0 {
		return nil
	}
	out := make([]AllocationDynamicEntryTemplate, len(in))
	copy(out, in)
	return out
}

func equalReturnPresenceRelations(a, b []ReturnPresenceRelation) bool {
	return equalFactSlices(a, b, func(x, y ReturnPresenceRelation) bool {
		return x == y
	})
}

func equalPathPresenceRefinements(a, b []PathPresenceRefinement) bool {
	return equalFactSlices(a, b, func(x, y PathPresenceRefinement) bool {
		return x.Path.Equal(y.Path) && presence.Equal(x.Presence, y.Presence)
	})
}

func equalPathTypeRefinements(a, b []PathTypeRefinement) bool {
	return equalFactSlices(a, b, func(x, y PathTypeRefinement) bool {
		return x.Path.Equal(y.Path) && typ.TypeEquals(x.Type, y.Type) && assertion.Equal(normalizePathTypeAssertion(x.Assertion), normalizePathTypeAssertion(y.Assertion))
	})
}

func equalPathPresenceImplications(a, b []PathPresenceImplication) bool {
	return equalFactSlices(a, b, func(x, y PathPresenceImplication) bool {
		return x.Trigger.Equal(y.Trigger) &&
			presence.Equal(x.TriggerPresence, y.TriggerPresence) &&
			typ.TypeEquals(x.TriggerType, y.TriggerType) &&
			x.HasTriggerType == y.HasTriggerType &&
			x.Target.Equal(y.Target) &&
			presence.Equal(x.TargetPresence, y.TargetPresence)
	})
}

func normalizePathTypeAssertion(v assertion.Value) assertion.Value {
	if v.IsBottom() {
		return assertion.Top()
	}
	return v
}

func equalPathStaticMemberFacts(a, b []PathStaticMemberFact) bool {
	return equalFactSlices(a, b, func(x, y PathStaticMemberFact) bool {
		return x.Path.Equal(y.Path) && typ.TypeEquals(x.Type, y.Type)
	})
}

func equalPathStaticMemberDeltas(a, b []PathStaticMemberDelta) bool {
	return equalFactSlices(a, b, func(x, y PathStaticMemberDelta) bool {
		return x.Path.Equal(y.Path) &&
			x.Required == y.Required &&
			typ.TypeEquals(x.Type, y.Type)
	})
}

func equalPathInvalidations(a, b []PathInvalidation) bool {
	return equalFactSlices(a, b, func(x, y PathInvalidation) bool {
		return x.Path.Equal(y.Path) && x.PreserveStructuralWitness == y.PreserveStructuralWitness
	})
}

func equalBranchProofs(a, b []BranchProof) bool {
	return equalFactSlices(a, b, func(x, y BranchProof) bool {
		return x.Kind == y.Kind &&
			x.Path.Equal(y.Path) &&
			presence.Equal(x.Presence, y.Presence) &&
			x.Other.Equal(y.Other)
	})
}

func equalDynamicIndexFacts(a, b []DynamicIndexFact) bool {
	return equalFactSlices(a, b, func(x, y DynamicIndexFact) bool {
		return x.Table.Equal(y.Table) &&
			x.Site == y.Site &&
			presence.Equal(x.KeyPresence, y.KeyPresence) &&
			equalDynamicIndexOperand(x.Key, y.Key) &&
			equalDynamicIndexOperand(x.Value, y.Value) &&
			x.Admission == y.Admission
	})
}

func equalDynamicIndexOperand(a, b DynamicIndexOperand) bool {
	return a.Path.Equal(b.Path) && typ.TypeEquals(a.Type, b.Type)
}

func equalKeyMemberships(a, b []KeyMembership) bool {
	return equalFactSlices(a, b, func(x, y KeyMembership) bool {
		return x.Key.Equal(y.Key) && x.Table.Equal(y.Table)
	})
}

func equalDynamicValueKeyMemberships(a, b []DynamicValueKeyMembership) bool {
	return equalFactSlices(a, b, func(x, y DynamicValueKeyMembership) bool {
		return x.Container.Equal(y.Container) && x.Site == y.Site && x.Table.Equal(y.Table)
	})
}

func equalFrozenTables(a, b []FrozenTable) bool {
	return equalFactSlices(a, b, func(x, y FrozenTable) bool {
		return x.Target.Equal(y.Target)
	})
}

func equalEscapeEvents(a, b []EscapeEvent) bool {
	return equalFactSlices(a, b, func(x, y EscapeEvent) bool {
		return x.Kind == y.Kind && x.Recursive == y.Recursive && x.Target.Equal(y.Target)
	})
}

func equalStoreRelations(a, b []StoreRelation) bool {
	return equalFactSlices(a, b, func(x, y StoreRelation) bool {
		return x.Source.Equal(y.Source) && x.Into.Equal(y.Into)
	})
}

func equalParamRelations(a, b []ParamRelation) bool {
	return equalFactSlices(a, b, func(x, y ParamRelation) bool {
		return x == y
	})
}

func equalReturnFlows(a, b []ReturnFlow) bool {
	return equalFactSlices(a, b, func(x, y ReturnFlow) bool {
		if x.ReturnIndex != y.ReturnIndex || x.Kind != y.Kind || x.Param != y.Param || len(x.Path) != len(y.Path) {
			return false
		}
		for i := range x.Path {
			if x.Path[i] != y.Path[i] {
				return false
			}
		}
		return true
	})
}

func equalLifecycleEffects(a, b []LifecycleEffect) bool {
	return equalFactSlices(a, b, func(x, y LifecycleEffect) bool {
		return x.Kind == y.Kind &&
			x.Protocol == y.Protocol &&
			x.From == y.From &&
			x.To == y.To &&
			x.Obligation == y.Obligation &&
			x.Target.Equal(y.Target)
	})
}

func equalReturnAllocationTemplates(a, b []ReturnAllocationTemplate) bool {
	return equalFactSlices(a, b, func(x, y ReturnAllocationTemplate) bool {
		return x.ReturnIndex == y.ReturnIndex && x.Root == y.Root && equalAllocationObjectTemplates(x.Objects, y.Objects)
	})
}

func equalAllocationObjectTemplates(a, b []AllocationObjectTemplate) bool {
	return equalFactSlices(a, b, func(x, y AllocationObjectTemplate) bool {
		return x.ID == y.ID && x.StableShape == y.StableShape && x.PrefixStable == y.PrefixStable && typ.TypeEquals(x.Type, y.Type) && equalAllocationStaticMemberTemplates(x.StaticMembers, y.StaticMembers) && equalAllocationDynamicEntryTemplates(x.DynamicEntries, y.DynamicEntries)
	})
}

func equalAllocationStaticMemberTemplates(a, b []AllocationStaticMemberTemplate) bool {
	return equalFactSlices(a, b, func(x, y AllocationStaticMemberTemplate) bool {
		return x.Value == y.Value && segment.FormatSegments(x.Suffix) == segment.FormatSegments(y.Suffix)
	})
}

func equalAllocationDynamicEntryTemplates(a, b []AllocationDynamicEntryTemplate) bool {
	return equalFactSlices(a, b, func(x, y AllocationDynamicEntryTemplate) bool {
		return x.Key == y.Key && x.Value == y.Value && typ.TypeEquals(x.KeyType, y.KeyType)
	})
}

// equalFactSlices reports whether a and b have equal length and every aligned
// pair is equal under equal.
func equalFactSlices[T any](a, b []T, equal func(a, b T) bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

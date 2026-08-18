package engine

// This file is the direct first-construction seam.  A ProgramMember, query,
// or observation is a small immutable owner row: it carries the typed owner
// implementation and the declarative coordinates needed to join that owner to
// the one sealed graph.  It does not carry a graph row, a construction handle,
// or an attachment callback.  ConstructProgram performs the whole join once
// and drops these descriptors before minting the Solver.

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ProgramGraph is the opaque graph authority accepted by ConstructProgram.
// ReceiptGraph implements it inside engine; the interface keeps this direct
// seam independent of the compile-side graph declaration file.
type ProgramGraph interface {
	directProgramGraphValid(*SchemaBinding) bool
	directProgramGraphState() *schemaBindingState
	directProgramGraphValue() *equation.Graph
	directProgramTopology() *equation.Topology
	directPublishedQueryKeys() ([]composition.Key, bool)
	directMountedRuleMember(RuleSlotCapability, identity.ContentID, identity.ContentID, identity.ContentID) (equation.RuleMember, bool)
	directLinkRuleMember(RuleSlotCapability, identity.ContentID) (equation.RuleMember, bool)
	directMountedActivationMember(RuleSlotCapability, identity.ContentID, identity.ContentID, identity.ContentID) (equation.RuleMember, bool)
	directQuery(identity.ContentID) (equation.Query, bool)
}

// ProgramMember is one owner implementation row to be joined with the sealed
// graph.  Values are issued by the typed constructors below; the representation
// is intentionally opaque so callers cannot manufacture a runtime member or
// retain an equation handle.
type ProgramMember struct {
	source directProgramMember
}

type directProgramMember interface {
	bind(*programPlane, ProgramGraph) (runtimeMember, bool)
	key(ProgramGraph) (composition.Key, bool)
}

type mountedProgramMember[K ~uint32 | ~uint64, V, O any] struct {
	implementation *RuleImplementation[K, V, O]
	capability     RuleSlotCapability
	mount          identity.ContentID
	point          identity.ContentID
	occurrence     identity.ContentID
}

func (member *mountedProgramMember[K, V, O]) key(graph ProgramGraph) (composition.Key, bool) {
	if member == nil || graph == nil {
		return composition.Key{}, false
	}
	row, ok := graph.directMountedRuleMember(member.capability, member.mount, member.point, member.occurrence)
	if !ok {
		return composition.Key{}, false
	}
	return row.Key(), row.Key().Available()
}

func (member *mountedProgramMember[K, V, O]) bind(plane *programPlane, graph ProgramGraph) (runtimeMember, bool) {
	if member == nil || graph == nil {
		return nil, false
	}
	row, ok := graph.directMountedRuleMember(member.capability, member.mount, member.point, member.occurrence)
	if !ok {
		return nil, false
	}
	operand, resolved := member.implementation.resolveOperand(OperandCoords{Mount: member.mount, Point: member.point, Occurrence: member.occurrence})
	if !resolved {
		return nil, false
	}
	return bindProgramRuleMember(plane, member.implementation, row, operand)
}

// NewMountedProgramMember creates one mounted owner row.  The graph member is
// resolved only by ConstructProgram, after the binding and graph have been
// checked as one authority pair.
func NewMountedProgramMember[K ~uint32 | ~uint64, V, O any](implementation *RuleImplementation[K, V, O], capability RuleSlotCapability, mount, point, occurrence identity.ContentID) (ProgramMember, bool) {
	if implementation == nil || !capability.Mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return ProgramMember{}, false
	}
	return ProgramMember{source: &mountedProgramMember[K, V, O]{implementation: implementation, capability: capability, mount: mount, point: point, occurrence: occurrence}}, true
}

type linkProgramMember[K ~uint32 | ~uint64, V, O any] struct {
	implementation *RuleImplementation[K, V, O]
	capability     RuleSlotCapability
	occurrence     identity.ContentID
}

func (member *linkProgramMember[K, V, O]) key(graph ProgramGraph) (composition.Key, bool) {
	if member == nil || graph == nil {
		return composition.Key{}, false
	}
	row, ok := graph.directLinkRuleMember(member.capability, member.occurrence)
	if !ok {
		return composition.Key{}, false
	}
	return row.Key(), row.Key().Available()
}

func (member *linkProgramMember[K, V, O]) bind(plane *programPlane, graph ProgramGraph) (runtimeMember, bool) {
	if member == nil || graph == nil {
		return nil, false
	}
	row, ok := graph.directLinkRuleMember(member.capability, member.occurrence)
	if !ok {
		return nil, false
	}
	operand, resolved := member.implementation.resolveOperand(OperandCoords{Occurrence: member.occurrence})
	if !resolved {
		return nil, false
	}
	return bindProgramRuleMember(plane, member.implementation, row, operand)
}

// NewLinkProgramMember creates one Link-owned owner row.
func NewLinkProgramMember[K ~uint32 | ~uint64, V, O any](implementation *RuleImplementation[K, V, O], capability RuleSlotCapability, occurrence identity.ContentID) (ProgramMember, bool) {
	if implementation == nil || !capability.Link() || !occurrence.Available() {
		return ProgramMember{}, false
	}
	return ProgramMember{source: &linkProgramMember[K, V, O]{implementation: implementation, capability: capability, occurrence: occurrence}}, true
}

type activationProgramMember struct {
	implementation *ActivationRuleImplementation
	capability     RuleSlotCapability
	mount          identity.ContentID
	point          identity.ContentID
	occurrence     identity.ContentID
}

func (member *activationProgramMember) key(graph ProgramGraph) (composition.Key, bool) {
	if member == nil || graph == nil {
		return composition.Key{}, false
	}
	row, ok := graph.directMountedActivationMember(member.capability, member.mount, member.point, member.occurrence)
	if !ok {
		return composition.Key{}, false
	}
	return row.Key(), row.Key().Available()
}

func (member *activationProgramMember) bind(plane *programPlane, graph ProgramGraph) (runtimeMember, bool) {
	if member == nil || graph == nil || plane == nil || plane.runtime == nil || plane.runtime.graph == nil {
		return nil, false
	}
	row, ok := graph.directMountedActivationMember(member.capability, member.mount, member.point, member.occurrence)
	if !ok || plane.runtime.graph != graph.directProgramGraphValue() || graph.directProgramTopology() == nil {
		return nil, false
	}
	return bindActivationMemberReceipt(row, member.implementation, graph.directProgramTopology(), row.Key(), graph.directProgramGraphValue(), plane.byKey)
}

// NewMountedActivationProgramMember creates one mounted activation row.
func NewMountedActivationProgramMember(implementation *ActivationRuleImplementation, capability RuleSlotCapability, mount, point, occurrence identity.ContentID) (ProgramMember, bool) {
	if implementation == nil || !capability.Mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return ProgramMember{}, false
	}
	return ProgramMember{source: &activationProgramMember{implementation: implementation, capability: capability, mount: mount, point: point, occurrence: occurrence}}, true
}

// ProgramQuery is one owner query row.  Its graph query identity is resolved
// from id during construction; callers never receive or retain an equation
// Query value.
type ProgramQuery struct {
	source directProgramQuery
}

type directProgramQuery interface {
	bind(*programPlane, ProgramGraph) (runtimeQuery, bool)
}

type exactProgramQuery[V, R any] struct {
	implementation *ExactQueryImplementation[V, R]
	id             identity.ContentID
}

func (query *exactProgramQuery[V, R]) bind(plane *programPlane, graph ProgramGraph) (runtimeQuery, bool) {
	if query == nil || query.implementation == nil || graph == nil || !query.id.Available() {
		return nil, false
	}
	identity, ok := graph.directQuery(query.id)
	if !ok {
		return nil, false
	}
	return bindReceiptExactQueryRuntime(plane, query.implementation, identity)
}

// NewExactProgramQuery creates one exact query row.
func NewExactProgramQuery[V, R any](implementation *ExactQueryImplementation[V, R], id identity.ContentID) (ProgramQuery, bool) {
	if implementation == nil || !id.Available() {
		return ProgramQuery{}, false
	}
	return ProgramQuery{source: &exactProgramQuery[V, R]{implementation: implementation, id: id}}, true
}

type summaryProgramQuery[V, R any] struct {
	implementation *SummaryQueryImplementation[V, R]
	id             identity.ContentID
}

func (query *summaryProgramQuery[V, R]) bind(plane *programPlane, graph ProgramGraph) (runtimeQuery, bool) {
	if query == nil || query.implementation == nil || graph == nil || !query.id.Available() {
		return nil, false
	}
	identity, ok := graph.directQuery(query.id)
	if !ok {
		return nil, false
	}
	return bindReceiptSummaryQueryRuntime(plane, query.implementation, identity)
}

// NewSummaryProgramQuery creates one summary query row.
func NewSummaryProgramQuery[V, R any](implementation *SummaryQueryImplementation[V, R], id identity.ContentID) (ProgramQuery, bool) {
	if implementation == nil || !id.Available() {
		return ProgramQuery{}, false
	}
	return ProgramQuery{source: &summaryProgramQuery[V, R]{implementation: implementation, id: id}}, true
}

type ProgramObservation struct {
	source directProgramObservation
}

type directProgramObservation interface {
	bind(*programPlane, ProgramGraph) (runtimeObservation, bool)
	id() identity.ContentID
}

type summaryProgramObservation[V, R any] struct {
	implementation *SummaryQueryImplementation[V, R]
	capability     RuleSlotCapability
	mount          identity.ContentID
	point          identity.ContentID
	occurrence     identity.ContentID
	observationID  identity.ContentID
}

func (observation *summaryProgramObservation[V, R]) id() identity.ContentID {
	if observation == nil {
		return identity.ContentID{}
	}
	return observation.observationID
}

func (observation *summaryProgramObservation[V, R]) bind(plane *programPlane, graph ProgramGraph) (runtimeObservation, bool) {
	if observation == nil || observation.implementation == nil || graph == nil || plane == nil || !observation.observationID.Available() {
		return nil, false
	}
	member, ok := graph.directMountedRuleMember(observation.capability, observation.mount, observation.point, observation.occurrence)
	if !ok {
		return nil, false
	}
	state, authority, _, queryOrdinal, receiptOK := observation.implementation.boundTopologyQueryReceipt()
	if !receiptOK {
		return nil, false
	}
	owner := &receiptObservationOwner{state: state, authority: authority, schema: state.schema, query: queryOrdinal}
	return bindReceiptSummaryObservationRuntime(plane, observation.implementation, observation.observationID, member, owner)
}

// NewSummaryProgramObservation creates one solve-local summary observation
// row. The observation point is recovered from the graph member at bind time.
func NewSummaryProgramObservation[V, R any](implementation *SummaryQueryImplementation[V, R], capability RuleSlotCapability, mount, point, occurrence, observationID identity.ContentID) (ProgramObservation, bool) {
	if implementation == nil || !capability.Mounted() || !mount.Available() || !point.Available() || !occurrence.Available() || !observationID.Available() {
		return ProgramObservation{}, false
	}
	return ProgramObservation{source: &summaryProgramObservation[V, R]{implementation: implementation, capability: capability, mount: mount, point: point, occurrence: occurrence, observationID: observationID}}, true
}

type exactProgramObservation[V, R any] struct {
	implementation *ExactQueryImplementation[V, R]
	capability     RuleSlotCapability
	mount          identity.ContentID
	point          identity.ContentID
	occurrence     identity.ContentID
	observationID  identity.ContentID
}

func (observation *exactProgramObservation[V, R]) id() identity.ContentID {
	if observation == nil {
		return identity.ContentID{}
	}
	return observation.observationID
}

func (observation *exactProgramObservation[V, R]) bind(plane *programPlane, graph ProgramGraph) (runtimeObservation, bool) {
	if observation == nil || observation.implementation == nil || graph == nil || plane == nil || !observation.observationID.Available() {
		return nil, false
	}
	member, ok := graph.directMountedRuleMember(observation.capability, observation.mount, observation.point, observation.occurrence)
	if !ok {
		return nil, false
	}
	state, authority, _, queryOrdinal, receiptOK := observation.implementation.boundTopologyQueryReceipt()
	if !receiptOK {
		return nil, false
	}
	owner := &receiptObservationOwner{state: state, authority: authority, schema: state.schema, query: queryOrdinal}
	return bindReceiptExactObservationRuntime(plane, observation.implementation, observation.observationID, member, owner)
}

// NewExactProgramObservation creates one solve-local exact observation row.
func NewExactProgramObservation[V, R any](implementation *ExactQueryImplementation[V, R], capability RuleSlotCapability, mount, point, occurrence, observationID identity.ContentID) (ProgramObservation, bool) {
	if implementation == nil || !capability.Mounted() || !mount.Available() || !point.Available() || !occurrence.Available() || !observationID.Available() {
		return ProgramObservation{}, false
	}
	return ProgramObservation{source: &exactProgramObservation[V, R]{implementation: implementation, capability: capability, mount: mount, point: point, occurrence: occurrence, observationID: observationID}}, true
}

// ConstructProgram performs the direct first-construction join and seals one
// immutable runtime.  The graph remains the neutral geometry source; owner
// descriptors contribute only typed implementations and operands.  A failed
// join mints no Solver and releases no partially assembled runtime.
func ConstructProgram(binding *SchemaBinding, graph ProgramGraph, members []ProgramMember, queries []ProgramQuery, observations []ProgramObservation) (*Solver, SolveFailure, bool) {
	if binding == nil || graph == nil || !binding.Sealed() || !graph.directProgramGraphValid(binding) {
		return nil, ProgramConstructionFailure(ProgramConstructionStageAdmission), false
	}
	addressed, addressedOK := graph.directPublishedQueryKeys()
	if !addressedOK {
		return nil, ProgramConstructionFailure(ProgramConstructionStageQueryAddress), false
	}
	graphValue := graph.directProgramGraphValue()
	graphTopology := graph.directProgramTopology()
	if graphValue == nil || graphTopology == nil {
		return nil, ProgramConstructionFailure(ProgramConstructionStageAdmission), false
	}
	plane, planeOK := bindProgramPlane(bindingState(binding), graphValue)
	if !planeOK || plane == nil {
		return nil, ProgramConstructionFailure(ProgramConstructionStageFactorBind), false
	}

	drafts := make([]runtimeMember, 0, len(members))
	memberKeys := make(map[composition.Key]struct{}, len(members))
	for _, descriptor := range members {
		if descriptor.source == nil {
			return nil, ProgramConstructionFailure(ProgramConstructionStageMemberBind), false
		}
		key, keyOK := descriptor.source.key(graph)
		if !keyOK {
			return nil, ProgramConstructionFailure(ProgramConstructionStageMemberBind), false
		}
		if _, duplicate := memberKeys[key]; duplicate {
			return nil, ProgramConstructionFailure(ProgramConstructionStageMemberBind), false
		}
		row, rowOK := descriptor.source.bind(plane, graph)
		if !rowOK || row == nil || row.member().Key() != key {
			return nil, ProgramConstructionFailure(ProgramConstructionStageMemberBind), false
		}
		memberKeys[key] = struct{}{}
		drafts = append(drafts, row)
	}

	boundQueries := make(map[composition.Key]runtimeQuery, len(queries))
	for _, descriptor := range queries {
		if descriptor.source == nil {
			return nil, ProgramConstructionFailure(ProgramConstructionStageQueryAddress), false
		}
		row, rowOK := descriptor.source.bind(plane, graph)
		if !rowOK || row == nil || !row.query().Key().Available() {
			return nil, ProgramConstructionFailure(ProgramConstructionStageQueryAddress), false
		}
		key := row.query().Key()
		if _, duplicate := boundQueries[key]; duplicate {
			return nil, ProgramConstructionFailure(ProgramConstructionStageQueryAddress), false
		}
		boundQueries[key] = row
	}
	boundQueryRows, queriesOK := bindProgramQueryTable(addressed, graphValue, boundQueries)
	if !queriesOK {
		return nil, ProgramConstructionFailure(ProgramConstructionStageQueryAddress), false
	}

	boundObservations := make([]runtimeObservation, 0, len(observations))
	observationIDs := make(map[identity.ContentID]struct{}, len(observations))
	for _, descriptor := range observations {
		if descriptor.source == nil || !descriptor.source.id().Available() {
			return nil, ProgramConstructionFailure(ProgramConstructionStageObservationAddress), false
		}
		id := descriptor.source.id()
		if _, duplicate := observationIDs[id]; duplicate {
			return nil, ProgramConstructionFailure(ProgramConstructionStageObservationAddress), false
		}
		row, rowOK := descriptor.source.bind(plane, graph)
		if !rowOK || row == nil || row.observationID() != id {
			return nil, ProgramConstructionFailure(ProgramConstructionStageObservationAddress), false
		}
		observationIDs[id] = struct{}{}
		boundObservations = append(boundObservations, row)
	}

	state := bindingState(binding)
	if state == nil || state.authority == nil {
		return nil, ProgramConstructionFailure(ProgramConstructionStageAdmission), false
	}
	runtime, assembled := assembleReceiptRuntime(state, state.authority, graphValue, plane.carrier, plane.byKey, drafts, boundQueryRows, boundObservations)
	if !assembled || runtime == nil {
		return nil, ProgramConstructionFailure(ProgramConstructionStageProgramSeal), false
	}
	runtime.topology = graphTopology
	if !plane.releaseColdFactorBindings() {
		return nil, ProgramConstructionFailure(ProgramConstructionStageFactorBind), false
	}
	return mintProgramSolver(runtime)
}

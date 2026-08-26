package snapshot

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
)

// ObservationCell is a neutral copy of one descriptor-declared output. It
// carries an authenticated opaque value token but never decodes a domain
// value or retains a Proposal/Apply object.
type ObservationCell struct {
	// Destination is the exact mounted, scope-qualified address issued for
	// this output. Keeping the CellToken preserves both the owner-issued RowID
	// and the denominator witness; runtime never reconstructs either from a
	// parent row or a physical ordinal.
	Destination binding.CellToken
	Column      model.ColumnID
	Type        model.TypeID
	Presence    model.Presence
	Value       binding.ValueToken
}

func (cell ObservationCell) Available() bool {
	if !cell.Destination.Available() || !cell.Column.Available() || !cell.Type.Available() || !cell.Presence.Available() {
		return false
	}
	if cell.Destination.Column() != cell.Column || cell.Destination.Relation() != cell.Column.Relation() {
		return false
	}
	if cell.Presence.Is(model.Present) || cell.Presence.Is(model.AuthenticatedOpaque) {
		return cell.Value.Available() && cell.Value.Type() == cell.Type && cell.Value.Fence().Same(cell.Destination.Fence())
	}
	return !cell.Value.Available()
}

func (cell ObservationCell) ValidFor(fence binding.Fence) bool {
	return cell.Available() && cell.Destination.ValidFor(fence) && (!cell.Value.Available() || cell.Value.ValidFor(fence))
}

// Observation is the immutable neutral row copied from one Apply
// application. Its outcome is preserved exactly; publishing outcomes may
// carry descriptor-declared output cells, and Refused carries its stable
// reason.
type Observation struct {
	outcome outcome.Result
	lineage model.LineageRef
	outputs []ObservationCell
}

func newObservation(value outcome.Result, lineage model.LineageRef, outputs []ObservationCell) (Observation, bool) {
	if !value.Available() || !lineage.Available() || outputs == nil {
		return Observation{}, false
	}
	if !value.Code.Publishes() && len(outputs) != 0 {
		return Observation{}, false
	}
	if value.Code == outcome.Refused && len(outputs) != 0 {
		return Observation{}, false
	}
	for index, output := range outputs {
		if !output.Available() {
			return Observation{}, false
		}
		for _, prior := range outputs[:index] {
			// A column may be repeated only as a distinct destination row.
			// Cardinality is checked against the sealed descriptor before this
			// row is constructed; this invariant keeps malformed cells from
			// reintroducing same-row duplicates through another call path.
			if prior.Column == output.Column && prior.Destination.Row() == output.Destination.Row() {
				return Observation{}, false
			}
		}
	}
	// Preserve an authenticated empty output extent as non-nil. A terminal
	// NoSelection/NoCandidate row, or a publishing row whose declared optional
	// output is empty, remains a valid zero-cell observation; appending zero
	// elements to a nil slice would erase that distinction and make the sealed
	// observation unavailable.
	copyOutputs := make([]ObservationCell, len(outputs))
	copy(copyOutputs, outputs)
	result := Observation{outcome: value, lineage: lineage, outputs: copyOutputs}
	return result, result.Available()
}

func (observation Observation) Available() bool {
	if !observation.outcome.Available() || !observation.lineage.Available() || observation.outputs == nil {
		return false
	}
	if !observation.outcome.Code.Publishes() && len(observation.outputs) != 0 {
		return false
	}
	for index, output := range observation.outputs {
		if !output.Available() {
			return false
		}
		for _, prior := range observation.outputs[:index] {
			if prior.Column == output.Column && prior.Destination.Row() == output.Destination.Row() {
				return false
			}
		}
	}
	return true
}

// Outcome returns the exact closed terminal disposition, including Refused.
func (observation Observation) Outcome() outcome.Result {
	if !observation.Available() {
		return outcome.Result{}
	}
	return observation.outcome
}

func (observation Observation) Lineage() model.LineageRef {
	if !observation.Available() {
		return model.LineageRef{}
	}
	return observation.lineage
}

func (observation Observation) Outputs() []ObservationCell {
	if !observation.Available() {
		return nil
	}
	return append([]ObservationCell(nil), observation.outputs...)
}

func (observation Observation) Output(column model.ColumnID) (ObservationCell, bool) {
	if !observation.Available() || !column.Available() {
		return ObservationCell{}, false
	}
	for _, output := range observation.outputs {
		if output.Column == column {
			return output, true
		}
	}
	return ObservationCell{}, false
}

// OutputsFor returns every destination row carrying one declared column. A
// scalar descriptor yields at most one cell; bounded-many descriptors may
// retain several child rows for the same parent observation.
func (observation Observation) OutputsFor(column model.ColumnID) []ObservationCell {
	if !observation.Available() || !column.Available() {
		return nil
	}
	result := make([]ObservationCell, 0)
	for _, output := range observation.outputs {
		if output.Column == column {
			result = append(result, output)
		}
	}
	return result
}

// ObservationSnapshot owns only the canonical immutable Snapshot and the
// schema-derived query address. It does not retain terminal values,
// applications, proposal batches, a database root, or a second row store.
type ObservationSnapshot struct {
	published  canonical.Snapshot
	plan       canonical.QueryPlan[RowKey, Observation]
	family     identity.ContentID
	population model.DenominatorRef
	fence      binding.Fence
	sealed     bool
}

func (snapshot ObservationSnapshot) Available() bool {
	return snapshot.sealed && snapshot.published.Published() && snapshot.plan.Available() && snapshot.family.Available() && snapshot.population.Available() && snapshot.fence.Available()
}

func (snapshot ObservationSnapshot) Snapshot() canonical.Snapshot {
	if !snapshot.Available() {
		return canonical.Snapshot{}
	}
	return snapshot.published
}

func (snapshot ObservationSnapshot) Family() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
	}
	return snapshot.family
}

func (snapshot ObservationSnapshot) Plan() canonical.QueryPlan[RowKey, Observation] {
	if !snapshot.Available() {
		return canonical.QueryPlan[RowKey, Observation]{}
	}
	return snapshot.plan
}

func (snapshot ObservationSnapshot) Population() model.DenominatorRef {
	if !snapshot.Available() {
		return model.DenominatorRef{}
	}
	return snapshot.population
}

// Read answers one scope-qualified owner row. ReadProvenAbsent means the
// descriptor's closed population contains the key but no observation row was
// emitted for it; ReadMiss remains impossible for this published family.
func (snapshot ObservationSnapshot) Read(key RowKey) (Observation, canonical.ReadStatus) {
	if !snapshot.Available() || !key.ValidFor(snapshot.fence) {
		return Observation{}, canonical.ReadInvalid
	}
	return canonical.Query(&snapshot.published, snapshot.plan, key)
}

// Keys returns the closed population members in mounted denominator order for
// the authenticated observation scope. This is a projection of the
// descriptor, not a second row directory.
func (snapshot ObservationSnapshot) Keys() []RowKey {
	if !snapshot.Available() {
		return nil
	}
	return canonicalMembers(&snapshot.published, snapshot.plan)
}

// Project redeems one complete sealed Apply result extent from the terminal
// solve catalog into one canonical observation query column. The family
// argument is the content identity of a descriptor already sealed into the
// terminal result's mounted runtime. Snapshot never accepts a caller-selected
// Apply extent: the descriptor's exact (Dependency, Operation) key must redeem
// an authenticated catalog entry first.
//
// The result extent is consumed in one pass: all application rows are
// collected, all scope-qualified population members are declared once, and
// ExtendQuery publishes the family exactly once. Repeated per-application
// extension is deliberately impossible because a canonical query family is
// admitted only once.
//
// An empty extent is valid only when Apply retained its authenticated common
// input cofiber in Results.Scopes. Treating all mounted scopes as evaluated
// would fabricate absence; an empty extent with no scope remains refused.
func Project(result terminal.Result, base canonical.Snapshot, family identity.ContentID) (ObservationSnapshot, bool) {
	if !result.Available() || !base.Published() || !family.Available() {
		return ObservationSnapshot{}, false
	}
	root := result.Root()
	mounted := root.Mounted()
	if !mounted.Available() || !root.Fence().Same(mounted.RuntimeFence()) {
		return ObservationSnapshot{}, false
	}
	contract, contractOK := mounted.Observation(family)
	if !contractOK || !contract.Available() || contract.Digest() != family {
		return ObservationSnapshot{}, false
	}
	application, applicationOK := result.Applications().Lookup(contract.Dependency(), contract.Operation())
	if !applicationOK || !application.Available() || !application.Root().Fence().Same(root.Fence()) {
		return ObservationSnapshot{}, false
	}
	results := application.Result()
	if !results.Available() || results.Operation() != contract.Operation() {
		return ObservationSnapshot{}, false
	}
	operation := contract.Operation()
	bound, ok := mounted.Binding(operation)
	if !ok || bound == nil || !bound.Signature().Available() || bound.Signature().Identity() != operation {
		return ObservationSnapshot{}, false
	}
	expectedSchema := mounted.RuntimeFence().Schema().Content()
	expectedStore := mounted.Fence().StoreID()
	expectedGeneration := identity.Generation(root.Revision())
	if base.Schema() != expectedSchema || base.Store() != expectedStore || base.Generation() != expectedGeneration {
		return ObservationSnapshot{}, false
	}
	population := contract.Population()
	witnessValue, ok := mounted.Denominator(population)
	if !ok || !witnessValue.ValidFor(mounted.RuntimeFence()) || !witnessValue.Matches(population) {
		return ObservationSnapshot{}, false
	}
	// Every output owns a destination denominator in the sealed descriptor.
	// Resolve all of them before consuming an application so a missing or
	// foreign child population refuses the complete extent atomically.
	destinationWitnesses := make(map[model.DenominatorRef]binding.DenominatorWitness, len(contract.Outputs()))
	for _, output := range contract.Outputs() {
		destination := output.Destination()
		if !output.Available() || !destination.Available() || output.Column().Relation() != destination.Relation() {
			return ObservationSnapshot{}, false
		}
		if boundOutput, boundOK := bound.Signature().OutputFor(destination.Relation(), output.Column()); !boundOK || !boundOutput.Available() || boundOutput.Type != output.Type() {
			return ObservationSnapshot{}, false
		}
		// The sealed output owns its destination witness. Do not substitute a
		// different denominator or invent a routing adapter.
		if boundDestination, destinationOK := bound.Signature().OutputDestination(destination.Relation(), output.Column()); !destinationOK || boundDestination != destination {
			return ObservationSnapshot{}, false
		}
		value, destinationOK := mounted.Denominator(destination)
		if !destinationOK || !value.ValidFor(mounted.RuntimeFence()) || !value.Matches(destination) {
			return ObservationSnapshot{}, false
		}
		destinationWitnesses[destination] = value
	}
	rows := make(map[RowKey]Observation, results.Len())
	scopes := make([]witness.Scope, 0, len(results.Scopes()))
	for _, token := range results.Scopes() {
		scope, scopeOK := mounted.ScopeForToken(token)
		if !scopeOK || !scope.ValidFor(mounted.RuntimeFence()) {
			return ObservationSnapshot{}, false
		}
		duplicate := false
		for _, prior := range scopes {
			if prior.Same(scope) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			scopes = append(scopes, scope)
		}
	}
	for index := 0; index < results.Len(); index++ {
		application, applicationOK := results.At(index)
		if !applicationOK || !application.Available() || application.Operation() != operation || !application.Fence().Same(mounted.RuntimeFence()) {
			return ObservationSnapshot{}, false
		}
		invocation := application.Invocation()
		selector := contract.Source()
		child, childOK := invocation.ChildAt(int(selector.Child()))
		if !childOK {
			return ObservationSnapshot{}, false
		}
		tuple, tupleOK := child.At(int(selector.Tuple()))
		if !tupleOK {
			return ObservationSnapshot{}, false
		}
		row, rowOK := tuple.At(int(selector.Source()))
		if !rowOK || !row.Available() || !witnessValue.Contains(row) {
			return ObservationSnapshot{}, false
		}
		scopeValue, scopeOK := mounted.ScopeForToken(invocation.Scope())
		if !scopeOK || !scopeValue.ValidFor(mounted.RuntimeFence()) {
			return ObservationSnapshot{}, false
		}
		key := RowKey{Relation: population.Relation(), Row: row, Scope: scopeValue}
		if !key.ValidFor(mounted.RuntimeFence()) {
			return ObservationSnapshot{}, false
		}
		outputs, outputsOK := copyDeclaredOutputs(mounted, contract, destinationWitnesses, application)
		if !outputsOK {
			return ObservationSnapshot{}, false
		}
		if prior, duplicate := rows[key]; duplicate {
			merged, mergeOK := mergeObservation(mounted, prior, application, outputs)
			if !mergeOK {
				return ObservationSnapshot{}, false
			}
			rows[key] = merged
		} else {
			value, valueOK := newObservation(application.Outcome(), application.Lineage(), outputs)
			if !valueOK {
				return ObservationSnapshot{}, false
			}
			rows[key] = value
		}
		seenScope := false
		for _, prior := range scopes {
			if prior.Same(scopeValue) {
				seenScope = true
				break
			}
		}
		if !seenScope {
			scopes = append(scopes, scopeValue)
		}
	}
	for _, observation := range rows {
		if !validateOutputCardinality(contract, observation) {
			return ObservationSnapshot{}, false
		}
	}
	if len(scopes) == 0 {
		return ObservationSnapshot{}, false
	}
	keys := make([]RowKey, 0, witnessValue.Len()*len(scopes))
	for _, scopeValue := range scopes {
		for index := 0; index < witnessValue.Len(); index++ {
			member, memberOK := witnessValue.At(index)
			if !memberOK {
				return ObservationSnapshot{}, false
			}
			key := RowKey{Relation: population.Relation(), Row: member, Scope: scopeValue}
			if !key.ValidFor(mounted.RuntimeFence()) {
				return ObservationSnapshot{}, false
			}
			keys = append(keys, key)
		}
	}
	content := canonical.Content[RowKey, Observation]{Rows: rows, Denominator: family, Members: keys}
	published, plan, err := canonical.ExtendQuery(base, family, content)
	if err != nil || !published.Published() || published.Store() != base.Store() || published.Generation() != base.Generation() {
		return ObservationSnapshot{}, false
	}
	snapshotValue := ObservationSnapshot{published: published, plan: plan, family: family, population: population, fence: mounted.RuntimeFence(), sealed: true}
	if !snapshotValue.Available() {
		return ObservationSnapshot{}, false
	}
	return snapshotValue, true
}

func copyDeclaredOutputs(mounted witness.Mounted, contract algebra.ObservationContract, destinationWitnesses map[model.DenominatorRef]binding.DenominatorWitness, application apply.Application) ([]ObservationCell, bool) {
	if !application.Outcome().Code.Publishes() {
		if batch, ok := application.Proposals(); ok && batch.Len() != 0 {
			return nil, false
		}
		return []ObservationCell{}, true
	}
	batch, ok := application.Proposals()
	if !ok {
		return nil, false
	}
	declared := contract.Outputs()
	result := make([]ObservationCell, 0, batch.Len())
	rows := make(map[model.ColumnID]map[model.RowID]struct{}, len(declared))
	for index := 0; index < batch.Len(); index++ {
		proposal, proposalOK := batch.At(index)
		if !proposalOK || !proposal.Available() || !proposal.Destination().ValidFor(mounted.RuntimeFence()) || !proposal.Destination().Scope().Same(application.Invocation().Scope()) {
			return nil, false
		}
		output, outputOK := boundOutput(declared, proposal.Destination().Column())
		if !outputOK || output.Column().Relation() != proposal.Destination().Relation() {
			return nil, false
		}
		destination := output.Destination()
		witnessValue, witnessOK := destinationWitnesses[destination]
		if !witnessOK || !witnessValue.ValidFor(mounted.RuntimeFence()) || !witnessValue.Matches(destination) || !proposal.Destination().Witness().Same(witnessValue) {
			return nil, false
		}
		declaredOperationOutput, declaredOperationOK := mountedSignatureOutput(mounted, contract, output)
		if !declaredOperationOK || declaredOperationOutput.Denominator != destination || !declaredOperationOutput.Presence.Allows(proposal.Presence()) || declaredOperationOutput.Type != output.Type() {
			return nil, false
		}
		if proposal.Value().Available() && proposal.Value().Type() != output.Type() {
			return nil, false
		}
		columnRows := rows[output.Column()]
		if columnRows == nil {
			columnRows = make(map[model.RowID]struct{})
			rows[output.Column()] = columnRows
		}
		if _, duplicate := columnRows[proposal.Destination().Row()]; duplicate {
			return nil, false
		}
		columnRows[proposal.Destination().Row()] = struct{}{}
		result = append(result, ObservationCell{Destination: proposal.Destination(), Column: output.Column(), Type: output.Type(), Presence: proposal.Presence(), Value: proposal.Value()})
	}
	return result, true
}

func mergeObservation(mounted witness.Mounted, prior Observation, application apply.Application, outputs []ObservationCell) (Observation, bool) {
	if !prior.Available() || !application.Available() || prior.Outcome().Code != application.Outcome().Code || application.Outcome().Code == outcome.Refused {
		return Observation{}, false
	}
	authority, authorityOK := mounted.Lineage()
	if !authorityOK || authority == nil || !authority.Fence().Same(mounted.RuntimeFence()) || !authority.Validate(prior.Lineage()) || !authority.Validate(application.Lineage()) {
		return Observation{}, false
	}
	lineageValue, lineageOK := authority.Join(prior.Lineage(), application.Lineage())
	if !lineageOK || !lineageValue.Available() || !authority.Validate(lineageValue) {
		return Observation{}, false
	}
	mergedOutputs := make([]ObservationCell, 0, len(prior.outputs)+len(outputs))
	mergedOutputs = append(mergedOutputs, prior.outputs...)
	mergedOutputs = append(mergedOutputs, outputs...)
	return newObservation(prior.Outcome(), lineageValue, mergedOutputs)
}

func validateOutputCardinality(contract algebra.ObservationContract, observation Observation) bool {
	if !contract.Available() || !observation.Available() {
		return false
	}
	// Cardinality constrains retained proposal cells only. A non-publishing
	// terminal outcome is a valid parent observation with a closed, empty
	// output extent; applying ExactlyOne to it would erase NoSelection,
	// NoCandidate, or Refused instead of preserving the outcome. Opaque is a
	// publishing outcome and therefore follows the ordinary cardinality path.
	if !observation.Outcome().Code.Publishes() {
		return len(observation.outputs) == 0
	}
	outputs := observation.outputs
	counts := make(map[model.ColumnID]uint32, len(contract.Outputs()))
	for _, output := range outputs {
		if !output.Available() {
			return false
		}
		counts[output.Column]++
	}
	for _, output := range contract.Outputs() {
		if !cardinalityHolds(output.Cardinality(), counts[output.Column()]) {
			return false
		}
	}
	return true
}

func mountedSignatureOutput(mounted witness.Mounted, contract algebra.ObservationContract, output algebra.ObservationOutput) (signature.Output, bool) {
	bound, ok := mounted.Binding(contract.Operation())
	if !ok || bound == nil || !bound.Signature().Available() {
		return signature.Output{}, false
	}
	return bound.Signature().OutputFor(output.Destination().Relation(), output.Column())
}

func cardinalityHolds(cardinality model.Cardinality, count uint32) bool {
	if !cardinality.Available() {
		return false
	}
	switch cardinality.Kind() {
	case model.ExactlyOne:
		return count == 1
	case model.Optional:
		return count <= 1
	case model.BoundedMany:
		bound, ok := cardinality.Bound()
		return ok && count <= bound
	case model.CompleteDenominator:
		// CompleteDenominator is intentionally unavailable to observation
		// outputs. Its authority belongs to the operation signature and the
		// mounted ProposalBuffer, while observations retain only copied cells.
		return false
	default:
		return false
	}
}

func boundOutput(outputs []algebra.ObservationOutput, column model.ColumnID) (algebra.ObservationOutput, bool) {
	for _, output := range outputs {
		if output.Column() == column {
			return output, true
		}
	}
	return algebra.ObservationOutput{}, false
}

func canonicalMembers(snapshot *canonical.Snapshot, plan canonical.QueryPlan[RowKey, Observation]) []RowKey {
	count, ok := canonical.MemberCountAtAxis(snapshot, plan.Axis())
	if !ok {
		return nil
	}
	result := make([]RowKey, count)
	for index := 0; index < count; index++ {
		key, keyOK := canonical.MemberAtAxis(snapshot, plan.Axis(), index)
		if !keyOK {
			return nil
		}
		result[index] = key
	}
	return result
}

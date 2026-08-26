package binding

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Proposal is one authenticated destination row/column and encoded opaque
// value. Generated adapters create the value token; this layer never decodes it.
type Proposal struct {
	destination CellToken
	value       ValueToken
	presence    model.Presence
	remove      bool
}

func NewProposal(destination CellToken, value ValueToken, presence model.Presence) (Proposal, bool) {
	if !destination.Available() || !presence.Available() || presence.Is(model.Refused) || !valueMatches(value, presence, destination.Fence()) {
		return Proposal{}, false
	}
	return Proposal{destination: destination, value: value, presence: presence}, true
}

// NewRemovalProposal issues an owner-authorized sparse removal for one
// destination. Removal is an operation bit on Proposal, not a fabricated
// ProvenAbsent presence or a parallel proposal type.
func NewRemovalProposal(destination CellToken) (Proposal, bool) {
	if !destination.Available() {
		return Proposal{}, false
	}
	return Proposal{destination: destination, remove: true}, true
}

func (proposal Proposal) Available() bool {
	if proposal.remove {
		return proposal.destination.Available() && !proposal.value.Available() && !proposal.presence.Available()
	}
	return proposal.destination.Available() && proposal.presence.Available() && !proposal.presence.Is(model.Refused) && valueMatches(proposal.value, proposal.presence, proposal.destination.Fence())
}

func (proposal Proposal) Destination() CellToken   { return proposal.destination }
func (proposal Proposal) Value() ValueToken        { return proposal.value }
func (proposal Proposal) Presence() model.Presence { return proposal.presence }
func (proposal Proposal) Removal() bool            { return proposal.remove }

// ProposalBatch is a lease over the buffer's preallocated proposal storage.
// Reset on the buffer invalidates the lease after the consumer is finished.
type ProposalBatch struct {
	buffer    *ProposalBuffer
	lease     uint64
	result    outcome.Result
	proposals []Proposal
}

func (batch ProposalBatch) Available() bool {
	return batch.buffer != nil && batch.buffer.lease == batch.lease && batch.buffer.closed && !batch.buffer.failed && batch.result.Available() && batch.proposals != nil
}

func (batch ProposalBatch) Outcome() outcome.Result {
	if !batch.Available() {
		return outcome.Result{}
	}
	return batch.result
}

// Signature returns the exact sealed operation contract that owns this
// proposal lease.  A batch never manufactures an operation identity from a
// destination; it redeems the signature retained by its proposal buffer.
func (batch ProposalBatch) Signature() signature.Signature {
	if !batch.Available() || batch.buffer == nil {
		return signature.Signature{}
	}
	return batch.buffer.operation
}

// Operation returns the exact schema operation that sealed this proposal
// lease.  Consumers that carry an application identity alongside the lease
// use this to prove the two authorities are the same operation.
func (batch ProposalBatch) Operation() signature.Identity {
	operation := batch.Signature()
	if !operation.Available() {
		return signature.Identity{}
	}
	return operation.Identity()
}

// Fence returns the runtime fence that authenticated this proposal lease.
func (batch ProposalBatch) Fence() Fence {
	if !batch.Available() || batch.buffer == nil {
		return Fence{}
	}
	return batch.buffer.fence
}

// Scope returns the exact invocation scope bound to this proposal lease.
func (batch ProposalBatch) Scope() ScopeToken {
	if !batch.Available() || batch.buffer == nil {
		return ScopeToken{}
	}
	return batch.buffer.scope
}
func (batch ProposalBatch) Len() int {
	if !batch.Available() {
		return 0
	}
	return len(batch.proposals)
}

func (batch ProposalBatch) At(index int) (Proposal, bool) {
	if !batch.Available() || index < 0 || index >= len(batch.proposals) {
		return Proposal{}, false
	}
	return batch.proposals[index], true
}

// ProposalBuffer is a signature/runtime-fence-bound fixed-capacity staging
// area. Its storage is allocated once and reused across worker calls.
type ProposalBuffer struct {
	operation            signature.Signature
	fence                Fence
	destinationWitnesses map[model.DenominatorRef]DenominatorWitness
	destination          DestinationView
	scope                ScopeToken
	staged               []Proposal
	failed               bool
	closed               bool
	lease                uint64
}

// NewProposalBuffer binds every declared output denominator to its exact
// owner-issued witness. The witness set is explicit: no operation-wide
// destination or inferred fallback can authorize a proposal.
func NewProposalBuffer(operation signature.Signature, fence Fence, witnesses []DenominatorWitness, scope ScopeToken, destination DestinationView) (ProposalBuffer, bool) {
	if !operation.Available() || !fence.Available() || fence.Schema() != operation.Fence().Schema || !scope.ValidFor(fence) || !destination.Available() {
		return ProposalBuffer{}, false
	}
	completeDenominator, complete := model.DenominatorRef{}, operation.Cardinality().Kind() == model.CompleteDenominator
	if complete {
		var completeOK bool
		completeDenominator, completeOK = completeOutputDenominator(operation)
		if !completeOK {
			return ProposalBuffer{}, false
		}
		if destination.IsOwnerNamed() {
			ownerRelation, ownerOK := destination.OwnerRelation()
			if !ownerOK || ownerRelation != completeDenominator.Relation() {
				return ProposalBuffer{}, false
			}
		}
	}
	byDenominator := make(map[model.DenominatorRef]DenominatorWitness, len(witnesses))
	for _, candidate := range witnesses {
		if !candidate.ValidFor(fence) {
			return ProposalBuffer{}, false
		}
		ref := model.DenominatorRef{}
		for _, output := range operation.Outputs() {
			if candidate.Matches(output.Denominator) {
				ref = output.Denominator
				break
			}
		}
		if !ref.Available() {
			return ProposalBuffer{}, false
		}
		if prior, exists := byDenominator[ref]; exists && !prior.Same(candidate) {
			return ProposalBuffer{}, false
		}
		byDenominator[ref] = candidate
	}
	for _, output := range operation.Outputs() {
		candidate, exists := byDenominator[output.Denominator]
		if !exists || !candidate.Matches(output.Denominator) {
			return ProposalBuffer{}, false
		}
	}
	maximum, ok := maximumOutputs(operation)
	if complete {
		witness, witnessOK := byDenominator[completeDenominator]
		if !witnessOK {
			return ProposalBuffer{}, false
		}
		maximum, ok = completeMaximumOutputs(operation, witness)
	}
	if !ok {
		return ProposalBuffer{}, false
	}
	return ProposalBuffer{operation: operation, fence: fence, destinationWitnesses: byDenominator, destination: destination, scope: scope, staged: make([]Proposal, 0, maximum), lease: 1}, true
}

// completeOutputDenominator validates the closed shape needed by
// CompleteDenominator. The operation must expose at least one distinct output
// column, and every output must name the same exact denominator. Keeping this
// check beside the buffer's capacity calculation makes malformed unchecked
// signatures fail closed even when a caller bypasses the schema checkers.
func completeOutputDenominator(operation signature.Signature) (model.DenominatorRef, bool) {
	if !operation.Available() || operation.Cardinality().Kind() != model.CompleteDenominator {
		return model.DenominatorRef{}, false
	}
	outputs := operation.Outputs()
	if len(outputs) == 0 {
		return model.DenominatorRef{}, false
	}
	first := outputs[0]
	if !first.Available() || !first.Denominator.Available() || first.Denominator.Relation() != first.Relation {
		return model.DenominatorRef{}, false
	}
	denominator := first.Denominator
	type outputKey struct {
		relation model.RelationID
		column   model.ColumnID
	}
	seen := make(map[outputKey]struct{}, len(outputs))
	for _, output := range outputs {
		if !output.Available() || output.Denominator != denominator || output.Relation != denominator.Relation() {
			return model.DenominatorRef{}, false
		}
		key := outputKey{relation: output.Relation, column: output.Column}
		if _, exists := seen[key]; exists {
			return model.DenominatorRef{}, false
		}
		seen[key] = struct{}{}
	}
	return denominator, true
}

func completeMaximumOutputs(operation signature.Signature, witness DenominatorWitness) (int, bool) {
	denominator, ok := completeOutputDenominator(operation)
	if !ok || !witness.Available() || !witness.Matches(denominator) {
		return 0, false
	}
	columns := operation.OutputLen()
	rows := witness.Len()
	if columns == 0 || rows == 0 {
		return 0, true
	}
	if uint64(rows) > uint64(^uint(0)>>1)/uint64(columns) {
		return 0, false
	}
	return rows * columns, true
}

func maximumOutputs(operation signature.Signature) (int, bool) {
	cardinality := operation.Cardinality()
	if !cardinality.Available() {
		return 0, false
	}
	columns := operation.OutputLen()
	if columns == 0 {
		return 0, true
	}
	rows, ok := rowLimit(cardinality)
	if !ok || uint64(rows) > uint64(^uint(0)>>1)/uint64(columns) {
		return 0, false
	}
	return int(rows) * columns, true
}

func rowLimit(cardinality model.Cardinality) (uint32, bool) {
	if !cardinality.Available() {
		return 0, false
	}
	switch cardinality.Kind() {
	case model.ExactlyOne, model.Optional:
		return 1, true
	case model.BoundedMany:
		return cardinality.Bound()
	default:
		return 0, false
	}
}

func (buffer *ProposalBuffer) Available() bool {
	return buffer != nil && buffer.operation.Available() && buffer.fence.Available() && buffer.destination.Available() && buffer.staged != nil && !buffer.closed
}

func (buffer *ProposalBuffer) Signature() signature.Signature {
	if buffer == nil {
		return signature.Signature{}
	}
	return buffer.operation
}

func (buffer *ProposalBuffer) Fence() Fence {
	if buffer == nil {
		return Fence{}
	}
	return buffer.fence
}

// DestinationWitness returns the exact mounted witness required by one
// sealed output denominator.
func (buffer *ProposalBuffer) DestinationWitness(ref model.DenominatorRef) (DenominatorWitness, bool) {
	if buffer == nil || buffer.destinationWitnesses == nil {
		return DenominatorWitness{}, false
	}
	witness, ok := buffer.destinationWitnesses[ref]
	return witness, ok
}

func (buffer *ProposalBuffer) Scope() ScopeToken {
	if buffer == nil {
		return ScopeToken{}
	}
	return buffer.scope
}

// Destination returns the borrowed plan-resolved output row view. The view is
// copied by value; its sequential cursor is owned by the generated Emitter,
// not by callers of the buffer.
func (buffer *ProposalBuffer) Destination() DestinationView {
	if buffer == nil {
		return DestinationView{}
	}
	return buffer.destination
}

func (buffer *ProposalBuffer) Len() int {
	if buffer == nil || buffer.failed {
		return 0
	}
	return len(buffer.staged)
}

func (buffer *ProposalBuffer) Append(proposal Proposal) bool {
	if !buffer.Available() || buffer.failed || !proposal.Available() || !buffer.authorized(proposal) || !buffer.withinRowBound(proposal) || len(buffer.staged) == cap(buffer.staged) {
		if buffer != nil {
			buffer.failed = true
			if buffer.staged != nil {
				buffer.staged = buffer.staged[:0]
			}
		}
		return false
	}
	for _, prior := range buffer.staged {
		if prior.Destination().Same(proposal.Destination()) {
			buffer.failed = true
			buffer.staged = buffer.staged[:0]
			return false
		}
	}
	buffer.staged = append(buffer.staged, proposal)
	return true
}

func (buffer *ProposalBuffer) withinRowBound(proposal Proposal) bool {
	if buffer == nil {
		return false
	}
	if buffer.operation.Cardinality().Kind() == model.CompleteDenominator {
		// CompleteDenominator is bounded by the mounted witness-backed cell
		// capacity and closed by exact row/column coverage at Seal. It has no
		// static row limit to consult here.
		return true
	}
	limit, ok := rowLimit(buffer.operation.Cardinality())
	if !ok {
		return false
	}
	row := proposal.Destination().Row()
	rows := uint32(0)
	for index, prior := range buffer.staged {
		if prior.Destination().Row() == row {
			return true
		}
		seen := false
		for _, earlier := range buffer.staged[:index] {
			if earlier.Destination().Row() == prior.Destination().Row() {
				seen = true
				break
			}
		}
		if !seen {
			rows++
		}
	}
	return rows < limit
}

func (buffer *ProposalBuffer) authorized(proposal Proposal) bool {
	if buffer == nil || !proposal.Available() || !proposal.Destination().ValidFor(buffer.fence) || !proposal.Destination().Scope().Same(buffer.scope) {
		return false
	}
	if !buffer.operation.AllowsDestination(proposal.Destination().Relation(), proposal.Destination().Column()) {
		return false
	}
	output, ok := buffer.operation.OutputFor(proposal.Destination().Relation(), proposal.Destination().Column())
	if !ok || !output.Denominator.Available() {
		return false
	}
	witness, witnessOK := buffer.DestinationWitness(output.Denominator)
	if !witnessOK || !proposal.Destination().Witness().Same(witness) || !witness.Contains(proposal.Destination().Row()) {
		return false
	}
	if proposal.Removal() {
		return true
	}
	if !output.Presence.Allows(proposal.Presence()) || !valueMatches(proposal.Value(), proposal.Presence(), buffer.fence) {
		return false
	}
	return !proposal.Value().Available() || proposal.Value().Type() == output.Type
}

func (buffer *ProposalBuffer) Seal(result outcome.Result) (ProposalBatch, bool) {
	if !buffer.Available() || buffer.failed || !result.Available() || !buffer.operation.Allows(result.Code) {
		return ProposalBatch{}, false
	}
	buffer.closed = true
	if !result.Code.Publishes() {
		if len(buffer.staged) != 0 {
			buffer.staged = buffer.staged[:0]
			return ProposalBatch{}, false
		}
		return ProposalBatch{buffer: buffer, lease: buffer.lease, result: result, proposals: buffer.staged}, true
	}
	if buffer.operation.Cardinality().Kind() == model.CompleteDenominator {
		if !buffer.completeProduced() {
			buffer.staged = buffer.staged[:0]
			return ProposalBatch{}, false
		}
		return ProposalBatch{buffer: buffer, lease: buffer.lease, result: result, proposals: buffer.staged}, true
	}
	maximum, ok := maximumOutputs(buffer.operation)
	if !ok || len(buffer.staged) > maximum {
		buffer.staged = buffer.staged[:0]
		return ProposalBatch{}, false
	}
	return ProposalBatch{buffer: buffer, lease: buffer.lease, result: result, proposals: buffer.staged}, true
}

// completeProduced proves totality over the exact mounted denominator. Every
// witness row and every declared output column must occur once; a zero-row
// witness therefore admits an empty Produced batch naturally.
func (buffer *ProposalBuffer) completeProduced() bool {
	if buffer == nil || buffer.operation.Cardinality().Kind() != model.CompleteDenominator {
		return false
	}
	denominator, ok := completeOutputDenominator(buffer.operation)
	if !ok {
		return false
	}
	witness, ok := buffer.DestinationWitness(denominator)
	if !ok || !witness.ValidFor(buffer.fence) {
		return false
	}
	expected, ok := completeMaximumOutputs(buffer.operation, witness)
	if !ok || len(buffer.staged) != expected {
		return false
	}
	type proposalKey struct {
		relation model.RelationID
		column   model.ColumnID
		row      model.RowID
	}
	required := make(map[proposalKey]struct{}, expected)
	for index := 0; index < witness.Len(); index++ {
		row, rowOK := witness.At(index)
		if !rowOK || !witness.Contains(row) {
			return false
		}
		for _, output := range buffer.operation.Outputs() {
			required[proposalKey{relation: output.Relation, column: output.Column, row: row}] = struct{}{}
		}
	}
	if len(required) != expected {
		return false
	}
	for _, proposal := range buffer.staged {
		destination := proposal.Destination()
		key := proposalKey{relation: destination.Relation(), column: destination.Column(), row: destination.Row()}
		if _, exists := required[key]; !exists {
			return false
		}
		delete(required, key)
	}
	return len(required) == 0
}

func (buffer *ProposalBuffer) Abandon() {
	if buffer == nil {
		return
	}
	buffer.failed = true
	buffer.closed = true
	buffer.staged = buffer.staged[:0]
}

// Reset invalidates the current batch lease and reuses the backing storage.
func (buffer *ProposalBuffer) Reset() bool {
	if buffer == nil || buffer.staged == nil || (!buffer.closed && !buffer.failed) {
		return false
	}
	buffer.staged = buffer.staged[:0]
	buffer.failed = false
	buffer.closed = false
	buffer.lease++
	if buffer.lease == 0 {
		buffer.lease = 1
	}
	return true
}

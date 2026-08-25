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
}

func NewProposal(destination CellToken, value ValueToken, presence model.Presence) (Proposal, bool) {
	if !destination.Available() || !presence.Available() || presence.Is(model.Refused) || !valueMatches(value, presence, destination.Fence()) {
		return Proposal{}, false
	}
	return Proposal{destination: destination, value: value, presence: presence}, true
}

func (proposal Proposal) Available() bool {
	return proposal.destination.Available() && proposal.presence.Available() && !proposal.presence.Is(model.Refused) && valueMatches(proposal.value, proposal.presence, proposal.destination.Fence())
}

func (proposal Proposal) Destination() CellToken   { return proposal.destination }
func (proposal Proposal) Value() ValueToken        { return proposal.value }
func (proposal Proposal) Presence() model.Presence { return proposal.presence }

// ProposalBatch is a lease over the buffer's preallocated proposal storage.
// Reset on the buffer invalidates the lease after the consumer is finished.
type ProposalBatch struct {
	buffer    *ProposalBuffer
	lease     uint64
	result    outcome.Result
	proposals []Proposal
}

func (batch ProposalBatch) Available() bool {
	return batch.buffer != nil && batch.buffer.lease == batch.lease && batch.buffer.closed && batch.result.Available() && batch.proposals != nil
}

func (batch ProposalBatch) Outcome() outcome.Result {
	if !batch.Available() {
		return outcome.Result{}
	}
	return batch.result
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
	operation signature.Signature
	fence     Fence
	witness   DenominatorWitness
	scope     ScopeToken
	staged    []Proposal
	failed    bool
	closed    bool
	lease     uint64
}

func NewProposalBuffer(operation signature.Signature, fence Fence, witness DenominatorWitness, scope ScopeToken) (ProposalBuffer, bool) {
	if !operation.Available() || !fence.Available() || fence.Schema() != operation.Fence().Schema || !witness.ValidFor(fence) || !witness.Matches(operation.Authority().Denominator) || !scope.ValidFor(fence) {
		return ProposalBuffer{}, false
	}
	maximum, ok := maximumOutputs(operation)
	if !ok {
		return ProposalBuffer{}, false
	}
	return ProposalBuffer{operation: operation, fence: fence, witness: witness, scope: scope, staged: make([]Proposal, 0, maximum), lease: 1}, true
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
	return buffer != nil && buffer.operation.Available() && buffer.fence.Available() && buffer.staged != nil && !buffer.closed
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

func (buffer *ProposalBuffer) Witness() DenominatorWitness {
	if buffer == nil {
		return DenominatorWitness{}
	}
	return buffer.witness
}

func (buffer *ProposalBuffer) Scope() ScopeToken {
	if buffer == nil {
		return ScopeToken{}
	}
	return buffer.scope
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
	if buffer == nil || !proposal.Available() || !proposal.Destination().ValidFor(buffer.fence) || !proposal.Destination().Witness().Same(buffer.witness) || !proposal.Destination().Scope().Same(buffer.scope) || !valueMatches(proposal.Value(), proposal.Presence(), buffer.fence) {
		return false
	}
	if !buffer.operation.AllowsDestination(proposal.Destination().Relation(), proposal.Destination().Column()) {
		return false
	}
	output, ok := buffer.operation.OutputFor(proposal.Destination().Relation(), proposal.Destination().Column())
	if !ok || !output.Presence.Allows(proposal.Presence()) {
		return false
	}
	return !proposal.Value().Available() || proposal.Value().Type() == output.Type
}

func (buffer *ProposalBuffer) Seal(result outcome.Result) (ProposalBatch, bool) {
	if !buffer.Available() || buffer.failed || !result.Available() || !buffer.operation.Allows(result.Code) {
		return ProposalBatch{}, false
	}
	buffer.closed = true
	if result.Code != outcome.Produced {
		if len(buffer.staged) != 0 {
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

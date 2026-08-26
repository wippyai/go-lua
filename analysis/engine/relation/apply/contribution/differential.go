package contribution

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
	semanticSignature "github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// TransitionsForDifferential classifies the signed Before/After proposal
// leases retained by one Apply differential.
//
// A proposal is a contribution proposal only when the mounted arrangement
// redeems its exact operation and CellToken through ContributionCell.  The
// proposal's position in either lease, its column name, and a reconstructed
// OutputPort are not classification keys.  Before and After are first
// collected independently and then joined by the sealed descriptor and the
// exact CellToken.  Thus a common key becomes one atomic replacement, while a
// missing key becomes a one-sided transition.  A destination move is the
// intentional pair of an old removal and a new insertion.
//
// Ordinary proposals are ignored.  A malformed semantic proposal batch is
// not ordinary transport, however: removal proposals, foreign fences,
// operation identities, invocation scopes, destinations, and duplicate keys
// refuse the whole classification.
func TransitionsForDifferential(mounted witness.Mounted, value differential.Differential) ([]invocation.ContributionTransition, bool) {
	if !mounted.Available() || !value.Available() {
		return nil, false
	}
	fence := mounted.RuntimeFence()
	if !fence.Available() || !value.Fence().Same(fence) || !value.Invocation().ValidFor(fence) {
		return nil, false
	}
	operation := value.Operation()
	if !operation.Available() {
		return nil, false
	}

	// A differential carrying an operation not admitted by this mount must not
	// quietly turn into an empty ordinary projection.  Binding is the mounted
	// operation authority; its exact signature also supplies the denominator
	// witness check used below for every destination cell.
	bound, ok := mounted.Binding(operation)
	if !ok || bound == nil {
		return nil, false
	}
	signature := bound.Signature()
	if !signature.Available() || signature.Identity() != operation {
		return nil, false
	}
	plan := mounted.Arrangement()
	if !plan.Available() || !plan.Fence().Same(mounted.Fence()) {
		return nil, false
	}

	before, beforeOK := value.Before()
	after, afterOK := value.After()
	if !beforeOK && !afterOK {
		return nil, false
	}
	if beforeOK && !validApplicationEnvelope(before, value, fence) {
		return nil, false
	}
	if afterOK && !validApplicationEnvelope(after, value, fence) {
		return nil, false
	}

	beforeRecords, ok := collectContributionRecords(mounted, plan, signature, value, before, beforeOK)
	if !ok {
		return nil, false
	}
	afterRecords, ok := collectContributionRecords(mounted, plan, signature, value, after, afterOK)
	if !ok {
		return nil, false
	}

	// Canonical presentation order is useful to callers and makes the result
	// independent of the physical proposal order.  Pairing itself below still
	// uses exact equality, never this order.
	sortRecords(beforeRecords)
	sortRecords(afterRecords)

	result := make([]invocation.ContributionTransition, 0, len(beforeRecords)+len(afterRecords))
	matchedAfter := make([]bool, len(afterRecords))
	for _, prior := range beforeRecords {
		match := -1
		for index, successor := range afterRecords {
			if matchedAfter[index] || !sameRecord(prior, successor) {
				continue
			}
			match = index
			break
		}
		if match < 0 {
			transition, transitionOK := makeContributionTransition(prior, value.Invocation(), fence, prior.side, binding.NoContributionSide())
			if !transitionOK {
				return nil, false
			}
			result = append(result, transition)
			continue
		}
		matchedAfter[match] = true
		successor := afterRecords[match]
		transition, transitionOK := makeContributionTransition(prior, value.Invocation(), fence, prior.side, successor.side)
		if !transitionOK {
			return nil, false
		}
		result = append(result, transition)
	}
	for index, successor := range afterRecords {
		if matchedAfter[index] {
			continue
		}
		transition, transitionOK := makeContributionTransition(successor, value.Invocation(), fence, binding.NoContributionSide(), successor.side)
		if !transitionOK {
			return nil, false
		}
		result = append(result, transition)
	}
	return result, true
}

// contributionRecord is the side-local authenticated payload together with
// the descriptor and destination that redeemed it.  Keeping the descriptor
// object (rather than just its port/column) prevents a later projection from
// becoming a second schema authority.
type contributionRecord struct {
	descriptor arrangement.ContributionCell
	cell       binding.CellToken
	side       binding.ContributionSide
}

func validApplicationEnvelope(application apply.Application, value differential.Differential, fence binding.Fence) bool {
	if !application.Available() || !application.Fence().Same(fence) || application.Operation() != value.Operation() || !application.Invocation().ValidFor(fence) || !application.Invocation().Same(value.Invocation()) {
		return false
	}
	proposals, hasProposals := application.Proposals()
	if !hasProposals {
		// Refused has no proposal lease by construction.  Other valid terminal
		// outcomes are not expected to carry one either, but Application owns
		// that distinction and has already authenticated it.
		return application.Outcome().Available()
	}
	if !proposals.Available() || proposals.Operation() != value.Operation() || !proposals.Fence().Same(fence) || proposals.Outcome() != application.Outcome() || !proposals.Scope().Same(value.Invocation().Scope()) {
		return false
	}
	// Proposal leases are empty for every terminal outcome other than
	// A nonempty reflect-forged side must not be promoted into a signed
	// contribution event. Both Produced and Opaque are publishing outcomes;
	// the shared predicate is the semantic authority for that distinction.
	return proposals.Len() == 0 || application.Outcome().Code.Publishes()
}

func collectContributionRecords(
	mounted witness.Mounted,
	plan arrangement.Plan,
	signature semanticSignature.Signature,
	value differential.Differential,
	application apply.Application,
	present bool,
) ([]contributionRecord, bool) {
	if !present {
		return []contributionRecord{}, true
	}
	proposals, hasProposals := application.Proposals()
	if !hasProposals {
		return []contributionRecord{}, true
	}
	if !proposals.Available() {
		return nil, false
	}
	result := make([]contributionRecord, 0, proposals.Len())
	for index := 0; index < proposals.Len(); index++ {
		proposal, ok := proposals.At(index)
		if !ok || !proposal.Available() || proposal.Removal() {
			return nil, false
		}
		cell := proposal.Destination()
		if !validDestination(mounted, signature, value, cell) {
			return nil, false
		}
		descriptor, declared := plan.ContributionCell(value.Operation(), cell)
		if !declared {
			// A valid, mounted ordinary proposal is deliberately outside this
			// classifier.  validDestination has already rejected foreign cells.
			continue
		}
		if !descriptor.Available() || !descriptor.ValidFor(plan.Fence()) || descriptor.Operation() != value.Operation() || descriptor.Column() != cell.Column() {
			return nil, false
		}
		spec := descriptor.Spec()
		if !spec.Available() || spec.Port().Operation != value.Operation() || spec.Column() != cell.Column() || !spec.Presence().Allows(proposal.Presence()) {
			return nil, false
		}
		side, ok := binding.NewContributionSide(proposal.Value(), proposal.Presence(), application.Lineage())
		if !ok || !side.ValidFor(value.Fence()) {
			return nil, false
		}
		candidate := contributionRecord{descriptor: descriptor, cell: cell, side: side}
		for _, prior := range result {
			if sameRecord(prior, candidate) {
				// ProposalBuffer normally catches this before sealing.  Keep the
				// classifier hostile to a forged or reflect-mutated lease too.
				return nil, false
			}
		}
		result = append(result, candidate)
	}
	return result, true
}

// validDestination redeems the whole mounted destination envelope.  A
// CellToken's runtime fence alone is insufficient: a foreign denominator
// witness can share that fence, and the invocation scope is part of the
// address authority.  The mounted signature and denominator directory prove
// both before ContributionCell gets a chance to classify the cell.
func validDestination(
	mounted witness.Mounted,
	signature semanticSignature.Signature,
	value differential.Differential,
	cell binding.CellToken,
) bool {
	if !cell.Available() || !cell.ValidFor(value.Fence()) || !cell.Scope().Same(value.Invocation().Scope()) {
		return false
	}
	declared, ok := signature.OutputFor(cell.Relation(), cell.Column())
	if !ok || !declared.Available() || !declared.Denominator.Available() {
		return false
	}
	witnessValue, witnessOK := mounted.Denominator(declared.Denominator)
	return witnessOK && witnessValue.Same(cell.Witness())
}

func sameRecord(left, right contributionRecord) bool {
	return left.descriptor.Available() && right.descriptor.Available() && left.descriptor.Operation() == right.descriptor.Operation() && left.descriptor.Column() == right.descriptor.Column() && left.descriptor.Spec().Equal(right.descriptor.Spec()) && left.cell.Same(right.cell)
}

func makeContributionTransition(record contributionRecord, address invocation.InvocationAddress, fence binding.Fence, before, after binding.ContributionSide) (invocation.ContributionTransition, bool) {
	return invocation.NewContributionTransition(record.descriptor.Spec(), address, record.cell, fence, before, after)
}

func sortRecords(values []contributionRecord) {
	sort.SliceStable(values, func(left, right int) bool {
		return compareRecords(values[left], values[right]) < 0
	})
}

func compareRecords(left, right contributionRecord) int {
	leftDigest, rightDigest := left.descriptor.Spec().Digest(), right.descriptor.Spec().Digest()
	if compared := bytes.Compare(leftDigest[:], rightDigest[:]); compared != 0 {
		return compared
	}
	return compareCells(left.cell, right.cell)
}

func compareCells(left, right binding.CellToken) int {
	if compared := compareRelation(left.Relation(), right.Relation()); compared != 0 {
		return compared
	}
	if compared := compareColumn(left.Column(), right.Column()); compared != 0 {
		return compared
	}
	if compared := compareRow(left.Row(), right.Row()); compared != 0 {
		return compared
	}
	if compared := binding.CompareScope(left.Scope(), right.Scope()); compared != 0 {
		return compared
	}
	leftWitness, rightWitness := left.Witness(), right.Witness()
	if leftWitness.Relation() != rightWitness.Relation() {
		return compareRelation(leftWitness.Relation(), rightWitness.Relation())
	}
	if leftWitness.Key() != rightWitness.Key() {
		return compareKey(leftWitness.Key(), rightWitness.Key())
	}
	leftEvidence, leftOK := leftWitness.Evidence()
	rightEvidence, rightOK := rightWitness.Evidence()
	if leftOK != rightOK {
		if !leftOK {
			return -1
		}
		return 1
	}
	if leftOK {
		return bytes.Compare(leftEvidence[:], rightEvidence[:])
	}
	return 0
}

func compareRelation(left, right model.RelationID) int {
	if left.Owner() != right.Owner() {
		leftOwner, rightOwner := left.Owner().Content(), right.Owner().Content()
		if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
			return compared
		}
	}
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:])
}

func compareColumn(left, right model.ColumnID) int {
	if compared := compareRelation(left.Relation(), right.Relation()); compared != 0 {
		return compared
	}
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:])
}

func compareKey(left, right model.KeyID) int {
	if compared := compareRelation(left.Relation(), right.Relation()); compared != 0 {
		return compared
	}
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:])
}

func compareRow(left, right model.RowID) int {
	if compared := compareRelation(left.Relation(), right.Relation()); compared != 0 {
		return compared
	}
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:])
}

package transaction

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/contribution"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/contribution/reduction"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// SubmissionBatch is the sealed state-facing result of one semantic
// application. One application has one exact operation and invocation
// address, so the transaction accepts one lineage authority and applies it
// to every proposal in the live batch. The operation/address are retained in
// unexported fields and can only be installed by NewSubmissionBatch; callers
// cannot provide a second public identity sidecar. Widening is one opaque,
// mount-issued permit for the whole invocation. A stale/reset ProposalBatch is
// rejected; its copied values are never accepted as a new authority.
// Contributions is the authenticated schema-declared subset of the same
// proposal lease; each item must match one proposal exactly and is removed
// from ordinary aggregate admission.
type SubmissionBatch struct {
	proposals        binding.ProposalBatch
	lineage          model.LineageRef
	widening         witness.WideningPermit
	contributions    []invocation.ContributionTransition
	operation        signature.Identity
	address          invocation.InvocationAddress
	differential     applydifferential.Differential
	differentialMode bool
	sealed           bool
}

// NewSubmissionBatch seals the exact application transport consumed by
// Prepare. The operation must agree with the signature that owns the proposal
// lease; the invocation address must be from the same runtime fence and
// scope. Every contribution transition is checked against both identities at
// construction time and checked again against the mounted plan during
// admission. The transition slice is copied so callers cannot mutate the
// authenticated subset after construction.
func NewSubmissionBatch(
	application apply.Application,
	widening witness.WideningPermit,
	contributions []invocation.ContributionTransition,
) (SubmissionBatch, bool) {
	if !application.Available() {
		return SubmissionBatch{}, false
	}
	operation := application.Operation()
	address := application.Invocation()
	lineageValue := application.Lineage()
	proposals, proposalsOK := application.Proposals()
	if !proposalsOK {
		return SubmissionBatch{}, false
	}
	if !operation.Available() || !proposals.Available() || proposals.Operation() != operation {
		return SubmissionBatch{}, false
	}
	fence := proposals.Fence()
	if !fence.Available() || !address.ValidFor(fence) || !address.Scope().Same(proposals.Scope()) {
		return SubmissionBatch{}, false
	}
	if proposals.Len() != 0 && !lineageValue.Available() {
		return SubmissionBatch{}, false
	}
	owned := append([]invocation.ContributionTransition(nil), contributions...)
	for _, transition := range owned {
		if !transition.ValidFor(fence) || !transition.Address().Same(address) || transition.Spec().Port().Operation != operation {
			return SubmissionBatch{}, false
		}
	}
	result := SubmissionBatch{
		proposals:     proposals,
		lineage:       lineageValue,
		widening:      widening,
		contributions: owned,
		operation:     operation,
		address:       address,
		sealed:        true,
	}
	return result, result.Available()
}

// NewDifferentialSubmissionBatch seals a signed before/after application
// transport for the state transaction.  The Differential itself is retained
// rather than rebuilding either application from proposal cells: its exact
// proposal leases and each side's lineage remain the authorities redeemed by
// Prepare.  Ordinary writes are sourced exclusively from After; Before is
// consumed only when a contribution transition supplies an exact predecessor
// side.
func NewDifferentialSubmissionBatch(
	transport applydifferential.Differential,
	widening witness.WideningPermit,
	contributions []invocation.ContributionTransition,
) (SubmissionBatch, bool) {
	if !transport.Available() {
		return SubmissionBatch{}, false
	}
	operation := transport.Operation()
	address := transport.Invocation()
	fence := transport.Fence()
	if !operation.Available() || !address.ValidFor(fence) || !fence.Available() {
		return SubmissionBatch{}, false
	}

	// Redeem both retained sides now.  This checks that a present side really
	// owns a live ProposalBatch; an omitted side remains omitted and is never
	// represented by a fabricated empty lease.
	before, beforeOK := transport.Before()
	after, afterOK := transport.After()
	if !beforeOK && !afterOK {
		return SubmissionBatch{}, false
	}
	var afterProposals binding.ProposalBatch
	if beforeOK {
		beforeProposals, proposalsOK := before.Proposals()
		if !proposalsOK || !validApplicationLease(before, beforeProposals, operation, address, fence) {
			return SubmissionBatch{}, false
		}
	}
	if afterOK {
		var proposalsOK bool
		afterProposals, proposalsOK = after.Proposals()
		if !proposalsOK || !validApplicationLease(after, afterProposals, operation, address, fence) {
			return SubmissionBatch{}, false
		}
	}

	owned := append([]invocation.ContributionTransition(nil), contributions...)
	for _, transition := range owned {
		if !transition.ValidFor(fence) || !transition.Address().Same(address) || transition.Spec().Port().Operation != operation {
			return SubmissionBatch{}, false
		}
		if beforeSide, hasBefore := transition.Before(); hasBefore {
			if !beforeOK || beforeSide.Lineage() != before.Lineage() {
				return SubmissionBatch{}, false
			}
		}
		if afterSide, hasAfter := transition.After(); hasAfter {
			if !afterOK || afterSide.Lineage() != after.Lineage() {
				return SubmissionBatch{}, false
			}
		}
	}

	result := SubmissionBatch{
		// proposals/lineage are the After compatibility projection.  The
		// Differential remains the authoritative owner for both sides.
		proposals:        afterProposals,
		widening:         widening,
		contributions:    owned,
		operation:        operation,
		address:          address,
		differential:     transport,
		differentialMode: true,
		sealed:           true,
	}
	if afterOK {
		result.lineage = after.Lineage()
	}
	return result, result.Available()
}

func validApplicationLease(application apply.Application, proposals binding.ProposalBatch, operation signature.Identity, address invocation.InvocationAddress, fence binding.Fence) bool {
	return application.Available() && proposals.Available() && proposals.Operation() == operation && proposals.Fence().Same(fence) && address.ValidFor(fence) && address.Scope().Same(proposals.Scope()) && application.Operation() == operation && application.Fence().Same(fence) && application.Invocation().Same(address) && application.Lineage().Available()
}

// Available reports whether the batch still owns a live proposal lease and
// retains the exact operation/invocation authorities sealed at construction.
func (batch SubmissionBatch) Available() bool {
	if !batch.sealed || !batch.operation.Available() {
		return false
	}
	if batch.differentialMode {
		if !batch.differential.Available() || batch.differential.Operation() != batch.operation || !batch.differential.Invocation().Same(batch.address) {
			return false
		}
	} else if !batch.proposals.Available() || batch.proposals.Operation() != batch.operation {
		return false
	}
	fence := batch.operationFence()
	if !fence.Available() || !batch.address.ValidFor(fence) {
		return false
	}
	if batch.differentialMode {
		_, _, _, _, ok := batch.proposalSides()
		if !ok {
			return false
		}
	} else if !batch.address.Scope().Same(batch.proposals.Scope()) {
		return false
	}
	if !batch.afterLineageAvailable() {
		return false
	}
	for _, transition := range batch.contributions {
		if !transition.ValidFor(fence) || !transition.Address().Same(batch.address) || transition.Spec().Port().Operation != batch.operation {
			return false
		}
	}
	return true
}

// Len reports the number of live proposals carried by the sealed
// application transport.
func (batch SubmissionBatch) Len() int {
	if !batch.Available() {
		return 0
	}
	_, after, _, afterOK, ok := batch.proposalSides()
	if !ok || !afterOK {
		return 0
	}
	return after.Len()
}

// operationFence returns the exact fence carried by the retained proposal
// lease.  A Differential may omit After, so its common transport fence is the
// authority in that case.
func (batch SubmissionBatch) operationFence() binding.Fence {
	if batch.differentialMode {
		return batch.differential.Fence()
	}
	return batch.proposals.Fence()
}

// proposalSides exposes the exact retained leases without creating a missing
// side.  The bools identify presence of an application side, not whether its
// proposal vector happens to be empty.
func (batch SubmissionBatch) proposalSides() (binding.ProposalBatch, binding.ProposalBatch, bool, bool, bool) {
	if batch.differentialMode {
		if !batch.differential.Available() {
			return binding.ProposalBatch{}, binding.ProposalBatch{}, false, false, false
		}
		before, beforeOK := batch.differential.Before()
		after, afterOK := batch.differential.After()
		var beforeProposals, afterProposals binding.ProposalBatch
		if beforeOK {
			var proposalsOK bool
			beforeProposals, proposalsOK = before.Proposals()
			if !proposalsOK || !beforeProposals.Available() {
				return binding.ProposalBatch{}, binding.ProposalBatch{}, false, false, false
			}
		}
		if afterOK {
			var proposalsOK bool
			afterProposals, proposalsOK = after.Proposals()
			if !proposalsOK || !afterProposals.Available() {
				return binding.ProposalBatch{}, binding.ProposalBatch{}, false, false, false
			}
		}
		return beforeProposals, afterProposals, beforeOK, afterOK, true
	}
	if !batch.proposals.Available() {
		return binding.ProposalBatch{}, binding.ProposalBatch{}, false, false, false
	}
	return binding.ProposalBatch{}, batch.proposals, false, true, true
}

func (batch SubmissionBatch) afterLineageAvailable() bool {
	if batch.differentialMode {
		after, afterOK := batch.differential.After()
		if !afterOK {
			return true
		}
		return after.Lineage().Available()
	}
	return batch.proposals.Len() == 0 || batch.lineage.Available()
}

func (batch SubmissionBatch) afterLineageValue() (model.LineageRef, bool) {
	if batch.differentialMode {
		after, afterOK := batch.differential.After()
		if !afterOK {
			return model.LineageRef{}, false
		}
		lineageValue := after.Lineage()
		return lineageValue, lineageValue.Available()
	}
	return batch.lineage, batch.lineage.Available()
}

// Prepare validates and normalizes one proposal batch without publishing any
// database root. Each changed column is staged exactly once and the complete
// private candidate is handed to database.Commit, the sole publication door.
// Any invalid, foreign, stale, conflicting, or unauthorised input returns an
// unavailable candidate and publishes nothing.
func Prepare(
	base database.Version,
	view geometry.Geometry,
	readScratch *store.ReadScratch,
	batch SubmissionBatch,
) (database.Prepared, bool) {
	if !batch.Available() || !base.Available() {
		return database.Prepared{}, false
	}
	sourceBase := base.Store()
	if !sourceBase.Available() {
		return database.Prepared{}, false
	}
	_, afterProposals, _, afterOK, sidesOK := batch.proposalSides()
	if !sidesOK {
		return database.Prepared{}, false
	}
	afterLen := 0
	if afterOK {
		afterLen = afterProposals.Len()
	}
	// Empty input is an explicit no-op.  It shares the exact immutable root
	// and does not require manufacturing a database delta.
	if afterLen == 0 && len(batch.contributions) == 0 {
		// A widening permit is an invocation capability, not a free-standing
		// fact.  Without a live proposal to redeem it, accepting the permit
		// would let a caller launder recurrence evidence through a no-op.
		if batch.widening.Available() {
			return database.Prepared{}, false
		}
		prepared, ok := store.Prepare(sourceBase)
		if !ok {
			return database.Prepared{}, false
		}
		return database.Prepare(base, prepared, readScratch, base.ContributionDirectory(), base.ContributionState(), nil)
	}
	mounted := base.Mounted()
	lineageAuthority, lineageOK := mounted.Lineage()
	if !view.ValidFor(mounted) || !mounted.Available() || !lineageOK || lineageAuthority == nil || readScratch == nil || !readScratch.Available() || !view.Fence().Same(base.Fence()) || !lineageAuthority.Fence().Same(base.Fence()) || !lineageAuthority.Owner().Available() || !lineageAuthority.Identity().Available() || !admitWidening(mounted, batch.widening) {
		return database.Prepared{}, false
	}
	excluded, ok := matchContributionProposals(mounted, batch)
	if !ok {
		return database.Prepared{}, false
	}
	groups, managers, ok := resolveSubmissions(sourceBase, view, mounted, lineageAuthority, batch, excluded)
	if !ok {
		return database.Prepared{}, false
	}

	// Contribution roots and their aggregate projections are staged in the
	// same private transaction as ordinary column writes.  No contribution
	// root is visible until database.Commit redeems the complete candidate.
	nextDirectory := base.ContributionDirectory()
	nextContribution := base.ContributionState()
	var contributionDelta *contribution.Delta
	derived := make(map[model.ColumnID][]column.Update)
	if len(batch.contributions) != 0 {
		var applied contribution.Delta
		nextDirectory, nextContribution, applied, ok = contribution.ApplyTransitions(base.ContributionDirectory(), base.ContributionState(), batch.contributions)
		if !ok || !nextDirectory.Available() || !nextContribution.Available() || !applied.Available() {
			return database.Prepared{}, false
		}
		if applied.Changed() {
			contributionDelta = &applied
		}
		derived, ok = reduceContributionUpdates(sourceBase, view, mounted, applied)
		if !ok {
			return database.Prepared{}, false
		}
	}

	var work *support.Work
	if len(groups) != 0 {
		if len(managers) != 1 || managers[0] == nil {
			return database.Prepared{}, false
		}
		work = support.New(managers[0])
		if work == nil {
			return database.Prepared{}, false
		}
		defer work.Close()
	}
	updatesByColumn := make(map[model.ColumnID][]column.Update, len(groups)+len(derived))
	for _, columnID := range sourceBase.ColumnIDs() {
		keys, present := groups[columnID]
		if present {
			version, valid := sourceBase.Column(columnID)
			if !valid || len(managers) != 1 || version.Guards() != managers[0] {
				return database.Prepared{}, false
			}
			updates, valid := normalizeColumn(sourceBase, version, keys, mounted, lineageAuthority, batch.widening, readScratch, work)
			if !valid {
				return database.Prepared{}, false
			}
			updatesByColumn[columnID] = append(updatesByColumn[columnID], updates...)
		}
		if extra := derived[columnID]; len(extra) != 0 {
			updatesByColumn[columnID] = append(updatesByColumn[columnID], extra...)
		}
	}

	changes := make([]column.Delta, 0, len(updatesByColumn))
	for _, columnID := range sourceBase.ColumnIDs() {
		updates := updatesByColumn[columnID]
		if len(updates) == 0 {
			continue
		}
		version, valid := sourceBase.Column(columnID)
		if !valid {
			return database.Prepared{}, false
		}
		next, delta, valid := version.Next(updates...)
		if !valid || !next.Available() || !delta.Available() {
			return database.Prepared{}, false
		}
		if next.Same(version) {
			// A contribution transition can replace a producer below a surviving
			// higher sibling.  The producer root still advances, but its derived
			// aggregate is exactly unchanged.  Column.Next deliberately returns
			// the predecessor plus an empty delta in that case; do not turn that
			// semantic no-op into a fabricated column/index successor or reject
			// the enclosing contribution-only database transaction.
			if !delta.Empty() {
				return database.Prepared{}, false
			}
			continue
		}
		if !next.SuccessorOf(version) || delta.Empty() {
			return database.Prepared{}, false
		}
		changes = append(changes, delta)
	}
	if !batch.Available() {
		return database.Prepared{}, false
	}
	if len(changes) == 0 {
		prepared, ok := store.Prepare(sourceBase)
		if !ok {
			return database.Prepared{}, false
		}
		return database.Prepare(base, prepared, readScratch, nextDirectory, nextContribution, contributionDelta)
	}

	// Store.Prepare authenticates every column delta. Database.Prepare then
	// attaches every actual arrangement successor; database.Commit owns the
	// sole root publication.
	prepared, valid := store.Prepare(sourceBase, changes...)
	if !valid || !prepared.Available() {
		return database.Prepared{}, false
	}
	return database.Prepare(base, prepared, readScratch, nextDirectory, nextContribution, contributionDelta)
}

// admitWidening redeems the only widening authority accepted by a
// transaction. A zero permit means an ordinary join. A non-zero permit is
// accepted only when the exact mounted witness returns the same immutable
// dependency/relation/evidence triple; callers cannot substitute a boolean,
// dependency, or recurrence authority.
func admitWidening(mounted witness.Mounted, permit witness.WideningPermit) bool {
	if !permit.Available() {
		return true
	}
	if !mounted.Available() {
		return false
	}
	expected, ok := mounted.Widening(permit.Dependency(), permit.Relation())
	return ok && expected.Available() && expected == permit
}

// matchContributionProposals binds the signed contribution transport to the
// live proposal lease. A transition never creates a second write path: it
// must consume exactly one proposal with the same destination and exact After
// payload (or the operation-bit removal form), and it must belong to the
// exact application operation/address retained by the batch. Conversely,
// every proposal under a mounted contribution declaration must be consumed by
// one transition; an omitted transition is never allowed to fall through to
// ordinary aggregate admission. Duplicate destinations are refused rather
// than arbitrarily assigning one proposal to one owner.
func matchContributionProposals(mounted witness.Mounted, batch SubmissionBatch) (map[int]struct{}, bool) {
	excluded := make(map[int]struct{}, len(batch.contributions))
	if !mounted.Available() || !batch.Available() {
		return nil, false
	}
	beforeProposals, afterProposals, beforeOK, afterOK, sidesOK := batch.proposalSides()
	if !sidesOK {
		return nil, false
	}
	plan := mounted.Arrangement()
	if !plan.Available() {
		return nil, false
	}
	for _, transition := range batch.contributions {
		if !transition.ValidFor(mounted.RuntimeFence()) || !transition.Address().Same(batch.address) || transition.Spec().Port().Operation != batch.operation {
			return nil, false
		}
		port := transition.Port()
		declared, declaredOK := plan.Contribution(port)
		if !declaredOK || !declared.Equal(transition.Spec()) {
			return nil, false
		}
		before, hasBefore := transition.Before()
		if hasBefore {
			// The application-owned constructor predates Differential and
			// carries one proposal lease for the After/write side; its Before
			// payload is already authenticated by the contribution state
			// transition.  Only a Differential has a retained Before lease
			// that must be matched here.
			if batch.differentialMode && (!beforeOK || !matchContributionSide(beforeProposals, transition.Destination(), before)) {
				return nil, false
			}
		}
		after, hasAfter := transition.After()
		if hasAfter {
			if !afterOK {
				return nil, false
			}
			candidate, matched := matchContributionSideIndex(afterProposals, transition.Destination(), after)
			if !matched {
				return nil, false
			}
			if _, duplicate := excluded[candidate]; duplicate {
				return nil, false
			}
			excluded[candidate] = struct{}{}
		} else if !batch.differentialMode {
			// Preserve the existing application-owned removal spelling: a
			// before-only producer transition consumed the operation-bit
			// removal proposal from its one live lease.  Differential
			// before-only removal deliberately skips this branch; it has no
			// After proposal to synthesize or consume.
			candidate, matched := matchRemovalProposal(afterProposals, transition.Destination())
			if !matched {
				return nil, false
			}
			if _, duplicate := excluded[candidate]; duplicate {
				return nil, false
			}
			excluded[candidate] = struct{}{}
		}
	}
	// The declaration is a positive admission obligation for After. Any
	// declared After contribution proposal that was not consumed above is
	// refused before resolveSubmissions can place it in an ordinary column
	// group. Before proposals are historical evidence only and are never
	// admitted to ordinary groups; an unmatched ordinary Before is ignored.
	for index := 0; index < afterProposals.Len(); index++ {
		proposal, proposalOK := afterProposals.At(index)
		if !proposalOK || !proposal.Available() {
			return nil, false
		}
		descriptor, declaredOK := plan.ContributionCell(batch.operation, proposal.Destination())
		if !declaredOK {
			continue
		}
		if !descriptor.Available() || !descriptor.ValidFor(plan.Fence()) || descriptor.Operation() != batch.operation || descriptor.Column() != proposal.Destination().Column() {
			return nil, false
		}
		if _, consumed := excluded[index]; !consumed {
			return nil, false
		}
	}
	return excluded, true
}

func matchContributionSide(proposals binding.ProposalBatch, destination binding.CellToken, side binding.ContributionSide) bool {
	_, ok := matchContributionSideIndex(proposals, destination, side)
	return ok
}

func matchContributionSideIndex(proposals binding.ProposalBatch, destination binding.CellToken, side binding.ContributionSide) (int, bool) {
	if !proposals.Available() || !destination.Available() || !side.Available() || !side.Present() {
		return -1, false
	}
	candidate := -1
	matches := 0
	for index := 0; index < proposals.Len(); index++ {
		proposal, proposalOK := proposals.At(index)
		if !proposalOK || !proposal.Available() {
			return -1, false
		}
		if !proposal.Destination().Same(destination) {
			continue
		}
		matches++
		if proposalMatchesContributionSide(proposal, side) {
			candidate = index
		}
	}
	return candidate, matches == 1 && candidate >= 0
}

func proposalMatchesContributionSide(proposal binding.Proposal, side binding.ContributionSide) bool {
	if !proposal.Available() || !side.Available() || !side.Present() {
		return false
	}
	return !proposal.Removal() && proposal.Presence() == side.Presence() && proposal.Value().Same(side.Value())
}

func matchRemovalProposal(proposals binding.ProposalBatch, destination binding.CellToken) (int, bool) {
	if !proposals.Available() || !destination.Available() {
		return -1, false
	}
	candidate := -1
	matches := 0
	for index := 0; index < proposals.Len(); index++ {
		proposal, ok := proposals.At(index)
		if !ok || !proposal.Available() {
			return -1, false
		}
		if proposal.Destination().Same(destination) {
			matches++
			if proposal.Removal() {
				candidate = index
			}
		}
	}
	return candidate, matches == 1 && candidate >= 0
}

// reduceContributionUpdates materializes only the aggregate cells touched by
// one contribution delta.  The producer state remains the authority; the
// aggregate is an exact derived replacement/removal in the same store
// candidate.  Scope-qualified destinations are reduced independently so a
// support partition cannot overwrite a sibling partition.
func reduceContributionUpdates(base store.Version, view geometry.Geometry, mounted witness.Mounted, delta contribution.Delta) (map[model.ColumnID][]column.Update, bool) {
	if !base.Available() || !view.ValidFor(mounted) || !mounted.Available() || !delta.Available() {
		return nil, false
	}
	before, after := delta.Base(), delta.Next()
	updates := make(map[model.ColumnID][]column.Update)
	for _, target := range delta.AffectedTargets() {
		if !target.Available() {
			return nil, false
		}
		plan := mounted.Arrangement()
		spec, specOK := plan.Contribution(target.Port)
		if !specOK || !spec.Available() || spec.Port() != target.Port {
			return nil, false
		}
		priorRows := before.RowsFor(target)
		nextRows := after.RowsFor(target)
		destinations := make([]binding.CellToken, 0, len(priorRows)+len(nextRows))
		for _, row := range append(append([]contribution.Row(nil), priorRows...), nextRows...) {
			if !row.ValidFor(mounted.RuntimeFence()) || !row.Target().Same(target) {
				return nil, false
			}
			cell := row.Cell()
			if !cell.ValidFor(mounted.RuntimeFence()) || !containsCell(destinations, cell) {
				if !cell.ValidFor(mounted.RuntimeFence()) {
					return nil, false
				}
				destinations = append(destinations, cell)
			}
		}
		for _, destination := range destinations {
			coordinate, coordinateOK := view.Resolve(destination)
			if !coordinateOK || !coordinate.Available() {
				return nil, false
			}
			version, versionOK := base.Column(target.Port.Column)
			if !versionOK || version.Type() != spec.ValueType() || version.Guards() != coordinate.Mask().Manager() {
				return nil, false
			}
			rows := rowsForCell(nextRows, destination)
			aggregate, aggregateOK := reduction.ReduceAt(target, rows, destination, mounted, spec)
			if !aggregateOK || !aggregate.Available() {
				return nil, false
			}
			if aggregate.Removal() {
				removal, removalOK := column.NewRemoval(coordinate.Dense(), coordinate.Mask())
				if !removalOK {
					return nil, false
				}
				updates[target.Port.Column] = append(updates[target.Port.Column], removal)
				continue
			}
			value, valueOK := aggregate.Value()
			presence, presenceOK := aggregate.Presence()
			lineageValue, lineageOK := aggregate.Lineage()
			if !valueOK || !presenceOK || !lineageOK {
				return nil, false
			}
			cell, cellOK := column.NewCell(value, presence)
			if !cellOK {
				return nil, false
			}
			update, updateOK := column.NewUpdate(coordinate.Dense(), coordinate.Mask(), cell, lineageValue)
			if !updateOK {
				return nil, false
			}
			updates[target.Port.Column] = append(updates[target.Port.Column], update)
		}
	}
	return updates, true
}

func containsCell(cells []binding.CellToken, wanted binding.CellToken) bool {
	for _, cell := range cells {
		if cell.Same(wanted) {
			return true
		}
	}
	return false
}

func rowsForCell(rows []contribution.Row, destination binding.CellToken) []contribution.Row {
	result := make([]contribution.Row, 0, len(rows))
	for _, row := range rows {
		if row.Cell().Same(destination) {
			result = append(result, row)
		}
	}
	return result
}

type columnGroups map[model.ColumnID]map[geometry.Key][]resolvedSubmission

type resolvedSubmission struct {
	proposal binding.Proposal
	lineage  model.LineageRef
	key      geometry.Key
	mask     support.Mask
	cell     column.Cell
	remove   bool
}

func resolveSubmissions(
	base store.Version,
	view geometry.Geometry,
	mounted witness.Mounted,
	lineageAuthority lineage.Authority,
	batch SubmissionBatch,
	excluded map[int]struct{},
) (columnGroups, []*guard.Manager, bool) {
	_, afterProposals, _, afterOK, sidesOK := batch.proposalSides()
	if !sidesOK {
		return nil, nil, false
	}
	afterLineage, afterLineageOK := batch.afterLineageValue()
	groups := make(columnGroups)
	managers := make([]*guard.Manager, 0, 1)
	ordinary := 0
	for index := 0; afterOK && index < afterProposals.Len(); index++ {
		if _, skip := excluded[index]; skip {
			continue
		}
		ordinary++
		proposal, proposalOK := afterProposals.At(index)
		if !proposalOK || !proposal.Available() {
			return nil, nil, false
		}
		if !afterLineageOK || !lineageAuthority.Validate(afterLineage) {
			return nil, nil, false
		}
		cellToken := proposal.Destination()
		if !cellToken.ValidFor(base.Fence()) || !cellToken.ValidFor(view.Fence()) {
			return nil, nil, false
		}
		version, ok := base.Column(cellToken.Column())
		if !ok || version.Relation() != cellToken.Relation() || !version.Fence().Same(base.Fence()) {
			return nil, nil, false
		}
		coordinate, ok := view.Resolve(cellToken)
		if !ok || !coordinate.Available() || coordinate.Mask().Manager() != version.Guards() {
			return nil, nil, false
		}
		if batch.widening.Available() && batch.widening.Relation() != version.Relation() {
			return nil, nil, false
		}
		var cell column.Cell
		if !proposal.Removal() {
			value := proposal.Value()
			if value.Available() && (!value.ValidFor(base.Fence()) || value.Type() != version.Type()) {
				return nil, nil, false
			}
			cell, ok = column.NewCell(value, proposal.Presence())
			if !ok {
				return nil, nil, false
			}
		}
		resolved := resolvedSubmission{proposal: proposal, lineage: afterLineage, key: coordinate.Dense(), mask: coordinate.Mask(), cell: cell, remove: proposal.Removal()}
		byKey := groups[version.ID()]
		if byKey == nil {
			byKey = make(map[geometry.Key][]resolvedSubmission)
			groups[version.ID()] = byKey
		}
		byKey[coordinate.Dense()] = append(byKey[coordinate.Dense()], resolved)
		if len(managers) == 0 {
			managers = append(managers, version.Guards())
		} else if managers[0] != version.Guards() {
			return nil, nil, false
		}
	}
	if afterOK && !afterProposals.Available() || ordinary == 0 && len(groups) != 0 {
		return nil, nil, false
	}
	return groups, managers, true
}

func normalizeColumn(
	state store.Version,
	version column.Version,
	byKey map[geometry.Key][]resolvedSubmission,
	mounted witness.Mounted,
	lineageAuthority lineage.Authority,
	widening witness.WideningPermit,
	readScratch *store.ReadScratch,
	work *support.Work,
) ([]column.Update, bool) {
	keys := make([]geometry.Key, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	updates := make([]column.Update, 0, len(keys))
	for _, key := range keys {
		proposals := append([]resolvedSubmission(nil), byKey[key]...)
		sort.SliceStable(proposals, func(left, right int) bool { return resolvedLess(proposals[left], proposals[right]) })
		proposals = deduplicate(proposals)
		atoms, existing, valid := readAtoms(state, version, key, proposals, lineageAuthority, readScratch, work)
		if !valid {
			return nil, false
		}
		// Decode-only and authenticated-opaque cells have no lattice
		// operation.  Their exact token identity is checked by
		// combineSemantic; only Present cells require an owner algebra.
		algebra, algebraOK := mounted.Algebra(version.Type())
		if algebraOK && (algebra == nil || algebra.Type() != version.Type()) {
			return nil, false
		}
		for _, atom := range atoms {
			prior, hasPrior, valid := priorAt(atom, existing)
			if !valid {
				return nil, false
			}
			incoming := proposalsAt(atom, proposals)
			if len(incoming) == 0 {
				if !hasPrior {
					return nil, false
				}
				continue
			}
			hasRemoval := false
			hasWrite := false
			for _, proposal := range incoming {
				if proposal.remove {
					hasRemoval = true
				} else {
					hasWrite = true
				}
			}
			if hasRemoval {
				// A removal and semantic write over one refined atom are
				// conflicting owner submissions. Disjoint masks are handled
				// independently by the atom refinement above.
				if hasWrite {
					return nil, false
				}
				if !hasPrior {
					continue
				}
				update, ok := column.NewRemoval(key, atom)
				if !ok {
					return nil, false
				}
				updates = append(updates, update)
				continue
			}
			cell, changed, valid := combineSemantic(prior, hasPrior, incoming, algebra, widening, version)
			if !valid {
				return nil, false
			}
			lineageValue, valid := combineLineage(prior, hasPrior, incoming, lineageAuthority)
			if !valid {
				return nil, false
			}
			if hasPrior && !changed && lineageValue == prior.lineage {
				continue
			}
			if !hasPrior && !changed {
				return nil, false
			}
			if !hasPrior {
				// combineSemantic always returns a cell when incoming is
				// nonempty, but retain the guard as an explicit no-fallback law.
				if !cell.Available() {
					return nil, false
				}
			} else if !cell.Available() {
				return nil, false
			}
			update, ok := column.NewUpdate(key, atom, cell, lineageValue)
			if !ok {
				return nil, false
			}
			updates = append(updates, update)
		}
	}
	return sortUpdates(updates)
}

func deduplicate(proposals []resolvedSubmission) []resolvedSubmission {
	if len(proposals) < 2 {
		return proposals
	}
	result := proposals[:0]
	for _, candidate := range proposals {
		duplicate := false
		for _, prior := range result {
			if candidate.remove != prior.remove || !candidate.mask.Equal(prior.mask) || !candidate.proposal.Destination().Same(prior.proposal.Destination()) {
				continue
			}
			if !candidate.remove && (candidate.lineage != prior.lineage || !candidate.cell.SemanticSame(prior.cell)) {
				continue
			}
			duplicate = true
			break
		}
		if !duplicate {
			result = append(result, candidate)
		}
	}
	return result
}

type existingPart struct {
	region  support.Mask
	cell    column.Cell
	lineage model.LineageRef
}

func readAtoms(
	state store.Version,
	version column.Version,
	key geometry.Key,
	proposals []resolvedSubmission,
	lineageAuthority lineage.Authority,
	readScratch *store.ReadScratch,
	work *support.Work,
) ([]support.Mask, []existingPart, bool) {
	if len(proposals) == 0 || lineageAuthority == nil || readScratch == nil || !readScratch.Available() || work == nil || !work.OwnsManager(version.Guards()) {
		return nil, nil, false
	}
	union := proposals[0].mask
	if !union.Valid() || union.Manager() != version.Guards() {
		return nil, nil, false
	}
	for _, proposal := range proposals[1:] {
		var ok bool
		union, ok = support.UnionWithWork(work, nil, union, proposal.mask)
		if !ok {
			return nil, nil, false
		}
	}
	existing := make([]existingPart, 0, 4)
	completed, valid := state.Read(version.ID(), key, union, readScratch, func(part store.ReadPart) bool {
		if !part.Region().Valid() || part.Region().Manager() != version.Guards() || !part.Presence().Available() || !part.Lineage().Available() || !lineageAuthority.Validate(part.Lineage()) || part.Type() != version.Type() {
			return false
		}
		cell, ok := column.NewCell(part.Value(), part.Presence())
		if !ok || !cell.Available() {
			return false
		}
		existing = append(existing, existingPart{region: part.Region(), cell: cell, lineage: part.Lineage()})
		return true
	})
	if !completed || !valid {
		return nil, nil, false
	}
	atoms := []support.Mask{union}
	for _, proposal := range proposals {
		var ok bool
		atoms, ok = addBoundary(work, atoms, proposal.mask)
		if !ok {
			return nil, nil, false
		}
	}
	for _, prior := range existing {
		var ok bool
		atoms, ok = addBoundary(work, atoms, prior.region)
		if !ok {
			return nil, nil, false
		}
	}
	return atoms, existing, true
}

func addBoundary(work *support.Work, atoms []support.Mask, boundary support.Mask) ([]support.Mask, bool) {
	if work == nil || !boundary.Valid() || !work.OwnsManager(boundary.Manager()) {
		return nil, false
	}
	if support.Empty(boundary) {
		return atoms, true
	}
	remaining := boundary
	result := make([]support.Mask, 0, len(atoms)+1)
	for _, atom := range atoms {
		split, ok := support.ThreeWithWork(work, nil, atom, boundary)
		if !ok {
			return nil, false
		}
		if !support.Empty(split.LeftOnly()) {
			result = append(result, split.LeftOnly())
		}
		if !support.Empty(split.Overlap()) {
			result = append(result, split.Overlap())
		}
		remainingSplit, ok := support.ThreeWithWork(work, nil, remaining, atom)
		if !ok {
			return nil, false
		}
		remaining = remainingSplit.LeftOnly()
	}
	if !support.Empty(remaining) {
		result = append(result, remaining)
	}
	return result, true
}

func priorAt(atom support.Mask, existing []existingPart) (existingPart, bool, bool) {
	var prior existingPart
	found := false
	for _, candidate := range existing {
		region := candidate.region
		overlap, ok := support.Intersect(atom, region)
		if !ok {
			return existingPart{}, false, false
		}
		if support.Empty(overlap) {
			continue
		}
		if !atom.Entails(region) {
			return existingPart{}, false, false
		}
		if found {
			// Read's semantic/lineage product must be disjoint. Any second
			// terminal under one atom means the source diagram was not
			// canonical and must fail closed.
			return existingPart{}, false, false
		}
		prior = candidate
		found = true
	}
	return prior, found, true
}

func proposalsAt(atom support.Mask, proposals []resolvedSubmission) []resolvedSubmission {
	result := make([]resolvedSubmission, 0, len(proposals))
	for _, proposal := range proposals {
		if atom.Entails(proposal.mask) {
			result = append(result, proposal)
		}
	}
	return result
}

func combineSemantic(
	prior existingPart,
	hasPrior bool,
	incoming []resolvedSubmission,
	algebra binding.ValueAlgebra,
	widening witness.WideningPermit,
	version column.Version,
) (column.Cell, bool, bool) {
	if len(incoming) == 0 || (algebra != nil && algebra.Type() != version.Type()) {
		return column.Cell{}, false, false
	}
	kind := incoming[0].cell.Presence().Kind()
	for _, proposal := range incoming[1:] {
		if proposal.cell.Presence().Kind() != kind {
			return column.Cell{}, false, false
		}
	}
	if hasPrior && !prior.cell.Available() {
		return column.Cell{}, false, false
	}
	if hasPrior {
		priorKind := prior.cell.Presence().Kind()
		switch priorKind {
		case model.UnprovenMissing:
			if kind == model.UnprovenMissing {
				return prior.cell, false, true
			}
			if kind != model.ProvenAbsent && kind != model.Present && kind != model.AuthenticatedOpaque {
				return column.Cell{}, false, false
			}
			if kind == model.AuthenticatedOpaque {
				for _, proposal := range incoming[1:] {
					if widening.Available() || !proposal.cell.Value().Same(incoming[0].cell.Value()) {
						return column.Cell{}, false, false
					}
				}
			}
		case model.ProvenAbsent:
			if kind != model.ProvenAbsent {
				return column.Cell{}, false, false
			}
			return prior.cell, false, true
		case model.AuthenticatedOpaque:
			if kind != model.AuthenticatedOpaque {
				return column.Cell{}, false, false
			}
			for _, proposal := range incoming {
				if widening.Available() || !proposal.cell.Value().Same(prior.cell.Value()) {
					return column.Cell{}, false, false
				}
			}
			return prior.cell, false, true
		case model.Present:
			if kind != model.Present || !prior.cell.Value().Available() {
				return column.Cell{}, false, false
			}
		default:
			return column.Cell{}, false, false
		}
	}

	if kind != model.Present {
		if widening.Available() {
			return column.Cell{}, false, false
		}
		if kind == model.AuthenticatedOpaque {
			for _, proposal := range incoming[1:] {
				if !proposal.cell.Value().Same(incoming[0].cell.Value()) {
					return column.Cell{}, false, false
				}
			}
		}
		if hasPrior {
			if prior.cell.Presence().Kind() == model.UnprovenMissing {
				return incoming[0].cell, true, incoming[0].cell.Available()
			}
			return prior.cell, false, true
		}
		return incoming[0].cell, true, incoming[0].cell.Available()
	}
	// Present is a lattice value and cannot be admitted without the
	// owner-issued ascent authority.  Opaque/absence branches above are
	// intentionally algebra-free.
	if algebra == nil {
		return column.Cell{}, false, false
	}

	// Opaque values have no algebra at this layer. Even a sparse first
	// publication therefore requires every overlapping opaque proposal to be
	// the exact same token.
	if !hasPrior || prior.cell.Presence().Kind() != model.Present {
		for _, proposal := range incoming {
			if !proposal.cell.Value().Available() {
				return column.Cell{}, false, false
			}
		}
	}
	hasValuePrior := hasPrior && prior.cell.Presence().Kind() == model.Present
	current := binding.ValueToken{}
	if hasValuePrior {
		current = prior.cell.Value()
	} else {
		current = incoming[0].cell.Value()
		if !current.Available() {
			return column.Cell{}, false, false
		}
	}
	start := current
	for index, proposal := range incoming {
		value := proposal.cell.Value()
		if !value.Available() || value.Type() != version.Type() || !value.ValidFor(version.Fence()) {
			return column.Cell{}, false, false
		}
		if !hasValuePrior && index == 0 {
			continue
		}
		var next binding.ValueToken
		var ok bool
		if widening.Available() {
			next, ok = algebra.Widen(current, value)
		} else {
			next, ok = algebra.Join(current, value)
		}
		if !ok || !next.Available() || next.Type() != version.Type() || !next.ValidFor(version.Fence()) || !algebra.LessOrEqual(current, next) || !algebra.LessOrEqual(value, next) {
			return column.Cell{}, false, false
		}
		current = next
	}
	if hasValuePrior && algebra.LessOrEqual(current, start) && algebra.LessOrEqual(start, current) {
		return prior.cell, false, true
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		return column.Cell{}, false, false
	}
	cell, ok := column.NewCell(current, presence)
	return cell, !hasValuePrior || !cell.SemanticSame(prior.cell), ok
}

func combineLineage(prior existingPart, hasPrior bool, incoming []resolvedSubmission, authority lineage.Authority) (model.LineageRef, bool) {
	if authority == nil || len(incoming) == 0 {
		return model.LineageRef{}, false
	}
	current := model.LineageRef{}
	if hasPrior {
		current = prior.lineage
		if !current.Available() || !authority.Validate(current) {
			return model.LineageRef{}, false
		}
	} else {
		current = incoming[0].lineage
		if !current.Available() || !authority.Validate(current) {
			return model.LineageRef{}, false
		}
	}
	start := current
	for index, proposal := range incoming {
		if !proposal.lineage.Available() || !authority.Validate(proposal.lineage) {
			return model.LineageRef{}, false
		}
		if !hasPrior && index == 0 {
			continue
		}
		joined, ok := authority.Join(current, proposal.lineage)
		if !ok || !joined.Available() || !authority.Validate(joined) {
			return model.LineageRef{}, false
		}
		current = joined
	}
	if hasPrior && current == start {
		return start, true
	}
	return current, true
}

func sortUpdates(updates []column.Update) ([]column.Update, bool) {
	type candidate struct {
		update   column.Update
		identity [32]byte
	}
	values := make([]candidate, len(updates))
	for index, update := range updates {
		identity, ok := update.Mask().Identity()
		if !ok {
			return nil, false
		}
		values[index] = candidate{update: update, identity: identity}
	}
	sort.SliceStable(values, func(left, right int) bool {
		if values[left].update.Key() != values[right].update.Key() {
			return values[left].update.Key() < values[right].update.Key()
		}
		return bytes.Compare(values[left].identity[:], values[right].identity[:]) < 0
	})
	result := make([]column.Update, len(values))
	for index, value := range values {
		result[index] = value.update
	}
	return result, true
}

func resolvedLess(left, right resolvedSubmission) bool {
	leftKind, rightKind := left.cell.Presence().Kind(), right.cell.Presence().Kind()
	if leftKind != rightKind {
		return leftKind < rightKind
	}
	leftValue, rightValue := left.cell.Value(), right.cell.Value()
	if leftValue.Available() != rightValue.Available() {
		return !leftValue.Available()
	}
	if leftValue.Available() {
		if comparison := compareContent(leftValue.Opaque(), rightValue.Opaque()); comparison != 0 {
			return comparison < 0
		}
	}
	leftMask, leftOK := left.mask.Identity()
	rightMask, rightOK := right.mask.Identity()
	if leftOK && rightOK {
		if comparison := bytes.Compare(leftMask[:], rightMask[:]); comparison != 0 {
			return comparison < 0
		}
	}
	if comparison := compareLineage(left.lineage, right.lineage); comparison != 0 {
		return comparison < 0
	}
	return false
}

func compareLineage(left, right model.LineageRef) int {
	if comparison := compareContent(left.Owner().Content(), right.Owner().Content()); comparison != 0 {
		return comparison
	}
	return compareContent(left.Content(), right.Content())
}

func compareContent(left, right [32]byte) int { return bytes.Compare(left[:], right[:]) }

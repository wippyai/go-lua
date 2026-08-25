package transaction

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
)

// Submission is the sidecar for one live proposal in SubmissionBatch.  The
// proposal itself stays in binding.ProposalBatch, whose lease is checked by
// Prepare. Widening is an invocation-level capability carried by the batch,
// never a per-proposal flag or an inferred fallback.
type Submission struct {
	Lineage model.LineageRef
}

// SubmissionBatch keeps the live binding proposal lease separate from the
// immutable proof-sidecar metadata. Lineages have exactly the same cardinality
// as Proposals. Widening is one opaque, mount-issued permit for the whole
// invocation. A stale/reset ProposalBatch is rejected; its copied values are
// never accepted as a new authority.
type SubmissionBatch struct {
	Proposals binding.ProposalBatch
	Sidecars  []Submission
	Widening  witness.WideningPermit
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
	if !base.Available() {
		return database.Prepared{}, false
	}
	sourceBase := base.Store()
	if !sourceBase.Available() {
		return database.Prepared{}, false
	}
	if !batch.Proposals.Available() {
		return database.Prepared{}, false
	}
	// Empty input is an explicit no-op.  It shares the exact immutable root
	// and does not require manufacturing a database delta.
	if batch.Proposals.Len() == 0 {
		// A widening permit is an invocation capability, not a free-standing
		// fact.  Without a live proposal to redeem it, accepting the permit
		// would let a caller launder recurrence evidence through a no-op.
		if len(batch.Sidecars) != 0 || batch.Widening.Available() {
			return database.Prepared{}, false
		}
		prepared, ok := store.Prepare(sourceBase)
		if !ok {
			return database.Prepared{}, false
		}
		return database.Prepare(base, prepared, readScratch)
	}
	if batch.Sidecars == nil || batch.Proposals.Len() != len(batch.Sidecars) {
		return database.Prepared{}, false
	}
	// Sidecars are copied once at the door. Proposal values remain borrowed
	// through the live ProposalBatch lease and are never copied into an
	// authority object.
	batch.Sidecars = append([]Submission(nil), batch.Sidecars...)
	mounted := base.Mounted()
	lineageAuthority, lineageOK := mounted.Lineage()
	if !view.Available() || !mounted.Available() || !lineageOK || lineageAuthority == nil || readScratch == nil || !readScratch.Available() || !view.Fence().Same(base.Fence()) || !lineageAuthority.Fence().Same(base.Fence()) || !lineageAuthority.Owner().Available() || !lineageAuthority.Identity().Available() || !admitWidening(mounted, batch.Widening) {
		return database.Prepared{}, false
	}

	groups, managers, ok := resolveSubmissions(sourceBase, view, mounted, lineageAuthority, batch)
	if !ok || len(groups) == 0 {
		return database.Prepared{}, false
	}
	manager := managers[0]
	if manager == nil {
		return database.Prepared{}, false
	}
	work := support.New(manager)
	if work == nil {
		return database.Prepared{}, false
	}
	defer work.Close()

	changes := make([]column.Delta, 0, len(groups))
	for _, columnID := range sourceBase.ColumnIDs() {
		keys, present := groups[columnID]
		if !present {
			continue
		}
		version, valid := sourceBase.Column(columnID)
		if !valid || version.Guards() != manager {
			return database.Prepared{}, false
		}
		updates, valid := normalizeColumn(sourceBase, version, keys, mounted, lineageAuthority, batch.Widening, readScratch, work)
		if !valid {
			return database.Prepared{}, false
		}
		if len(updates) == 0 {
			continue
		}
		next, delta, valid := version.Next(updates...)
		if !valid || !next.Available() || !delta.Available() || !next.SuccessorOf(version) {
			return database.Prepared{}, false
		}
		changes = append(changes, delta)
	}
	if !batch.Proposals.Available() || batch.Proposals.Len() != len(batch.Sidecars) {
		return database.Prepared{}, false
	}
	if len(changes) == 0 {
		prepared, ok := store.Prepare(sourceBase)
		if !ok {
			return database.Prepared{}, false
		}
		return database.Prepare(base, prepared, readScratch)
	}

	// Store.Prepare authenticates every column delta. Database.Prepare then
	// attaches every actual arrangement successor; database.Commit owns the
	// sole root publication.
	prepared, valid := store.Prepare(sourceBase, changes...)
	if !valid || !prepared.Available() {
		return database.Prepared{}, false
	}
	return database.Prepare(base, prepared, readScratch)
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

type columnGroups map[model.ColumnID]map[geometry.Key][]resolvedSubmission

type resolvedSubmission struct {
	proposal binding.Proposal
	lineage  model.LineageRef
	key      geometry.Key
	mask     support.Mask
	cell     column.Cell
}

func resolveSubmissions(
	base store.Version,
	view geometry.Geometry,
	mounted witness.Mounted,
	lineageAuthority lineage.Authority,
	batch SubmissionBatch,
) (columnGroups, []*guard.Manager, bool) {
	groups := make(columnGroups)
	managers := make([]*guard.Manager, 0, 1)
	for index, submission := range batch.Sidecars {
		proposal, proposalOK := batch.Proposals.At(index)
		if !proposalOK || !proposal.Available() || !submission.Lineage.Available() || !lineageAuthority.Validate(submission.Lineage) {
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
		algebra, ok := mounted.Algebra(version.Type())
		if !ok || algebra == nil || algebra.Type() != version.Type() {
			return nil, nil, false
		}
		value := proposal.Value()
		if value.Available() && (!value.ValidFor(base.Fence()) || value.Type() != version.Type()) {
			return nil, nil, false
		}
		if batch.Widening.Available() && batch.Widening.Relation() != version.Relation() {
			return nil, nil, false
		}
		cell, ok := column.NewCell(value, proposal.Presence())
		if !ok {
			return nil, nil, false
		}
		resolved := resolvedSubmission{proposal: proposal, lineage: submission.Lineage, key: coordinate.Dense(), mask: coordinate.Mask(), cell: cell}
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
	if !batch.Proposals.Available() || batch.Proposals.Len() != len(batch.Sidecars) {
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
		algebra, ok := mounted.Algebra(version.Type())
		if !ok || algebra == nil || algebra.Type() != version.Type() {
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
			if candidate.lineage != prior.lineage || !candidate.mask.Equal(prior.mask) || !candidate.cell.SemanticSame(prior.cell) || !candidate.proposal.Destination().Same(prior.proposal.Destination()) {
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
	if algebra == nil || algebra.Type() != version.Type() || len(incoming) == 0 {
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

	// Opaque values have no algebra at this layer. Even a sparse first
	// publication therefore requires every overlapping opaque proposal to be
	// the exact same token.
	if !hasPrior || prior.cell.Presence().Kind() != model.Present {
		for _, proposal := range incoming {
			if widening.Available() || !proposal.cell.Value().Available() {
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

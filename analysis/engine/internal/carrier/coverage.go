package carrier

import (
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// Contribution is the sole publishable Rule/Point value. State remains the
// semantic carrier; coverage is the exact authored Target x Guard relation
// needed only while independent producer contributions are folded. Keeping
// the pair opaque prevents sparse Default from being confused with an
// uncovered Factor without exposing typed keys or creating another fact
// plane.
type Contribution struct {
	state    State
	coverage contributionCoverage
	// seal is issued only by an owning Work after the complete contribution
	// (including every coverage row) has passed the ordinary deep validator.
	// It is deliberately private: code outside carrier cannot manufacture the
	// admission proof or retarget a publication to another Work.
	seal      *contributionSeal
	authority *stateAuthority
}

// contributionSeal is one evaluator-local admission capability. Contributions
// retain the same immutable token as they move through a Work's fold; the
// token is not a second state or coverage representation. A Work also owns a
// distinct neutral seal. That seal is issued only after the exact initial-root
// and false-support proof, so the fast merge identity cannot be forged by a
// caller with an ordinary empty-coverage Contribution.
type contributionSeal struct {
	work        *Work
	composition *Composition
}

type contributionCoverage struct {
	composition *Composition
	// proof is the admission token of the Work that walked every authored row
	// of this exact immutable surface once. A surface is admitted again on
	// every refresh that transports, folds, or compares the point holding it,
	// and its rows cannot change, so the walk is a construction-time
	// obligation rather than a per-admission one. The token is issued per Work
	// epoch, so a surface built by an earlier evaluator carries none and pays
	// the whole proof again.
	proof *contributionSeal
	slots []slotCoverage
	// occupied is the slot-plane issuance. Every producer writes it in the
	// same statement that writes its slot row, so a consumer enumerates the
	// slots that actually carry an authored relation instead of scanning the
	// whole Factor plane. A sparse contribution therefore costs its own width.
	occupied change.Slots
}

// CoverageRole distinguishes a carried predecessor surface from a sibling
// authored effect.  Effect is deliberately the zero value so existing raw
// TargetRegion literals remain ordinary authored rows.
//
// CoverageDisplacement is the authored row of a routed strong write. Its
// declaration proves the write is the reduction of the value read at that
// same coordinate on that same Factor, so the row states the complete value
// of its cell rather than one more term to join: within its own region it
// supersedes every predecessor generation reaching the same Target. The
// distinction is the program point - a routed write publishes the cell after
// the operation, and the baselines the fold transports in are the cell before
// it. A published point carries no displacement row: publishPointState
// restamps every row of a new generation as a baseline of that generation.
type CoverageRole uint8

const (
	CoverageEffect CoverageRole = iota
	CoverageBaseline
	CoverageDisplacement
)

// validRole reports whether a role is one this carrier authors.
func (role CoverageRole) validRole() bool {
	return role == CoverageEffect || role == CoverageBaseline || role == CoverageDisplacement
}

// LineageToken is an opaque identity for one published PointState generation.
// It carries no roots or coverage and is comparable, so typed fold code can
// group rows without receiving a second state representation. The private
// owner/sequence fields are authenticated by Work before a row crosses any
// carrier boundary.
type LineageToken struct{ value *pointLineage }

// Valid reports whether the token has the shape of a carrier-issued lineage.
// Work ownership is checked separately at admission because a token copied
// from a sibling Work is structurally valid but not valid for this Work.
func (token LineageToken) Valid() bool {
	return token.value != nil && token.value.owner != nil && token.value.sequence != 0
}

type slotCoverage struct {
	targets []TargetRegion
}

// TargetRegion is one immutable authored write surface. Target keeps the
// concrete key set beside its typed Binding; carrier owns only its exact
// nonempty Guard region.
type TargetRegion struct {
	target  Target
	region  support.Mask
	lineage LineageToken
	role    CoverageRole
}

func (row TargetRegion) Target() Target        { return row.target }
func (row TargetRegion) Region() support.Mask  { return row.region }
func (row TargetRegion) Lineage() LineageToken { return row.lineage }
func (row TargetRegion) Role() CoverageRole    { return row.role }

// NewTargetRegion creates an ordinary authored effect row. Carried and
// transformed rows use WithLineage/WithRole below so their provenance remains
// attached to the canonical coverage row.
func NewTargetRegion(target Target, region support.Mask) TargetRegion {
	return TargetRegion{target: target, region: region, role: CoverageEffect}
}

// NewDisplacementRegion creates the authored row of a routed strong write.
// The region is the one the write's own selected read observed its
// predecessor in, so the row displaces exactly what it read and nothing else.
func NewDisplacementRegion(target Target, region support.Mask) TargetRegion {
	return TargetRegion{target: target, region: region, role: CoverageDisplacement}
}

func (row TargetRegion) WithLineage(lineage LineageToken, role CoverageRole) TargetRegion {
	row.lineage = lineage
	row.role = role
	return row
}

func (row TargetRegion) WithRegion(region support.Mask) TargetRegion {
	row.region = region
	return row
}

func sameCoverageMetadata(left, right TargetRegion) bool {
	return left.lineage == right.lineage && left.role == right.role
}

// compareCoverageRows is the one canonical order for authored rows. Target
// remains the primary key; duplicate Targets are ordered by role first and
// then by carrier-issued lineage sequence. This makes baseline/effect order
// independent of operand/input order while retaining different metadata rows.
func compareCoverageRows(left, right TargetRegion) int {
	if left.target.Less(right.target) {
		return -1
	}
	if right.target.Less(left.target) {
		return 1
	}
	if left.role != right.role {
		if left.role == CoverageBaseline {
			return -1
		}
		if right.role == CoverageBaseline {
			return 1
		}
		if left.role < right.role {
			return -1
		}
		return 1
	}
	leftValid, rightValid := left.lineage.Valid(), right.lineage.Valid()
	if leftValid != rightValid {
		if !leftValid {
			return -1
		}
		return 1
	}
	if leftValid && left.lineage.value.sequence != right.lineage.value.sequence {
		if left.lineage.value.sequence < right.lineage.value.sequence {
			return -1
		}
		return 1
	}
	return 0
}

func sameCoverageRow(left, right TargetRegion) bool {
	return left.target.Same(right.target) && sameCoverageMetadata(left, right) && (left.region.SameHandle(right.region) || left.region.Equal(right.region))
}

func sameCoverageSurfaceRow(left, right TargetRegion) bool {
	return left.target.Same(right.target) && (left.region.SameHandle(right.region) || left.region.Equal(right.region))
}

// SlotCoverage is the read-only slot-local projection supplied to SlotWork.
// Its backing rows remain carrier-owned and contain no typed key vocabulary.
type SlotCoverage struct{ value *slotCoverage }

func (coverage SlotCoverage) Count() int {
	if coverage.value == nil {
		return 0
	}
	return len(coverage.value.targets)
}

func (coverage SlotCoverage) At(index int) (TargetRegion, bool) {
	if coverage.value == nil || index < 0 || index >= len(coverage.value.targets) {
		return TargetRegion{}, false
	}
	return coverage.value.targets[index], true
}

// State returns the semantic half of this exact paired publication.
func (contribution Contribution) State() State { return contribution.state }
func (contribution Contribution) Support() support.Mask {
	return contribution.state.Support()
}
func (contribution Contribution) Scope() Scope { return contribution.state.Scope() }
func (contribution Contribution) HandleAt(slot shape.Slot) (RootHandle, bool) {
	return contribution.state.HandleAt(slot)
}

func (contribution Contribution) Valid() bool {
	return contribution.state.live() && contribution.coverage.validFor(contribution.state)
}

// admittedContribution is the hot internal boundary for an already published
// Contribution. Public Valid remains deliberately deep; this check is only
// used after carrier construction has issued the private seal and therefore
// proves ownership and the immutable outer shape in O(1), without walking
// authored rows.
func (work *Work) admittedContribution(contribution Contribution) bool {
	if work == nil || !work.live() || work.contributionSeal == nil || contribution.seal == nil ||
		(contribution.seal != work.contributionSeal && contribution.seal != work.neutralSeal) ||
		contribution.seal.work != work || contribution.seal.composition != work.composition {
		return false
	}
	state := contribution.state
	if contribution.authority == nil || state.authority != contribution.authority || state.authority.composition != work.composition || !state.live() || state.previewMarked() || state.contributionMarked() {
		return false
	}
	coverage := contribution.coverage
	return work.validContributionCoverage(state, coverage)
}

// admitContribution attaches the Work-local seal only after the existing
// state/coverage validator succeeds. Every carrier construction route goes
// through this helper so an unsealed or malformed internal value cannot enter
// the admitted fast path.
func (work *Work) admitContribution(state State, coverage contributionCoverage) (Contribution, bool) {
	result := Contribution{state: state, coverage: coverage}
	if work == nil || !work.live() || !work.OwnsState(state) || state.previewMarked() || state.contributionMarked() || !result.Valid() || !work.closedContributionRoots(state, coverage) {
		return Contribution{}, false
	}
	result.seal = work.contributionSeal
	result.authority = state.authority
	if !work.admittedContribution(result) {
		return Contribution{}, false
	}
	result.coverage.proof = work.contributionSeal
	return result, true
}

// closedContributionRoots is the cold issuance proof for a State accepted
// through the untrusted/raw boundary.  Canonical producer paths close their
// roots while constructing their pending patch and use
// admitConstructedContribution below, so this traversal is never a hot
// admitted-read check or a duplicate per-fold validation.
func (work *Work) closedContributionRoots(state State, coverage contributionCoverage) bool {
	if work == nil || !work.live() || !work.OwnsState(state) || coverage.composition != work.composition {
		return false
	}
	for position, slot := range work.slots {
		if slot == nil || position >= len(state.roots) || !slot.ContributionClosedUnder(state.roots[position], state.support, coverageRows(coverage.slot(shape.Slot(position)))) {
			return false
		}
	}
	return true
}

// admitConstructedContribution is the private hot cut for carrier-produced
// publications.  Its callers have already proved the complete State through
// the commit/transport/projection operation and have canonicalized every
// authored row through canonicalCoverage or an equivalent carrier-owned
// relation.  Rechecking those rows here would turn every fold back into an
// O(F) validation pass, so this helper checks only the immutable outer shape
// and reuses the existing Work seal and State authority.
//
// This is deliberately separate from admitContribution: every caller is a
// canonical carrier producer which has already physically closed its roots at
// the typed construction boundary.  Raw State/coverage admission continues
// through admitContribution and pays the whole-root closure proof.
func (work *Work) admitConstructedContribution(state State, coverage contributionCoverage) (Contribution, bool) {
	if work == nil || !work.live() || !work.OwnsState(state) || state.previewMarked() || state.contributionMarked() || coverage.composition != work.composition || len(coverage.slots) != 0 && len(coverage.slots) != work.composition.Count() {
		return Contribution{}, false
	}
	result := Contribution{state: state, coverage: coverage, seal: work.contributionSeal, authority: state.authority}
	if !work.admittedContribution(result) {
		return Contribution{}, false
	}
	result.coverage.proof = work.contributionSeal
	return result, true
}

// exactNeutralState is the carrier's issuance-time semantic identity proof.
// It checks that roots remain the immutable Composition initial vector and
// support is the exact empty Boolean region. This deep proof is deliberately
// paid only while EmptyContribution issues the private seal; the hot
// neutralContribution check uses that seal (plus ordinary ownership and empty
// coverage admission) without revisiting the root vector. Scope is deliberately
// not folded into this proof: MergeContribution's ordinary Work/scope fence
// checks both operands before the identity is used.
func (work *Work) exactNeutralState(state State) bool {
	return work != nil && work.live() && work.OwnsState(state) &&
		support.Empty(state.support) && sameRootVector(state.roots, work.composition.initial)
}

func (work *Work) admitNeutralContribution(state State, coverage contributionCoverage) (Contribution, bool) {
	result := Contribution{state: state, coverage: coverage}
	if work == nil || work.neutralSeal == nil || !work.exactNeutralState(state) || len(coverage.slots) != 0 || coverage.composition != work.composition || !result.Valid() {
		return Contribution{}, false
	}
	result.seal = work.neutralSeal
	result.authority = state.authority
	return result, work.admittedContribution(result)
}

// neutralContribution reports the private proof carried by a Contribution.
// The deep initial-root/false-support proof runs only when EmptyContribution
// issues the seal. The hot merge check is only token/ownership admission plus
// the empty authored-coverage shape; it never walks the slot vector.
func (work *Work) neutralContribution(contribution Contribution) bool {
	return work != nil && contribution.seal == work.neutralSeal && work.admittedContribution(contribution) && len(contribution.coverage.slots) == 0
}

// EmptyContribution is the sole Point-base construction route.  A raw State
// cannot be relabelled as an empty RHS: only the composition's immutable
// initial root vector is valid, under any already-issued outer support.  The
// false-support case additionally receives the merge-neutral seal; a
// nonempty initial support remains a normal (non-neutral) contribution so a
// later fold cannot silently discard its structural support.
func (work *Work) EmptyContribution(state State) (Contribution, bool) {
	if !work.live() || !work.OwnsState(state) || state.previewMarked() || state.contributionMarked() || !sameRootVector(state.roots, work.composition.initial) {
		return Contribution{}, false
	}
	coverage := contributionCoverage{composition: work.composition}
	if work.exactNeutralState(state) {
		return work.admitNeutralContribution(state, coverage)
	}
	return work.admitContribution(state, coverage)
}

// admitDerivedContribution preserves the neutral seal only when an operation
// started from an already-proved neutral input and the derived State remains
// the exact initial-root/false-support, empty-coverage identity. Ordinary
// derived Contributions use the private constructed cut because Transport
// and ProjectContribution prove their State roots and canonical coverage
// before reaching this boundary. Ordinary empty Contributions never gain the
// neutral proof merely by passing through a transport or projection.
func (work *Work) admitDerivedContribution(input Contribution, state State, coverage contributionCoverage) (Contribution, bool) {
	if work.neutralContribution(input) {
		if result, ok := work.admitNeutralContribution(state, coverage); ok {
			return result, true
		}
	}
	return work.admitConstructedContribution(state, coverage)
}

// rescopeContribution publishes one coordinate-identical PointState in the
// relation's exact target Scope without touching its immutable support,
// roots, or authored coverage. This is a new carrier wrapper, not a second
// semantic root or transport cache.
func (work *Work) rescopeContribution(input Contribution, target Scope) (Contribution, bool) {
	if work == nil || !work.live() || !work.admittedContribution(input) || !target.validFor(work.composition) {
		return Contribution{}, false
	}
	state := input.state
	state.scope = target
	return work.admitConstructedContribution(state, input.coverage)
}

// shapeFor proves the facts that depend on the State this surface is paired
// with. They are the only part of validity a proven surface must still answer,
// because the same immutable rows may be offered against another State.
func (coverage contributionCoverage) shapeFor(state State) bool {
	return state.live() && coverage.composition != nil && state.authority.composition == coverage.composition &&
		(len(coverage.slots) == 0 || len(coverage.slots) == coverage.composition.Count()) &&
		(len(coverage.slots) != 0 || coverage.occupied.Empty())
}

func (coverage contributionCoverage) validFor(state State) bool {
	if !coverage.shapeFor(state) {
		return false
	}
	for position := range coverage.slots {
		if coverage.occupied.Test(position) != (len(coverage.slots[position].targets) != 0) {
			return false
		}
		for index, row := range coverage.slots[position].targets {
			slot, ok := row.target.Slot()
			if !ok || int(slot) != position || !coverage.composition.OwnsTarget(slot, row.target) || !row.region.Valid() || row.region.Manager() != coverage.composition.guards || support.Empty(row.region) || !row.role.validRole() || row.role == CoverageBaseline && !row.lineage.Valid() || row.lineage.value != nil && !row.lineage.Valid() || index > 0 && compareCoverageRows(row, coverage.slots[position].targets[index-1]) < 0 {
				return false
			}
		}
	}
	return true
}

// ownsLineage authenticates a copied token against the Work that issued it.
// A token from another live Work over the same Composition is intentionally
// rejected: lineage is evaluator-local provenance, not Composition identity.
func (work *Work) ownsLineage(token LineageToken) bool {
	return work != nil && work.live() && token.Valid() && token.value.owner == work
}

func (work *Work) validCoverageRow(row TargetRegion) bool {
	if work == nil || !row.role.validRole() {
		return false
	}
	if row.role == CoverageBaseline && !row.lineage.Valid() {
		return false
	}
	return row.lineage.value == nil || work.ownsLineage(row.lineage)
}

// validContributionCoverage is the Work-specific half of coverage validity.
// The structural validator can prove row shape, but only the issuing Work can
// prove that every nonzero lineage belongs to this evaluator epoch.
//
// A surface this Work already proved carries its admission token and answers
// here in O(1). The shape facts that depend on the State it is paired with -
// liveness, composition, and slot width - are cheap and are proved every
// time, because the same immutable surface may be offered against a different
// State.
func (work *Work) validContributionCoverage(state State, coverage contributionCoverage) bool {
	if work == nil || coverage.composition != work.composition || !coverage.shapeFor(state) {
		return false
	}
	if work.contributionSeal != nil && coverage.proof == work.contributionSeal {
		return true
	}
	if !coverage.validFor(state) {
		return false
	}
	for position, more := coverage.occupied.Next(0); more; position, more = coverage.occupied.Next(position + 1) {
		if position < 0 || position >= len(coverage.slots) {
			return false
		}
		rows := coverage.slots[position].targets
		dbgCoverageProofRows.Add(uint64(len(rows)))
		for _, row := range rows {
			if !work.validCoverageRow(row) {
				return false
			}
		}
	}
	return true
}

// dbgCoverageProofRows counts the authored rows the deep coverage proof
// walks. An authored surface owes that walk once, when it is constructed, so
// a refresh that admits an already proven surface adds nothing here.
var dbgCoverageProofRows atomic.Uint64

// DbgCoverageProofRows reports the accumulated deep coverage row walk.
func DbgCoverageProofRows() uint64 { return dbgCoverageProofRows.Load() }

// DbgCoverageProofRowsReset clears the accumulated deep coverage row walk.
func DbgCoverageProofRowsReset() { dbgCoverageProofRows.Store(0) }

func (work *Work) validSlotCoverage(rows slotCoverage) bool {
	if work == nil || !work.live() {
		return false
	}
	for index, row := range rows.targets {
		slot, ok := row.target.Slot()
		if !ok || !work.composition.OwnsTarget(slot, row.target) || !row.region.Valid() || row.region.Manager() != work.composition.guards || support.Empty(row.region) || !work.validCoverageRow(row) || index > 0 && compareCoverageRows(row, rows.targets[index-1]) < 0 {
			return false
		}
	}
	return true
}

// newContributionCoverage assembles one coverage header and issues its
// occupied slot set from the rows it carries. Every construction of the
// private representation from an existing slot vector goes through it, so the
// issuance cannot be forgotten by a producer that builds its rows first.
func newContributionCoverage(composition *Composition, slots []slotCoverage) contributionCoverage {
	coverage := contributionCoverage{composition: composition, slots: slots}
	for position := range slots {
		if len(slots[position].targets) != 0 {
			coverage.occupied.Set(position)
		}
	}
	if coverage.occupied.Empty() {
		coverage.slots = nil
	}
	return coverage
}

// unionOccupiedInto accumulates this coverage's occupied slots into a
// caller-owned reusable set. It is the join every multi-operand coverage
// consumer walks instead of the whole Factor plane.
func (coverage contributionCoverage) unionOccupiedInto(dst *change.Slots) bool {
	return coverage.occupied.UnionInto(dst)
}

func (coverage contributionCoverage) slot(slot shape.Slot) slotCoverage {
	if coverage.composition == nil || !coverage.composition.shape.ValidSlot(slot) || len(coverage.slots) == 0 {
		return slotCoverage{}
	}
	return coverage.slots[int(slot)]
}

func sameSlotCoverage(left, right slotCoverage) bool {
	if len(left.targets) != len(right.targets) {
		return false
	}
	if len(left.targets) != 0 && &left.targets[0] == &right.targets[0] {
		return true
	}
	for index := range left.targets {
		if !sameCoverageSurfaceRow(left.targets[index], right.targets[index]) {
			return false
		}
	}
	return true
}

func sameSlotCoverageProvenance(left, right slotCoverage) bool {
	if len(left.targets) != len(right.targets) {
		return false
	}
	if len(left.targets) != 0 && &left.targets[0] == &right.targets[0] {
		return true
	}
	for index := range left.targets {
		if !sameCoverageRow(left.targets[index], right.targets[index]) {
			return false
		}
	}
	return true
}

// sameContributionCoverage compares two authored surfaces over the join of
// their occupied slots. Every slot outside that join is empty on both sides
// and is therefore equal without being visited.
func sameContributionCoverage(left, right contributionCoverage) bool {
	if left.composition == nil || left.composition != right.composition {
		return false
	}
	count := left.composition.Count()
	for position, more := left.occupied.Next(0); more; position, more = left.occupied.Next(position + 1) {
		if position >= count {
			return false
		}
		if !sameSlotCoverage(left.slots[position], right.slot(shape.Slot(position))) {
			return false
		}
	}
	// A slot the right side authors and the left side does not is a
	// difference by itself; the join is walked from both ends so neither
	// direction needs the whole plane.
	for position, more := right.occupied.Next(0); more; position, more = right.occupied.Next(position + 1) {
		if position >= count || !left.occupied.Test(position) {
			return false
		}
	}
	return true
}

// unionCoverage joins two already-admitted closed contribution surfaces.
// The caller has proved that output support is the union of the operands, so
// every row is already within the result and needs neither clipping nor a
// repeated Target ownership scan. Cold/raw coverage still enters through the
// validating canonicalCoverage/restrictCoverage paths.
func (work *Work) unionCoverage(left, right contributionCoverage) (contributionCoverage, bool) {
	if !work.live() || left.composition != work.composition || right.composition != work.composition {
		return contributionCoverage{}, false
	}
	if left.occupied.Empty() {
		return right, true
	}
	if right.occupied.Empty() {
		return left, true
	}
	result := contributionCoverage{composition: work.composition, slots: make([]slotCoverage, work.composition.Count())}
	joined := work.borrowSlotSet()
	defer work.releaseSlotSet()
	if joined == nil || !left.unionOccupiedInto(&joined.set) || !right.unionOccupiedInto(&joined.set) {
		return contributionCoverage{}, false
	}
	for position, more := joined.set.Next(0); more; position, more = joined.set.Next(position + 1) {
		merged, ok := work.unionSlotCoverage(left.slot(shape.Slot(position)), right.slot(shape.Slot(position)))
		if !ok {
			return contributionCoverage{}, false
		}
		if len(merged.targets) == 0 {
			continue
		}
		result.slots[position] = merged
		result.occupied.Set(position)
	}
	if result.occupied.Empty() {
		result.slots = nil
	}
	return result, true
}

func (work *Work) unionSlotCoverage(left, right slotCoverage) (slotCoverage, bool) {
	if !work.validSlotCoverage(left) || !work.validSlotCoverage(right) {
		return slotCoverage{}, false
	}
	switch {
	case len(left.targets) == 0:
		return right, true
	case len(right.targets) == 0:
		return left, true
	case sameSlotCoverageProvenance(left, right) || work.slotCoverageContainsProvenance(left, right):
		return left, true
	case work.slotCoverageContainsProvenance(right, left):
		return right, true
	}
	rows := make([]TargetRegion, 0, len(left.targets)+len(right.targets))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left.targets) || rightIndex < len(right.targets) {
		if rightIndex == len(right.targets) {
			rows = append(rows, left.targets[leftIndex])
			leftIndex++
			continue
		}
		if leftIndex == len(left.targets) {
			rows = append(rows, right.targets[rightIndex])
			rightIndex++
			continue
		}
		leftRow, rightRow := left.targets[leftIndex], right.targets[rightIndex]
		switch compareCoverageRows(leftRow, rightRow) {
		case -1:
			rows = append(rows, leftRow)
			leftIndex++
		case 1:
			rows = append(rows, rightRow)
			rightIndex++
		default:
			region, ok := work.unionSupport(leftRow.region, rightRow.region)
			if !ok {
				return slotCoverage{}, false
			}
			rows = append(rows, leftRow.WithRegion(region))
			leftIndex++
			rightIndex++
		}
	}
	return slotCoverage{targets: rows}, true
}

func (work *Work) canonicalCoverage(rows []TargetRegion, within support.Mask) (slotCoverage, bool) {
	if !work.live() || !within.Valid() || within.Manager() != work.composition.guards {
		return slotCoverage{}, false
	}
	if len(rows) == 0 || support.Empty(within) {
		return slotCoverage{}, true
	}
	// Most producer rows are already sorted and clipped. Equal Targets are
	// intentionally retained when their lineage/role differs: a carried
	// baseline and a transformed effect may overlap the same Target.
	orderedRows := true
	for index, row := range rows {
		slot, ok := row.target.Slot()
		if !ok || !work.composition.OwnsTarget(slot, row.target) || !row.region.Valid() || row.region.Manager() != work.composition.guards || support.Empty(row.region) {
			return slotCoverage{}, false
		}
		if !work.validCoverageRow(row) {
			return slotCoverage{}, false
		}
		if index != 0 && compareCoverageRows(row, rows[index-1]) < 0 {
			orderedRows = false
		}
		if !work.entailsSupport(row.region, within) {
			orderedRows = false
		}
	}
	ordered := rows
	if !orderedRows {
		ordered = append([]TargetRegion(nil), rows...)
		sort.SliceStable(ordered, func(left, right int) bool { return compareCoverageRows(ordered[left], ordered[right]) < 0 })
	}
	canonicalRows := true
	for index := 1; index < len(ordered); index++ {
		if ordered[index-1].target.Same(ordered[index].target) && sameCoverageMetadata(ordered[index-1], ordered[index]) {
			canonicalRows = false
			break
		}
	}
	if orderedRows && canonicalRows {
		return slotCoverage{targets: rows}, true
	}
	result := make([]TargetRegion, 0, len(ordered))
	for _, row := range ordered {
		region := row.region
		if !work.entailsSupport(region, within) {
			var ok bool
			region, ok = work.intersectSupport(region, within)
			if !ok {
				return slotCoverage{}, false
			}
		}
		if support.Empty(region) {
			continue
		}
		row = row.WithRegion(region)
		if len(result) != 0 && result[len(result)-1].target.Same(row.target) && sameCoverageMetadata(result[len(result)-1], row) {
			var ok bool
			result[len(result)-1].region, ok = work.unionSupport(result[len(result)-1].region, row.region)
			if !ok {
				return slotCoverage{}, false
			}
			continue
		}
		result = append(result, row)
	}
	return slotCoverage{targets: result}, true
}

// restrictSlotCoverage clips one already-canonical immutable slot relation.
// The common fold case reuses its backing rows when the destination support
// contains every authored region; it allocates only after the first actual
// clipping or deletion.
func (work *Work) restrictSlotCoverage(input slotCoverage, within support.Mask) (slotCoverage, bool) {
	if !work.live() || !within.Valid() || within.Manager() != work.composition.guards {
		return slotCoverage{}, false
	}
	if len(input.targets) == 0 || support.Empty(within) {
		return slotCoverage{}, true
	}
	var result []TargetRegion
	for index, row := range input.targets {
		slot, ok := row.target.Slot()
		if !ok || !work.composition.OwnsTarget(slot, row.target) || !row.region.Valid() || row.region.Manager() != work.composition.guards || support.Empty(row.region) || !work.validCoverageRow(row) || index > 0 && compareCoverageRows(row, input.targets[index-1]) < 0 {
			return slotCoverage{}, false
		}
		region := row.region
		if !work.entailsSupport(region, within) {
			region, ok = work.intersectSupport(region, within)
			if !ok {
				return slotCoverage{}, false
			}
		}
		if region.Equal(row.region) && result == nil {
			continue
		}
		if result == nil {
			result = make([]TargetRegion, 0, len(input.targets))
			result = append(result, input.targets[:index]...)
		}
		if !support.Empty(region) {
			result = append(result, row.WithRegion(region))
		}
	}
	if result == nil {
		return input, true
	}
	return slotCoverage{targets: result}, true
}

func (work *Work) mergeSlotCoverage(left, right slotCoverage, within support.Mask) (slotCoverage, bool) {
	if len(left.targets) == 0 {
		return work.restrictSlotCoverage(right, within)
	}
	if len(right.targets) == 0 {
		return work.restrictSlotCoverage(left, within)
	}
	left, ok := work.restrictSlotCoverage(left, within)
	if !ok {
		return slotCoverage{}, false
	}
	right, ok = work.restrictSlotCoverage(right, within)
	if !ok {
		return slotCoverage{}, false
	}
	if len(left.targets) == 0 {
		return right, true
	}
	if len(right.targets) == 0 {
		return left, true
	}
	// The union of two canonical relations is one operand whenever the first
	// relation covers every target/region of the second.  Prove that before
	// allocating the merged row vector; this is exact authored coverage
	// inclusion, not a semantic-root shortcut, so coverage-only wake/version
	// evidence remains unchanged by the caller's outer publication.
	if sameSlotCoverageProvenance(left, right) || work.slotCoverageContainsProvenance(left, right) {
		return left, true
	}
	if work.slotCoverageContainsProvenance(right, left) {
		return right, true
	}
	rows := make([]TargetRegion, 0, len(left.targets)+len(right.targets))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left.targets) || rightIndex < len(right.targets) {
		if rightIndex == len(right.targets) {
			rows = append(rows, left.targets[leftIndex])
			leftIndex++
			continue
		}
		if leftIndex == len(left.targets) {
			rows = append(rows, right.targets[rightIndex])
			rightIndex++
			continue
		}
		leftRow, rightRow := left.targets[leftIndex], right.targets[rightIndex]
		switch compareCoverageRows(leftRow, rightRow) {
		case -1:
			rows = append(rows, leftRow)
			leftIndex++
		case 1:
			rows = append(rows, rightRow)
			rightIndex++
		default:
			region, valid := work.unionSupport(leftRow.region, rightRow.region)
			if !valid {
				return slotCoverage{}, false
			}
			rows = append(rows, leftRow.WithRegion(region))
			leftIndex++
			rightIndex++
		}
	}
	result := slotCoverage{targets: rows}
	if sameSlotCoverageProvenance(result, left) {
		return left, true
	}
	if sameSlotCoverageProvenance(result, right) {
		return right, true
	}
	return result, true
}

// slotCoverageContains proves relation inclusion for canonical, sorted slot
// rows.  `super` contains `sub` exactly when every target in sub is present in
// super and its authored region is included by super's region.  The merge
// union is therefore super.  This helper is intentionally local to the one
// carrier fold and retains no index/cache authority.
func (work *Work) slotCoverageContains(super, sub slotCoverage) bool {
	if len(sub.targets) == 0 {
		return true
	}
	if len(super.targets) < len(sub.targets) {
		return false
	}
	superIndex, subIndex := 0, 0
	for superIndex < len(super.targets) && subIndex < len(sub.targets) {
		superRow, subRow := super.targets[superIndex], sub.targets[subIndex]
		switch {
		case superRow.target.Less(subRow.target):
			superIndex++
		case subRow.target.Less(superRow.target):
			return false
		default:
			if !superRow.target.Same(subRow.target) || !subRow.region.SameHandle(superRow.region) && !work.entailsSupport(subRow.region, superRow.region) {
				return false
			}
			superIndex++
			subIndex++
		}
	}
	return subIndex == len(sub.targets)
}

// slotCoverageContainsProvenance is the representation-level inclusion used
// while joining authored rows.  A baseline and an effect may have the same
// Target and region but are intentionally distinct rows, so the generic
// semantic inclusion above must not discard either one during that join.
func (work *Work) slotCoverageContainsProvenance(super, sub slotCoverage) bool {
	if !work.validSlotCoverage(super) || !work.validSlotCoverage(sub) {
		return false
	}
	if len(sub.targets) == 0 {
		return true
	}
	if len(super.targets) < len(sub.targets) {
		return false
	}
	superIndex, subIndex := 0, 0
	for superIndex < len(super.targets) && subIndex < len(sub.targets) {
		superRow, subRow := super.targets[superIndex], sub.targets[subIndex]
		switch compareCoverageRows(superRow, subRow) {
		case -1:
			superIndex++
		case 1:
			return false
		default:
			if subRow.region.SameHandle(superRow.region) || work.entailsSupport(subRow.region, superRow.region) {
				superIndex++
				subIndex++
				continue
			}
			return false
		}
	}
	return subIndex == len(sub.targets)
}

// validContributionSurface is the common private role fence for all lifted
// comparisons.  A PointState and PointRHS use the same State+C storage as a
// RuleContribution, but they may retain physical root fibers outside their
// support.  Those fibers are intentionally not part of this surface.
func (work *Work) validContributionSurface(state State, coverage contributionCoverage) bool {
	return work != nil && work.live() && work.OwnsState(state) && !state.previewMarked() && !state.contributionMarked() && work.validContributionCoverage(state, coverage)
}

// lessOrEqContributionSurface proves the lifted partial order over one
// private State+C pair.  Support inclusion is the outer feasibility proof;
// each typed Binding expands opaque Target rows only for the keys it owns.
// It deliberately never calls raw State order, which would inspect latent
// physical point branches or totalize an absent authored cell to Default.
func (work *Work) lessOrEqContributionSurface(leftState State, leftCoverage contributionCoverage, rightState State, rightCoverage contributionCoverage) bool {
	if !work.validContributionSurface(leftState, leftCoverage) || !work.validContributionSurface(rightState, rightCoverage) || !work.liveFor(leftState, rightState) || !work.entailsSupport(leftState.support, rightState.support) {
		return false
	}
	if sameState(leftState, rightState) && sameContributionCoverage(leftCoverage, rightCoverage) {
		return true
	}
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			return false
		}
		physical := shape.Slot(position)
		ordered, valid := slot.LessOrEqContributionUnder(leftState.roots[position], rightState.roots[position], leftState.support, rightState.support, coverageRows(leftCoverage.slot(physical)), coverageRows(rightCoverage.slot(physical)))
		if !valid || !ordered {
			return false
		}
	}
	return true
}

func (work *Work) ascentOrderedContributionSurface(leftState State, leftCoverage contributionCoverage, rightState State, rightCoverage contributionCoverage) bool {
	if !work.validContributionSurface(leftState, leftCoverage) || !work.validContributionSurface(rightState, rightCoverage) || !work.liveFor(leftState, rightState) || !work.entailsSupport(leftState.support, rightState.support) {
		return false
	}
	if sameState(leftState, rightState) && sameContributionCoverage(leftCoverage, rightCoverage) {
		return true
	}
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			return false
		}
		physical := shape.Slot(position)
		ordered, valid := slot.AscentOrderedContributionUnder(leftState.roots[position], rightState.roots[position], leftState.support, rightState.support, coverageRows(leftCoverage.slot(physical)), coverageRows(rightCoverage.slot(physical)))
		if !valid || !ordered {
			return false
		}
	}
	return true
}

func (work *Work) equalContributionSurface(leftState State, leftCoverage contributionCoverage, rightState State, rightCoverage contributionCoverage) bool {
	if work.validContributionSurface(leftState, leftCoverage) && work.validContributionSurface(rightState, rightCoverage) && work.liveFor(leftState, rightState) && sameState(leftState, rightState) && sameContributionCoverage(leftCoverage, rightCoverage) {
		return true
	}
	return work.lessOrEqContributionSurface(leftState, leftCoverage, rightState, rightCoverage) && work.lessOrEqContributionSurface(rightState, rightCoverage, leftState, leftCoverage)
}

func coverageRows(slot slotCoverage) SlotCoverage { return SlotCoverage{value: &slot} }

// mergeSelectedContributionSurface is the sole physical recurrence
// transaction. Its inputs are private paired State+C headers, never a raw
// State relabelled as an RHS.  Its output is physically closed to exact C even
// when the current PointState retains a latent root outside support.
//
// Keeping this transaction role-neutral is important: recurrence has one
// carrier authority and one exact-C law for every nominal point role that
// reaches it.
func (work *Work) mergeSelectedContributionSurface(kind MergeKind, currentState State, currentCoverage contributionCoverage, selectedState State, selectedCoverage contributionCoverage, exactState State, exactCoverage contributionCoverage, exactClosed bool, selected MergeScope) (State, ChangeSet, bool) {
	if !work.validContributionSurface(currentState, currentCoverage) || !work.validContributionSurface(selectedState, selectedCoverage) || !work.validContributionSurface(exactState, exactCoverage) || !selected.validFor(work.composition, kind) {
		return State{}, ChangeSet{}, false
	}
	if kind != Widen && kind != Narrow || !work.liveFor(currentState, selectedState) || !work.liveFor(currentState, exactState) {
		return State{}, ChangeSet{}, false
	}
	hasSelected := !selected.members.Empty()
	if kind == Widen && !selectedState.support.Equal(exactState.support) || hasSelected && kind == Widen && !work.entailsSupport(currentState.support, selectedState.support) || hasSelected && kind == Narrow && (!work.entailsSupport(selectedState.support, currentState.support) || !selectedState.support.Equal(exactState.support)) {
		return State{}, ChangeSet{}, false
	}
	// Narrow has one exact desired RHS. Accepting an independently supplied
	// selected operand would let a foreign authored surface or value influence
	// selected keys before the final exact-C close.
	if kind == Narrow && !work.equalContributionSurface(selectedState, selectedCoverage, exactState, exactCoverage) {
		return State{}, ChangeSet{}, false
	}
	// Narrow publishes a mixed surface whose unselected slots are closed
	// directly from exactRight. Proving descent only on the selected keys is
	// therefore not enough: an unselected Factor could grow in exactRight and
	// make the published transition globally incomparable to its predecessor.
	// lessOrEqContributionSurface is the closed lifted order authority, so the
	// whole exact surface must be below current before any slot prepares a
	// candidate.
	if kind == Narrow && !work.lessOrEqContributionSurface(exactState, exactCoverage, currentState, currentCoverage) {
		return State{}, ChangeSet{}, false
	}
	// This proof applies even to an empty selection, whose implementation is
	// an all-slot exact close rather than a typed Widen. The operation still
	// carries Widen phase meaning and therefore may not erase authored
	// presence before that publication cut.
	if kind == Widen {
		if !work.lessOrEqContributionSurface(exactState, exactCoverage, selectedState, selectedCoverage) {
			return State{}, ChangeSet{}, false
		}
		for position, slot := range work.slots {
			if !work.live() || slot == nil {
				return State{}, ChangeSet{}, false
			}
			physical := shape.Slot(position)
			if !slot.ContributionPresenceIncludedUnder(currentState.support, exactState.support, coverageRows(currentCoverage.slot(physical)), coverageRows(exactCoverage.slot(physical))) ||
				!slot.ContributionPresenceIncludedUnder(selectedState.support, exactState.support, coverageRows(selectedCoverage.slot(physical)), coverageRows(exactCoverage.slot(physical))) {
				return State{}, ChangeSet{}, false
			}
		}
	}
	selectedSplit, ok := work.threeSupport(currentState.support, selectedState.support)
	if !ok {
		return State{}, ChangeSet{}, false
	}
	exactSplit, ok := work.threeSupport(currentState.support, exactState.support)
	if !ok {
		return State{}, ChangeSet{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return State{}, ChangeSet{}, false
	}
	patches, held := work.beginPatches(len(work.slots))
	if !held {
		delta.Discard()
		return State{}, ChangeSet{}, false
	}
	defer work.releasePatches(&patches)
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
		physical := shape.Slot(position)
		var change ChangeHandle
		var valid bool
		if hasSelected && selected.members.Test(position) {
			change, valid = slot.MergeSelectedContributionUnder(kind, selected.scopes[position], currentState.roots[position], selectedState.roots[position], exactState.roots[position], selectedSplit, exactSplit, coverageRows(currentCoverage.slot(physical)), coverageRows(selectedCoverage.slot(physical)), coverageRows(exactCoverage.slot(physical)), delta)
		} else if exactClosed {
			// The exact RHS owner has already physically closed this root to its
			// authored surface. Reusing that issued root through the typed
			// replacement path preserves the current-to-exact ChangeSet while
			// avoiding a second CloseContribution/EqualUnder reconstruction.
			// Selected slots still take the Widen/Narrow path above because their
			// result is newly derived and therefore not covered by this proof.
			change, valid = slot.ReplaceUnder(currentState.roots[position], exactState.roots[position], exactSplit, delta)
		} else {
			change, valid = slot.CloseContributionUnder(currentState.roots[position], exactState.roots[position], exactSplit, coverageRows(exactCoverage.slot(physical)), delta)
		}
		if !valid || !work.acceptInto(&patches, currentState, change, delta) {
			delta.Discard()
			if valid {
				dropPatches(patches)
			}
			return State{}, ChangeSet{}, false
		}
	}
	added, removed := emptyMask(work.composition.guards), emptyMask(work.composition.guards)
	if !added.Valid() || !removed.Valid() {
		delta.Discard()
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	if !hasSelected {
		// Factor-free recurrence is an exact publication. Unlike a selected
		// Narrow it may replace support in either direction, so preserve both
		// structural sides exactly as Replace would.
		added, removed = exactSplit.RightOnly(), exactSplit.LeftOnly()
	} else if kind == Widen {
		added = exactSplit.RightOnly()
	} else {
		removed = exactSplit.LeftOnly()
	}
	state, changes, ok := work.commit(currentState, patches, exactState.support, added, removed, delta)
	if !ok {
		return State{}, ChangeSet{}, false
	}
	// A Narrow is a descent by definition of the operation, not by what its
	// support happened to do. Its support entailment is satisfied by equality
	// and its lifted order permits a strict coverage shrink, so the direction
	// is issued here by the operation that owns the phase.
	if kind == Narrow {
		changes.set.Direction |= change.Descends
	}
	return state, changes, true
}

// TransportPointContribution transports a semantic PointState through the
// total-Default State relation, then closes the result to transported RHS
// coverage.  The point source is not a lifted partial RuleContribution:
// absent reachable source cells are the Factor Default during omega.  C
// decides only which target cells become RHS-present afterwards.
func (work *Work) TransportPointContribution(input Contribution, pre support.Mask, omega ReindexPlan, post support.Mask) (Contribution, bool) {
	if !work.live() || work.reindexing || !work.admittedContribution(input) || !omega.validFor(work.composition) || !input.state.scope.same(omega.source()) || !validBoundaryMask(pre, input.state.scope) || !validBoundaryMask(post, omega.target()) {
		return Contribution{}, false
	}
	if omega.identity() && pre.IsTrue() && post.IsTrue() {
		return input, true
	}
	if omega.coordinateIdentity() && pre.IsTrue() && post.IsTrue() {
		return work.rescopeContribution(input, omega.target())
	}
	work.reindexing = true
	defer func() { work.reindexing = false }()
	sourceSupport, ok := work.intersectSupport(input.state.support, pre)
	if !ok {
		return Contribution{}, false
	}
	reindexedSupport, ok := work.reindexSupport(sourceSupport, omega.relation)
	if !ok {
		return Contribution{}, false
	}
	targetSupport, ok := work.intersectSupport(reindexedSupport, post)
	if !ok {
		return Contribution{}, false
	}
	coverage, ok := work.transportContributionCoverage(input.coverage, pre, omega, post, targetSupport)
	if !ok {
		return Contribution{}, false
	}
	split, ok := work.threeSupport(input.state.support, targetSupport)
	if !ok {
		return Contribution{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return Contribution{}, false
	}
	patches, held := work.beginPatches(len(work.slots))
	if !held {
		delta.Discard()
		return Contribution{}, false
	}
	defer work.releasePatches(&patches)
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, false
		}
		physical := shape.Slot(position)
		change, valid := slot.ReindexPointContributionUnder(input.state.roots[position], sourceSupport, targetSupport, omega.relation, coverageRows(coverage.slot(physical)), delta)
		if !valid {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, false
		}
		if !work.acceptInto(&patches, input.state, change, delta) {
			delta.Discard()
			return Contribution{}, false
		}
	}
	next, _, committed := work.commit(input.state, patches, targetSupport, split.RightOnly(), split.LeftOnly(), delta)
	if !committed {
		return Contribution{}, false
	}
	next.scope = omega.target()
	result, valid := work.admitDerivedContribution(input, next, coverage)
	return result, valid
}

// TransportRuleContribution is the lifted-partial transport for an already
// authored RuleContribution.  Only source-present C fibers participate;
// Present(Default) remains distinct from Absent through noninjective reindex.
// No runtime PointState path calls this operation unless it has a genuine
// RuleContribution source role.
func (work *Work) TransportRuleContribution(input Contribution, pre support.Mask, omega ReindexPlan, post support.Mask) (Contribution, bool) {
	if !work.live() || work.reindexing || !work.admittedContribution(input) || !omega.validFor(work.composition) || !input.state.scope.same(omega.source()) || !validBoundaryMask(pre, input.state.scope) || !validBoundaryMask(post, omega.target()) {
		return Contribution{}, false
	}
	if omega.identity() && pre.IsTrue() && post.IsTrue() {
		return input, true
	}
	work.reindexing = true
	defer func() { work.reindexing = false }()
	sourceSupport, ok := work.intersectSupport(input.state.support, pre)
	if !ok {
		return Contribution{}, false
	}
	reindexedSupport, ok := work.reindexSupport(sourceSupport, omega.relation)
	if !ok {
		return Contribution{}, false
	}
	targetSupport, ok := work.intersectSupport(reindexedSupport, post)
	if !ok {
		return Contribution{}, false
	}
	coverage, ok := work.transportContributionCoverage(input.coverage, pre, omega, post, targetSupport)
	if !ok {
		return Contribution{}, false
	}
	split, ok := work.threeSupport(input.state.support, targetSupport)
	if !ok {
		return Contribution{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return Contribution{}, false
	}
	patches, held := work.beginPatches(len(work.slots))
	if !held {
		delta.Discard()
		return Contribution{}, false
	}
	defer work.releasePatches(&patches)
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, false
		}
		physical := shape.Slot(position)
		change, valid := slot.ReindexContributionUnder(input.state.roots[position], sourceSupport, targetSupport, omega.relation, coverageRows(input.coverage.slot(physical)), coverageRows(coverage.slot(physical)), delta)
		if !valid {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, false
		}
		if !work.acceptInto(&patches, input.state, change, delta) {
			delta.Discard()
			return Contribution{}, false
		}
	}
	next, _, committed := work.commit(input.state, patches, targetSupport, split.RightOnly(), split.LeftOnly(), delta)
	if !committed {
		return Contribution{}, false
	}
	next.scope = omega.target()
	result, valid := work.admitDerivedContribution(input, next, coverage)
	return result, valid
}

// CoverageRegion is one slot-local authorship change. It is structural wake
// evidence only and must never be converted into a semantic FactorRegion.
//
// The producer holds the direction of every row it emits, so the two halves
// of the authorship split are carried separately: added names the region this
// slot gained, removed the region it lost. Region remains their union for the
// consumers that only need to know that something moved.
type CoverageRegion struct {
	slot    shape.Slot
	region  support.Mask
	added   support.Mask
	removed support.Mask
}

func (row CoverageRegion) Slot() shape.Slot      { return row.slot }
func (row CoverageRegion) Region() support.Mask  { return row.region }
func (row CoverageRegion) Added() support.Mask   { return row.added }
func (row CoverageRegion) Removed() support.Mask { return row.removed }

// CoverageChangeSet is one publication's authorship delta. It carries the
// change vocabulary the producer classified, the slot set it touched, and the
// row projections consumers read.
//
// slots borrows this Work's reusable set, so it is stamped with the operation
// generation that issued it: a read taken after the buffer was recycled is
// refused rather than answered with recycled words.
type CoverageChangeSet struct {
	composition *Composition
	rows        []CoverageRegion
	targets     []CoverageTargetRegion
	set         change.Set
	slab        *slotSetSlab
	generation  uint64
}

// Evidence returns the classified change facts this delta carries.
func (set CoverageChangeSet) Evidence() change.Set { return set.set }

// Slots returns the borrowed slot set this delta touched. It refuses a stale
// read: the set lives only until the issuing Work recycles its buffers.
func (set CoverageChangeSet) Slots() (change.Slots, bool) {
	if set.slab == nil || set.slab.generation != set.generation {
		return change.Slots{}, false
	}
	return set.slab.set, true
}

// CoverageTargetRegion is the exact Target-local authorship delta retained
// for incremental structural transport. Demand routing deliberately consumes
// only the coarser CoverageRegion projection; typed Binding is the only layer
// allowed to resolve this Target to concrete keys.
type CoverageTargetRegion struct {
	slot   shape.Slot
	target Target
	region support.Mask
}

func (row CoverageTargetRegion) Slot() shape.Slot     { return row.slot }
func (row CoverageTargetRegion) Target() Target       { return row.target }
func (row CoverageTargetRegion) Region() support.Mask { return row.region }

func (set CoverageChangeSet) Count() int { return len(set.rows) }
func (set CoverageChangeSet) TargetCount() int {
	return len(set.targets)
}
func (set CoverageChangeSet) At(index int) (CoverageRegion, bool) {
	if set.composition == nil || index < 0 || index >= len(set.rows) {
		return CoverageRegion{}, false
	}
	return set.rows[index], true
}
func (set CoverageChangeSet) TargetAt(index int) (CoverageTargetRegion, bool) {
	if set.composition == nil || index < 0 || index >= len(set.targets) {
		return CoverageTargetRegion{}, false
	}
	return set.targets[index], true
}
func (composition *Composition) OwnsCoverageChangeSet(set CoverageChangeSet) bool {
	return composition != nil && set.composition == composition
}

// coverageChangesSurface derives one Target-local delta from two private
// compact coverage headers. It is shared by closed RuleContributions and
// semantic PointStates; only the latter can retain latent payload outside
// support, which never enters this coverage calculation.
// accumulateRegion widens one half of an authorship split. The first
// nonempty contribution owns the region; every later one unions into it.
func (work *Work) accumulateRegion(into support.Mask, has bool, next support.Mask) (support.Mask, bool, bool) {
	if support.Empty(next) {
		return into, has, true
	}
	if !has {
		return next, true, true
	}
	joined, ok := work.unionSupport(into, next)
	return joined, true, ok
}

func (work *Work) coverageChangesSurface(left, right contributionCoverage, retainTargets bool) (CoverageChangeSet, bool) {
	if !work.live() || left.composition != work.composition || right.composition != work.composition {
		return CoverageChangeSet{}, false
	}
	joined := work.borrowSlotSet()
	defer work.releaseSlotSet()
	slab := work.borrowSlotSet()
	defer work.releaseSlotSet()
	if joined == nil || slab == nil || !left.unionOccupiedInto(&joined.set) || !right.unionOccupiedInto(&joined.set) {
		return CoverageChangeSet{}, false
	}
	// The producer classifies its own delta. A row present only on the left is
	// authorship this publication lost, a row present only on the right is
	// authorship it gained, and a row on both sides splits into exactly those
	// two halves. Unioning them into one direction-free mask would destroy the
	// only ordering fact this operation holds.
	result := CoverageChangeSet{composition: work.composition, set: change.Set{Direction: change.Known}, slab: slab, generation: slab.generation}
	empty := emptyMask(work.composition.guards)
	if !empty.Valid() {
		return CoverageChangeSet{}, false
	}
	for position, more := joined.set.Next(0); more; position, more = joined.set.Next(position + 1) {
		leftSlot, rightSlot := left.slot(shape.Slot(position)), right.slot(shape.Slot(position))
		if sameSlotCoverage(leftSlot, rightSlot) {
			continue
		}
		added, removed := empty, empty
		hasAdded, hasRemoved := false, false
		leftIndex, rightIndex := 0, 0
		for leftIndex < len(leftSlot.targets) || rightIndex < len(rightSlot.targets) {
			var target Target
			var lost, gained support.Mask
			switch {
			case rightIndex == len(rightSlot.targets) || leftIndex < len(leftSlot.targets) && leftSlot.targets[leftIndex].target.Less(rightSlot.targets[rightIndex].target):
				target, lost, gained = leftSlot.targets[leftIndex].target, leftSlot.targets[leftIndex].region, empty
				leftIndex++
			case leftIndex == len(leftSlot.targets) || rightSlot.targets[rightIndex].target.Less(leftSlot.targets[leftIndex].target):
				target, lost, gained = rightSlot.targets[rightIndex].target, empty, rightSlot.targets[rightIndex].region
				rightIndex++
			default:
				split, ok := work.threeSupport(leftSlot.targets[leftIndex].region, rightSlot.targets[rightIndex].region)
				if !ok {
					return CoverageChangeSet{}, false
				}
				target, lost, gained = leftSlot.targets[leftIndex].target, split.LeftOnly(), split.RightOnly()
				leftIndex++
				rightIndex++
			}
			lostEmpty, gainedEmpty := support.Empty(lost), support.Empty(gained)
			if lostEmpty && gainedEmpty {
				continue
			}
			var ok bool
			if added, hasAdded, ok = work.accumulateRegion(added, hasAdded, gained); !ok {
				return CoverageChangeSet{}, false
			}
			if removed, hasRemoved, ok = work.accumulateRegion(removed, hasRemoved, lost); !ok {
				return CoverageChangeSet{}, false
			}
			if !retainTargets {
				continue
			}
			changed := gained
			switch {
			case gainedEmpty:
				changed = lost
			case lostEmpty:
			default:
				if changed, ok = work.unionSupport(lost, gained); !ok {
					return CoverageChangeSet{}, false
				}
			}
			result.targets = append(result.targets, CoverageTargetRegion{slot: shape.Slot(position), target: target, region: changed})
		}
		if !hasAdded && !hasRemoved {
			continue
		}
		region := added
		if !hasAdded {
			region = removed
		} else if hasRemoved {
			var ok bool
			if region, ok = work.unionSupport(added, removed); !ok {
				return CoverageChangeSet{}, false
			}
		}
		if support.Empty(region) {
			continue
		}
		result.rows = append(result.rows, CoverageRegion{slot: shape.Slot(position), region: region, added: added, removed: removed})
		slab.set.Set(position)
		if hasAdded {
			result.set.Reasons |= change.SupportAdded
			result.set.Direction |= change.Ascends
		}
		if hasRemoved {
			result.set.Reasons |= change.SupportRemoved
			result.set.Direction |= change.Descends
		}
	}
	if len(result.rows) != 0 {
		result.set.Reasons |= change.AuthorshipChanged
	}
	return result, true
}

package carrier

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// RuleContribution is an independently authored, closed rule result.  Its
// compact Target x Guard coverage is the only lifted presence authority and
// its roots are physically masked to that surface.  The wrapper is nominal on
// purpose: a semantic PointState cannot be folded as an authored rule result
// without first crossing LiftRuleContribution.
//
// Contribution remains the compatibility representation while executor
// storage migrates.  It has exactly the same closed invariant; this wrapper
// adds the role proof without copying roots, coverage, or typed payloads.
type RuleContribution struct {
	value    Contribution
	roleSeal *contributionSeal
}

func (rule RuleContribution) State() State          { return rule.value.State() }
func (rule RuleContribution) Support() support.Mask { return rule.value.Support() }
func (rule RuleContribution) Scope() Scope          { return rule.value.Scope() }
func (rule RuleContribution) HandleAt(slot shape.Slot) (RootHandle, bool) {
	return rule.value.HandleAt(slot)
}

// Valid is structural only.  Work-owned operations additionally require the
// private role seal, just as legacy Contribution operations require their
// evaluator-local contribution seal.
func (rule RuleContribution) Valid() bool { return rule.roleSeal != nil && rule.value.Valid() }

// PointState is one published semantic point value.  It owns no second fact
// plane: state and coverage are the same immutable headers used by a closed
// RuleContribution.  Unlike RuleContribution, its roots may retain latent
// values outside State.Support after a point-to-point boundary.  Every
// semantic Point operation respects that support; only LiftRuleContribution
// makes the physical mask durable for a later authored fold.
type PointState struct {
	state     State
	coverage  contributionCoverage
	roleSeal  *contributionSeal
	authority *stateAuthority
	// closed is a carrier-proven fast path, not an alternate semantic plane.
	// A nonidentity point boundary clears it because immutable roots may now
	// retain source fibers outside the new support.
	closed bool
}

func (point PointState) State() State          { return point.state }
func (point PointState) Support() support.Mask { return point.state.Support() }
func (point PointState) Scope() Scope          { return point.state.Scope() }
func (point PointState) HandleAt(slot shape.Slot) (RootHandle, bool) {
	return point.state.HandleAt(slot)
}
func (point PointState) Valid() bool {
	return point.roleSeal != nil && point.authority != nil && point.state.authority == point.authority && point.state.live() && point.coverage.validFor(point.state)
}

// PointRHS is one semantic point-fold accumulator.  Its base is a PointState,
// not a RuleContribution: roots may therefore retain latent fibers outside
// the point's current support.  Closed RuleContributions overlay that base
// only at their authored surface while their support is already contained by
// the base support.  A support-growing rule or a second environment base must
// use JoinPointRHS, the explicit total semantic join.
//
// This is still one root+coverage header, not a second fact plane or an
// accumulator tree.  The role distinguishes the directional semantic overlay
// from the closed ACI RuleContribution algebra.
type PointRHS struct {
	point    PointState
	roleSeal *contributionSeal
}

func (rhs PointRHS) State() State          { return rhs.point.State() }
func (rhs PointRHS) Support() support.Mask { return rhs.point.Support() }
func (rhs PointRHS) Scope() Scope          { return rhs.point.Scope() }
func (rhs PointRHS) HandleAt(slot shape.Slot) (RootHandle, bool) {
	return rhs.point.HandleAt(slot)
}
func (rhs PointRHS) Valid() bool { return rhs.roleSeal != nil && rhs.point.Valid() }

// OwnsRuleContribution, OwnsPointState, and OwnsPointRHS are the public
// nominal ownership cuts. They deliberately expose no raw State or coverage
// unwrap: callers can establish that an opaque role belongs to this carrier
// without acquiring a second construction route for its root+surface pair.
func (work *Work) OwnsRuleContribution(rule RuleContribution) bool {
	return work.admittedRuleContribution(rule)
}

func (work *Work) OwnsPointState(point PointState) bool {
	return work.admittedPointState(point)
}

func (work *Work) OwnsPointRHS(rhs PointRHS) bool {
	return work.admittedPointRHS(rhs)
}

func (work *Work) admittedRuleContribution(rule RuleContribution) bool {
	return work != nil && work.live() && rule.roleSeal == work.contributionSeal && work.admittedContribution(rule.value)
}

func (work *Work) admittedPointState(point PointState) bool {
	if work == nil || !work.live() || point.roleSeal != work.contributionSeal || point.authority == nil || point.state.authority != point.authority || !work.OwnsState(point.state) || point.coverage.composition != work.composition {
		return false
	}
	return len(point.coverage.slots) == 0 || len(point.coverage.slots) == work.composition.Count()
}

func (work *Work) admittedPointRHS(rhs PointRHS) bool {
	return work != nil && work.live() && rhs.roleSeal == work.contributionSeal && work.admittedPointState(rhs.point)
}

// EqualRuleContribution and LessOrEqRuleContribution expose the closed
// lifted algebra through its nominal role. They compare only C-present cells
// (where undefined payload means Present(Default)); physical residue cannot
// participate because RuleContribution issuance has already closed it.
func (work *Work) EqualRuleContribution(left, right RuleContribution) bool {
	return work.admittedRuleContribution(left) && work.admittedRuleContribution(right) && work.equalContributionSurface(left.value.state, left.value.coverage, right.value.state, right.value.coverage)
}

func (work *Work) LessOrEqRuleContribution(left, right RuleContribution) bool {
	return work.admittedRuleContribution(left) && work.admittedRuleContribution(right) && work.lessOrEqContributionSurface(left.value.state, left.value.coverage, right.value.state, right.value.coverage)
}

// EqualPointRHS and LessOrEqPointRHS use the same lifted surface relation as
// a RuleContribution while deliberately ignoring latent PointState root
// fibers outside support. A raw State comparison would make those physical
// implementation details observable and is therefore not a valid RHS order.
func (work *Work) EqualPointRHS(left, right PointRHS) bool {
	return work.admittedPointRHS(left) && work.admittedPointRHS(right) && work.equalContributionSurface(left.point.state, left.point.coverage, right.point.state, right.point.coverage)
}

func (work *Work) LessOrEqPointRHS(left, right PointRHS) bool {
	return work.admittedPointRHS(left) && work.admittedPointRHS(right) && work.lessOrEqContributionSurface(left.point.state, left.point.coverage, right.point.state, right.point.coverage)
}

// EqualPointState is the semantic publication equality. It retains compact C
// as presence authority and ignores any root fibers outside outer support.
func (work *Work) EqualPointState(left, right PointState) bool {
	return work.admittedPointState(left) && work.admittedPointState(right) && work.equalContributionSurface(left.state, left.coverage, right.state, right.coverage)
}

// LessOrEqPointRHSPoint and LessOrEqPointStateRHS are explicit cross-role
// lifecycle comparisons. Keeping their direction in the name prevents the
// executor from silently using raw State order at a Point/RHS boundary.
func (work *Work) LessOrEqPointRHSPoint(left PointRHS, right PointState) bool {
	return work.admittedPointRHS(left) && work.admittedPointState(right) && work.lessOrEqContributionSurface(left.point.state, left.point.coverage, right.state, right.coverage)
}

func (work *Work) LessOrEqPointStateRHS(left PointState, right PointRHS) bool {
	return work.admittedPointState(left) && work.admittedPointRHS(right) && work.lessOrEqContributionSurface(left.state, left.coverage, right.point.state, right.point.coverage)
}

// CoverageChangesPointStates derives the exact Target-local authored delta
// between two nominal point publications. It is intentionally separate from
// semantic ChangeSet: a Present(Default) or coverage-only ascent must wake a
// structural seminaive edge even when its sparse payload root is unchanged.
func (work *Work) CoverageChangesPointStates(previous, current PointState) (CoverageChangeSet, bool) {
	if !work.admittedPointState(previous) || !work.admittedPointState(current) || !work.liveFor(previous.state, current.state) {
		return CoverageChangeSet{}, false
	}
	return work.coverageChangesSurface(previous.coverage, current.coverage, true)
}

// CoverageWakeChangesPointStates is the demand-facing projection of a nominal
// point coverage change. It keeps the exact same slot/guard wake rows as
// CoverageChangesPointStates, but intentionally drops Target-local rows: only
// a structural seminaive edge needs those opaque target capabilities. This
// avoids allocating a target delta merely to wake demand.
func (work *Work) CoverageWakeChangesPointStates(previous, current PointState) (CoverageChangeSet, bool) {
	if !work.admittedPointState(previous) || !work.admittedPointState(current) || !work.liveFor(previous.state, current.state) {
		return CoverageChangeSet{}, false
	}
	return work.coverageChangesSurface(previous.coverage, current.coverage, false)
}

// AsRuleContribution is the one staging bridge from the existing closed
// Contribution API.  It changes neither semantic root nor coverage; runtime
// migration can replace this bridge with typed Finish/transport return values
// without changing the algebra.
func (work *Work) AsRuleContribution(value Contribution) (RuleContribution, bool) {
	if work == nil || !work.admittedContribution(value) {
		return RuleContribution{}, false
	}
	rule := RuleContribution{value: value, roleSeal: work.contributionSeal}
	return rule, work.admittedRuleContribution(rule)
}

// EmptyPointState is the nominal point-base route.  It deliberately reuses
// EmptyContribution's composition-initial proof, so arbitrary raw State can
// never be relabelled as a Point base or an empty authored RHS.
func (work *Work) EmptyPointState(state State) (PointState, bool) {
	empty, ok := work.EmptyContribution(state)
	if !ok {
		return PointState{}, false
	}
	rule, ok := work.AsRuleContribution(empty)
	if !ok {
		return PointState{}, false
	}
	return work.PointStateFromRuleContribution(rule)
}

// PointStateFromRuleContribution publishes a closed rule result as a semantic
// point without rebuilding its roots.  The PointState remembers that this
// exact immutable root vector is already closed, making a later immediate
// lift allocation-free.
func (work *Work) PointStateFromRuleContribution(rule RuleContribution) (PointState, bool) {
	if !work.admittedRuleContribution(rule) {
		return PointState{}, false
	}
	point := PointState{state: rule.value.state, coverage: rule.value.coverage, roleSeal: work.contributionSeal, authority: rule.value.authority, closed: true}
	return point, work.admittedPointState(point)
}

// PointRHSFromRuleContribution starts a semantic RHS from one closed rule.
// It is useful for rule-only points; PointRHSFromPointState is the zero-copy
// environment-base path.
func (work *Work) PointRHSFromRuleContribution(rule RuleContribution) (PointRHS, bool) {
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok {
		return PointRHS{}, false
	}
	return work.PointRHSFromPointState(point)
}

// PointRHSFromPointState adopts one semantic environment base without
// rebuilding a root.  This is the hot guard-route cut: a post-filtered point
// retains its raw root header until an operation genuinely needs to modify an
// authored cell.
func (work *Work) PointRHSFromPointState(point PointState) (PointRHS, bool) {
	if !work.admittedPointState(point) {
		return PointRHS{}, false
	}
	rhs := PointRHS{point: point, roleSeal: work.contributionSeal}
	return rhs, work.admittedPointRHS(rhs)
}

// closedInitialPoint is the only zero-copy identity-adoption proof.  Empty
// authored coverage alone is insufficient: a PointState may carry latent
// semantic root data outside its support, and adopting that as an empty RHS
// would later make it observable after support growth.  The immutable
// composition-initial root vector plus the closed bit proves there is no such
// latent payload.
func (work *Work) closedInitialPoint(point PointState) bool {
	return work.admittedPointState(point) && point.closed && len(point.coverage.slots) == 0 && sameRootVector(point.state.roots, work.composition.initial)
}

// AddPointEnvironment folds one environment PointState into a PointRHS.
// A proven initial, C-empty base can adopt the environment header directly.
// A contained environment takes the directional Point-surface overlay below;
// only a support-growing environment needs closed lifted confluence. In
// particular, this never raw-joins State values, because C-absent cells are
// not semantic Default operands in the RHS algebra.
func (work *Work) AddPointEnvironment(rhs PointRHS, environment PointState) (PointRHS, bool) {
	if !work.admittedPointRHS(rhs) || !work.admittedPointState(environment) || !work.liveFor(rhs.point.state, environment.state) {
		return PointRHS{}, false
	}
	if work.closedInitialPoint(rhs.point) && rhs.point.state.support.Entails(environment.state.support) {
		return work.PointRHSFromPointState(environment)
	}
	if environment.state.support.Entails(rhs.point.state.support) {
		return work.OverlayPointEnvironment(rhs, environment)
	}
	right, ok := work.PointRHSFromPointState(environment)
	if !ok {
		return PointRHS{}, false
	}
	return work.JoinPointRHS(rhs, right)
}

// AddRuleContribution is the sole PointRHS rule-fold operation.  It chooses
// the only three lawful cases: adopt into a proven empty initial base,
// directional sparse overlay when the rule cannot grow support, or closed
// lifted confluence when it can.  No caller needs to decide whether to use a
// raw State merge or a closed contribution merge.
func (work *Work) AddRuleContribution(rhs PointRHS, rule RuleContribution) (PointRHS, bool) {
	if !work.admittedPointRHS(rhs) || !work.admittedRuleContribution(rule) || !work.liveFor(rhs.point.state, rule.value.state) {
		return PointRHS{}, false
	}
	if work.closedInitialPoint(rhs.point) && rhs.point.state.support.Entails(rule.value.state.support) {
		return work.PointRHSFromRuleContribution(rule)
	}
	if rule.value.state.support.Entails(rhs.point.state.support) {
		return work.OverlayRuleContribution(rhs, rule)
	}
	return work.JoinRuleContribution(rhs, rule)
}

// ProjectPointState keeps one selected factor root and selected coverage row,
// replacing every other factor with its immutable initial root.  It is a
// PointState operation, so it intentionally preserves the source support and
// does not physically close the selected root; a later factor edge performs
// TransportPointState then LiftRuleContribution before AddRuleContribution.
func (work *Work) ProjectPointState(point PointState, selected shape.Slot) (PointState, bool) {
	if !work.admittedPointState(point) || !work.composition.shape.ValidSlot(selected) {
		return PointState{}, false
	}
	roots := make([]RootHandle, len(point.state.roots))
	copy(roots, work.composition.initial)
	roots[int(selected)] = point.state.roots[int(selected)]
	coverage := contributionCoverage{composition: work.composition}
	if row := point.coverage.slot(selected); len(row.targets) != 0 {
		coverage.slots = make([]slotCoverage, work.composition.Count())
		coverage.slots[int(selected)] = row
	}
	state := point.state
	state.roots = roots
	projected := PointState{
		state:     state,
		coverage:  coverage,
		roleSeal:  work.contributionSeal,
		authority: state.authority,
		closed:    point.closed,
	}
	return projected, work.admittedPointState(projected)
}

// OverlayRuleContribution applies a sparse closed RuleContribution to one
// already-supported semantic PointRHS. It is a nominal wrapper around the
// one private Point-surface overlay transaction shared with environments.
// A rule whose support grows that output must take JoinRuleContribution
// instead, so an old latent source branch can never become semantic merely
// because support grew.
func (work *Work) OverlayRuleContribution(rhs PointRHS, rule RuleContribution) (PointRHS, bool) {
	if !work.admittedPointRHS(rhs) || !work.admittedRuleContribution(rule) {
		return PointRHS{}, false
	}
	return work.overlayPointSurface(rhs, rule.value.state, rule.value.coverage)
}

// OverlayPointEnvironment folds a contained environment into a PointRHS
// without lifting or closing either PointState. The environment's right root
// is consulted only on its C surface, so its latent branches cannot act as
// Default or payload. The left physical mask remains expand(left C) union
// not(left support), preserving left latent branches until the explicit
// LiftRuleContribution boundary.
func (work *Work) OverlayPointEnvironment(rhs PointRHS, environment PointState) (PointRHS, bool) {
	if !work.admittedPointRHS(rhs) || !work.admittedPointState(environment) || !work.liveFor(rhs.point.state, environment.state) {
		return PointRHS{}, false
	}
	return work.overlayPointSurface(rhs, environment.state, environment.coverage)
}

// overlayPointSurface is the one directional PointRHS transaction. The right
// State+C pair is private to a nominal caller (RuleContribution or
// PointState), never an arbitrary raw State. It requires right support to be
// contained by left support: only then can the result retain left's physical
// out-of-support root branches without making them semantic after support
// growth.
func (work *Work) overlayPointSurface(rhs PointRHS, rightState State, rightCoverage contributionCoverage) (PointRHS, bool) {
	if !work.admittedPointRHS(rhs) || !work.validContributionSurface(rightState, rightCoverage) || !work.liveFor(rhs.point.state, rightState) || !rightState.support.Entails(rhs.point.state.support) {
		return PointRHS{}, false
	}
	// Coverage is the authoritative lifted presence plane. A contained right
	// surface with no authored rows is therefore the exact overlay identity,
	// even when its raw PointState retains latent payload outside support.
	// Keep that representation-private payload out of typed work entirely.
	if len(rightCoverage.slots) == 0 {
		return rhs, true
	}
	nextCoverage, ok := work.unionCoverage(rhs.point.coverage, rightCoverage)
	if !ok {
		return PointRHS{}, false
	}
	empty := emptyMask(work.composition.guards)
	if !empty.Valid() {
		return PointRHS{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return PointRHS{}, false
	}
	patches := make([]Patch, 0, len(work.slots))
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return PointRHS{}, false
		}
		physical := shape.Slot(position)
		rightSlot := rightCoverage.slot(physical)
		// Absent is the contribution join identity. Calling into a typed
		// Binding for this slot would only resolve immutable roots and open an
		// empty transaction before proving the same fact. Untouched slots retain
		// their exact root handles through commit.
		if len(rightSlot.targets) == 0 {
			continue
		}
		change, valid := slot.OverlayPointRHSUnder(rhs.point.state.roots[position], rightState.roots[position], rhs.point.state.support, rightState.support, coverageRows(rhs.point.coverage.slot(physical)), coverageRows(rightSlot), delta)
		if !valid {
			delta.Discard()
			dropPatches(patches)
			return PointRHS{}, false
		}
		if !work.acceptInto(&patches, rhs.point.state, change, delta) {
			delta.Discard()
			return PointRHS{}, false
		}
	}
	next, _, ok := work.commit(rhs.point.state, patches, rhs.point.state.support, empty, empty, delta)
	if !ok {
		return PointRHS{}, false
	}
	// A C-empty right side produces no typed change and preserves a preexisting
	// closure proof. Any authored overlay may retain left physical branches
	// outside support, so it deliberately clears the fast-path bit.
	point := PointState{state: next, coverage: nextCoverage, roleSeal: work.contributionSeal, authority: next.authority, closed: rhs.point.closed && len(rightCoverage.slots) == 0}
	if !work.admittedPointState(point) {
		return PointRHS{}, false
	}
	result := PointRHS{point: point, roleSeal: work.contributionSeal}
	return result, work.admittedPointRHS(result)
}

// MergeChangedCoordinatePointRHS is the phase-local structural seminaive
// append for one coordinate-identical environment edge. The caller's
// publication window certifies previous <= current, including authored
// presence monotonicity; this hot path deliberately does not repeat the full
// lifted order zipper. It still fences the cheap necessary support monotonicity
// and refuses support growth at the target: a growing RHS must take the
// closed JoinPointRHS path so a latent left root cannot become visible.
//
// Unlike legacy Contribution transport, this method keeps PointRHS nominal
// throughout. ChangedPointSlotWork starts from the left physical root and
// changes only source-owned ascending regions, so fibers outside left support
// remain byte/root-persistent exactly as with OverlayRuleContribution. The
// returned accumulator is deliberately marked non-closed; a later
// LiftRuleContribution is the one durable RuleContribution boundary.
func (work *Work) MergeChangedCoordinatePointRHS(left PointRHS, previous, current PointState, semantic []ChangeSet, authored CoverageChangeSet, pre support.Mask, omega ReindexPlan, post support.Mask) (PointRHS, bool) {
	if !work.live() || work.reindexing || !work.admittedPointRHS(left) || !work.admittedPointState(previous) || !work.admittedPointState(current) ||
		!work.composition.OwnsCoverageChangeSet(authored) || !omega.validFor(work.composition) || !omega.coordinateIdentity() ||
		!left.point.state.scope.same(omega.target()) || !previous.state.scope.same(omega.source()) || !current.state.scope.same(omega.source()) ||
		!validBoundaryMask(pre, current.state.scope) || !validBoundaryMask(post, left.point.state.scope) || !previous.state.support.Entails(current.state.support) {
		return PointRHS{}, false
	}
	for index := range semantic {
		if !work.composition.OwnsChangeSet(semantic[index]) {
			return PointRHS{}, false
		}
	}
	for _, slot := range work.slots {
		if _, ok := slot.(ChangedPointSlotWork); !ok {
			// There is no safe role-preserving fallback: the complete legacy
			// path closes a Contribution and would erase the intended point
			// root sharing. The executor must rebuild through its normal fold.
			return PointRHS{}, false
		}
	}
	work.reindexing = true
	defer func() { work.reindexing = false }()

	sourceSupport, ok := work.intersectSupport(current.state.support, pre)
	if !ok {
		return PointRHS{}, false
	}
	reindexedSupport, ok := work.reindexSupport(sourceSupport, omega.relation)
	if !ok {
		return PointRHS{}, false
	}
	targetSupport, ok := work.intersectSupport(reindexedSupport, post)
	if !ok || !targetSupport.Entails(left.point.state.support) {
		return PointRHS{}, false
	}
	targetCoverage, ok := work.transportContributionCoverage(current.coverage, pre, omega, post, targetSupport)
	if !ok {
		return PointRHS{}, false
	}
	nextCoverage, ok := work.unionCoverage(left.point.coverage, targetCoverage)
	if !ok {
		return PointRHS{}, false
	}
	empty := emptyMask(work.composition.guards)
	if !empty.Valid() {
		return PointRHS{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return PointRHS{}, false
	}
	patches := make([]Patch, 0, len(work.slots))
	for position, slot := range work.slots {
		changed, ok := slot.(ChangedPointSlotWork)
		if !ok {
			delta.Discard()
			dropPatches(patches)
			return PointRHS{}, false
		}
		physical := shape.Slot(position)
		change, valid := changed.MergeChangedCoordinatePointUnder(left.point.state.roots[position], current.state.roots[position], left.point.state.support, sourceSupport, targetSupport, pre, post, coverageRows(left.point.coverage.slot(physical)), coverageRows(targetCoverage.slot(physical)), semantic, authored, delta)
		if !valid {
			delta.Discard()
			dropPatches(patches)
			return PointRHS{}, false
		}
		if !work.acceptInto(&patches, left.point.state, change, delta) {
			delta.Discard()
			return PointRHS{}, false
		}
	}
	next, _, committed := work.commit(left.point.state, patches, left.point.state.support, empty, empty, delta)
	if !committed {
		return PointRHS{}, false
	}
	point := PointState{state: next, coverage: nextCoverage, roleSeal: work.contributionSeal, authority: next.authority}
	if !work.admittedPointState(point) {
		return PointRHS{}, false
	}
	result := PointRHS{point: point, roleSeal: work.contributionSeal}
	return result, work.admittedPointRHS(result)
}

// JoinPointRHS is the explicit total semantic join for two environment bases
// (or any support-growing RHS step). It first closes each semantic base to
// its own lifted authored surface, then uses closed RuleContribution join.
// Raw State join would totalize an absent C cell to Default and could pollute
// a newly authored cell. A support-growing confluence also cannot retain an
// old latent branch where the union makes it newly visible.
func (work *Work) JoinPointRHS(left, right PointRHS) (PointRHS, bool) {
	if !work.admittedPointRHS(left) || !work.admittedPointRHS(right) || !work.liveFor(left.point.state, right.point.state) {
		return PointRHS{}, false
	}
	leftRule, ok := work.LiftRuleContribution(left.point)
	if !ok {
		return PointRHS{}, false
	}
	rightRule, ok := work.LiftRuleContribution(right.point)
	if !ok {
		return PointRHS{}, false
	}
	merged, _, ok := work.MergeRuleContributions(leftRule, rightRule)
	if !ok {
		return PointRHS{}, false
	}
	return work.PointRHSFromRuleContribution(merged)
}

// JoinRuleContribution makes the support-growing rule case explicit.  It
// first changes only the nominal role, then routes through JoinPointRHS; it
// never feeds a raw PointState to RuleContribution algebra.
func (work *Work) JoinRuleContribution(rhs PointRHS, rule RuleContribution) (PointRHS, bool) {
	if !work.admittedPointRHS(rhs) || !work.admittedRuleContribution(rule) {
		return PointRHS{}, false
	}
	right, ok := work.PointRHSFromRuleContribution(rule)
	if !ok {
		return PointRHS{}, false
	}
	return work.JoinPointRHS(rhs, right)
}

// PublishPointRHS performs the role-only publication from an assembled RHS to
// a semantic PointState. It shares the immutable root header; the point role
// is intentionally not a closed RuleContribution unless LiftRuleContribution
// later performs that physical mask.
func (work *Work) PublishPointRHS(rhs PointRHS) (PointState, bool) {
	if !work.admittedPointRHS(rhs) {
		return PointState{}, false
	}
	return rhs.point, true
}

// ReplacePointWithRHS is the future sole point publication cut.  The result
// uses the exact RHS root and coverage but is nominally a PointState, leaving
// later point transport free to retain latent out-of-support root fibers.
func (work *Work) ReplacePointWithRHS(current PointState, rhs PointRHS) (PointState, ChangeSet, bool) {
	if !work.admittedPointState(current) || !work.admittedPointRHS(rhs) || !work.liveFor(current.state, rhs.point.state) {
		return PointState{}, ChangeSet{}, false
	}
	next, changes, ok := work.Replace(current.state, rhs.point.state)
	if !ok {
		return PointState{}, ChangeSet{}, false
	}
	point := PointState{state: next, coverage: rhs.point.coverage, roleSeal: work.contributionSeal, authority: next.authority, closed: rhs.point.closed}
	return point, changes, work.admittedPointState(point)
}

// MergeSelectedPointState is the nominal recurrence publication boundary.
// It shares the exact same State+C transaction as compatibility
// MergeSelectedContribution, but never casts a PointState/RHS through the
// legacy Contribution role. The result always takes exactRight's authored
// surface and is physically closed to it, so a latent current point root
// cannot survive Widen or Narrow and reappear after a later support change.
func (work *Work) MergeSelectedPointState(kind MergeKind, current PointState, selectedRight, exactRight PointRHS, selected MergeScope) (PointState, ChangeSet, bool) {
	if !work.admittedPointState(current) || !work.admittedPointRHS(selectedRight) || !work.admittedPointRHS(exactRight) {
		return PointState{}, ChangeSet{}, false
	}
	state, changes, ok := work.mergeSelectedContributionSurface(kind, current.state, current.coverage, selectedRight.point.state, selectedRight.point.coverage, exactRight.point.state, exactRight.point.coverage, selected)
	if !ok {
		return PointState{}, ChangeSet{}, false
	}
	point := PointState{
		state:     state,
		coverage:  exactRight.point.coverage,
		roleSeal:  work.contributionSeal,
		authority: state.authority,
		closed:    true,
	}
	if !work.admittedPointState(point) {
		return PointState{}, ChangeSet{}, false
	}
	return point, changes, true
}

// TransportPointState is total semantic point transport.  It deliberately
// does not call ReindexPointContributionUnder or CloseContributionUnder.  On
// a coordinate-identity relation the immutable typed roots are shared even
// when pre/post narrow support; any payload outside the resulting support is
// latent and cannot re-enter RuleContribution algebra except through the
// closing lift below.  Non-coordinate transport uses the existing total raw
// State transport, which masks source support before reindexing.
func (work *Work) TransportPointState(input PointState, pre support.Mask, omega ReindexPlan, post support.Mask) (PointState, bool) {
	if !work.live() || !work.admittedPointState(input) || !omega.validFor(work.composition) || !input.state.scope.same(omega.source()) || !validBoundaryMask(pre, input.state.scope) || !validBoundaryMask(post, omega.target()) {
		return PointState{}, false
	}
	if omega.identity() && pre.IsTrue() && post.IsTrue() {
		return input, true
	}
	if omega.coordinateIdentity() && pre.IsTrue() && post.IsTrue() {
		state := input.state
		state.scope = omega.target()
		point := input
		point.state = state
		point.authority = state.authority
		return point, work.admittedPointState(point)
	}
	if omega.coordinateIdentity() {
		source, ok := work.intersectSupport(input.state.support, pre)
		if !ok {
			return PointState{}, false
		}
		reindexed, ok := work.reindexSupport(source, omega.relation)
		if !ok {
			return PointState{}, false
		}
		target, ok := work.intersectSupport(reindexed, post)
		if !ok {
			return PointState{}, false
		}
		coverage, ok := work.transportContributionCoverage(input.coverage, pre, omega, post, target)
		if !ok {
			return PointState{}, false
		}
		state := input.state
		state.scope, state.support = omega.target(), target
		point := PointState{state: state, coverage: coverage, roleSeal: work.contributionSeal, authority: state.authority}
		return point, work.admittedPointState(point)
	}
	state, ok := work.Transport(input.state, pre, omega, post)
	if !ok {
		return PointState{}, false
	}
	coverage, ok := work.transportContributionCoverage(input.coverage, pre, omega, post, state.support)
	if !ok {
		return PointState{}, false
	}
	point := PointState{state: state, coverage: coverage, roleSeal: work.contributionSeal, authority: state.authority}
	return point, work.admittedPointState(point)
}

// LiftRuleContribution is the only PointState -> RuleContribution boundary.
// It turns the semantic support mask into a physical root mask before the
// value can take part in a coverage-aware RHS join.  Thus a latent point root
// is useful for total point routes but can never revive after unrelated
// support growth in RuleContribution algebra.
func (work *Work) LiftRuleContribution(point PointState) (RuleContribution, bool) {
	if !work.admittedPointState(point) {
		return RuleContribution{}, false
	}
	if point.closed {
		value, ok := work.admitConstructedContribution(point.state, point.coverage)
		if !ok {
			return RuleContribution{}, false
		}
		return work.AsRuleContribution(value)
	}
	empty := emptyMask(work.composition.guards)
	if !empty.Valid() {
		return RuleContribution{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return RuleContribution{}, false
	}
	patches := make([]Patch, 0, len(work.slots))
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return RuleContribution{}, false
		}
		physical := shape.Slot(position)
		change, ok := slot.CloseContributionUnder(point.state.roots[position], point.state.roots[position], point.state.support, coverageRows(point.coverage.slot(physical)), delta)
		if !ok || !work.acceptInto(&patches, point.state, change, delta) {
			delta.Discard()
			if ok {
				dropPatches(patches)
			}
			return RuleContribution{}, false
		}
	}
	next, _, ok := work.commit(point.state, patches, point.state.support, empty, empty, delta)
	if !ok {
		return RuleContribution{}, false
	}
	value, ok := work.admitConstructedContribution(next, point.coverage)
	if !ok {
		return RuleContribution{}, false
	}
	return work.AsRuleContribution(value)
}

// MergeRuleContributions exposes the closed C-only semilattice through the
// nominal role.  It intentionally accepts no PointState or PointRHS; callers
// must first use LiftRuleContribution.  PointRHS uses either directional
// OverlayRuleContribution or explicit total JoinRuleContribution instead.
func (work *Work) MergeRuleContributions(left, right RuleContribution) (RuleContribution, ChangeSet, bool) {
	if !work.admittedRuleContribution(left) || !work.admittedRuleContribution(right) {
		return RuleContribution{}, ChangeSet{}, false
	}
	next, changes, ok := work.MergeContribution(left.value, right.value)
	if !ok {
		return RuleContribution{}, ChangeSet{}, false
	}
	result, ok := work.AsRuleContribution(next)
	return result, changes, ok
}

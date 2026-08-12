package carrier

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// ContributionSource names one declared ordered input from which a group
// carries its output Factor. It is carrier-internal structural metadata: the
// caller can name a physical slot only while sealing a closed Composition.
type ContributionSource struct {
	Slot  shape.Slot
	Input int
}

// ContributionPlan is the cold ownership proof for one atomic Rule group.
// It records every Factor that group may write and the only whole-Factor
// carries that may enter its contribution. Dynamic authorship is not inferred
// from this static eligibility list; it arrives only from accepted patches or
// the named input contribution's exact carried coverage.
type ContributionPlan struct{ value *contributionPlan }

type contributionPlan struct {
	composition  *Composition
	inputs       int
	writes       []shape.Slot
	carries      []ContributionSource
	supportPrune bool
	environment  bool
}

// ContributionBase is a single-use, unpublishable write predecessor. Its
// State is intentionally accepted by typed Patch construction, but ordinary
// carrier publication and lattice operations reject it. FinishContribution is
// its sole route to a normal immutable Contribution.
type ContributionBase struct{ value *contributionBase }

type contributionBase struct {
	work           *Work
	plan           *contributionPlan
	inputs         []Contribution
	environment    Contribution
	hasEnvironment bool
	state          State
	rootsOwned     bool
	live           bool
}

// SealContribution freezes the structural projection of one Rule group before
// any evaluator Work exists. A plan may include a structural support prune
// alongside distinct Factor writes/carries; they still enter only through one
// atomic contribution. A zero-write prune is simply the degenerate member
// set, not a second transaction. Input positions are an ordered finite vector:
// zero inputs name an explicit immutable ingress rather than an implicit
// predecessor.
func (composition *Composition) SealContribution(inputCount int, writes []shape.Slot, carries []ContributionSource, supportPrune bool, environment ...bool) (ContributionPlan, bool) {
	if composition == nil || composition.shape == nil || inputCount < 0 {
		return ContributionPlan{}, false
	}
	if len(environment) > 1 {
		return ContributionPlan{}, false
	}
	hasEnvironment := len(environment) == 1 && environment[0]
	composition.scopeMu.Lock()
	defer composition.scopeMu.Unlock()
	if composition.workOpened {
		return ContributionPlan{}, false
	}
	// A structural activation member has no patch or support restriction, but
	// still belongs to its Group's one atomic transaction.  The empty plan is
	// therefore a valid degenerate contribution, not a second evaluator.
	if len(writes) == 0 && len(carries) == 0 {
		return ContributionPlan{value: &contributionPlan{composition: composition, inputs: inputCount, supportPrune: supportPrune, environment: hasEnvironment}}, true
	}
	orderedWrites := append([]shape.Slot(nil), writes...)
	sort.Slice(orderedWrites, func(left, right int) bool { return orderedWrites[left] < orderedWrites[right] })
	for index, slot := range orderedWrites {
		if !composition.shape.ValidSlot(slot) || index > 0 && orderedWrites[index-1] >= slot {
			return ContributionPlan{}, false
		}
	}
	orderedCarries := append([]ContributionSource(nil), carries...)
	sort.Slice(orderedCarries, func(left, right int) bool { return orderedCarries[left].Slot < orderedCarries[right].Slot })
	for index, carry := range orderedCarries {
		if carry.Input < 0 || carry.Input >= inputCount || !composition.shape.ValidSlot(carry.Slot) || index > 0 && orderedCarries[index-1].Slot >= carry.Slot {
			return ContributionPlan{}, false
		}
	}
	return ContributionPlan{value: &contributionPlan{composition: composition, inputs: inputCount, writes: orderedWrites, carries: orderedCarries, supportPrune: supportPrune, environment: hasEnvironment}}, true
}

// BeginContribution constructs the one common patch predecessor for a Rule
// group. Ordinary roots start at the Composition initial vector, then only
// sealed carry slots are replaced from their named input States. A support
// prune restricts support only: it never copies input zero's roots or
// authorship as an implicit carry. Every group names its output Scope
// explicitly.
// premise is the sealed structural activation condition, not a caller-proved
// predecessor support: this boundary computes input-intersection ∩ premise
// itself. A zero-input group therefore publishes exactly premise. The later
// supportPrune path alone may further restrict that computed predecessor.
func (work *Work) BeginContribution(plan ContributionPlan, target Scope, inputs []Contribution, premise support.Mask, environment ...Contribution) (ContributionBase, bool) {
	if !work.live() || work.composition == nil || plan.value == nil || plan.value.composition != work.composition || !target.validFor(work.composition) || len(inputs) != plan.value.inputs || !premise.Valid() || premise.Manager() != work.composition.guards {
		return ContributionBase{}, false
	}
	if len(environment) > 1 || plan.value.environment != (len(environment) == 1) {
		return ContributionBase{}, false
	}
	root, ok := premise.Guard()
	if !ok || !target.guard.Contains(root) {
		return ContributionBase{}, false
	}
	within := premise
	for index, input := range inputs {
		if !work.admittedContribution(input) || !work.OwnsState(input.state) || input.state.previewMarked() || input.state.contributionMarked() {
			return ContributionBase{}, false
		}
		if index == 0 {
			if !input.state.scope.same(target) {
				return ContributionBase{}, false
			}
			within = input.state.Support()
			continue
		}
		if !input.state.scope.same(target) {
			return ContributionBase{}, false
		}
		var ok bool
		within, ok = work.intersectSupport(within, input.state.Support())
		if !ok {
			return ContributionBase{}, false
		}
	}
	if len(inputs) != 0 {
		within, ok = work.intersectSupport(within, premise)
		if !ok {
			return ContributionBase{}, false
		}
	}
	var environmentValue Contribution
	if len(environment) == 1 {
		environmentValue = environment[0]
		if !work.admittedContribution(environmentValue) || !work.OwnsState(environmentValue.state) || environmentValue.state.previewMarked() || environmentValue.state.contributionMarked() || !environmentValue.state.scope.same(target) {
			return ContributionBase{}, false
		}
		var intersectOK bool
		within, intersectOK = work.intersectSupport(within, environmentValue.state.Support())
		if !intersectOK {
			return ContributionBase{}, false
		}
	}
	roots := work.composition.initial
	rootsOwned := false
	if plan.value.environment {
		roots = append([]RootHandle(nil), environmentValue.state.roots...)
		rootsOwned = true
		for _, carry := range plan.value.carries {
			roots[int(carry.Slot)] = work.composition.initial[int(carry.Slot)]
		}
	}
	if len(plan.value.carries) != 0 {
		// Carrier has one published representation: a complete immutable root
		// vector. Keeping a sparse carry overlay here would add a second
		// predecessor representation and weaken the one atomic Finish cut.
		// A future Point cache may defer this materialization, but this carrier
		// base deliberately remains a complete patch predecessor.
		if !rootsOwned {
			roots = append([]RootHandle(nil), roots...)
			rootsOwned = true
		}
		for _, carry := range plan.value.carries {
			roots[int(carry.Slot)] = inputs[carry.Input].state.roots[int(carry.Slot)]
		}
	}
	owner := &contributionBase{work: work, plan: plan.value, inputs: append([]Contribution(nil), inputs...), environment: environmentValue, hasEnvironment: plan.value.environment, rootsOwned: rootsOwned, live: true}
	owner.state = State{authority: &stateAuthority{composition: work.composition, epoch: work.epoch, contribution: owner}, scope: target, support: within, roots: roots}
	if !owner.state.live() {
		owner.live = false
		return ContributionBase{}, false
	}
	return ContributionBase{value: owner}, true
}

// State returns the exact shared predecessor used by every Rule patch in this
// group. It is valid only until FinishContribution or AbortContribution.
func (base ContributionBase) State() State {
	if base.value == nil || !base.value.live || !base.value.state.live() {
		return State{}
	}
	return base.value.state
}

// OwnsContribution proves this Work owns the exact still-live group base and
// that every caller-supplied source State is the one used to construct it.
// It prevents a sibling Rule from reusing a group predecessor after any input
// Point has changed or from substituting an equal-looking State.
func (work *Work) OwnsContribution(base ContributionBase, inputs []Contribution) bool {
	if work == nil || base.value == nil || !base.value.live || base.value.work != work || base.value.plan == nil || base.value.plan.composition != work.composition || len(inputs) != len(base.value.inputs) || !base.value.state.live() || base.value.state.contributionOwner() != base.value {
		return false
	}
	for index := range inputs {
		if !work.admittedContribution(inputs[index]) || !sameState(inputs[index].state, base.value.inputs[index].state) || !sameContributionCoverage(inputs[index].coverage, base.value.inputs[index].coverage) {
			return false
		}
	}
	return true
}

// OwnsContributionStates validates the semantic input vector passed to Rule
// callbacks against the exact paired inputs used to open base. Coverage never
// crosses the typed callback surface, but it remains retained by base for the
// sole Finish carry publication.
func (work *Work) OwnsContributionStates(base ContributionBase, inputs []State) bool {
	if work == nil || base.value == nil || !base.value.live || base.value.work != work || len(inputs) != len(base.value.inputs) {
		return false
	}
	for index := range inputs {
		if !sameState(inputs[index], base.value.inputs[index].state) {
			return false
		}
	}
	return true
}

// FinishContribution admits the group’s already accepted patches at the sole
// carrier cut. Every patch must have been built from this exact shared base
// and must target one of the plan’s sealed write slots. The returned State has
// normal Composition authority; the temporary base is invalidated regardless
// of success, so it cannot become a second publication route.
func (work *Work) FinishContribution(base ContributionBase, patches []Patch) (Contribution, bool) {
	return work.finishContribution(base, patches, support.Mask{}, false)
}

// FinishContributionWithSupport closes the same one-shot contribution with a
// support result proven by structural Rule members. The support remains tied
// to the common full predecessor used by every Factor Patch, so a mixed group
// cannot publish a prune before its sibling typed patches have all passed
// admission. Only a plan declared with supportPrune may retain a strict
// subset; ordinary groups retain their exact input-intersection ∩ premise.
func (work *Work) FinishContributionWithSupport(base ContributionBase, patches []Patch, retained support.Mask) (Contribution, bool) {
	return work.finishContribution(base, patches, retained, true)
}

func (work *Work) finishContribution(base ContributionBase, patches []Patch, retained support.Mask, withSupport bool) (Contribution, bool) {
	// Establish exact base ownership before touching any caller-supplied
	// Patch. A foreign Work/base pair retains both resources for its owner.
	if work == nil || base.value == nil || !work.OwnsContribution(base, base.value.inputs) {
		return Contribution{}, false
	}
	owner := base.value
	defer owner.invalidate()
	// Once the base belongs here, a failed batch still consumes that one-shot
	// base. Foreign Patch handles remain caller-owned and completely untouched.
	if !contributionPatchesOwned(work, owner, patches) {
		return Contribution{}, false
	}
	if !work.live() || work.composition == nil {
		dropPatches(patches)
		return Contribution{}, false
	}
	if withSupport {
		if owner.plan == nil || !owner.plan.supportPrune {
			dropPatches(patches)
			return Contribution{}, false
		}
	} else {
		retained = owner.state.support
	}
	if !retained.Valid() || retained.Manager() != work.composition.guards || !retained.Entails(owner.state.support) || !owner.plan.supportPrune && !retained.Equal(owner.state.support) {
		dropPatches(patches)
		return Contribution{}, false
	}
	if !contributionPatchesAllowed(owner.plan, patches) {
		dropPatches(patches)
		return Contribution{}, false
	}
	composition := work.composition
	coverage := contributionCoverage{composition: composition, slots: make([]slotCoverage, composition.Count())}
	rows := make([][]TargetRegion, composition.Count())
	if owner.plan.environment {
		for position := range rows {
			physical := shape.Slot(position)
			if !containsContributionCarry(owner.plan.carries, physical) {
				rows[position] = append(rows[position], owner.environment.coverage.slot(physical).targets...)
			}
		}
	}
	for _, carry := range owner.plan.carries {
		carried := owner.inputs[carry.Input].coverage.slot(carry.Slot)
		rows[int(carry.Slot)] = append(rows[int(carry.Slot)], carried.targets...)
	}
	for _, patch := range patches {
		rows[int(patch.slot)] = append(rows[int(patch.slot)], patch.authored...)
	}
	nonempty := false
	for position := range rows {
		canonical, canonicalOK := work.canonicalCoverage(rows[position], retained)
		if !canonicalOK {
			dropPatches(patches)
			return Contribution{}, false
		}
		coverage.slots[position] = canonical
		nonempty = nonempty || len(canonical.targets) != 0
	}
	if !nonempty {
		coverage.slots = nil
	}
	composition.layout.publish.Lock()
	defer composition.layout.publish.Unlock()
	if !work.live() || work.publishing || !owner.live || !owner.state.live() {
		dropPatches(patches)
		return Contribution{}, false
	}
	work.publishing = true
	defer func() { work.publishing = false }()
	split, splitOK := work.threeSupport(owner.state.support, retained)
	empty := emptyMask(composition.guards)
	if !splitOK || !empty.Valid() {
		dropPatches(patches)
		return Contribution{}, false
	}
	prepared, ok := work.prepareCommit(owner.state, patches, retained, split.RightOnly(), split.LeftOnly(), nil)
	if !ok {
		dropPatches(patches)
		return Contribution{}, false
	}
	next := owner.state.roots
	if prepared.rootsChanged && !owner.rootsOwned {
		next = append([]RootHandle(nil), owner.state.roots...)
	}
	for _, patch := range patches {
		if !work.live() {
			dropPatches(patches)
			return Contribution{}, false
		}
		if publisher := patch.change.record.publisher; publisher != nil && !publisher.Reserve() {
			dropPatches(patches)
			return Contribution{}, false
		}
		if !work.live() {
			dropPatches(patches)
			return Contribution{}, false
		}
	}
	if !work.live() {
		dropPatches(patches)
		return Contribution{}, false
	}
	// Publish is the final non-interruptible cut: polling after the first
	// publisher would allow a visible partial vector.
	for _, patch := range patches {
		record := patch.change.record
		if publisher := record.publisher; publisher != nil {
			root := publisher.Publish()
			if !composition.operations[int(patch.slot)].ValidRoot(root) {
				panic("contribution root publication violated carrier invariant")
			}
			next[int(patch.slot)] = root
		} else if prepared.rootsChanged {
			next[int(patch.slot)] = record.after
		}
	}
	dropPatches(patches)
	state := State{authority: work.authority, scope: owner.state.scope, support: retained, roots: next}
	return work.admitConstructedContribution(state, coverage)
}

// AbortContribution consumes every ready but unpublished Patch and invalidates
// the base. Epoch cancellation and callback failure must use this operation;
// it is the only cleanup path for a partially staged group.
func (work *Work) AbortContribution(base ContributionBase, patches []Patch) bool {
	if work == nil || base.value == nil || !work.OwnsContribution(base, base.value.inputs) {
		return false
	}
	owner := base.value
	defer owner.invalidate()
	if !contributionPatchesOwned(work, owner, patches) {
		return false
	}
	dropPatches(patches)
	return true
}

func (owner *contributionBase) invalidate() {
	if owner == nil {
		return
	}
	owner.live = false
	clear(owner.inputs)
	owner.inputs = nil
	owner.work = nil
	owner.plan = nil
	owner.state = State{}
	owner.environment = Contribution{}
	owner.hasEnvironment = false
	owner.rootsOwned = false
}

func containsContributionCarry(carries []ContributionSource, slot shape.Slot) bool {
	for _, carry := range carries {
		if carry.Slot == slot {
			return true
		}
	}
	return false
}

// contributionPatchesOwned proves the whole proposed batch belongs to one
// exact still-live group predecessor before either Finish or Abort may consume
// a single prepared publisher. In particular, a different Work over the same
// Composition is not interchangeable: Patch ownership is evaluator-local.
func contributionPatchesOwned(work *Work, owner *contributionBase, patches []Patch) bool {
	if work == nil || owner == nil || !owner.live || owner.work != work || !owner.state.live() {
		return false
	}
	for _, patch := range patches {
		if patch.work != work || !sameState(patch.state, owner.state) || patch.change.issuer == nil || patch.change.record == nil || patch.change.record.consumed {
			return false
		}
	}
	return true
}

func contributionPatchesAllowed(plan *contributionPlan, patches []Patch) bool {
	if plan == nil || len(patches) > len(plan.writes) {
		return false
	}
	write := 0
	for index, patch := range patches {
		if index > 0 && patches[index-1].slot >= patch.slot {
			return false
		}
		for write < len(plan.writes) && plan.writes[write] < patch.slot {
			write++
		}
		if write == len(plan.writes) || plan.writes[write] != patch.slot {
			return false
		}
	}
	return true
}

func (state State) contributionMarked() bool {
	return state.authority != nil && state.authority.contribution != nil
}

func (state State) contributionOwner() *contributionBase {
	if state.authority == nil {
		return nil
	}
	return state.authority.contribution
}

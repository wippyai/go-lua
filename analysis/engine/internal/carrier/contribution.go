package carrier

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
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
	composition *Composition
	inputs      int
	writes      []shape.Slot
	carries     []ContributionSource
	// carrySlots is the seal-time issuance of the slots carries name. Finish
	// asks membership of it instead of rescanning the carry vector once per
	// physical slot.
	carrySlots  change.Slots
	environment bool
}

// ContributionBase is a single-use, unpublishable write predecessor. Its
// State is intentionally accepted by typed Patch construction, but ordinary
// carrier publication and lattice operations reject it. FinishContribution is
// its sole route to a normal immutable Contribution.
type ContributionBase struct{ value *contributionBase }

// RuleContributionBase is the nominal rule-evaluation predecessor. It has the
// same one-shot transaction ownership as ContributionBase, but its inputs are
// PointStates rather than already-closed RuleContributions. That distinction
// lets a coordinate-filtered point retain its raw root through a declared
// carry without opening a raw-State construction route. FinishRuleContribution
// is the only route from this base to a closed RuleContribution.
type RuleContributionBase struct{ value *contributionBase }

// contributionInput is private paired semantic state plus compact authored
// coverage. It is the one internal source representation used by both legacy
// closed Contribution inputs and nominal PointState inputs. The former is
// already physically closed; the latter may retain latent roots, which the
// shared Finish closure removes at the RuleContribution boundary.
type contributionInput struct {
	state    State
	coverage contributionCoverage
	// closed is the source PointState's carrier-issued physical-closure proof.
	// A legacy Contribution carries the same proof by construction.  It stays
	// private because it is usable only together with the exact source State,
	// root, support, and coverage checks at Finish.
	closed bool
}

type contributionBase struct {
	work           *Work
	plan           *contributionPlan
	inputs         []contributionInput
	environment    contributionInput
	hasEnvironment bool
	state          State
	rootsOwned     bool
	live           bool
}

// SealContribution freezes the structural projection of one Rule group before
// any evaluator Work exists. Input positions are an ordered finite vector:
// zero inputs name an explicit immutable ingress rather than an implicit
// predecessor.
func (composition *Composition) SealContribution(inputCount int, writes []shape.Slot, carries []ContributionSource, environment ...bool) (ContributionPlan, bool) {
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
		return ContributionPlan{value: &contributionPlan{composition: composition, inputs: inputCount, environment: hasEnvironment}}, true
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
	plan := &contributionPlan{composition: composition, inputs: inputCount, writes: orderedWrites, carries: orderedCarries, environment: hasEnvironment}
	for _, carry := range orderedCarries {
		plan.carrySlots.Set(int(carry.Slot))
	}
	return ContributionPlan{value: plan}, true
}

// BeginContribution is the compatibility entry for an already closed
// Contribution input vector. It delegates to the same private transaction as
// BeginRuleContribution; there is no second Finish or carry implementation.
func (work *Work) BeginContribution(plan ContributionPlan, target Scope, inputs []Contribution, premise support.Mask, environment ...Contribution) (ContributionBase, bool) {
	prepared, ok := work.contributionInputsFromContributions(inputs)
	if !ok {
		return ContributionBase{}, false
	}
	var preparedEnvironment []contributionInput
	if len(environment) == 1 {
		environmentInput, valid := work.contributionInputFromContribution(environment[0])
		if !valid {
			return ContributionBase{}, false
		}
		preparedEnvironment = []contributionInput{environmentInput}
	} else if len(environment) != 0 {
		return ContributionBase{}, false
	}
	owner, ok := work.beginContribution(plan, target, prepared, premise, preparedEnvironment)
	if !ok {
		return ContributionBase{}, false
	}
	return ContributionBase{value: owner}, true
}

// BeginRuleContribution opens the one common patch predecessor for a rule
// evaluation over published PointStates. Point inputs retain their raw roots
// and compact C through declared carries/environment slots; callbacks still
// receive only the resulting []State through RuleContributionBase.State and
// OwnsRuleContributionStates. No raw State may enter this API.
func (work *Work) BeginRuleContribution(plan ContributionPlan, target Scope, inputs []PointState, premise support.Mask, environment ...PointState) (RuleContributionBase, bool) {
	prepared, ok := work.contributionInputsFromPoints(inputs)
	if !ok {
		return RuleContributionBase{}, false
	}
	var preparedEnvironment []contributionInput
	if len(environment) == 1 {
		environmentInput, valid := work.contributionInputFromPoint(environment[0])
		if !valid {
			return RuleContributionBase{}, false
		}
		preparedEnvironment = []contributionInput{environmentInput}
	} else if len(environment) != 0 {
		return RuleContributionBase{}, false
	}
	owner, ok := work.beginContribution(plan, target, prepared, premise, preparedEnvironment)
	if !ok {
		return RuleContributionBase{}, false
	}
	return RuleContributionBase{value: owner}, true
}

func (work *Work) contributionInputFromContribution(input Contribution) (contributionInput, bool) {
	if work == nil || !work.admittedContribution(input) || !work.OwnsState(input.state) || input.state.previewMarked() || input.state.contributionMarked() {
		return contributionInput{}, false
	}
	return contributionInput{state: input.state, coverage: input.coverage, closed: true}, true
}

func (work *Work) contributionInputFromPoint(input PointState) (contributionInput, bool) {
	if work == nil || !work.admittedPointState(input) || input.state.previewMarked() || input.state.contributionMarked() {
		return contributionInput{}, false
	}
	return contributionInput{state: input.state, coverage: input.coverage, closed: input.closed}, true
}

func (work *Work) contributionInputsFromContributions(inputs []Contribution) ([]contributionInput, bool) {
	if work == nil {
		return nil, false
	}
	if len(inputs) == 0 {
		return nil, true
	}
	prepared := make([]contributionInput, len(inputs))
	for index := range inputs {
		input, ok := work.contributionInputFromContribution(inputs[index])
		if !ok {
			return nil, false
		}
		prepared[index] = input
	}
	return prepared, true
}

func (work *Work) contributionInputsFromPoints(inputs []PointState) ([]contributionInput, bool) {
	if work == nil {
		return nil, false
	}
	if len(inputs) == 0 {
		return nil, true
	}
	prepared := make([]contributionInput, len(inputs))
	for index := range inputs {
		input, ok := work.contributionInputFromPoint(inputs[index])
		if !ok {
			return nil, false
		}
		prepared[index] = input
	}
	return prepared, true
}

// beginContribution constructs the one common patch predecessor for either
// nominal input role. Ordinary roots start at Composition initial, then only
// sealed carry slots are replaced from named input roots. A support prune
// restricts support only: it never creates an implicit carry. The paired C
// surfaces stay private on owner until the one Finish closure publishes them.
func (work *Work) beginContribution(plan ContributionPlan, target Scope, inputs []contributionInput, premise support.Mask, environment []contributionInput) (*contributionBase, bool) {
	if !work.live() || work.composition == nil || plan.value == nil || plan.value.composition != work.composition || !target.validFor(work.composition) || len(inputs) != plan.value.inputs || !premise.Valid() || premise.Manager() != work.composition.guards || len(environment) > 1 || plan.value.environment != (len(environment) == 1) {
		return nil, false
	}
	root, ok := premise.Guard()
	if !ok || !target.guard.Contains(root) {
		return nil, false
	}
	within := premise
	for index, input := range inputs {
		if !work.validContributionInput(input) || !input.state.scope.same(target) {
			return nil, false
		}
		if index == 0 {
			within = input.state.Support()
			continue
		}
		within, ok = work.intersectSupport(within, input.state.Support())
		if !ok {
			return nil, false
		}
	}
	if len(inputs) != 0 {
		within, ok = work.intersectSupport(within, premise)
		if !ok {
			return nil, false
		}
	}
	var environmentValue contributionInput
	if len(environment) == 1 {
		environmentValue = environment[0]
		if !work.validContributionInput(environmentValue) || !environmentValue.state.scope.same(target) {
			return nil, false
		}
		within, ok = work.intersectSupport(within, environmentValue.state.Support())
		if !ok {
			return nil, false
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
	owner := &contributionBase{work: work, plan: plan.value, inputs: append([]contributionInput(nil), inputs...), environment: environmentValue, hasEnvironment: plan.value.environment, rootsOwned: rootsOwned, live: true}
	owner.state = State{authority: &stateAuthority{composition: work.composition, epoch: work.epoch, contribution: owner}, scope: target, support: within, roots: roots}
	if !owner.state.live() {
		owner.live = false
		return nil, false
	}
	return owner, true
}

func (work *Work) validContributionInput(input contributionInput) bool {
	return work != nil && work.OwnsState(input.state) && !input.state.previewMarked() && !input.state.contributionMarked() && input.coverage.composition == work.composition && (len(input.coverage.slots) == 0 || len(input.coverage.slots) == work.composition.Count())
}

// State returns the exact shared predecessor used by every compatibility Rule
// patch in this group. It is valid only until FinishContribution or
// AbortContribution.
func (base ContributionBase) State() State {
	if base.value == nil || !base.value.live || !base.value.state.live() {
		return State{}
	}
	return base.value.state
}

// State returns the exact shared predecessor used by every nominal
// RuleContribution patch. The callback surface deliberately remains State;
// PointState coverage stays carrier-private until FinishRuleContribution.
func (base RuleContributionBase) State() State {
	if base.value == nil || !base.value.live || !base.value.state.live() {
		return State{}
	}
	return base.value.state
}

func (work *Work) ownsContributionBase(owner *contributionBase) bool {
	return work != nil && owner != nil && owner.live && owner.work == work && owner.plan != nil && owner.plan.composition == work.composition && owner.state.live() && owner.state.contributionOwner() == owner
}

func sameContributionInput(left, right contributionInput) bool {
	return left.closed == right.closed && sameState(left.state, right.state) && sameContributionCoverage(left.coverage, right.coverage)
}

// OwnsRuleContributionBase proves this Work owns a nominal PointState-input
// base and the exact paired PointState headers which opened it. It prevents a
// sibling Rule from substituting an equal-looking raw State or dropping C.
//
// It is deliberately named after the ephemeral base rather than the published
// RuleContribution role. The latter has its own ownership predicate below;
// conflating the two would let an API which needs a sealed published operand
// accidentally accept a still-open authoring transaction.
func (work *Work) OwnsRuleContributionBase(base RuleContributionBase, inputs []PointState) bool {
	if !work.ownsContributionBase(base.value) || len(inputs) != len(base.value.inputs) {
		return false
	}
	for index := range inputs {
		input, ok := work.contributionInputFromPoint(inputs[index])
		if !ok || !sameContributionInput(input, base.value.inputs[index]) {
			return false
		}
	}
	return true
}

// OwnsRuleContributionStates validates the semantic input vector passed to Rule
// callbacks against the exact paired inputs used to open base. Coverage never
// crosses the typed callback surface, but it remains retained by base for the
// sole Finish carry publication.
func (work *Work) OwnsRuleContributionStates(base RuleContributionBase, inputs []State) bool {
	owner := base.value
	if !work.ownsContributionBase(owner) || len(inputs) != len(owner.inputs) {
		return false
	}
	for index := range inputs {
		if !sameState(inputs[index], owner.inputs[index].state) {
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
	return work.finishContribution(base.value, patches)
}

// FinishRuleContribution closes a PointState-input rule base at the same one
// atomic publication cut as legacy FinishContribution, then applies only the
// nominal role seal. In particular, carried latent PointState branches cannot
// escape into the returned RuleContribution.
func (work *Work) FinishRuleContribution(base RuleContributionBase, patches []Patch) (RuleContribution, bool) {
	value, ok := work.finishContribution(base.value, patches)
	if !ok {
		return RuleContribution{}, false
	}
	return work.AsRuleContribution(value)
}

func (work *Work) finishContribution(owner *contributionBase, patches []Patch) (Contribution, bool) {
	// Establish exact base ownership before touching any caller-supplied
	// Patch. A foreign Work/base pair retains both resources for its owner.
	if !work.ownsContributionBase(owner) {
		return Contribution{}, false
	}
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
	retained := owner.state.support
	if !retained.Valid() || retained.Manager() != work.composition.guards {
		dropPatches(patches)
		return Contribution{}, false
	}
	if !contributionPatchesAllowed(owner.plan, patches) {
		dropPatches(patches)
		return Contribution{}, false
	}
	composition := work.composition
	coverage := contributionCoverage{composition: composition, slots: make([]slotCoverage, composition.Count())}
	// The authored surface of a group is the join of three issued slot sets:
	// the environment's occupied slots minus the carried ones, the carried
	// slots, and the patched slots. A sparse group therefore assembles its own
	// width instead of walking the whole Factor plane twice.
	authored := work.borrowSlotSet()
	defer work.releaseSlotSet()
	if authored == nil {
		dropPatches(patches)
		return Contribution{}, false
	}
	if owner.plan.environment {
		for position, more := owner.environment.coverage.occupied.Next(0); more; position, more = owner.environment.coverage.occupied.Next(position + 1) {
			if !owner.plan.carrySlots.Test(position) {
				authored.set.Set(position)
			}
		}
	}
	for _, carry := range owner.plan.carries {
		if len(owner.inputs[carry.Input].coverage.slot(carry.Slot).targets) != 0 {
			authored.set.Set(int(carry.Slot))
		}
	}
	for _, patch := range patches {
		if len(patch.authored) != 0 {
			authored.set.Set(int(patch.slot))
		}
	}
	for position, more := authored.set.Next(0); more; position, more = authored.set.Next(position + 1) {
		physical := shape.Slot(position)
		rows := work.assembleAuthoredRows(owner, patches, physical)
		canonical, canonicalOK := work.canonicalCoverage(rows, retained)
		if !canonicalOK {
			dropPatches(patches)
			return Contribution{}, false
		}
		if len(canonical.targets) == 0 {
			continue
		}
		coverage.slots[position] = canonical
		coverage.occupied.Set(position)
	}
	if coverage.occupied.Empty() {
		coverage.slots = nil
	}
	split, splitOK := work.threeSupport(owner.state.support, retained)
	if !splitOK {
		dropPatches(patches)
		return Contribution{}, false
	}
	// A plain rule with no carried/environment roots starts from the immutable
	// Composition initial vector. Its accepted Patches are already closed under
	// their exact authored Target rows and ordinary Finish retains the complete
	// predecessor support. Therefore the final C assembled above is a direct
	// construction proof: re-closing and comparing every typed root would only
	// rebuild the same sparse planes. Carry and environment paths deliberately
	// stay on the general close below.
	if !owner.plan.environment && len(owner.plan.carries) == 0 {
		state, _, committed := work.commit(owner.state, patches, retained, split.RightOnly(), split.LeftOnly(), nil)
		if !committed {
			return Contribution{}, false
		}
		state.authority = work.authority
		return work.admitConstructedContribution(state, coverage)
	}
	// Every slot, including an untouched carry/environment slot, is closed
	// against the final C and final support before any publisher reserves. A
	// staged patch is first exposed only as a transaction-local preview; its
	// original publisher is dropped only after the closed replacement patch has
	// crossed ordinary acceptance. This preserves the one atomic root cut and
	// prevents a narrowed Finish from retaining a hidden carried fiber.
	delta := work.newSupportWork()
	if delta == nil {
		dropPatches(patches)
		return Contribution{}, false
	}
	closed, held := work.beginPatches(composition.Count())
	if !held {
		delta.Discard()
		dropPatches(patches)
		return Contribution{}, false
	}
	defer work.releasePatches(&closed)
	patchIndex := 0
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(closed)
			dropPatches(patches)
			return Contribution{}, false
		}
		physical := shape.Slot(position)
		candidate := owner.state.roots[position]
		var authored []TargetRegion
		var original *Patch
		if patchIndex < len(patches) && patches[patchIndex].slot == physical {
			original = &patches[patchIndex]
			record := original.change.record
			if record == nil {
				delta.Discard()
				dropPatches(closed)
				dropPatches(patches)
				return Contribution{}, false
			}
			if publisher := record.publisher; publisher != nil {
				preview, previewOK := publisher.(PreviewRootPublisher)
				if !previewOK {
					delta.Discard()
					dropPatches(closed)
					dropPatches(patches)
					return Contribution{}, false
				}
				var candidateOK bool
				candidate, candidateOK = preview.PreviewRoot()
				if !candidateOK || !preview.OwnsPreviewRoot(candidate) {
					delta.Discard()
					dropPatches(closed)
					dropPatches(patches)
					return Contribution{}, false
				}
			} else {
				candidate = record.after
			}
			authored = original.authored
		}
		// A closed PointState/Contribution source can retain its exact root
		// without reopening the typed close when this slot is untouched and the
		// final surface is unchanged.  The helper revalidates both opaque roots
		// through State.HandleAt before allowing the sparse commit to omit this
		// slot; any patch, alias, or coverage difference stays on
		// the ordinary typed close below.
		if original == nil && work.canBorrowClosedContributionSlot(owner, physical, retained, coverage.slot(physical)) {
			continue
		}
		change, valid := slot.CloseContributionUnder(owner.state.roots[position], candidate, split, coverageRows(coverage.slot(physical)), delta)
		if !valid {
			delta.Discard()
			dropPatches(closed)
			dropPatches(patches)
			return Contribution{}, false
		}
		if !work.acceptInto(&closed, owner.state, change, delta) {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, false
		}
		if original != nil {
			closed[len(closed)-1].authored = authored
			if !work.Discard(*original) {
				delta.Discard()
				dropPatches(closed)
				dropPatches(patches)
				return Contribution{}, false
			}
			patchIndex++
		}
	}
	if patchIndex != len(patches) {
		delta.Discard()
		dropPatches(closed)
		dropPatches(patches)
		return Contribution{}, false
	}
	state, _, committed := work.commit(owner.state, closed, retained, split.RightOnly(), split.LeftOnly(), delta)
	if !committed {
		return Contribution{}, false
	}
	// commit preserves its predecessor verbatim on a physical no-op.  A
	// ContributionBase is deliberately marked unpublishable, however, so the
	// successful Finish cut must normalize that otherwise identical immutable
	// State to this Work's ordinary authority before admitting it.  Roots,
	// scope, and support already crossed the sole publication/closure cut
	// above; this is only the nominal base-to-normal authority transition.
	state.authority = work.authority
	return work.admitConstructedContribution(state, coverage)
}

// AbortContribution consumes every ready but unpublished Patch and invalidates
// the base. Epoch cancellation and callback failure must use this operation;
// it is the only cleanup path for a partially staged group.
func (work *Work) AbortContribution(base ContributionBase, patches []Patch) bool {
	return work.abortContribution(base.value, patches)
}

// AbortRuleContribution is the nominal PointState-input cleanup cut. It
// consumes only patches prepared from this exact one-shot base, exactly like
// the compatibility API; PointState inputs themselves remain immutable.
func (work *Work) AbortRuleContribution(base RuleContributionBase, patches []Patch) bool {
	return work.abortContribution(base.value, patches)
}

func (work *Work) abortContribution(owner *contributionBase, patches []Patch) bool {
	if !work.ownsContributionBase(owner) {
		return false
	}
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
	owner.environment = contributionInput{}
	owner.hasEnvironment = false
	owner.rootsOwned = false
}

// assembleAuthoredRows gathers one slot's authored rows from the group's
// three declared sources. A slot with a single already-canonical source hands
// that immutable row vector straight to the canonicalizer, so the common
// sparse case copies nothing.
func (work *Work) assembleAuthoredRows(owner *contributionBase, patches []Patch, slot shape.Slot) []TargetRegion {
	var sole []TargetRegion
	sources, total := 0, 0
	if owner.plan.environment && !owner.plan.carrySlots.Test(int(slot)) {
		if rows := owner.environment.coverage.slot(slot).targets; len(rows) != 0 {
			sole, sources, total = rows, sources+1, total+len(rows)
		}
	}
	for _, carry := range owner.plan.carries {
		if carry.Slot != slot {
			continue
		}
		if rows := owner.inputs[carry.Input].coverage.slot(slot).targets; len(rows) != 0 {
			sole, sources, total = rows, sources+1, total+len(rows)
		}
	}
	for _, patch := range patches {
		if patch.slot == slot && len(patch.authored) != 0 {
			sole, sources, total = patch.authored, sources+1, total+len(patch.authored)
		}
	}
	if sources <= 1 {
		return sole
	}
	rows := make([]TargetRegion, 0, total)
	if owner.plan.environment && !owner.plan.carrySlots.Test(int(slot)) {
		rows = append(rows, owner.environment.coverage.slot(slot).targets...)
	}
	for _, carry := range owner.plan.carries {
		if carry.Slot == slot {
			rows = append(rows, owner.inputs[carry.Input].coverage.slot(slot).targets...)
		}
	}
	for _, patch := range patches {
		if patch.slot == slot {
			rows = append(rows, patch.authored...)
		}
	}
	return rows
}

func (work *Work) canBorrowClosedContributionSlot(owner *contributionBase, slot shape.Slot, retained support.Mask, final slotCoverage) bool {
	if work == nil || owner == nil || owner.plan == nil || !retained.Valid() || !owner.state.support.Equal(retained) {
		return false
	}
	source, ok := contributionSourceAt(owner, slot)
	if !ok || !source.closed || !source.state.Support().Equal(retained) || !sameSlotCoverage(source.coverage.slot(slot), final) {
		return false
	}
	sourceRoot, sourceOK := source.state.HandleAt(slot)
	ownerRoot, ownerOK := owner.state.HandleAt(slot)
	return sourceOK && ownerOK && sameRoot(sourceRoot, ownerRoot)
}

func contributionSourceAt(owner *contributionBase, slot shape.Slot) (contributionInput, bool) {
	if owner == nil || owner.plan == nil {
		return contributionInput{}, false
	}
	if owner.plan.carrySlots.Test(int(slot)) {
		for _, carry := range owner.plan.carries {
			if carry.Slot == slot && carry.Input >= 0 && carry.Input < len(owner.inputs) {
				return owner.inputs[carry.Input], true
			}
		}
	}
	if owner.plan.environment && owner.hasEnvironment {
		return owner.environment, true
	}
	return contributionInput{}, false
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

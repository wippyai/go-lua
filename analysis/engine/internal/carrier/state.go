package carrier

import (
	"sort"
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// MergeKind selects the one typed operation run on the overlap of a
// carrier-owned three-region split.
type MergeKind uint8

const (
	Join MergeKind = iota + 1
	Widen
	Narrow
)

// FactorOperation is a cold typed-Binding entry before it receives a physical
// layout slot. Operations arrive in solver-canonical order. Preflight consumes
// the operation and completes every fallible local allocation before carrier
// attaches any issuer or publishes an initial root.
type FactorOperation interface {
	Preflight() (SlotOperation, bool)
}

// SlotOperation is the prepared, payload-free typed authority for one slot.
// Attach is total after Preflight: it attaches the private physical owner and
// publishes the previously reserved initial root. Carrier never boxes or
// reflects on a domain payload.
type SlotOperation interface {
	Attach(SlotOwner) RootHandle
	Guards() *guard.Manager
	// InitialRootReady proves every fallible and semantic check for the
	// operation's reserved initial root completed during Preflight. Attach is
	// therefore a total publication operation, never a late validation route.
	InitialRootReady() bool
	ValidRoot(RootHandle) bool
	// Declared* proves cold membership in this operation's immutable
	// declaration table.  It must not require issuer attachment or make a
	// capability live; PreparedComposition uses it before the final cut.
	DeclaredUnit(Unit) bool
	DeclaredTarget(Target) bool
	DeclaredSelector(Selector, SelectorKind) bool
	// TargetNotifications is cold structural evidence for the exact set of
	// declared observations a write through Target may invalidate. Target
	// itself remains the opaque authored write scope; this query deliberately
	// does not expose that scope's keys or confuse it with notification rows.
	TargetNotifications(Target) ([]Unit, bool)
	// PrepareWidening and PrepareNarrowing seal one canonical nonempty set of
	// this Factor's Targets into an opaque operation-local scope ordinal. The
	// ordinal is meaningful only to this SlotOperation and its SlotWork;
	// neither keys nor a second generic scope vocabulary cross the carrier
	// boundary.
	PrepareWidening([]Target) (uint64, bool)
	PrepareNarrowing([]Target) (uint64, bool)
	DeclaredSelectorTargets(Selector) ([]Target, bool)
	// Valid* is the corresponding live authority after Attach.
	ValidUnit(Unit) bool
	ValidTarget(Target) bool
	ValidSelector(Selector, SelectorKind) bool
	Supports(MergeKind) bool
	NewWork() (SlotWork, bool)
}

// RuntimeRecurrenceOperation is the optional late immutable selection seam
// used when a structural activation turns an already-sealed acyclic graph into
// a feedback region. It may only resolve existing Target capabilities into a
// slot-private key set; it has no authority to add keys, targets, domains, or
// typed facts. Production fact bindings implement it. Structural test doubles
// that never accept live feedback overlays may omit it.
type RuntimeRecurrenceOperation interface {
	PrepareRuntimeWidening([]Target) (uint64, bool)
	PrepareRuntimeNarrowing([]Target) (uint64, bool)
}

// SlotWork is one operation's evaluator-local state. Its concrete
// implementation remains typed beside the Binding, where it may retain typed
// traversal storage without teaching carrier any payload vocabulary.
// A SlotWork belongs to exactly one Work and is not concurrent or reentrant.
type SlotWork interface {
	// SetCheckpoint receives the evaluator epoch's opaque cancellation probe.
	// It must retain no scheduler/context state and must not turn a false probe
	// into a partial output. This is installed once before evaluation, never on
	// a semantic hot path.
	SetCheckpoint(Checkpoint) bool
	EqualUnder(left, right RootHandle, within support.Mask) bool
	LessOrEqUnder(left, right RootHandle, within support.Mask) bool
	// LessOrEqContributionUnder is the typed order boundary for a closed
	// RuleContribution.  Coverage is the sole presence authority: undefined
	// under a row is Present(Default), and every cell outside it is Absent.
	LessOrEqContributionUnder(left, right RootHandle, leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage SlotCoverage) (bool, bool)
	// ContributionClosedUnder is the issuance-time proof that no physical
	// non-Default root cell lies outside the compact authored surface or final
	// outer support.  Carrier never calls it on hot admitted reads.
	ContributionClosedUnder(root RootHandle, within support.Mask, coverage SlotCoverage) bool
	// ContributionPresenceIncludedUnder proves extensional authored-presence
	// inclusion after expanding opaque Target rows beside their typed Binding.
	// It intentionally ignores payload order: selected Widen needs to reject
	// Present-to-Absent descent before applying its value operation.
	ContributionPresenceIncludedUnder(leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage SlotCoverage) bool
	// MergeContributionUnder folds one independently authored producer slot
	// into the accumulated Point slot. Coverage, not sparse root shape, decides
	// whether a region is absent, installs explicit Default, or invokes Join.
	MergeContributionUnder(left, right RootHandle, leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage SlotCoverage, delta *support.Work) (ChangeHandle, bool)
	// OverlayPointRHSUnder applies a closed RuleContribution to a semantic
	// PointRHS whose outer support is unchanged. Both coverage surfaces remain
	// lifted-partial: an absent left cell is not Factor Default. Implementations
	// preserve latent left root fibers only outside leftSupport, while using
	// leftCoverage inside leftSupport and rightCoverage for the sparse overlay.
	// Support growth is deliberately a separate closed lifted join.
	OverlayPointRHSUnder(left, right RootHandle, leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage SlotCoverage, delta *support.Work) (ChangeHandle, bool)
	// MergeTransportedPointUnder is the fused Point-state environment edge.
	// It applies total-Default State transport through the relation, then joins
	// under the transported output coverage and closes the final RHS root.  It
	// intentionally has no source coverage: PointState transport is semantic,
	// not lifted-partial RuleContribution transport.
	MergeTransportedPointUnder(left, right RootHandle, leftSupport, sourceSupport, reindexedSupport, rightSupport support.Mask, relation guard.Reindex, leftCoverage, rightCoverage SlotCoverage, delta *support.Work) (ChangeHandle, bool)
	Merge3Under(kind MergeKind, recurrence bool, scope uint64, left, right RootHandle, split support.Split, delta *support.Work) (ChangeHandle, bool)
	// MergeSelectedUnder is the typed half of one three-State recurrence
	// transition. Selected target keys apply kind to current and
	// selectedRight; every other key installs exactRight. It returns one
	// current-to-output change proof and never publishes an intermediate root.
	MergeSelectedUnder(kind MergeKind, scope uint64, current, selectedRight, exactRight RootHandle, selectedSplit, exactSplit support.Split, delta *support.Work) (ChangeHandle, bool)
	// MergeSelectedContributionUnder is the closed contribution recurrence
	// boundary.  It publishes only the exact-right authored surface, never a
	// historical current surface retained by Widen/Narrow.
	MergeSelectedContributionUnder(kind MergeKind, scope uint64, current, selectedRight, exactRight RootHandle, selectedSplit, exactSplit support.Split, currentCoverage, selectedCoverage, exactCoverage SlotCoverage, delta *support.Work) (ChangeHandle, bool)
	// ReindexUnder transports this exact typed root through carrier's sealed
	// source-to-target relation. source is the only support from which fibers
	// may contribute; target is the already transformed outer support. The
	// relation is opaque and complete, so a slot receives no caller atom list
	// or substitution map and cannot choose a competing coordinate transport.
	ReindexUnder(left RootHandle, source, target support.Mask, relation guard.Reindex, delta *support.Work) (ChangeHandle, bool)
	// ReindexContributionUnder is the closed, lifted-partial contribution
	// transport boundary.  It receives both source and target authored rows;
	// raw State ReindexUnder remains deliberately totalizing Default.
	ReindexContributionUnder(left RootHandle, source, target support.Mask, relation guard.Reindex, sourceCoverage, targetCoverage SlotCoverage, delta *support.Work) (ChangeHandle, bool)
	// ReindexPointContributionUnder is total-Default PointState transport
	// followed by a final RHS close to target coverage.  It is deliberately
	// distinct from ReindexContributionUnder so source Absence cannot be
	// mistaken for RuleContribution Absence.
	ReindexPointContributionUnder(left RootHandle, source, target support.Mask, relation guard.Reindex, targetCoverage SlotCoverage, delta *support.Work) (ChangeHandle, bool)
	// CloseContributionUnder physically masks an arbitrary staged root to the
	// final authored surface before it can cross a contribution publication
	// cut.  input may be a transaction-local preview root; the returned handle
	// is always a normal pending publication rooted at before.
	CloseContributionUnder(before, input RootHandle, within support.Mask, coverage SlotCoverage, delta *support.Work) (ChangeHandle, bool)
	// ReplaceUnder is the structural coordinate-replacement half of one
	// carrier Replace. It retains right exactly and reports only old-to-right
	// semantic differences in split.Overlap(). It is not a lattice operation.
	ReplaceUnder(left, right RootHandle, split support.Split, delta *support.Work) (ChangeHandle, bool)
	BeginObservation() bool
	EndObservation() bool
	ObserveUnder(root RootHandle, unit Unit, within support.Mask, visit func(ObservationRow) bool) bool
}

// ChangedPointSlotWork is the optional sparse ascent path implemented by the
// production typed Binding. It consumes one exact published Point transition
// and applies only its owner-issued semantic/authorship regions through a
// coordinate-identity environment boundary. Structural test doubles may omit
// it; carrier then uses the complete transport fold.
type ChangedPointSlotWork interface {
	MergeChangedCoordinatePointUnder(left, current RootHandle, leftSupport, currentSupport, targetSupport, pre, post support.Mask, leftCoverage, currentCoverage SlotCoverage, semantic []ChangeSet, authored CoverageChangeSet, delta *support.Work) (ChangeHandle, bool)
}

// EpochSlotWork is the optional lifecycle half of a typed SlotWork that
// publishes dynamic roots.  The carrier invokes it exactly once while opening
// a Work and exactly once while closing or evicting that Work.  It carries an
// opaque RootEpoch, never a domain value or a second State representation.
//
// Test-only structural SlotWork implementations that never own typed dynamic
// roots need not implement this interface.  The canonical factbinding work
// does, so every production candidate root is epoch-owned.
type EpochSlotWork interface {
	BindRootEpoch(*RootEpoch) bool
	CloseRootEpoch()
}

// SlotOwner is a private layout capability supplied to one FactorOperation
// exactly while Composition seals. It binds a handle to one exact physical
// vector slot.
type SlotOwner struct{ value *slotOwner }

// layout is shared by every immutable Composition copy for one owner epoch.
// publish serializes the sole root-store publication cut across those copies.
type layout struct {
	marker    byte
	publish   sync.Mutex
	published atomic.Bool
}

type slotOwner struct {
	layout *layout
	slot   shape.Slot
}

// PreparedComposition is the cold, single-use composition authority.  It has
// fixed canonical operations, shape, and guard universe, but deliberately has
// neither attached issuers nor published roots.  It may therefore validate a
// Rule projection without making any retained capability live.
//
// value keeps copied PreparedComposition values on the same single attach cut.
// The type is exported only inside analysis/engine/internal so projection and
// Solver can retain this authority without exposing a second public route.
type PreparedComposition struct{ value *preparedComposition }

type preparedComposition struct {
	shape      *shape.Shape
	layout     *layout
	operations []SlotOperation
	guards     *guard.Manager

	attach      sync.Mutex
	composition atomic.Pointer[Composition]
}

// Composition is the cold sealed runtime composition of arbitrary operations.
// Input order fixes its dense physical layout; State has only a parallel
// RootHandle vector and never a second dynamic vocabulary.
type Composition struct {
	shape      *shape.Shape
	layout     *layout
	operations []SlotOperation
	initial    []RootHandle
	all        []bool
	zeroScopes []uint64
	guards     *guard.Manager
	scope      Scope
	authority  *stateAuthority

	// scopeMu serializes the cold transition from scope construction to first
	// evaluator work. It is never acquired by State operations or SlotWork.
	scopeMu    sync.Mutex
	workOpened bool
}

// stateAuthority is the one provenance pointer carried by every State.  The
// normal authority is owned by Composition; Preview substitutes a temporary
// authority with the same Composition and its linear owner.  This preserves
// the 48-byte hot State layout without encoding lifecycle in slice internals.
type stateAuthority struct {
	composition  *Composition
	epoch        *RootEpoch
	preview      *previewOwner
	contribution *contributionBase
}

func (authority *stateAuthority) live() bool {
	return authority != nil && authority.composition != nil && authority.composition.shape != nil && authority.composition.guards != nil && (authority.epoch == nil || authority.epoch.Live())
}

// Work is one evaluator-owned dense vector of operation-local work objects.
// States remain immutable and shareable; Work supplies the sole mutable
// traversal storage needed to compare or merge opaque typed roots.
type Work struct {
	composition *Composition
	slots       []SlotWork
	// contributionSeal and neutralSeal are private admission tokens shared by
	// immutable Contributions published through this evaluator Work. The latter
	// is issued only for the exact initial-root/false-support identity; neither
	// token is a registry or a second State/coverage store.
	contributionSeal *contributionSeal
	neutralSeal      *contributionSeal
	// supportWork is the carrier-owned Boolean scratch shell. It is reused
	// only across terminal support transactions; an open delta remains in this
	// shell while typed SlotWork runs, so nested typed support work owns its
	// separate Binding-local shell.
	supportWork *support.Work
	epoch       *RootEpoch
	authority   *stateAuthority
	// checkpointProbe is the one Work-owned liveness callback shared by all
	// support transactions.  It is installed once when Work is opened; hot
	// support operations only select it or nil and never manufacture a
	// closure capturing Work.
	checkpointProbe Checkpoint
	checkpoint      Checkpoint
	publishing      bool
	previewing      bool
	replacing       bool
	reindexing      bool
}

// RetainedWork is the sole internal ownership unit for a validated completed
// solution cache.  It keeps one immutable epoch arena alive for reuse without
// making public State own carrier roots.  Close is the explicit eviction cut.
// It has no evaluator API, so retaining it cannot reopen or mutate a solve.
type RetainedWork struct {
	composition *Composition
	slots       []SlotWork
	epoch       *RootEpoch
}

// PrepareComposition freezes operations already in canonical order and
// validates one shared guard universe. Preflight normally derives that
// universe from the operations; a zero-width composition supplies the already
// graph-bound universe explicitly. Both cases produce the same Composition,
// Work, contribution, and publication authority. This phase does not attach
// issuers or publish initial roots.
func PrepareComposition(operations []FactorOperation, universes ...*guard.Manager) (*PreparedComposition, bool) {
	if len(universes) > 1 {
		return nil, false
	}
	var universe *guard.Manager
	if len(universes) == 1 {
		universe = universes[0]
		if universe == nil {
			return nil, false
		}
	}
	owner, ok := shape.Seal(len(operations))
	if !ok {
		return nil, false
	}
	prepared := &preparedComposition{shape: owner, layout: &layout{}, operations: make([]SlotOperation, len(operations)), guards: universe}
	valid := true
	for index, operation := range operations {
		if operation == nil {
			valid = false
			continue
		}
		preparedOperation, ok := operation.Preflight()
		if !ok || preparedOperation == nil || !preparedOperation.InitialRootReady() {
			valid = false
			continue
		}
		if prepared.guards == nil {
			prepared.guards = preparedOperation.Guards()
		}
		if prepared.guards == nil || preparedOperation.Guards() != prepared.guards {
			valid = false
			continue
		}
		prepared.operations[index] = preparedOperation
	}
	if !valid {
		return nil, false
	}
	if prepared.guards == nil {
		return nil, false
	}
	return &PreparedComposition{value: prepared}, true
}

// Attach crosses the sole composition publication cut.  Every prepared
// operation has already completed fallible work, so the first call only binds
// canonical slot owners and publishes their reserved roots.  Copies share the
// same one-shot state; a later call cannot attach again.
func (prepared *PreparedComposition) Attach() (*Composition, bool) {
	if prepared == nil || prepared.value == nil {
		return nil, false
	}
	value := prepared.value
	value.attach.Lock()
	defer value.attach.Unlock()
	if value.composition.Load() != nil || value.layout.published.Load() {
		return nil, false
	}
	composition := &Composition{shape: value.shape, layout: value.layout, operations: value.operations, initial: make([]RootHandle, len(value.operations)), all: make([]bool, len(value.operations)), zeroScopes: make([]uint64, len(value.operations)), guards: value.guards}
	composition.scope = Scope{composition: composition, guard: value.guards.AllScope()}
	composition.authority = &stateAuthority{composition: composition}
	for index, operation := range composition.operations {
		slot := SlotOwner{value: &slotOwner{layout: composition.layout, slot: shape.Slot(index)}}
		initial := operation.Attach(slot)
		if initial.issuer == nil || initial.id == 0 || initial.id&previewRootBit != 0 {
			// Attach is an internal total contract.  This is only a structural
			// assertion for a malicious implementation; layout remains closed,
			// so no partial issuer/root publication can become externally live.
			panic("prepared initial root missing structural identity")
		}
		composition.initial[index] = initial
		composition.all[index] = true
	}
	// Store only after every total issuer/root attachment completes, then open
	// the shared layout latch once.  External issuer/capability paths acquire
	// that latch before exposing a slot, root, or Binding-owned authority.
	value.composition.Store(composition)
	value.layout.published.Store(true)
	return composition, true
}

// Shape returns the provisional dense physical layout.  A shape slot is
// canonical ordering information only; it does not make any capability's
// Slot method live before Attach.
func (prepared *PreparedComposition) Shape() *shape.Shape {
	if prepared == nil || prepared.value == nil {
		return nil
	}
	return prepared.value.shape
}

// Guards returns the one guard universe proved common during preflight.
func (prepared *PreparedComposition) Guards() *guard.Manager {
	if prepared == nil || prepared.value == nil {
		return nil
	}
	return prepared.value.guards
}

// OwnsUnit proves a cold Unit capability belongs to the prepared operation at
// its provisional canonical slot.  Issuers remain unattached throughout this
// check; Slot visibility is reserved for Attach.
func (prepared *PreparedComposition) OwnsUnit(slot shape.Slot, unit Unit) bool {
	if prepared == nil || prepared.value == nil {
		return false
	}
	value := prepared.value
	value.attach.Lock()
	defer value.attach.Unlock()
	if value.composition.Load() != nil || value.layout.published.Load() || value.shape == nil || !value.shape.ValidSlot(slot) || !value.operations[int(slot)].DeclaredUnit(unit) {
		return false
	}
	return unit.issuer != nil && unit.issuer.slot.Load() == nil && unit.id != 0 && unit.closure != 0 && (unit.kind == ExactUnit || unit.kind == SummaryUnit)
}

// OwnsTarget proves a cold Target capability belongs to the prepared
// operation at its provisional canonical slot.
func (prepared *PreparedComposition) OwnsTarget(slot shape.Slot, target Target) bool {
	if prepared == nil || prepared.value == nil {
		return false
	}
	value := prepared.value
	value.attach.Lock()
	defer value.attach.Unlock()
	if value.composition.Load() != nil || value.layout.published.Load() || value.shape == nil || !value.shape.ValidSlot(slot) || !value.operations[int(slot)].DeclaredTarget(target) {
		return false
	}
	return target.issuer != nil && target.issuer.slot.Load() == nil && target.id != 0 && (target.mode == StrongTarget || target.mode == WeakTarget)
}

// TargetNotifications returns the frozen possible notification closure of a
// cold target owned by slot. Target itself is the authored write scope; the
// returned Units are only the complete reverse wake surface.
func (prepared *PreparedComposition) TargetNotifications(slot shape.Slot, target Target) ([]Unit, bool) {
	if prepared == nil || prepared.value == nil {
		return nil, false
	}
	value := prepared.value
	value.attach.Lock()
	defer value.attach.Unlock()
	if value.composition.Load() != nil || value.layout.published.Load() || value.shape == nil || !value.shape.ValidSlot(slot) || !value.operations[int(slot)].DeclaredTarget(target) || target.issuer == nil || target.issuer.slot.Load() != nil || target.id == 0 || (target.mode != StrongTarget && target.mode != WeakTarget) {
		return nil, false
	}
	return value.operations[int(slot)].TargetNotifications(target)
}

// OwnsSelectorAt proves a cold selector belongs to the prepared operation at
// the caller-declared provisional canonical slot.  A selector cannot expose
// Slot before Attach, so the schema supplies that routing position directly.
func (prepared *PreparedComposition) OwnsSelectorAt(slot shape.Slot, selector Selector, kind SelectorKind) bool {
	if prepared == nil || prepared.value == nil {
		return false
	}
	value := prepared.value
	value.attach.Lock()
	defer value.attach.Unlock()
	if value.composition.Load() != nil || value.layout.published.Load() || value.shape == nil || !value.shape.ValidSlot(slot) || !value.operations[int(slot)].DeclaredSelector(selector, kind) {
		return false
	}
	return selector.issuer != nil && selector.issuer.slot.Load() == nil && selector.kind == kind && selector.id != 0
}

// SelectorTargets returns the complete finite candidate surface of one cold
// target selector owned by slot.
func (prepared *PreparedComposition) SelectorTargets(slot shape.Slot, selector Selector) ([]Target, bool) {
	if prepared == nil || prepared.value == nil {
		return nil, false
	}
	value := prepared.value
	value.attach.Lock()
	defer value.attach.Unlock()
	if value.composition.Load() != nil || value.layout.published.Load() || value.shape == nil || !value.shape.ValidSlot(slot) || !value.operations[int(slot)].DeclaredSelector(selector, TargetSelector) || selector.issuer == nil || selector.issuer.slot.Load() != nil || selector.kind != TargetSelector || selector.id == 0 {
		return nil, false
	}
	return value.operations[int(slot)].DeclaredSelectorTargets(selector)
}

// Count returns the number of dynamically composed operations.
func (composition *Composition) Count() int {
	if composition == nil {
		return 0
	}
	return len(composition.operations)
}

// Guards returns the one guard universe proved common to every sealed operation.
func (composition *Composition) Guards() *guard.Manager {
	if composition == nil {
		return nil
	}
	return composition.guards
}

// NewWork makes the one dense hot dispatch vector for an evaluator of this
// sealed composition. It runs once per evaluator, never per State operation.
func (composition *Composition) NewWork() (*Work, bool) {
	if composition == nil || composition.shape == nil || composition.guards == nil || len(composition.operations) != composition.shape.Count() {
		return nil, false
	}
	composition.scopeMu.Lock()
	composition.workOpened = true
	composition.scopeMu.Unlock()
	epoch := newRootEpoch(composition.layout)
	if epoch == nil {
		return nil, false
	}
	supportWork := support.New(composition.guards)
	if supportWork == nil {
		epoch.close()
		return nil, false
	}
	work := &Work{composition: composition, slots: make([]SlotWork, len(composition.operations)), supportWork: supportWork, epoch: epoch, authority: &stateAuthority{composition: composition, epoch: epoch}}
	work.checkpointProbe = func() bool { return work.live() }
	work.contributionSeal = &contributionSeal{work: work, composition: composition}
	work.neutralSeal = &contributionSeal{work: work, composition: composition}
	for index, operation := range composition.operations {
		slot, ok := operation.NewWork()
		if !ok || slot == nil {
			work.Close()
			return nil, false
		}
		work.slots[index] = slot
		if rooted, ownsRoots := slot.(EpochSlotWork); ownsRoots && !rooted.BindRootEpoch(epoch) {
			work.Close()
			return nil, false
		}
	}
	return work, true
}

// Close revokes one unfinished evaluator epoch.  Revocation happens before
// slot stores are detached, making all escaped candidate handles and States
// fail closed even if a stale caller still retains a Work/State shell.
func (work *Work) Close() bool {
	if work == nil || work.epoch == nil || work.publishing || work.previewing || work.replacing || work.reindexing {
		return false
	}
	if !work.epoch.close() {
		return false
	}
	if work.supportWork != nil {
		work.supportWork.Close()
		work.supportWork = nil
	}
	closeEpochSlotWorks(work.slots)
	clear(work.slots)
	work.slots = nil
	work.composition = nil
	work.contributionSeal = nil
	work.neutralSeal = nil
	work.authority = nil
	work.checkpointProbe = nil
	work.checkpoint = nil
	work.epoch = nil
	return true
}

// Retain transfers one fully validated, quiescent epoch arena into the sole
// immutable completed-solution cache ownership unit.  Public State remains
// only frozen Query results; the runtime may later Close the returned lease on
// invalidation or eviction.  An active Work loses all evaluator authority at
// this cut and cannot be used to publish further roots.
func (work *Work) Retain() (*RetainedWork, bool) {
	if work == nil || work.composition == nil || work.epoch == nil || !work.epoch.Active() || work.publishing || work.previewing || work.replacing || work.reindexing {
		return nil, false
	}
	if !work.epoch.retain() {
		return nil, false
	}
	if work.supportWork != nil {
		work.supportWork.Close()
		work.supportWork = nil
	}
	retained := &RetainedWork{composition: work.composition, slots: work.slots, epoch: work.epoch}
	work.composition = nil
	work.contributionSeal = nil
	work.neutralSeal = nil
	work.slots = nil
	work.authority = nil
	work.checkpointProbe = nil
	work.checkpoint = nil
	work.epoch = nil
	return retained, true
}

// Live reports whether this retained cache still owns one resolvable immutable
// root arena.  It intentionally says nothing about evaluator execution.
func (retained *RetainedWork) Live() bool {
	return retained != nil && retained.composition != nil && retained.epoch != nil && retained.epoch.Live()
}

// Close evicts a retained completed cache.  It revokes roots before clearing
// typed stores, exactly like cancellation, and is deliberately one-shot.
func (retained *RetainedWork) Close() bool {
	if retained == nil || retained.epoch == nil || !retained.epoch.close() {
		return false
	}
	closeEpochSlotWorks(retained.slots)
	clear(retained.slots)
	retained.slots = nil
	retained.composition = nil
	retained.epoch = nil
	return true
}

func closeEpochSlotWorks(slots []SlotWork) {
	for _, slot := range slots {
		if rooted, ownsRoots := slot.(EpochSlotWork); ownsRoots {
			rooted.CloseRootEpoch()
		}
	}
}

// SetCheckpoint installs the one opaque liveness probe for this evaluator
// epoch. It is a cold epoch setup operation: replacing it while a carrier
// operation is active would make one attempt observe two different epochs and
// is therefore rejected. A nil probe restores the allocation-free normal
// path. SlotWork receives the same probe solely to stop private structural
// traversals before they prepare a ChangeHandle.
func (work *Work) SetCheckpoint(checkpoint Checkpoint) bool {
	if work == nil || work.composition == nil || work.epoch == nil || !work.epoch.Active() || work.publishing || work.previewing || work.replacing || work.reindexing {
		return false
	}
	for _, slot := range work.slots {
		if slot == nil || !slot.SetCheckpoint(checkpoint) {
			return false
		}
	}
	work.checkpoint = checkpoint
	return true
}

func (work *Work) live() bool {
	return work != nil && work.composition != nil && work.epoch != nil && work.epoch.Active() && work.checkpoint.live()
}

// Checkpoint samples the evaluator's one opaque epoch liveness probe. It is
// for executor-owned traversal boundaries such as Product and Query rows;
// callers receive only completion/failure, never a cancellation authority or
// semantic budget.
func (work *Work) Checkpoint() bool { return work.live() }

func (work *Work) checkpointFunc() func() bool {
	if work == nil || work.checkpoint == nil {
		return nil
	}
	return work.checkpointProbe
}

func (work *Work) newSupportWork() *support.Work {
	if !work.live() || work.composition == nil {
		return nil
	}
	if work.supportWork == nil {
		work.supportWork = support.New(work.composition.guards)
	}
	if work.supportWork == nil || !work.supportWork.BeginTransaction(work.checkpointProbe) {
		return nil
	}
	return work.supportWork
}

func (work *Work) threeSupport(left, right support.Mask) (support.Split, bool) {
	if !work.live() || work.supportWork == nil {
		return support.Split{}, false
	}
	return support.ThreeWithWork(work.supportWork, work.checkpointFunc(), left, right)
}

func (work *Work) intersectSupport(left, right support.Mask) (support.Mask, bool) {
	if !work.live() || work.supportWork == nil {
		return support.Mask{}, false
	}
	return support.IntersectWithWork(work.supportWork, work.checkpointFunc(), left, right)
}

func (work *Work) unionSupport(left, right support.Mask) (support.Mask, bool) {
	if !work.live() || work.supportWork == nil {
		return support.Mask{}, false
	}
	return support.UnionWithWork(work.supportWork, work.checkpointFunc(), left, right)
}

func (work *Work) reindexSupport(mask support.Mask, plan guard.Reindex) (support.Mask, bool) {
	if !work.live() || work.supportWork == nil {
		return support.Mask{}, false
	}
	return support.ReindexWithWork(work.supportWork, mask, plan)
}

// SlotWork returns the exact attached typed evaluator for one physical slot.
// It exposes only the payload-free SlotWork interface; semantic observation
// resolution remains with the owning Binding.
func (work *Work) SlotWork(slot shape.Slot) (SlotWork, bool) {
	if work == nil || work.composition == nil || work.composition.shape == nil || !work.composition.shape.ValidSlot(slot) || len(work.slots) != work.composition.Count() {
		return nil, false
	}
	result := work.slots[int(slot)]
	return result, result != nil
}

// OwnsState proves that state is an immutable carrier produced for this
// evaluator's exact sealed composition. It is intentionally structural and
// O(1): a State can only be assembled through NewState or carrier publication,
// and each typed root is checked by its Factor at the slot that consumes it.
// Calling State.Valid here would turn a one-slot admission into an O(F) scan.
func (work *Work) OwnsState(state State) bool {
	return work != nil && work.live() && len(work.slots) == len(work.composition.operations) &&
		state.live() && state.authority != nil && state.authority.composition == work.composition
}

// OwnsViewOf proves that view is a restricted read of this exact immutable
// predecessor. sameState binds its private composition, root-vector backing,
// and support-handle identities, so semantically equal but separately
// published predecessors cannot be exchanged at an exact-predecessor cut.
func (work *Work) OwnsViewOf(state State, view View) bool {
	return work.OwnsState(state) && view.state.live() && sameState(state, view.state) &&
		view.support.Valid() && view.support.Manager() == work.composition.guards &&
		view.support.Entails(state.support)
}

// OwnsUnit proves a unit belongs to exactly this composition and physical
// slot. It validates only structural issuer/closure data; the typed
// Binding remains the sole resolver of its semantic meaning.
func (composition *Composition) OwnsUnit(slot shape.Slot, unit Unit) bool {
	owner := issuerOwner(unit.issuer, true)
	return composition != nil && composition.shape != nil && composition.shape.ValidSlot(slot) && owner != nil && owner.layout == composition.layout && owner.slot == slot && unit.id != 0 && unit.closure != 0 && (unit.kind == ExactUnit || unit.kind == SummaryUnit) && composition.operations[int(slot)].ValidUnit(unit)
}

// OwnsTarget proves a typed write target belongs to one exact layout slot.
func (composition *Composition) OwnsTarget(slot shape.Slot, target Target) bool {
	owner := issuerOwner(target.issuer, true)
	return composition != nil && composition.shape != nil && composition.shape.ValidSlot(slot) && owner != nil && owner.layout == composition.layout && owner.slot == slot && target.id != 0 && (target.mode == StrongTarget || target.mode == WeakTarget) && composition.operations[int(slot)].ValidTarget(target)
}

// TargetNotifications returns the immutable possible reverse wake closure of
// one live target. It does not reveal or stand in for the target's authored
// key scope.
func (composition *Composition) TargetNotifications(slot shape.Slot, target Target) ([]Unit, bool) {
	if !composition.OwnsTarget(slot, target) {
		return nil, false
	}
	return composition.operations[int(slot)].TargetNotifications(target)
}

// MergeScope is a sealed membership mask for recurrence operations. It is
// bound to one Composition and avoids hot duplicate scans or caller-supplied
// arbitrary slot slices.
type MergeScope struct {
	composition *Composition
	members     []bool
	// scopes is zero for Join. A selected Widen or Narrow scope is
	// operation-private metadata prepared once at seal time; recurrence rejects
	// a selected zero scope.
	scopes []uint64
	kind   MergeKind
	all    bool
}

// AllMergeScope selects every operation. Join requires this exact scope so
// no plane can be accidentally skipped on a right-only support region.
func (composition *Composition) AllMergeScope() MergeScope {
	if composition == nil {
		return MergeScope{}
	}
	return MergeScope{composition: composition, members: composition.all, scopes: composition.zeroScopes, kind: Join, all: true}
}

// SealWidening prepares exact authored Target scopes for one scoped carrier
// Widen. Targets are canonically ordered capabilities; each belongs to one
// attached Factor and is converted once into that Factor's private key scope.
// An empty target set is a valid factor-free recurrence selection. The
// resulting MergeScope performs no Target/key work on the hot merge path.
func (composition *Composition) SealWidening(targets []Target) (MergeScope, bool) {
	return composition.sealRecurrence(Widen, targets, false)
}

// SealNarrowing prepares exact authored Target scopes for one key-local
// carrier Narrow. Narrow has no factor-wide form: every selected key must be
// covered by an attached Factor target and its typed descent measure.
func (composition *Composition) SealNarrowing(targets []Target) (MergeScope, bool) {
	return composition.sealRecurrence(Narrow, targets, false)
}

// SealRuntimeWidening and SealRuntimeNarrowing are the post-Work counterparts
// used by a settled selected-edge overlay. They retain the same opaque
// MergeScope representation and differ only in lifecycle admission.
func (composition *Composition) SealRuntimeWidening(targets []Target) (MergeScope, bool) {
	return composition.sealRecurrence(Widen, targets, true)
}

func (composition *Composition) SealRuntimeNarrowing(targets []Target) (MergeScope, bool) {
	return composition.sealRecurrence(Narrow, targets, true)
}

func (composition *Composition) sealRecurrence(kind MergeKind, targets []Target, runtime bool) (MergeScope, bool) {
	if composition == nil || composition.shape == nil || (kind != Widen && kind != Narrow) {
		return MergeScope{}, false
	}
	composition.scopeMu.Lock()
	defer composition.scopeMu.Unlock()
	if composition.workOpened != runtime {
		return MergeScope{}, false
	}
	ordered := append([]Target(nil), targets...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].Less(ordered[right]) })
	unique := ordered[:0]
	for _, target := range ordered {
		if len(unique) == 0 || !unique[len(unique)-1].Same(target) {
			unique = append(unique, target)
		}
	}
	ordered = unique
	members := make([]bool, composition.Count())
	scopes := make([]uint64, composition.Count())
	for begin := 0; begin < len(ordered); {
		slot, ok := ordered[begin].Slot()
		if !ok || !composition.shape.ValidSlot(slot) || !composition.OwnsTarget(slot, ordered[begin]) {
			return MergeScope{}, false
		}
		end := begin + 1
		for end < len(ordered) {
			nextSlot, valid := ordered[end].Slot()
			if !valid {
				return MergeScope{}, false
			}
			if nextSlot != slot {
				break
			}
			if !composition.OwnsTarget(slot, ordered[end]) {
				return MergeScope{}, false
			}
			end++
		}
		if members[int(slot)] || !composition.operations[int(slot)].Supports(kind) {
			return MergeScope{}, false
		}
		var scope uint64
		var valid bool
		if runtime {
			operation, supported := composition.operations[int(slot)].(RuntimeRecurrenceOperation)
			if !supported {
				return MergeScope{}, false
			}
			if kind == Widen {
				scope, valid = operation.PrepareRuntimeWidening(ordered[begin:end])
			} else {
				scope, valid = operation.PrepareRuntimeNarrowing(ordered[begin:end])
			}
		} else if kind == Widen {
			scope, valid = composition.operations[int(slot)].PrepareWidening(ordered[begin:end])
		} else {
			scope, valid = composition.operations[int(slot)].PrepareNarrowing(ordered[begin:end])
		}
		if !valid || scope == 0 {
			return MergeScope{}, false
		}
		members[int(slot)], scopes[int(slot)] = true, scope
		begin = end
	}
	return MergeScope{composition: composition, members: members, scopes: scopes, kind: kind}, true
}

func (selection MergeScope) validFor(composition *Composition, kind MergeKind) bool {
	if selection.composition != composition || len(selection.members) != composition.Count() || len(selection.scopes) != composition.Count() || selection.kind != kind {
		return false
	}
	return kind != Join || selection.all
}

// State is one immutable carrier.  support is its sole feasibility region;
// roots is the one flat vector of opaque persistent typed-plane identities.
type State struct {
	authority *stateAuthority
	scope     Scope
	support   support.Mask
	roots     []RootHandle
}

// NewState creates the canonical all-initial State for one explicit issued
// coordinate scope. No plane is copied or attached one at a time. A caller
// must name the structural Point scope rather than inheriting Composition's
// complete atom universe implicitly.
func NewState(composition *Composition, scope Scope, feasible support.Mask) (State, bool) {
	if composition == nil || composition.shape == nil || composition.guards == nil || !scope.validFor(composition) || !feasible.Valid() || feasible.Manager() != composition.guards || len(composition.operations) != composition.shape.Count() {
		return State{}, false
	}
	// A copied Composition intentionally remains a distinct structural owner
	// for test/adversarial isolation.  Rehome only its tiny State provenance
	// cell; operations, layout, and root stores stay exactly as sealed.
	if composition.authority == nil || composition.authority.composition != composition {
		composition.authority = &stateAuthority{composition: composition}
	}
	root, rootOK := feasible.Guard()
	if !rootOK || !scope.guard.Contains(root) {
		return State{}, false
	}
	return State{authority: composition.authority, scope: scope, support: feasible, roots: composition.initial}, true
}

// Valid proves State has the sealed composition's single shared support and
// one structurally valid typed root at every canonical slot.
func (state State) Valid() bool {
	if !state.live() || state.previewMarked() || state.contributionMarked() {
		return false
	}
	for index, operation := range state.authority.composition.operations {
		if !operation.ValidRoot(state.roots[index]) {
			return false
		}
	}
	return true
}

// Same reports whether two opaque State values are the same immutable carrier
// snapshot. It reveals no typed plane/root data. Rule-admission derivations
// use it to let a checker relate only the exact sealed input snapshots it was
// issued.
func (state State) Same(other State) bool { return sameState(state, other) }

func (state State) live() bool {
	return state.authority.live() && state.scope.validFor(state.authority.composition) && state.support.Valid() && state.support.Manager() == state.authority.composition.guards && len(state.roots) == len(state.authority.composition.operations) && (!state.previewMarked() || state.previewOwner() != nil && state.previewOwner().live) && (!state.contributionMarked() || state.contributionOwner() != nil && state.contributionOwner().live)
}

// Support returns State's sole shared feasibility region.
func (state State) Support() support.Mask { return state.support }

// Scope returns State's exact finite guard-coordinate interface.
func (state State) Scope() Scope {
	if !state.live() {
		return Scope{}
	}
	return state.scope
}

// HandleAt returns an opaque root handle at one canonical slot.  Carrying this
// handle does not reveal a typed Plane; only the owning Binding can resolve it.
func (state State) HandleAt(slot shape.Slot) (RootHandle, bool) {
	if !state.live() || !state.authority.composition.shape.ValidSlot(slot) {
		return RootHandle{}, false
	}
	root := state.roots[int(slot)]
	if !state.acceptsRoot(slot, root) {
		return RootHandle{}, false
	}
	return root, true
}

// acceptsRoot is carrier's one root-admission law.  A retained root is
// accepted only by its typed slot operation.  A transient root has no Factor
// mode: it must be tagged, belong to this State's exact live Preview, and be
// re-proved by the publisher which created its Binding-local plane.
func (state State) acceptsRoot(slot shape.Slot, root RootHandle) bool {
	if !state.live() || !state.authority.composition.shape.ValidSlot(slot) {
		return false
	}
	if !isPreviewRoot(root) {
		return state.authority.composition.operations[int(slot)].ValidRoot(root)
	}
	owner := state.previewOwner()
	return owner != nil && owner.owns(slot, root)
}

// View is a support-only row view of one committed State.  It shares immutable
// root handles but cannot stage a Patch or Commit, so a restricted row can
// never masquerade as a new publication predecessor.
type View struct {
	state   State
	support support.Mask
}

// Restrict creates a support-only View.  Typed plane roots neither retain a
// second support nor get rebuilt here.
func (state State) Restrict(within support.Mask) (View, bool) {
	if !validViewInput(state, within) {
		return View{}, false
	}
	// State construction and publication already proved state.support belongs
	// to state.scope. A same-handle request therefore needs no second cold
	// scope traversal (which otherwise allocates a read-only BDD walk).
	if state.support.SameHandle(within) {
		return View{state: state, support: state.support}, true
	}
	if !withinScope(state, within) {
		return View{}, false
	}
	restricted, ok := restrictViewSupport(state.support, within)
	if !ok {
		return View{}, false
	}
	return View{state: state, support: restricted}, true
}

func validViewInput(state State, within support.Mask) bool {
	if !state.live() || !within.Valid() || within.Manager() != state.authority.composition.guards {
		return false
	}
	return true
}

func withinScope(state State, within support.Mask) bool {
	root, ok := within.Guard()
	return ok && state.scope.guard.Contains(root)
}

func restrictViewSupport(left, right support.Mask) (support.Mask, bool) {
	return intersectSupport(left, right)
}

func intersectSupport(left, right support.Mask) (support.Mask, bool) {
	return support.Intersect(left, right)
}

// Support returns the exact row support for this non-publishable view.
func (view View) Support() support.Mask { return view.support }

// HandleAt carries an opaque root for a selected operation into a typed read
// operation.  The view itself cannot be committed or used as State identity.
func (view View) HandleAt(slot shape.Slot) (RootHandle, bool) {
	if !view.state.live() || !view.support.Valid() || view.support.Manager() != view.state.support.Manager() {
		return RootHandle{}, false
	}
	return view.state.HandleAt(slot)
}

func (work *Work) liveFor(left, right State) bool {
	return work != nil && work.OwnsState(left) && work.OwnsState(right) && left.scope.same(right.scope)
}

// EqualUnder compares canonical support first, then dispatches through this
// evaluator's fixed typed work vector. It never constructs Product rows or
// restricts a full plane merely to test equality.
func (work *Work) EqualUnder(left, right State) bool {
	if !work.live() || !work.liveFor(left, right) || left.contributionMarked() || right.contributionMarked() || !left.support.Equal(right.support) {
		return false
	}
	for index, slot := range work.slots {
		if !work.live() || slot == nil || !sameRoot(left.roots[index], right.roots[index]) && !slot.EqualUnder(left.roots[index], right.roots[index], left.support) {
			return false
		}
	}
	return true
}

// LessOrEqUnder compares only the left support after the outer inclusion
// proof, then invokes this Work's typed operation order checks without Product.
func (work *Work) LessOrEqUnder(left, right State) bool {
	if !work.live() || !work.liveFor(left, right) || left.contributionMarked() || right.contributionMarked() || !left.support.Entails(right.support) {
		return false
	}
	for index, slot := range work.slots {
		if !work.live() || slot == nil || !sameRoot(left.roots[index], right.roots[index]) && !slot.LessOrEqUnder(left.roots[index], right.roots[index], left.support) {
			return false
		}
	}
	return true
}

// LessOrEqContribution proves the lifted order of two closed contribution
// artifacts.  State support inclusion remains the outer feasibility proof;
// authored coverage and sparse non-Default presence are checked by the typed
// SlotWork, where Target rows can be expanded without leaking their keys into
// carrier.  Raw State order deliberately remains separate because it has
// different hidden-root semantics.
func (work *Work) LessOrEqContribution(left, right Contribution) bool {
	return work != nil && work.admittedContribution(left) && work.admittedContribution(right) && work.lessOrEqContributionSurface(left.state, left.coverage, right.state, right.coverage)
}

// Merge3Under computes one exact three-region split and hands that same split
// plus one shared candidate delta work item to every typed operation.
// Join retains union support. Widen requires left support to be a subset of
// right and retains right support exactly; Narrow also retains right support
// after its required subset proof. It returns the State and its exact
// ChangeSet together.
func (work *Work) Merge3Under(kind MergeKind, left, right State, selected MergeScope) (State, ChangeSet, bool) {
	if !selected.validFor(work.composition, kind) {
		return State{}, ChangeSet{}, false
	}
	return work.merge3Under(kind, left, right, selected.members, selected.scopes)
}

// MergeContribution joins one complete producer artifact into an accumulated
// Point RHS. Dynamic authored coverage is the only presence authority:
// uncovered regions are fold identity, while covered sparse zero is the
// Factor's explicit typed Default. The returned coverage union is published
// atomically with the semantic State; ChangeSet remains semantic-only.
func (work *Work) MergeContribution(left, right Contribution) (Contribution, ChangeSet, bool) {
	if !work.live() || !work.admittedContribution(left) || !work.admittedContribution(right) || !work.liveFor(left.state, right.state) || left.state.previewMarked() || right.state.previewMarked() || left.state.contributionMarked() || right.state.contributionMarked() {
		return Contribution{}, ChangeSet{}, false
	}
	// A carrier-issued neutral contribution is the exact empty Point identity:
	// no authored coverage and no non-initial root exists anywhere in its
	// semantic State. Work/scope admission above is intentionally first, so a
	// neutral from another evaluator or scope can never bypass ownership.
	if work.neutralContribution(left) {
		empty, ok := emptyChangeSet(work.composition)
		return right, empty, ok
	}
	if work.neutralContribution(right) {
		empty, ok := emptyChangeSet(work.composition)
		return left, empty, ok
	}
	if sameState(left.state, right.state) && sameContributionCoverage(left.coverage, right.coverage) {
		empty, ok := emptyChangeSet(work.composition)
		return left, empty, ok
	}
	split, ok := work.threeSupport(left.state.support, right.state.support)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	nextCoverage, ok := work.unionCoverage(left.coverage, right.coverage)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	// A contribution slot can be an exact carrier-level identity even when its
	// enclosing States differ.  Coverage is the authored-presence authority:
	// an uncovered right slot is fold identity, while equal immutable roots are
	// stable under Join.  The coverage union is still retained above for
	// coverage-only wake semantics, and the support union remains visible in the
	// carrier ChangeSet below.
	fastSlots := 0
	for position := range work.slots {
		if work.contributionSlotIdentity(position, left, right) {
			fastSlots++
		}
	}
	if fastSlots == len(work.slots) {
		empty := emptyMask(work.composition.guards)
		if !empty.Valid() {
			return Contribution{}, ChangeSet{}, false
		}
		next, changes, committed := work.commit(left.state, nil, split.Union(), split.RightOnly(), empty, nil)
		if !committed {
			return Contribution{}, ChangeSet{}, false
		}
		result, valid := work.admitConstructedContribution(next, nextCoverage)
		return result, changes, valid
	}
	delta := work.newSupportWork()
	if delta == nil {
		return Contribution{}, ChangeSet{}, false
	}
	patches := make([]Patch, 0, len(work.slots)-fastSlots)
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, ChangeSet{}, false
		}
		physical := shape.Slot(position)
		leftSlot, rightSlot := left.coverage.slot(physical), right.coverage.slot(physical)
		if work.contributionSlotIdentity(position, left, right) {
			continue
		}
		change, okay := slot.MergeContributionUnder(left.state.roots[position], right.state.roots[position], left.state.support, right.state.support, coverageRows(leftSlot), coverageRows(rightSlot), delta)
		if !okay {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, ChangeSet{}, false
		}
		if !work.acceptInto(&patches, left.state, change, delta) {
			delta.Discard()
			return Contribution{}, ChangeSet{}, false
		}
	}
	next, changes, ok := work.commit(left.state, patches, split.Union(), split.RightOnly(), emptyMask(work.composition.guards), delta)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	result, valid := work.admitConstructedContribution(next, nextCoverage)
	return result, changes, valid
}

// FoldRHSContribution joins one operand while an equation RHS is still being
// assembled and no intermediate semantic delta is observable. A closed
// support-only base has no authored cells in any Factor, so the lifted product
// law is exactly (S0, Abs) join (S1, R) = (S0 union S1, R). In that case the
// immutable right roots and coverage are adopted without a typed merge or root
// publication; the final Point replacement derives the one externally visible
// ChangeSet. Every other shape uses the ordinary exact contribution join.
func (work *Work) FoldRHSContribution(left, right Contribution) (Contribution, bool) {
	if !work.live() || !work.admittedContribution(left) || !work.admittedContribution(right) || !work.liveFor(left.state, right.state) {
		return Contribution{}, false
	}
	if len(left.coverage.slots) == 0 && sameRootVector(left.state.roots, work.composition.initial) {
		union, ok := work.unionSupport(left.state.support, right.state.support)
		if !ok {
			return Contribution{}, false
		}
		state := right.state
		state.support = union
		return work.admitConstructedContribution(state, right.coverage)
	}
	result, _, ok := work.MergeContribution(left, right)
	return result, ok
}

// contributionSlotIdentity is the closed-slot theorem used by
// MergeContribution.  Empty compact coverage is exactly Absent, never a
// sparse-root inference; the issuance cut already removed every physical
// payload outside coverage.
func (work *Work) contributionSlotIdentity(position int, left, right Contribution) bool {
	if work == nil || work.composition == nil || position < 0 || position >= len(work.slots) || work.slots[position] == nil || position >= len(work.composition.initial) || position >= len(left.state.roots) || position >= len(right.state.roots) {
		return false
	}
	leftRoot, rightRoot := left.state.roots[position], right.state.roots[position]
	physical := shape.Slot(position)
	rightSlot := right.coverage.slot(physical)
	return len(rightSlot.targets) == 0 || sameRoot(leftRoot, rightRoot)
}

func (work *Work) merge3Under(kind MergeKind, left, right State, members []bool, scopes []uint64) (State, ChangeSet, bool) {
	if !work.live() || !work.liveFor(left, right) || left.previewMarked() || right.previewMarked() || left.contributionMarked() || right.contributionMarked() || len(members) != work.composition.Count() || len(scopes) != work.composition.Count() {
		return State{}, ChangeSet{}, false
	}
	if kind != Join && kind != Widen && kind != Narrow {
		return State{}, ChangeSet{}, false
	}
	if kind == Widen && !left.support.Entails(right.support) {
		// Widen is monotone in carrier support.  A support-shrinking update is
		// structural replacement, not a recurrence; callers must use Replace.
		return State{}, ChangeSet{}, false
	}
	if kind == Narrow && !right.support.Entails(left.support) {
		return State{}, ChangeSet{}, false
	}
	if kind == Join && sameState(left, right) {
		empty, ok := emptyChangeSet(left.authority.composition)
		return left, empty, ok
	}
	split, ok := work.threeSupport(left.support, right.support)
	if !ok {
		return State{}, ChangeSet{}, false
	}
	nextSupport := split.Union()
	added, removed := split.RightOnly(), emptyMask(left.authority.composition.guards)
	if kind == Widen {
		nextSupport = right.support
	}
	if kind == Narrow {
		nextSupport = right.support
		added, removed = emptyMask(left.authority.composition.guards), split.LeftOnly()
	}
	if !added.Valid() || !removed.Valid() {
		return State{}, ChangeSet{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return State{}, ChangeSet{}, false
	}
	patches := make([]Patch, 0, len(left.authority.composition.operations))
	for position, operation := range left.authority.composition.operations {
		if !work.live() || work.slots[position] == nil {
			delta.Discard()
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
		if kind == Join && !members[position] {
			continue
		}
		recurrence := kind == Join || members[position]
		scope := scopes[position]
		if (kind == Widen || kind == Narrow) && recurrence && scope == 0 {
			return State{}, ChangeSet{}, false
		}
		change, okay := work.slots[position].Merge3Under(kind, recurrence, scope, left.roots[position], right.roots[position], split, delta)
		if !okay {
			delta.Discard()
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
		if !work.acceptInto(&patches, left, change, delta) {
			delta.Discard()
			return State{}, ChangeSet{}, false
		}
		_ = operation
	}
	return work.commit(left, patches, nextSupport, added, removed, delta)
}

// MergeSelectedUnder computes one atomic recurrence update from three States.
// Selected slots apply kind to current and selectedRight; every other slot
// installs exactRight structurally. The published support is always exactly
// exactRight's support, so a restart may retract stale carrier support without
// letting an unselected coordinate retain current or selected-right meaning.
//
// Widen requires selectedRight to exactly equal exactRight at the atomic
// boundary; when a slot is selected, current support must additionally be
// contained by that right support. Narrow requires
// selectedRight and exactRight to name the same output support, while
// selectedRight remains contained by current. An empty scope references no
// selected operand and is therefore an exact support-reset transition.
func (work *Work) MergeSelectedUnder(kind MergeKind, current, selectedRight, exactRight State, selected MergeScope) (State, ChangeSet, bool) {
	if !work.live() || !work.liveFor(current, selectedRight) || !work.liveFor(current, exactRight) || current.previewMarked() || selectedRight.previewMarked() || exactRight.previewMarked() || current.contributionMarked() || selectedRight.contributionMarked() || exactRight.contributionMarked() || !selected.validFor(work.composition, kind) {
		return State{}, ChangeSet{}, false
	}
	if kind != Widen && kind != Narrow {
		return State{}, ChangeSet{}, false
	}
	hasSelected := false
	for _, member := range selected.members {
		hasSelected = hasSelected || member
	}
	if kind == Widen && !selectedRight.support.Equal(exactRight.support) {
		return State{}, ChangeSet{}, false
	}
	if hasSelected && kind == Widen && !current.support.Entails(selectedRight.support) {
		return State{}, ChangeSet{}, false
	}
	if hasSelected && kind == Narrow && (!selectedRight.support.Entails(current.support) || !selectedRight.support.Equal(exactRight.support)) {
		return State{}, ChangeSet{}, false
	}
	// Narrow publishes a mixed state whose unselected slots are copied from
	// exactRight.  Checking only selectedRight's support is therefore not
	// enough: an unselected Factor could grow in exactRight and make the
	// allegedly descending transition globally incomparable.  The complete
	// exact-right plane must be below current before any slot prepares a
	// candidate.  LessOrEqUnder is the carrier's sole typed order authority;
	// it checks every Factor on exactRight's support.
	if kind == Narrow && !work.LessOrEqUnder(exactRight, current) {
		return State{}, ChangeSet{}, false
	}
	selectedSplit, ok := work.threeSupport(current.support, selectedRight.support)
	if !ok {
		return State{}, ChangeSet{}, false
	}
	exactSplit, ok := work.threeSupport(current.support, exactRight.support)
	if !ok {
		return State{}, ChangeSet{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return State{}, ChangeSet{}, false
	}
	patches := make([]Patch, 0, len(current.authority.composition.operations))
	for position, operation := range current.authority.composition.operations {
		if !work.live() || work.slots[position] == nil {
			delta.Discard()
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
		var change ChangeHandle
		var okay bool
		if selected.members[position] {
			scope := selected.scopes[position]
			if scope == 0 {
				delta.Discard()
				dropPatches(patches)
				return State{}, ChangeSet{}, false
			}
			change, okay = work.slots[position].MergeSelectedUnder(kind, scope, current.roots[position], selectedRight.roots[position], exactRight.roots[position], selectedSplit, exactSplit, delta)
		} else {
			change, okay = work.slots[position].ReplaceUnder(current.roots[position], exactRight.roots[position], exactSplit, delta)
		}
		if !okay {
			delta.Discard()
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
		if !work.acceptInto(&patches, current, change, delta) {
			delta.Discard()
			return State{}, ChangeSet{}, false
		}
		_ = operation
	}
	return work.commit(current, patches, exactRight.support, exactSplit.RightOnly(), exactSplit.LeftOnly(), delta)
}

// Reindex atomically transports one State across a sealed guard-coordinate
// boundary. It is input transport, not a Point publication or demand event,
// so it returns no ChangeSet. The target support is transformed and sealed
// before any Factor is asked to transform; every prepared root is dropped if
// any later slot rejects, leaving the source State untouched.
func (work *Work) Reindex(state State, plan ReindexPlan) (State, bool) {
	if !work.live() || work.reindexing || state.previewMarked() || state.contributionMarked() || !work.OwnsState(state) || !plan.validFor(work.composition) || !state.scope.same(plan.source()) {
		return State{}, false
	}
	work.reindexing = true
	defer func() { work.reindexing = false }()
	if plan.identity() {
		return state, true
	}
	targetSupport, ok := work.reindexSupport(state.support, plan.relation)
	if !ok {
		return State{}, false
	}
	if root, valid := targetSupport.Guard(); !valid || !plan.target().guard.Contains(root) {
		return State{}, false
	}
	empty := emptyMask(work.composition.guards)
	if !empty.Valid() {
		return State{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return State{}, false
	}
	patches := make([]Patch, 0, len(work.slots))
	for index, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return State{}, false
		}
		change, valid := slot.ReindexUnder(state.roots[index], state.support, targetSupport, plan.relation, delta)
		if !valid {
			delta.Discard()
			dropPatches(patches)
			return State{}, false
		}
		if !work.acceptInto(&patches, state, change, delta) {
			delta.Discard()
			return State{}, false
		}
	}
	next, _, committed := work.commit(state, patches, targetSupport, empty, empty, delta)
	if !committed {
		return State{}, false
	}
	// commit owns the one root publication cut. Only after that all-slot cut
	// succeeds may the returned immutable State receive its target interface.
	next.scope = plan.target()
	return next, next.live()
}

func emptyMask(manager *guard.Manager) support.Mask {
	mask, _ := support.FromGuard(manager, manager.False())
	return mask
}

func emptyChangeSet(composition *Composition) (ChangeSet, bool) {
	if composition == nil || composition.guards == nil {
		return ChangeSet{}, false
	}
	empty := emptyMask(composition.guards)
	if !empty.Valid() {
		return ChangeSet{}, false
	}
	return ChangeSet{composition: composition, added: empty, removed: empty}, true
}

// Patch is one validated, single-output operation root replacement from a common
// predecessor.  Its private fields prevent rule identity or a typed root from
// being retargeted after it is staged.
type Patch struct {
	work     *Work
	state    State
	slot     shape.Slot
	change   ChangeHandle
	authored []TargetRegion
}

// Accept validates a Binding-produced prepared proof for this Work's exact
// immutable predecessor. Work is deliberately the only public publication
// authority: State itself cannot attach roots.
func (work *Work) Accept(state State, change ChangeHandle) (Patch, bool) {
	if !work.live() {
		return Patch{}, false
	}
	return work.accept(state, change, nil)
}

// AcceptAuthored is the contribution-only publication admission. The typed
// Binding supplies exact Target x nonempty Guard rows independently of the
// semantic ChangeHandle, so an explicit Default remains authored even when
// sparse canonicalization produces no KeyChanges.
func (work *Work) AcceptAuthored(state State, change ChangeHandle, targets []Target, regions []support.Mask) (Patch, bool) {
	if len(targets) == 0 || len(targets) != len(regions) {
		return Patch{}, false
	}
	patch, ok := work.Accept(state, change)
	if !ok {
		return Patch{}, false
	}
	rows := make([]TargetRegion, len(targets))
	for index, target := range targets {
		slot, slotOK := target.Slot()
		region := regions[index]
		if !slotOK || slot != patch.slot || !work.composition.OwnsTarget(slot, target) || !region.Valid() || region.Manager() != work.composition.guards || support.Empty(region) || !region.Entails(state.support) {
			work.Discard(patch)
			return Patch{}, false
		}
		rows[index] = TargetRegion{target: target, region: region}
	}
	patch.authored = rows
	return patch, true
}

// Discard consumes one accepted-but-uncommitted Patch.  Evaluators use this
// when a Rule callback, cancellation, or scheduler admission failure drops a
// staged output before Commit; it releases a pending typed root publisher
// without exposing the Patch internals or publishing State.
func (work *Work) Discard(patch Patch) bool {
	if work == nil || patch.work != work || patch.change.record == nil || patch.change.record.consumed {
		return false
	}
	return discardChange(patch.change)
}

// DiscardChange consumes one caller-retained ChangeHandle that could not be
// admitted as a Patch. It is the public failure counterpart to Accept: only a
// Work over the handle's exact attached Composition may drop its pending
// publisher, so passing a foreign handle never transfers ownership by error.
func (work *Work) DiscardChange(change ChangeHandle) bool {
	if work == nil || work.composition == nil || change.issuer == nil || change.record == nil || change.record.consumed {
		return false
	}
	if _, owned := slotInLayout(change.issuer, work.composition.layout); !owned {
		return false
	}
	return discardChange(change)
}

func discardChange(change ChangeHandle) bool {
	if change.record == nil || change.record.consumed {
		return false
	}
	change.record.consumed = true
	if change.record.publisher != nil {
		change.record.publisher.Drop()
	}
	return true
}

func (work *Work) accept(state State, change ChangeHandle, candidate *support.Work) (Patch, bool) {
	if work == nil || work.composition == nil || state.authority == nil || work.composition != state.authority.composition {
		return Patch{}, false
	}
	return preparePatch(work, state, change, candidate)
}

// acceptInto is the sole multi-slot acceptance transaction boundary. On
// rejection it disposes the current operation-produced handle and every
// earlier accepted Patch; no caller can accidentally lose the final pending
// publisher while handling a later slot failure.
func (work *Work) acceptInto(patches *[]Patch, state State, change ChangeHandle, candidate *support.Work) bool {
	if patches == nil {
		_ = work.DiscardChange(change)
		return false
	}
	patch, ok := work.accept(state, change, candidate)
	if !ok {
		_ = work.DiscardChange(change)
		dropPatches(*patches)
		return false
	}
	*patches = append(*patches, patch)
	return true
}

// slotInLayout resolves a live issuer only after its shared layout latch is
// open.  State publication never reads issuer attachment fields directly.
func slotInLayout(issued *issuer, layout *layout) (shape.Slot, bool) {
	owner := issuerOwner(issued, true)
	if owner == nil || owner.layout != layout {
		return 0, false
	}
	return owner.slot, true
}

func issuerAt(issued *issuer, layout *layout, slot shape.Slot) bool {
	owned, ok := slotInLayout(issued, layout)
	return ok && owned == slot
}

func preparePatch(work *Work, state State, change ChangeHandle, candidate *support.Work) (Patch, bool) {
	if !state.live() || change.issuer == nil || change.record == nil || change.record.consumed {
		return Patch{}, false
	}
	slot, owned := slotInLayout(change.issuer, state.authority.composition.layout)
	if !owned {
		return Patch{}, false
	}
	if !state.authority.composition.shape.ValidSlot(slot) {
		return Patch{}, false
	}
	before := state.roots[int(slot)]
	record := change.record
	presentFactor, factorOK := optionalNonemptyRegion(record.factor, candidate)
	if !sameRoot(before, record.before) || len(record.units) != len(record.regions) || !factorOK || record.publisher != nil && !record.publisher.Ready() || record.publisher == nil && !state.acceptsRoot(slot, record.after) || record.publisher == nil && sameRoot(record.before, record.after) && (presentFactor || len(record.units) != 0) || len(record.units) != 0 && !presentFactor {
		return Patch{}, false
	}
	if presentFactor && !candidateEntails(candidate, record.factor, state.support) {
		return Patch{}, false
	}
	for index, unit := range record.units {
		validRegion := record.regions[index].Valid() || candidate != nil && candidate.Valid(record.regions[index])
		if !state.authority.composition.OwnsUnit(slot, unit) || !validRegion || !regionEntails(candidate, record.regions[index], record.factor) || index > 0 && !record.units[index-1].Less(unit) {
			return Patch{}, false
		}
	}
	return Patch{work: work, state: state, slot: slot, change: change}, true
}

// UnitRegion is one canonical changed dependency unit and its exact Guard
// region. Its fields are immutable and can only originate in a typed Binding
// operation.
type UnitRegion struct {
	unit   Unit
	region support.Mask
}

func (row UnitRegion) Unit() Unit           { return row.unit }
func (row UnitRegion) Region() support.Mask { return row.region }

// FactorRegion is one exact slot-level semantic root difference and its
// nonempty support region. The slot comes only from the attached issuer that
// carrier already proved owns the accepted Patch; no Factor or caller can
// retarget a delta to another physical coordinate.
type FactorRegion struct {
	slot   shape.Slot
	region support.Mask
}

func (row FactorRegion) Slot() shape.Slot     { return row.slot }
func (row FactorRegion) Region() support.Mask { return row.region }

// ChangeSet is the exact semantic result consumed by reverse invalidation.
// Direct writes preserve outer support, so Added and Removed are the sealed
// empty region; merge-produced support changes join this same type later.
type ChangeSet struct {
	composition *Composition
	added       support.Mask
	removed     support.Mask
	factors     []FactorRegion
	rows        []UnitRegion
}

func (set ChangeSet) Added() support.Mask   { return set.added }
func (set ChangeSet) Removed() support.Mask { return set.removed }
func (set ChangeSet) Count() int            { return len(set.rows) }
func (set ChangeSet) FactorCount() int      { return len(set.factors) }
func (set ChangeSet) Empty() bool {
	return set.composition != nil && support.Empty(set.added) && support.Empty(set.removed) && len(set.factors) == 0 && len(set.rows) == 0
}
func (set ChangeSet) At(index int) (UnitRegion, bool) {
	if set.composition == nil || index < 0 || index >= len(set.rows) {
		return UnitRegion{}, false
	}
	return set.rows[index], true
}

func (set ChangeSet) FactorAt(index int) (FactorRegion, bool) {
	if set.composition == nil || index < 0 || index >= len(set.factors) {
		return FactorRegion{}, false
	}
	return set.factors[index], true
}

// OwnsChangeSet proves that set was published by this exact sealed carrier
// composition.  ChangeSet intentionally exposes neither its issuer nor a
// mutable composition handle; pointer identity is the one provenance cut for
// reverse invalidation.  In particular, equal dense slot layouts are not
// interchangeable owners.
func (composition *Composition) OwnsChangeSet(set ChangeSet) bool {
	return composition != nil && set.composition == composition
}

// Commit accepts only distinct-operation prepared patches derived from this
// Work's exact common predecessor. It preflights every typed successor before
// any root-store publication, then publishes canonically in one infallible
// cut.
func (work *Work) Commit(state State, patches []Patch) (State, ChangeSet, bool) {
	if !work.live() || work.composition == nil || state.authority == nil || work.composition != state.authority.composition || !state.live() || state.previewMarked() || state.contributionMarked() {
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	empty, ok := support.FromGuard(work.composition.guards, work.composition.guards.False())
	if !ok {
		return State{}, ChangeSet{}, false
	}
	return work.commit(state, patches, state.support, empty, empty, nil)
}

// Transfer applies already accepted, distinct-Factor patches from one exact
// committed predecessor to one exact support-only predecessor view.  It is the
// carrier-private structural execution cut: Rules return Patch values but do
// not publish a successor State.  The view binds both the immutable root-vector
// provenance and the restricted support, so an equal-looking view of another
// State or Composition cannot be substituted here.
//
// A transfer retains only the view's support.  Consequently it reports the
// exact predecessor region excluded by that view as Removed, including when
// every Factor carries its root or the view is empty.  The ordinary commit
// proof below also requires every changed UnitRegion to be contained by that
// restricted support before any pending root can reserve or publish.
func (work *Work) Transfer(predecessor State, restricted View, patches []Patch) (State, ChangeSet, bool) {
	if !work.live() || predecessor.previewMarked() || predecessor.contributionMarked() || !work.OwnsViewOf(predecessor, restricted) {
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	split, ok := work.threeSupport(predecessor.support, restricted.support)
	if !ok {
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	empty := emptyMask(work.composition.guards)
	removed := split.LeftOnly()
	if !empty.Valid() || !removed.Valid() {
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	return work.commit(predecessor, patches, restricted.support, empty, removed, nil)
}

// Replace atomically installs recomputed as the exact structural successor of
// old. This is deliberately neither Join, Widen, Narrow, nor a Rule patch:
// support and every opaque Factor root come from recomputed, including roots
// whose meaning was previously outside old support. SlotWork reports only
// old-to-right unit differences on the overlap; support growth and shrink
// remain carrier-level Added and Removed evidence.
//
// Neither input is mutated. An identical immutable predecessor is the
// allocation-free no-op, but semantically equal distinct right roots still
// replace old roots so a later support growth cannot reveal hidden old meaning.
func (work *Work) Replace(old, recomputed State) (State, ChangeSet, bool) {
	if !work.live() || work.replacing || old.previewMarked() || recomputed.previewMarked() || old.contributionMarked() || recomputed.contributionMarked() || !work.liveFor(old, recomputed) {
		return State{}, ChangeSet{}, false
	}
	work.replacing = true
	defer func() { work.replacing = false }()
	if sameState(old, recomputed) {
		empty, ok := emptyChangeSet(old.authority.composition)
		return old, empty, ok
	}
	split, ok := work.threeSupport(old.support, recomputed.support)
	if !ok {
		return State{}, ChangeSet{}, false
	}
	// Support is the first carrier coordinate. When every immutable plane root
	// is already identical, structural growth/retraction has no typed delta to
	// derive and no root vector to copy; publish its support ChangeSet directly
	// through the sole commit cut.
	if sameRootVector(old.roots, recomputed.roots) {
		return work.commit(old, nil, recomputed.support, split.RightOnly(), split.LeftOnly(), nil)
	}
	delta := work.newSupportWork()
	if delta == nil {
		return State{}, ChangeSet{}, false
	}
	patches := make([]Patch, 0, len(work.slots))
	for index, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
		change, okay := slot.ReplaceUnder(old.roots[index], recomputed.roots[index], split, delta)
		if !okay {
			delta.Discard()
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
		if !work.acceptInto(&patches, old, change, delta) {
			delta.Discard()
			return State{}, ChangeSet{}, false
		}
	}
	return work.commit(old, patches, recomputed.support, split.RightOnly(), split.LeftOnly(), delta)
}

// commit is carrier's one structural publication cut. Direct batches and
// Merge3 both enter here; neither owns a second root-attachment route.
func (work *Work) commit(state State, patches []Patch, nextSupport, added, removed support.Mask, candidate *support.Work) (State, ChangeSet, bool) {
	if !work.live() || work.composition == nil || work.composition.layout == nil {
		dropPatches(patches)
		discardCandidate(candidate)
		return State{}, ChangeSet{}, false
	}
	composition := work.composition
	composition.layout.publish.Lock()
	defer composition.layout.publish.Unlock()
	if !work.live() || state.authority == nil || composition != state.authority.composition || work.publishing || !state.live() {
		dropPatches(patches)
		discardCandidate(candidate)
		return State{}, ChangeSet{}, false
	}
	work.publishing = true
	defer func() { work.publishing = false }()
	committed := false
	defer func() {
		if !committed {
			discardCandidate(candidate)
		}
	}()
	prepared, valid := work.prepareCommit(state, patches, nextSupport, added, removed, candidate)
	if !valid {
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	if !prepared.changed {
		discardCandidate(candidate)
		dropPatches(patches)
		committed = true
		return state, prepared.set, true
	}
	var next []RootHandle
	if prepared.rootsChanged {
		next = append([]RootHandle(nil), state.roots...)
	} else {
		next = state.roots
	}
	// Every RootPublisher reservation is still unpublishable. Poll before and
	// after each reservation, then once immediately before the irreversible
	// root cut. A false result drops every reservation through dropPatches and
	// leaves the predecessor/root store unchanged.
	for _, patch := range patches {
		if !work.live() {
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
		if publisher := patch.change.record.publisher; publisher != nil && !publisher.Reserve() {
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
		if !work.live() {
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
	}
	if candidate != nil && !candidate.Seal() {
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	if !work.live() {
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	// No checkpoint follows: Publish is the indivisible final cut. Once it
	// starts every earlier reservation is consumed in canonical slot order.
	for _, patch := range patches {
		record := patch.change.record
		if publisher := record.publisher; publisher != nil {
			root := publisher.Publish()
			if !state.authority.composition.operations[int(patch.slot)].ValidRoot(root) {
				panic("prepared root publication violated carrier invariant")
			}
			next[int(patch.slot)] = root
		} else if prepared.rootsChanged {
			next[int(patch.slot)] = record.after
		}
	}
	dropPatches(patches)
	committed = true
	return State{authority: work.authority, scope: state.scope, support: nextSupport, roots: next}, prepared.set, true
}

// preparedCommit is the common semantic admission result for the ordinary
// publication cut and the non-publishing Preview cut.  It validates the same
// predecessor binding, typed roots, dependency regions, batch order, and
// support containment before either path decides what to do with a prepared
// Factor root.
type preparedCommit struct {
	set          ChangeSet
	rootsChanged bool
	changed      bool
}

func (work *Work) prepareCommit(state State, patches []Patch, nextSupport, added, removed support.Mask, candidate *support.Work) (preparedCommit, bool) {
	if work == nil || work.composition == nil || state.authority == nil || !state.live() || state.authority.composition != work.composition || !nextSupport.Valid() || !added.Valid() || !removed.Valid() || nextSupport.Manager() != state.authority.composition.guards || added.Manager() != state.authority.composition.guards || removed.Manager() != state.authority.composition.guards {
		return preparedCommit{}, false
	}
	changed, rowCount, factorCount := false, 0, 0
	for index, patch := range patches {
		if !work.live() {
			return preparedCommit{}, false
		}
		record := patch.change.record
		presentFactor, factorOK := patch.change.factorRegion(candidate)
		if patch.work != work || !sameState(patch.state, state) || !state.authority.composition.shape.ValidSlot(patch.slot) || patch.change.issuer == nil || record == nil || record.consumed || !factorOK || !issuerAt(patch.change.issuer, state.authority.composition.layout, patch.slot) || !sameRoot(record.before, state.roots[int(patch.slot)]) || record.publisher != nil && !record.publisher.Ready() || record.publisher == nil && !state.acceptsRoot(patch.slot, record.after) || record.publisher == nil && sameRoot(record.before, record.after) && (presentFactor || len(record.units) != 0) || len(record.units) != 0 && !presentFactor || presentFactor && (!candidateEntails(candidate, record.factor, state.support) || !candidateEntails(candidate, record.factor, nextSupport)) || index > 0 && patches[index-1].slot >= patch.slot {
			return preparedCommit{}, false
		}
		for row, unit := range record.units {
			if !work.live() {
				return preparedCommit{}, false
			}
			region := record.regions[row]
			validRegion := region.Valid() || candidate != nil && candidate.Valid(region)
			if !state.authority.composition.OwnsUnit(patch.slot, unit) || !validRegion || !regionEntails(candidate, region, record.factor) || row > 0 && !record.units[row-1].Less(unit) || !candidateEntails(candidate, region, state.support) || !candidateEntails(candidate, region, nextSupport) {
				return preparedCommit{}, false
			}
		}
		changed = changed || record.publisher != nil
		rowCount += len(record.units)
		if presentFactor {
			factorCount++
		}
	}
	set := ChangeSet{composition: state.authority.composition, added: added, removed: removed}
	if factorCount != 0 {
		set.factors = make([]FactorRegion, 0, factorCount)
		for _, patch := range patches {
			if present, _ := patch.change.factorRegion(candidate); present {
				set.factors = append(set.factors, FactorRegion{slot: patch.slot, region: patch.change.record.factor})
			}
		}
	}
	if rowCount != 0 {
		set.rows = make([]UnitRegion, 0, rowCount)
		for _, patch := range patches {
			for index, unit := range patch.change.record.units {
				set.rows = append(set.rows, UnitRegion{unit: unit, region: patch.change.record.regions[index]})
			}
		}
	}
	rootsChanged := false
	for _, patch := range patches {
		record := patch.change.record
		rootsChanged = rootsChanged || record.publisher != nil || !sameRoot(record.before, record.after)
	}
	return preparedCommit{set: set, rootsChanged: rootsChanged, changed: changed || rootsChanged || !state.support.SameHandle(nextSupport)}, true
}

func candidateEntails(candidate *support.Work, region, within support.Mask) bool {
	if candidate != nil && candidate.Valid(region) {
		return candidate.Entails(region, within)
	}
	return region.Valid() && region.Entails(within)
}

func dropPatches(patches []Patch) {
	for _, patch := range patches {
		_ = discardChange(patch.change)
	}
}

func discardCandidate(candidate *support.Work) {
	if candidate != nil && candidate.Open() {
		candidate.Discard()
	}
}

func sameState(left, right State) bool {
	return left.authority == right.authority && left.support.SameHandle(right.support) && sameRootVector(left.roots, right.roots)
}

// sameRootVector is immutable-predecessor provenance, not semantic equality.
// State publication reuses a vector only for an exact no-op and allocates one
// replacement vector for every changed Commit/Merge.  Its backing identity,
// together with the exact composition and BDD support handle, is therefore an
// O(1), allocation-free predecessor proof for Patch.Commit.
func sameRootVector(left, right []RootHandle) bool {
	if len(left) != len(right) {
		return false
	}
	return len(left) == 0 || &left[0] == &right[0]
}

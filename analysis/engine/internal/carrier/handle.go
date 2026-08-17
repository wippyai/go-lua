// Package carrier owns the one heterogeneous hot-state representation.  Its
// payload is a vector of opaque handles, never domain values or erased planes.
package carrier

import (
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
)

// RootHandle identifies one immutable typed plane root.  A normal handle names
// a retained root-store entry.  A preview handle names a Factor-owned,
// transaction-local plane instead: it is valid only while its carrier Preview
// remains live and can never be published by the ordinary State cut.  Neither
// form exposes or constructs a semantic value.
type RootHandle struct {
	issuer *issuer
	id     uint64
	// epoch is nil only for the immutable composition-attached initial root.
	// Every dynamically published root carries the one Work epoch that owns
	// its typed arena.  The token contains no semantic value; it is solely
	// lifecycle/provenance, so a stale root can fail closed without keeping the
	// slot's FDD alive.
	epoch *RootEpoch
}

// EpochRootStore is the minimal opaque membership fence for one typed slot's
// dynamic root arena.  Carrier never receives a Plane or a payload: it asks
// only whether a compact root ID was actually published by this epoch.
//
// A Work owns one RootEpoch.  Its bound slot works register exactly one store
// each before evaluation.  Close clears that registry before slot work drops
// the typed stores, so an escaped handle cannot either resolve or retain an
// old candidate arena.
type EpochRootStore interface {
	EpochRootValid(uint64) bool
}

const (
	rootEpochActive uint32 = iota + 1
	rootEpochRetained
	rootEpochClosed
)

// RootEpoch is the opaque lifetime/provenance token for roots dynamically
// published by one carrier.Work.  It is deliberately not a State authority
// and carries no semantic facts.  Retained is the sole completed-cache state;
// Closed revokes every dynamic root from this epoch.
type RootEpoch struct {
	layout *layout
	state  atomic.Uint32
	mu     sync.RWMutex
	roots  map[*issuer]EpochRootStore
}

func newRootEpoch(layout *layout) *RootEpoch {
	if layout == nil {
		return nil
	}
	epoch := &RootEpoch{layout: layout, roots: make(map[*issuer]EpochRootStore)}
	epoch.state.Store(rootEpochActive)
	return epoch
}

// Active reports whether this token may still drive evaluator work.  A
// retained completed cache keeps roots readable but never reopens execution.
func (epoch *RootEpoch) Active() bool {
	return epoch != nil && epoch.state.Load() == rootEpochActive
}

// Live reports whether dynamically published roots remain resolvable.  It is
// true for the one immutable retained completed cache and false after Close.
func (epoch *RootEpoch) Live() bool {
	return epoch != nil && epoch.state.Load() != rootEpochClosed
}

// OwnsRoot is the allocation-free same-epoch fast path.  The caller already
// owns its typed slot store, so it need not take the epoch registry lock to
// re-find that store on every hot root resolution.  The caller still validates
// the compact ID in its own store before using it.
func (epoch *RootEpoch) OwnsRoot(issuer Issuer, handle RootHandle) (uint64, bool) {
	if epoch == nil || !epoch.Live() || handle.epoch != epoch || handle.issuer != issuer.value || handle.id == 0 || handle.id&previewRootBit != 0 || !epoch.ownsIssuer(issuer) {
		return 0, false
	}
	return handle.id, true
}

func (epoch *RootEpoch) ownsIssuer(issuer Issuer) bool {
	owner := issuerOwner(issuer.value, true)
	return epoch != nil && epoch.layout != nil && owner != nil && owner.layout == epoch.layout
}

// BindRootStore installs one slot's dynamic-root membership authority before
// the Work evaluates.  A later work gets a distinct RootEpoch and distinct
// store, even when both use the same sealed Binding and compact root IDs.
func (epoch *RootEpoch) BindRootStore(issuer Issuer, store EpochRootStore) bool {
	if epoch == nil || store == nil || !epoch.Active() || !epoch.ownsIssuer(issuer) {
		return false
	}
	epoch.mu.Lock()
	defer epoch.mu.Unlock()
	if !epoch.Active() {
		return false
	}
	if _, exists := epoch.roots[issuer.value]; exists {
		return false
	}
	epoch.roots[issuer.value] = store
	return true
}

func (epoch *RootEpoch) rootStore(issuer Issuer) EpochRootStore {
	if epoch == nil || !epoch.Live() || !epoch.ownsIssuer(issuer) {
		return nil
	}
	epoch.mu.RLock()
	defer epoch.mu.RUnlock()
	if !epoch.Live() {
		return nil
	}
	return epoch.roots[issuer.value]
}

// IssueRoot mints one dynamic root under this exact Work token.  Membership is
// checked before handle construction, so neither an arbitrary ID nor a root
// from a sibling Work can become a valid carrier root.
func (epoch *RootEpoch) IssueRoot(issuer Issuer, id uint64) (RootHandle, bool) {
	if id == 0 || id&previewRootBit != 0 {
		return RootHandle{}, false
	}
	store := epoch.rootStore(issuer)
	if store == nil || !store.EpochRootValid(id) {
		return RootHandle{}, false
	}
	return RootHandle{issuer: issuer.value, id: id, epoch: epoch}, true
}

// ResolveRoot proves both the epoch token and its slot-local root-store
// membership.  It returns only the local compact ID; typed Plane resolution
// remains with the Factor binding that registered the store.
func (epoch *RootEpoch) ResolveRoot(issuer Issuer, handle RootHandle) (uint64, bool) {
	_, id, ok := epoch.RootStore(issuer, handle)
	return id, ok
}

// RootStore returns only the opaque membership store and compact ID for one
// live dynamic handle.  The carrier cannot inspect its payload; the owning
// typed Binding may type-assert its own private store to resolve a Plane.  It
// is what lets a concurrent Work read an immutable source epoch without
// merging arenas or accepting a sibling epoch as its own publication target.
func (epoch *RootEpoch) RootStore(issuer Issuer, handle RootHandle) (EpochRootStore, uint64, bool) {
	if epoch == nil || handle.epoch != epoch || handle.issuer != issuer.value || handle.id == 0 || handle.id&previewRootBit != 0 {
		return nil, 0, false
	}
	store := epoch.rootStore(issuer)
	if store == nil || !store.EpochRootValid(handle.id) {
		return nil, 0, false
	}
	return store, handle.id, true
}

// IssuePreviewRoot and ResolvePreviewRoot use the same epoch token but leave
// the temporary-plane map in the slot work.  Preview IDs never enter a root
// store and are revoked by the linear publisher or epoch close.
func (epoch *RootEpoch) IssuePreviewRoot(issuer Issuer, id uint64) (RootHandle, bool) {
	if id == 0 || id&previewRootBit != 0 || epoch == nil || !epoch.Active() || !epoch.ownsIssuer(issuer) {
		return RootHandle{}, false
	}
	return RootHandle{issuer: issuer.value, id: id | previewRootBit, epoch: epoch}, true
}

func (epoch *RootEpoch) ResolvePreviewRoot(issuer Issuer, handle RootHandle) (uint64, bool) {
	if epoch == nil || !epoch.Live() || handle.epoch != epoch || !epoch.ownsIssuer(issuer) || handle.issuer != issuer.value || handle.id&previewRootBit == 0 {
		return 0, false
	}
	return handle.id &^ previewRootBit, true
}

func (epoch *RootEpoch) retain() bool {
	return epoch != nil && epoch.state.CompareAndSwap(rootEpochActive, rootEpochRetained)
}

func (epoch *RootEpoch) close() bool {
	if epoch == nil {
		return false
	}
	for {
		state := epoch.state.Load()
		if state == rootEpochClosed {
			return false
		}
		if epoch.state.CompareAndSwap(state, rootEpochClosed) {
			epoch.mu.Lock()
			clear(epoch.roots)
			epoch.roots = nil
			epoch.mu.Unlock()
			return true
		}
	}
}

// previewRootBit keeps transient and root-store identities in one compact
// word. Root stores are bounded below it; the tag is explicit token encoding,
// not a Go-layout trick. RootHandle itself is process-local and never
// serialized.
const previewRootBit uint64 = 1 << 63

func isPreviewRoot(handle RootHandle) bool {
	return handle.issuer != nil && handle.id&previewRootBit != 0
}

// RootPublisher is the Factor-owned prepared publication of one sealed typed
// root. It is opaque to carrier: the Factor retains its payload, terminal
// owner, and reservation mechanics. Ready/Reserve must not mutate published
// state; after every reservation succeeds, Publish is total and returns the
// one new opaque root. Drop consumes an abandoned candidate without rollback.
//
// This small boundary is necessary because carrier cannot hold a generic V
// while direct and merge publications must share one atomic preflight.
type RootPublisher interface {
	Ready() bool
	Reserve() bool
	Publish() RootHandle
	Drop()
}

// Checkpoint is an epoch-local liveness probe. Carrier deliberately knows
// neither context.Context nor a scheduler cancellation vocabulary: the epoch
// owner installs one opaque probe on its evaluator Work before evaluation
// starts. A false result aborts the current unpublishable attempt; it is
// never a semantic budget, approximation, or alternate result.
//
// Implementations must be allocation-free on their hot path. The carrier
// samples the probe only at finite traversal/publication boundaries; once the
// final root-publication cut begins it performs no further sample, so a
// cancelled epoch can never expose a partially published vector.
type Checkpoint func() bool

func (checkpoint Checkpoint) live() bool {
	return checkpoint == nil || checkpoint()
}

// PreviewRootPublisher is the optional non-publishing half of a prepared
// root.  Carrier uses it only inside Preview.  Implementations must allocate
// no root-store identity; Drop revokes the returned handle.
//
// It is deliberately an extension of the existing pending publication proof,
// rather than a second typed-state protocol.  Factor bindings remain the sole
// owners of the opaque temporary plane.
type PreviewRootPublisher interface {
	RootPublisher
	PreviewRoot() (RootHandle, bool)
	// OwnsPreviewRoot is the publisher's typed proof that this exact temporary
	// token is still its live local plane. Carrier combines it with the
	// Preview's exact slot record; no SlotOperation or domain receives Preview
	// vocabulary.
	OwnsPreviewRoot(RootHandle) bool
}

// ChangeHandle is one ephemeral immutable operation-produced proof coupling a
// predecessor root, an optional Factor-owned pending publisher, and canonical
// dependency-unit regions. Only an attached Binding-held Issuer can mint it;
// callers cannot construct or edit its private record.
type ChangeHandle struct {
	issuer *issuer
	record *changeRecord
}

type changeRecord struct {
	before    RootHandle
	after     RootHandle
	publisher RootPublisher
	// factor is the one exact region in which this slot's semantic root
	// differs from its predecessor. It is optional: a distinct physical root
	// may retain only off-support meaning, which becomes observable through a
	// later support Added transition rather than a fictitious plane delta.
	// Its slot is intentionally absent here; prepareCommit derives that from
	// this record's attached issuer after it has proved patch ownership.
	factor   support.Mask
	units    []Unit
	regions  []support.Mask
	consumed bool
}

// ObservationHandle identifies a Factor-owned observation published while a
// compiled read projection is evaluated.  It is intentionally distinct from
// a RootHandle: an observation is not an alternate plane-root authority.
type ObservationHandle struct {
	issuer      *issuer
	work        *observationWork
	generation  identity.Generation
	observation uint64
}

// ObservationWork is one SlotWork-local authority for observations.  A
// Binding creates it only after its Issuer is attached, and retains it inside
// the SlotWork that created the records.  It is deliberately separate from
// the Binding Issuer: roots and Units live for the epoch, while an observation
// result is meaningful only to the evaluator work that produced it.
type ObservationWork struct{ value *observationWork }

type observationWork struct {
	issuer     *issuer
	generation identity.Generation
	active     bool
	rows       []observationRow
}

// observationRow is generation-owned revocable storage for a support cell.
// An escaped ObservationRow stores only an index into its work's current
// generation, never a direct Mask reference. EndObservation clears this slice
// before closing, so stale rows neither expose nor retain support pages.
type observationRow struct {
	generation identity.Generation
	region     support.Mask
}

// ObservationRow is one exact nonempty support piece paired with the opaque
// observation recorded for it.  It has no key, terminal, or semantic payload.
// Only its issuing ObservationWork may resolve it back to a local record.
type ObservationRow struct {
	work   *observationWork
	handle ObservationHandle
	row    uint64
}

// UnitKind distinguishes an exact dependency from a Factor-proved summary.
// It is structural metadata; only the issuing Binding interprets its stable
// identity and closure witness.
type UnitKind uint8

const (
	ExactUnit UnitKind = iota + 1
	SummaryUnit
)

// Unit is a Factor-issued opaque dependency capability.  closure is the
// Factor's presealed proof identity that this capability covers exactly the
// declared dependency surface; neither key nor terminal data is exposed.
type Unit struct {
	issuer  *issuer
	kind    UnitKind
	id      uint64
	closure uint64
}

// TargetMode identifies the presealed strong or weak update discipline.
type TargetMode uint8

const (
	StrongTarget TargetMode = iota + 1
	WeakTarget
)

// Target is a Factor-issued opaque authored write scope. Its typed Binding
// owns the concrete key footprint, strong/weak proof, and separately its
// possible notification closure; carrier sees only stable identity and mode.
type Target struct {
	issuer *issuer
	id     uint64
	mode   TargetMode
}

// Issuer is held privately by one typed Binding. It is the only capability
// that can mint or resolve handles for that Binding's current operation epoch.
// The exposed type contains no semantic value and has no decoding API.
type Issuer struct{ value *issuer }

type issuer struct {
	slot atomic.Pointer[slotOwner]
}

// issuerOwner resolves the one physical owner atomically.  Requiring the
// layout latch makes an attached issuer intentionally inert while the carrier
// is still binding the other operations in its one final publication cut.
func issuerOwner(value *issuer, requirePublished bool) *slotOwner {
	if value == nil {
		return nil
	}
	owner := value.slot.Load()
	if owner == nil || owner.layout == nil || requirePublished && !owner.layout.published.Load() {
		return nil
	}
	return owner
}

// Live reports whether this Issuer crossed its composition's shared layout
// publication latch.  Binding uses it before consulting mutable active-epoch
// fields such as bound, roots, and initial reservation state.
func (issuer Issuer) Live() bool { return issuerOwner(issuer.value, true) != nil }

// NewIssuer starts one private Binding epoch before its physical carrier slot
// is known. Unit and Target declarations may retain this identity while an
// operation is assembled, but remain unusable until Attach binds it.
func NewIssuer() (Issuer, bool) { return Issuer{value: &issuer{}}, true }

// Attach binds this declaration epoch to its one canonical physical slot.
// Only a SlotOwner created by Composition sealing can authorize attachment.
// It is total after operation preflight; violating that internal contract is
// a programming error, not a recoverable partial-publication route.
func (issuer Issuer) Attach(owner SlotOwner) {
	if issuer.value == nil || owner.value == nil || owner.value.layout == nil || !issuer.value.slot.CompareAndSwap(nil, owner.value) {
		panic("invalid issuer attachment")
	}
}

// Slot returns this issuer's attached physical position. A declaration epoch
// has no slot until composition's final attach cut.
func (issuer Issuer) Slot() (shape.Slot, bool) {
	owner := issuerOwner(issuer.value, true)
	if owner == nil {
		return 0, false
	}
	return owner.slot, true
}

// IssueRoot converts a slot-local persistent root ID into an opaque
// carrier handle.  ID zero is deliberately reserved as invalid.
func (issuer Issuer) IssueRoot(id uint64) (RootHandle, bool) {
	if issuerOwner(issuer.value, false) == nil || id == 0 || id&previewRootBit != 0 {
		return RootHandle{}, false
	}
	return RootHandle{issuer: issuer.value, id: id}, true
}

// IssuePreviewRoot issues a transaction-local root token.  The typed Binding
// retains the associated plane and revokes it through its pending publisher;
// this method deliberately has no root-store interaction.
func (issuer Issuer) IssuePreviewRoot(id uint64) (RootHandle, bool) {
	if issuerOwner(issuer.value, true) == nil || id == 0 || id&previewRootBit != 0 {
		return RootHandle{}, false
	}
	return RootHandle{issuer: issuer.value, id: id | previewRootBit}, true
}

// IssueChange freezes one exact prepared result beside its owning typed
// Binding. publisher is nil only when after carries a proven pre-existing
// root. factor is the one exact semantic root-difference region, if one is
// observable under the operation's output support. Its physical slot is never
// caller-authored: carrier derives it from this Issuer after attachment.
// units are typed-read evidence contained within factor and must already be
// in canonical Binding declaration order.
func (issuer Issuer) IssueChange(before, after RootHandle, publisher RootPublisher, factor support.Mask, units []Unit, regions []support.Mask, candidate *support.Work) (ChangeHandle, bool) {
	if issuerOwner(issuer.value, true) == nil || !issuer.ownsRoot(before) || len(units) != len(regions) {
		return ChangeHandle{}, false
	}
	if publisher != nil {
		if !publisher.Ready() || after != (RootHandle{}) {
			return ChangeHandle{}, false
		}
	} else if !issuer.ownsRoot(after) {
		return ChangeHandle{}, false
	}
	presentFactor, factorOK := optionalNonemptyRegion(factor, candidate)
	if !factorOK || publisher == nil && sameRoot(before, after) && (presentFactor || len(units) != 0) || len(units) != 0 && !presentFactor {
		return ChangeHandle{}, false
	}
	for index, unit := range units {
		validRegion := regions[index].Valid() || candidate != nil && candidate.Valid(regions[index])
		if unit.issuer != issuer.value || unit.id == 0 || unit.closure == 0 || (unit.kind != ExactUnit && unit.kind != SummaryUnit) || !validRegion || !regionEntails(candidate, regions[index], factor) || index > 0 && !units[index-1].Less(unit) {
			return ChangeHandle{}, false
		}
	}
	record := &changeRecord{
		before:    before,
		after:     after,
		publisher: publisher,
		factor:    factor,
		units:     append([]Unit(nil), units...),
		regions:   append([]support.Mask(nil), regions...),
	}
	return ChangeHandle{issuer: issuer.value, record: record}, true
}

// optionalNonemptyRegion distinguishes the zero Mask used for "no observable
// Factor change" from one supplied region. A supplied empty region is not a
// row: accepting it would make the ChangeHandle vocabulary ambiguous and
// would let a UnitRegion claim containment in an empty FactorRegion.
func optionalNonemptyRegion(region support.Mask, candidate *support.Work) (present, valid bool) {
	if region == (support.Mask{}) {
		return false, true
	}
	var (
		view support.Decomposition
		ok   bool
	)
	if region.Valid() {
		view, ok = region.Decompose()
	} else if candidate != nil && candidate.Valid(region) {
		view, ok = candidate.Decompose(region)
	}
	if !ok {
		return false, false
	}
	return !(view.Terminal && !view.Value), !(view.Terminal && !view.Value)
}

func (change ChangeHandle) factorRegion(candidate *support.Work) (present, valid bool) {
	if change.record == nil {
		return false, false
	}
	return optionalNonemptyRegion(change.record.factor, candidate)
}

// regionEntails admits sealed and one shared candidate support transaction.
// It is deliberately the only containment check used while minting a
// ChangeHandle, so Unit evidence cannot escape the semantic Factor region
// that justifies it.
func regionEntails(candidate *support.Work, premise, conclusion support.Mask) bool {
	if candidate != nil && candidate.Valid(premise) && candidate.Valid(conclusion) {
		return candidate.Entails(premise, conclusion)
	}
	return premise.Valid() && conclusion.Valid() && premise.Entails(conclusion)
}

// ownsRoot validates the two mutually exclusive issuer-local root token
// forms.  Binding later proves a preview token still maps to a live typed
// plane; Issuer deliberately has no access to that payload table.
func (issuer Issuer) ownsRoot(handle RootHandle) bool {
	if handle.issuer != issuer.value || handle.id == 0 {
		return false
	}
	if handle.epoch == nil {
		return issuerOwner(issuer.value, true) != nil
	}
	if handle.id&previewRootBit != 0 {
		_, ok := handle.epoch.ResolvePreviewRoot(issuer, handle)
		return ok
	}
	_, ok := handle.epoch.ResolveRoot(issuer, handle)
	return ok
}

// ResolveRoot proves this handle came from this precise Factor epoch and
// returns only the structural Diagram identity for typed Binding resolution.
func (issuer Issuer) ResolveRoot(handle RootHandle) (uint64, bool) {
	if issuerOwner(issuer.value, true) == nil || handle.epoch != nil || handle.issuer != issuer.value || handle.id == 0 || handle.id&previewRootBit != 0 {
		return 0, false
	}
	return handle.id, true
}

// ValidRoot is the uniform typed-slot root fence.  Composition-attached
// initial roots resolve directly through Issuer; dynamic roots additionally
// prove membership in their still-live Work epoch's slot-local root store.
func (issuer Issuer) ValidRoot(handle RootHandle) bool {
	if handle.epoch == nil {
		_, ok := issuer.ResolveRoot(handle)
		return ok
	}
	_, ok := handle.epoch.ResolveRoot(issuer, handle)
	return ok
}

// ResolveEpochRoot proves a dynamic root is still live and returns only its
// opaque typed-store membership.  It is deliberately unavailable for the
// composition-attached initial root, which remains resolved directly through
// Issuer.ResolveRoot.
func (issuer Issuer) ResolveEpochRoot(handle RootHandle) (EpochRootStore, uint64, bool) {
	if handle.epoch == nil {
		return nil, 0, false
	}
	return handle.epoch.RootStore(issuer, handle)
}

// ResolvePreviewRoot proves that handle is one local temporary root of this
// exact Binding epoch.  It returns only the opaque token needed by Binding to
// find its typed plane; the carrier never receives that plane.
func (issuer Issuer) ResolvePreviewRoot(handle RootHandle) (uint64, bool) {
	if issuerOwner(issuer.value, true) == nil || handle.issuer != issuer.value || handle.id&previewRootBit == 0 {
		return 0, false
	}
	if handle.epoch != nil {
		return handle.epoch.ResolvePreviewRoot(issuer, handle)
	}
	return handle.id &^ previewRootBit, true
}

// NewObservationWork creates one evaluator-local observation namespace for
// this attached issuer. A Binding keeps it private to its SlotWork, so a row
// cannot be resolved through another evaluator of the same Factor.
func (issuer Issuer) NewObservationWork() (ObservationWork, bool) {
	if issuerOwner(issuer.value, true) == nil {
		return ObservationWork{}, false
	}
	return ObservationWork{value: &observationWork{issuer: issuer.value}}, true
}

// BeginObservation opens one explicit callback generation. A row is valid
// only while this generation remains open; EndObservation rejects every row
// that escaped its callback before a later generation can reuse scratch.
func (issuer Issuer) BeginObservation(work ObservationWork) (identity.Generation, bool) {
	if issuerOwner(issuer.value, true) == nil || work.value == nil || work.value.issuer != issuer.value || work.value.active {
		return 0, false
	}
	opened := work.value.generation.Next()
	if !opened.Available() {
		return 0, false
	}
	clear(work.value.rows)
	work.value.rows = work.value.rows[:0]
	work.value.generation = opened
	work.value.active = true
	return work.value.generation, true
}

// EndObservation closes one exact callback generation. It is intentionally
// idempotent for stale or already-aborted cleanup paths.
func (issuer Issuer) EndObservation(work ObservationWork, generation identity.Generation) {
	if issuerOwner(issuer.value, true) == nil || work.value == nil || work.value.issuer != issuer.value || !work.value.active || !generation.Available() || work.value.generation != generation {
		return
	}
	clear(work.value.rows)
	work.value.rows = work.value.rows[:0]
	work.value.active = false
}

// IssueObservation converts a Binding-owned local record ID into an opaque
// observation handle for exactly one open ObservationWork generation.
func (issuer Issuer) IssueObservation(work ObservationWork, generation identity.Generation, id uint64) (ObservationHandle, bool) {
	if issuerOwner(issuer.value, true) == nil || work.value == nil || work.value.issuer != issuer.value || !work.value.active || !generation.Available() || work.value.generation != generation || id == 0 {
		return ObservationHandle{}, false
	}
	return ObservationHandle{issuer: issuer.value, work: work.value, generation: generation, observation: id}, true
}

// IssueUnit issues one exact or summary dependency capability.  Stable unit
// and closure identities are Factor-local and must both be nonzero.
func (issuer Issuer) IssueUnit(kind UnitKind, id, closure uint64) (Unit, bool) {
	if issuer.value == nil || (kind != ExactUnit && kind != SummaryUnit) || id == 0 || closure == 0 {
		return Unit{}, false
	}
	return Unit{issuer: issuer.value, kind: kind, id: id, closure: closure}, true
}

// IssueTarget issues one presealed strong or weak typed write capability.
func (issuer Issuer) IssueTarget(id uint64, mode TargetMode) (Target, bool) {
	if issuer.value == nil || id == 0 || (mode != StrongTarget && mode != WeakTarget) {
		return Target{}, false
	}
	return Target{issuer: issuer.value, id: id, mode: mode}, true
}

// Kind returns Unit's sealed exact/summary class.
func (unit Unit) Kind() UnitKind { return unit.kind }

// Slot returns Unit's canonical physical owner slot.  It is structural routing
// information only; the typed Binding retains key and closure interpretation.
func (unit Unit) Slot() (shape.Slot, bool) {
	owner := issuerOwner(unit.issuer, true)
	if owner == nil {
		return 0, false
	}
	return owner.slot, true
}

// Same reports identical issued dependency capability identity.
func (unit Unit) Same(other Unit) bool {
	return unit.issuer == other.issuer && unit.kind == other.kind && unit.id == other.id && unit.closure == other.closure
}

// Less provides the canonical structural event order: Factor slot, kind,
// stable unit identity, then closure proof identity.
func (unit Unit) Less(other Unit) bool {
	leftSlot, leftOK := unit.Slot()
	rightSlot, rightOK := other.Slot()
	if !leftOK || !rightOK {
		return false
	}
	if leftSlot != rightSlot {
		return leftSlot < rightSlot
	}
	if unit.kind != other.kind {
		return unit.kind < other.kind
	}
	if unit.id != other.id {
		return unit.id < other.id
	}
	return unit.closure < other.closure
}

// Mode returns Target's sealed strong/weak class.
func (target Target) Mode() TargetMode { return target.mode }

// Slot returns Target's canonical physical owner slot.
func (target Target) Slot() (shape.Slot, bool) {
	owner := issuerOwner(target.issuer, true)
	if owner == nil {
		return 0, false
	}
	return owner.slot, true
}

// Same reports identical issued write capability identity.
func (target Target) Same(other Target) bool {
	return target.issuer == other.issuer && target.id == other.id && target.mode == other.mode
}

// Less provides canonical write-capability order: physical Factor slot,
// target discipline, then the Binding-issued stable identity.  It exposes no
// typed key or target surface.
func (target Target) Less(other Target) bool {
	leftSlot, leftOK := target.Slot()
	rightSlot, rightOK := other.Slot()
	if !leftOK || !rightOK {
		return false
	}
	if leftSlot != rightSlot {
		return leftSlot < rightSlot
	}
	if target.mode != other.mode {
		return target.mode < other.mode
	}
	return target.id < other.id
}

// ResolveObservation proves issuer, work, and live-generation ownership and
// returns only the Binding-local record identity; it never returns a terminal
// or domain value.
func (issuer Issuer) ResolveObservation(work ObservationWork, generation identity.Generation, handle ObservationHandle) (uint64, bool) {
	if issuerOwner(issuer.value, true) == nil || work.value == nil || work.value.issuer != issuer.value || !work.value.active || !generation.Available() || work.value.generation != generation || handle.issuer != issuer.value || handle.work != work.value || handle.generation != generation || handle.observation == 0 {
		return 0, false
	}
	return handle.observation, true
}

// Row binds one opaque observation handle to its exact support piece.  The
// caller cannot construct a row for another work or an unissued handle.
func (work ObservationWork) Row(handle ObservationHandle, region support.Mask) (ObservationRow, bool) {
	if work.value == nil || issuerOwner(work.value.issuer, true) == nil || !work.value.active || handle.issuer != work.value.issuer || handle.work != work.value || !handle.generation.Available() || handle.generation != work.value.generation || handle.observation == 0 || !region.Valid() {
		return ObservationRow{}, false
	}
	if uint64(len(work.value.rows)) == ^uint64(0) {
		return ObservationRow{}, false
	}
	work.value.rows = append(work.value.rows, observationRow{generation: work.value.generation, region: region})
	return ObservationRow{work: work.value, handle: handle, row: uint64(len(work.value.rows))}, true
}

// ResolveRow proves this row came from exactly this evaluator-local
// observation namespace.  The returned handle is still opaque.
func (work ObservationWork) ResolveRow(row ObservationRow) (ObservationHandle, support.Mask, bool) {
	if work.value == nil || issuerOwner(work.value.issuer, true) == nil || !work.value.active || row.work != work.value || row.row == 0 || row.row > uint64(len(work.value.rows)) || row.handle.issuer != work.value.issuer || row.handle.work != work.value || !row.handle.generation.Available() || row.handle.generation != work.value.generation || row.handle.observation == 0 {
		return ObservationHandle{}, support.Mask{}, false
	}
	record := work.value.rows[row.row-1]
	if record.generation != work.value.generation || !record.region.Valid() {
		return ObservationHandle{}, support.Mask{}, false
	}
	return row.handle, record.region, true
}

// Handle returns this piece's opaque typed observation identity.
func (row ObservationRow) Handle() ObservationHandle { return row.handle }

// Region returns this piece's exact support cell only while the row's own
// observation generation remains live. A stale escaped row returns an invalid
// zero Mask and no longer retains the generation's support page through row
// storage.
func (row ObservationRow) Region() support.Mask {
	if row.work == nil || !row.work.active || row.row == 0 || row.row > uint64(len(row.work.rows)) || row.handle.work != row.work || row.handle.generation != row.work.generation {
		return support.Mask{}
	}
	record := row.work.rows[row.row-1]
	if record.generation != row.work.generation {
		return support.Mask{}
	}
	return record.region
}

func sameRoot(left, right RootHandle) bool {
	return left.issuer == right.issuer && left.id == right.id && left.epoch == right.epoch
}

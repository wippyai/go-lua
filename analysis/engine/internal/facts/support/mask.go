// Package support owns immutable fact-definedness regions.  Its sole
// representation is the engine's exact reduced ordered BDD: there is no
// conjunction-only mask language and therefore no parallel symbolic carrier.
package support

import "github.com/wippyai/go-lua/analysis/engine/internal/guard"

// Mask is one exact Boolean region over a sealed guard manager.  It says
// where a fact is defined, never what terminal the fact carries.
type Mask struct {
	manager *guard.Manager
	root    guard.Guard
}

// Split is one exact left/right three-region decomposition.  It belongs beside
// Mask because every consumer—carrier and typed semantic plane alike—must use
// the same shared Boolean calculation rather than rebuilding private splits.
type Split struct {
	left      Mask
	right     Mask
	leftOnly  Mask
	rightOnly Mask
	overlap   Mask
	union     Mask
	sealed    bool
}

// Valid reports whether split is a complete decomposition constructed by
// Three for manager.  Its unexported seal makes exact disjoint-cover
// provenance package-owned; this check deliberately does not re-traverse the
// six Boolean relations at every semantic merge.
func (split Split) Valid(manager *guard.Manager) bool {
	return split.sealed && manager != nil &&
		split.left.manager == manager && split.right.manager == manager &&
		split.leftOnly.manager == manager && split.rightOnly.manager == manager &&
		split.overlap.manager == manager && split.union.manager == manager &&
		split.left.Valid() && split.right.Valid() && split.leftOnly.Valid() &&
		split.rightOnly.Valid() && split.overlap.Valid() && split.union.Valid()
}

// Left returns the exact original left region.
func (split Split) Left() Mask { return split.left }

// Right returns the exact original right region.
func (split Split) Right() Mask { return split.right }

// LeftOnly returns the exact left-minus-right region.
func (split Split) LeftOnly() Mask { return split.leftOnly }

// RightOnly returns the exact right-minus-left region.
func (split Split) RightOnly() Mask { return split.rightOnly }

// Overlap returns the exact intersection region.
func (split Split) Overlap() Mask { return split.overlap }

// Union returns the exact combined region.
func (split Split) Union() Mask { return split.union }

// Work is a single guard candidate transaction used to construct one or more
// correlated Masks.  Its BDD caches are private to the transaction and are
// dropped by Seal or Discard.
type Work struct {
	manager    *guard.Manager
	work       *guard.Work
	checkpoint func() bool
	// active is the single-owner reservation for helper transactions such as
	// Three/Union. It prevents a nested helper from borrowing the shell while
	// its caller is still constructing a candidate (notably while carrier's
	// delta candidate remains open).
	active bool
}

// FromGuard rewraps one already-sealed Guard as this package's sole support
// representation.  It performs no Boolean construction and retains no
// alternate guard carrier; callers crossing the engine boundary must prove
// the exact manager ownership before a Mask can exist.
func FromGuard(manager *guard.Manager, region guard.Guard) (Mask, bool) {
	if manager == nil || !manager.Valid(region) {
		return Mask{}, false
	}
	return Mask{manager: manager, root: region}, true
}

// New opens one exact BDD construction scope for manager.
func New(manager *guard.Manager) *Work {
	if manager == nil || !manager.Valid(manager.True()) {
		return nil
	}
	return &Work{manager: manager, work: manager.NewWork()}
}

// Begin reopens this support shell after Seal or Discard. The first
// transaction starts already open from New; callers that own a long-lived
// shell call Begin between terminal transactions. Reopening an open shell is
// rejected so a nested symbolic operation cannot reset its owner's candidate.
func (work *Work) Begin() bool {
	if work == nil || work.work == nil || work.Open() || work.active || !work.work.Begin() {
		return false
	}
	work.checkpoint = nil
	work.active = false
	return true
}

// Reset and Reopen are explicit aliases for the same linear reopen cut.
func (work *Work) Reset() bool  { return work.Begin() }
func (work *Work) Reopen() bool { return work.Begin() }

// BeginTransaction reserves this shell for one helper transaction. It is
// intentionally separate from Begin: a freshly-created support shell is
// already open, while a reused shell must first reopen its guard Work.
func (work *Work) BeginTransaction(checkpoint func() bool) bool {
	if work == nil || work.work == nil || work.active {
		return false
	}
	if !work.Open() && !work.Begin() {
		return false
	}
	work.active = true
	if checkpoint != nil && !work.SetCheckpoint(checkpoint) {
		work.active = false
		work.Discard()
		return false
	}
	return true
}

func (work *Work) markActive() bool {
	if work == nil || !work.Open() {
		return false
	}
	work.active = true
	return true
}

// SetCheckpoint forwards one evaluator-owned opaque liveness probe into this
// candidate BDD transaction. It affects only whether the unsealed candidate
// completes; it never changes the Boolean algebra or the identity of a
// completed Mask.
func (work *Work) SetCheckpoint(checkpoint func() bool) bool {
	if !work.Open() {
		return false
	}
	work.active = true
	work.checkpoint = checkpoint
	return work.work.SetCheckpoint(checkpoint)
}

func (work *Work) live() bool {
	return work != nil && (work.checkpoint == nil || work.checkpoint()) && work.work != nil && work.work.Live()
}

// True returns the sealed unconstrained region for manager without exposing a
// raw guard handle.  It is the cold construction counterpart to Work.True.
func True(manager *guard.Manager) (Mask, bool) {
	work := New(manager)
	if work == nil {
		return Mask{}, false
	}
	value := work.True()
	if !work.Seal() {
		work.Discard()
		return Mask{}, false
	}
	return value, true
}

// True returns the unconstrained candidate region.
func (work *Work) True() Mask {
	if !work.live() || !work.markActive() {
		return Mask{}
	}
	return Mask{manager: work.manager, root: work.work.True()}
}

// False returns the empty candidate region.
func (work *Work) False() Mask {
	if !work.live() || !work.markActive() {
		return Mask{}
	}
	return Mask{manager: work.manager, root: work.work.False()}
}

// Literal returns atom or its exact complement.
func (work *Work) Literal(atom guard.Atom, value bool) (Mask, bool) {
	if !work.live() || !work.markActive() {
		return Mask{}, false
	}
	root, valid := work.work.Literal(atom)
	if !valid || !work.live() {
		return Mask{}, false
	}
	if !value {
		root = work.work.Not(root)
	}
	if !work.live() {
		return Mask{}, false
	}
	return Mask{manager: work.manager, root: root}, true
}

// Conjoin adds one literal to an arbitrary candidate or sealed region.  BDD
// reduction normalizes duplicates, implications, and arbitrary shared
// topology exactly.
func (work *Work) Conjoin(mask Mask, atom guard.Atom, value bool) (Mask, bool) {
	literal, valid := work.Literal(atom, value)
	if !valid || !work.Valid(mask) {
		return Mask{}, false
	}
	root := work.work.And(mask.root, literal.root)
	if !work.live() {
		return Mask{}, false
	}
	return Mask{manager: work.manager, root: root}, true
}

// And intersects two exact regions.
func (work *Work) And(left, right Mask) (Mask, bool) {
	if !work.live() || !work.markActive() || !work.Valid(left) || !work.Valid(right) {
		return Mask{}, false
	}
	root := work.work.And(left.root, right.root)
	return Mask{manager: work.manager, root: root}, work.live()
}

// Or unions two exact regions.
func (work *Work) Or(left, right Mask) (Mask, bool) {
	if !work.live() || !work.markActive() || !work.Valid(left) || !work.Valid(right) {
		return Mask{}, false
	}
	root := work.work.Or(left.root, right.root)
	return Mask{manager: work.manager, root: root}, work.live()
}

// Decision reconstructs one ordered Boolean cofactor pair.  It is used by
// synchronized fact operations which compute an exact Guard result alongside
// an FDD result without materializing path valuations.
func (work *Work) Decision(atom guard.Atom, low, high Mask) (Mask, bool) {
	if !work.live() || !work.markActive() || !work.Valid(low) || !work.Valid(high) {
		return Mask{}, false
	}
	if low.root == high.root {
		return low, true
	}
	literal, valid := work.work.Literal(atom)
	if !valid || !work.live() {
		return Mask{}, false
	}
	positive := work.work.And(literal, high.root)
	if !work.live() {
		return Mask{}, false
	}
	negative := work.work.And(work.work.Not(literal), low.root)
	if !work.live() {
		return Mask{}, false
	}
	root := work.work.Or(negative, positive)
	return Mask{manager: work.manager, root: root}, work.live()
}

// Not complements one exact region.
func (work *Work) Not(mask Mask) (Mask, bool) {
	if !work.live() || !work.markActive() || !work.Valid(mask) {
		return Mask{}, false
	}
	root := work.work.Not(mask.root)
	return Mask{manager: work.manager, root: root}, work.live()
}

// Exists discharges one exact guard atom.  It is used only at a proven
// boundary (for example Mu); the typed fact carrier performs the matching
// terminal join separately, so Boolean support never gains a payload role.
func (work *Work) Exists(mask Mask, atom guard.Atom) (Mask, bool) {
	if !work.live() || !work.markActive() || !work.Valid(mask) {
		return Mask{}, false
	}
	if _, admitted := work.manager.Rank(atom); !admitted {
		return Mask{}, false
	}
	root := work.work.Exists(mask.root, atom)
	return Mask{manager: work.manager, root: root}, work.live()
}

// Reindex transports one support region through a sealed source-to-target
// relation. The caller supplies no raw atoms or replacement map on this hot
// path. A complete identity relation retains the exact immutable mask root.
func Reindex(mask Mask, plan guard.Reindex) (Mask, bool) {
	return ReindexWithWork(nil, mask, plan)
}

// ReindexWithWork transports a support region using a caller-owned shell.
// The shell is reserved for the duration of this one candidate and is
// released by Seal or Discard, so a concurrent delta candidate cannot be
// accidentally reset by a nested transport.
func ReindexWithWork(work *Work, mask Mask, plan guard.Reindex) (Mask, bool) {
	if !mask.Valid() || !plan.Valid() || mask.Manager() != plan.Source().Manager() || !plan.Source().Contains(mask.root) {
		return Mask{}, false
	}
	if plan.Identity() {
		return mask, true
	}
	if work == nil {
		work = New(mask.manager)
	}
	if work == nil || work.manager != mask.manager || !work.BeginTransaction(nil) {
		return Mask{}, false
	}
	root, ok := work.work.Reindex(mask.root, plan)
	if !ok {
		work.Discard()
		return Mask{}, false
	}
	result := Mask{manager: mask.manager, root: root}
	if !work.Seal() {
		work.Discard()
		return Mask{}, false
	}
	return result, true
}

// Open reports whether candidate construction remains possible.
func (work *Work) Open() bool {
	return work != nil && work.manager != nil && work.work != nil && work.work.Open()
}

// Valid accepts sealed regions from this manager and candidate regions built
// by this Work.  guard.Work is the exact candidate-page authority.
func (work *Work) Valid(mask Mask) bool {
	return work != nil && work.manager != nil && mask.manager == work.manager && work.work != nil && work.work.Valid(mask.root)
}

// Entails proves inclusion while either side may still belong to this one
// candidate Boolean transaction. It is the candidate counterpart of
// Mask.Entails and never publishes a second support representation.
func (work *Work) Entails(premise, conclusion Mask) bool {
	return work.live() && work.Valid(premise) && work.Valid(conclusion) && work.work.Entails(premise.root, conclusion.root) && work.live()
}

// Decompose reads either a sealed region or a candidate region owned by this
// Work.  Candidate-aware decomposition is required by synchronized symbolic
// operations which build an exact result while continuing to refine it; it
// does not publish the candidate or create another Boolean representation.
func (work *Work) Decompose(mask Mask) (Decomposition, bool) {
	if !work.live() || !work.Valid(mask) {
		return Decomposition{}, false
	}
	view, valid := work.work.Decompose(mask.root)
	if !valid {
		return Decomposition{}, false
	}
	if view.Terminal {
		return Decomposition{Terminal: true, Value: view.Value}, true
	}
	return Decomposition{
		Atom: view.Atom,
		Low:  Mask{manager: work.manager, root: view.Low},
		High: Mask{manager: work.manager, root: view.High},
	}, true
}

// Seal publishes every Mask constructed by this Work in one BDD cut.  A Mask
// then becomes valid through its manager and no construction cache survives.
func (work *Work) Seal() bool {
	if !work.live() {
		return false
	}
	work.work.Seal()
	if !work.work.Published() {
		return false
	}
	work.active = false
	work.checkpoint = nil
	return true
}

// Discard invalidates all candidate masks and releases local BDD caches.
func (work *Work) Discard() {
	if work != nil && work.work != nil {
		work.work.Discard()
		work.active = false
		work.checkpoint = nil
	}
}

// Close permanently evicts this owner-local shell and all retained immutable
// interner/memo references. Published Masks remain valid through their Guard
// page handles, but this Work cannot be reopened after Close.
func (work *Work) Close() {
	if work == nil {
		return
	}
	if work.work != nil {
		work.work.Close()
	}
	work.manager = nil
	work.work = nil
	work.active = false
	work.checkpoint = nil
}

// Valid reports whether mask is a sealed, readable exact BDD region.
func (mask Mask) Valid() bool { return mask.manager != nil && mask.manager.Valid(mask.root) }

// Manager returns mask's exact sealed BDD manager.
func (mask Mask) Manager() *guard.Manager {
	if !mask.Valid() {
		return nil
	}
	return mask.manager
}

// Guard returns Mask's exact already-sealed guard handle.  Together with
// FromGuard it is the complete zero-copy boundary between engine guards and
// fact support; neither side creates a parallel condition representation.
func (mask Mask) Guard() (guard.Guard, bool) {
	if !mask.Valid() {
		return guard.Guard{}, false
	}
	return mask.root, true
}

// Identity returns the collision-resistant canonical identity of this exact
// support formula. It forwards the sole guard representation without creating
// a second support encoding, so equal reduced formulas from separate manager
// generations have the same evidence identity.
func (mask Mask) Identity() (guard.FormulaID, bool) {
	return mask.IdentityWithCheckpoint(nil)
}

// IdentityWithCheckpoint forwards an evaluator-owned liveness probe into the
// one guard-formula identity traversal. Cancellation produces no partial
// support identity and never creates an alternate support encoding.
func (mask Mask) IdentityWithCheckpoint(checkpoint func() bool) (guard.FormulaID, bool) {
	if !mask.Valid() {
		return guard.FormulaID{}, false
	}
	return guard.IdentityWithCheckpoint(mask.root, checkpoint)
}

// Equal reports exact reduced-BDD equivalence.  Identical page handles are
// only a fast path inside guard; cross-generation structural equality remains
// the authority.
func (mask Mask) Equal(other Mask) bool {
	return mask.Valid() && other.Valid() && mask.manager == other.manager && mask.manager.Equivalent(mask.root, other.root)
}

// SameHandle reports whether two valid masks retain the same physical BDD
// handle.  It is an immutable-predecessor identity check, not semantic
// equality: Equal remains the authority for cross-generation BDD equivalence.
func (mask Mask) SameHandle(other Mask) bool {
	return mask.Valid() && other.Valid() && mask.manager == other.manager && mask.root == other.root
}

// IsTrue reports the canonical unrestricted formula without opening a BDD
// work transaction.  Boundary transport uses it solely for its exact
// allocation-free identity fast path; semantic filtering always goes through
// Restrict/Transfer below it.
func (mask Mask) IsTrue() bool {
	return mask.Valid() && mask.root == mask.manager.True()
}

// Entails proves exact Boolean support inclusion without exposing a raw guard
// handle.  `premise.Entails(conclusion)` means every supported valuation of
// premise is also supported by conclusion.
func (mask Mask) Entails(conclusion Mask) bool {
	return mask.Valid() && conclusion.Valid() && mask.manager == conclusion.manager && mask.manager.Entails(mask.root, conclusion.root)
}

// Decomposition is one read-only BDD view.  A terminal has Terminal set and
// Value is its exact Boolean result; a decision exposes its ordered atom and
// exact low/high subregions.  It carries no alternate mask representation.
type Decomposition struct {
	Atom     guard.Atom
	Low      Mask
	High     Mask
	Terminal bool
	Value    bool
}

// Decompose returns one sealed BDD node as an exact support view.
func (mask Mask) Decompose() (Decomposition, bool) {
	if !mask.Valid() {
		return Decomposition{}, false
	}
	view, valid := mask.manager.Decompose(mask.root)
	if !valid {
		return Decomposition{}, false
	}
	if view.Terminal {
		return Decomposition{Terminal: true, Value: view.Value}, true
	}
	return Decomposition{
		Atom: view.Atom,
		Low:  Mask{manager: mask.manager, root: view.Low}, High: Mask{manager: mask.manager, root: view.High},
	}, true
}

// Matches evaluates one sealed region without constructing a new BDD.
func (mask Mask) Matches(valuation func(guard.Atom) bool) bool {
	if !mask.Valid() || valuation == nil {
		return false
	}
	current := mask.root
	for {
		view, valid := mask.manager.Decompose(current)
		if !valid {
			return false
		}
		if view.Terminal {
			return view.Value
		}
		if valuation(view.Atom) {
			current = view.High
		} else {
			current = view.Low
		}
	}
}

// Three computes the exact left-only, right-only, overlap, and union masks
// once.  A consumer receives the original supports together with their
// decomposition so it never reconstructs either side from the three pieces.
func Three(left, right Mask) (Split, bool) {
	return ThreeWithCheckpoint(nil, left, right)
}

// ThreeWithCheckpoint is Three's same exact Boolean decomposition with an
// evaluator-owned opaque liveness probe. The probe changes only whether the
// disposable BDD transaction completes; a successful split is byte-for-byte
// the normal exact split.
func ThreeWithCheckpoint(checkpoint func() bool, left, right Mask) (Split, bool) {
	return ThreeWithWork(nil, checkpoint, left, right)
}

// ThreeWithWork computes one exact decomposition using a caller-owned
// reusable shell. A shell that is already active is rejected, preserving the
// single-owner rule for nested symbolic operations.
func ThreeWithWork(work *Work, checkpoint func() bool, left, right Mask) (Split, bool) {
	if !left.Valid() || !right.Valid() || left.manager != right.manager {
		return Split{}, false
	}
	if left == right {
		falseMask := Mask{manager: left.manager, root: left.manager.False()}
		return Split{
			left: left, right: right, leftOnly: falseMask, rightOnly: falseMask,
			overlap: left, union: left, sealed: true,
		}, true
	}
	if work == nil {
		work = New(left.manager)
	}
	if work == nil || work.manager != left.manager || !work.BeginTransaction(checkpoint) {
		return Split{}, false
	}
	notLeft, ok := work.Not(left)
	if !ok {
		work.Discard()
		return Split{}, false
	}
	notRight, ok := work.Not(right)
	if !ok {
		work.Discard()
		return Split{}, false
	}
	leftOnly, ok := work.And(left, notRight)
	if !ok {
		work.Discard()
		return Split{}, false
	}
	rightOnly, ok := work.And(right, notLeft)
	if !ok {
		work.Discard()
		return Split{}, false
	}
	overlap, ok := work.And(left, right)
	if !ok {
		work.Discard()
		return Split{}, false
	}
	union, ok := work.Or(left, right)
	if !ok || !work.Seal() {
		work.Discard()
		return Split{}, false
	}
	return Split{left: left, right: right, leftOnly: leftOnly, rightOnly: rightOnly, overlap: overlap, union: union, sealed: true}, true
}

// Intersect returns one exact support-only view region.  It has no Factor or
// payload meaning and is the sole Boolean operation used by carrier views.
func Intersect(left, right Mask) (Mask, bool) {
	return IntersectWithCheckpoint(nil, left, right)
}

// IntersectWithCheckpoint is Intersect with the same exact algebra and an
// optional evaluator-owned liveness probe. Cancellation abandons only the
// disposable candidate transaction; it never substitutes an approximation.
func IntersectWithCheckpoint(checkpoint func() bool, left, right Mask) (Mask, bool) {
	return IntersectWithWork(nil, checkpoint, left, right)
}

// IntersectWithWork is Intersect over one caller-owned reusable shell.
func IntersectWithWork(work *Work, checkpoint func() bool, left, right Mask) (Mask, bool) {
	if !left.Valid() || !right.Valid() || left.manager != right.manager {
		return Mask{}, false
	}
	if left == right {
		return left, true
	}
	if work == nil {
		work = New(left.manager)
	}
	if work == nil || work.manager != left.manager || !work.BeginTransaction(checkpoint) {
		return Mask{}, false
	}
	result, ok := work.And(left, right)
	if !ok || !work.Seal() {
		work.Discard()
		return Mask{}, false
	}
	return result, true
}

// Union returns one exact support union.  It never combines a Factor payload.
func Union(left, right Mask) (Mask, bool) {
	return UnionWithCheckpoint(nil, left, right)
}

// UnionWithCheckpoint is Union with the same exact algebra and an optional
// evaluator-owned liveness probe. A false probe discards the candidate rather
// than returning an approximate region.
func UnionWithCheckpoint(checkpoint func() bool, left, right Mask) (Mask, bool) {
	return UnionWithWork(nil, checkpoint, left, right)
}

// UnionWithWork is Union over one caller-owned reusable shell.
func UnionWithWork(work *Work, checkpoint func() bool, left, right Mask) (Mask, bool) {
	if !left.Valid() || !right.Valid() || left.manager != right.manager {
		return Mask{}, false
	}
	if left == right {
		return left, true
	}
	if work == nil {
		work = New(left.manager)
	}
	if work == nil || work.manager != left.manager || !work.BeginTransaction(checkpoint) {
		return Mask{}, false
	}
	result, ok := work.Or(left, right)
	if !ok || !work.Seal() {
		work.Discard()
		return Mask{}, false
	}
	return result, true
}

// Empty reports whether mask contains no valuation.
func Empty(mask Mask) bool {
	view, ok := mask.Decompose()
	return ok && view.Terminal && !view.Value
}

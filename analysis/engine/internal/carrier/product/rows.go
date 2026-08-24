// Package product owns support-cell refinement for a projection-scoped
// Product.  Factor observation tuples remain opaque to this component.
package product

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// nextRowsIdentity issues exact publication identities without allocating a
// token object. Copies of a Rows value retain the same identity; each newly
// sealed result receives a distinct one.
var nextRowsIdentity atomic.Uint64

func mintRowsIdentity() uint64 {
	for {
		identity := nextRowsIdentity.Add(1)
		if identity != 0 {
			return identity
		}
	}
}

// Rows owns exact disjoint support cells.  It cannot inspect or accidentally
// partition an unselected Factor observation.
type Rows struct {
	manager *guard.Manager
	// identity is exact producer provenance. Equal support geometry from a
	// different producer is not an interchangeable Rows operand.
	identity uint64
	// first is the inline one-cell representation used by the seed and by the
	// common identity product. Keeping that cell in the value avoids a slice
	// allocation for every invocation's initial support row. cells is retained
	// for genuine multi-cell refinements and for package-internal fixtures that
	// construct Rows directly.
	first support.Mask
	cells []support.Mask
	// declared is the immutable support identity over which this row set was
	// sealed.  It is deliberately retained beside the cells: a product may
	// publish a cross only after it has proved both operands cover the same
	// declared support.
	declared support.Mask
	// seal is an opaque publication witness.  A boolean flag is insufficient:
	// the witness is minted only by NewRows, a successful exact-cover Seal, or
	// CrossWork after it has re-proved every pair intersection. It is embedded
	// by value so the authenticated seed remains allocation-free.
	seal rowsSeal
	// crossOwner/crossGeneration fence Rows backed by CrossWork's reusable
	// buffers. A subsequent Cross invalidates the prior carrier view before its
	// regions are overwritten.
	crossOwner      *CrossWork
	crossGeneration uint64
}

type rowsSeal struct {
	manager  *guard.Manager
	declared support.Mask
	count    int
}

// NewRows starts the one-row Product for a nonempty input-support intersection.
func NewRows(initial support.Mask) (Rows, bool) {
	if !initial.Valid() || support.Empty(initial) {
		return Rows{}, false
	}
	return Rows{
		manager:  initial.Manager(),
		identity: mintRowsIdentity(),
		first:    initial,
		declared: initial,
		seal:     rowsSeal{manager: initial.Manager(), declared: initial, count: 1},
	}, true
}

// Count returns the number of exact Product cells.
func (rows Rows) Count() int {
	if rows.manager == nil || !rows.crossLive() {
		return 0
	}
	if rows.cells != nil {
		return len(rows.cells)
	}
	if rows.first.Valid() && rows.first.Manager() == rows.manager && !support.Empty(rows.first) {
		return 1
	}
	return 0
}

// Valid reports whether rows carry a carrier-issued exact-cover witness.  The
// witness is structural authority; callers cannot promote an arbitrary slice
// of support cells into a cross operand by claiming that a cursor completed.
func (rows Rows) Valid() bool {
	if rows.manager == nil || rows.identity == 0 || !rows.crossLive() || rows.seal.manager != rows.manager ||
		rows.seal.declared != rows.declared || !rows.declared.Valid() ||
		rows.declared.Manager() != rows.manager || rows.seal.count != rows.Count() || rows.Count() == 0 {
		return false
	}
	for index := 0; index < rows.Count(); index++ {
		cell, ok := rows.At(index)
		if !ok || !cell.Valid() || cell.Manager() != rows.manager || support.Empty(cell) {
			return false
		}
	}
	return true
}

// Support returns the declared support identity sealed by this Rows value.
// An unsealed package fixture has no support identity and returns an invalid
// mask; it is consequently ineligible for CrossWork.
func (rows Rows) Support() support.Mask {
	if !rows.crossLive() || rows.seal.manager == nil || !rows.declared.Valid() || rows.seal.declared != rows.declared {
		return support.Mask{}
	}
	return rows.declared
}

// At returns one exact Product cell.
func (rows Rows) At(index int) (support.Mask, bool) {
	if rows.manager == nil || !rows.crossLive() || index < 0 || index >= rows.Count() {
		return support.Mask{}, false
	}
	if rows.cells != nil {
		return rows.cells[index], true
	}
	return rows.first, true
}

func (rows Rows) crossLive() bool {
	return rows.crossOwner == nil || rows.crossOwner.manager != nil && !rows.crossOwner.exhausted && rows.crossOwner.generation == rows.crossGeneration
}

func (rows Rows) cell(index int) (support.Mask, bool) { return rows.At(index) }

func sealedRows(manager *guard.Manager, declared support.Mask, cells []support.Mask) Rows {
	if len(cells) == 1 {
		return Rows{
			manager:  manager,
			identity: mintRowsIdentity(),
			first:    cells[0],
			declared: declared,
			seal:     rowsSeal{manager: manager, declared: declared, count: 1},
		}
	}
	return Rows{
		manager:  manager,
		identity: mintRowsIdentity(),
		cells:    cells,
		declared: declared,
		seal:     rowsSeal{manager: manager, declared: declared, count: len(cells)},
	}
}

// CrossPairs is an opaque source-major mapping for one CrossWork result.  It
// carries only left/right row indices; support geometry stays owned by Rows and
// is re-proved by CrossWork on every call.
type CrossPairs struct {
	pairs      []crossPair
	leftCount  int
	rightCount int
	leftID     uint64
	rightID    uint64
	resultID   uint64
	owner      *CrossWork
	generation uint64
}

type crossPair struct {
	left  int
	right int
}

// Count returns the number of nonempty left/right intersections.
func (pairs CrossPairs) Count() int {
	if pairs.owner != nil && (pairs.owner.manager == nil || pairs.owner.exhausted || pairs.owner.generation != pairs.generation) {
		return 0
	}
	return len(pairs.pairs)
}

// At returns the left and right row indices for one nonempty intersection.
// The mapping is immutable from the caller's perspective and is meaningful
// only with the Rows returned by the same CrossWork call.
func (pairs CrossPairs) At(index int) (left, right int, ok bool) {
	if pairs.owner != nil && (pairs.owner.manager == nil || pairs.owner.exhausted || pairs.owner.generation != pairs.generation) || index < 0 || index >= len(pairs.pairs) {
		return 0, 0, false
	}
	pair := pairs.pairs[index]
	return pair.left, pair.right, true
}

// ValidFor authenticates a pair map against the exact left/right operands and
// result publication that produced it. Generation and cardinality alone are
// deliberately insufficient: same-shaped foreign Rows must be rejected.
func (pairs CrossPairs) ValidFor(left, right, result Rows) bool {
	return pairs.owner != nil && !pairs.owner.exhausted && pairs.owner.manager != nil &&
		pairs.owner.generation == pairs.generation &&
		left.Valid() && right.Valid() && result.Valid() &&
		left.identity == pairs.leftID && right.identity == pairs.rightID && result.identity == pairs.resultID &&
		left.Count() == pairs.leftCount && right.Count() == pairs.rightCount && result.Count() == len(pairs.pairs)
}

// CrossWork owns reusable support construction and pair buffers for one exact
// product crossing. It never accepts caller-computed intersections, offsets, or
// a claimed cached result. A cache miss traverses every left×right pair,
// computes its exact intersections, and re-proves the complete output cover;
// only a same-handle hit may republish that carrier-owned sealed geometry.
type CrossWork struct {
	manager *guard.Manager
	proof   *support.Work
	regions []support.Mask
	pairs   []crossPair
	// cache keys are copied immutable support handles, never caller geometry.
	// A hit republishes the carrier's own previously sealed geometry under a new
	// generation; a miss recomputes and re-proves every pair. This cache stores
	// no typed value or semantic answer.
	cacheLeft    []support.Mask
	cacheRight   []support.Mask
	cacheSupport support.Mask
	cacheValid   bool
	// generation revokes Rows/CrossPairs views before reusable buffers are
	// overwritten by the next call or released by Close.
	generation uint64
	// exhausted is terminal once generation would wrap. A fresh CrossWork is
	// required after that point; resetting generation to zero revokes all views
	// from the exhausted generation without permitting reuse.
	exhausted bool
}

// NewCrossWork opens one reusable cross shell. A zero manager is accepted so a
// caller may also use the zero CrossWork value and let the first Cross bind its
// immutable support manager.
func NewCrossWork(manager *guard.Manager) *CrossWork {
	work := &CrossWork{manager: manager}
	if manager != nil {
		work.proof = support.New(manager)
	}
	return work
}

// Close releases the reusable shell and all retained pair/region capacity.
func (work *CrossWork) Close() {
	if work == nil {
		return
	}
	if work.proof != nil {
		work.proof.Close()
	}
	work.manager = nil
	if work.generation == ^uint64(0) {
		work.generation = 0
		work.exhausted = true
	} else {
		work.generation++
	}
	work.proof = nil
	work.regions = nil
	work.pairs = nil
	work.cacheLeft = nil
	work.cacheRight = nil
	work.cacheSupport = support.Mask{}
	work.cacheValid = false
}

// beginAttempt revokes every prior Rows/CrossPairs publication before the
// first admission check. Malformed operands, cover mismatches, cancellation,
// and generation exhaustion therefore all kill the prior view.
func (work *CrossWork) beginAttempt() bool {
	if work == nil || work.exhausted {
		return false
	}
	if work.generation == ^uint64(0) {
		work.generation = 0
		work.exhausted = true
		work.regions = work.regions[:0]
		work.pairs = work.pairs[:0]
		work.cacheValid = false
		return false
	}
	work.generation++
	return true
}

func (work *CrossWork) ensure(manager *guard.Manager) bool {
	if work == nil || manager == nil {
		return false
	}
	if work.manager != manager {
		if work.proof != nil {
			work.proof.Close()
		}
		work.manager = manager
		work.proof = support.New(manager)
		work.regions = work.regions[:0]
		work.pairs = work.pairs[:0]
		work.cacheLeft = work.cacheLeft[:0]
		work.cacheRight = work.cacheRight[:0]
		work.cacheSupport = support.Mask{}
		work.cacheValid = false
	}
	if work.proof == nil {
		work.manager = manager
		work.proof = support.New(manager)
	}
	return work.proof != nil && work.proof.OwnsManager(manager)
}

func (work *CrossWork) cacheMatches(left, right Rows, declared support.Mask) bool {
	if work == nil || !work.cacheValid || !declared.SameHandle(work.cacheSupport) ||
		len(work.cacheLeft) != left.Count() || len(work.cacheRight) != right.Count() {
		return false
	}
	for index, expected := range work.cacheLeft {
		actual, ok := left.At(index)
		if !ok || !actual.SameHandle(expected) {
			return false
		}
	}
	for index, expected := range work.cacheRight {
		actual, ok := right.At(index)
		if !ok || !actual.SameHandle(expected) {
			return false
		}
	}
	return len(work.regions) == len(work.pairs) && len(work.regions) != 0
}

func (work *CrossWork) rememberCache(left, right Rows, declared support.Mask) bool {
	if work == nil {
		return false
	}
	if cap(work.cacheLeft) < left.Count() {
		work.cacheLeft = make([]support.Mask, left.Count())
	} else {
		work.cacheLeft = work.cacheLeft[:left.Count()]
	}
	if cap(work.cacheRight) < right.Count() {
		work.cacheRight = make([]support.Mask, right.Count())
	} else {
		work.cacheRight = work.cacheRight[:right.Count()]
	}
	for index := range work.cacheLeft {
		cell, ok := left.At(index)
		if !ok {
			return false
		}
		work.cacheLeft[index] = cell
	}
	for index := range work.cacheRight {
		cell, ok := right.At(index)
		if !ok {
			return false
		}
		work.cacheRight[index] = cell
	}
	work.cacheSupport = declared
	work.cacheValid = true
	return true
}

// Cross computes every nonempty left×right intersection internally on a cache
// miss and returns the carrier-sealed output rows plus an opaque pair mapping.
// Both operands must carry a valid carrier seal over the same declared support
// identity. A failed checkpoint or proof clears this call's output buffers
// before return; no partial result or prior semantic answer is published.
func (work *CrossWork) Cross(left, right Rows, checkpoint func() bool) (Rows, CrossPairs, bool) {
	if !work.beginAttempt() {
		return Rows{}, CrossPairs{}, false
	}
	if work == nil || !left.Valid() || !right.Valid() || left.manager == nil ||
		right.manager != left.manager || left.Count() == 0 || right.Count() == 0 {
		return Rows{}, CrossPairs{}, false
	}
	leftSupport, rightSupport := left.Support(), right.Support()
	if !leftSupport.Valid() || !rightSupport.Valid() ||
		leftSupport.Manager() != left.manager || rightSupport.Manager() != left.manager ||
		!(leftSupport.SameHandle(rightSupport) || leftSupport.Equal(rightSupport)) {
		return Rows{}, CrossPairs{}, false
	}
	if checkpoint != nil && !checkpoint() {
		return Rows{}, CrossPairs{}, false
	}
	if !work.ensure(left.manager) {
		return Rows{}, CrossPairs{}, false
	}
	if work.cacheMatches(left, right, leftSupport) {
		if checkpoint != nil && !checkpoint() {
			return Rows{}, CrossPairs{}, false
		}
		result := sealedRows(left.manager, leftSupport, work.regions)
		result.crossOwner = work
		result.crossGeneration = work.generation
		if !result.Valid() {
			return Rows{}, CrossPairs{}, false
		}
		return result, CrossPairs{pairs: work.pairs, leftCount: left.Count(), rightCount: right.Count(), leftID: left.identity, rightID: right.identity, resultID: result.identity, owner: work, generation: work.generation}, true
	}
	work.cacheValid = false
	work.regions = work.regions[:0]
	work.pairs = work.pairs[:0]
	if !work.proof.BeginTransaction(checkpoint) {
		return Rows{}, CrossPairs{}, false
	}
	union := work.proof.False()

	for leftIndex := 0; leftIndex < left.Count(); leftIndex++ {
		leftCell, leftOK := left.At(leftIndex)
		if !leftOK || !leftCell.Valid() || leftCell.Manager() != left.manager || support.Empty(leftCell) {
			work.proof.Discard()
			work.regions = work.regions[:0]
			work.pairs = work.pairs[:0]
			return Rows{}, CrossPairs{}, false
		}
		for rightIndex := 0; rightIndex < right.Count(); rightIndex++ {
			if checkpoint != nil && !checkpoint() {
				work.proof.Discard()
				work.regions = work.regions[:0]
				work.pairs = work.pairs[:0]
				return Rows{}, CrossPairs{}, false
			}
			rightCell, rightOK := right.At(rightIndex)
			if !rightOK || !rightCell.Valid() || rightCell.Manager() != left.manager || support.Empty(rightCell) {
				work.proof.Discard()
				work.regions = work.regions[:0]
				work.pairs = work.pairs[:0]
				return Rows{}, CrossPairs{}, false
			}
			intersection, intersectionOK := work.proof.And(leftCell, rightCell)
			if !intersectionOK {
				work.proof.Discard()
				work.regions = work.regions[:0]
				work.pairs = work.pairs[:0]
				return Rows{}, CrossPairs{}, false
			}
			if work.proof.Empty(intersection) {
				continue
			}
			if !entails(work.proof, intersection, leftSupport) {
				work.proof.Discard()
				work.regions = work.regions[:0]
				work.pairs = work.pairs[:0]
				return Rows{}, CrossPairs{}, false
			}
			overlap, overlapOK := work.proof.And(union, intersection)
			if !overlapOK || !work.proof.Empty(overlap) {
				work.proof.Discard()
				work.regions = work.regions[:0]
				work.pairs = work.pairs[:0]
				return Rows{}, CrossPairs{}, false
			}
			union, overlapOK = work.proof.Or(union, intersection)
			if !overlapOK {
				work.proof.Discard()
				work.regions = work.regions[:0]
				work.pairs = work.pairs[:0]
				return Rows{}, CrossPairs{}, false
			}
			work.regions = append(work.regions, intersection)
			work.pairs = append(work.pairs, crossPair{left: leftIndex, right: rightIndex})
		}
	}
	if len(work.regions) == 0 || len(work.regions) != len(work.pairs) || checkpoint != nil && !checkpoint() ||
		!entails(work.proof, union, leftSupport) || !entails(work.proof, leftSupport, union) || !work.proof.Seal() {
		work.proof.Discard()
		work.regions = work.regions[:0]
		work.pairs = work.pairs[:0]
		return Rows{}, CrossPairs{}, false
	}

	// The single transaction above re-proved nonempty cells, manager ownership,
	// subset, pairwise disjointness, and exact union coverage before publication.
	// No caller-supplied offsets or intersection list can bypass that cut.
	result := sealedRows(left.manager, leftSupport, work.regions)
	result.crossOwner = work
	result.crossGeneration = work.generation
	if !result.Valid() {
		work.regions = work.regions[:0]
		work.pairs = work.pairs[:0]
		return Rows{}, CrossPairs{}, false
	}
	if !work.rememberCache(left, right, leftSupport) {
		work.regions = work.regions[:0]
		work.pairs = work.pairs[:0]
		work.cacheValid = false
		return Rows{}, CrossPairs{}, false
	}
	return result, CrossPairs{pairs: work.pairs, leftCount: left.Count(), rightCount: right.Count(), leftID: left.identity, rightID: right.identity, resultID: result.identity, owner: work, generation: work.generation}, true
}

// PrefixQuotientWithCheckpoint compacts the output of one just-sealed
// refinement without changing the preceding product prefix.  It applies the
// opaque equality only to same-fingerprint output cells from the same source
// range, retains the first matching output index in source-major order, and
// unions the exact support of every resulting class. Fingerprints are only a
// comparison index: equality resolves every collision and remains semantic
// authority.
//
// sources must be the complete SourceRows returned alongside rows by Seal.
// Invalid or incomplete source ranges, cancellation, and failed BDD work all
// fail closed: no partially compacted Rows or representative indices escape.
// The representatives name output indices in rows, rather than source rows.
func (rows Rows) PrefixQuotientWithCheckpoint(sources SourceRows, checkpoint func() bool, fingerprint func(int) (uint64, bool), equal func(int, int) bool) (Rows, []int, bool) {
	if fingerprint == nil || equal == nil || !rows.Valid() || !rows.validPrefixQuotientSources(sources, checkpoint) {
		return Rows{}, nil, false
	}

	count := rows.Count()
	merged := make([]support.Mask, 0, count)
	representatives := make([]int, 0, count)
	previous := make([]int, 0, count)
	buckets := make(map[uint64]int)
	var unionWork *support.Work
	published := false
	defer func() {
		if !published && unionWork != nil {
			unionWork.Discard()
		}
	}()
	for source := 0; source < sources.SourceCount(); source++ {
		if checkpoint != nil && !checkpoint() {
			return Rows{}, nil, false
		}
		start, end, ok := sources.Range(source)
		if !ok || start >= end {
			return Rows{}, nil, false
		}

		clear(buckets)
		for index := start; index < end; index++ {
			if checkpoint != nil && !checkpoint() {
				return Rows{}, nil, false
			}
			hash, ok := fingerprint(index)
			if !ok || checkpoint != nil && !checkpoint() {
				return Rows{}, nil, false
			}
			found := -1
			candidate, present := buckets[hash]
			for present {
				if checkpoint != nil && !checkpoint() {
					return Rows{}, nil, false
				}
				if equal(index, representatives[candidate]) {
					if checkpoint != nil && !checkpoint() {
						return Rows{}, nil, false
					}
					found = candidate
					break
				}
				if checkpoint != nil && !checkpoint() {
					return Rows{}, nil, false
				}
				candidate = previous[candidate]
				present = candidate >= 0
			}
			if found < 0 {
				cell, cellOK := rows.At(index)
				if !cellOK {
					return Rows{}, nil, false
				}
				merged = append(merged, cell)
				representatives = append(representatives, index)
				prior := -1
				if candidate, exists := buckets[hash]; exists {
					prior = candidate
				}
				previous = append(previous, prior)
				buckets[hash] = len(representatives) - 1
				continue
			}
			if unionWork == nil {
				unionWork = support.New(rows.manager)
				if unionWork == nil || (checkpoint != nil && !unionWork.SetCheckpoint(checkpoint)) {
					return Rows{}, nil, false
				}
			}
			cell, cellOK := rows.At(index)
			if !cellOK {
				return Rows{}, nil, false
			}
			next, ok := unionWork.Or(merged[found], cell)
			if !ok || !unionWork.Valid(next) {
				return Rows{}, nil, false
			}
			merged[found] = next
		}
	}
	if unionWork != nil && !unionWork.Seal() {
		return Rows{}, nil, false
	}
	if !rows.declared.Valid() || rows.seal.manager != rows.manager {
		return Rows{}, nil, false
	}
	published = true
	return sealedRows(rows.manager, rows.declared, merged), representatives, true
}

// validPrefixQuotientSources checks the complete, canonical source-major map
// before equality observes any output index.  A source range for a sealed
// refinement is never empty, and together the ranges cover every cell once.
func (rows Rows) validPrefixQuotientSources(sources SourceRows, checkpoint func() bool) bool {
	if rows.manager == nil || !rows.Valid() || rows.Count() == 0 || sources.sourceID == 0 || sources.rowsID != rows.identity {
		return false
	}
	for index := 0; index < rows.Count(); index++ {
		cell, cellOK := rows.At(index)
		if !cellOK {
			return false
		}
		if checkpoint != nil && !checkpoint() {
			return false
		}
		if !cell.Valid() || cell.Manager() != rows.manager || support.Empty(cell) {
			return false
		}
	}

	if sources.identity != 0 {
		return sources.identity == rows.Count() && len(sources.offsets) == 0
	}
	if len(sources.offsets) < 2 || sources.offsets[0] != 0 {
		return false
	}
	previous := 0
	for source := 0; source+1 < len(sources.offsets); source++ {
		if checkpoint != nil && !checkpoint() {
			return false
		}
		start, end := sources.offsets[source], sources.offsets[source+1]
		if start != previous || start < 0 || end <= start || end > rows.Count() {
			return false
		}
		previous = end
	}
	return previous == rows.Count()
}

// BeginRefine starts one Factor-owned unit observation pass.
func (rows Rows) BeginRefine() *Refinement {
	if !rows.refinementReady() {
		return nil
	}
	return &Refinement{source: rows, identity: true, active: -1}
}

// BeginRefineWithCheckpoint binds one opaque evaluator liveness probe to the
// disposable refinement. The probe affects only whether the candidate
// completes; source/output rows retain their normal exact identities.
func (rows Rows) BeginRefineWithCheckpoint(checkpoint func() bool) *Refinement {
	if !rows.refinementReady() {
		return nil
	}
	return &Refinement{source: rows, identity: true, active: -1, checkpoint: checkpoint}
}

// BeginRefineWithWork starts a refinement with one caller-owned reusable proof
// shell.  A non-identity Seal publishes the shell, retaining its map capacity
// for the next transaction; a failed/cancelled Seal discards it.  The shell is
// never used as a semantic answer cache: every refinement re-proves its own
// nonempty, ownership, disjointness, subset, and union obligations.
func (rows Rows) BeginRefineWithWork(proof *support.Work, checkpoint func() bool) *Refinement {
	if !rows.refinementReady() {
		return nil
	}
	return &Refinement{source: rows, proof: proof, identity: true, active: -1, checkpoint: checkpoint}
}

// refinementReady is the small admission predicate kept on the hot Begin
// path. Full support geometry/authentication remains in Seal; this predicate
// only prevents the zero/unsealed fixture shape from entering a refinement.
func (rows Rows) refinementReady() bool {
	return rows.manager != nil && rows.identity != 0 && rows.seal.manager == rows.manager && rows.seal.count > 0
}

// Refinement is a candidate replacement for one Rows partition pass.
type Refinement struct {
	source     Rows
	checkpoint func() bool
	// proof is an optional owner-supplied reusable exact-cover shell.  It is
	// sealed on success and discarded on failure, so a later refinement can
	// reopen the same BDD work without retaining a prior semantic result.
	proof *support.Work

	// identity remains true while every started source emitted exactly one
	// semantically equal replacement. This needs neither a candidate BDD
	// transaction nor duplicate output/range buffers. The direct-handle branch
	// is allocation-free; cross-generation Equal is still accepted as the
	// semantic authority and may use read-only comparison scratch. The first
	// non-identity or second piece materializes the general representation and
	// its single exact-cover proof transaction.
	identity bool

	// pieces is one source-major buffer.  A Factor must emit its canonical
	// pieces for source 0, then source 1, and so on; product preserves the
	// Factor-owned order within every source and never reorders it.
	pieces  []support.Mask
	offsets []int
	active  int

	// validation owns the only candidate BDD work for a non-identity exact-cover
	// proof. It is allocated lazily when the identity law no longer applies. A
	// successful transaction is sealed (for warm capacity reuse); a failed one
	// is discarded before any Rows can escape.
	validation *support.Work

	closed bool
}

func (refinement *Refinement) live() bool {
	return refinement != nil && !refinement.closed && (refinement.checkpoint == nil || refinement.checkpoint())
}

// Add records one nonempty Factor piece in canonical source-major order.
//
// Add performs admission checks plus a read-only identity comparison while
// that allocation-free fast path remains possible. In particular, arbitrary
// parent inclusion is part of Seal's single-work exact-cover proof, not a
// per-piece candidate construction or Manager.Entails traversal. Once Add
// advances to the next source, the preceding source cannot receive more
// pieces.
func (refinement *Refinement) Add(source int, piece support.Mask) bool {
	if !refinement.live() || source < 0 || source >= refinement.source.Count() ||
		!piece.Valid() || piece.Manager() != refinement.source.manager || support.Empty(piece) {
		return false
	}
	if source != refinement.active && source != refinement.active+1 {
		return false
	}
	if refinement.identity {
		parent, parentOK := refinement.source.At(source)
		if source == refinement.active+1 && parentOK && sameMask(piece, parent) {
			refinement.active = source
			return true
		}
		if !refinement.materialize() {
			return false
		}
	}
	if source == refinement.active+1 {
		refinement.offsets[source] = len(refinement.pieces)
		refinement.active = source
	}
	refinement.pieces = append(refinement.pieces, piece)
	return true
}

// SourceRows maps each output cell from one sealed Refinement to the exact
// input row it refined.  Its compact source-major ranges are sufficient for
// an executor to extend every output row's opaque observation tuple without a
// redundant source index for every output piece.  Observations and every
// Factor-owned terminal stay outside product.
//
// The map is deliberately produced with Rows, rather than retained by Rows.
// Its source indices are meaningful only relative to the particular input
// Rows supplied to BeginRefine.
type SourceRows struct {
	// sourceID is the exact input Rows supplied to BeginRefine; rowsID is the
	// exact output Rows published by that refinement. Both identities are
	// required so a same-shaped map from another producer cannot be replayed.
	sourceID uint64
	rowsID   uint64

	// offsets[source:source+1] is the half-open output-piece range for source.
	// A sealed refinement has one nonempty range per input source.
	offsets []int

	// identity is a no-allocation compact representation for an unchanged
	// source-major row set: output piece i came from source i.  It is private so
	// callers still observe only Source/Range, never an alternate mapping API.
	identity int
}

// Count returns the number of output cells covered by this map.
func (sources SourceRows) Count() int {
	if sources.identity != 0 {
		return sources.identity
	}
	if len(sources.offsets) == 0 {
		return 0
	}
	return sources.offsets[len(sources.offsets)-1]
}

// SourceCount returns the number of input rows represented by the ranges.
func (sources SourceRows) SourceCount() int {
	if sources.identity != 0 {
		return sources.identity
	}
	if len(sources.offsets) == 0 {
		return 0
	}
	return len(sources.offsets) - 1
}

// Range returns the half-open output-piece range emitted by source.  The
// output order is source-major, so callers can copy each source row's opaque
// observation tuple over this range without asking Source for every piece.
func (sources SourceRows) Range(source int) (start, end int, ok bool) {
	if sources.identity != 0 {
		if source < 0 || source >= sources.identity {
			return 0, 0, false
		}
		return source, source + 1, true
	}
	if source < 0 || source >= sources.SourceCount() {
		return 0, 0, false
	}
	return sources.offsets[source], sources.offsets[source+1], true
}

// Source returns the input row that produced output cell piece.
func (sources SourceRows) Source(piece int) (int, bool) {
	if sources.identity != 0 {
		if piece < 0 || piece >= sources.identity {
			return 0, false
		}
		return piece, true
	}
	if piece < 0 || piece >= sources.Count() {
		return 0, false
	}
	// Find the first range end greater than piece.  The binary search also
	// remains correct for a future sparse range producer.
	low, high := 1, len(sources.offsets)
	for low < high {
		middle := low + (high-low)/2
		if sources.offsets[middle] <= piece {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low - 1, true
}

// Seal proves each source row was partitioned exactly and transfers the
// source-major cell buffer plus compact source ranges to its result.  Opaque
// observation equality owns any row merge.
func (refinement *Refinement) Seal() (Rows, SourceRows, bool) {
	if refinement == nil || refinement.closed {
		return Rows{}, SourceRows{}, false
	}
	if refinement.checkpoint != nil && !refinement.checkpoint() {
		refinement.closed = true
		refinement.discard()
		return Rows{}, SourceRows{}, false
	}
	refinement.closed = true
	if !refinement.source.Valid() || refinement.source.manager == nil ||
		refinement.active != refinement.source.Count()-1 {
		refinement.discard()
		return Rows{}, SourceRows{}, false
	}
	if refinement.identity {
		result := refinement.source
		refinement.discard()
		return result, SourceRows{identity: result.Count(), sourceID: result.identity, rowsID: result.identity}, true
	}
	if refinement.validation == nil {
		refinement.discard()
		return Rows{}, SourceRows{}, false
	}
	count := refinement.source.Count()
	refinement.offsets[count] = len(refinement.pieces)
	for source := 0; source < count; source++ {
		if refinement.checkpoint != nil && !refinement.checkpoint() {
			refinement.discard()
			return Rows{}, SourceRows{}, false
		}
		start, end := refinement.offsets[source], refinement.offsets[source+1]
		parent, parentOK := refinement.source.At(source)
		if !parentOK {
			refinement.discard()
			return Rows{}, SourceRows{}, false
		}
		if start == end || !exactPartition(refinement.validation, parent, refinement.pieces[start:end]) {
			refinement.discard()
			return Rows{}, SourceRows{}, false
		}
	}

	// Publish the complete proof before transferring the output.  Seal retains
	// reusable map capacity while the immutable output masks remain the sole
	// semantic result.  A cancelled or failed proof never reaches this cut.
	if !refinement.validation.Seal() {
		refinement.discard()
		return Rows{}, SourceRows{}, false
	}
	manager := refinement.source.manager
	pieces, offsets := refinement.pieces, refinement.offsets
	declared := refinement.source.declared
	sourceID := refinement.source.identity
	if !declared.Valid() || sourceID == 0 {
		refinement.discard()
		return Rows{}, SourceRows{}, false
	}
	refinement.source = Rows{}
	// pieces and offsets are transferred to the result, but remain owned by
	// this linear Refinement so a later Begin...Into can reuse their capacity.
	// Product's generation fence invalidates the prior typed result before this
	// owner writes those buffers again.
	refinement.active = -1
	refinement.identity = false
	refinement.checkpoint = nil
	result := sealedRows(manager, declared, pieces)
	return result, SourceRows{offsets: offsets, sourceID: sourceID, rowsID: result.identity}, true
}

// discard releases every retained predecessor and candidate buffer after a
// failed terminal Seal.  A successful Seal transfers pieces and offsets first.
func (refinement *Refinement) discard() {
	if refinement.validation != nil {
		refinement.validation.Discard()
	}
	refinement.source = Rows{}
	if refinement.pieces != nil {
		refinement.pieces = refinement.pieces[:0]
	}
	if refinement.offsets != nil {
		refinement.offsets = refinement.offsets[:0]
	}
	refinement.identity = false
	refinement.active = -1
	refinement.checkpoint = nil
}

// materialize turns the prefix of one-for-one identity emissions into the
// source-major buffers used by a real refinement. The next Add completes the
// transition. It is deliberately the only place that allocates proof work.
func (refinement *Refinement) materialize() bool {
	if !refinement.live() || !refinement.identity || refinement.source.manager == nil {
		return false
	}
	validation := refinement.validation
	if validation == nil {
		validation = refinement.proof
		if validation == nil {
			validation = support.New(refinement.source.manager)
		}
		if validation == nil {
			return false
		}
		// BeginTransaction reopens a previously sealed/discarded reusable Work,
		// while also claiming a fresh Work returned by support.New.  It is the
		// only proof transaction this refinement is allowed to own.
		if !validation.BeginTransaction(refinement.checkpoint) {
			return false
		}
	}
	count := refinement.active + 1
	sourceCount := refinement.source.Count()
	if cap(refinement.pieces) < sourceCount {
		refinement.pieces = make([]support.Mask, 0, sourceCount)
	}
	pieces := refinement.pieces[:count]
	for source := 0; source < count; source++ {
		piece, ok := refinement.source.At(source)
		if !ok {
			validation.Discard()
			return false
		}
		pieces[source] = piece
	}
	if cap(refinement.offsets) < sourceCount+1 {
		refinement.offsets = make([]int, 0, sourceCount+1)
	}
	offsets := refinement.offsets[:sourceCount+1]
	for source := 0; source < count; source++ {
		offsets[source] = source
	}
	refinement.identity = false
	refinement.pieces = pieces
	refinement.offsets = offsets
	refinement.validation = validation
	return true
}

// sameMask keeps the existing identity-refinement theorem: an identical
// support handle takes the no-work fast path, while a semantically equal mask
// from a later BDD generation is also an admissible identity replacement.
// The latter leaves the identity path through materialize, where the ordinary
// exact-cover proof remains the publication authority.
func sameMask(left, right support.Mask) bool {
	return left.SameHandle(right) || left.Equal(right)
}

func exactPartition(work *support.Work, parent support.Mask, pieces []support.Mask) bool {
	if work == nil || !work.Valid(parent) {
		return false
	}
	union := work.False()
	for _, piece := range pieces {
		if !work.Valid(piece) || !entails(work, piece, parent) {
			return false
		}
		overlap, ok := work.And(union, piece)
		if !ok || !empty(work, overlap) {
			return false
		}
		union, ok = work.Or(union, piece)
		if !ok {
			return false
		}
	}
	return entails(work, union, parent) && entails(work, parent, union)
}

func empty(work *support.Work, mask support.Mask) bool {
	view, ok := work.Decompose(mask)
	return ok && view.Terminal && !view.Value
}

func entails(work *support.Work, premise, conclusion support.Mask) bool {
	return premise.SameHandle(conclusion) || work.Entails(premise, conclusion)
}

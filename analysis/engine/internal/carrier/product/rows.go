// Package product owns support-cell refinement for a projection-scoped
// Product.  Factor observation tuples remain opaque to this component.
package product

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// Rows owns exact disjoint support cells.  It cannot inspect or accidentally
// partition an unselected Factor observation.
type Rows struct {
	manager *guard.Manager
	cells   []support.Mask
}

// NewRows starts the one-row Product for a nonempty input-support intersection.
func NewRows(initial support.Mask) (Rows, bool) {
	if !initial.Valid() || support.Empty(initial) {
		return Rows{}, false
	}
	return Rows{manager: initial.Manager(), cells: []support.Mask{initial}}, true
}

// Count returns the number of exact Product cells.
func (rows Rows) Count() int {
	if rows.manager == nil {
		return 0
	}
	return len(rows.cells)
}

// At returns one exact Product cell.
func (rows Rows) At(index int) (support.Mask, bool) {
	if rows.manager == nil || index < 0 || index >= len(rows.cells) {
		return support.Mask{}, false
	}
	return rows.cells[index], true
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
	if fingerprint == nil || equal == nil || !rows.validPrefixQuotientSources(sources, checkpoint) {
		return Rows{}, nil, false
	}

	merged := make([]support.Mask, 0, len(rows.cells))
	representatives := make([]int, 0, len(rows.cells))
	previous := make([]int, 0, len(rows.cells))
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
				merged = append(merged, rows.cells[index])
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
			next, ok := unionWork.Or(merged[found], rows.cells[index])
			if !ok || !unionWork.Valid(next) {
				return Rows{}, nil, false
			}
			merged[found] = next
		}
	}
	if unionWork != nil && !unionWork.Seal() {
		return Rows{}, nil, false
	}
	published = true
	return Rows{manager: rows.manager, cells: merged}, representatives, true
}

// validPrefixQuotientSources checks the complete, canonical source-major map
// before equality observes any output index.  A source range for a sealed
// refinement is never empty, and together the ranges cover every cell once.
func (rows Rows) validPrefixQuotientSources(sources SourceRows, checkpoint func() bool) bool {
	if rows.manager == nil || len(rows.cells) == 0 {
		return false
	}
	for _, cell := range rows.cells {
		if checkpoint != nil && !checkpoint() {
			return false
		}
		if !cell.Valid() || cell.Manager() != rows.manager || support.Empty(cell) {
			return false
		}
	}

	if sources.identity != 0 {
		return sources.identity == len(rows.cells) && len(sources.offsets) == 0
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
		if start != previous || start < 0 || end <= start || end > len(rows.cells) {
			return false
		}
		previous = end
	}
	return previous == len(rows.cells)
}

// BeginRefine starts one Factor-owned unit observation pass.
func (rows Rows) BeginRefine() *Refinement {
	return rows.BeginRefineWithCheckpoint(nil)
}

// BeginRefineWithCheckpoint binds one opaque evaluator liveness probe to the
// disposable refinement. The probe affects only whether the candidate
// completes; source/output rows retain their normal exact identities.
func (rows Rows) BeginRefineWithCheckpoint(checkpoint func() bool) *Refinement {
	if rows.manager == nil || len(rows.cells) == 0 {
		return nil
	}
	return &Refinement{
		source:     rows,
		identity:   true,
		active:     -1,
		checkpoint: checkpoint,
	}
}

// Refinement is a candidate replacement for one Rows partition pass.
type Refinement struct {
	source     Rows
	checkpoint func() bool

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
	// proof. It is allocated lazily when the identity law no longer applies,
	// then discarded after either terminal outcome.
	validation *support.Work
	closed     bool
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
	if !refinement.live() || source < 0 || source >= len(refinement.source.cells) ||
		!piece.Valid() || piece.Manager() != refinement.source.manager || support.Empty(piece) {
		return false
	}
	if source != refinement.active && source != refinement.active+1 {
		return false
	}
	if refinement.identity {
		if source == refinement.active+1 && sameMask(piece, refinement.source.cells[source]) {
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
	if !refinement.live() {
		return Rows{}, SourceRows{}, false
	}
	refinement.closed = true
	if refinement.source.manager == nil ||
		refinement.active != len(refinement.source.cells)-1 {
		refinement.discard()
		return Rows{}, SourceRows{}, false
	}
	if refinement.identity {
		result := refinement.source
		refinement.discard()
		return result, SourceRows{identity: len(result.cells)}, true
	}
	if refinement.validation == nil {
		refinement.discard()
		return Rows{}, SourceRows{}, false
	}
	refinement.offsets[len(refinement.source.cells)] = len(refinement.pieces)
	for source := range refinement.source.cells {
		// Seal closes this single-use candidate before validating it, so that a
		// second caller cannot publish the same rows.  From this point its
		// liveness is the opaque epoch probe alone; live() also rejects closed
		// candidates and therefore must not be used here.
		if refinement.checkpoint != nil && !refinement.checkpoint() {
			refinement.discard()
			return Rows{}, SourceRows{}, false
		}
		start, end := refinement.offsets[source], refinement.offsets[source+1]
		parent := refinement.source.cells[source]
		if start == end || !exactPartition(refinement.validation, parent, refinement.pieces[start:end]) {
			refinement.discard()
			return Rows{}, SourceRows{}, false
		}
	}

	// exactPartition built only disposable proof candidates.  Outputs are the
	// already-sealed Factor masks admitted by Add, so publishing the work would
	// retain pages that no output can reference.
	refinement.validation.Discard()
	manager := refinement.source.manager
	pieces, offsets := refinement.pieces, refinement.offsets
	refinement.source = Rows{}
	refinement.pieces = nil
	refinement.offsets = nil
	refinement.validation = nil
	refinement.active = -1
	return Rows{manager: manager, cells: pieces}, SourceRows{offsets: offsets}, true
}

// discard releases every retained predecessor and candidate buffer after a
// failed terminal Seal.  A successful Seal transfers pieces and offsets first.
func (refinement *Refinement) discard() {
	if refinement.validation != nil {
		refinement.validation.Discard()
	}
	refinement.source = Rows{}
	refinement.pieces = nil
	refinement.offsets = nil
	refinement.validation = nil
	refinement.identity = false
	refinement.active = -1
}

// materialize turns the prefix of one-for-one identity emissions into the
// source-major buffers used by a real refinement. The next Add completes the
// transition. It is deliberately the only place that allocates proof work.
func (refinement *Refinement) materialize() bool {
	if !refinement.live() || !refinement.identity || refinement.source.manager == nil {
		return false
	}
	validation := support.New(refinement.source.manager)
	if validation == nil {
		return false
	}
	if refinement.checkpoint != nil && !validation.SetCheckpoint(refinement.checkpoint) {
		validation.Discard()
		return false
	}
	count := refinement.active + 1
	pieces := make([]support.Mask, count, len(refinement.source.cells))
	copy(pieces, refinement.source.cells[:count])
	offsets := make([]int, len(refinement.source.cells)+1)
	for source := 0; source < count; source++ {
		offsets[source] = source
	}
	refinement.identity = false
	refinement.pieces = pieces
	refinement.offsets = offsets
	refinement.validation = validation
	return true
}

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

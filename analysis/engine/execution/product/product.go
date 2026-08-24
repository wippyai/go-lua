// Package product owns composable common refinement of exact reads.
//
// Each Extender observes one canonical exact partition under every current
// product row, then uses carrier/product's exact-cover proof to publish the
// next typed Rows value. A later extender consumes that Rows value and conses
// its Cell onto the typed tail; no arity-specific product or local mask
// algebra is involved.
package product

import (
	engineexecution "github.com/wippyai/go-lua/analysis/engine/execution"
	carrierproduct "github.com/wippyai/go-lua/analysis/engine/internal/carrier/product"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// RefineStatus distinguishes a valid empty Product from a structural cursor
// or partition failure. Sparse absence is retained by Cell and is never a
// status.
type RefineStatus uint8

const (
	RefineRefuse RefineStatus = iota
	RefineAvailable
	RefineEmpty
)

// Cell is one typed exact observation. Value remains available for sparse
// absence, while Present says whether the owner stored a value in this cell.
type Cell[V any] struct {
	value   V
	present bool
}

// Read returns the typed value and its owner-issued sparse presence bit.
func (cell Cell[V]) Read() (V, bool) { return cell.value, cell.present }

// Value returns the typed value without changing the separate presence bit.
func (cell Cell[V]) Value() V { return cell.value }

// Present reports whether the current exact cell contains a stored value.
func (cell Cell[V]) Present() bool { return cell.present }

// Cons is the typed product tuple constructor. A chain of extenders creates
// Cons[Cell[Vn], Cons[Cell[Vn-1], ...struct{}]] without interfaces or erased
// payloads.
type Cons[H, T any] struct {
	head H
	tail T
}

// Head returns the newest typed product cell.
func (cons Cons[H, T]) Head() H { return cons.head }

// Tail returns the preceding typed product tuple.
func (cons Cons[H, T]) Tail() T { return cons.tail }

// Rows is a completed source-major exact product with one typed tuple per
// support cell. It is valid until the Extender that produced it is refined
// again; callers consume it synchronously before that next use.
type Rows[T any] struct {
	product carrierproduct.Rows
	values  []T
	sealed  bool
	// empty is the constructor-issued empty-seed witness. A zero carrier Rows
	// has no geometry seal, so empty support must carry an explicit product
	// token rather than being inferred from Count()==0.
	empty bool
	// lease authenticates the Ticket that issued these rows.  Product rows are
	// ephemeral execution views: a closed Ticket must revoke both the support
	// cells and their typed tuples, even when the caller retained the value.
	lease rowsLease
	// owner/generation fence the Extender-owned tuple buffer.  A subsequent
	// refinement reuses that buffer, so an older Rows value must refuse rather
	// than observe overwritten tuples.
	owner      *uint64
	generation uint64
}

type rowsLease struct {
	frame  engineexecution.Frame
	ticket engineexecution.Ticket
}

func (lease rowsLease) valid(current engineexecution.Ticket) bool {
	return lease.frame.Valid(lease.ticket) && lease.frame.Valid(current)
}

func (rows Rows[T]) live() bool {
	if !rows.sealed || !rows.lease.valid(rows.lease.ticket) {
		return false
	}
	return rows.owner == nil || rows.generation != 0 && *rows.owner == rows.generation
}

func (rows Rows[T]) liveFor(ticket engineexecution.Ticket) bool {
	if !rows.live() || !rows.lease.valid(ticket) {
		return false
	}
	return true
}

// Count returns the number of exact product cells.
func (rows Rows[T]) Count() int {
	if !rows.live() {
		return 0
	}
	if rows.product.Count() != len(rows.values) {
		return 0
	}
	return len(rows.values)
}

// At returns one support cell and its typed tuple. The support.Mask return is
// intentionally consumed through inference by domain callers; support
// authority remains inside execution and carrier.
func (rows Rows[T]) At(index int) (support.Mask, T, bool) {
	if !rows.live() {
		var zero T
		return support.Mask{}, zero, false
	}
	region, regionOK := rows.product.At(index)
	if !regionOK || index < 0 || index >= len(rows.values) {
		var zero T
		return support.Mask{}, zero, false
	}
	return region, rows.values[index], true
}

// Valid reports that the Rows value was sealed by this product package and
// that its typed tuple count agrees with the canonical product. An
// authenticated empty Rows is valid; the zero value remains unsealed.
func (rows Rows[T]) Valid() bool {
	if !rows.live() {
		return false
	}
	if rows.empty {
		return len(rows.values) == 0 && rows.product.Count() == 0
	}
	return rows.product.Valid() && rows.product.Count() == len(rows.values)
}

// Seed is the authenticated one-row empty tuple product. NewSeed obtains its
// support from the live Ticket, so callers cannot substitute a support mask.
type Seed struct{ rows Rows[struct{}] }

// NewSeed starts the canonical product at the Ticket's authenticated support.
// Empty support is a valid RefineEmpty result and never a structural refuse.
func NewSeed(ticket engineexecution.Ticket) (Seed, RefineStatus, bool) {
	within, withinOK := ticket.Within()
	frame, frameOK := engineexecution.NewFrame(ticket)
	if !ticket.Valid() || !ticket.Checkpoint() || !withinOK || !within.Valid() || !frameOK {
		return Seed{}, RefineRefuse, false
	}
	lease := rowsLease{frame: frame, ticket: ticket}
	if support.Empty(within) {
		return Seed{rows: Rows[struct{}]{sealed: true, empty: true, lease: lease}}, RefineEmpty, true
	}
	productRows, rowsOK := carrierproduct.NewRows(within)
	if !rowsOK {
		return Seed{}, RefineRefuse, false
	}
	return Seed{rows: Rows[struct{}]{product: productRows, values: []struct{}{{}}, sealed: true, lease: lease}}, RefineAvailable, true
}

// Rows exposes the seed's typed empty tuple rows to the first Extender.
func (seed Seed) Rows() Rows[struct{}] { return seed.rows }

// Valid reports whether the seed carries an authenticated canonical product;
// this includes the explicitly authenticated empty support.
func (seed Seed) Valid() bool { return seed.rows.Valid() }

// Extender appends one exact-read Cell to an existing typed product tuple.
// It owns only reusable cell, region, and tuple buffers; the sealed read
// descriptor is supplied to Extend and is never retained in product state.
type Extender[K scalar.Key, V any, T any] struct {
	rows       []Cons[Cell[V], T]
	ticket     engineexecution.Ticket
	checkpoint func() bool
	// readRegions/readCells are one ticket-wide canonical read partition. The
	// partition is drained once, then each source row intersects it in source
	// order. This keeps the product from reopening the same ExactRead under
	// every source region while retaining the read's sparse presence bits.
	readRegions []support.Mask
	readCells   []Cell[V]
	// coverProof is the reusable carrier-owned exact-cover transaction used to
	// seal the read cursor's cells over the authenticated Ticket support.
	coverProof *support.Work
	// readCache retains only carrier-sealed geometry from an exact handle-key
	// hit. Typed values are never cached here; every Extend drains the supplied
	// ExactRead and rematerializes its current Cells against the pair map.
	readCache        carrierproduct.Rows
	readCacheRegions []support.Mask
	readCacheSupport support.Mask
	readCacheValid   bool
	// cross owns reusable pair and intersection capacity, but no semantic answer
	// cache. A handle-key geometry hit only republishes carrier-owned geometry;
	// a miss computes every left×right intersection and obtains a fresh opaque
	// pair map from the carrier authority.
	cross      carrierproduct.CrossWork
	generation uint64
	// exhausted is terminal once generation would wrap. A fresh Extender is
	// required after that point; the overflow attempt still revokes prior rows.
	exhausted bool
}

func (extender *Extender[K, V, T]) live() bool {
	return extender != nil && !extender.exhausted && extender.ticket.Checkpoint()
}

// beginAttempt revokes the prior typed Rows generation before any input,
// ticket, read, or scratch admission check. This keeps every failed Extend
// from leaving a previously published view alive.
func (extender *Extender[K, V, T]) beginAttempt() bool {
	if extender == nil || extender.exhausted {
		return false
	}
	if extender.generation == ^uint64(0) {
		extender.generation = 0
		extender.exhausted = true
		extender.rows = extender.rows[:0]
		return false
	}
	extender.generation++
	extender.rows = extender.rows[:0]
	return true
}

// Extend drains the ExactRead's ticket-wide rows once, seals those rows over
// the authenticated Ticket support, then asks carrier/product to re-prove the
// complete left×right cross. Typed values are retained only alongside the
// opaque pair map; no caller-provided geometry enters the carrier.
func (extender *Extender[K, V, T]) Extend(
	ticket engineexecution.Ticket,
	input Rows[T],
	read engineexecution.ExactRead[K, V],
	scratch *engineexecution.Scratch[K, V],
) (output Rows[Cons[Cell[V], T]], status RefineStatus, ok bool) {
	if !extender.beginAttempt() {
		return Rows[Cons[Cell[V], T]]{}, RefineRefuse, false
	}
	accepted := false
	var within support.Mask
	var readRows, productRows carrierproduct.Rows
	var pairs carrierproduct.CrossPairs
	var crossed, withinOK, readOK, drained, emptyRead bool
	if !ticket.Valid() || !ticket.Checkpoint() || !input.Valid() || !input.liveFor(ticket) || !read.Valid() || scratch == nil {
		goto finish
	}
	// Recover a caller that closed an ExactRead before this attempt. A fresh
	// zero scratch simply refuses Reuse and proceeds through Read's own reset.
	_ = scratch.Reuse(ticket)
	extender.ticket = ticket
	if extender.checkpoint == nil {
		extender.checkpoint = extender.live
	}
	extender.rows = extender.rows[:0]
	drained, emptyRead = extender.drain(ticket, read, scratch)
	if !drained {
		goto finish
	}
	if emptyRead {
		// Empty is a valid authenticated result only for the authenticated
		// empty seed. The read was still opened, exhausted, closed, and scratch
		// reset above, so a foreign read/nil scratch cannot bypass admission.
		if !input.empty || input.Count() != 0 || input.product.Count() != 0 {
			goto finish
		}
		accepted = true
		status = RefineEmpty
		ok = true
		goto finish
	}
	within, withinOK = ticket.Within()
	if !withinOK {
		goto finish
	}
	readRows, readOK = extender.sealRead(ticket, within)
	if !readOK {
		goto finish
	}
	productRows, pairs, crossed = extender.cross.Cross(input.product, readRows, extender.checkpoint)
	if !crossed || productRows.Count() != pairs.Count() {
		goto finish
	}
	if !extender.materializeRows(ticket, input, readRows, productRows, pairs) {
		goto finish
	}
	if len(extender.rows) != pairs.Count() {
		goto finish
	}
	accepted = true
	output = Rows[Cons[Cell[V], T]]{product: productRows, values: extender.rows, sealed: true, lease: input.lease, owner: &extender.generation, generation: extender.generation}
	status = RefineAvailable
	ok = true

finish:
	if !accepted && ticket.Valid() && scratch != nil {
		// ExactRead.Close leaves the lane reusable but not reset. Reuse is the
		// deterministic recovery cut after cancellation; a foreign or active
		// scratch refuses and therefore remains terminally rejected.
		_ = scratch.Reuse(ticket)
	}
	if !ok {
		status = RefineRefuse
	}
	return
}

// sealRead asks carrier/product to re-prove the complete read cover over W.
// ExactRead.Close only closes the cursor; it is not geometric authority. The
// declared Ticket support, every nonempty cell, subset, pairwise disjointness,
// and exact union are all checked by Refinement.Seal using coverProof.
func (extender *Extender[K, V, T]) sealRead(ticket engineexecution.Ticket, declared support.Mask) (carrierproduct.Rows, bool) {
	if extender == nil || !ticket.Checkpoint() || !declared.Valid() || len(extender.readRegions) == 0 {
		return carrierproduct.Rows{}, false
	}
	manager := declared.Manager()
	if extender.coverProof == nil || !extender.coverProof.OwnsManager(manager) {
		if extender.coverProof != nil {
			extender.coverProof.Close()
		}
		extender.coverProof = support.New(manager)
	}
	if extender.coverProof == nil {
		return carrierproduct.Rows{}, false
	}
	if extender.readCacheValid && declared.SameHandle(extender.readCacheSupport) && len(extender.readCacheRegions) == len(extender.readRegions) {
		match := true
		for index, expected := range extender.readCacheRegions {
			if !expected.SameHandle(extender.readRegions[index]) {
				match = false
				break
			}
		}
		if match {
			if !ticket.Checkpoint() {
				return carrierproduct.Rows{}, false
			}
			if extender.readCache.Valid() {
				return extender.readCache, true
			}
			extender.readCacheValid = false
		}
	}
	seed, seedOK := carrierproduct.NewRows(declared)
	if !seedOK {
		return carrierproduct.Rows{}, false
	}
	refinement := seed.BeginRefineWithWork(extender.coverProof, extender.checkpoint)
	if refinement == nil {
		return carrierproduct.Rows{}, false
	}
	for _, region := range extender.readRegions {
		if !refinement.Add(0, region) {
			_, _, _ = refinement.Seal()
			return carrierproduct.Rows{}, false
		}
	}
	rows, _, ok := refinement.Seal()
	if !ok || !rows.Valid() {
		return carrierproduct.Rows{}, false
	}
	if cap(extender.readCacheRegions) < len(extender.readRegions) {
		extender.readCacheRegions = make([]support.Mask, len(extender.readRegions))
	} else {
		extender.readCacheRegions = extender.readCacheRegions[:len(extender.readRegions)]
	}
	copy(extender.readCacheRegions, extender.readRegions)
	extender.readCacheSupport = declared
	extender.readCache = rows
	extender.readCacheValid = true
	return rows, true
}

func (extender *Extender[K, V, T]) materializeRows(ticket engineexecution.Ticket, input Rows[T], readRows, productRows carrierproduct.Rows, pairs carrierproduct.CrossPairs) bool {
	if extender == nil || !ticket.Checkpoint() {
		return false
	}
	if !pairs.ValidFor(input.product, readRows, productRows) {
		return false
	}
	extender.rows = extender.rows[:0]
	for index := 0; index < pairs.Count(); index++ {
		if !ticket.Checkpoint() {
			return false
		}
		leftIndex, readIndex, pairOK := pairs.At(index)
		if !pairOK || leftIndex < 0 || leftIndex >= input.Count() || readIndex < 0 || readIndex >= len(extender.readCells) {
			return false
		}
		_, tail, sourceOK := input.At(leftIndex)
		if !sourceOK {
			return false
		}
		extender.rows = append(extender.rows, Cons[Cell[V], T]{head: extender.readCells[readIndex], tail: tail})
	}
	return len(extender.rows) == pairs.Count()
}

// drain obtains the ExactRead's complete canonical partition under the
// authenticated ticket support. The product then performs only support
// intersections; it never asks the Factor to repartition the same key under
// each product source row.
func (extender *Extender[K, V, T]) drain(ticket engineexecution.Ticket, read engineexecution.ExactRead[K, V], scratch *engineexecution.Scratch[K, V]) (bool, bool) {
	if extender == nil || !ticket.Checkpoint() || !read.Valid() || scratch == nil {
		return false, false
	}
	ticketRegion, regionOK := ticket.Within()
	if !regionOK || !ticketRegion.Valid() {
		return false, false
	}
	extender.readRegions = extender.readRegions[:0]
	extender.readCells = extender.readCells[:0]
	for {
		switch read.Read(ticket, scratch) {
		case engineexecution.ReadAvailable:
			value, valueOK := scratch.Value()
			region, regionOK := scratch.Region()
			if !valueOK || !regionOK || !region.Valid() || !region.Entails(ticketRegion) {
				_ = scratch.Discard(ticket)
				return false, false
			}
			extender.readRegions = append(extender.readRegions, region)
			extender.readCells = append(extender.readCells, Cell[V]{value: value, present: scratch.Present()})
		case engineexecution.ReadExhausted:
			closed := read.Close(ticket, scratch)
			reused := closed && scratch.Reuse(ticket)
			if !closed || !reused || len(extender.readRegions) != len(extender.readCells) {
				return false, false
			}
			return true, len(extender.readRegions) == 0
		default:
			_ = scratch.Discard(ticket)
			return false, false
		}
	}
}

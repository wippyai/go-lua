package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// GlobalCell is one binder-authorized Program global-cell reservation. Slot is
// zero-based at the lower boundary; Ordinal is the corresponding one-based
// Program Cell ordinal. The identity is scoped to the Result that produced it.
// No Program term is present here: binder identities are construction
// capabilities, not persisted Program identities.
type GlobalCell struct {
	identity GlobalIdentity
	slot     uint32
	origin   ast.Position
}

// Identity returns the opaque binder identity reserved by this cell.
func (cell GlobalCell) Identity() GlobalIdentity { return cell.identity }

// Name returns the authored global spelling selected by the identity.
func (cell GlobalCell) Name() string { return cell.identity.Name() }

// Slot returns the zero-based dense lower-boundary slot.
func (cell GlobalCell) Slot() uint32 { return cell.slot }

// Ordinal returns the one-based dense Program Cell ordinal.
func (cell GlobalCell) Ordinal() uint32 { return cell.slot + 1 }

// Origin returns the canonical first authored source occurrence for the global
// identity. A runtime-type-only occurrence is retained here if that identity
// later upgrades to a real Cell.
func (cell GlobalCell) Origin() ast.Position { return cell.origin }

// GlobalCensus is the immutable, binder-owned global Cell denominator. It
// contains no name map or mutable construction state; identity lookup is an
// O(1) scoped slot check and ordered iteration is through At.
type GlobalCensus struct {
	owner *Result
	cells []GlobalCell
}

type globalAuthority struct {
	records     []globalRecord
	byID        []uint32
	occurrences []globalOccurrence
	rootHead    uint32
	rootTail    uint32
	cells       []GlobalCell
	frames      []globalOrderFrame
}

type globalOrderFrame struct {
	rhsHead uint32
	rhsTail uint32
	lhsHead uint32
	lhsTail uint32
	targets bool
}

type globalOccurrence struct {
	id     symbol.ID
	origin ast.Position
	next   uint32
}

// Len returns the exact number of reserved global Cells.
func (c GlobalCensus) Len() int { return len(c.cells) }

// At returns the reserved global Cell at its canonical source-order slot.
func (c GlobalCensus) At(index int) (GlobalCell, bool) {
	if index < 0 || index >= len(c.cells) {
		return GlobalCell{}, false
	}
	return c.cells[index], true
}

// Cell resolves an identity in O(1), rejecting identities from another
// binding Result even when their local symbol numbers happen to match.
func (c GlobalCensus) Cell(identity GlobalIdentity) (GlobalCell, bool) {
	if c.owner == nil || identity.owner != c.owner || identity.id == 0 {
		return GlobalCell{}, false
	}
	slot, ok := c.owner.globalSlot(identity.id)
	if !ok || int(slot) >= len(c.cells) {
		return GlobalCell{}, false
	}
	cell := c.cells[slot]
	if cell.identity.id != identity.id || cell.identity.owner != c.owner {
		return GlobalCell{}, false
	}
	return cell, true
}

// Contains reports whether identity is one of this census's reserved Cells.
func (c GlobalCensus) Contains(identity GlobalIdentity) bool {
	_, ok := c.Cell(identity)
	return ok
}

type globalRecord struct {
	id          symbol.ID
	firstOrigin ast.Position
	needsCell   bool
	observed    bool
	slot        uint32
}

func (r *Result) observeGlobal(id symbol.ID, ident *ast.IdentExpr, needsCell bool) {
	if r == nil || id == 0 || ident == nil {
		return
	}
	r.observeGlobalAt(id, authoredPosition(ident), needsCell)
}

func (r *Result) observeGlobalAt(id symbol.ID, origin ast.Position, needsCell bool) {
	if r == nil || id == 0 {
		return
	}
	record := r.globalRecord(id)
	if record == nil {
		return
	}
	if needsCell && !record.needsCell {
		record.needsCell = true
	}
	r.appendGlobalObservation(id, origin)
}

// beginGlobalOrderSegment opens the one assignment-order segment. The binder
// visits RHS values for semantic binding before assignment targets, so those
// transitions are buffered until the authored target range has been visited.
func (r *Result) beginGlobalOrderSegment() {
	if r == nil {
		return
	}
	r.globals.frames = append(r.globals.frames, globalOrderFrame{})
}

func (r *Result) beginGlobalOrderTargets() {
	if r == nil || len(r.globals.frames) == 0 {
		return
	}
	r.globals.frames[len(r.globals.frames)-1].targets = true
}

func (r *Result) appendGlobalObservation(id symbol.ID, origin ast.Position) {
	if r == nil || id == 0 {
		return
	}
	if len(r.globals.occurrences) == 0 {
		r.globals.occurrences = append(r.globals.occurrences, globalOccurrence{})
	}
	node := uint32(len(r.globals.occurrences))
	r.globals.occurrences = append(r.globals.occurrences, globalOccurrence{
		id:     id,
		origin: origin,
	})
	if len(r.globals.frames) == 0 {
		r.appendChain(&r.globals.rootHead, &r.globals.rootTail, node, node)
		return
	}
	frame := &r.globals.frames[len(r.globals.frames)-1]
	if frame.targets {
		r.appendChain(&frame.lhsHead, &frame.lhsTail, node, node)
	} else {
		r.appendChain(&frame.rhsHead, &frame.rhsTail, node, node)
	}
}

func (r *Result) endGlobalOrderSegment() {
	if r == nil || len(r.globals.frames) == 0 {
		return
	}
	index := len(r.globals.frames) - 1
	frame := r.globals.frames[index]
	r.globals.frames = r.globals.frames[:index]
	segmentHead, segmentTail := r.joinFrame(frame)
	if segmentHead == 0 {
		return
	}
	if len(r.globals.frames) == 0 {
		r.appendChain(&r.globals.rootHead, &r.globals.rootTail, segmentHead, segmentTail)
		return
	}
	parent := &r.globals.frames[len(r.globals.frames)-1]
	if parent.targets {
		r.appendChain(&parent.lhsHead, &parent.lhsTail, segmentHead, segmentTail)
	} else {
		r.appendChain(&parent.rhsHead, &parent.rhsTail, segmentHead, segmentTail)
	}
}

func (r *Result) joinFrame(frame globalOrderFrame) (uint32, uint32) {
	if frame.lhsHead == 0 {
		return frame.rhsHead, frame.rhsTail
	}
	if frame.rhsHead == 0 {
		return frame.lhsHead, frame.lhsTail
	}
	r.globals.occurrences[frame.lhsTail].next = frame.rhsHead
	return frame.lhsHead, frame.rhsTail
}

func (r *Result) appendChain(head, tail *uint32, segmentHead, segmentTail uint32) {
	if r == nil || segmentHead == 0 || segmentTail == 0 {
		return
	}
	if *head == 0 {
		*head, *tail = segmentHead, segmentTail
		return
	}
	r.globals.occurrences[*tail].next = segmentHead
	*tail = segmentTail
}

func authoredPosition(node ast.PositionHolder) ast.Position {
	if node == nil {
		return ast.Position{}
	}
	return ast.Position{
		Line:      node.Line(),
		Column:    node.Column(),
		EndLine:   node.LastLine(),
		EndColumn: node.LastColumn(),
	}
}

func (r *Result) addGlobalRecord(id symbol.ID) {
	if r == nil || id == 0 {
		return
	}
	if int(id) >= len(r.globals.byID) {
		r.globals.byID = append(r.globals.byID, make([]uint32, int(id)-len(r.globals.byID)+1)...)
	}
	r.globals.records = append(r.globals.records, globalRecord{id: id})
	r.globals.byID[id] = uint32(len(r.globals.records))
}

func (r *Result) globalRecord(id symbol.ID) *globalRecord {
	if r == nil || id == 0 || int(id) >= len(r.globals.byID) {
		return nil
	}
	index := r.globals.byID[id]
	if index == 0 || int(index) > len(r.globals.records) {
		return nil
	}
	return &r.globals.records[index-1]
}

func (r *Result) globalSlot(id symbol.ID) (uint32, bool) {
	record := r.globalRecord(id)
	if record == nil || !record.needsCell || int(record.slot) >= len(r.globals.cells) {
		return 0, false
	}
	cell := r.globals.cells[record.slot]
	return cell.slot, cell.identity.owner == r && cell.identity.id == id
}

func (r *Result) finalizeGlobalCensus() {
	if r == nil {
		return
	}
	r.globals.cells = make([]GlobalCell, 0)
	for index := r.globals.rootHead; index != 0; {
		node := r.globals.occurrences[index]
		index = node.next
		record := r.globalRecord(node.id)
		if record == nil || record.observed {
			continue
		}
		record.observed = true
		record.firstOrigin = node.origin
		if !record.needsCell {
			continue
		}
		slot := len(r.globals.cells)
		record.slot = uint32(slot)
		r.globals.cells = append(r.globals.cells, GlobalCell{
			identity: GlobalIdentity{owner: r, id: record.id},
			slot:     uint32(slot),
			origin:   record.firstOrigin,
		})
	}
	r.globals.occurrences = nil
	r.globals.frames = nil
	r.globals.rootHead = 0
	r.globals.rootTail = 0
}

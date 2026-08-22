// Package stage owns private staged-execution planning for the artifact
// compiler. It accepts declared stage requests, closes computation order, and
// returns only declarative placements; canonical schema emission remains with
// the parent compiler.
package stage

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	artifactdigest "github.com/wippyai/go-lua/analysis/program/artifact/digest"
	"github.com/wippyai/go-lua/analysis/schema"
)

type computation struct {
	point      identity.ContentID
	occurrence identity.ContentID
	left       identity.ContentID
	right      identity.ContentID
}

type computationBucket struct {
	rows         []computation
	byOccurrence map[identity.ContentID]int
	byPoint      map[identity.ContentID]int
}

type predecessor struct {
	point  identity.ContentID
	writes map[schema.Key]struct{}
}

type call struct {
	dispatch identity.ContentID
	summary  identity.ContentID
	effect   identity.ContentID
}

// Builder is the one-shot mutable owner of every staged execution request.
// It deliberately has no dependency on compiler state, Program, or schema
// emission.
type Builder struct {
	format       uint64
	locals       map[identity.ContentID]identity.ContentID
	successors   map[identity.ContentID]identity.ContentID
	predecessors map[identity.ContentID]predecessor
	computations map[identity.ContentID]*computationBucket
	calls        map[identity.ContentID]call
	sealed       bool
}

// New creates an empty stage-request owner for one artifact format.
func New(format uint64) *Builder {
	return &Builder{
		format:       format,
		locals:       make(map[identity.ContentID]identity.ContentID),
		successors:   make(map[identity.ContentID]identity.ContentID),
		predecessors: make(map[identity.ContentID]predecessor),
		computations: make(map[identity.ContentID]*computationBucket),
		calls:        make(map[identity.ContentID]call),
	}
}

// Successor requests the declaration-framed cut immediately after base's
// ordinary Local cut. The caller must also request Local for the same base;
// Seal rejects an orphan successor so the resulting transport chain is total.
func (builder *Builder) Successor(base identity.ContentID, framing string) (identity.ContentID, bool) {
	if !builder.usable(base, framing) {
		return identity.ContentID{}, false
	}
	if point := builder.successors[base]; point.Available() {
		return point, true
	}
	point := artifactdigest.Digest(framing, builder.format, artifactdigest.ContentID(base))
	if !point.Available() || point == base || point == builder.locals[base] {
		return identity.ContentID{}, false
	}
	builder.successors[base] = point
	return point, true
}

func (builder *Builder) usable(base identity.ContentID, framing string) bool {
	return builder != nil && !builder.sealed && base.Available() && framing != ""
}

// Local requests the declaration-framed local cut for base.
func (builder *Builder) Local(base identity.ContentID, framing string) (identity.ContentID, bool) {
	if !builder.usable(base, framing) {
		return identity.ContentID{}, false
	}
	if point := builder.locals[base]; point.Available() {
		return point, true
	}
	point := artifactdigest.Digest(framing, builder.format, artifactdigest.ContentID(base))
	if !point.Available() || point == base {
		return identity.ContentID{}, false
	}
	builder.locals[base] = point
	return point, true
}

// Predecessor requests the declaration-framed predecessor cut for base and
// records its strong-write axis directly at issuance time.
func (builder *Builder) Predecessor(base identity.ContentID, framing string, writes schema.Key) (identity.ContentID, bool) {
	if !builder.usable(base, framing) || !writes.Available() {
		return identity.ContentID{}, false
	}
	row, known := builder.predecessors[base]
	if !known {
		row.point = artifactdigest.Digest(framing, builder.format, artifactdigest.ContentID(base))
		if !row.point.Available() || row.point == base {
			return identity.ContentID{}, false
		}
		row.writes = make(map[schema.Key]struct{})
	}
	row.writes[writes] = struct{}{}
	builder.predecessors[base] = row
	return row.point, true
}

// Computation requests one local computation cut and rejects repeated point or
// occurrence identities in O(1) time per base.
func (builder *Builder) Computation(base identity.ContentID, framing string, key schema.Key, occurrence, left, right identity.ContentID) (identity.ContentID, bool) {
	if !builder.usable(base, framing) || !key.Available() || !occurrence.Available() || !left.Available() || !right.Available() {
		return identity.ContentID{}, false
	}
	point := artifactdigest.Digest(framing, builder.format, artifactdigest.ContentID(base), artifactdigest.Key(key), artifactdigest.ContentID(occurrence))
	if !point.Available() || point == base {
		return identity.ContentID{}, false
	}
	bucket := builder.computations[base]
	if bucket == nil {
		bucket = &computationBucket{byOccurrence: make(map[identity.ContentID]int), byPoint: make(map[identity.ContentID]int)}
		builder.computations[base] = bucket
	}
	if _, duplicate := bucket.byOccurrence[occurrence]; duplicate {
		return identity.ContentID{}, false
	}
	if _, duplicate := bucket.byPoint[point]; duplicate {
		return identity.ContentID{}, false
	}
	index := len(bucket.rows)
	bucket.rows = append(bucket.rows, computation{point: point, occurrence: occurrence, left: left, right: right})
	bucket.byOccurrence[occurrence] = index
	bucket.byPoint[point] = index
	return point, true
}

// Call requests the three declaration-framed call cuts for base.
func (builder *Builder) Call(base identity.ContentID, dispatchFraming, summaryFraming, effectFraming string) (Call, bool) {
	if !builder.usable(base, dispatchFraming) || summaryFraming == "" || effectFraming == "" {
		return Call{}, false
	}
	if row, known := builder.calls[base]; known {
		return Call{dispatch: row.dispatch, summary: row.summary, effect: row.effect}, true
	}
	row := call{
		dispatch: artifactdigest.Digest(dispatchFraming, builder.format, artifactdigest.ContentID(base)),
		summary:  artifactdigest.Digest(summaryFraming, builder.format, artifactdigest.ContentID(base)),
		effect:   artifactdigest.Digest(effectFraming, builder.format, artifactdigest.ContentID(base)),
	}
	if !row.dispatch.Available() || !row.summary.Available() || !row.effect.Available() ||
		row.dispatch == base || row.summary == base || row.effect == base ||
		row.dispatch == row.summary || row.dispatch == row.effect || row.summary == row.effect {
		return Call{}, false
	}
	builder.calls[base] = row
	return Call{dispatch: row.dispatch, summary: row.summary, effect: row.effect}, true
}

// Empty reports whether no stage request has been admitted.
func (builder *Builder) Empty() bool {
	return builder == nil || len(builder.locals) == 0 && len(builder.successors) == 0 && len(builder.predecessors) == 0 && len(builder.computations) == 0 && len(builder.calls) == 0
}

// Plan is an immutable, base-ordered staged execution description.
type Plan struct{ placements []Placement }

func (plan Plan) Count() int { return len(plan.placements) }

func (plan Plan) At(index int) (Placement, bool) {
	if index < 0 || index >= len(plan.placements) {
		return Placement{}, false
	}
	return plan.placements[index], true
}

// Placement describes every requested stage over one Program point.
type Placement struct {
	base              identity.ContentID
	local             identity.ContentID
	successor         identity.ContentID
	predecessor       identity.ContentID
	predecessorWrites []schema.Key
	computations      []Computation
	call              Call
	hasCall           bool
}

func (placement Placement) Base() identity.ContentID { return placement.base }

func (placement Placement) Local() (identity.ContentID, bool) {
	return placement.local, placement.local.Available()
}

func (placement Placement) Successor() (identity.ContentID, bool) {
	return placement.successor, placement.successor.Available()
}

func (placement Placement) Predecessor() (identity.ContentID, bool) {
	return placement.predecessor, placement.predecessor.Available()
}

func (placement Placement) PredecessorWriteCount() int { return len(placement.predecessorWrites) }

func (placement Placement) PredecessorWriteAt(index int) (schema.Key, bool) {
	if index < 0 || index >= len(placement.predecessorWrites) {
		return "", false
	}
	return placement.predecessorWrites[index], true
}

func (placement Placement) ComputationCount() int { return len(placement.computations) }

func (placement Placement) ComputationAt(index int) (Computation, bool) {
	if index < 0 || index >= len(placement.computations) {
		return Computation{}, false
	}
	return placement.computations[index], true
}

func (placement Placement) Call() (Call, bool) { return placement.call, placement.hasCall }

// Computation is one already topologically ordered local computation point.
type Computation struct{ point identity.ContentID }

func (computation Computation) Point() identity.ContentID { return computation.point }

// Call is the dispatch, summary, and effect cut triple for one base point.
type Call struct {
	dispatch identity.ContentID
	summary  identity.ContentID
	effect   identity.ContentID
}

func (call Call) Dispatch() identity.ContentID { return call.dispatch }
func (call Call) Summary() identity.ContentID  { return call.summary }
func (call Call) Effect() identity.ContentID   { return call.effect }

// Seal closes all computation dependencies with deterministic Kahn order and
// transfers the resulting declarative plan out of the mutable builder.
func (builder *Builder) Seal() (Plan, bool) {
	if builder == nil || builder.sealed {
		return Plan{}, false
	}
	bases := make(map[identity.ContentID]struct{}, len(builder.locals)+len(builder.successors)+len(builder.predecessors)+len(builder.computations)+len(builder.calls))
	for base := range builder.locals {
		bases[base] = struct{}{}
	}
	for base := range builder.predecessors {
		bases[base] = struct{}{}
	}
	for base := range builder.successors {
		bases[base] = struct{}{}
	}
	for base := range builder.computations {
		bases[base] = struct{}{}
	}
	for base := range builder.calls {
		bases[base] = struct{}{}
	}
	orderedBases := make([]identity.ContentID, 0, len(bases))
	for base := range bases {
		orderedBases = append(orderedBases, base)
	}
	identity.SortContentIDs(orderedBases)
	placements := make([]Placement, 0, len(orderedBases))
	for _, base := range orderedBases {
		placement := Placement{base: base, local: builder.locals[base], successor: builder.successors[base]}
		if placement.successor.Available() && !placement.local.Available() {
			return Plan{}, false
		}
		if predecessor, known := builder.predecessors[base]; known {
			placement.predecessor = predecessor.point
			placement.predecessorWrites = make([]schema.Key, 0, len(predecessor.writes))
			for write := range predecessor.writes {
				placement.predecessorWrites = append(placement.predecessorWrites, write)
			}
			sort.Slice(placement.predecessorWrites, func(left, right int) bool {
				return placement.predecessorWrites[left] < placement.predecessorWrites[right]
			})
		}
		if bucket := builder.computations[base]; bucket != nil {
			ordered, ok := orderComputations(bucket.rows)
			if !ok {
				return Plan{}, false
			}
			placement.computations = ordered
		}
		if call, known := builder.calls[base]; known {
			placement.call, placement.hasCall = Call{dispatch: call.dispatch, summary: call.summary, effect: call.effect}, true
		}
		placements = append(placements, placement)
	}
	builder.locals, builder.successors, builder.predecessors, builder.computations, builder.calls = nil, nil, nil, nil, nil
	builder.sealed = true
	return Plan{placements: placements}, true
}

func orderComputations(rows []computation) ([]Computation, bool) {
	if len(rows) == 0 {
		return nil, true
	}
	producer := make(map[identity.ContentID]int, len(rows))
	for index, row := range rows {
		if !row.point.Available() || !row.occurrence.Available() || !row.left.Available() || !row.right.Available() {
			return nil, false
		}
		if _, duplicate := producer[row.occurrence]; duplicate {
			return nil, false
		}
		producer[row.occurrence] = index
	}
	inDegree := make([]int, len(rows))
	dependents := make([][]int, len(rows))
	for index, row := range rows {
		for _, input := range [...]identity.ContentID{row.left, row.right} {
			dependency, found := producer[input]
			if !found {
				continue
			}
			inDegree[index]++
			dependents[dependency] = append(dependents[dependency], index)
		}
	}
	ready := make([]int, 0, len(rows))
	for index := range rows {
		if inDegree[index] == 0 {
			ready = pushReady(ready, index, rows)
		}
	}
	ordered := make([]Computation, 0, len(rows))
	for len(ready) != 0 {
		index := ready[0]
		ready = popReady(ready, rows)
		ordered = append(ordered, Computation{point: rows[index].point})
		for _, dependent := range dependents[index] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				ready = pushReady(ready, dependent, rows)
			}
		}
	}
	return ordered, len(ordered) == len(rows)
}

func contentIDBefore(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

func readyBefore(left, right int, rows []computation) bool {
	return contentIDBefore(rows[left].point, rows[right].point)
}

func pushReady(heap []int, value int, rows []computation) []int {
	heap = append(heap, value)
	for index := len(heap) - 1; index != 0; {
		parent := (index - 1) / 2
		if !readyBefore(heap[index], heap[parent], rows) {
			break
		}
		heap[index], heap[parent] = heap[parent], heap[index]
		index = parent
	}
	return heap
}

func popReady(heap []int, rows []computation) []int {
	last := len(heap) - 1
	heap[0] = heap[last]
	heap = heap[:last]
	for index := 0; ; {
		left := index*2 + 1
		if left >= len(heap) {
			return heap
		}
		right := left + 1
		smallest := left
		if right < len(heap) && readyBefore(heap[right], heap[left], rows) {
			smallest = right
		}
		if !readyBefore(heap[smallest], heap[index], rows) {
			return heap
		}
		heap[index], heap[smallest] = heap[smallest], heap[index]
		index = smallest
	}
}

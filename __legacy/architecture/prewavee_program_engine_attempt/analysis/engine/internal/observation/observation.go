// Package observation owns the epoch-private reverse index for facts-native
// reads.  It retains only semantic observation units and never sees a factor
// value, a diagram node, or published State.
package observation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// Equation is the dense identity of one compiled equation in this disposable
// epoch.  It is deliberately private to this scheduling plane: it is neither
// a Rule identity nor an artifact identity.
type Equation uint32

// NewEquation constructs one equation identity.
func NewEquation(value uint32) Equation { return Equation(value) }

// Scheduler is the transaction-owned scheduling boundary.  Observation calls
// it at most once per matching Equation in one Dispatch; the queue behind it
// remains the sole authority for cross-dispatch pending-work deduplication.
type Scheduler func(Equation) bool

// Index is the complete reverse observation projection for one active epoch.
// Replacing an equation removes its old projection before installing its new
// one.  No previous epoch, value, factor instance, FDD node, or source syntax
// is retained.
type Index struct {
	guards *guard.Manager

	equations map[Equation][]unit
	keyed     map[unitID]map[Equation]support.Mask
	planes    map[planeID]map[Equation]support.Mask
	all       map[coordinate.Coordinate]map[Equation]support.Mask
}

// New starts an empty observation epoch over one exact support universe.
func New(guards *guard.Manager) *Index {
	if guards == nil || !guards.Valid(guards.True()) {
		return nil
	}
	return &Index{
		guards:    guards,
		equations: make(map[Equation][]unit),
		keyed:     make(map[unitID]map[Equation]support.Mask),
		planes:    make(map[planeID]map[Equation]support.Mask),
		all:       make(map[coordinate.Coordinate]map[Equation]support.Mask),
	}
}

// Begin starts a replacement observation projection for equation.  The Log
// is candidate-private until Seal; Discard has no observable effect.
func (index *Index) Begin(equation Equation) *Log {
	if index == nil || index.guards == nil {
		return nil
	}
	work := support.New(index.guards)
	if work == nil {
		return nil
	}
	return &Log{index: index, equation: equation, work: work, units: make(map[unitID]unit)}
}

// Log accumulates the complete replacement projection of one equation.  Its
// work scope owns only Boolean union construction; it never owns facts.
type Log struct {
	index    *Index
	equation Equation
	work     *support.Work
	units    map[unitID]unit
	done     bool
}

type unitID struct {
	source coordinate.Coordinate
	factor uint32
	key    uint64
	plane  bool
}

type planeID struct {
	source coordinate.Coordinate
	factor uint32
}

type unit struct {
	id     unitID
	region support.Mask
}

// Read records one direct-key read.  It intentionally accepts an absent key:
// absence denotes the Factor's declared Default and is still invalidated by a
// later explicit or default-changing key delta.
func (log *Log) Read(source coordinate.Coordinate, factor uint32, key uint64, row support.Mask) bool {
	return log.record(unitID{source: source, factor: factor, key: key}, row)
}

// Plane records a whole-plane read, used by Carry.  A later plane delta wakes
// it regardless of which direct key changed.
func (log *Log) Plane(source coordinate.Coordinate, factor uint32, row support.Mask) bool {
	return log.record(unitID{source: source, factor: factor, plane: true}, row)
}

func (log *Log) record(id unitID, row support.Mask) bool {
	if log == nil || log.done || log.index == nil || log.work == nil || !log.work.Open() || !id.source.Valid() || !row.Valid() || row.Manager() != log.index.guards {
		return false
	}
	previous, exists := log.units[id]
	if !exists {
		log.units[id] = unit{id: id, region: row}
		return true
	}
	union, ok := log.work.Or(previous.region, row)
	if !ok {
		return false
	}
	log.units[id] = unit{id: id, region: union}
	return true
}

// Seal atomically replaces equation's entire old observation projection with
// this log.  All candidate mask unions are sealed before the index mutates.
func (log *Log) Seal() bool {
	if log == nil || log.done || log.index == nil || log.work == nil || !log.work.Open() {
		return false
	}
	if !log.work.Seal() {
		log.work.Discard()
		log.done = true
		return false
	}
	next := make([]unit, 0, len(log.units))
	for _, entry := range log.units {
		if !entry.region.Valid() || entry.region.Manager() != log.index.guards {
			log.done = true
			return false
		}
		next = append(next, entry)
	}
	sort.Slice(next, func(left, right int) bool { return compareUnit(next[left], next[right]) < 0 })
	log.index.replace(log.equation, next)
	log.done = true
	return true
}

// Discard invalidates the candidate log without replacing the prior equation
// projection.  It publishes neither reads nor a scheduler event.
func (log *Log) Discard() {
	if log == nil || log.done {
		return
	}
	if log.work != nil {
		log.work.Discard()
	}
	log.done = true
}

// Dispatch routes one exact facts-native semantic delta from source to its
// compatible reverse readers.  Support, Plane, and Key are deliberately
// separate events: a direct Read does not subscribe to a Carry, and Carry
// does not simulate a finite set of direct reads.  Scheduler is called at
// most once per matching equation and in Equation order; its queue remains
// the authority for deduplication across separate deltas.
func (index *Index) Dispatch(source coordinate.Coordinate, change facts.Delta, schedule Scheduler) bool {
	if index == nil || index.guards == nil || !source.Valid() || schedule == nil {
		return false
	}
	region := change.Mask()
	if !region.Valid() || region.Manager() != index.guards || maskEmpty(region) {
		return false
	}
	var readers map[Equation]support.Mask
	switch {
	case change.IsSupport():
		readers = index.all[source]
	case change.IsPlane():
		readers = index.planes[planeID{source: source, factor: change.Plane}]
	case change.IsKey():
		readers = index.keyed[unitID{source: source, factor: change.Plane, key: change.Key}]
	default:
		return false
	}
	if len(readers) == 0 {
		return true
	}
	equations := make([]Equation, 0, len(readers))
	for equation, observed := range readers {
		if intersects(index.guards, observed, region) {
			equations = append(equations, equation)
		}
	}
	sort.Slice(equations, func(left, right int) bool { return equations[left] < equations[right] })
	for _, equation := range equations {
		if !schedule(equation) {
			return false
		}
	}
	return true
}

// intersects proves satisfiability of the exact Boolean conjunction.  There
// is intentionally no syntactic guard shortcut: implication, complements,
// and reduced shared topology all remain semantic cases.
func intersects(guards *guard.Manager, left, right support.Mask) bool {
	if guards == nil || !left.Valid() || !right.Valid() || left.Manager() != guards || right.Manager() != guards {
		return false
	}
	work := support.New(guards)
	if work == nil {
		return false
	}
	intersection, ok := work.And(left, right)
	if !ok || !work.Seal() {
		work.Discard()
		return false
	}
	return !maskEmpty(intersection)
}

func maskEmpty(region support.Mask) bool {
	view, ok := region.Decompose()
	return !ok || view.Terminal && !view.Value
}

func compareUnit(left, right unit) int {
	if order := left.id.source.Compare(right.id.source); order != 0 {
		return order
	}
	if left.id.factor < right.id.factor {
		return -1
	}
	if left.id.factor > right.id.factor {
		return 1
	}
	if left.id.plane != right.id.plane {
		if !left.id.plane {
			return -1
		}
		return 1
	}
	if left.id.key < right.id.key {
		return -1
	}
	if left.id.key > right.id.key {
		return 1
	}
	return 0
}

func (index *Index) replace(equation Equation, next []unit) {
	previous := index.equations[equation]
	touched := make(map[coordinate.Coordinate]struct{}, len(previous)+len(next))
	for _, entry := range previous {
		touched[entry.id.source] = struct{}{}
		index.remove(entry, equation)
	}
	if len(next) == 0 {
		delete(index.equations, equation)
	} else {
		index.equations[equation] = next
		for _, entry := range next {
			touched[entry.id.source] = struct{}{}
			index.add(entry, equation)
		}
	}
	for source := range touched {
		index.rebuildAll(source, equation)
	}
}

func (index *Index) add(entry unit, equation Equation) {
	if entry.id.plane {
		bucket := planeID{source: entry.id.source, factor: entry.id.factor}
		insert(index.planes, bucket, equation, entry.region)
		return
	}
	insert(index.keyed, entry.id, equation, entry.region)
}

func (index *Index) remove(entry unit, equation Equation) {
	if entry.id.plane {
		remove(index.planes, planeID{source: entry.id.source, factor: entry.id.factor}, equation)
		return
	}
	remove(index.keyed, entry.id, equation)
}

func insert[K comparable](index map[K]map[Equation]support.Mask, key K, equation Equation, region support.Mask) {
	bucket := index[key]
	if bucket == nil {
		bucket = make(map[Equation]support.Mask)
		index[key] = bucket
	}
	bucket[equation] = region
}

func remove[K comparable](index map[K]map[Equation]support.Mask, key K, equation Equation) {
	bucket := index[key]
	if bucket == nil {
		return
	}
	delete(bucket, equation)
	if len(bucket) == 0 {
		delete(index, key)
	}
}

// rebuildAll retains the exact union of an equation's current keyed/plane
// observation regions at one coordinate.  It is the Support-event reverse
// bucket, not a synthetic whole-factor observation.
func (index *Index) rebuildAll(source coordinate.Coordinate, equation Equation) {
	bucket := index.all[source]
	if bucket != nil {
		delete(bucket, equation)
		if len(bucket) == 0 {
			delete(index.all, source)
		}
	}
	regions := make([]support.Mask, 0)
	for _, entry := range index.equations[equation] {
		if entry.id.source == source {
			regions = append(regions, entry.region)
		}
	}
	region, ok := union(index.guards, regions)
	if !ok {
		return
	}
	bucket = index.all[source]
	if bucket == nil {
		bucket = make(map[Equation]support.Mask)
		index.all[source] = bucket
	}
	bucket[equation] = region
}

func union(guards *guard.Manager, regions []support.Mask) (support.Mask, bool) {
	if len(regions) == 0 {
		return support.Mask{}, false
	}
	if len(regions) == 1 {
		return regions[0], regions[0].Valid() && regions[0].Manager() == guards
	}
	work := support.New(guards)
	if work == nil {
		return support.Mask{}, false
	}
	current := regions[0]
	if !work.Valid(current) {
		work.Discard()
		return support.Mask{}, false
	}
	for _, next := range regions[1:] {
		var ok bool
		current, ok = work.Or(current, next)
		if !ok {
			work.Discard()
			return support.Mask{}, false
		}
	}
	if !work.Seal() {
		work.Discard()
		return support.Mask{}, false
	}
	return current, true
}

package engine

import (
	"context"

	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/internal/observation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/program/link"
)

// transaction is one private evaluation generation. Its queue, Guard work,
// typed Factor candidates, and joint Facts roots are discarded on every failed
// generation and retained only through an immutable published State.
type transaction struct {
	solver  *Solver
	queue   *actionQueue
	reads   *observation.Index
	readLog *observation.Log
	// guards is the one sealed Program-decision universe. Facts owns the
	// corresponding symbolic support; transaction does not construct a second
	// guard work graph.
	guards *guard.Manager
	done   <-chan struct{}

	roots          []transactionRoot
	contributions  []facts.Facts
	relationInput  []facts.Facts
	relationOrigin []coordinate.Coordinate

	// relationFrames and relationTerms are callback-scoped activation scratch.
	// Relation is a generation-stamped value capability, so these private
	// frames may be reused without allowing a retained resolver to revive.
	relationFrames   [4]relationFrame
	relationOverflow []*relationFrame
	relationDepth    int
	relationTerms    []termOrigin
	relationTermTop  int
	execution        ruleExecution
	executing        bool
	accessEpoch      uint64
	dirty            []bool
	sweeps           []regionSweep

	// supports is the exact guard support of each immutable active relation.
	// A new relation is structural growth. Discovery stays open while this
	// immutable carrier settles so every selector already reachable in this
	// topology contributes to one canonical next-epoch batch; it does not
	// terminate the schedule at the first relation it sees.
	supports          []support.Mask
	activation        []activationSource
	activationOutputs []compiledSupportOutput
	activationOpen    bool
	activationNext    []support.Mask
	activationWork    *support.Work
	discovered        []activeRelation
	relationEpoch     uint64
	rebuild           bool
}

// transactionRoot is the one private mutable location of the joint Facts
// carrier. State never retains this root: publication materializes only
// declared scalar observations.
type transactionRoot struct {
	coordinate coordinate.Coordinate
	value      facts.Facts
}

// regionSweep holds only the previous joint Facts tuple for one explicit-Mu
// schedule region.  It never reconstructs a second carrier or a per-Factor
// topology: Facts owns support and every registered plane as one value.
type regionSweep struct {
	prior        []facts.Facts
	supports     []support.Mask
	nextSupports []support.Mask
	active       bool
	phase        sweepPhase
}

type sweepPhase uint8

const (
	sweepWiden sweepPhase = iota + 1
	sweepNarrow
	sweepVerify
)

type scheduleFrame struct {
	region int
	outer  int
}

// Solve evaluates exactly the backwards Rule slice rooted by sealed Queries.
// A root or explicit Body Candidate supplies the existing activation entry;
// candidate selection itself never manufactures a Program relation or an
// additional State plane. Cancellation is external caller control only: it
// abandons this private transaction and never changes the mathematical
// fixed-point operator, its ordering, or its convergence conditions.
func (solver *Solver) Solve(ctx context.Context, base *State) (*State, bool) {
	if ctx == nil || !solver.valid() || solver.evaluating || base != nil && !solver.validState(base) {
		return nil, false
	}
	if len(solver.queries) == 0 {
		return nil, false
	}
	done := ctx.Done()
	// Check before reusing an immutable result or allocating a new epoch. A
	// canceled request has no result, even when its supplied State is valid.
	if canceled(done) {
		return nil, false
	}
	if base != nil {
		return base, true
	}
	solver.evaluating = true
	defer func() { solver.evaluating = false }()

	for {
		// Every rebuilt epoch belongs to this one caller request. Do not begin
		// a new generation after its cancellation has become observable.
		if canceled(done) {
			return nil, false
		}
		transaction, ok := solver.beginEpoch(done)
		if !ok {
			return nil, false
		}
		if transaction.canceled() || !transaction.run() || transaction.canceled() {
			transaction.abort()
			return nil, false
		}
		if transaction.rebuild {
			if transaction.canceled() {
				transaction.abort()
				return nil, false
			}
			relations, valid := transaction.nextAcceptedRelations()
			transaction.abort()
			if !valid {
				return nil, false
			}
			// This assignment is the one structural acceptance cut. It is
			// deliberately independent of the caller's cancellation channel:
			// accepted Program/Link relations are harmless cached structure, not
			// a semantic result, and survive an interrupted solve.
			solver.active = relations
			// U is Link's sealed Candidate universe and E grows only by a new
			// exact (template, caller Coordinate, Candidate, selector) fact.
			// Rebuilding the disposable epoch from canonical Init is therefore
			// finite without a round bound, Go recursion, or a stale carrier.
			if !solver.rebuildEpoch(relations) {
				return nil, false
			}
			continue
		}
		state, ok := transaction.publish()
		if !ok {
			return nil, false
		}
		return state, true
	}
}

// canceled is intentionally channel-only. Context is read once at Solve's
// caller boundary; the hot evaluator sees a nil channel for the ordinary
// Background case and otherwise performs this non-blocking external poll.
func canceled(done <-chan struct{}) bool {
	if done == nil {
		return false
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (transaction *transaction) canceled() bool {
	return transaction == nil || canceled(transaction.done)
}

func (solver *Solver) beginEpoch(done <-chan struct{}) (*transaction, bool) {
	if canceled(done) || solver == nil || len(solver.actions) == 0 || solver.guards == nil || solver.facts == nil || !solver.facts.Valid(solver.zero) || !solver.facts.Valid(solver.entry) {
		return nil, false
	}
	queue := newActionQueue(len(solver.actions))
	if queue == nil {
		return nil, false
	}
	reads := observation.New(solver.guards)
	if reads == nil {
		queue.Discard()
		return nil, false
	}
	transaction := &transaction{solver: solver, queue: queue, reads: reads, guards: solver.guards, done: done}
	if !transaction.openRoots(solver.roots) || !transaction.openSupports() {
		transaction.abort()
		return nil, false
	}
	return transaction, true
}

func (transaction *transaction) openRoots(coordinates []coordinate.Coordinate) bool {
	if transaction == nil || transaction.solver == nil || transaction.solver.facts == nil || transaction.guards == nil || !transaction.solver.facts.Valid(transaction.solver.zero) || !transaction.solver.facts.Valid(transaction.solver.entry) {
		return false
	}
	transaction.roots = make([]transactionRoot, len(coordinates))
	for index, coordinate := range coordinates {
		if !coordinate.Valid() {
			return false
		}
		// False support is the only ordinary-root seed. Entry support is
		// admitted solely at an explicit demanded Program Entry.
		value := transaction.solver.zero
		if transaction.entryCoordinate(coordinate) {
			value = transaction.solver.entry
		}
		transaction.roots[index] = transactionRoot{coordinate: coordinate, value: value}
	}
	return true
}

// entryCoordinate is the only root seed rule in this tranche. Every other
// demanded Program location starts absent and can acquire support only from a
// selected causal Edge transfer.
func (transaction *transaction) entryCoordinate(coordinate coordinate.Coordinate) bool {
	if transaction == nil || transaction.solver == nil || transaction.solver.coordinate == nil || transaction.solver.link == nil {
		return false
	}
	candidate, shard, term, ok := transaction.solver.coordinate.Semantic(coordinate)
	if !ok {
		return false
	}
	p, ok := transaction.solver.link.Program(shard)
	if !ok || p == nil {
		return false
	}
	if candidate != (link.Candidate{}) {
		return false
	}
	if _, explicit := transaction.solver.entrySeeds[coordinate]; !explicit {
		return false
	}
	entry, ok := p.Entry()
	return ok && term == entry
}

func (transaction *transaction) run() bool {
	if transaction == nil || transaction.canceled() || transaction.solver == nil || transaction.solver.schedule == nil || len(transaction.solver.actions) == 0 {
		return false
	}
	transaction.dirty = make([]bool, len(transaction.solver.actions))
	for index := range transaction.dirty {
		transaction.dirty[index] = true
	}
	transaction.sweeps = make([]regionSweep, len(transaction.solver.regions))
	if !transaction.drive() {
		return false
	}
	return transaction.drainDirty() && !transaction.anyDirty()
}

func (transaction *transaction) dispatch(action *compiledAction) bool {
	if transaction == nil || transaction.canceled() || transaction.reads == nil || action == nil || action.index < 0 || action.index >= len(transaction.solver.actions) || action.run == nil || !action.coordinate.Valid() || !action.open(transaction) {
		return false
	}
	log := transaction.reads.Begin(observation.NewEquation(uint32(action.equation)))
	if log == nil {
		action.close(transaction)
		return false
	}
	transaction.readLog = log
	defer func() {
		if transaction.readLog == log {
			log.Discard()
			transaction.readLog = nil
		}
	}()
	if transaction.canceled() {
		return false
	}
	if !transaction.beginActivation(action) {
		action.close(transaction)
		return false
	}
	if transaction.canceled() {
		return false
	}
	if !action.run(transaction, action) {
		transaction.finishActivation(action)
		action.close(transaction)
		return false
	}
	if transaction.canceled() {
		return false
	}
	if !action.close(transaction) {
		transaction.finishActivation(action)
		return false
	}
	if !transaction.finishActivation(action) || !log.Seal() {
		return false
	}
	transaction.readLog = nil
	return transaction.drainDirty()
}

// drive interprets the one immutable Schedule event stream. Queue delivery is
// reduced to dense dirty flags; it never chooses execution order. Frames are
// explicit, so deeply nested WTO regions cannot consume the Go call stack.
// Only an outer region owns convergence. Every cyclic compiled equation
// receives one deterministic Mu head and every pass computes its complete
// correlated tuple in schedule order before applying one forced widening.
// Program/Link boundaries govern Guard/value transport inside an action; they
// do not decide whether a finite monotone equation is cyclic. Nested WTO
// regions are ordering brackets only; they never manufacture a second
// recurrence loop or update mode.
func (transaction *transaction) drive() bool {
	if transaction == nil || transaction.solver == nil || transaction.solver.schedule == nil || len(transaction.dirty) != len(transaction.solver.actions) {
		return false
	}
	scheduled := transaction.solver.schedule
	frames := make([]scheduleFrame, 0, len(transaction.solver.regions))
	for eventIndex := 0; eventIndex < scheduled.EventCount(); {
		if transaction.canceled() {
			return true
		}
		event, ok := scheduled.EventAt(eventIndex)
		if !ok || event.Node < 0 || int(event.Node) >= len(transaction.solver.actions) {
			return false
		}
		switch event.Kind {
		case schedule.EventEnter:
			if event.Region < 0 || event.Region >= len(transaction.solver.regions) {
				return false
			}
			region := transaction.solver.regions[event.Region]
			if region.head != int(event.Node) {
				return false
			}
			outer := -1
			if region.outer {
				outer = event.Region
			} else if len(frames) != 0 {
				outer = frames[len(frames)-1].outer
			}
			if !region.outer && outer < 0 {
				return false
			}
			if region.outer && !transaction.beginSweep(event.Region, sweepWiden) {
				return false
			}
			frames = append(frames, scheduleFrame{region: event.Region, outer: outer})
			eventIndex++
		case schedule.EventNode:
			actionIndex := int(event.Node)
			if !transaction.dirty[actionIndex] {
				eventIndex++
				continue
			}
			transaction.dirty[actionIndex] = false
			if !transaction.dispatch(&transaction.solver.actions[actionIndex]) {
				return false
			}
			eventIndex++
		case schedule.EventExit:
			if event.Region < 0 || event.Region >= len(transaction.solver.regions) || len(frames) == 0 || frames[len(frames)-1].region != event.Region {
				return false
			}
			frame := frames[len(frames)-1]
			region := transaction.solver.regions[frame.region]
			if region.head != int(event.Node) || !transaction.drainDirty() {
				return false
			}
			if region.outer {
				restart, nextPhase, valid := transaction.finishSweep(frame.region)
				if !valid {
					return false
				}
				if restart {
					if !transaction.beginSweep(frame.region, nextPhase) {
						return false
					}
					// EventEnter is followed immediately by the outer head EventNode.
					// Re-enter only its body; the one frame remains live throughout.
					eventIndex = transaction.regionBodyStart(frame.region)
					if eventIndex < 0 {
						return false
					}
					continue
				}
			}
			frames = frames[:len(frames)-1]
			eventIndex++
		default:
			return false
		}
	}
	return len(frames) == 0 && transaction.drainDirty()
}

func (transaction *transaction) beginSweep(regionIndex int, phase sweepPhase) bool {
	if transaction == nil || transaction.canceled() || transaction.solver == nil || regionIndex < 0 || regionIndex >= len(transaction.solver.regions) || regionIndex >= len(transaction.sweeps) {
		return false
	}
	region := transaction.solver.regions[regionIndex]
	sweep := &transaction.sweeps[regionIndex]
	if !region.outer || sweep.active || len(region.slots) == 0 || phase < sweepWiden || phase > sweepVerify {
		return false
	}
	if cap(sweep.prior) < len(region.slots) {
		sweep.prior = make([]facts.Facts, len(region.slots))
	} else {
		sweep.prior = sweep.prior[:len(region.slots)]
	}
	for index, slot := range region.slots {
		if slot < 0 || slot >= len(transaction.roots) {
			return false
		}
		sweep.prior[index] = transaction.roots[slot].value
	}
	if cap(sweep.supports) < len(region.supports) {
		sweep.supports = make([]support.Mask, len(region.supports))
	} else {
		sweep.supports = sweep.supports[:len(region.supports)]
	}
	for index, slot := range region.supports {
		if slot < 0 || slot >= len(transaction.supports) || !transaction.supports[slot].Valid() {
			return false
		}
		sweep.supports[index] = transaction.supports[slot]
	}
	sweep.active = true
	sweep.phase = phase
	return true
}

// finishSweep is the sole outer-SCC convergence transition. Each coordinate
// is one joint Facts value, so support and all typed planes take the same
// exact Widen/Narrow transition. No legacy topology or per-domain engine
// callback participates in this fixed-point operator.
func (transaction *transaction) finishSweep(regionIndex int) (bool, sweepPhase, bool) {
	if transaction == nil || transaction.canceled() || transaction.solver == nil || regionIndex < 0 || regionIndex >= len(transaction.solver.regions) || regionIndex >= len(transaction.sweeps) {
		return false, 0, false
	}
	region := transaction.solver.regions[regionIndex]
	sweep := &transaction.sweeps[regionIndex]
	if !region.outer || !sweep.active || sweep.phase < sweepWiden || sweep.phase > sweepVerify || len(region.slots) == 0 || len(region.slots) != len(sweep.prior) || len(region.supports) != len(sweep.supports) {
		return false, 0, false
	}
	phase := sweep.phase
	if phase == sweepVerify {
		relation, valid := transaction.sweepProductRelation(region, sweep)
		if !valid {
			return false, 0, false
		}
		// Verification proves a single fixed product point only. Even a pure
		// support descent is not committed here: it has not yet been paired
		// with a Factor narrowing step, so conservatively widen both halves.
		if relation != sweepEqual {
			advanced, ok := transaction.widenSweepProduct(region, sweep)
			if !ok {
				return false, 0, false
			}
			sweep.active = false
			if !transaction.drainDirty() || !advanced {
				return false, 0, false
			}
			return true, sweepWiden, transaction.markRegion(region)
		}
		postfixed := transaction.sweepRootsBelow(region, sweep)
		// Verification's ordinary body writes are deliberately transient. Its
		// predecessor is the narrowed candidate that just proved stable.
		for index, slot := range region.slots {
			if slot < 0 || slot >= len(transaction.roots) {
				return false, 0, false
			}
			transaction.roots[slot].value = sweep.prior[index]
		}
		sweep.active = false
		if !postfixed || !transaction.drainDirty() {
			return false, 0, false
		}
		if !transaction.clearRegionMembers(region) {
			return false, 0, false
		}
		return false, 0, true
	}
	relation, valid := transaction.sweepProductRelation(region, sweep)
	if !valid {
		return false, 0, false
	}
	var advanced bool
	var ok bool
	switch phase {
	case sweepWiden:
		// Widen is never allowed to retract a transient cyclic row.  It
		// joins the complete product until support and values stabilize; the
		// following topology-narrow pass is the sole place a proven descent
		// takes NarrowReplace.
		advanced, ok = transaction.widenSweepProduct(region, sweep)
	case sweepNarrow:
		if relation == sweepGrowth || relation == sweepIncomparable {
			advanced, ok = transaction.widenSweepProduct(region, sweep)
			if !ok {
				return false, 0, false
			}
			sweep.active = false
			if !transaction.drainDirty() || !advanced {
				return false, 0, false
			}
			return true, sweepWiden, transaction.markRegion(region)
		}
		// A descending product is accepted only after the same body result
		// proves the complete joint root below its predecessor. Facts.Narrow
		// then removes support and narrows every typed plane atomically.
		if relation == sweepDescent && !transaction.sweepRootsBelow(region, sweep) {
			advanced, ok = transaction.widenSweepProduct(region, sweep)
			if !ok {
				return false, 0, false
			}
			sweep.active = false
			if !transaction.drainDirty() || !advanced {
				return false, 0, false
			}
			return true, sweepWiden, transaction.markRegion(region)
		}
		if relation == sweepDescent {
			advanced, ok = transaction.replaceSweepProduct(region, sweep)
		} else {
			advanced, ok = transaction.narrowSweepProduct(region, sweep)
		}
	default:
		return false, 0, false
	}
	if !ok {
		return false, 0, false
	}
	sweep.active = false
	if !transaction.drainDirty() {
		return false, 0, false
	}
	if advanced {
		return true, phase, transaction.markRegion(region)
	}
	if phase == sweepWiden && region.narrow {
		return true, sweepNarrow, transaction.markRegion(region)
	}
	if phase == sweepNarrow {
		return true, sweepVerify, transaction.markRegion(region)
	}
	if !transaction.clearRegionMembers(region) {
		return false, 0, false
	}
	return false, 0, true
}

// sweepRelation is the one partial order for an outer SCC tuple. Relation
// activation supports are control-only; root order is delegated wholesale to
// the Facts schema so no Factor judgment leaks into the engine.
type sweepRelation uint8

const (
	sweepEqual sweepRelation = iota + 1
	sweepDescent
	sweepGrowth
	sweepIncomparable
)

func (transaction *transaction) sweepProductRelation(region compiledRegion, sweep *regionSweep) (sweepRelation, bool) {
	if transaction == nil || transaction.guards == nil || len(region.supports) != len(sweep.supports) {
		return 0, false
	}
	descending, growing := false, false
	observe := func(descends, grows bool) (sweepRelation, bool) {
		switch {
		case descends && grows:
			return sweepIncomparable, true
		case descends:
			descending = true
		case grows:
			growing = true
		}
		if descending && growing {
			return sweepIncomparable, true
		}
		return sweepEqual, true
	}
	for index, slot := range region.supports {
		if slot < 0 || slot >= len(transaction.supports) {
			return 0, false
		}
		prior, desired := sweep.supports[index], transaction.supports[slot]
		if !prior.Valid() || !desired.Valid() {
			return 0, false
		}
		descends := desired.Entails(prior)
		grows := prior.Entails(desired)
		if !descends && !grows {
			return sweepIncomparable, true
		}
		relation, ok := observe(descends && !grows, grows && !descends)
		if !ok {
			return 0, false
		}
		if relation == sweepIncomparable {
			return relation, true
		}
	}
	for index, slot := range region.slots {
		if slot < 0 || slot >= len(transaction.roots) {
			return 0, false
		}
		prior, desired := sweep.prior[index], transaction.roots[slot].value
		if !transaction.solver.facts.Valid(prior) || !transaction.solver.facts.Valid(desired) {
			return 0, false
		}
		if transaction.solver.facts.Equal(prior, desired) {
			continue
		}
		descends := transaction.solver.facts.LessOrEq(desired, prior)
		grows := transaction.solver.facts.LessOrEq(prior, desired)
		if !descends && !grows {
			return sweepIncomparable, true
		}
		relation, valid := observe(descends, grows)
		if !valid {
			return 0, false
		}
		if relation == sweepIncomparable {
			return relation, true
		}
	}
	if descending {
		return sweepDescent, true
	}
	if growing {
		return sweepGrowth, true
	}
	return sweepEqual, true
}

func (transaction *transaction) sweepRootsBelow(region compiledRegion, sweep *regionSweep) bool {
	if transaction == nil || len(region.slots) != len(sweep.prior) {
		return false
	}
	for index, slot := range region.slots {
		if slot < 0 || slot >= len(transaction.roots) || !transaction.solver.facts.LessOrEq(transaction.roots[slot].value, sweep.prior[index]) {
			return false
		}
	}
	return true
}

func (transaction *transaction) widenSweepProduct(region compiledRegion, sweep *regionSweep) (bool, bool) {
	if transaction == nil || transaction.guards == nil || transaction.solver == nil || transaction.solver.facts == nil {
		return false, false
	}
	advanced, ok := transaction.widenSweepSupports(region, sweep)
	if !ok {
		return false, false
	}
	for index, slot := range region.slots {
		if slot < 0 || slot >= len(transaction.roots) {
			return false, false
		}
		prior, desired := sweep.prior[index], transaction.roots[slot].value
		next, ok := transaction.solver.facts.Widen(prior, desired)
		if !ok {
			return false, false
		}
		advanced = advanced || !transaction.solver.facts.Equal(next, prior)
		transaction.roots[slot].value = next
	}
	return advanced, true
}

// widenSweepSupports performs all relation-support unions in one candidate
// BDD work scope and seals them together. The resulting masks are the sole
// activation representation; raw guards never become transaction state.
func (transaction *transaction) widenSweepSupports(region compiledRegion, sweep *regionSweep) (bool, bool) {
	if transaction == nil || transaction.guards == nil || len(region.supports) != len(sweep.supports) {
		return false, false
	}
	work := support.New(transaction.guards)
	if work == nil {
		return false, false
	}
	if cap(sweep.nextSupports) < len(region.supports) {
		sweep.nextSupports = make([]support.Mask, len(region.supports))
	} else {
		sweep.nextSupports = sweep.nextSupports[:len(region.supports)]
	}
	next := sweep.nextSupports
	for index, slot := range region.supports {
		if slot < 0 || slot >= len(transaction.supports) || !sweep.supports[index].Valid() || !transaction.supports[slot].Valid() {
			work.Discard()
			return false, false
		}
		var ok bool
		next[index], ok = work.Or(sweep.supports[index], transaction.supports[slot])
		if !ok {
			work.Discard()
			return false, false
		}
	}
	if !work.Seal() {
		work.Discard()
		return false, false
	}
	advanced := false
	for index, slot := range region.supports {
		advanced = advanced || !sweep.supports[index].Equal(next[index])
		transaction.supports[slot] = next[index]
	}
	return advanced, true
}

func (transaction *transaction) narrowSweepProduct(region compiledRegion, sweep *regionSweep) (bool, bool) {
	if transaction == nil || transaction.guards == nil || transaction.solver == nil || transaction.solver.facts == nil {
		return false, false
	}
	advanced := false
	for index, slot := range region.slots {
		if slot < 0 || slot >= len(transaction.roots) {
			return false, false
		}
		prior, desired := sweep.prior[index], transaction.roots[slot].value
		next, ok := transaction.solver.facts.Narrow(prior, desired)
		if !ok {
			return false, false
		}
		advanced = advanced || !transaction.solver.facts.Equal(next, prior)
		transaction.roots[slot].value = next
	}
	for index, slot := range region.supports {
		if slot < 0 || slot >= len(transaction.supports) || !transaction.supports[slot].Valid() || !sweep.supports[index].Valid() {
			return false, false
		}
		advanced = advanced || !sweep.supports[index].Equal(transaction.supports[slot])
	}
	return advanced, true
}

// replaceSweepProduct accepts a proven joint descent. Facts.Narrow is the
// sole operation that may remove support rows; it simultaneously narrows all
// planes, so a support/Factor split cannot reappear in the engine.
func (transaction *transaction) replaceSweepProduct(region compiledRegion, sweep *regionSweep) (bool, bool) {
	if transaction == nil || len(region.slots) != len(sweep.prior) || len(region.supports) != len(sweep.supports) {
		return false, false
	}
	advanced := false
	for index, slot := range region.slots {
		if slot < 0 || slot >= len(transaction.roots) {
			return false, false
		}
		prior, desired := sweep.prior[index], transaction.roots[slot].value
		next, valid := transaction.solver.facts.Narrow(prior, desired)
		if !valid {
			return false, false
		}
		advanced = advanced || !transaction.solver.facts.Equal(next, prior)
		transaction.roots[slot].value = next
	}
	for index, slot := range region.supports {
		if slot < 0 || slot >= len(transaction.supports) || !sweep.supports[index].Valid() || !transaction.supports[slot].Valid() {
			return false, false
		}
		advanced = advanced || !sweep.supports[index].Equal(transaction.supports[slot])
	}
	return advanced, true
}

// clearRegionMembers removes stale action wakes created by a final temporary
// body sweep after its tuple operator restored the predecessor. Successors are
// intentionally untouched: their dirty state is the only authority for later
// Schedule work outside this region.
func (transaction *transaction) clearRegionMembers(region compiledRegion) bool {
	if transaction == nil || len(transaction.dirty) != len(transaction.solver.actions) {
		return false
	}
	for _, action := range region.members {
		if action < 0 || action >= len(transaction.dirty) {
			return false
		}
		transaction.dirty[action] = false
	}
	return true
}

func (transaction *transaction) regionBodyStart(wanted int) int {
	if transaction == nil || transaction.solver == nil || transaction.solver.schedule == nil || wanted < 0 || wanted >= len(transaction.solver.regions) {
		return -1
	}
	region, ok := transaction.solver.schedule.RegionAt(wanted)
	if !ok || region.Enter < 0 || region.Enter+1 >= transaction.solver.schedule.EventCount() {
		return -1
	}
	event, ok := transaction.solver.schedule.EventAt(region.Enter + 1)
	if !ok || event.Kind != schedule.EventNode || event.Region != wanted || int(event.Node) != transaction.solver.regions[wanted].head {
		return -1
	}
	return region.Enter + 1
}

func (transaction *transaction) drainDirty() bool {
	if transaction == nil || transaction.canceled() || transaction.queue == nil || len(transaction.dirty) != len(transaction.solver.actions) {
		return false
	}
	for {
		if transaction.canceled() {
			return false
		}
		equation, present := transaction.queue.Next()
		if !present {
			return true
		}
		index := int(equation)
		if index < 0 || index >= len(transaction.dirty) {
			return false
		}
		transaction.dirty[index] = true
	}
}

func (transaction *transaction) anyDirty() bool {
	for _, dirty := range transaction.dirty {
		if dirty {
			return true
		}
	}
	return false
}

func (transaction *transaction) runConfluence(action *compiledAction, confluence *compiledConfluence) bool {
	if transaction == nil || transaction.canceled() || action == nil || confluence == nil || action.coordinate != confluence.coordinate || transaction.solver == nil || transaction.solver.facts == nil || transaction.guards == nil {
		return false
	}
	// Rebuild this coordinate from its entry seed, local Program edges and
	// explicit relation contributions.  Every contribution is one Facts root;
	// there is no vector carrier or ambient cross-boundary transport.
	destination := transaction.solver.zero
	if transaction.entryCoordinate(confluence.coordinate) {
		destination = transaction.solver.entry
	}
	transaction.contributions = transaction.contributions[:0]
	for _, edge := range confluence.incoming {
		if transaction.canceled() {
			return false
		}
		source, ok := transaction.rootAt(edge.input)
		if !ok {
			return false
		}
		when, ok := transaction.edgeSupport(edge)
		if !ok {
			return false
		}
		restricted, ok := transaction.solver.facts.Restrict(source.value, when)
		if !ok {
			return false
		}
		contribution, ok := transaction.applyLocal(action, edge.input, restricted, edge.rules, false)
		if !ok {
			return false
		}
		if len(edge.reset) != 0 {
			contribution, ok = transaction.discharge(contribution, edge.reset)
			if !ok {
				return false
			}
		}
		transaction.contributions = append(transaction.contributions, contribution)
	}
	for index := range confluence.relations {
		if transaction.canceled() {
			return false
		}
		relation := &confluence.relations[index]
		region, ok := transaction.relationSupport(*relation)
		if !ok {
			return false
		}
		contribution, ok := transaction.applyRelation(action, relation, region)
		if !ok {
			return false
		}
		if len(relation.reset) != 0 {
			contribution, ok = transaction.discharge(contribution, relation.reset)
			if !ok {
				return false
			}
		}
		transaction.contributions = append(transaction.contributions, contribution)
	}
	for _, contribution := range transaction.contributions {
		if transaction.canceled() {
			return false
		}
		var ok bool
		destination, ok = transaction.solver.facts.Join(destination, contribution)
		if !ok {
			return false
		}
	}
	return transaction.replaceRoot(action, confluence.coordinate, destination)
}

func (transaction *transaction) runLocal(action *compiledAction, member ruleDeclaration) bool {
	if transaction == nil || transaction.canceled() || action == nil || !action.coordinate.Valid() || member.apply == nil || transaction.solver == nil || transaction.solver.facts == nil {
		return false
	}
	source, ok := transaction.rootAt(action.coordinate)
	if !ok {
		return false
	}
	// An absent coordinate is not a bottom-valued activation.
	if source.value.Support().Equal(transaction.solver.zero.Support()) {
		return true
	}
	next, ok := transaction.applyLocal(action, action.coordinate, source.value, []ruleDeclaration{member}, true)
	return ok && transaction.replaceRoot(action, action.coordinate, next)
}

// applyLocal uses ordinary Product, whose designated output begins as the
// source Facts root. This is the sole identity transport path and it is confined
// to one existing Program activation.
func (transaction *transaction) applyLocal(action *compiledAction, input coordinate.Coordinate, source facts.Facts, members []ruleDeclaration, current bool) (facts.Facts, bool) {
	if transaction == nil || transaction.canceled() || action == nil || !input.Valid() || !action.coordinate.Valid() {
		return facts.Facts{}, false
	}
	// A Program edge with no bound domain Rule is the identity on the one
	// joint carrier. It must not enter cache/Product: there is no callback,
	// read projection, draft, or semantic output to retain. Besides avoiding
	// needless work, this preserves identity even when output and sole input
	// name the same guarded root.
	if len(members) == 0 {
		return source, true
	}
	inputs := transaction.productInputs(1)
	origins := transaction.productOrigins(1)
	inputs[0], origins[0] = source, input
	defer clear(inputs)
	defer clear(origins)
	result := transaction.solver.zero
	ok := transaction.solver.facts.Product(inputs, func(region support.Mask, tuple facts.Tuple) bool {
		base, valid := tuple.Input(0)
		if !valid {
			return false
		}
		row, present, valid := transaction.applyRow(action, origins, region, base, tuple, current, nil, members)
		if !valid || !present {
			return valid
		}
		result, valid = transaction.solver.facts.Join(result, row)
		return valid
	})
	return result, ok
}

// applyRelation constructs a fresh contribution from every ordered relation
// input.  ProductContribution starts from the canonical all-default vector;
// no Factor can cross unless the target Rule explicitly writes or Carries it.
func (transaction *transaction) applyRelation(action *compiledAction, relation *compiledRelation, region support.Mask) (facts.Facts, bool) {
	if transaction == nil || transaction.canceled() || action == nil || relation == nil || relation.rule.apply == nil || len(relation.inputs) == 0 || len(relation.inputs) != len(relation.relation.inputs) || !region.Valid() || transaction.solver == nil || transaction.solver.facts == nil {
		return facts.Facts{}, false
	}
	inputs := transaction.productInputs(len(relation.inputs))
	origins := transaction.productOrigins(len(relation.inputs))
	defer clear(inputs)
	defer clear(origins)
	for index, input := range relation.inputs {
		if transaction.canceled() {
			return facts.Facts{}, false
		}
		source, ok := transaction.rootAt(input)
		if !ok {
			return facts.Facts{}, false
		}
		restricted, ok := transaction.solver.facts.Restrict(source.value, region)
		if !ok {
			return facts.Facts{}, false
		}
		inputs[index], origins[index] = restricted, input
	}
	result := transaction.solver.zero
	ok := transaction.solver.facts.Product(inputs, func(rowRegion support.Mask, tuple facts.Tuple) bool {
		base, valid := transaction.solver.facts.Restrict(transaction.solver.entry, rowRegion)
		if !valid {
			return false
		}
		row, present, valid := transaction.applyRow(action, origins, rowRegion, base, tuple, false, &relation.relation, []ruleDeclaration{relation.rule})
		if !valid || !present {
			return valid
		}
		result, valid = transaction.solver.facts.Join(result, row)
		return valid
	})
	return result, ok
}

// applyRow owns exactly one Facts Product callback lifetime.  Its input tuple
// is already restricted to the atomic symbolic row; patches stage all writes
// privately and are attached only after every Rule in the row accepts.
func (transaction *transaction) applyRow(action *compiledAction, origins []coordinate.Coordinate, region support.Mask, output facts.Facts, inputs facts.Tuple, current bool, relation *activeRelation, members []ruleDeclaration) (facts.Facts, bool, bool) {
	if transaction == nil || transaction.canceled() || transaction.solver == nil || transaction.solver.facts == nil || action == nil || !region.Valid() || !transaction.solver.facts.Valid(output) || len(origins) != inputs.Len() {
		return facts.Facts{}, false, false
	}
	patches := newExecutionPatches(output)
	execution, ok := transaction.openExecution(ruleExecution{
		transaction: transaction, equation: action.equation, output: action.coordinate,
		origins: origins, current: current, region: region,
		outputFacts: output, inputs: inputs, patches: patches, relation: relation,
	})
	if !ok {
		patches.discard()
		return facts.Facts{}, false, false
	}
	defer transaction.closeExecution(execution)
	for _, member := range members {
		if member.apply == nil || !member.apply(execution) {
			patches.discard()
			return facts.Facts{}, false, false
		}
		if execution.pruned {
			patches.discard()
			return facts.Facts{}, false, true
		}
	}
	next, _, ok := patches.commit()
	if !ok || !transaction.solver.facts.Valid(next) {
		return facts.Facts{}, false, false
	}
	return next, true, true
}

// openExecution borrows the transaction's Product frame. Rule callbacks take
// their own distinct epoch inside it; this frame remains private throughout.
// The explicit busy bit turns any future nested Product attempt into a failed
// transaction rather than aliasing two terminal frames.
func (transaction *transaction) openExecution(next ruleExecution) (*ruleExecution, bool) {
	if transaction == nil || transaction.executing {
		return nil, false
	}
	transaction.execution = next
	transaction.executing = true
	return &transaction.execution, true
}

func (transaction *transaction) closeExecution(execution *ruleExecution) {
	if transaction == nil || execution == nil || execution != &transaction.execution || !transaction.executing {
		return
	}
	transaction.execution = ruleExecution{}
	transaction.executing = false
}

// observeRead records the exact source plane/key observed by the current
// action under this Product row. The log is action-private until dispatch
// seals it, so a failed callback cannot replace a prior dependency set.
func (transaction *transaction) observeRead(source coordinate.Coordinate, factor uint32, key uint64, region support.Mask) bool {
	return transaction != nil && transaction.readLog != nil && transaction.readLog.Read(source, factor, key, region)
}

// observePlane records Carry's whole-plane dependency. It intentionally does
// not enumerate keys: a later whole-plane delta is its exact invalidator.
func (transaction *transaction) observePlane(source coordinate.Coordinate, factor uint32, region support.Mask) bool {
	return transaction != nil && transaction.readLog != nil && transaction.readLog.Plane(source, factor, region)
}

func (execution *ruleExecution) openAccess(rule *ruleIdentity) (uint64, bool) {
	if execution == nil || execution.transaction == nil || !execution.transaction.executing || execution.rule != nil || rule == nil {
		return 0, false
	}
	// An epoch is a capability identity, never a cyclic generation counter.
	// Exhaustion fails the transaction closed rather than allowing an ancient
	// retained value to collide with a future callback after wraparound.
	if execution.transaction.accessEpoch == ^uint64(0) {
		return 0, false
	}
	execution.transaction.accessEpoch++
	execution.epoch = execution.transaction.accessEpoch
	execution.rule = rule
	execution.carried = false
	return execution.epoch, true
}

func (execution *ruleExecution) closeAccess(epoch uint64) {
	if execution == nil || execution.epoch != epoch {
		return
	}
	execution.epoch = 0
	execution.rule = nil
	execution.carried = false
}

func (transaction *transaction) productInputs(length int) []facts.Facts {
	if transaction == nil || length < 0 {
		return nil
	}
	if cap(transaction.relationInput) < length {
		transaction.relationInput = make([]facts.Facts, length)
	} else {
		transaction.relationInput = transaction.relationInput[:length]
	}
	return transaction.relationInput
}

func (transaction *transaction) productOrigins(length int) []coordinate.Coordinate {
	if transaction == nil || length < 0 {
		return nil
	}
	if cap(transaction.relationOrigin) < length {
		transaction.relationOrigin = make([]coordinate.Coordinate, length)
	} else {
		transaction.relationOrigin = transaction.relationOrigin[:length]
	}
	return transaction.relationOrigin
}

func (transaction *transaction) discharge(root facts.Facts, atoms []guard.Atom) (facts.Facts, bool) {
	if transaction == nil || transaction.solver == nil || transaction.solver.facts == nil || !transaction.solver.facts.Valid(root) {
		return facts.Facts{}, false
	}
	next := root
	for _, atom := range atoms {
		if atom == 0 {
			return facts.Facts{}, false
		}
		var ok bool
		next, ok = transaction.solver.facts.Mu(next, atom)
		if !ok {
			return facts.Facts{}, false
		}
	}
	return next, true
}

func (transaction *transaction) edgeSupport(edge compiledEdge) (support.Mask, bool) {
	if transaction == nil || transaction.guards == nil {
		return support.Mask{}, false
	}
	if edge.atom == 0 {
		return support.FromGuard(transaction.guards, transaction.guards.True())
	}
	work := support.New(transaction.guards)
	if work == nil {
		return support.Mask{}, false
	}
	region, ok := work.Literal(edge.atom, edge.truthy)
	if !ok || !work.Seal() {
		work.Discard()
		return support.Mask{}, false
	}
	return region, true
}

func (transaction *transaction) rootAt(coordinate coordinate.Coordinate) (transactionRoot, bool) {
	if transaction == nil || transaction.solver == nil || !coordinate.Valid() {
		return transactionRoot{}, false
	}
	index, ok := transaction.solver.coordinate.Slot(coordinate)
	if !ok || index < 0 || index >= len(transaction.roots) || transaction.roots[index].coordinate != coordinate {
		return transactionRoot{}, false
	}
	return transaction.roots[index], true
}

// replaceRoot is the only ordinary coordinate publication route.
// Contributions are constructed quietly; this method performs exactly one
// whole-carrier replacement, and dispatches only exact Facts deltas through
// the epoch-private observation index. The outer tuple widening cut is
// separate and private to finishSweep.
func (transaction *transaction) replaceRoot(action *compiledAction, coordinate coordinate.Coordinate, desired facts.Facts) bool {
	if transaction == nil || transaction.solver == nil || transaction.solver.facts == nil || transaction.reads == nil || transaction.queue == nil || action == nil || action.index < 0 || action.index >= len(transaction.solver.actions) || !coordinate.Valid() || !transaction.solver.facts.Valid(desired) {
		return false
	}
	index, ok := transaction.solver.coordinate.Slot(coordinate)
	if !ok || index < 0 || index >= len(transaction.roots) || transaction.roots[index].coordinate != coordinate {
		return false
	}
	prior := transaction.roots[index].value
	if transaction.solver.facts.Equal(prior, desired) {
		return true
	}
	supportChanged := false
	if !transaction.solver.facts.Delta(prior, desired, func(change facts.Delta) bool {
		if change.IsSupport() {
			supportChanged = true
		}
		return transaction.reads.Dispatch(coordinate, change, func(equation observation.Equation) bool {
			return transaction.queue.Seed(equation)
		})
	}) {
		return false
	}
	transaction.roots[index].value = desired
	return !supportChanged || transaction.markPresenceFollowers(action.index)
}

// markPresenceFollowers wakes static equations whose input support changed.
// It deliberately has no Factor vocabulary: exact value dependencies are
// recorded dynamically by the Rule access path and wake through the ordinary
// dependency index instead.
func (transaction *transaction) markPresenceFollowers(source int) bool {
	if transaction == nil || transaction.solver == nil || source < 0 || source >= len(transaction.solver.presenceFollowers) || len(transaction.dirty) != len(transaction.solver.actions) {
		return false
	}
	for _, target := range transaction.solver.presenceFollowers[source] {
		if target < 0 || target >= len(transaction.dirty) {
			return false
		}
		transaction.dirty[target] = true
	}
	return true
}

// markRegion dirties the exact action members of one original SCC and their
// already-compiled action successors after its correlated tuple advanced.
// It walks no Program relation and constructs no auxiliary graph; members and
// followers are immutable projections of the one action graph compiled at
// Seal. Marking every member is what makes the next pass a complete sweep
// rather than a head-triggered or FIFO-defined partial iteration.
func (transaction *transaction) markRegion(region compiledRegion) bool {
	if transaction == nil || transaction.solver == nil || !region.outer || len(region.members) == 0 || len(transaction.dirty) != len(transaction.solver.actions) {
		return false
	}
	for _, action := range region.members {
		if action < 0 || action >= len(transaction.dirty) {
			return false
		}
		transaction.dirty[action] = true
		if !transaction.markPresenceFollowers(action) {
			return false
		}
	}
	return true
}

func (solver *Solver) validState(state *State) bool {
	// This is the public State provenance gate, deliberately shallow and
	// constant-time. State is private-field constructed only at publication;
	// its exact selected root and all reachable terminal vectors are validated
	// together at that terminal cut, before a Query can observe either.
	return state != nil && solver != nil && solver.owner != nil && solver.link != nil &&
		state.owner == solver.owner && state.content == solver.link.ContentID()
}

func (transaction *transaction) publish() (*State, bool) {
	if transaction == nil || transaction.solver == nil || transaction.solver.facts == nil || transaction.queue == nil || transaction.canceled() {
		return nil, false
	}
	// State retains only values materialized through the compiler-sealed
	// query layout. Facts, supports, and all scheduler/observation machinery
	// remain epoch-local and are discarded immediately below.
	nextRoots := make([]stateRoot, len(transaction.solver.queryRoots))
	for index, coordinate := range transaction.solver.queryRoots {
		slot, ok := transaction.solver.coordinate.Slot(coordinate)
		if !ok || slot < 0 || slot >= len(transaction.roots) || transaction.roots[slot].coordinate != coordinate {
			transaction.abort()
			return nil, false
		}
		root := transaction.roots[slot]
		projections := transaction.solver.queryResults[index]
		results := make([]stateResult, len(projections))
		for resultSlot, projection := range projections {
			if projection.coordinate != coordinate || projection.resultSlot != resultSlot || projection.materialize == nil {
				transaction.abort()
				return nil, false
			}
			value, present := projection.materialize(root.value)
			results[resultSlot] = stateResult{value: value, present: present}
		}
		nextRoots[index] = stateRoot{coordinate: coordinate, results: results}
	}
	transaction.clear()
	return &State{
		owner:   transaction.solver.owner,
		content: transaction.solver.link.ContentID(),
		roots:   nextRoots,
	}, true
}

func (transaction *transaction) abort() {
	if transaction == nil {
		return
	}
	if transaction.queue != nil {
		transaction.queue.Discard()
	}
	transaction.clear()
}

func (transaction *transaction) clear() {
	if transaction == nil {
		return
	}
	if transaction.readLog != nil {
		transaction.readLog.Discard()
		transaction.readLog = nil
	}
	if transaction.activationWork != nil {
		transaction.activationWork.Discard()
		transaction.activationWork = nil
	}
	clear(transaction.roots)
	clear(transaction.contributions)
	clear(transaction.relationInput)
	clear(transaction.relationOrigin)
	for index := range transaction.relationFrames {
		transaction.relationFrames[index].expire()
	}
	for _, relation := range transaction.relationOverflow {
		if relation != nil {
			relation.expire()
		}
	}
	clear(transaction.relationTerms)
	transaction.execution = ruleExecution{}
	transaction.executing = false
	clear(transaction.dirty)
	clear(transaction.supports)
	clear(transaction.activationNext)
	clear(transaction.discovered)
	for index := range transaction.sweeps {
		clear(transaction.sweeps[index].prior)
		clear(transaction.sweeps[index].supports)
		clear(transaction.sweeps[index].nextSupports)
	}
	transaction.roots = nil
	transaction.contributions = nil
	transaction.relationInput = nil
	transaction.relationOrigin = nil
	transaction.relationOverflow = nil
	transaction.relationDepth = 0
	transaction.relationTerms = nil
	transaction.relationTermTop = 0
	transaction.dirty = nil
	transaction.sweeps = nil
	transaction.supports = nil
	transaction.activation = nil
	transaction.activationOutputs = nil
	transaction.activationNext = nil
	transaction.guards = nil
	transaction.reads = nil
	transaction.queue = nil
	transaction.done = nil
}

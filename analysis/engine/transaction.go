package engine

import (
	"context"

	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/dependency"
	"github.com/wippyai/go-lua/analysis/engine/internal/fiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/program/link"
)

// transaction is one private evaluation generation. Its queue, Guard work,
// typed Factor candidates, and root fibers are discarded on every failed
// generation and retained only through an immutable published State.
type transaction struct {
	solver *Solver
	queue  *dependency.Queue
	guards *guard.Work
	fibers *fiber.Work
	done   <-chan struct{}

	slots          []transactionSlot
	roots          []transactionRoot
	contributions  []fiber.Guarded
	relationInput  []fiber.Guarded
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
	bodyPending      *bodyAdmissionBatch

	// supports is the exact guard support of each immutable active relation.
	// A new relation is structural growth. Discovery stays open while this
	// immutable carrier settles so every selector already reachable in this
	// topology contributes to one canonical next-epoch batch; it does not
	// terminate the schedule at the first relation it sees.
	supports          []guard.Guard
	activation        []activationSource
	activationOutputs []compiledSupportOutput
	activationOpen    bool
	activationNext    []guard.Guard
	discovered        []activeRelation
	relationEpoch     uint64
	rebuild           bool
}

// transactionRoot is the private mutable carrier coordinate used only while
// one fixed-point generation is open. It is intentionally distinct from
// stateRoot: publication discards this Guarded topology after materializing
// the exact immutable terminal vectors a Query can observe.
type transactionRoot struct {
	coordinate coordinate.Coordinate
	value      fiber.Guarded
}

// regionSweep retains the prior correlated coordinate tuple while the one
// Schedule stream computes its next tuple. It is transaction scratch only:
// roots remain the sole candidate State, and no source/action graph is copied.
type regionSweep struct {
	prior    []fiber.Guarded
	supports []guard.Guard
	active   bool
	phase    sweepPhase
}

// sweepPhase is private evaluator control, not an engine/domain lifecycle
// noun. All phases run the same immutable action Schedule over the same root
// tuple; only the terminal tuple operator differs.
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

	mark := solver.coordinate.Mark()
	prior := solver.snapshotEpoch()
	for {
		// Every rebuilt epoch belongs to this one caller request. Do not begin
		// a new generation after its cancellation has become observable.
		if canceled(done) {
			solver.restoreEpoch(prior)
			solver.coordinate.Rewind(mark)
			return nil, false
		}
		transaction, ok := solver.beginEpoch(done)
		if !ok {
			solver.restoreEpoch(prior)
			solver.coordinate.Rewind(mark)
			return nil, false
		}
		if transaction.canceled() || !transaction.run() || transaction.canceled() {
			transaction.abort()
			solver.restoreEpoch(prior)
			solver.coordinate.Rewind(mark)
			return nil, false
		}
		if transaction.rebuild {
			if transaction.canceled() {
				transaction.abort()
				solver.restoreEpoch(prior)
				solver.coordinate.Rewind(mark)
				return nil, false
			}
			relations, valid := transaction.nextActiveEpoch()
			stopped := transaction.canceled()
			transaction.abort()
			// U is Link's sealed Candidate universe and E grows only by a new
			// exact (template, caller Coordinate, Candidate, selector) fact.
			// Rebuilding the disposable epoch from canonical Init is therefore
			// finite without a round bound, Go recursion, or a stale carrier.
			if stopped || canceled(done) || !valid || !solver.rebuildEpoch(relations) {
				solver.restoreEpoch(prior)
				solver.coordinate.Rewind(mark)
				return nil, false
			}
			continue
		}
		state, ok := transaction.publish()
		if !ok {
			solver.restoreEpoch(prior)
			solver.coordinate.Rewind(mark)
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
	if canceled(done) || solver == nil || len(solver.actions) == 0 || solver.guards == nil || solver.fibers == nil {
		return nil, false
	}
	queue := dependency.NewQueue(uint32(len(solver.actions)))
	if !queue.Open() {
		return nil, false
	}
	guards := solver.guards.NewWork()
	if guards == nil {
		queue.Discard()
		return nil, false
	}
	work, ok := solver.fibers.Begin(queue, guards)
	if !ok {
		guards.Discard()
		queue.Discard()
		return nil, false
	}
	// Pending body records are transactional admission storage only. Reuse
	// reads the Solver's one flat canonical cache, and these records become
	// visible only after this transaction freezes successfully.
	transaction := &transaction{solver: solver, queue: queue, guards: guards, fibers: work, done: done, bodyPending: newBodyAdmissionBatch()}
	if !transaction.openSlots(solver.initial) || !transaction.openRoots(solver.roots) || !transaction.openSupports() {
		transaction.abort()
		return nil, false
	}
	return transaction, true
}

func (transaction *transaction) openRoots(coordinates []coordinate.Coordinate) bool {
	if transaction == nil || transaction.solver == nil || transaction.fibers == nil || transaction.guards == nil {
		return false
	}
	transaction.roots = make([]transactionRoot, len(coordinates))
	for index, coordinate := range coordinates {
		if !coordinate.Valid() {
			return false
		}
		value := transaction.solver.fibers.Empty()
		if transaction.entryCoordinate(coordinate) {
			var ok bool
			value, ok = transaction.fibers.Under(transaction.guards.True(), transaction.solver.zero)
			if !ok {
				return false
			}
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
	if transaction == nil || transaction.canceled() || action == nil || action.index < 0 || action.index >= len(transaction.solver.actions) || action.run == nil || !action.coordinate.Valid() || !action.open(transaction) {
		return false
	}
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
	return transaction.finishActivation(action) && transaction.drainDirty()
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
		sweep.prior = make([]fiber.Guarded, len(region.slots))
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
		sweep.supports = make([]guard.Guard, len(region.supports))
	} else {
		sweep.supports = sweep.supports[:len(region.supports)]
	}
	for index, slot := range region.supports {
		if slot < 0 || slot >= len(transaction.supports) || !transaction.guards.Valid(transaction.supports[slot]) {
			return false
		}
		sweep.supports[index] = transaction.supports[slot]
	}
	sweep.active = true
	sweep.phase = phase
	return true
}

// finishSweep is the sole outer-SCC convergence transition. The region tuple
// is the product of its Fiber-root support topology and compiler-assigned
// relation supports. Values remain in those same Fiber leaves. A descent is
// accepted only through NarrowReplace; growth or incomparability returns the
// whole product to Widen. There is no independent reachability state.
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
		postfixed := true
		for index, slot := range region.slots {
			if slot < 0 || slot >= len(transaction.roots) || !transaction.postfixpoint(sweep.prior[index], transaction.roots[slot].value) {
				postfixed = false
				break
			}
		}
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
		// proves pointwise below the previous Fiber tuple. Its carrier
		// transition then uses Fiber.NarrowReplace: overlap applies typed
		// Narrow, while prior-only rows are removed atomically.
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

// sweepRelation is the one partial order for an outer SCC tuple.  It merges
// relation guard supports and Fiber support topology; Factor values are
// checked separately by the same typed postfixpoint witness before a descent
// invokes NarrowReplace.
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
		if !transaction.guards.Valid(prior) || !transaction.guards.Valid(desired) {
			return 0, false
		}
		descends := transaction.guards.Entails(desired, prior)
		grows := transaction.guards.Entails(prior, desired)
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
		topology, ok := transaction.fibers.SupportRelation(sweep.prior[index], transaction.roots[slot].value)
		if !ok {
			return 0, false
		}
		var descends, grows bool
		switch topology {
		case fiber.TopologyEqual:
		case fiber.TopologyDescent:
			descends = true
		case fiber.TopologyGrowth:
			grows = true
		case fiber.TopologyIncomparable:
			return sweepIncomparable, true
		default:
			return 0, false
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
		if slot < 0 || slot >= len(transaction.roots) || !transaction.postfixpoint(sweep.prior[index], transaction.roots[slot].value) {
			return false
		}
	}
	return true
}

func (transaction *transaction) widenSweepProduct(region compiledRegion, sweep *regionSweep) (bool, bool) {
	if transaction == nil || transaction.guards == nil {
		return false, false
	}
	advanced := false
	for index, slot := range region.supports {
		if slot < 0 || slot >= len(transaction.supports) || !transaction.guards.Valid(sweep.supports[index]) || !transaction.guards.Valid(transaction.supports[slot]) {
			return false, false
		}
		next := transaction.guards.Or(sweep.supports[index], transaction.supports[slot])
		if !transaction.guards.Valid(next) {
			return false, false
		}
		advanced = advanced || !transaction.guards.Equivalent(sweep.supports[index], next)
		transaction.supports[slot] = next
	}
	for index, slot := range region.slots {
		if slot < 0 || slot >= len(transaction.roots) {
			return false, false
		}
		prior, desired := sweep.prior[index], transaction.roots[slot].value
		next, ok := transaction.combineRoot(prior, desired, transaction.roots[slot].coordinate, true, func(declaration factorDeclaration, condition guard.Guard, draft *fiber.Draft, left, right fiber.Leaf) bool {
			return declaration.widen != nil && declaration.widen(transaction, transaction.roots[slot].coordinate, condition, draft, left, right)
		})
		if !ok {
			return false, false
		}
		advanced = advanced || next != prior
		transaction.roots[slot].value = next
	}
	return advanced, true
}

func (transaction *transaction) narrowSweepProduct(region compiledRegion, sweep *regionSweep) (bool, bool) {
	if transaction == nil || transaction.guards == nil {
		return false, false
	}
	advanced := false
	for index, slot := range region.slots {
		if slot < 0 || slot >= len(transaction.roots) {
			return false, false
		}
		prior, desired := sweep.prior[index], transaction.roots[slot].value
		next, ok := transaction.combineRoot(prior, desired, transaction.roots[slot].coordinate, true, func(declaration factorDeclaration, condition guard.Guard, draft *fiber.Draft, left, right fiber.Leaf) bool {
			return declaration.narrow != nil && declaration.narrow(transaction, transaction.roots[slot].coordinate, condition, draft, left, right)
		})
		if !ok {
			return false, false
		}
		advanced = advanced || next != prior
		transaction.roots[slot].value = next
	}
	for index, slot := range region.supports {
		if slot < 0 || slot >= len(transaction.supports) || !transaction.guards.Valid(transaction.supports[slot]) || !transaction.guards.Valid(sweep.supports[index]) {
			return false, false
		}
		advanced = advanced || !transaction.guards.Equivalent(sweep.supports[index], transaction.supports[slot])
	}
	return advanced, true
}

// replaceSweepProduct applies a proven paired support descent exactly. The
// body result has already established desired ≤ prior for every Factor row;
// Fiber.NarrowReplace narrows overlap rows, removes a prior-only row, and
// rejects a right-only row. A left-oriented Zip/Narrow would retain such a
// row and make the smaller support claim false. This remains one carrier
// route; it does not introduce a second state representation.
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
		next, valid := transaction.narrowReplaceCarrier(prior, desired, transaction.roots[slot].coordinate)
		if !valid {
			return false, false
		}
		advanced = advanced || next != prior
		transaction.roots[slot].value = next
	}
	for index, slot := range region.supports {
		if slot < 0 || slot >= len(transaction.supports) || !transaction.guards.Valid(sweep.supports[index]) || !transaction.guards.Valid(transaction.supports[slot]) {
			return false, false
		}
		advanced = advanced || !transaction.guards.Equivalent(sweep.supports[index], transaction.supports[slot])
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

// postfixpoint proves desired ⊑ prior for one complete correlated Fiber.
// Fiber.Replace already visits exactly the unequal compatible terminal pairs,
// including an absent support paired with the canonical zero vector. This
// uses its pair traversal only as a read-only relation witness: no root,
// Factor page, dependency log, or queue entry is constructed here.
func (transaction *transaction) postfixpoint(prior, desired fiber.Guarded) bool {
	if transaction == nil || transaction.fibers == nil {
		return false
	}
	proven := true
	_, _, _, valid := transaction.fibers.Replace(prior, desired, transaction.solver.zero, func(_ guard.Guard, left, right fiber.Leaf) (bool, bool) {
		for _, declaration := range transaction.solver.factors {
			if declaration.lessOrEq == nil {
				return false, false
			}
			// Replace presents prior as left and desired as right. A verified
			// postfixpoint requires desired ≤ prior in every complete leaf.
			ordered, ok := declaration.lessOrEq(transaction, right, left)
			if !ok {
				return false, false
			}
			if !ordered {
				proven = false
			}
		}
		return false, true
	})
	return valid && proven
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
	if transaction == nil || transaction.canceled() || action == nil || confluence == nil || action.coordinate != confluence.coordinate || transaction.fibers == nil || transaction.guards == nil {
		return false
	}
	// Rebuild this coordinate from its explicit root seed, local Program
	// edges, and declared n-ary relation contributions.  No previous vector
	// supplies an ambient cross-boundary transport path.
	destination := transaction.solver.fibers.Empty()
	if transaction.entryCoordinate(confluence.coordinate) {
		var ok bool
		destination, ok = transaction.fibers.Under(transaction.guards.True(), transaction.solver.zero)
		if !ok {
			return false
		}
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
		when, ok := transaction.edgeGuard(edge)
		if !ok {
			return false
		}
		restricted, ok := transaction.fibers.Restrict(source.value, when)
		if !ok {
			return false
		}
		contribution, ok := transaction.applyLocal(action, edge.input, restricted, edge.rules, false, edge.reuse)
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
		support, ok := transaction.relationSupport(*relation)
		if !ok {
			return false
		}
		if transaction.guards.Equivalent(support, transaction.guards.False()) {
			continue
		}
		contribution, ok := transaction.applyRelation(action, relation, support)
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
		destination, ok = transaction.quietConfluence(destination, contribution)
		if !ok {
			return false
		}
	}
	return transaction.replaceRoot(action, confluence.coordinate, destination)
}

func (transaction *transaction) runLocal(action *compiledAction, member ruleDeclaration, reuse bodyProjection) bool {
	if transaction == nil || transaction.canceled() || action == nil || !action.coordinate.Valid() || member.apply == nil {
		return false
	}
	source, ok := transaction.rootAt(action.coordinate)
	if !ok {
		return false
	}
	// An absent coordinate is not a bottom-valued activation. Local rules may
	// not manufacture reachability without a Program edge, explicit relation,
	// or explicitly demanded root seed.
	if !transaction.fibers.Present(source.value) {
		return true
	}
	next, ok := transaction.applyLocal(action, action.coordinate, source.value, []ruleDeclaration{member}, true, reuse)
	return ok && transaction.replaceRoot(action, action.coordinate, next)
}

// applyLocal uses ordinary Product, whose designated output begins as the
// source Fiber.  This is the sole identity transport path and it is confined
// to one existing Program activation.
func (transaction *transaction) applyLocal(action *compiledAction, input coordinate.Coordinate, source fiber.Guarded, members []ruleDeclaration, current bool, reuse bodyProjection) (fiber.Guarded, bool) {
	if transaction == nil || transaction.canceled() || action == nil || !input.Valid() || !action.coordinate.Valid() {
		return fiber.Guarded{}, false
	}
	// A Program edge with no bound domain Rule is the identity on the one
	// joint carrier. It must not enter cache/Product: there is no callback,
	// read projection, draft, or semantic output to retain. Besides avoiding
	// needless work, this preserves identity even when output and sole input
	// name the same guarded root.
	if len(members) == 0 {
		return source, true
	}
	if cached, reused, valid := transaction.reuseBodyProjection(reuse, action, input, source); !valid {
		return fiber.Guarded{}, false
	} else if reused {
		return cached, true
	}
	capture := transaction.beginBodyProjection(reuse, source)
	inputs := transaction.productInputs(1)
	origins := transaction.productOrigins(1)
	inputs[0], origins[0] = source, input
	defer clear(inputs)
	defer clear(origins)
	result, ok := transaction.fibers.Product(source, inputs, func(condition guard.Guard, outputLeaf fiber.Leaf, tuple fiber.Tuple) (fiber.ProductResult, bool) {
		if transaction.canceled() {
			return fiber.ProductResult{}, false
		}
		execution, valid := transaction.openExecution(ruleExecution{
			transaction: transaction, equation: action.equation,
			output: action.coordinate, origins: origins, condition: condition, current: current,
			outputLeaf: outputLeaf, inputs: tuple, capture: capture,
		})
		if !valid {
			return fiber.ProductResult{}, false
		}
		for _, member := range members {
			if member.apply == nil || !member.apply(execution) {
				transaction.closeExecution(execution)
				return fiber.ProductResult{}, false
			}
			if execution.pruned {
				transaction.closeExecution(execution)
				return fiber.Prune(), true
			}
		}
		if execution.draft == nil {
			transaction.closeExecution(execution)
			return fiber.Keep(), true
		}
		result := fiber.Replace(execution.draft)
		transaction.closeExecution(execution)
		return result, true
	})
	if !ok || !transaction.retainBodyProjection(capture, result) {
		return fiber.Guarded{}, false
	}
	return result, true
}

// applyRelation constructs a fresh contribution from every ordered relation
// input.  ProductContribution starts from the canonical all-default vector;
// no Factor can cross unless the target Rule explicitly writes or Carries it.
func (transaction *transaction) applyRelation(action *compiledAction, relation *compiledRelation, support guard.Guard) (fiber.Guarded, bool) {
	if transaction == nil || transaction.canceled() || action == nil || relation == nil || relation.rule.apply == nil || len(relation.inputs) == 0 || len(relation.inputs) != len(relation.relation.inputs) || !transaction.guards.Valid(support) {
		return fiber.Guarded{}, false
	}
	inputs := transaction.productInputs(len(relation.inputs))
	origins := transaction.productOrigins(len(relation.inputs))
	defer clear(inputs)
	defer clear(origins)
	for index, input := range relation.inputs {
		if transaction.canceled() {
			return fiber.Guarded{}, false
		}
		source, ok := transaction.rootAt(input)
		if !ok {
			return fiber.Guarded{}, false
		}
		restricted, ok := transaction.fibers.Restrict(source.value, support)
		if !ok {
			return fiber.Guarded{}, false
		}
		inputs[index], origins[index] = restricted, input
	}
	return transaction.fibers.ProductContribution(inputs, func(condition guard.Guard, outputLeaf fiber.Leaf, tuple fiber.Tuple) (fiber.ProductResult, bool) {
		if transaction.canceled() {
			return fiber.ProductResult{}, false
		}
		execution, valid := transaction.openExecution(ruleExecution{
			transaction: transaction, equation: action.equation,
			output: action.coordinate, origins: origins, condition: condition,
			current: false, outputLeaf: outputLeaf, inputs: tuple, relation: &relation.relation,
		})
		if !valid {
			return fiber.ProductResult{}, false
		}
		if !relation.rule.apply(execution) {
			transaction.closeExecution(execution)
			return fiber.ProductResult{}, false
		}
		if execution.pruned {
			transaction.closeExecution(execution)
			return fiber.Prune(), true
		}
		if execution.draft == nil {
			transaction.closeExecution(execution)
			return fiber.Keep(), true
		}
		result := fiber.Replace(execution.draft)
		transaction.closeExecution(execution)
		return result, true
	})
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

func (transaction *transaction) productInputs(length int) []fiber.Guarded {
	if transaction == nil || length < 0 {
		return nil
	}
	if cap(transaction.relationInput) < length {
		transaction.relationInput = make([]fiber.Guarded, length)
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

func (transaction *transaction) quietConfluence(leftRoot, rightRoot fiber.Guarded) (fiber.Guarded, bool) {
	if transaction == nil || transaction.fibers == nil {
		return fiber.Guarded{}, false
	}
	next, ok := transaction.fibers.Zip(leftRoot, rightRoot, false, func(condition guard.Guard, left, right fiber.Leaf) (*fiber.Draft, bool) {
		draft, ok := transaction.fibers.EditLeaf(left)
		if !ok {
			return nil, false
		}
		for _, declaration := range transaction.solver.factors {
			if declaration.joinContribution == nil || !declaration.joinContribution(transaction, draft, left, right) {
				return nil, false
			}
		}
		return draft, true
	})
	return next, ok
}

func (transaction *transaction) discharge(root fiber.Guarded, atoms []guard.Atom) (fiber.Guarded, bool) {
	if transaction == nil || transaction.fibers == nil {
		return fiber.Guarded{}, false
	}
	return transaction.fibers.Discharge(root, atoms, func(left, right fiber.Leaf) (*fiber.Draft, bool) {
		draft, ok := transaction.fibers.EditLeaf(left)
		if !ok {
			return nil, false
		}
		for _, declaration := range transaction.solver.factors {
			if declaration.joinContribution == nil || !declaration.joinContribution(transaction, draft, left, right) {
				return nil, false
			}
		}
		return draft, true
	})
}

// edgeGuard returns the exact Program edge clause in this transaction's
// sealed root decision universe. Unguarded Program Edges leave the source
// fiber unchanged; conditional Edges select the existing literal/polarity.
func (transaction *transaction) edgeGuard(edge compiledEdge) (guard.Guard, bool) {
	if transaction == nil || transaction.guards == nil || !transaction.guards.Open() {
		return guard.Guard{}, false
	}
	if edge.atom == 0 {
		return transaction.guards.True(), true
	}
	literal, ok := transaction.guards.Literal(edge.atom)
	if !ok {
		return guard.Guard{}, false
	}
	if !edge.truthy {
		literal = transaction.guards.Not(literal)
	}
	return literal, transaction.guards.Valid(literal)
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
// whole-carrier replacement, streams exact Factor deltas, and dirties
// structural successors only for a semantic joint-row/support change. The
// outer tuple widening cut is separate and private to finishSweep.
func (transaction *transaction) replaceRoot(action *compiledAction, coordinate coordinate.Coordinate, desired fiber.Guarded) bool {
	if transaction == nil || transaction.solver == nil || transaction.fibers == nil || action == nil || action.index < 0 || action.index >= len(transaction.solver.actions) || !coordinate.Valid() || !transaction.fibers.Valid(desired) {
		return false
	}
	index, ok := transaction.solver.coordinate.Slot(coordinate)
	if !ok || index < 0 || index >= len(transaction.roots) || transaction.roots[index].coordinate != coordinate {
		return false
	}
	prior := transaction.roots[index].value
	next, _, supportChanged, valid := transaction.replaceCarrier(prior, desired, coordinate)
	if !valid {
		return false
	}
	transaction.roots[index].value = next
	// Dynamic Factor logs already wake the exact equations that actually read
	// or carry a value. Ordinary followers preserve Program control flow on a
	// changed carrier; relation-input followers are stricter and only need a
	// support addition/removal. Fiber reports both from this one replacement
	// traversal, so this is not a second reachability scan or fact plane.
	if prior == next {
		return true
	}
	if !transaction.markFollowers(action.index) {
		return false
	}
	if supportChanged && !transaction.markPresenceFollowers(action.index) {
		return false
	}
	return true
}

// replaceCarrier is the exact root replacement primitive for normal
// publication. It observes additions and removals against Fiber's canonical
// zero row before replacing the complete correlated carrier root.
func (transaction *transaction) replaceCarrier(prior, desired fiber.Guarded, coordinate coordinate.Coordinate) (fiber.Guarded, bool, bool, bool) {
	if transaction == nil || transaction.solver == nil || transaction.fibers == nil || !coordinate.Valid() {
		return fiber.Guarded{}, false, false, false
	}
	return transaction.fibers.Replace(prior, desired, transaction.solver.zero, func(condition guard.Guard, left, right fiber.Leaf) (bool, bool) {
		return transaction.observeCarrierChange(coordinate, condition, left, right)
	})
}

// narrowReplaceCarrier is the paired product-refinement counterpart of
// replaceCarrier. Fiber owns the one topology traversal: it rejects a
// right-only row, calls typed Narrow on every overlap, and reports a removed
// left-only row through the same exact Factor delta callback.
func (transaction *transaction) narrowReplaceCarrier(prior, desired fiber.Guarded, coordinate coordinate.Coordinate) (fiber.Guarded, bool) {
	if transaction == nil || transaction.solver == nil || transaction.fibers == nil || !coordinate.Valid() {
		return fiber.Guarded{}, false
	}
	return transaction.fibers.NarrowReplace(prior, desired, transaction.solver.zero, func(condition guard.Guard, draft *fiber.Draft, left, right fiber.Leaf) bool {
		for _, declaration := range transaction.solver.factors {
			if declaration.narrow == nil || !declaration.narrow(transaction, coordinate, condition, draft, left, right) {
				return false
			}
		}
		return true
	}, func(condition guard.Guard, left, right fiber.Leaf) (bool, bool) {
		return transaction.observeCarrierChange(coordinate, condition, left, right)
	})
}

func (transaction *transaction) observeCarrierChange(coordinate coordinate.Coordinate, condition guard.Guard, left, right fiber.Leaf) (bool, bool) {
	if transaction == nil || transaction.solver == nil || !coordinate.Valid() {
		return false, false
	}
	changed := false
	for _, declaration := range transaction.solver.factors {
		if declaration.changed == nil {
			return false, false
		}
		columnChanged, ok := declaration.changed(transaction, coordinate, condition, left, right)
		if !ok {
			return false, false
		}
		changed = changed || columnChanged
	}
	return changed, true
}

func (transaction *transaction) combineRoot(prior, desired fiber.Guarded, coordinate coordinate.Coordinate, force bool, apply func(factorDeclaration, guard.Guard, *fiber.Draft, fiber.Leaf, fiber.Leaf) bool) (fiber.Guarded, bool) {
	if transaction == nil || transaction.fibers == nil || apply == nil || !coordinate.Valid() {
		return fiber.Guarded{}, false
	}
	return transaction.fibers.Zip(prior, desired, force, func(condition guard.Guard, left, right fiber.Leaf) (*fiber.Draft, bool) {
		draft, ok := transaction.fibers.EditLeaf(left)
		if !ok {
			return nil, false
		}
		for _, declaration := range transaction.solver.factors {
			if !apply(declaration, condition, draft, left, right) {
				return nil, false
			}
		}
		return draft, true
	})
}

func (transaction *transaction) markFollowers(source int) bool {
	if transaction == nil || transaction.solver == nil || source < 0 || source >= len(transaction.solver.followers) || len(transaction.dirty) != len(transaction.solver.actions) {
		return false
	}
	for _, target := range transaction.solver.followers[source] {
		if target < 0 || target >= len(transaction.dirty) {
			return false
		}
		transaction.dirty[target] = true
	}
	return true
}

// markPresenceFollowers wakes relation equations whose input support changed.
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
		if !transaction.markFollowers(action) {
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

func (transaction *transaction) openSlots(slots []stateSlot) bool {
	if transaction == nil || len(slots) != len(transaction.solver.factors) {
		return false
	}
	transaction.slots = make([]transactionSlot, 0, len(slots))
	for _, slot := range slots {
		if slot.open == nil {
			return false
		}
		opened, ok := slot.open(transaction)
		if !ok {
			return false
		}
		transaction.slots = append(transaction.slots, opened)
	}
	return true
}

func (transaction *transaction) publish() (*State, bool) {
	if transaction == nil || transaction.solver == nil || transaction.queue == nil || transaction.fibers == nil {
		return nil, false
	}
	// The transaction needs the complete coordinate table while evaluating;
	// completed State owns only roots that an explicit Query can read.  Body
	// projection retention is independent and has already registered its own
	// exact root with this Work.
	for _, coordinate := range transaction.solver.queryRoots {
		slot, ok := transaction.solver.coordinate.Slot(coordinate)
		if !ok || slot < 0 || slot >= len(transaction.roots) || transaction.roots[slot].coordinate != coordinate || !transaction.fibers.Retain(transaction.roots[slot].value) {
			transaction.abort()
			return nil, false
		}
	}
	if !transaction.fibers.Prepare() {
		transaction.abort()
		return nil, false
	}
	for _, slot := range transaction.slots {
		if slot.prepare == nil || !slot.prepare(transaction.fibers) {
			transaction.abort()
			return nil, false
		}
	}
	if !transaction.fibers.Ready() {
		transaction.abort()
		return nil, false
	}
	// This is the last cancellation cut. FreezeTerminal commits every prepared
	// Factor root, dependency index, and retained Fiber atomically; once it
	// succeeds, that completed publication wins even if the caller cancels
	// concurrently afterwards.
	if transaction.canceled() {
		transaction.abort()
		return nil, false
	}
	frozen, ok := transaction.queue.FreezeTerminal()
	if !ok {
		transaction.abort()
		return nil, false
	}
	publication := transaction.fibers.Finalize(frozen)
	nextSlots := make([]stateSlot, len(transaction.slots))
	for index, slot := range transaction.slots {
		if slot.release == nil {
			panic("engine: missing Factor terminal release")
		}
		nextSlots[index] = slot.release(publication)
	}
	nextRoots := make([]stateRoot, len(transaction.solver.queryRoots))
	for index, coordinate := range transaction.solver.queryRoots {
		slot, ok := transaction.solver.coordinate.Slot(coordinate)
		if !ok || slot < 0 || slot >= len(transaction.roots) || transaction.roots[slot].coordinate != coordinate {
			panic("engine: invalid query publication root")
		}
		root := transaction.roots[slot]
		first, rest, present := publication.RootLeaves(root.value)
		nextRoots[index] = stateRoot{
			coordinate: coordinate,
			first:      first,
			rest:       rest,
			present:    present,
		}
	}
	// A body projection becomes reusable only at this terminal point. Its
	// retained Fiber root has passed the same Finalize and Publication.Root
	// ownership path as State roots, and every typed Factor slot above has
	// released its sealed dependency index. All earlier error branches abort
	// before this call, so no failed or partial generation can enter reuse.
	// Cache publication follows the same generation cut as Factor roots. The
	// Solver retains one flat cache for this carrier epoch; an empty solve does
	// not allocate or extend a cache path.
	transaction.solver.bodyReuse.promote(transaction.bodyPending, publication)
	// The released typed indexes are the Solver's next immutable dependency
	// baseline. They are operational input to a later transaction, not a
	// public State continuation capability.
	transaction.solver.initial = nextSlots
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
	for index := len(transaction.slots) - 1; index >= 0; index-- {
		if transaction.slots[index].discard != nil {
			transaction.slots[index].discard()
		}
	}
	if transaction.fibers != nil {
		transaction.fibers.Discard()
	}
	if transaction.guards != nil {
		transaction.guards.Discard()
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
	clear(transaction.slots)
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
	}
	transaction.slots = nil
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
	transaction.fibers = nil
	transaction.guards = nil
	transaction.queue = nil
	transaction.done = nil
	transaction.bodyPending = nil
}

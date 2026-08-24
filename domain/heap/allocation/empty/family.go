package empty

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// reducer is one row's judgment: the Heap world its predecessor left, extended
// with the fresh object this constructor allocates. The candidate is sealed
// into the value because the fold's answer is indexed by it; the type is the
// family's, so the call below is a static direct call.
type reducer struct{ key heapdomain.Key }

func (fold reducer) Reduce(predecessor heapdomain.Value, present bool) (heapdomain.Value, structure.ReductionOutcome) {
	if !present {
		// An unwritten predecessor is no world to extend. Whether that is a
		// candidate at all is this domain's judgment, which is why the fold
		// hands sparse absence over rather than hiding it.
		return heapdomain.Value{}, structure.NoCandidate
	}
	return heapdomain.EmptyAllocationFact(fold.key, predecessor)
}

// row is the sealed static half of one empty-constructor invocation.
type row struct {
	fold  reducer
	read  execution.ExactRead[heapdomain.DenseCoordinate, heapdomain.Value]
	write execution.CarryWrite[heapdomain.DenseCoordinate, heapdomain.Value]
}

type family struct{ rows []row }

func (sealed *family) NewExecutor(run *execution.Run) execution.Executor {
	if sealed == nil || run == nil {
		return nil
	}
	return &worker{family: sealed, run: run}
}

func (*family) InputCapacity() int  { return 1 }
func (*family) OutputCapacity() int { return 1 }

// worker is one epoch's reusable invocation lane. Both scratches live for the
// worker's lifetime, so a warm invocation opens its read cursor and its write
// transaction without allocating.
type worker struct {
	family *family
	run    *execution.Run
	reads  execution.Scratch[heapdomain.DenseCoordinate, heapdomain.Value]
	writes execution.Scratch[heapdomain.DenseCoordinate, heapdomain.Value]
}

func (lane *worker) Execute(frame execution.Frame, ticket execution.Ticket) (execution.Result, bool) {
	if lane == nil || lane.family == nil || !frame.Valid(ticket) || !lane.run.Owns(ticket) {
		return execution.Result{}, false
	}
	local, localOK := ticket.LocalOrdinal()
	if !localOK || uint64(local) >= uint64(len(lane.family.rows)) {
		return execution.Result{}, false
	}
	sealed := lane.family.rows[local]
	outcome := execution.FoldCarry(ticket, sealed.fold, sealed.read, &lane.reads, sealed.write, &lane.writes)
	if !ticket.Submit(outcome) {
		return execution.Result{}, false
	}
	count := 0
	if outcome == structure.Concrete {
		count = 1
	}
	return execution.NewResult(outcome, count)
}

// installer authors the execution family of the heap-empty rule. It is this
// package's, not the heap owner's: the constructor directory the candidate is
// resolved in and the transition the carry applies are this rule's knowledge,
// and an owner built from the heap schema alone could supply neither.
type installer struct {
	schema heapdomain.Schema
	rule   uint32
}

func (install installer) InstallRuleFamily(plane execution.FormPlane[heapdomain.DenseCoordinate, heapdomain.Value], ruleOrdinal uint32, rows []execution.FormRow) (execution.Family, []execution.FormAddress, bool) {
	if !install.schema.Valid() || ruleOrdinal != install.rule || !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}
	sealed := &family{rows: make([]row, 0, len(rows))}
	addresses := make([]execution.FormAddress, 0, len(rows))
	for _, planRow := range rows {
		if planRow.Form != execution.FormCarry {
			return nil, nil, false
		}
		key, keyOK := install.schema.EmptyAllocationAt(int(planRow.Candidate))
		if !keyOK {
			return nil, nil, false
		}
		read, readOK := plane.ExactRead(planRow.Unit, planRow.Input)
		// The transition is the allocation coordinate's own, applied to the
		// facts this row carries forward. It is a different map at every row,
		// which is why it is sealed against the row's candidate here.
		write, writeOK := plane.RowCarry(planRow, key.Age)
		if !readOK || !writeOK {
			return nil, nil, false
		}
		addresses = append(addresses, execution.FormAddress{Member: planRow.Member, Local: uint32(len(sealed.rows))})
		sealed.rows = append(sealed.rows, row{fold: reducer{key: key}, read: read, write: write})
	}
	return sealed, addresses, true
}

// InstallFamily is the generated lane's bind arm for this rule. The claim is
// made against the rule's own sealed ordinal and the Factor it writes to, at
// the one bind where the heap schema is in scope.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	owner := authorities.HeapAuthority()
	if owner == nil || !owner.Schema().Valid() {
		return false
	}
	ordinal, ordinalOK := slot.Ordinal()
	if !ordinalOK || ordinal > uint64(^uint32(0)) {
		return false
	}
	return engine.BindRuleFamily[heapdomain.DenseCoordinate](binding, slot, owner.FactorRef(), installer{schema: owner.Schema(), rule: uint32(ordinal)})
}

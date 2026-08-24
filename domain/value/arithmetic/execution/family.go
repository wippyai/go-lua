// Package execution owns binary arithmetic's temporary typed installation of
// a sealed rule Program. It contains no arithmetic judgment: Value owns that
// in ArithmeticValue, while the engine owns reads, writes, and scheduling.
//
// Scheduled death: delete this package when schema compilation emits the
// generic exact-product executor from the reducer contribution.
package execution

import (
	"github.com/wippyai/go-lua/analysis/engine"
	engineexecution "github.com/wippyai/go-lua/analysis/engine/execution"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type exactProductRow struct {
	candidate valuedomain.BinaryArithmetic
	left      engineexecution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
	right     engineexecution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
	write     engineexecution.ExactWrite[valuedomain.DenseCoordinate, valuedomain.Value]
}

type exactProductFamily struct {
	rows   []exactProductRow
	values *valuedomain.Schema
}

func (family *exactProductFamily) NewExecutor(run *engineexecution.Run) engineexecution.Executor {
	if family == nil || family.values == nil || run == nil {
		return nil
	}
	return &exactProductWorker{family: family, run: run}
}

func (*exactProductFamily) InputCapacity() int  { return 1 }
func (*exactProductFamily) OutputCapacity() int { return 1 }

type exactProductWorker struct {
	family *exactProductFamily
	run    *engineexecution.Run
	left   engineexecution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	right  engineexecution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	write  engineexecution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
}

func (worker *exactProductWorker) Execute(frame engineexecution.Frame, ticket engineexecution.Ticket) (engineexecution.Result, bool) {
	if worker == nil || worker.family == nil || worker.run == nil || !frame.Valid(ticket) || !worker.run.Owns(ticket) {
		return engineexecution.Result{}, false
	}
	local, ok := ticket.LocalOrdinal()
	if !ok || uint64(local) >= uint64(len(worker.family.rows)) {
		return engineexecution.Result{}, false
	}
	row := worker.family.rows[local]
	switch row.left.Read(ticket, &worker.left) {
	case engineexecution.ReadExhausted:
		if !row.left.Close(ticket, &worker.left) {
			return worker.finish(ticket, structure.Refuse, 0)
		}
		return worker.finish(ticket, structure.NoCandidate, 0)
	case engineexecution.ReadAvailable:
	default:
		_ = worker.left.Discard(ticket)
		return worker.finish(ticket, structure.Refuse, 0)
	}
	left, leftOK := worker.left.Value()
	leftPresent := worker.left.Present()
	leftRegion, leftRegionOK := worker.left.Region()
	if !row.left.Close(ticket, &worker.left) || !leftOK || !leftRegionOK || !worker.family.values.Equal(left, left) {
		return worker.finish(ticket, structure.Refuse, 0)
	}
	if !leftPresent {
		return worker.finish(ticket, structure.NoCandidate, 0)
	}
	switch row.right.Read(ticket, &worker.right) {
	case engineexecution.ReadExhausted:
		if !row.right.Close(ticket, &worker.right) {
			return worker.finish(ticket, structure.Refuse, 0)
		}
		return worker.finish(ticket, structure.NoCandidate, 0)
	case engineexecution.ReadAvailable:
	default:
		_ = worker.right.Discard(ticket)
		return worker.finish(ticket, structure.Refuse, 0)
	}
	right, rightOK := worker.right.Value()
	rightPresent := worker.right.Present()
	rightRegion, rightRegionOK := worker.right.Region()
	if !row.right.Close(ticket, &worker.right) || !rightOK || !rightRegionOK || !worker.family.values.Equal(right, right) {
		return worker.finish(ticket, structure.Refuse, 0)
	}
	if !rightPresent {
		return worker.finish(ticket, structure.NoCandidate, 0)
	}
	if !leftRegion.Equal(rightRegion) {
		return worker.finish(ticket, structure.Refuse, 0)
	}
	result, outcome := valuedomain.ArithmeticValue(row.candidate, left, right)
	if outcome == structure.Concrete {
		if !row.write.Stage(ticket, &worker.write, leftRegion, result) || !row.write.Close(ticket, &worker.write) {
			return worker.finish(ticket, structure.Refuse, 0)
		}
		return worker.finish(ticket, outcome, 1)
	}
	return worker.finish(ticket, outcome, 0)
}

func (worker *exactProductWorker) finish(ticket engineexecution.Ticket, outcome structure.ReductionOutcome, count int) (engineexecution.Result, bool) {
	if !ticket.Submit(outcome) {
		return engineexecution.Result{}, false
	}
	return engineexecution.NewResult(outcome, count)
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	ValueSchema() *valuedomain.Schema
}

type installer struct {
	values *valuedomain.Schema
	rule   uint32
}

func (install installer) InstallRuleFamily(plane engineexecution.FormPlane[valuedomain.DenseCoordinate, valuedomain.Value], ruleOrdinal uint32, rows []engineexecution.FormRow) (engineexecution.Family, []engineexecution.FormAddress, bool) {
	if install.values == nil || !install.values.Valid() || ruleOrdinal != install.rule || !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}
	sealed := &exactProductFamily{values: install.values, rows: make([]exactProductRow, 0, len(rows))}
	addresses := make([]engineexecution.FormAddress, 0, len(rows))
	for _, row := range rows {
		if row.Form != engineexecution.FormExact || !row.Rule.Available() || row.Rule.ReadCount() != 2 || row.Rule.OutputCount() != 1 {
			return nil, nil, false
		}
		first, firstOK := row.Rule.ReadAt(0)
		second, secondOK := row.Rule.ReadAt(1)
		output, outputOK := row.Rule.OutputAt(0)
		if !firstOK || !secondOK || !outputOK || first.Form != ruleprogram.Exact || second.Form != ruleprogram.Exact ||
			first.Factor != second.Factor || first.Input != 0 || second.Input != 0 || output.Mode != ruleprogram.ModeExact || output.Slot != 0 {
			return nil, nil, false
		}
		candidate, candidateOK := install.values.BinaryArithmeticAt(int(row.Candidate))
		foreign, foreignOK := plane.Foreign(first.Factor)
		if !candidateOK || !install.values.OwnsBinaryArithmetic(candidate) || !foreignOK {
			return nil, nil, false
		}
		left, leftOK := engineexecution.ForeignRowExactRead[valuedomain.DenseCoordinate, valuedomain.Value](foreign, row, 0)
		right, rightOK := engineexecution.ForeignRowExactRead[valuedomain.DenseCoordinate, valuedomain.Value](foreign, row, 1)
		write, writeOK := plane.ExactWrite(row.Target, uint16(output.Slot))
		if !leftOK || !rightOK || !writeOK {
			return nil, nil, false
		}
		addresses = append(addresses, engineexecution.FormAddress{Member: row.Member, Local: uint32(len(sealed.rows))})
		sealed.rows = append(sealed.rows, exactProductRow{candidate: candidate, left: left, right: right, write: write})
	}
	return sealed, addresses, true
}

// InstallFamily claims only the arithmetic rule ordinal. All coordinates are
// redeemed from the sealed FormRow and Value owner directory.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	owner := authorities.ValueAuthority()
	schema := authorities.ValueSchema()
	if owner == nil || schema == nil || !schema.Valid() || owner.Schema() != schema {
		return false
	}
	ordinal, ok := slot.Ordinal()
	if !ok || ordinal > uint64(^uint32(0)) {
		return false
	}
	return engine.BindRuleFamily[valuedomain.DenseCoordinate](binding, slot, owner.FactorRef(), installer{values: schema, rule: uint32(ordinal)})
}

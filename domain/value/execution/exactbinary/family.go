// Package exactbinary owns Value's shared same-axis exact-binary execution
// family. Arithmetic, equality, and order all have the same sealed program
// geometry; their generated owner dispatch is the only semantic distinction.
//
// The installer is deliberately cold: it authenticates the reducer and its
// candidate through Value's generated directory and retains only dense
// ordinals. The worker never keeps a candidate map, callback, or owner
// capability; it redeems the already-authenticated ordinals through the
// generated Schema dispatch at execution time.
package exactbinary

import (
	"github.com/wippyai/go-lua/analysis/engine"
	engineexecution "github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/generated"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// exactBinaryRow is the immutable invocation geometry for one candidate.
// reducer and candidate are owner-issued dense addresses; the typed read and
// write descriptors are sealed from the same Value Factor plane.
type exactBinaryRow struct {
	reducer   uint32
	candidate uint32
	left      engineexecution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
	right     engineexecution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
	write     engineexecution.ExactWrite[valuedomain.DenseCoordinate, valuedomain.Value]
}

// family is one shared implementation for every Value same-axis exact-binary
// reducer. No arithmetic/equality/order-specific family is needed: the
// reducer ordinal selects the generated owner dispatch.
type family struct {
	rows   []exactBinaryRow
	values *valuedomain.Schema
}

func (sealed *family) NewExecutor(run *engineexecution.Run) engineexecution.Executor {
	if sealed == nil || sealed.values == nil || !sealed.values.Valid() || run == nil {
		return nil
	}
	return &worker{family: sealed, run: run}
}

func (*family) InputCapacity() int  { return 1 }
func (*family) OutputCapacity() int { return 1 }

// worker is an epoch-local invocation lane. Its scratch values are allocated
// once by NewExecutor and reused by every warm invocation.
type worker struct {
	family *family
	run    *engineexecution.Run
	left   engineexecution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	right  engineexecution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	write  engineexecution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
}

// Execute performs the exact-binary fold. Each read is stepped once and then
// closed before the next read or the owner dispatch. Sparse absence is the
// explicit NoCandidate case. Any structural failure leaves execution to the
// engine's fail-stop path; in particular a false owner dispatch is never
// converted into a submitted Refuse result.
func (lane *worker) Execute(frame engineexecution.Frame, ticket engineexecution.Ticket) (engineexecution.Result, bool) {
	if lane == nil || lane.family == nil || lane.family.values == nil || lane.run == nil ||
		!frame.Valid(ticket) || !lane.run.Owns(ticket) || ticket.InputCount() != 1 || ticket.OutputCount() != 1 {
		return engineexecution.Result{}, false
	}
	local, localOK := ticket.LocalOrdinal()
	if !localOK || uint64(local) >= uint64(len(lane.family.rows)) {
		return engineexecution.Result{}, false
	}
	row := lane.family.rows[local]
	candidate, candidateOK := ticket.CandidateOrdinal()
	if !candidateOK || candidate != row.candidate {
		return engineexecution.Result{}, false
	}

	if !row.left.Valid() || !row.right.Valid() || !row.write.Valid() {
		return engineexecution.Result{}, false
	}

	var left valuedomain.Value
	leftRegion, leftRegionOK := lane.left.Region()
	switch row.left.Read(ticket, &lane.left) {
	case engineexecution.ReadAvailable:
		var leftValueOK bool
		left, leftValueOK = lane.left.Value()
		leftPresent := lane.left.Present()
		leftRegion, leftRegionOK = lane.left.Region()
		if !leftRegionOK || !row.left.Close(ticket, &lane.left) {
			_ = lane.left.Discard(ticket)
			return engineexecution.Result{}, false
		}
		if !leftPresent {
			return lane.settle(ticket, structure.NoCandidate)
		}
		if !leftValueOK || !lane.family.values.Equal(left, left) {
			return engineexecution.Result{}, false
		}
	case engineexecution.ReadExhausted:
		if !row.left.Close(ticket, &lane.left) {
			_ = lane.left.Discard(ticket)
			return engineexecution.Result{}, false
		}
		return lane.settle(ticket, structure.NoCandidate)
	case engineexecution.ReadRefuse:
		_ = lane.left.Discard(ticket)
		return lane.settle(ticket, structure.Refuse)
	default:
		return engineexecution.Result{}, false
	}

	var right valuedomain.Value
	rightRegion, rightRegionOK := lane.right.Region()
	switch row.right.Read(ticket, &lane.right) {
	case engineexecution.ReadAvailable:
		var rightValueOK bool
		right, rightValueOK = lane.right.Value()
		rightPresent := lane.right.Present()
		rightRegion, rightRegionOK = lane.right.Region()
		if !rightRegionOK || !row.right.Close(ticket, &lane.right) {
			_ = lane.right.Discard(ticket)
			return engineexecution.Result{}, false
		}
		if !rightPresent {
			return lane.settle(ticket, structure.NoCandidate)
		}
		if !rightValueOK || !lane.family.values.Equal(right, right) {
			return engineexecution.Result{}, false
		}
	case engineexecution.ReadExhausted:
		if !row.right.Close(ticket, &lane.right) {
			_ = lane.right.Discard(ticket)
			return engineexecution.Result{}, false
		}
		return lane.settle(ticket, structure.NoCandidate)
	case engineexecution.ReadRefuse:
		_ = lane.right.Discard(ticket)
		return lane.settle(ticket, structure.Refuse)
	default:
		return engineexecution.Result{}, false
	}
	if !leftRegion.Equal(rightRegion) {
		return engineexecution.Result{}, false
	}

	result, outcome, dispatched := lane.family.values.ReduceExactBinary(row.reducer, row.candidate, left, right)
	if !dispatched {
		// Dispatch validity is a construction/runtime-structure fence, not a
		// domain refusal. The caller aborts the open Run on false execution.
		return engineexecution.Result{}, false
	}
	if !outcome.Available() {
		return engineexecution.Result{}, false
	}
	switch outcome {
	case structure.Concrete:
		if !lane.family.values.Equal(result, result) ||
			!row.write.Stage(ticket, &lane.write, leftRegion, result) ||
			!row.write.Close(ticket, &lane.write) {
			_ = lane.write.Discard(ticket)
			return engineexecution.Result{}, false
		}
		return lane.settle(ticket, structure.Concrete)
	case structure.Refuse, structure.NoCandidate:
		return lane.settle(ticket, outcome)
	default:
		// An exact binary row has no selected/opaque publication contract. A
		// generated owner returning one is a malformed dispatch result, not a
		// value to widen or invent locally.
		return engineexecution.Result{}, false
	}
}

func (lane *worker) settle(ticket engineexecution.Ticket, outcome structure.ReductionOutcome) (engineexecution.Result, bool) {
	if lane == nil || !outcome.Available() || !ticket.Submit(outcome) {
		return engineexecution.Result{}, false
	}
	count := 0
	if outcome == structure.Concrete {
		count = 1
	}
	return engineexecution.NewResult(outcome, count)
}

// installer owns the one generated Rule ordinal it claims. Value is the rule
// authority here because the reducer's candidate directory and fold are both
// Value-owned, while the output plane is supplied by the engine bind seam.
type installer struct {
	values *valuedomain.Schema
	rule   uint32
}

func (install installer) InstallRuleFamily(
	plane engineexecution.FormPlane[valuedomain.DenseCoordinate, valuedomain.Value],
	ruleOrdinal uint32,
	rows []engineexecution.FormRow,
) (engineexecution.Family, []engineexecution.FormAddress, bool) {
	if install.values == nil || !install.values.Valid() || ruleOrdinal != install.rule || !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}

	sealed := &family{values: install.values, rows: make([]exactBinaryRow, 0, len(rows))}
	addresses := make([]engineexecution.FormAddress, 0, len(rows))
	for _, planRow := range rows {
		if planRow.Member < 0 || !exactBinaryShape(planRow.Rule, planRow.Form) {
			return nil, nil, false
		}
		first, firstOK := planRow.Rule.ReadAt(0)
		second, secondOK := planRow.Rule.ReadAt(1)
		output, outputOK := planRow.Rule.OutputAt(0)
		if !firstOK || !secondOK || !outputOK ||
			first.Factor != second.Factor || first.Axis != second.Axis ||
			first.Factor != output.Factor || first.Axis != output.Axis ||
			first.Relation.Axis != output.Axis || second.Relation.Axis != output.Axis ||
			first.Key.Axis != output.Axis || second.Key.Axis != output.Axis {
			return nil, nil, false
		}

		reducer := planRow.Rule.Reducer()
		candidate := planRow.Rule.CandidateRelation()
		if reducer.Axis != output.Axis || candidate.Axis != output.Axis ||
			!valuedomain.SupportsExactBinaryReducer(reducer.Member) ||
			!install.values.ExactBinaryCandidateAvailable(reducer.Member, planRow.Candidate) {
			return nil, nil, false
		}

		foreign, foreignOK := plane.Foreign(first.Factor)
		left, leftOK := engineexecution.ForeignRowExactRead[valuedomain.DenseCoordinate, valuedomain.Value](foreign, planRow, 0)
		right, rightOK := engineexecution.ForeignRowExactRead[valuedomain.DenseCoordinate, valuedomain.Value](foreign, planRow, 1)
		write, writeOK := plane.ExactWrite(planRow.Target, uint16(output.Slot))
		if !foreignOK || !leftOK || !rightOK || !writeOK {
			return nil, nil, false
		}
		addresses = append(addresses, engineexecution.FormAddress{Member: planRow.Member, Local: uint32(len(sealed.rows))})
		sealed.rows = append(sealed.rows, exactBinaryRow{
			reducer: reducer.Member, candidate: planRow.Candidate,
			left: left, right: right, write: write,
		})
	}
	return sealed, addresses, true
}

func exactBinaryShape(rule generated.CompiledRule, form engineexecution.Form) bool {
	if !rule.Available() || form != engineexecution.FormExact || rule.InputCount() != 1 || rule.ReadCount() != 2 || rule.OutputCount() != 1 || !rule.CarryIdentity() || rule.CarryInput() != 0 {
		return false
	}
	first, firstOK := rule.ReadAt(0)
	second, secondOK := rule.ReadAt(1)
	output, outputOK := rule.OutputAt(0)
	if !firstOK || !secondOK || !outputOK || first.Form != ruleprogram.Exact || second.Form != ruleprogram.Exact ||
		first.Input != 0 || second.Input != 0 || output.Mode != ruleprogram.ModeExact || output.Slot != 0 ||
		!output.Exact || !output.Strong || first.Factor != second.Factor || first.Axis != second.Axis ||
		first.Factor != output.Factor || first.Axis != output.Axis ||
		first.Relation.Axis != output.Axis || second.Relation.Axis != output.Axis ||
		first.Key.Axis != output.Axis || second.Key.Axis != output.Axis {
		return false
	}
	if rule.Reducer().Axis != output.Axis || rule.CandidateRelation().Axis != output.Axis {
		return false
	}
	return exactContract(first) && exactContract(second)
}

func exactContract(read generated.ReadPlan) bool {
	return read.Contract.Order == ruleprogram.OrderCanonical &&
		read.Contract.Sparse == ruleprogram.SparseExplicit &&
		read.Contract.OnOpaque == ruleprogram.OnOpaqueRefuse &&
		read.Contract.Multiplicity == ruleprogram.MultiplicityOne &&
		!read.Denominator.Present
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	ValueSchema() *valuedomain.Schema
}

// InstallFamily claims one generated Value exact-binary rule. Arithmetic,
// equality, and order call this same installer; the owner dispatch's reducer
// ordinal is the only family-specific data retained by the sealed rows.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	owner := authorities.ValueAuthority()
	schema := authorities.ValueSchema()
	if owner == nil || schema == nil || !schema.Valid() || owner.Schema() != schema {
		return false
	}
	ordinal, ordinalOK := slot.Ordinal()
	if !ordinalOK || ordinal > uint64(^uint32(0)) {
		return false
	}
	return engine.BindRuleFamily[valuedomain.DenseCoordinate](binding, slot, owner.FactorRef(), installer{values: schema, rule: uint32(ordinal)})
}

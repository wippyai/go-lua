// Package exactbinary owns Value's shared same-axis exact-binary execution
// family. Arithmetic, equality, and order all have the same sealed program
// geometry; their generated owner dispatch is the only semantic distinction.
//
// The installer is deliberately cold: it authenticates the reducer and its
// candidate through Value's generated directory and seals the concrete
// payload. The worker never keeps a candidate map, callback, or owner
// capability and performs only direct dispatch over that immutable payload.
package exactbinary

import (
	"github.com/wippyai/go-lua/analysis/engine"
	engineexecution "github.com/wippyai/go-lua/analysis/engine/execution"
	exactproduct "github.com/wippyai/go-lua/analysis/engine/execution/product"
	"github.com/wippyai/go-lua/analysis/engine/generated"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// exactBinaryRow is the immutable invocation geometry for one candidate. The
// payload contains the owner-issued reducer/candidate identity and concrete
// fold value; typed read and write descriptors are sealed from the same Value
// Factor plane.
type exactBinaryRow struct {
	payload valuedomain.ExactBinaryPayload
	left    engineexecution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
	right   engineexecution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
	write   engineexecution.ExactWrite[valuedomain.DenseCoordinate, valuedomain.Value]
}

// family is one shared implementation for every Value same-axis exact-binary
// reducer. No arithmetic/equality/order-specific family is needed: the
// reducer ordinal selects the generated owner dispatch.
type family struct {
	rows   []exactBinaryRow
	values *valuedomain.Schema
	rule   uint32
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
	family       *family
	run          *engineexecution.Run
	left         engineexecution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	right        engineexecution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	write        engineexecution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	leftProduct  exactproduct.Extender[valuedomain.DenseCoordinate, valuedomain.Value, struct{}]
	rightProduct exactproduct.Extender[valuedomain.DenseCoordinate, valuedomain.Value, exactproduct.Cons[exactproduct.Cell[valuedomain.Value], struct{}]]
}

// Execute performs the exact-binary fold. The canonical product cursor emits
// every common-refinement cell; the worker consumes all cells before closing
// its one output transaction. Sparse absence is the explicit NoCandidate
// case. Any structural failure leaves execution to the engine's fail-stop
// path; in particular a false owner dispatch is never converted into a
// submitted Refuse result.
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
	rule, ruleOK := ticket.RuleOrdinal()
	if !ruleOK || rule != lane.family.rule {
		return engineexecution.Result{}, false
	}
	payloadReducer, payloadReducerOK := row.payload.ReducerOrdinal()
	payloadCandidate, payloadCandidateOK := row.payload.CandidateOrdinal()
	candidate, candidateOK := ticket.CandidateOrdinal()
	if !payloadReducerOK || !valuedomain.SupportsExactBinaryReducer(payloadReducer) ||
		!payloadCandidateOK || !candidateOK || candidate != payloadCandidate {
		return engineexecution.Result{}, false
	}

	if !row.left.Valid() || !row.right.Valid() || !row.write.Valid() {
		return engineexecution.Result{}, false
	}
	seed, seedStatus, seedOK := exactproduct.NewSeed(ticket)
	if !seedOK || seedStatus == exactproduct.RefineRefuse {
		return engineexecution.Result{}, false
	}
	if seedStatus == exactproduct.RefineEmpty {
		return lane.settle(ticket, structure.NoCandidate, 0)
	}
	leftRows, leftStatus, leftOK := lane.leftProduct.Extend(ticket, seed.Rows(), row.left, &lane.left)
	if !leftOK || leftStatus != exactproduct.RefineAvailable {
		return engineexecution.Result{}, false
	}
	rows, rightStatus, rightOK := lane.rightProduct.Extend(ticket, leftRows, row.right, &lane.right)
	if !rightOK || rightStatus != exactproduct.RefineAvailable {
		return engineexecution.Result{}, false
	}
	concrete := 0
	for index := 0; index < rows.Count(); index++ {
		if !ticket.Checkpoint() {
			_ = lane.write.Discard(ticket)
			return engineexecution.Result{}, false
		}
		region, rightTuple, rowOK := rows.At(index)
		if !rowOK {
			_ = lane.write.Discard(ticket)
			return engineexecution.Result{}, false
		}
		rightCell := rightTuple.Head()
		leftTuple := rightTuple.Tail()
		leftCell := leftTuple.Head()
		left, leftPresent := leftCell.Value(), leftCell.Present()
		right, rightPresent := rightCell.Value(), rightCell.Present()
		if !region.Valid() {
			_ = lane.write.Discard(ticket)
			return engineexecution.Result{}, false
		}
		if !leftPresent || !rightPresent {
			// Sparse absence is an explicit empty region, not a default value.
			continue
		}
		if !lane.family.values.Equal(left, left) || !lane.family.values.Equal(right, right) {
			_ = lane.write.Discard(ticket)
			return engineexecution.Result{}, false
		}
		result, outcome, dispatched := lane.family.values.ReduceExactBinaryPayload(row.payload, left, right)
		if !dispatched || !outcome.Available() {
			_ = lane.write.Discard(ticket)
			return engineexecution.Result{}, false
		}
		switch outcome {
		case structure.Concrete:
			if !lane.family.values.Equal(result, result) || !row.write.Stage(ticket, &lane.write, region, result) {
				_ = lane.write.Discard(ticket)
				return engineexecution.Result{}, false
			}
			concrete++
		case structure.NoCandidate:
			// The explicit empty successor contributes no patch for this cell.
		default:
			_ = lane.write.Discard(ticket)
			return engineexecution.Result{}, false
		}
	}
	if concrete == 0 {
		return lane.settle(ticket, structure.NoCandidate, 0)
	}
	if !row.write.Close(ticket, &lane.write) {
		_ = lane.write.Discard(ticket)
		return engineexecution.Result{}, false
	}
	// One accepted write patch is one concrete result, regardless of how many
	// product cells were staged into that patch.
	return lane.settle(ticket, structure.Concrete, 1)
}

func (lane *worker) settle(ticket engineexecution.Ticket, outcome structure.ReductionOutcome, count int) (engineexecution.Result, bool) {
	if lane == nil || !outcome.Available() || !ticket.Submit(outcome) {
		return engineexecution.Result{}, false
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

	sealed := &family{values: install.values, rule: ruleOrdinal, rows: make([]exactBinaryRow, 0, len(rows))}
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
			!valuedomain.SupportsExactBinaryReducer(reducer.Member) {
			return nil, nil, false
		}
		mapping, mappingOK := install.values.ExactBinaryMappingAt(reducer.Member)
		if !mappingOK || mapping.ReducerOrdinal != reducer.Member ||
			!exactBinaryMappingMatches(planRow.Rule, mapping) {
			return nil, nil, false
		}
		payload, payloadOK := install.values.ExactBinaryPayloadAt(reducer.Member, candidate.Member, planRow.Candidate)
		payloadReducer, payloadReducerOK := payload.ReducerOrdinal()
		payloadCandidate, payloadCandidateOK := payload.CandidateOrdinal()
		payloadRelation, payloadRelationOK := payload.CandidateRelationMember()
		if !payloadOK || !payloadReducerOK || !payloadCandidateOK || !payloadRelationOK ||
			payloadReducer != reducer.Member || payloadRelation != candidate.Member || payloadCandidate != planRow.Candidate {
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
			payload: payload,
			left:    left, right: right, write: write,
		})
	}
	return sealed, addresses, true
}

// exactBinaryMappingMatches authenticates every owner-local member coordinate
// in one generated exact-binary descriptor. The generated Value owner issues
// the mapping; this helper only compares that sealed payload to the descriptor
// and retains no family-owned relation table.
func exactBinaryMappingMatches(rule generated.CompiledRule, mapping valuedomain.ExactBinaryMapping) bool {
	if !rule.Available() || mapping.ReducerOrdinal != rule.Reducer().Member ||
		mapping.CandidateRelationMember != rule.CandidateRelation().Member {
		return false
	}
	first, firstOK := rule.ReadAt(0)
	second, secondOK := rule.ReadAt(1)
	output, outputOK := rule.OutputAt(0)
	return firstOK && secondOK && outputOK &&
		first.Relation.Member == mapping.Read0RelationMember && first.Key.Member == mapping.Read0KeyMember &&
		second.Relation.Member == mapping.Read1RelationMember && second.Key.Member == mapping.Read1KeyMember &&
		output.Destination.Member == mapping.DestinationProjectionMember
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

// Package exactfold owns Value's one generated same-axis exact fold family.
// Arithmetic, equality, order and presence refinement all have the same
// sealed program geometry - a candidate payload, one to ExactFoldArity exact
// multiplicity-one reads of the Value factor, and one exact own-axis write -
// and their generated owner dispatch is the only semantic distinction.
//
// The installer is deliberately cold: it authenticates the reducer, its
// candidate and the complete read/write geometry through Value's generated
// directory and seals the concrete payload. The worker never keeps a
// candidate table, callback, or owner capability and performs only direct
// dispatch over that immutable payload.
package exactfold

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

type (
	exactRead  = engineexecution.ExactRead[valuedomain.DenseCoordinate, valuedomain.Value]
	exactWrite = engineexecution.ExactWrite[valuedomain.DenseCoordinate, valuedomain.Value]
	scratch    = engineexecution.Scratch[valuedomain.DenseCoordinate, valuedomain.Value]
	factCell   = exactproduct.Cell[valuedomain.Value]
	// tupleN is the typed product tuple after N reads have been consed on. The
	// chain is the product package's own vocabulary; no arity-specific product
	// or local mask algebra is introduced here.
	tuple1 = exactproduct.Cons[factCell, struct{}]
	tuple2 = exactproduct.Cons[factCell, tuple1]
	tuple3 = exactproduct.Cons[factCell, tuple2]
	// reads is the dense read vector the generated owner dispatch consumes.
	reads = [valuedomain.ExactFoldArity]valuedomain.Value
)

// exactFoldRow is the immutable invocation geometry for one candidate. The
// payload contains the owner-issued reducer/candidate identity and concrete
// fold value; typed read and write descriptors are sealed from the same Value
// Factor plane.
type exactFoldRow struct {
	payload valuedomain.ExactFoldPayload
	read    [valuedomain.ExactFoldArity]exactRead
	count   int
	write   exactWrite
}

// family is one shared implementation for every Value same-axis exact fold
// reducer. No per-rule family is needed: the reducer ordinal selects the
// generated owner dispatch and the sealed read count selects the product
// depth.
type family struct {
	rows   []exactFoldRow
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

// worker is an epoch-local invocation lane. Its scratch values and product
// extenders are allocated once by NewExecutor and reused by every warm
// invocation.
type worker struct {
	family   *family
	run      *engineexecution.Run
	read     [valuedomain.ExactFoldArity]scratch
	write    scratch
	product1 exactproduct.Extender[valuedomain.DenseCoordinate, valuedomain.Value, struct{}]
	product2 exactproduct.Extender[valuedomain.DenseCoordinate, valuedomain.Value, tuple1]
	product3 exactproduct.Extender[valuedomain.DenseCoordinate, valuedomain.Value, tuple2]
	vector   reads
}

// Execute performs the exact fold. The canonical product cursor emits every
// common-refinement cell; the worker consumes all cells before closing its one
// output transaction. Sparse absence is the explicit NoCandidate case. Any
// structural failure leaves execution to the engine's fail-stop path; in
// particular a false owner dispatch is never converted into a submitted Refuse
// result.
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
	payloadReducer, payloadReducerOK := row.payload.ReducerOrdinal()
	payloadCandidate, payloadCandidateOK := row.payload.CandidateOrdinal()
	payloadReads, payloadReadsOK := row.payload.ReadCount()
	candidate, candidateOK := ticket.CandidateOrdinal()
	if !payloadReducerOK || !valuedomain.SupportsExactFoldReducer(payloadReducer) ||
		!payloadCandidateOK || !candidateOK || candidate != payloadCandidate ||
		!payloadReadsOK || payloadReads != row.count {
		return engineexecution.Result{}, false
	}
	if row.count < 1 || row.count > valuedomain.ExactFoldArity || !row.write.Valid() {
		return engineexecution.Result{}, false
	}
	for join := 0; join < row.count; join++ {
		if !row.read[join].Valid() {
			return engineexecution.Result{}, false
		}
	}
	seed, seedStatus, seedOK := exactproduct.NewSeed(ticket)
	if !seedOK || seedStatus == exactproduct.RefineRefuse {
		return engineexecution.Result{}, false
	}
	if seedStatus == exactproduct.RefineEmpty {
		return lane.settle(ticket, structure.NoCandidate, 0)
	}
	rows1, status, ok := lane.product1.Extend(ticket, seed.Rows(), row.read[0], &lane.read[0])
	if !ok || status != exactproduct.RefineAvailable {
		return engineexecution.Result{}, false
	}
	concrete, folded := 0, false
	switch row.count {
	case 1:
		concrete, folded = foldRows(lane, ticket, row, rows1, spread1)
	case 2:
		rows2, status, ok := lane.product2.Extend(ticket, rows1, row.read[1], &lane.read[1])
		if !ok || status != exactproduct.RefineAvailable {
			return engineexecution.Result{}, false
		}
		concrete, folded = foldRows(lane, ticket, row, rows2, spread2)
	case 3:
		rows2, status, ok := lane.product2.Extend(ticket, rows1, row.read[1], &lane.read[1])
		if !ok || status != exactproduct.RefineAvailable {
			return engineexecution.Result{}, false
		}
		rows3, status3, ok3 := lane.product3.Extend(ticket, rows2, row.read[2], &lane.read[2])
		if !ok3 || status3 != exactproduct.RefineAvailable {
			return engineexecution.Result{}, false
		}
		concrete, folded = foldRows(lane, ticket, row, rows3, spread3)
	}
	if !folded {
		return engineexecution.Result{}, false
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

// foldRows drains one completed product to exhaustion, dispatching the owner
// fold over every cell whose reads are all present and staging its concrete
// results into the single open write transaction. The spread argument is the
// static tuple flattener for this product depth; the worker retains neither it
// nor any state derived from it.
func foldRows[T any](
	lane *worker,
	ticket engineexecution.Ticket,
	row exactFoldRow,
	rows exactproduct.Rows[T],
	spread func(T, *reads) bool,
) (int, bool) {
	if lane == nil || spread == nil || !rows.Valid() {
		return 0, false
	}
	concrete := 0
	for index := 0; index < rows.Count(); index++ {
		if !ticket.Checkpoint() {
			_ = lane.write.Discard(ticket)
			return 0, false
		}
		region, tuple, rowOK := rows.At(index)
		if !rowOK || !region.Valid() {
			_ = lane.write.Discard(ticket)
			return 0, false
		}
		lane.vector = reads{}
		if !spread(tuple, &lane.vector) {
			// Sparse absence is an explicit empty region, not a default value.
			continue
		}
		for position := 0; position < row.count; position++ {
			if !lane.family.values.Equal(lane.vector[position], lane.vector[position]) {
				_ = lane.write.Discard(ticket)
				return 0, false
			}
		}
		result, outcome, dispatched := lane.family.values.ReduceExactFoldPayload(row.payload, lane.vector)
		if !dispatched || !outcome.Available() {
			_ = lane.write.Discard(ticket)
			return 0, false
		}
		switch outcome {
		case structure.Concrete:
			if !lane.family.values.Equal(result, result) || !row.write.Stage(ticket, &lane.write, region, result) {
				_ = lane.write.Discard(ticket)
				return 0, false
			}
			concrete++
		case structure.NoCandidate:
			// The explicit empty successor contributes no patch for this cell.
		default:
			_ = lane.write.Discard(ticket)
			return 0, false
		}
	}
	return concrete, true
}

// spread1, spread2 and spread3 place a typed product tuple into the dense read
// vector in declared join order. The extender conses the newest read at the
// head, so the tail is walked back to join zero. The boolean answers whether
// every cell of this tuple carries a stored value.
func spread1(tuple tuple1, vector *reads) bool {
	cell := tuple.Head()
	vector[0] = cell.Value()
	return cell.Present()
}

func spread2(tuple tuple2, vector *reads) bool {
	second := tuple.Head()
	vector[1] = second.Value()
	return spread1(tuple.Tail(), vector) && second.Present()
}

func spread3(tuple tuple3, vector *reads) bool {
	third := tuple.Head()
	vector[2] = third.Value()
	return spread2(tuple.Tail(), vector) && third.Present()
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
// It holds no rule ordinal: which rule this installer authors is the claim it
// was installed under, and the family table resolves it only for that claim.
type installer struct {
	values *valuedomain.Schema
}

func (install installer) InstallRuleFamily(
	plane engineexecution.FormPlane[valuedomain.DenseCoordinate, valuedomain.Value],
	_ uint32,
	rows []engineexecution.FormRow,
) (engineexecution.Family, []engineexecution.FormAddress, bool) {
	if install.values == nil || !install.values.Valid() || !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}

	sealed := &family{values: install.values, rows: make([]exactFoldRow, 0, len(rows))}
	addresses := make([]engineexecution.FormAddress, 0, len(rows))
	for _, planRow := range rows {
		count, shapeOK := exactFoldShape(planRow.Rule, planRow.Form)
		if planRow.Member < 0 || !shapeOK {
			return nil, nil, false
		}
		output, outputOK := planRow.Rule.OutputAt(0)
		if !outputOK {
			return nil, nil, false
		}

		reducer := planRow.Rule.Reducer()
		candidate := planRow.Rule.CandidateRelation()
		if reducer.Axis != output.Axis || candidate.Axis != output.Axis ||
			!valuedomain.SupportsExactFoldReducer(reducer.Member) {
			return nil, nil, false
		}
		mapping, mappingOK := install.values.ExactFoldMappingAt(reducer.Member)
		if !mappingOK || mapping.ReducerOrdinal != reducer.Member ||
			!exactFoldMappingMatches(planRow.Rule, mapping) {
			return nil, nil, false
		}
		payload, payloadOK := install.values.ExactFoldPayloadAt(reducer.Member, candidate.Member, planRow.Candidate)
		payloadReducer, payloadReducerOK := payload.ReducerOrdinal()
		payloadCandidate, payloadCandidateOK := payload.CandidateOrdinal()
		payloadRelation, payloadRelationOK := payload.CandidateRelationMember()
		payloadReads, payloadReadsOK := payload.ReadCount()
		if !payloadOK || !payloadReducerOK || !payloadCandidateOK || !payloadRelationOK || !payloadReadsOK ||
			payloadReducer != reducer.Member || payloadRelation != candidate.Member ||
			payloadCandidate != planRow.Candidate || payloadReads != count {
			return nil, nil, false
		}

		sealedRow := exactFoldRow{payload: payload, count: count}
		foreign, foreignOK := plane.Foreign(output.Factor)
		write, writeOK := plane.ExactWrite(planRow.Target, uint16(output.Slot))
		if !foreignOK || !writeOK {
			return nil, nil, false
		}
		sealedRow.write = write
		for join := 0; join < count; join++ {
			// Every declared read of this shape observes the Factor the rule
			// writes, so all reads are sealed through that one foreign handle.
			read, readOK := engineexecution.ForeignRowExactRead[valuedomain.DenseCoordinate, valuedomain.Value](foreign, planRow, join)
			if !readOK {
				return nil, nil, false
			}
			sealedRow.read[join] = read
		}
		addresses = append(addresses, engineexecution.FormAddress{Member: planRow.Member, Local: uint32(len(sealed.rows))})
		sealed.rows = append(sealed.rows, sealedRow)
	}
	return sealed, addresses, true
}

// exactFoldMappingMatches authenticates every owner-local member coordinate in
// one generated exact fold descriptor. The generated Value owner issues the
// mapping; this helper only compares that sealed payload to the descriptor and
// retains no family-owned relation table.
func exactFoldMappingMatches(rule generated.CompiledRule, mapping valuedomain.ExactFoldMapping) bool {
	if !rule.Available() || mapping.ReducerOrdinal != rule.Reducer().Member ||
		mapping.CandidateRelationMember != rule.CandidateRelation().Member ||
		mapping.ReadCount < 1 || mapping.ReadCount > valuedomain.ExactFoldArity ||
		int(mapping.ReadCount) != rule.ReadCount() {
		return false
	}
	output, outputOK := rule.OutputAt(0)
	if !outputOK || output.Destination.Member != mapping.DestinationProjectionMember {
		return false
	}
	for join := 0; join < int(mapping.ReadCount); join++ {
		read, readOK := rule.ReadAt(join)
		if !readOK || read.Relation.Member != mapping.ReadRelationMember[join] ||
			read.Key.Member != mapping.ReadKeyMember[join] {
			return false
		}
	}
	return true
}

// exactFoldShape answers the declared read count of one sealed descriptor that
// belongs to this family, and false for every descriptor that does not. Each
// read must be an own-axis exact multiplicity-one read of the Factor the rule
// writes: that is the one read class this family seals. A read of a foreign
// axis is a different read class and is refused here rather than sealed
// against this plane.
func exactFoldShape(rule generated.CompiledRule, form engineexecution.Form) (int, bool) {
	if !rule.Available() || form != engineexecution.FormExact || rule.InputCount() != 1 ||
		rule.OutputCount() != 1 || !rule.CarryIdentity() || rule.CarryInput() != 0 {
		return 0, false
	}
	count := rule.ReadCount()
	if count < 1 || count > valuedomain.ExactFoldArity {
		return 0, false
	}
	output, outputOK := rule.OutputAt(0)
	if !outputOK || output.Mode != ruleprogram.ModeExact || output.Slot != 0 || !output.Exact || !output.Strong {
		return 0, false
	}
	if rule.Reducer().Axis != output.Axis || rule.CandidateRelation().Axis != output.Axis {
		return 0, false
	}
	for join := 0; join < count; join++ {
		read, readOK := rule.ReadAt(join)
		if !readOK || read.Form != ruleprogram.Exact || read.Input != 0 ||
			read.Factor != output.Factor || read.Axis != output.Axis ||
			read.Relation.Axis != output.Axis || read.Key.Axis != output.Axis ||
			!exactContract(read) {
			return 0, false
		}
	}
	return count, true
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

// InstallFamily claims one generated Value exact fold rule. Arithmetic,
// equality, order and presence refinement call this same installer; the owner
// dispatch's reducer ordinal is the only family-specific data retained by the
// sealed rows.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	owner := authorities.ValueAuthority()
	schema := authorities.ValueSchema()
	if owner == nil || schema == nil || !schema.Valid() || owner.Schema() != schema {
		return false
	}
	return engine.BindRuleFamily[valuedomain.DenseCoordinate](binding, slot, owner.FactorRef(), installer{values: schema})
}

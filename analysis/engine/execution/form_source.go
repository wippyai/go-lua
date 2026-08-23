// form_source.go owns the Z form: a read-free write of the owner's
// materialized source column, indexed by the issued candidate ordinal.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// classifySourceForm claims a descriptor with no join and no input whose
// candidate relation is published by the output Factor itself: the relation
// member is the form's only coordinate.
func classifySourceForm(rule generated.CompiledRule) (FormRow, bool) {
	mode, modeOK := rule.OutputMode()
	if !modeOK || mode != ruleprogram.ModeExact || rule.ReadCount() != 0 || rule.InputCount() != 0 {
		return FormRow{}, false
	}
	candidate := rule.CandidateRelation()
	if candidate.Axis != rule.OutputFactor() {
		return FormRow{}, false
	}
	return FormRow{Form: FormSource, Relation: candidate.Member}, true
}

// buildSourceForm seals one typed Z family from this Factor's source rows. Rows
// that name the same relation column and the same write target share one
// sealed descriptor: the candidate ordinal, not the row, selects the value.
func buildSourceForm[K scalar.Key, V any](plane FormPlane[K, V], rows []FormRow) (Family, []FormAddress, bool) {
	if !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}
	type descriptorKey struct {
		relation uint32
		target   carrier.Target
	}
	sealed := make([]SourceRow[K, V], 0, len(rows))
	addresses := make([]FormAddress, 0, len(rows))
	locals := make(map[descriptorKey]uint32, len(rows))
	for _, row := range rows {
		column, columnOK := plane.column(row.Relation)
		if !columnOK {
			return nil, nil, false
		}
		key := descriptorKey{relation: row.Relation, target: row.Target}
		local, present := locals[key]
		if !present {
			source, ok := NewSourceRow(plane.binding, row.Target, 0, column)
			if !ok {
				return nil, nil, false
			}
			local = uint32(len(sealed))
			locals[key] = local
			sealed = append(sealed, source)
		}
		addresses = append(addresses, FormAddress{Member: row.Member, Local: local})
	}
	family, ok := NewSourceFamily(sealed)
	if !ok {
		return nil, nil, false
	}
	return family, addresses, true
}

// SourceRow is static typed row data for the Z form. The owner materialized
// column is copied at seal; the runtime indexes it only by the ticket's issued
// candidate ordinal.
type SourceRow[K scalar.Key, V any] struct {
	write  ExactWrite[K, V]
	column memberrelation.SourceColumn[V]
}

func NewSourceRow[K scalar.Key, V any](binding *factbinding.Binding[K, V], target carrier.Target, output uint16, column memberrelation.SourceColumn[V]) (SourceRow[K, V], bool) {
	write, writeOK := NewExactWrite(binding, target, output)
	if !writeOK || !column.Valid() {
		return SourceRow[K, V]{}, false
	}
	// The column has already been copied from the cold owner into the sealed
	// bound Factor. A family borrows that immutable Program value; cloning here
	// once per candidate would turn source lowering into quadratic retention.
	return SourceRow[K, V]{write: write, column: column}, true
}

type sourceFamily[K scalar.Key, V any] struct{ rows []SourceRow[K, V] }
type sourceWorker[K scalar.Key, V any] struct {
	family  *sourceFamily[K, V]
	run     *Run
	scratch Scratch[K, V]
}

func NewSourceFamily[K scalar.Key, V any](rows []SourceRow[K, V]) (Family, bool) {
	if len(rows) == 0 {
		return nil, false
	}
	sealed := append([]SourceRow[K, V](nil), rows...)
	for index := range sealed {
		if !sealed[index].write.Valid() || !sealed[index].column.Valid() {
			return nil, false
		}
	}
	return &sourceFamily[K, V]{rows: sealed}, true
}

func (family *sourceFamily[K, V]) NewExecutor(run *Run) Executor {
	if family == nil || run == nil {
		return nil
	}
	return &sourceWorker[K, V]{family: family, run: run}
}
func (*sourceFamily[K, V]) InputCapacity() int  { return 0 }
func (*sourceFamily[K, V]) OutputCapacity() int { return 1 }

func (worker *sourceWorker[K, V]) Execute(frame Frame, ticket Ticket) (Result, bool) {
	if worker == nil || worker.family == nil || worker.run == nil || !frame.Valid(ticket) || ticket.issuer != worker.run || ticket.InputCount() != 0 {
		return Result{}, false
	}
	_, local, localOK := ticket.familyLocal()
	candidate, candidateOK := ticket.CandidateOrdinal()
	if !localOK || !candidateOK || uint64(local) >= uint64(len(worker.family.rows)) {
		return Result{}, false
	}
	row := worker.family.rows[local]
	value, outcome, valueOK := row.column.At(candidate)
	if !valueOK || !outcome.Available() || outcome == structure.Refuse {
		return Result{}, false
	}
	// A sealed non-Concrete row stages nothing. The candidate stays in the
	// directory - it is a real occurrence - and the fold concludes the
	// disposition its materializer sealed.
	if outcome != structure.Concrete {
		if !ticket.Submit(outcome) {
			return Result{}, false
		}
		return NewResult(outcome, 0)
	}
	_, _, within, contextOK := row.write.context(ticket)
	if !contextOK || !row.write.Stage(ticket, &worker.scratch, within, value) || !row.write.Close(ticket, &worker.scratch) || !ticket.Submit(structure.Concrete) {
		return Result{}, false
	}
	return NewResult(structure.Concrete, 1)
}

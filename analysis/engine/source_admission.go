package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// sourceAdmissionCore is the one engine-private bridge to the equation Batch
// source-row admission operations. SourceAssembly uses its scalar methods and
// boundary materialization; staged activation uses all, which is the
// prospective atomic form. Keeping
// the bridge here prevents a second validator or a second batch authority from
// growing in either caller.
type sourceAdmissionCore struct {
	batch *equation.Batch
}

func newSourceAdmissionCore(batch *equation.Batch) sourceAdmissionCore {
	return sourceAdmissionCore{batch: batch}
}

func (core sourceAdmissionCore) admitSite(source composition.Key, scope equation.Scope, init equation.Expr, disposition equation.InitDisposition) (equation.Site, bool) {
	if core.batch == nil {
		return equation.Site{}, false
	}
	return core.batch.AdmitSite(source, scope, init, disposition)
}

func (core sourceAdmissionCore) admitOccurrence(site equation.Site, kind equation.OccurrenceKind, entity composition.Key) (equation.Occurrence, bool) {
	if core.batch == nil {
		return equation.Occurrence{}, false
	}
	switch kind {
	case equation.OccurrenceAt:
		return core.batch.At(site)
	case equation.OccurrenceRelation:
		return core.batch.Relation(site, entity)
	default:
		core.reject()
		return equation.Occurrence{}, false
	}
}

func (core sourceAdmissionCore) admitOperand(occurrence equation.Occurrence, entity composition.Key) (equation.Operand, bool) {
	if core.batch == nil {
		return equation.Operand{}, false
	}
	return core.batch.AdmitOperand(occurrence, entity)
}

func (core sourceAdmissionCore) admitBoundary(source, target equation.Site, provenance composition.Key, pre equation.Expr, reindex equation.Reindex, post equation.Expr) equation.Input {
	return equation.BoundaryInput(source, target, provenance, pre, reindex, post)
}

// admitSourceRows is the complete-set admission operation. equation.Batch
// validates the prospective set before publishing any row, preserving
// activation's all-or-nothing semantics.
func (core sourceAdmissionCore) admitSourceRows(rows []equation.Admission) ([]equation.AdmissionResult, bool) {
	if core.batch == nil {
		return nil, false
	}
	return core.batch.AdmitAll(rows)
}

func (core sourceAdmissionCore) reject() {
	if core.batch != nil {
		core.batch.Reject()
	}
}

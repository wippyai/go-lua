package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ModuleLoadTransaction is the sole immutable execution bridge for one
// prepared module-load producer. It retains the exact argument ValueSource and
// both full-width ownership identities; the evaluated argument product remains
// an executor input and is never replaced by a preparation-time literal.
type ModuleLoadTransaction struct {
	point       cfg.Point
	operation   operationplan.ModuleLoadOperation
	operationID operationplan.ModuleLoadContentID
	tableID     operationplan.ModuleLoadExportTableContentID
	argument    factflow.ValueSource
}

// PlanModuleLoadTransaction freezes the operation at point without copying its
// body-shared export table. Absence is explicit and cannot mint authority.
func PlanModuleLoadTransaction(reg *axis.Registry, plan *operationplan.Plan, point cfg.Point) (ModuleLoadTransaction, bool) {
	if reg == nil || plan == nil {
		return ModuleLoadTransaction{}, false
	}
	operation, ok := plan.ModuleLoadOperation(point)
	if !ok || !operation.ValidFor(reg) {
		return ModuleLoadTransaction{}, false
	}
	transaction := ModuleLoadTransaction{
		point: point, operation: operation, operationID: operation.ContentID(),
		tableID: operation.ExportTable().ContentID(), argument: operation.Argument(),
	}
	return transaction, transaction.Valid(reg)
}

func (t ModuleLoadTransaction) Point() cfg.Point { return t.point }

func (t ModuleLoadTransaction) Argument() factflow.ValueSource { return t.argument }

func (t ModuleLoadTransaction) OperationID() operationplan.ModuleLoadContentID { return t.operationID }

func (t ModuleLoadTransaction) TableID() operationplan.ModuleLoadExportTableContentID {
	return t.tableID
}

func (t ModuleLoadTransaction) Valid(reg *axis.Registry) bool {
	return reg != nil && t.argument.Valid() && t.operation.ValidFor(reg) &&
		t.operationID.Available() && t.operationID == t.operation.ContentID() &&
		t.tableID.Available() && t.tableID == t.operation.ExportTable().ContentID() &&
		factflow.ValueSourceEqual(t.argument, t.operation.Argument())
}

// ResolvedModuleLoadTransaction is the exact N0 result of evaluating one
// ModuleLoadTransaction's retained argument in its point-local world. It owns
// no resolver callback and cannot be detached from the operation/table version
// that selected the export.
type ResolvedModuleLoadTransaction struct {
	point               cfg.Point
	operationID         operationplan.ModuleLoadContentID
	tableID             operationplan.ModuleLoadExportTableContentID
	result              CallResultTransaction
	postReturnAuthority bool
}

// Resolve selects the exact export for evaluatedArgument and freezes its slot
// zero value as the canonical N0 CallResultTransaction. Nonliteral, missing,
// foreign-registry, or stale-operation inputs fail closed.
func (t ModuleLoadTransaction) Resolve(reg *axis.Registry, evaluatedArgument product.Value) (ResolvedModuleLoadTransaction, bool) {
	if !t.Valid(reg) || !product.BelongsToRegistry(reg, evaluatedArgument) {
		return ResolvedModuleLoadTransaction{}, false
	}
	resolution, ok := t.operation.ResolveArgument(reg, evaluatedArgument)
	if !ok || !resolution.Matches(t.operation) || resolution.ResultIndex() != operationplan.ModuleLoadResultIndex {
		return ResolvedModuleLoadTransaction{}, false
	}
	result, ok := NewResolvedCallResultTransaction(reg, t.point, resolution.ResultIndex(), resolution.Value())
	if !ok {
		return ResolvedModuleLoadTransaction{}, false
	}
	resolved := ResolvedModuleLoadTransaction{
		point: t.point, operationID: t.operationID, tableID: t.tableID,
		result: result, postReturnAuthority: resolution.PostReturnAuthority(),
	}
	return resolved, resolved.Valid(reg) && resolved.Matches(t)
}

func (r ResolvedModuleLoadTransaction) Valid(reg *axis.Registry) bool {
	if reg == nil || !r.operationID.Available() || !r.tableID.Available() ||
		r.result.Point() != r.point || r.result.Len() != 1 || !r.result.Valid(reg) || !r.result.HasMaterializeSteps() ||
		r.result.HasPostconditionSteps() || r.result.HasPublicationSteps() {
		return false
	}
	step, ok := r.result.Step(0)
	value, isValue := step.ResultValue()
	return ok && isValue && value.Index() == operationplan.ModuleLoadResultIndex &&
		product.BelongsToRegistry(reg, value.Value())
}

func (r ResolvedModuleLoadTransaction) Matches(transaction ModuleLoadTransaction) bool {
	return r.operationID == transaction.operationID && r.tableID == transaction.tableID && r.point == transaction.point
}

func (r ResolvedModuleLoadTransaction) PostReturnAuthority() bool { return r.postReturnAuthority }

// ResultTransaction returns the detached exact N0 payload for boundary syntax.
func (r ResolvedModuleLoadTransaction) ResultTransaction() CallResultTransaction {
	return r.result.Clone()
}

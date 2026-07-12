package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// GenericForSourceKind identifies the syntax-free producer of a generic-for
// iterator. The source expression itself is deliberately not retained.
type GenericForSourceKind = operationplan.GenericForSourceKind

const (
	GenericForSourceUnknown    = operationplan.GenericForSourceUnknown
	GenericForSourceExpression = operationplan.GenericForSourceExpression
	GenericForSourceCall       = operationplan.GenericForSourceCall
)

// GenericForSource is the immutable engine projection of the first iterator
// source. RootPath is populated only for a directly nameable expression;
// CallPoint is populated only for a lowered call source.
type GenericForSource = operationplan.GenericForSource

// GenericForOperation is the complete syntax-free payload for one loop
// variable binding. SourceContracts are indexed by iterator-call parameter.
// NewGenericForOperation copies all caller-owned slices and paths.
type GenericForOperation = operationplan.GenericForOperation

func NewGenericForOperation(variableIndex int, target, firstTarget symbol.ID, source GenericForSource, sourceContracts []typ.Type) (GenericForOperation, bool) {
	return operationplan.NewGenericForOperation(variableIndex, target, firstTarget, source, sourceContracts)
}

// ConcreteGenericForRequest preserves the established generic-for transaction:
// membership for the target is cleared first, value resolution observes Input,
// and membership/heap descendants are applied to the evolving Output.
type ConcreteGenericForRequest struct {
	Context   transfer.NodeContext
	Resolver  *visibility.Resolver
	Input     state.State
	Output    state.State
	Operation GenericForOperation
	Semantics ConcreteGenericForSemantics
}

// ConcreteGenericForSemantics is the owner-supplied iterator/container
// interpretation. A prepared body owns one implementation, avoiding callback
// closure allocation on every fixed-point transfer.
type ConcreteGenericForSemantics interface {
	ResolveGenericFor(transfer.NodeContext, GenericForOperation, state.State) (product.Value, bool)
	ApplyGenericForMembership(transfer.NodeContext, GenericForOperation, state.State, pathdom.Path) state.State
}

type ConcreteGenericForResult struct {
	Output   state.State
	Canceled bool
}

// GenericForTransaction is the shared syntax-free binding transaction shape.
// Concrete and symbolic executors consume this same plan; iterator resolution
// and membership remain owner-supplied algebras.
type GenericForTransaction struct {
	Target          symbol.ID
	VariableIndex   int
	ClearMembership bool
	ResolveValue    bool
	ApplyMembership bool
}

func PlanGenericForTransaction(op GenericForOperation) (GenericForTransaction, bool) {
	if op.Target() == 0 || op.VariableIndex() < 0 {
		return GenericForTransaction{}, false
	}
	return GenericForTransaction{
		Target: op.Target(), VariableIndex: op.VariableIndex(),
		ClearMembership: true, ResolveValue: true, ApplyMembership: true,
	}, true
}

// ApplyConcreteGenericFor executes one loop-variable binding atomically.
// Cancellation before or after a provider callback publishes no prefix.
func ApplyConcreteGenericFor(req ConcreteGenericForRequest) ConcreteGenericForResult {
	token := req.Context.Session.Token()
	if token != nil && token.Canceled() {
		return ConcreteGenericForResult{Output: req.Input, Canceled: true}
	}
	op := req.Operation
	transaction, valid := PlanGenericForTransaction(op)
	if !valid {
		return ConcreteGenericForResult{Output: req.Output}
	}
	out := req.Output
	targetPath := pathdom.Path{Symbol: transaction.Target}
	if transaction.ClearMembership && req.Resolver != nil {
		if targetKey, ok := visibility.AddressAt(req.Resolver, req.Context.Point, targetPath).VisibleStateKey(); ok {
			out = out.ClearKeyMembershipsForPath(targetKey)
		}
	}
	if transaction.ResolveValue && req.Semantics != nil {
		if value, ok := req.Semantics.ResolveGenericFor(req.Context, op, req.Input); ok {
			out = out.WriteValue(req.Context.Registry, key.SymbolValue(op.Target()), value)
		}
	}
	if token != nil && token.Canceled() {
		return ConcreteGenericForResult{Output: req.Input, Canceled: true}
	}
	if transaction.ApplyMembership && req.Semantics != nil {
		out = req.Semantics.ApplyGenericForMembership(req.Context, op, out, targetPath)
	}
	if token != nil && token.Canceled() {
		return ConcreteGenericForResult{Output: req.Input, Canceled: true}
	}
	return ConcreteGenericForResult{Output: out}
}

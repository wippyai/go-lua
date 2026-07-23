package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
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

func NewGenericForOperation(variableIndex int, target, firstTarget symbol.ID, protocolSources []GenericForSource, sourceContracts []typ.Type) (GenericForOperation, bool) {
	return operationplan.NewGenericForOperation(variableIndex, target, firstTarget, protocolSources, sourceContracts)
}

// GenericForRequest is the canonical generic-for binding transaction.
// ResolvedValue is evaluated by the relation's frozen Projection term before
// entering this transaction; generic-for never re-enters call or source
// semantics. HasResolvedValue distinguishes an inexact projection from an
// exact bottom value.
type GenericForRequest struct {
	Context          transfer.NodeContext
	Input            state.State
	Output           state.State
	Operation        GenericForOperation
	ResolvedValue    product.Value
	HasResolvedValue bool
	Membership       GenericForMembershipAuthority
	Domain           state.ProductDomain
}

// GenericForMembershipAuthority freezes the one factor-native non-scalar
// transaction. Concrete State and formal tuples are only carrier adapters;
// neither owns generic-for semantics.
type GenericForMembershipAuthority interface {
	PrepareGenericForFactorTransaction(transfer.NodeContext, GenericForOperation, state.ProductDomain) (state.GenericForFactorTransaction, error)
}

type GenericForResult struct {
	Output   state.State
	Canceled bool
}

// GenericForTransaction is the syntax-free binding transaction shape consumed
// by the canonical guarded relation executor.
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

// ApplyGenericFor executes one loop-variable binding atomically. Projection is
// already resolved by the canonical term DAG; this function only performs the
// clear/write/membership transaction.
func ApplyGenericFor(req GenericForRequest) GenericForResult {
	token := req.Context.Session.Token()
	if token != nil && token.Canceled() {
		return GenericForResult{Output: req.Input, Canceled: true}
	}
	op := req.Operation
	transaction, valid := PlanGenericForTransaction(op)
	if !valid {
		return GenericForResult{Output: req.Output}
	}
	out := req.Output
	if req.Membership == nil || !req.Domain.Valid() {
		return GenericForResult{Output: req.Input, Canceled: true}
	}
	factorTransaction, err := req.Membership.PrepareGenericForFactorTransaction(req.Context, op, req.Domain)
	if err != nil || !factorTransaction.Valid() {
		return GenericForResult{Output: req.Input, Canceled: true}
	}
	sourceLanes, currentLanes := factorTransaction.SourceLanes(), factorTransaction.CurrentLanes()
	sourceFactors, err := req.Domain.DecomposeLanes(req.Input, sourceLanes)
	if err != nil {
		return GenericForResult{Output: req.Input, Canceled: true}
	}
	currentFactors, err := req.Domain.DecomposeLanes(req.Output, currentLanes)
	if err != nil {
		return GenericForResult{Output: req.Input, Canceled: true}
	}
	writes, err := factorTransaction.Apply(sourceFactors, currentFactors)
	if err != nil {
		return GenericForResult{Output: req.Input, Canceled: true}
	}
	if token != nil && token.Canceled() {
		return GenericForResult{Output: req.Input, Canceled: true}
	}
	delta, err := req.Domain.ComposeSparse(writes)
	if err != nil {
		return GenericForResult{Output: req.Input, Canceled: true}
	}
	writeLanes := factorTransaction.WriteLanes()
	writeIDs := make([]state.LaneID, len(writeLanes))
	for index, lane := range writeLanes {
		writeIDs[index] = lane.ID()
	}
	out, err = req.Domain.PatchFactors(out, delta, state.NewLaneSet(writeIDs...))
	if err != nil {
		return GenericForResult{Output: req.Input, Canceled: true}
	}
	if transaction.ResolveValue && req.HasResolvedValue {
		out = out.WriteValue(req.Context.Registry, key.SymbolValue(op.Target()), req.ResolvedValue)
	}
	if token != nil && token.Canceled() {
		return GenericForResult{Output: req.Input, Canceled: true}
	}
	return GenericForResult{Output: out}
}

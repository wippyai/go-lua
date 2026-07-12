package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// GenericForSourceKind identifies the syntax-free producer of a generic-for
// iterator. The source expression itself is deliberately not retained.
type GenericForSourceKind uint8

const (
	GenericForSourceUnknown GenericForSourceKind = iota
	GenericForSourceExpression
	GenericForSourceCall
)

// GenericForSource is the immutable engine projection of the first iterator
// source. RootPath is populated only for a directly nameable expression;
// CallPoint is populated only for a lowered call source.
type GenericForSource struct {
	Kind         GenericForSourceKind
	CallPoint    cfg.Point
	HasCallPoint bool
	RootPath     pathdom.Path
	HasRootPath  bool
}

// GenericForOperation is the complete syntax-free payload for one loop
// variable binding. SourceContracts are indexed by iterator-call parameter.
// NewGenericForOperation copies all caller-owned slices and paths.
type GenericForOperation struct {
	variableIndex   int
	target          symbol.ID
	firstTarget     symbol.ID
	source          GenericForSource
	sourceContracts []typ.Type
}

func NewGenericForOperation(variableIndex int, target, firstTarget symbol.ID, source GenericForSource, sourceContracts []typ.Type) (GenericForOperation, bool) {
	if variableIndex < 0 || target == 0 {
		return GenericForOperation{}, false
	}
	source.RootPath.Segments = append([]segment.Segment(nil), source.RootPath.Segments...)
	if source.HasCallPoint != (source.CallPoint != 0) || source.HasRootPath != !source.RootPath.IsEmpty() {
		return GenericForOperation{}, false
	}
	return GenericForOperation{
		variableIndex:   variableIndex,
		target:          target,
		firstTarget:     firstTarget,
		source:          source,
		sourceContracts: append([]typ.Type(nil), sourceContracts...),
	}, true
}

func (o GenericForOperation) VariableIndex() int     { return o.variableIndex }
func (o GenericForOperation) Target() symbol.ID      { return o.target }
func (o GenericForOperation) FirstTarget() symbol.ID { return o.firstTarget }

func (o GenericForOperation) Source() GenericForSource {
	source := o.source
	source.RootPath.Segments = append([]segment.Segment(nil), source.RootPath.Segments...)
	return source
}

func (o GenericForOperation) SourceContract(index int) (typ.Type, bool) {
	if index < 0 || index >= len(o.sourceContracts) || o.sourceContracts[index] == nil {
		return nil, false
	}
	return o.sourceContracts[index], true
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

// ApplyConcreteGenericFor executes one loop-variable binding atomically.
// Cancellation before or after a provider callback publishes no prefix.
func ApplyConcreteGenericFor(req ConcreteGenericForRequest) ConcreteGenericForResult {
	token := req.Context.Session.Token()
	if token != nil && token.Canceled() {
		return ConcreteGenericForResult{Output: req.Input, Canceled: true}
	}
	op := req.Operation
	if op.Target() == 0 {
		return ConcreteGenericForResult{Output: req.Output}
	}
	out := req.Output
	targetPath := pathdom.Path{Symbol: op.Target()}
	if req.Resolver != nil {
		if targetKey, ok := visibility.AddressAt(req.Resolver, req.Context.Point, targetPath).VisibleStateKey(); ok {
			out = out.ClearKeyMembershipsForPath(targetKey)
		}
	}
	if req.Semantics != nil {
		if value, ok := req.Semantics.ResolveGenericFor(req.Context, op, req.Input); ok {
			out = out.WriteValue(req.Context.Registry, key.SymbolValue(op.Target()), value)
		}
	}
	if token != nil && token.Canceled() {
		return ConcreteGenericForResult{Output: req.Input, Canceled: true}
	}
	if req.Semantics != nil {
		out = req.Semantics.ApplyGenericForMembership(req.Context, op, out, targetPath)
	}
	if token != nil && token.Canceled() {
		return ConcreteGenericForResult{Output: req.Input, Canceled: true}
	}
	return ConcreteGenericForResult{Output: out}
}

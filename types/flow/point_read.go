package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// PointReadPolicy supplies checker-owned evidence needed by the flow point
// reader without letting callers re-compose individual point-state axes.
type PointReadPolicy struct {
	UnannotatedSymbol func(cfg.SymbolID) bool
	CallableSignature CallableSignatureResolver
}

// ReadSymbolValue returns the product value visible for sym under policy.
func (f PointFacts) ReadSymbolValue(sym cfg.SymbolID, policy PointReadPolicy) ProductValue {
	if sym == 0 {
		return ProductValue{State: StateUnknown}
	}
	av, ok := f.SymbolValue(sym)
	if ok && !av.IsZero() {
		return ProductValue{Value: av, State: StateResolved}
	}
	if policy.UnannotatedSymbol != nil && policy.UnannotatedSymbol(sym) {
		return ProductValue{Value: product.GradualAny(), State: StateResolved}
	}
	return ProductValue{State: StateUnknown}
}

// ReadPathValue returns the product value visible for path under policy. It
// applies point-local product facts, callable identity overlays, and gradual
// unannotated-root reads in one normalized flow-domain law.
func (f PointFacts) ReadPathValue(path constraint.Path, policy PointReadPolicy) ProductValue {
	if path.Symbol == 0 {
		return ProductValue{State: StateUnknown}
	}
	if len(path.Segments) == 0 {
		return f.ReadSymbolValue(path.Symbol, policy)
	}
	if pv := f.ReadCallablePathValue(path, policy); pv.State == StateResolved {
		return pv
	}
	if policy.UnannotatedSymbol != nil && policy.UnannotatedSymbol(path.Symbol) {
		root := f.ReadSymbolValue(path.Symbol, policy)
		if root.State == StateResolved && !root.Value.IsZero() {
			if !root.Value.IsGradualTop() {
				return ProductValue{State: StateUnknown}
			}
			if av, ok := ProductMemberPathValue(root.Value, path.Segments); ok && !av.IsZero() {
				return ProductValue{Value: av, State: StateResolved}
			}
			return ProductValue{State: StateUnknown}
		}
		if av, ok := ProductMemberPathValue(product.GradualAny(), path.Segments); ok && !av.IsZero() {
			return ProductValue{Value: av, State: StateResolved}
		}
	}
	return ProductValue{State: StateUnknown}
}

// ReadStaticMemberValue returns the exact static-member fact for path, with
// callable identity evidence folded over the runtime read when available.
func (f PointFacts) ReadStaticMemberValue(path constraint.Path, policy PointReadPolicy) ProductValue {
	read, ok := f.StaticMemberValue(path)
	if !ok || read.IsZero() {
		return ProductValue{State: StateUnknown}
	}
	return f.ReadCallablePath(path, read, policy)
}

// ReadCallablePathValue reads path through the callable identity overlay
// without applying checker-only gradual parameter fallback.
func (f PointFacts) ReadCallablePathValue(path constraint.Path, policy PointReadPolicy) ProductValue {
	av, ok := f.CallablePathValue(path, policy.CallableSignature)
	if !ok || av.IsZero() {
		return ProductValue{State: StateUnknown}
	}
	return ProductValue{Value: av, State: StateResolved}
}

// ReadCallablePath overlays callable identity facts over an already-computed
// runtime read, preserving the read when no callable evidence exists.
func (f PointFacts) ReadCallablePath(path constraint.Path, read product.AbstractValue, policy PointReadPolicy) ProductValue {
	av, ok := f.CallablePathRead(path, read, policy.CallableSignature)
	if !ok || av.IsZero() {
		return ProductValue{State: StateUnknown}
	}
	return ProductValue{Value: av, State: StateResolved}
}

// ReadKnownCallablePath overlays callable evidence only when the point state can
// prove path is callable. Callers use this for strict member/index reads where a
// missing product slot should not become present unless callable identity says so.
func (f PointFacts) ReadKnownCallablePath(path constraint.Path, read product.AbstractValue, policy PointReadPolicy) ProductValue {
	if _, ok := f.CallablePathType(path, policy.CallableSignature); !ok {
		return ProductValue{State: StateUnknown}
	}
	return f.ReadCallablePath(path, read, policy)
}

// ReadPathType projects ReadPathValue to a concrete type and applies the
// point-local length proof that makes static sequence-index reads non-optional.
func (f PointFacts) ReadPathType(path constraint.Path, policy PointReadPolicy) TypedValue {
	pv := f.ReadPathValue(path, policy)
	if pv.State != StateResolved || pv.Value.IsZero() {
		return TypedValue{Type: nil, State: StateUnknown}
	}
	t := product.ProjectValueOrUnknown(pv.Value)
	if typ.IsAbsentOrUnknown(t) {
		return TypedValue{Type: nil, State: StateUnknown}
	}
	if len(path.Segments) > 0 {
		t = f.refineStaticIndexPathType(path, t)
	}
	return TypedValue{Type: t, State: StateResolved}
}

func (f PointFacts) refineStaticIndexPathType(path constraint.Path, t typ.Type) typ.Type {
	if t == nil || len(path.Segments) == 0 {
		return t
	}
	idx := -1
	for i := len(path.Segments) - 1; i >= 0; i-- {
		if path.Segments[i].Kind == constraint.SegmentIndexInt {
			idx = i
			break
		}
	}
	if idx < 0 {
		return t
	}
	seg := path.Segments[idx]
	if seg.Index < 1 {
		return t
	}
	containerPath := constraint.Path{
		Root:     path.Root,
		Symbol:   path.Symbol,
		Version:  path.Version,
		Segments: append([]constraint.Segment(nil), path.Segments[:idx]...),
	}
	lower, ok := f.LengthLowerBound(containerPath)
	if !ok && len(containerPath.Segments) == 0 {
		lower, ok = f.LengthLowerBound(constraint.NewPath(path.Symbol, ""))
	}
	if !ok || lower < int64(seg.Index) {
		return t
	}
	container, ok := f.readStaticIndexContainerType(containerPath)
	if !ok || typ.IsAbsentOrUnknown(container) {
		return t
	}
	if refined := narrow.RefineSequenceIndex(container, t, int64(seg.Index)); refined != nil {
		return refined
	}
	return t
}

func (f PointFacts) readStaticIndexContainerType(path constraint.Path) (typ.Type, bool) {
	if path.Symbol == 0 {
		return nil, false
	}
	if len(path.Segments) == 0 {
		return f.SymbolType(path.Symbol)
	}
	return f.PathType(path)
}

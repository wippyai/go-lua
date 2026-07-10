package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ReportableFunctionResults returns nested function results that should own
// post-solve reports. When a call-context materialization exists for the same
// function expression, the context-independent summary body is suppressed unless
// it has an explicit validation surface of its own.
func (r *Result) ReportableFunctionResults() []*Result {
	functions := r.FunctionResults()
	if len(functions) == 0 {
		return nil
	}
	hasContext := make(map[*ast.FunctionExpr]struct{})
	for _, fn := range functions {
		if fn == nil || !fn.IsCallContextResult() {
			continue
		}
		if expr := fn.Function(); expr != nil {
			hasContext[expr] = struct{}{}
		}
	}
	if len(hasContext) == 0 {
		return functions
	}
	out := make([]*Result, 0, len(functions))
	for _, fn := range functions {
		if fn == nil {
			continue
		}
		if !fn.IsCallContextResult() {
			if expr := fn.Function(); expr != nil {
				if _, ok := hasContext[expr]; ok && fn.hasImplicitSelfParameter() {
					continue
				}
				if _, ok := hasContext[expr]; ok && !fn.hasImplicitSelfEntrySurface() && !fn.hasExplicitValidationSurface() {
					continue
				}
			}
		}
		out = append(out, fn)
	}
	return out
}

func (r *Result) hasExplicitValidationSurface() bool {
	if r == nil {
		return false
	}
	fn := r.Function()
	if fn == nil {
		return false
	}
	if len(fn.TypeParams) != 0 {
		return false
	}
	if len(fn.ReturnTypes) != 0 {
		return true
	}
	for _, slot := range r.FunctionParamSlots(fn) {
		if !slot.ImplicitSelf && slot.Type != nil {
			return true
		}
	}
	if fn.ParList == nil {
		return false
	}
	for _, expr := range fn.ParList.Types {
		if expr != nil {
			return true
		}
	}
	return fn.ParList.VarargType != nil
}

func (r *Result) hasImplicitSelfParameter() bool {
	if r == nil {
		return false
	}
	fn := r.Function()
	if fn == nil {
		return false
	}
	for _, slot := range r.FunctionParamSlots(fn) {
		if slot.ImplicitSelf {
			return true
		}
	}
	return false
}

func (r *Result) hasImplicitSelfEntrySurface() bool {
	if r == nil {
		return false
	}
	fn := r.Function()
	if fn == nil {
		return false
	}
	self := symbol.ID(0)
	for _, slot := range r.FunctionParamSlots(fn) {
		if slot.ImplicitSelf && slot.Symbol != 0 {
			self = slot.Symbol
			break
		}
	}
	if self == 0 {
		return false
	}
	entry, ok := r.EntryState()
	if !ok {
		return false
	}
	ks := r.KeySpace()
	if ks == nil {
		return false
	}
	found := false
	entry.ForEachPathStaticMember(func(memberKey keyspace.Key, _ product.Value) bool {
		if memberKey.Sym != self {
			return true
		}
		switch memberKey.Kind {
		case keyspace.KindResolverSym, keyspace.KindStableSym:
			if len(ks.Segments(memberKey)) != 0 {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

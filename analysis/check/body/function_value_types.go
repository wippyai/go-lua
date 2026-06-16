package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// FunctionValueTypes is a read-only projection of converged function summaries
// into callable value types. The owner remains the fixed-point summary; body
// results use this only for post-solve reads such as diagnostics.
type FunctionValueTypes struct {
	ByIdentity         map[identity.ID]*typ.Function
	ByPath             map[pathdom.PathKey]*typ.Function
	ContextsByIdentity map[identity.ID][]FunctionValueContext
}

type FunctionValueContext struct {
	Entry state.State
	Type  *typ.Function
}

// WithFunctionValueTypes returns result after installing an immutable copy of
// the inferred function-value type projection.
func WithFunctionValueTypes(result *Result, types FunctionValueTypes) *Result {
	if result == nil {
		return nil
	}
	result.funcTypes = cloneFunctionValueTypes(types)
	return result
}

// FunctionValueTypeAtBoundary resolves expr's current callable value type at
// point. Runtime identity wins over syntactic path so reassigned function fields
// use the currently visible value rather than an older path definition.
func (r *Result) FunctionValueTypeAtBoundary(point cfg.Point, expr ast.Expr) (*typ.Function, bool) {
	if r == nil || expr == nil {
		return nil, false
	}
	p, ok := r.ExpressionPath(expr)
	if !ok || p.IsEmpty() {
		return nil, false
	}
	current, hasCurrent := r.StateAt(point)
	if value, ok := r.ExpressionValueAtBoundary(point, expr); ok {
		if fn, ok := r.functionTypeForValue(current, hasCurrent, value); ok {
			return fn, true
		}
		if _, hasID := product.Get(r.registry, value, identity.Key).ID(); hasID {
			return nil, false
		}
	}
	if fn, ok := r.funcTypes.ByPath[p.Key()]; ok && fn != nil {
		return fn, true
	}
	return nil, false
}

func (r *Result) functionTypeForValue(current state.State, hasCurrent bool, value product.Value) (*typ.Function, bool) {
	if r == nil || r.registry == nil {
		return nil, false
	}
	id, ok := product.Get(r.registry, value, identity.Key).ID()
	if !ok {
		return nil, false
	}
	if hasCurrent {
		for _, ctx := range r.funcTypes.ContextsByIdentity[id] {
			if ctx.Type != nil && functionContextEntryHolds(r.registry, ctx.Entry, current, id) {
				return ctx.Type, true
			}
		}
	}
	fn, ok := r.funcTypes.ByIdentity[id]
	return fn, ok && fn != nil
}

func functionContextEntryHolds(reg *axis.Registry, entry, current state.State, sourceID identity.ID) bool {
	if reg == nil {
		return false
	}
	refs := entry.PathRefinementsSnapshot()
	if refs.Bottom {
		return false
	}
	for pathKey, want := range refs.Refinements {
		got := current.ReadPathKey(reg, pathKey)
		if !product.LessOrEq(reg, want, got) {
			if valueHasIdentity(reg, want, sourceID) && product.Equal(reg, got, product.Bottom(reg)) {
				continue
			}
			return false
		}
	}
	members := entry.PathStaticMembersSnapshot()
	if members.Bottom {
		return false
	}
	for pathKey, want := range members.Members {
		got, ok := current.ReadPathStaticMember(pathKey)
		if !ok || !product.LessOrEq(reg, want, got) {
			return false
		}
	}
	requiredHeap := entry.HeapTableObjectsSnapshot()
	if requiredHeap.Top {
		return false
	}
	currentHeap := current.HeapTableObjectsSnapshot()
	if !currentHeap.Top && !heapidentity.MapDomain(reg).LessOrEq(requiredHeap.Objects, currentHeap.Objects) {
		return false
	}
	return true
}

func valueHasIdentity(reg *axis.Registry, value product.Value, id identity.ID) bool {
	got, ok := product.Get(reg, value, identity.Key).ID()
	return ok && got == id
}

func cloneFunctionValueTypes(in FunctionValueTypes) FunctionValueTypes {
	out := FunctionValueTypes{}
	if len(in.ByIdentity) != 0 {
		out.ByIdentity = make(map[identity.ID]*typ.Function, len(in.ByIdentity))
		for id, fn := range in.ByIdentity {
			out.ByIdentity[id] = fn
		}
	}
	if len(in.ByPath) != 0 {
		out.ByPath = make(map[pathdom.PathKey]*typ.Function, len(in.ByPath))
		for key, fn := range in.ByPath {
			out.ByPath[key] = fn
		}
	}
	if len(in.ContextsByIdentity) != 0 {
		out.ContextsByIdentity = make(map[identity.ID][]FunctionValueContext, len(in.ContextsByIdentity))
		for id, contexts := range in.ContextsByIdentity {
			copied := make([]FunctionValueContext, len(contexts))
			for i, ctx := range contexts {
				copied[i] = FunctionValueContext{
					Entry: ctx.Entry.Snapshot(),
					Type:  ctx.Type,
				}
			}
			out.ContextsByIdentity[id] = copied
		}
	}
	return out
}

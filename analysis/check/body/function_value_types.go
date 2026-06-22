package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
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
	ByPath             map[factflow.CalleePathKey]*typ.Function
	ContextsByIdentity map[identity.ID][]FunctionValueContext
}

type FunctionValueContext struct {
	Entry state.State
	// EntryKeys is the structural key interner that produced Entry's path
	// evidence. Entry's value-lane keys are meaningful only within it, so the
	// holds-check formats Entry through EntryKeys, then re-interns each spelling
	// into the consuming analysis's keyspace to read the current state.
	EntryKeys *keyspace.KeySpace
	Type      *typ.Function
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
		return nil, false
	}
	if pathKey, ok := factflow.CalleePathKeyFromPath(p); ok {
		if fn, ok := r.funcTypes.ByPath[pathKey]; ok && fn != nil {
			return fn, true
		}
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
			if ctx.Type != nil && functionContextEntryHolds(r.registry, ctx.EntryKeys, r.KeySpace(), ctx.Entry, current, id) {
				return ctx.Type, true
			}
		}
	}
	fn, ok := r.funcTypes.ByIdentity[id]
	return fn, ok && fn != nil
}

func functionContextEntryHolds(reg *axis.Registry, entryKeys, currentKeys *keyspace.KeySpace, entry, current state.State, sourceID identity.ID) bool {
	if reg == nil {
		return false
	}
	refs := entry.PathRefinementsSnapshot(entryKeys)
	if refs.Bottom {
		return false
	}
	for pathKey, want := range refs.Refinements {
		got := current.ReadPathKey(reg, currentKeys, pathKey)
		if !contextPathValueSatisfies(reg, got, want, sourceID) {
			return false
		}
	}
	members := entry.PathStaticMembersSnapshot(entryKeys)
	if members.Bottom {
		return false
	}
	for pathKey, want := range members.Members {
		got, ok := current.ReadPathStaticMember(currentKeys, pathKey)
		if !ok || !contextValueSatisfies(reg, got, want) {
			return false
		}
	}
	requiredHeap := entry.HeapTableObjectsSnapshot()
	return heapTableContextHolds(reg, entryKeys, currentKeys, requiredHeap, current.HeapTableObjectsSnapshot())
}

func contextPathValueSatisfies(reg *axis.Registry, got, want product.Value, sourceID identity.ID) bool {
	if product.Equal(reg, got, product.Bottom(reg)) {
		return valueHasIdentity(reg, want, sourceID) &&
			product.LessOrEq(reg, sourceIdentityValue(reg, sourceID), want)
	}
	return product.LessOrEq(reg, got, want)
}

func contextValueSatisfies(reg *axis.Registry, got, want product.Value) bool {
	if product.Equal(reg, got, product.Bottom(reg)) {
		return false
	}
	return product.LessOrEq(reg, got, want)
}

func heapTableContextHolds(reg *axis.Registry, entryKeys, currentKeys *keyspace.KeySpace, requiredHeap, currentHeap state.HeapTableObjectsSnapshot) bool {
	if requiredHeap.Top {
		return false
	}
	if len(requiredHeap.Objects) == 0 {
		return true
	}
	if currentHeap.Top {
		return false
	}
	for id, want := range requiredHeap.Objects {
		got, ok := currentHeap.Objects[id]
		if !ok || !heapTableObjectContextHolds(reg, want.Rekey(entryKeys, currentKeys), got) {
			return false
		}
	}
	return true
}

func heapTableObjectContextHolds(reg *axis.Registry, want, got heapidentity.TableObject) bool {
	if !contextValueSatisfies(reg, got.Root(), want.Root()) {
		return false
	}
	for key, wantMember := range want.StaticMembers() {
		gotMember, ok := got.StaticMember(key)
		if !ok || !contextValueSatisfies(reg, gotMember, wantMember) {
			return false
		}
	}
	dynamicDomain := dynamicindex.Domain(reg)
	for key, wantFact := range want.DynamicIndexFacts() {
		gotFact, ok := got.DynamicIndexFact(key)
		if !ok || !dynamicDomain.LessOrEq(gotFact, wantFact) {
			return false
		}
	}
	return true
}

func valueHasIdentity(reg *axis.Registry, value product.Value, id identity.ID) bool {
	got, ok := product.Get(reg, value, identity.Key).ID()
	return ok && got == id
}

func sourceIdentityValue(reg *axis.Registry, id identity.ID) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	return product.Set(reg, value, identity.Key, identity.Singleton(id))
}

func cloneFunctionValueTypes(in FunctionValueTypes) FunctionValueTypes {
	out := in
	out.ByIdentity = nil
	out.ByPath = nil
	out.ContextsByIdentity = nil
	if len(in.ByIdentity) != 0 {
		out.ByIdentity = make(map[identity.ID]*typ.Function, len(in.ByIdentity))
		for id, fn := range in.ByIdentity {
			out.ByIdentity[id] = fn
		}
	}
	if len(in.ByPath) != 0 {
		out.ByPath = make(map[factflow.CalleePathKey]*typ.Function, len(in.ByPath))
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
					Entry:     ctx.Entry.Snapshot(),
					EntryKeys: ctx.EntryKeys,
					Type:      ctx.Type,
				}
			}
			out.ContextsByIdentity[id] = copied
		}
	}
	return out
}

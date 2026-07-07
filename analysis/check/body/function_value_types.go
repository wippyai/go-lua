package body

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/compiler/ast"
)

// FunctionValueTypes is a read-only projection of converged function summaries
// into callable value types. The owner remains the fixed-point summary; body
// results use this only for post-solve reads such as diagnostics.
type FunctionValueTypes struct {
	ByIdentity         map[identity.ID]*typ.Function
	ByPath             map[factflow.CalleePathKey]*typ.Function
	ParamSpansByPath   map[factflow.CalleePathKey][]factflow.SourceSpan
	ReturnSpansByPath  map[factflow.CalleePathKey][]factflow.SourceSpan
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

// WithOwnedFunctionValueTypes returns result after installing types directly.
// Callers must own types and treat every map/slice inside it as immutable after
// this call. Use WithFunctionValueTypes at public or untrusted boundaries.
func WithOwnedFunctionValueTypes(result *Result, types FunctionValueTypes) *Result {
	if result == nil {
		return nil
	}
	result.funcTypes = types
	return result
}

// HasFunctionValueTypes reports whether result already carries types. It lets
// fixed-point materialization avoid rewriting summaries when the converged
// function projection did not change.
func (r *Result) HasFunctionValueTypes(types FunctionValueTypes) bool {
	if r == nil {
		return false
	}
	return FunctionValueTypesEqual(r.registry, r.funcTypes, types)
}

// FunctionValueTypesEqual reports structural equality for the installed
// function-value projection. Map order is irrelevant; context order remains
// significant because context slices are generated in deterministic discovery
// order and later entries can be shadowed by earlier matching entries.
func FunctionValueTypesEqual(reg *axis.Registry, a, b FunctionValueTypes) bool {
	byIdentity := functionTypeMapsEqual(a.ByIdentity, b.ByIdentity)
	byPath := functionTypeMapsEqual(a.ByPath, b.ByPath)
	paramSpans := sourceSpanMapsEqual(a.ParamSpansByPath, b.ParamSpansByPath)
	returnSpans := sourceSpanMapsEqual(a.ReturnSpansByPath, b.ReturnSpansByPath)
	contexts := functionValueContextMapsEqual(reg, a.ContextsByIdentity, b.ContextsByIdentity)
	return byIdentity && byPath && paramSpans && returnSpans && contexts
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
		if valueProvesNonCallable(r.registry, r.typeValues, value) {
			return nil, false
		}
	}
	if pathKey, ok := factflow.CalleePathKeyFromPath(p); ok {
		if fn, ok := r.funcTypes.ByPath[pathKey]; ok && fn != nil {
			return fn, true
		}
	}
	return nil, false
}

// ExpressionProvenFunctionAtBoundary reports whether expr is proven to resolve
// to a Lua function value at point. It follows dominating assignments and
// current function-value summaries so diagnostics do not re-implement function
// binding flow.
func (r *Result) ExpressionProvenFunctionAtBoundary(point cfg.Point, expr ast.Expr) bool {
	if r == nil || expr == nil {
		return false
	}
	if _, ok := functionLiteralExpr(expr); ok {
		return true
	}
	if _, ok := r.FunctionValueTypeAtBoundary(point, expr); ok {
		return true
	}
	p, ok := r.ExpressionPath(expr)
	if !ok || p.IsEmpty() {
		return false
	}
	return r.pathProvenFunctionAtBoundary(point, p, nil)
}

// ExpressionMayBeFunctionBeforeBoundary reports whether expr is either proven
// callable or still may be a function before point's node-local effects run.
// Unknown reads return true so callers that model open registration/mutation
// stay conservative.
func (r *Result) ExpressionMayBeFunctionBeforeBoundary(point cfg.Point, expr ast.Expr) bool {
	if r.ExpressionProvenFunctionAtBoundary(point, expr) {
		return true
	}
	if r == nil || r.registry == nil {
		return true
	}
	value, ok := r.ExpressionValueBeforeBoundary(point, expr)
	if !ok {
		return true
	}
	if product.Get(r.registry, value, runtimekind.Key).Contains(runtimekind.Function) {
		return true
	}
	if valueType, ok := typevalue.TypeOf(r.registry, value); ok {
		return typ.IsAny(valueType) || typ.IsUnknown(valueType)
	}
	return false
}

func (r *Result) pathProvenFunctionAtBoundary(point cfg.Point, target pathdom.Path, seen map[pathdom.PathKey]struct{}) bool {
	graph := r.Graph()
	if graph == nil || target.IsEmpty() {
		return false
	}
	key := target.Key()
	if _, ok := seen[key]; ok {
		return false
	}
	if seen == nil {
		seen = make(map[pathdom.PathKey]struct{}, 1)
	}
	seen[key] = struct{}{}
	if r.DominatingFunctionDefinitionForPath(point, target) != nil {
		return true
	}
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return false
		}
		visited[cursor] = struct{}{}
		if fact, ok := r.LocalAssignment(cursor); ok &&
			len(target.Segments) == 0 &&
			fact.HasSymbol &&
			fact.Symbol == target.Symbol {
			return r.assignmentSourceProvenFunctionAtBoundary(cursor, fact.Expr, seen)
		}
		if fact, ok := r.OrdinaryAssignment(cursor); ok {
			if len(target.Segments) == 0 &&
				fact.HasSymbol &&
				fact.Symbol == target.Symbol {
				return r.assignmentSourceProvenFunctionAtBoundary(cursor, fact.Value, seen)
			}
			if fact.HasPath && fact.Path.Equal(target) {
				return r.assignmentSourceProvenFunctionAtBoundary(cursor, fact.Value, seen)
			}
			if fact.HasPath && target.HasPrefix(fact.Path) {
				return false
			}
		}
		parent, ok := r.ImmediateDominator(cursor)
		if !ok || parent == cursor {
			return false
		}
		cursor = parent
	}
}

func (r *Result) assignmentSourceProvenFunctionAtBoundary(point cfg.Point, expr ast.Expr, seen map[pathdom.PathKey]struct{}) bool {
	if _, ok := functionLiteralExpr(expr); ok {
		return true
	}
	if _, ok := r.FunctionValueTypeAtBoundary(point, expr); ok {
		return true
	}
	p, ok := r.ExpressionPath(expr)
	if !ok || p.IsEmpty() {
		return false
	}
	return r.pathProvenFunctionAtBoundary(point, p, seen)
}

func functionLiteralExpr(expr ast.Expr) (*ast.FunctionExpr, bool) {
	if fn, ok := expr.(*ast.FunctionExpr); ok {
		return fn, true
	}
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return nil, false
	}
	fn, ok := inner.(*ast.FunctionExpr)
	return fn, ok
}

// FunctionValueTypeForValue resolves a callable value's converged function type
// from its identity, independent of a program point. Context-sensitive entries
// still require FunctionValueTypeAtBoundary; this read model is for structural
// export/materialization paths that already hold the function value itself.
func (r *Result) FunctionValueTypeForValue(value product.Value) (*typ.Function, bool) {
	if r == nil {
		return nil, false
	}
	return r.functionTypeForValue(state.State{}, false, value)
}

// FunctionValueTypeForValueAtBoundary resolves a callable value's converged
// function type at a program point, including context-sensitive summaries whose
// entry facts still hold in the current state. It is the syntax-free companion
// to FunctionValueTypeAtBoundary for consumers that already hold the solved
// value, such as readmodel call arguments.
func (r *Result) FunctionValueTypeForValueAtBoundary(point cfg.Point, value product.Value) (*typ.Function, bool) {
	if r == nil {
		return nil, false
	}
	current, hasCurrent := r.StateAt(point)
	return r.functionTypeForValue(current, hasCurrent, value)
}

// FunctionValueTypeForPathValueAtBoundary resolves a callable path's converged
// function type using the currently solved value as the freshness guard. A path
// summary is used only when the value at the consumer is still callable, so a
// later reassignment to a non-function cannot reuse an older definition.
func (r *Result) FunctionValueTypeForPathValueAtBoundary(point cfg.Point, p pathdom.Path, value product.Value) (*typ.Function, bool) {
	if r == nil || p.IsEmpty() {
		return nil, false
	}
	if fn, ok := r.FunctionValueTypeForValueAtBoundary(point, value); ok {
		return fn, true
	}
	if !valueHasCallableType(r.registry, r.typeValues, value) {
		return nil, false
	}
	if key, ok := factflow.CalleePathKeyFromPath(p); ok {
		return r.FunctionValueTypeForCalleePath(key)
	}
	return nil, false
}

// FunctionValueTypeForCalleePath resolves a converged local function-value type
// by the callee path key carried in call-site evidence. It is intentionally
// syntax-free so post-solve consumers do not re-lower AST parameter slots.
func (r *Result) FunctionValueTypeForCalleePath(key factflow.CalleePathKey) (*typ.Function, bool) {
	if r == nil || !key.Valid() {
		return nil, false
	}
	fn, ok := r.funcTypes.ByPath[key]
	return fn, ok && fn != nil
}

// FunctionParamTypeSpansForCalleePath returns immutable parameter annotation
// spans associated with a function-value summary path, when that path came from
// a local function definition.
func (r *Result) FunctionParamTypeSpansForCalleePath(key factflow.CalleePathKey) []factflow.SourceSpan {
	if r == nil || !key.Valid() || len(r.funcTypes.ParamSpansByPath) == 0 {
		return nil
	}
	spans := r.funcTypes.ParamSpansByPath[key]
	if len(spans) == 0 {
		return nil
	}
	return append([]factflow.SourceSpan(nil), spans...)
}

// FunctionReturnTypeSpansForCalleePath returns immutable return annotation
// spans associated with a function-value summary path, when that path came from
// a local function definition.
func (r *Result) FunctionReturnTypeSpansForCalleePath(key factflow.CalleePathKey) []factflow.SourceSpan {
	if r == nil || !key.Valid() || len(r.funcTypes.ReturnSpansByPath) == 0 {
		return nil
	}
	spans := r.funcTypes.ReturnSpansByPath[key]
	if len(spans) == 0 {
		return nil
	}
	return append([]factflow.SourceSpan(nil), spans...)
}

// FunctionValueTypeForCallSiteAtBoundary resolves a call site's current
// callable value type without re-reading syntax. The point-visible value wins
// over the callee path summary so a reassigned callee cannot reuse a stale
// definition-path contract.
func (r *Result) FunctionValueTypeForCallSiteAtBoundary(point cfg.Point, site factflow.CallSite) (*typ.Function, bool) {
	if r == nil {
		return nil, false
	}
	p := site.CalleePathRef()
	if p.IsEmpty() {
		return nil, false
	}
	if value, ok := r.PathValueAtBoundary(point, p); ok {
		if fn, ok := r.FunctionValueTypeForValueAtBoundary(point, value); ok {
			return fn, true
		}
		fn, callable := callableTypeFromValue(r.registry, r.typeValues, value)
		if site.CalleeMemberAccess() && callable && fn != nil {
			return fn, true
		}
		if !callable && valueProvesNonCallable(r.registry, r.typeValues, value) {
			return nil, false
		}
	}
	if len(p.Segments) > 0 {
		if root, ok := r.currentRootValueForCallableFallback(point, p); ok && valueProvesNonTable(r.registry, r.typeValues, root) {
			return nil, false
		}
	}
	return r.FunctionValueTypeForCalleePath(site.View().CalleePathKey())
}

func (r *Result) currentRootValueForCallableFallback(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if root, ok := r.PathValueAtBoundary(point, p.RootOnly()); ok &&
		!product.Equal(r.registry, root, product.Bottom(r.registry)) &&
		!product.Equal(r.registry, root, product.Top()) {
		return root, true
	}
	if p.Symbol == 0 || r == nil || r.registry == nil {
		return product.Value{}, false
	}
	st, ok := r.StateAt(point)
	if !ok {
		return product.Value{}, false
	}
	root := st.ReadValue(r.registry, statekey.SymbolValue(p.Symbol))
	if product.Equal(r.registry, root, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return root, true
}

func valueHasCallableType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	_, ok := callableTypeFromValue(reg, typeValues, value)
	return ok
}

func valueProvesNonCallable(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	if reg == nil {
		return false
	}
	kinds := product.Get(reg, value, runtimekind.Key)
	if !kinds.IsBottom() && !kinds.IsTop() && !kinds.Contains(runtimekind.Function) {
		return true
	}
	if typeValues == nil {
		return false
	}
	t, ok := typeValues.TypeOf(reg, value)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return false
	}
	_, callable := typecall.Callable(t)
	return !callable
}

func valueProvesNonTable(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	if reg == nil {
		return false
	}
	kinds := product.Get(reg, value, runtimekind.Key)
	if !kinds.IsBottom() && !kinds.IsTop() && !kinds.Contains(runtimekind.Table) {
		return true
	}
	if typeValues == nil {
		return false
	}
	t, ok := typeValues.TypeOf(reg, value)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return false
	}
	return !staticMemberReadRuntimeTableMayMatch(t)
}

func callableTypeFromValue(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (*typ.Function, bool) {
	if reg == nil {
		return nil, false
	}
	t, ok := typeValues.TypeOf(reg, value)
	if !ok || t == nil {
		return nil, false
	}
	return typecall.Callable(t)
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
	values := entry.ValuesSnapshot()
	if values.Top {
		return false
	}
	for slot, want := range values.Values {
		got := current.ReadValue(reg, slot)
		if !contextValueSatisfies(reg, got, want) {
			return false
		}
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
	out.ParamSpansByPath = nil
	out.ReturnSpansByPath = nil
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
	if len(in.ParamSpansByPath) != 0 {
		out.ParamSpansByPath = make(map[factflow.CalleePathKey][]factflow.SourceSpan, len(in.ParamSpansByPath))
		for key, spans := range in.ParamSpansByPath {
			out.ParamSpansByPath[key] = append([]factflow.SourceSpan(nil), spans...)
		}
	}
	if len(in.ReturnSpansByPath) != 0 {
		out.ReturnSpansByPath = make(map[factflow.CalleePathKey][]factflow.SourceSpan, len(in.ReturnSpansByPath))
		for key, spans := range in.ReturnSpansByPath {
			out.ReturnSpansByPath[key] = append([]factflow.SourceSpan(nil), spans...)
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

type functionTypeMapKey interface {
	identity.ID | factflow.CalleePathKey
}

func functionTypeMapsEqual[K functionTypeMapKey](a, b map[K]*typ.Function) bool {
	if len(a) != len(b) {
		return false
	}
	for key, left := range a {
		right, ok := b[key]
		if !ok || !functionTypesEqual(left, right) {
			return false
		}
	}
	return true
}

func functionTypesEqual(a, b *typ.Function) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equals(b)
}

func sourceSpanMapsEqual[K comparable](a, b map[K][]factflow.SourceSpan) bool {
	if len(a) != len(b) {
		return false
	}
	for key, left := range a {
		right, ok := b[key]
		if !ok || !sourceSpansEqual(left, right) {
			return false
		}
	}
	return true
}

func sourceSpansEqual(a, b []factflow.SourceSpan) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func functionValueContextMapsEqual(reg *axis.Registry, a, b map[identity.ID][]FunctionValueContext) bool {
	if len(a) != len(b) {
		return false
	}
	for id, left := range a {
		right, ok := b[id]
		if !ok || !functionValueContextsEqual(reg, left, right) {
			return false
		}
	}
	return true
}

func functionValueContextsEqual(reg *axis.Registry, a, b []FunctionValueContext) bool {
	if len(a) != len(b) {
		return false
	}
	domain := state.Domain(reg)
	for i := range a {
		if !functionValueContextEntryEqual(domain, a[i], b[i]) ||
			!functionTypesEqual(a[i].Type, b[i].Type) {
			return false
		}
	}
	return true
}

func functionValueContextEntryEqual(domain lattice.Lattice[state.State], a, b FunctionValueContext) bool {
	left := a.Entry
	if a.EntryKeys != b.EntryKeys {
		left = left.RekeyPathEvidence(a.EntryKeys, b.EntryKeys)
	}
	return domain.Equal(left, b.Entry)
}

package summary

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// DiagnosticContextFrontier owns the diagnostic bridge's context discovery over
// already-converged summary dependencies. The driver supplies topology and
// projection callbacks; summary owns the key traversal, fallback ordering, and
// exact observer state cache because those are summary-context semantics rather
// than module-driver policy.
type DiagnosticContextFrontier struct {
	Root FuncRef
	Refs []FuncRef

	ValidKey   func(Key) bool
	DefaultKey func(FuncRef) Key
	Solve      func(Key) state.FunctionState

	ProjectCalls    func(FuncRef, state.FunctionState) []Key
	ProjectClosures func(FuncRef, state.FunctionState) []Key
}

// DiagnosticContextResult is the finite context set diagnostics should observe,
// plus the exact observer states already solved while discovering the frontier.
type DiagnosticContextResult struct {
	Contexts map[FuncRef][]Key
	States   map[Key]state.FunctionState
}

// State returns the cached exact observer state for key.
func (r DiagnosticContextResult) State(key Key) (state.FunctionState, bool) {
	if len(r.States) == 0 {
		return state.FunctionState{}, false
	}
	fs, ok := r.States[key]
	return fs, ok
}

// Build discovers the finite diagnostic context frontier. Primary contexts are
// actual call-site contexts reachable from the root. Closure/default contexts are
// fallbacks for functions with no primary context; they are never joined into a
// function that already has an actual call context.
func (f DiagnosticContextFrontier) Build() DiagnosticContextResult {
	b := diagnosticContextBuilder{
		frontier: f,
		result: DiagnosticContextResult{
			Contexts: make(map[FuncRef][]Key),
			States:   make(map[Key]state.FunctionState),
		},
		contextSet: make(map[FuncRef]map[Key]struct{}),
	}
	b.build()
	return b.result
}

type diagnosticContextBuilder struct {
	frontier   DiagnosticContextFrontier
	result     DiagnosticContextResult
	contextSet map[FuncRef]map[Key]struct{}
	seen       map[Key]struct{}

	primaryRefs      map[FuncRef]bool
	closureFallbacks map[FuncRef][]Key
}

func (b *diagnosticContextBuilder) build() {
	b.seen = make(map[Key]struct{})
	b.discoverReachable()
}

func (b *diagnosticContextBuilder) discoverReachable() {
	root := b.frontier.Root
	rootKey := b.defaultKey(root)
	b.result.Contexts = make(map[FuncRef][]Key)
	b.contextSet = make(map[FuncRef]map[Key]struct{})
	b.primaryRefs = make(map[FuncRef]bool)
	b.closureFallbacks = make(map[FuncRef][]Key)
	if !b.valid(rootKey) {
		return
	}
	b.markSeen(rootKey)
	visited := map[Key]struct{}{rootKey: {}}
	work := []Key{rootKey}
	b.addContext(rootKey)
	b.primaryRefs[root] = true
	enqueue := func(work *[]Key, key Key) {
		b.markSeen(key)
		if _, ok := visited[key]; ok {
			return
		}
		visited[key] = struct{}{}
		*work = append(*work, key)
	}
	for len(work) > 0 {
		key := work[0]
		work = work[1:]
		fs := b.solve(key)
		for _, next := range b.projectClosures(key.Ref, fs) {
			if !b.valid(next) {
				continue
			}
			b.closureFallbacks[next.Ref] = append(b.closureFallbacks[next.Ref], next)
		}
		for _, next := range b.projectCalls(key.Ref, fs) {
			if !b.valid(next) {
				continue
			}
			b.promotePrimary(next.Ref)
			b.addContext(next)
			enqueue(&work, next)
		}
	}

	var fallbackWork []Key
	for _, ref := range b.frontier.Refs {
		if ref == root || b.primaryRefs[ref] || len(b.result.Contexts[ref]) != 0 {
			continue
		}
		for _, key := range b.closureFallbacks[ref] {
			b.addContext(key)
			enqueue(&fallbackWork, key)
		}
	}
	for len(fallbackWork) > 0 {
		key := fallbackWork[0]
		fallbackWork = fallbackWork[1:]
		fs := b.solve(key)
		for _, next := range b.projectCalls(key.Ref, fs) {
			if !b.valid(next) {
				continue
			}
			b.promotePrimary(next.Ref)
			b.addContext(next)
			enqueue(&fallbackWork, next)
		}
		for _, next := range b.projectClosures(key.Ref, fs) {
			if !b.valid(next) || b.primaryRefs[next.Ref] {
				continue
			}
			b.addContext(next)
			enqueue(&fallbackWork, next)
		}
	}
	for _, ref := range b.frontier.Refs {
		if ref == root || len(b.result.Contexts[ref]) != 0 {
			continue
		}
		key := b.defaultKey(ref)
		if !b.valid(key) {
			continue
		}
		b.addContext(key)
		enqueue(&fallbackWork, key)
	}
	for len(fallbackWork) > 0 {
		key := fallbackWork[0]
		fallbackWork = fallbackWork[1:]
		fs := b.solve(key)
		for _, next := range b.projectCalls(key.Ref, fs) {
			if !b.valid(next) {
				continue
			}
			b.promotePrimary(next.Ref)
			b.addContext(next)
			enqueue(&fallbackWork, next)
		}
		for _, next := range b.projectClosures(key.Ref, fs) {
			if !b.valid(next) || b.primaryRefs[next.Ref] {
				continue
			}
			b.addContext(next)
			enqueue(&fallbackWork, next)
		}
	}
}

func (b *diagnosticContextBuilder) solve(key Key) state.FunctionState {
	if fs, ok := b.result.State(key); ok {
		return fs
	}
	fs := b.solveFresh(key)
	b.result.States[key] = fs
	return fs
}

func (b *diagnosticContextBuilder) solveFresh(key Key) state.FunctionState {
	if b.frontier.Solve == nil {
		return state.FunctionStateDomain.Bottom()
	}
	return b.frontier.Solve(key)
}

func (b *diagnosticContextBuilder) defaultKey(ref FuncRef) Key {
	if b.frontier.DefaultKey != nil {
		return b.frontier.DefaultKey(ref)
	}
	return NewDefaultKey(ref, nil)
}

func (b *diagnosticContextBuilder) valid(key Key) bool {
	if b.frontier.ValidKey == nil {
		return true
	}
	return b.frontier.ValidKey(key)
}

func (b *diagnosticContextBuilder) projectCalls(ref FuncRef, fs state.FunctionState) []Key {
	if b.frontier.ProjectCalls == nil {
		return nil
	}
	return b.frontier.ProjectCalls(ref, fs)
}

func (b *diagnosticContextBuilder) projectClosures(ref FuncRef, fs state.FunctionState) []Key {
	if b.frontier.ProjectClosures == nil {
		return nil
	}
	return b.frontier.ProjectClosures(ref, fs)
}

func (b *diagnosticContextBuilder) projectContexts(ref FuncRef, fs state.FunctionState) []Key {
	keys := b.projectCalls(ref, fs)
	keys = append(keys, b.projectClosures(ref, fs)...)
	return keys
}

func (b *diagnosticContextBuilder) promotePrimary(ref FuncRef) bool {
	if b.primaryRefs[ref] {
		return false
	}
	b.primaryRefs[ref] = true
	delete(b.contextSet, ref)
	delete(b.result.Contexts, ref)
	return true
}

func (b *diagnosticContextBuilder) enqueueIfNew(work *[]Key, key Key) bool {
	if !b.markSeen(key) {
		return false
	}
	*work = append(*work, key)
	return true
}

func (b *diagnosticContextBuilder) markSeen(key Key) bool {
	if _, ok := b.seen[key]; ok {
		return false
	}
	b.seen[key] = struct{}{}
	return true
}

func (b *diagnosticContextBuilder) addContext(key Key) bool {
	ref := key.Ref
	seen := b.contextSet[ref]
	if seen == nil {
		seen = make(map[Key]struct{})
		b.contextSet[ref] = seen
	}
	if _, ok := seen[key]; ok {
		return false
	}
	contexts := b.result.Contexts[ref]
	dst := contexts[:0]
	changed := false
	for _, existing := range contexts {
		if diagnosticContextSameBase(existing, key) {
			if diagnosticContextDominates(existing, key) {
				b.result.Contexts[ref] = contexts
				return false
			}
			if diagnosticContextDominates(key, existing) {
				delete(seen, existing)
				changed = true
				continue
			}
		}
		dst = append(dst, existing)
	}
	if changed {
		b.result.Contexts[ref] = dst
	}
	seen[key] = struct{}{}
	b.result.Contexts[ref] = append(b.result.Contexts[ref], key)
	return true
}

func diagnosticContextSameBase(a, b Key) bool {
	return a.Ref == b.Ref && a.Values == b.Values
}

func diagnosticContextDominates(a, b Key) bool {
	if !diagnosticContextSameBase(a, b) {
		return false
	}
	if !diagnosticReferenceContextDominates(a.References.Context(), b.References.Context()) {
		return false
	}
	return flow.BoundaryFactsDomain.LessOrEq(a.Facts.Facts(), b.Facts.Facts())
}

func diagnosticReferenceContextDominates(a, b flow.ReferenceContext) bool {
	if flow.ReferenceContextDomain.LessOrEq(b, a) {
		return true
	}
	return diagnosticCaptureCellsDominate(a.CaptureCells(), b.CaptureCells()) &&
		diagnosticFunctionRefsDominate(a.FunctionRefs(), b.FunctionRefs()) &&
		diagnosticClosureRefsDominate(a.ClosureRefs(), b.ClosureRefs())
}

func diagnosticCaptureCellsDominate(a, b flow.CaptureCells) bool {
	if flow.CaptureCellsDomain.Equal(a, b) || flow.CaptureCellsDomain.LessOrEq(b, a) {
		return true
	}
	if a.IsTop() {
		return true
	}
	if b.IsTop() {
		return false
	}
	for _, weak := range b.Entries() {
		strong, ok := a.Value(weak.Symbol)
		if !ok || !diagnosticProductDominates(strong, weak.Value) {
			return false
		}
	}
	return true
}

func diagnosticProductDominates(strong, weak product.AbstractValue) bool {
	if product.Domain.Equal(strong, weak) || product.Domain.LessOrEq(weak, strong) {
		return true
	}
	if weak.Covers(strong) {
		return true
	}
	strongType := product.ProjectValueOrUnknown(strong)
	weakType := product.ProjectValueOrUnknown(weak)
	return typ.MorePrecise(strongType, weakType)
}

func diagnosticFunctionRefsDominate(a, b flow.FunctionRefs) bool {
	return len(b) == 0 || flow.FunctionRefsDomain.LessOrEq(b, a)
}

func diagnosticClosureRefsDominate(a, b flow.ClosureRefs) bool {
	return len(b) == 0 || flow.ClosureRefsDomain.LessOrEq(b, a)
}

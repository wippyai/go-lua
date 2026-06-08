package summary

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
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
	// SolveWithDependencies is the exact-observer solve plus the exact overlay
	// keys read while solving. When present, the frontier refreshes only observer
	// keys that depend on an overlay key whose projected exact summary changed.
	SolveWithDependencies func(Key) (state.FunctionState, []Key)
	// ProjectSummary projects each exact observer state into a caller-visible
	// summary overlay. Solve callbacks may read that overlay through a snapshot
	// Reader, letting the diagnostic frontier converge exact context summaries
	// without creating new recursive summary query cells.
	ProjectSummary func(Key, state.FunctionState) Summary
	SummaryOverlay map[Key]Summary

	ProjectCalls    func(FuncRef, state.FunctionState) []Key
	ProjectClosures func(FuncRef, state.FunctionState) []Key
}

// DiagnosticContextResult is the finite context set diagnostics should observe,
// plus the exact observer states already solved while discovering the frontier.
type DiagnosticContextResult struct {
	Contexts  map[FuncRef][]Key
	States    map[Key]state.FunctionState
	Summaries map[Key]Summary
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
	summaries := f.SummaryOverlay
	if summaries == nil {
		summaries = make(map[Key]Summary)
	}
	b := diagnosticContextBuilder{
		frontier: f,
		result: DiagnosticContextResult{
			Contexts:  make(map[FuncRef][]Key),
			States:    make(map[Key]state.FunctionState),
			Summaries: summaries,
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
	known      []Key

	overlayDeps  map[Key]map[Key]struct{}
	overlayUsers map[Key]map[Key]struct{}
	refreshed    map[Key]struct{}

	primaryRefs      map[FuncRef]bool
	closureFallbacks map[FuncRef][]Key
}

func (b *diagnosticContextBuilder) build() {
	b.seen = make(map[Key]struct{})
	b.overlayDeps = make(map[Key]map[Key]struct{})
	b.overlayUsers = make(map[Key]map[Key]struct{})
	b.refreshed = make(map[Key]struct{})
	b.discoverReachable()
	for {
		b.refine()
		before := cloneDiagnosticContextMap(b.result.Contexts)
		knownBefore := len(b.known)
		b.discoverReachable()
		if knownBefore == len(b.known) && diagnosticContextMapEqual(before, b.result.Contexts) {
			return
		}
	}
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
		b.updateSummary(key, fs)
		return fs
	}
	fs, deps := b.solveFresh(key)
	b.result.States[key] = fs
	b.setOverlayDependencies(key, deps)
	b.updateSummary(key, fs)
	return fs
}

func (b *diagnosticContextBuilder) solveFresh(key Key) (state.FunctionState, []Key) {
	if b.frontier.SolveWithDependencies != nil {
		fs, deps := b.frontier.SolveWithDependencies(key)
		return fs, deps
	}
	if b.frontier.Solve == nil {
		return state.FunctionStateDomain.Bottom(), nil
	}
	return b.frontier.Solve(key), nil
}

func (b *diagnosticContextBuilder) refreshSolve(key Key) (state.FunctionState, bool, bool) {
	next, deps := b.solveFresh(key)
	b.refreshed[key] = struct{}{}
	b.setOverlayDependencies(key, deps)
	summaryChanged := b.updateSummary(key, next)
	prev, ok := b.result.State(key)
	if ok && diagnosticObserverStateEqual(prev, next) && !summaryChanged {
		return prev, false, false
	}
	stateChanged := !ok || !diagnosticObserverStateEqual(prev, next)
	b.result.States[key] = next
	return next, stateChanged, summaryChanged
}

func (b *diagnosticContextBuilder) updateSummary(key Key, fs state.FunctionState) bool {
	if b.frontier.ProjectSummary == nil || b.result.Summaries == nil {
		return false
	}
	projected := b.frontier.ProjectSummary(key, fs)
	next := projected
	if prev, ok := b.result.Summaries[key]; ok {
		next = mergeExactOverlaySummary(prev, projected)
		if SummaryDomain.Equal(prev, next) {
			return false
		}
	}
	b.result.Summaries[key] = next
	return true
}

func (b *diagnosticContextBuilder) refine() {
	var work []Key
	inWork := make(map[Key]struct{})
	enqueue := func(key Key) {
		if _, ok := inWork[key]; ok {
			return
		}
		inWork[key] = struct{}{}
		work = append(work, key)
	}
	enqueueAll := func() {
		for _, key := range b.known {
			enqueue(key)
		}
	}
	enqueueRefreshFrontier := func() {
		if b.frontier.SolveWithDependencies == nil {
			enqueueAll()
			return
		}
		for _, key := range b.known {
			if _, ok := b.refreshed[key]; ok {
				continue
			}
			enqueue(key)
		}
	}
	// Every exact observer key gets one post-discovery refresh so derived
	// contexts can surface after the local solver observes converged summaries.
	// Later passes rely on recorded overlay dependencies instead of replaying
	// every context after unrelated exact summaries move.
	enqueueRefreshFrontier()
	for len(work) > 0 {
		key := work[0]
		work = work[1:]
		delete(inWork, key)
		fs, stateChanged, summaryChanged := b.refreshSolve(key)
		if !stateChanged && !summaryChanged {
			continue
		}
		contextChanged := false
		if stateChanged {
			contextChanged = b.projectRefinedContexts(key, fs, enqueue)
		}
		if summaryChanged {
			b.enqueueOverlayUsers(key, enqueue)
		}
		if b.frontier.SolveWithDependencies == nil && (contextChanged || summaryChanged) {
			enqueueAll()
		}
	}
}

func (b *diagnosticContextBuilder) setOverlayDependencies(user Key, deps []Key) bool {
	if b.overlayDeps == nil {
		b.overlayDeps = make(map[Key]map[Key]struct{})
	}
	if b.overlayUsers == nil {
		b.overlayUsers = make(map[Key]map[Key]struct{})
	}
	next := keySet(deps)
	prev := b.overlayDeps[user]
	if keySetEqual(prev, next) {
		return false
	}
	for dep := range prev {
		users := b.overlayUsers[dep]
		delete(users, user)
		if len(users) == 0 {
			delete(b.overlayUsers, dep)
		}
	}
	if len(next) == 0 {
		delete(b.overlayDeps, user)
		return true
	}
	b.overlayDeps[user] = next
	for dep := range next {
		users := b.overlayUsers[dep]
		if users == nil {
			users = make(map[Key]struct{})
			b.overlayUsers[dep] = users
		}
		users[user] = struct{}{}
	}
	return true
}

func (b *diagnosticContextBuilder) enqueueOverlayUsers(dep Key, enqueue func(Key)) {
	if b.frontier.SolveWithDependencies == nil {
		return
	}
	for user := range b.overlayUsers[dep] {
		enqueue(user)
	}
}

func (b *diagnosticContextBuilder) projectRefinedContexts(key Key, fs state.FunctionState, enqueue func(Key)) bool {
	changed := false
	for _, next := range b.projectCalls(key.Ref, fs) {
		if !b.valid(next) {
			continue
		}
		if b.promotePrimary(next.Ref) {
			changed = true
		}
		if b.addContext(next) {
			changed = true
		}
		if b.markSeen(next) {
			enqueue(next)
			changed = true
		}
	}
	for _, next := range b.projectClosures(key.Ref, fs) {
		if !b.valid(next) || b.primaryRefs[next.Ref] {
			continue
		}
		b.closureFallbacks[next.Ref] = append(b.closureFallbacks[next.Ref], next)
		if b.addContext(next) {
			changed = true
		}
		if b.markSeen(next) {
			enqueue(next)
			changed = true
		}
	}
	return changed
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
	b.known = append(b.known, key)
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
	return a.Ref == b.Ref &&
		a.References == b.References &&
		a.Values == b.Values
}

func diagnosticContextDominates(a, b Key) bool {
	if !diagnosticContextSameBase(a, b) {
		return false
	}
	return flow.BoundaryFactsDomain.LessOrEq(a.Facts.Facts(), b.Facts.Facts())
}

func diagnosticObserverStateEqual(a, b state.FunctionState) bool {
	return state.FunctionStateDomain.Equal(a, b) &&
		pointStateMapEqual(a.InPoints, b.InPoints)
}

func pointStateMapEqual(a, b map[cfg.Point]flow.PointState) bool {
	for point, av := range a {
		if !flow.PointStateDomain.Equal(av, b[point]) {
			return false
		}
	}
	bottom := flow.PointStateDomain.Bottom()
	for point, bv := range b {
		if _, ok := a[point]; ok {
			continue
		}
		if !flow.PointStateDomain.Equal(bottom, bv) {
			return false
		}
	}
	return true
}

func cloneDiagnosticContextMap(in map[FuncRef][]Key) map[FuncRef][]Key {
	if len(in) == 0 {
		return nil
	}
	out := make(map[FuncRef][]Key, len(in))
	for ref, keys := range in {
		out[ref] = append([]Key(nil), keys...)
	}
	return out
}

func diagnosticContextMapEqual(a, b map[FuncRef][]Key) bool {
	if len(a) != len(b) {
		return false
	}
	for ref, ak := range a {
		if !slices.Equal(ak, b[ref]) {
			return false
		}
	}
	for ref, bk := range b {
		if _, ok := a[ref]; !ok && len(bk) != 0 {
			return false
		}
	}
	return true
}

func keySet(keys []Key) map[Key]struct{} {
	if len(keys) == 0 {
		return nil
	}
	out := make(map[Key]struct{}, len(keys))
	for _, key := range keys {
		out[key] = struct{}{}
	}
	return out
}

func keySetEqual(a, b map[Key]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if _, ok := b[key]; !ok {
			return false
		}
	}
	return true
}

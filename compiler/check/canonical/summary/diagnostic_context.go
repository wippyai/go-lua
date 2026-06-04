package summary

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
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
}

func (b *diagnosticContextBuilder) build() {
	root := b.frontier.Root
	rootKey := b.defaultKey(root)
	if !b.valid(rootKey) {
		return
	}
	seen := map[Key]struct{}{rootKey: {}}
	work := []Key{rootKey}
	b.addContext(rootKey)
	primaryRefs := map[FuncRef]bool{root: true}
	closureFallbacks := make(map[FuncRef][]Key)

	for len(work) > 0 {
		key := work[0]
		work = work[1:]
		fs := b.solve(key)
		for _, next := range b.projectClosures(key.Ref, fs) {
			if !b.valid(next) {
				continue
			}
			closureFallbacks[next.Ref] = append(closureFallbacks[next.Ref], next)
		}
		for _, next := range b.projectCalls(key.Ref, fs) {
			if !b.valid(next) {
				continue
			}
			primaryRefs[next.Ref] = true
			b.addContext(next)
			if _, ok := seen[next]; ok {
				continue
			}
			seen[next] = struct{}{}
			work = append(work, next)
		}
	}

	var fallbackWork []Key
	for _, ref := range b.frontier.Refs {
		if ref == root || primaryRefs[ref] || len(b.result.Contexts[ref]) != 0 {
			continue
		}
		for _, key := range closureFallbacks[ref] {
			b.addContext(key)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			fallbackWork = append(fallbackWork, key)
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
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		fallbackWork = append(fallbackWork, key)
	}
	for len(fallbackWork) > 0 {
		key := fallbackWork[0]
		fallbackWork = fallbackWork[1:]
		fs := b.solve(key)
		for _, next := range b.projectContexts(key.Ref, fs) {
			if !b.valid(next) || primaryRefs[next.Ref] {
				continue
			}
			b.addContext(next)
			if _, ok := seen[next]; ok {
				continue
			}
			seen[next] = struct{}{}
			fallbackWork = append(fallbackWork, next)
		}
	}
}

func (b *diagnosticContextBuilder) solve(key Key) state.FunctionState {
	if fs, ok := b.result.State(key); ok {
		return fs
	}
	if b.frontier.Solve == nil {
		fs := state.FunctionStateDomain.Bottom()
		b.result.States[key] = fs
		return fs
	}
	fs := b.frontier.Solve(key)
	b.result.States[key] = fs
	return fs
}

func (b *diagnosticContextBuilder) defaultKey(ref FuncRef) Key {
	if b.frontier.DefaultKey != nil {
		return b.frontier.DefaultKey(ref)
	}
	return NewKeyWithEntryContext(ref, flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom(), nil)
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

func (b *diagnosticContextBuilder) addContext(key Key) {
	ref := key.Ref
	seen := b.contextSet[ref]
	if seen == nil {
		seen = make(map[Key]struct{})
		b.contextSet[ref] = seen
	}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	b.result.Contexts[ref] = append(b.result.Contexts[ref], key)
}

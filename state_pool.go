package lua

import (
	"sync"
	"sync/atomic"
)

// statePool holds reusable LState objects for better performance
var statePool = sync.Pool{
	New: func() interface{} {
		return nil // We'll create states with specific options when needed
	},
}

// resetLState prepares a state for reuse by clearing its values but keeping the allocated structures
func resetLState(ls *LState) {
	// Clear registry but keep the underlying array
	if ls.reg != nil {
		ls.reg.top = 0
		// Explicitly nil out values to help with GC
		for i := range ls.reg.array {
			ls.reg.array[i] = LNil
		}
	}

	// Reset upvalue cache
	ls.uvcache = nil

	// Reset call frames
	if ls.stack != nil {
		// Different stack implementations handled in FreeAll()
		ls.stack.SetSp(0)
	}

	// Reset state properties
	ls.currentFrame = nil
	ls.Parent = nil
	ls.Dead = false
	ls.stop = 0
	ls.Env = nil
	ls.G = nil
	ls.hasErrorFunc = false
	ls.wrapped = false

	// Keep allocator but release its current slices
	if ls.alloc != nil {
		ls.alloc.Release()
	}
}

// Modified Close to return state to pool if appropriate
func (ls *LState) Close() {
	atomic.AddInt32(&ls.stop, 1)

	// Don't pool if registry has grown beyond initial size
	shouldPool := ls.reg != nil && cap(ls.reg.array) <= ls.Options.RegistrySize+ls.Options.RegistryGrowStep

	if shouldPool {
		resetLState(ls)
		statePool.Put(ls)
	} else {
		// Original close behavior for states we don't want to reuse
		ls.alloc.Release()
		ls.stack.FreeAll()
		ls.stack = nil
		ls.reg = nil
	}
}

// Replacement for newLStateWithG that uses pooled states when available
func newLStateWithG(options Options, G *Global, env *LTable) *LState {
	// Try to get a state from the pool
	pooledState := statePool.Get()

	if ls, ok := pooledState.(*LState); ok && ls != nil {
		// We got a pooled state, configure it for reuse
		ls.G = G
		ls.Env = env
		ls.Panic = panicWithTraceback
		ls.Options = options
		ls.mainLoop = mainLoop

		// Registry was preserved but might need resetting if options changed
		if ls.reg != nil && cap(ls.reg.array) != options.RegistrySize {
			// Registry size mismatch, create a new one
			ls.reg = newRegistry(ls, options.RegistrySize, options.RegistryGrowStep, options.RegistryMaxSize, ls.alloc)
		} else if ls.reg != nil {
			// Registry size is ok, just reset the handler
			ls.reg.handler = ls
		}

		// Return the recycled state
		return ls
	}

	// No suitable pooled state available, create a new one
	al := newAllocator(32)
	ls := &LState{
		G:            G,
		Parent:       nil,
		Panic:        panicWithTraceback,
		Dead:         false,
		Options:      options,
		stop:         0,
		alloc:        al,
		currentFrame: nil,
		wrapped:      false,
		uvcache:      nil,
		hasErrorFunc: false,
		mainLoop:     mainLoop,
		ctx:          nil,
	}

	// Create stack based on options
	if options.MinimizeStackMemory {
		ls.stack = newAutoGrowingCallFrameStack(options.CallStackSize)
	} else {
		ls.stack = newFixedCallFrameStack(options.CallStackSize)
	}

	// Create registry
	ls.reg = newRegistry(ls, options.RegistrySize, options.RegistryGrowStep, options.RegistryMaxSize, al)
	ls.Env = env

	return ls
}

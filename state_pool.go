package lua

import (
	"sync"
	"sync/atomic"
)

// statePool holds reusable LState objects for better performance
var statePool = sync.Pool{
	New: func() any {
		return nil // We'll create states with specific options when needed
	},
}

// resetLState prepares a state for reuse by clearing its values but keeping the allocated structures
func resetLState(ls *LState) {
	// Clear registry but keep the underlying Array
	if ls.reg != nil {
		// Only clear up to top, not the entire array
		top := ls.reg.top
		ls.reg.top = 0
		for i := 0; i < top; i++ {
			ls.reg.array[i] = LNil
		}
	}

	// Reset upvalue cache
	ls.uvcache = nil

	// Reset call frames
	if ls.stack != nil {
		ls.stack.Reset()
	}

	// Reset state properties
	ls.currentFrame = nil
	ls.Parent = nil
	ls.Dead = false
	// Note: stop is NOT reset here - it stays set so IsClosed() returns true
	// stop is reset when the state is retrieved from the pool for reuse
	ls.Env = nil
	ls.G = nil
	ls.hasErrorFunc = false
	ls.wrapped = false
	ls.yieldState = yieldNone
	ls.yieldCallRB = 0
	ls.releaseHold()
	ls.releaseHeld()
	ls.ctx = nil
	ls.ctxDone = nil
	ls.ctxCancelFn = nil

	// Frame extensions hold continuations and handlers keyed by frame index.
	ls.frameExt = nil
}

// Close returns the state to pool if appropriate.
func (ls *LState) Close() {
	atomic.AddInt32(&ls.stop, 1)
	ls.releaseHold()
	ls.releaseHeld()
	// Closures that escaped keep their captured values; the registers return to
	// the pool and are overwritten by the next user.
	ls.closeUpvalues(0)

	// Don't pool if registry has grown beyond initial size
	shouldPool := ls.reg != nil && cap(ls.reg.array) <= ls.Options.RegistrySize+ls.Options.RegistryGrowStep

	if shouldPool {
		resetLState(ls)
		statePool.Put(ls)
	} else {
		ls.stack.FreeAll()
		ls.stack = nil
		ls.reg = nil
	}
}

// newLStateWithGlobal creates a thread that shares the parent's global/env.
func newLStateWithGlobal(options Options, G *Global, env *LTable) *LState {
	// Try to get a state from the pool
	pooledState := statePool.Get()

	if ls, ok := pooledState.(*LState); ok && ls != nil {
		// We got a pooled state, configure it for reuse
		ls.G = G
		ls.Env = env
		ls.Panic = panicWithTraceback
		ls.Options = options
		ls.mainLoop = mainLoop
		ls.stop = 0
		ls.yieldState = yieldNone
		ls.ctx = nil
		ls.ctxDone = nil

		ls.reg = registryFor(ls, options)
		ls.stack = callStackFor(ls.stack, options)

		return ls
	}

	// No suitable pooled state available, create a new one
	ls := &LState{
		G:            G,
		Parent:       nil,
		Panic:        panicWithTraceback,
		Dead:         false,
		Options:      options,
		stop:         0,
		currentFrame: nil,
		wrapped:      false,
		uvcache:      nil,
		hasErrorFunc: false,
		mainLoop:     mainLoop,
		ctx:          nil,
	}

	ls.stack = newCallStack(options)
	ls.reg = newRegistry(ls, options.RegistrySize, options.RegistryGrowStep, options.RegistryMaxSize)
	ls.Env = env

	return ls
}

// newCallStack creates the call stack options describe.
func newCallStack(options Options) callFrameStack {
	if options.MinimizeStackMemory {
		return newAutoGrowingCallFrameStack(options.CallStackSize)
	}
	return newFixedCallFrameStack(options.CallStackSize)
}

// callStackFor returns an empty call stack for options, reusing a pooled one
// when its kind and capacity match.
func callStackFor(pooled callFrameStack, options Options) callFrameStack {
	switch cs := pooled.(type) {
	case *fixedCallFrameStack:
		if !options.MinimizeStackMemory && len(cs.array) == options.CallStackSize {
			return cs
		}
	case *autoGrowingCallFrameStack:
		if options.MinimizeStackMemory && len(cs.segments) == autoSegmentCount(options.CallStackSize) {
			return cs
		}
	}
	if pooled != nil {
		pooled.FreeAll()
	}
	return newCallStack(options)
}

// registryFor returns an empty registry for ls with the sizes options
// describe, reusing a pooled one that is large enough.
func registryFor(ls *LState, options Options) *registry {
	rg := ls.reg
	if rg == nil || cap(rg.array) < options.RegistrySize {
		return newRegistry(ls, options.RegistrySize, options.RegistryGrowStep, options.RegistryMaxSize)
	}
	rg.handler = ls
	rg.top = 0
	rg.maxSize = options.RegistryMaxSize
	rg.growBy = options.RegistryGrowStep
	return rg
}

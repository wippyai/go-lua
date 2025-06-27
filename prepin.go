package lua

import "sync"

var (
	// sharedPinningState is a private global LState used for creating reusable functions
	// This state is never closed and is used purely for function creation during runtime
	sharedPinningState *LState
	initPinningOnce    sync.Once
)

// getSharedPinningState returns the singleton LState used for function pinning
// This state is created once and reused for creating all shared functions
// CRITICAL: This must never open any libraries to avoid initialization cycles
func getSharedPinningState() *LState {
	initPinningOnce.Do(func() {
		// Create minimal state with NO libraries - absolutely critical for avoiding cycles
		sharedPinningState = newLState(Options{
			RegistrySize:        64, // Small registry since we only create functions
			CallStackSize:       32, // Small stack since we never call anything
			MinimizeStackMemory: true,
			SkipOpenLibs:        true, // NEVER open any libraries
		})
		// Never close this state - it's meant to live for the entire program
	})
	return sharedPinningState
}

package engine

import "sync"

// coldGate serializes one disposable declaration capability. It exists only
// while Program topology is assembled; no gate or lock survives compilation
// into Solver runtime state.
//
// An operation that overlaps another operation is rejected and records a
// declaration failure. Closing while an operation remains in flight records
// the same failure, waits for that operation to leave, and then permanently
// rejects retained uses. A use that begins after close is harmless: it returns
// false without changing the already-closed result.
type coldGate struct {
	mu      sync.Mutex
	changed *sync.Cond
	busy    bool
	closing bool
	closed  bool
	overlap bool
}

func newColdGate() *coldGate {
	gate := &coldGate{}
	gate.changed = sync.NewCond(&gate.mu)
	return gate
}

func (gate *coldGate) begin() bool {
	if gate == nil {
		return false
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.closing || gate.closed {
		return false
	}
	if gate.busy {
		gate.overlap = true
		return false
	}
	gate.busy = true
	return true
}

func (gate *coldGate) end() {
	if gate == nil {
		return
	}
	gate.mu.Lock()
	if gate.busy {
		gate.busy = false
		gate.changed.Broadcast()
	}
	gate.mu.Unlock()
}

func (gate *coldGate) close() bool {
	if gate == nil {
		return false
	}
	gate.mu.Lock()
	if gate.closed {
		accepted := !gate.overlap
		gate.mu.Unlock()
		return accepted
	}
	if gate.busy {
		gate.overlap = true
	}
	gate.closing = true
	gate.changed.Broadcast()
	for gate.busy {
		gate.changed.Wait()
	}
	gate.closed = true
	accepted := !gate.overlap
	gate.changed.Broadcast()
	gate.mu.Unlock()
	return accepted
}

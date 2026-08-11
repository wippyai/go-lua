package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/observation"

// actionQueue is the transaction's sole dynamic-work frontier.  Schedule
// fixes execution order; this queue only remembers exact observations made
// dirty while an action is running.  Its finite range is the already-compiled
// action set, never a convergence budget or semantic capacity.
type actionQueue struct {
	present []uint64
	items   []observation.Equation
	head    int
	tail    int
	pending int
	live    bool
}

func newActionQueue(actions int) *actionQueue {
	if actions <= 0 {
		return nil
	}
	return &actionQueue{
		present: make([]uint64, (actions+63)/64),
		items:   make([]observation.Equation, actions),
		live:    true,
	}
}

// Seed records one exact action once until it is drained.  Observation
// dispatches equations in canonical order; FIFO preserves that order across
// separate deltas while allowing a later change to re-enqueue a drained item.
func (queue *actionQueue) Seed(equation observation.Equation) bool {
	if queue == nil || !queue.live || uint64(equation) >= uint64(len(queue.items)) {
		return false
	}
	word, bit := uint64(equation)/64, uint64(1)<<(uint64(equation)%64)
	if queue.present[word]&bit != 0 {
		return true
	}
	if queue.pending == len(queue.items) {
		return false
	}
	queue.present[word] |= bit
	queue.items[queue.tail] = equation
	queue.tail = (queue.tail + 1) % len(queue.items)
	queue.pending++
	return true
}

func (queue *actionQueue) Next() (observation.Equation, bool) {
	if queue == nil || !queue.live || queue.pending == 0 {
		return 0, false
	}
	equation := queue.items[queue.head]
	queue.head = (queue.head + 1) % len(queue.items)
	queue.pending--
	word, bit := uint64(equation)/64, uint64(1)<<(uint64(equation)%64)
	queue.present[word] &^= bit
	return equation, true
}

func (queue *actionQueue) Discard() {
	if queue == nil || !queue.live {
		return
	}
	clear(queue.present)
	clear(queue.items)
	queue.head, queue.tail, queue.pending = 0, 0, 0
	queue.live = false
}

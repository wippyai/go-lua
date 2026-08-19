package guard

// Decomposition is one read-only view of a Guard. A terminal has Terminal
// set and Value reports its exact false/true value. A decision has its exact
// presealed Atom and its false/true Low/High successors. It deliberately
// exposes neither BDD pages nor nodes.
type Decomposition struct {
	Atom     Atom
	Low      Guard
	High     Guard
	Terminal bool
	Value    bool
}

// Decompose returns the exact immutable decision represented by g. It accepts
// only a sealed Guard owned by m.
func (m *Manager) Decompose(g Guard) (Decomposition, bool) {
	if !m.validSealed(g) {
		return Decomposition{}, false
	}
	return m.decompose(g), true
}

// Decompose returns the exact immutable decision represented by g. In an open
// candidate, g may be either a sealed predecessor or a Guard owned by this
// Work. The operation is read-only and does not alter candidate construction
// state.
func (w *Work) Decompose(g Guard) (Decomposition, bool) {
	if !w.Valid(g) {
		return Decomposition{}, false
	}
	return w.decompose(g), true
}

func (m *Manager) decompose(g Guard) Decomposition {
	if isTerminal(g) {
		return Decomposition{Terminal: true, Value: terminalValue(g)}
	}
	n := m.node(g)
	return Decomposition{Atom: m.atom(n.rank), Low: n.low, High: n.high}
}

func (w *Work) decompose(g Guard) Decomposition {
	if isTerminal(g) {
		return Decomposition{Terminal: true, Value: terminalValue(g)}
	}
	n := w.node(g)
	return Decomposition{Atom: w.manager.atom(n.rank), Low: n.low, High: n.high}
}

// Fold visits every Guard reachable from root exactly once in deterministic
// postorder: a decision's low branch, then its high branch, then the decision
// itself. Thus every successor has already been visited when visit sees its
// parent. Atom values remain the Manager's exact canonical order; every
// nonterminal successor has a strictly later atom on its path.
//
// Fold is read-only. It transports Guard structure only, never a Factor value
// or any other semantic payload. Returning false from visit stops the fold;
// completed then reports false while valid remains true.
func (m *Manager) Fold(root Guard, visit func(Guard, Decomposition) bool) (completed, valid bool) {
	if visit == nil {
		return false, false
	}
	return fold(root, m.validSealed, m.decompose, visit)
}

// Fold is Work's candidate counterpart to Manager.Fold. It accepts sealed
// predecessors and this Work's own open Guards, but rejects foreign or
// unsealed candidate pages.
func (w *Work) Fold(root Guard, visit func(Guard, Decomposition) bool) (completed, valid bool) {
	if visit == nil {
		return false, false
	}
	// Work is single-writer, but a callback may re-enter Fold on this same
	// Work. Keep the outer traversal's reusable scratch intact and use the
	// local implementation for the nested call, preserving the old callback
	// reentrancy behavior.
	if w == nil || w.foldBusy {
		return fold(root, w.Valid, w.decompose, visit)
	}
	w.clearFoldScratch()
	w.foldBusy = true
	defer w.clearFoldScratch()
	return w.fold(root, visit)
}

type foldFrame struct {
	guard Guard
	view  Decomposition
	ready bool
}

type foldItem struct {
	guard Guard
	view  Decomposition
}

// clearFoldScratch removes every Guard reference from Work-owned fold
// storage while retaining capacities for the next warmed traversal. The
// backing arrays are reused, so truncating without zeroing would retain every
// published page reached by the previous fold.
func (w *Work) clearFoldScratch() {
	if w == nil {
		return
	}
	for index := range w.foldStack {
		w.foldStack[index] = foldFrame{}
	}
	w.foldStack = w.foldStack[:0]
	for index := range w.foldItems {
		w.foldItems[index] = foldItem{}
	}
	w.foldItems = w.foldItems[:0]
	clear(w.foldSeen)
	w.foldBusy = false
}

// fold is Work's reusable-scratch traversal. It validates the complete DAG
// before invoking any callback, matching the local Manager.Fold traversal's
// no-partial-callback contract.
func (w *Work) fold(root Guard, visit func(Guard, Decomposition) bool) (completed, ok bool) {
	if !w.Valid(root) {
		return false, false
	}
	// A terminal has no reachable children. Keep the common one-node fold on
	// the direct path; the reusable traversal state is only needed for a DAG.
	if isTerminal(root) {
		return visit(root, w.decompose(root)), true
	}
	if w.foldSeen == nil {
		w.foldSeen = make(map[nodeKey]struct{})
	}
	w.foldStack = append(w.foldStack, foldFrame{guard: root})
	for len(w.foldStack) != 0 {
		last := len(w.foldStack) - 1
		frame := w.foldStack[last]
		w.foldStack[last] = foldFrame{}
		w.foldStack = w.foldStack[:last]
		if frame.ready {
			w.foldItems = append(w.foldItems, foldItem{guard: frame.guard, view: frame.view})
			continue
		}
		if _, found := w.foldSeen[keyOf(frame.guard)]; found {
			continue
		}
		if !w.Valid(frame.guard) {
			return false, false
		}
		w.foldSeen[keyOf(frame.guard)] = struct{}{}
		view := w.decompose(frame.guard)
		w.foldStack = append(w.foldStack, foldFrame{guard: frame.guard, view: view, ready: true})
		if view.Terminal {
			continue
		}
		// LIFO makes the canonical low branch complete before high.
		w.foldStack = append(w.foldStack, foldFrame{guard: view.High}, foldFrame{guard: view.Low})
	}
	for _, item := range w.foldItems {
		if !visit(item.guard, item.view) {
			return false, true
		}
	}
	return true, true
}

// fold validates and records the whole reachable DAG before invoking visit,
// so an invalid Guard never exposes a partial traversal. The explicit stack
// keeps Guard depth off the Go call stack.
func fold(root Guard, valid func(Guard) bool, decompose func(Guard) Decomposition, visit func(Guard, Decomposition) bool) (completed, ok bool) {
	if !valid(root) {
		return false, false
	}
	// A terminal has no reachable children. Keep the common one-node fold on
	// the direct path: the full DAG walk below needs its seen set, explicit
	// stack, and postorder item list, none of which can contribute for a
	// terminal. The callback result keeps the public completion contract exact.
	if isTerminal(root) {
		return visit(root, decompose(root)), true
	}
	seen := make(map[nodeKey]struct{})
	stack := []foldFrame{{guard: root}}
	items := make([]foldItem, 0)
	for len(stack) != 0 {
		last := len(stack) - 1
		frame := stack[last]
		stack = stack[:last]
		if frame.ready {
			items = append(items, foldItem{guard: frame.guard, view: frame.view})
			continue
		}
		if _, found := seen[keyOf(frame.guard)]; found {
			continue
		}
		if !valid(frame.guard) {
			return false, false
		}
		seen[keyOf(frame.guard)] = struct{}{}
		view := decompose(frame.guard)
		stack = append(stack, foldFrame{guard: frame.guard, view: view, ready: true})
		if view.Terminal {
			continue
		}
		// LIFO makes the canonical low branch complete before high.
		stack = append(stack, foldFrame{guard: view.High}, foldFrame{guard: view.Low})
	}
	for _, item := range items {
		if !visit(item.guard, item.view) {
			return false, true
		}
	}
	return true, true
}

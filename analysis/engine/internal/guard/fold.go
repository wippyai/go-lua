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
	return fold(root, w.Valid, w.decompose, visit)
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

// fold validates and records the whole reachable DAG before invoking visit,
// so an invalid Guard never exposes a partial traversal. The explicit stack
// keeps Guard depth off the Go call stack.
func fold(root Guard, valid func(Guard) bool, decompose func(Guard) Decomposition, visit func(Guard, Decomposition) bool) (completed, ok bool) {
	if !valid(root) {
		return false, false
	}
	seen := make(map[Guard]struct{})
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
		if _, found := seen[frame.guard]; found {
			continue
		}
		if !valid(frame.guard) {
			return false, false
		}
		seen[frame.guard] = struct{}{}
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

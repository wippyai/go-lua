package guard

// Restrict fixes atom to value in g.
func (w *Work) Restrict(g Guard, atom Atom, value bool) Guard {
	if !w.Live() {
		return Guard{}
	}
	w.requireOpen()
	w.require(g)
	rank, exists := w.manager.atoms[atom]
	if !exists {
		return g
	}
	return w.restrictNode(g, rank, value)
}

func (w *Work) restrictNode(root Guard, target uint64, value bool) Guard {
	if result, done := w.restrictResult(root, target, value); done {
		return result
	}
	if w.restrict == nil {
		w.restrict = make(map[restrictKey]Guard)
	}
	stack := []unaryFrame{{guard: root}}
	for len(stack) != 0 {
		if !w.Live() {
			return Guard{}
		}
		frame := &stack[len(stack)-1]
		if _, done := w.restrictResult(frame.guard, target, value); done {
			stack = stack[:len(stack)-1]
			continue
		}
		n := w.node(frame.guard)
		if n.rank == target {
			key := restrictKey{guard: frame.guard, rank: target, value: value}
			if value {
				w.restrict[key] = n.high
			} else {
				w.restrict[key] = n.low
			}
			stack = stack[:len(stack)-1]
			continue
		}
		switch frame.phase {
		case 0:
			frame.phase = 1
			if _, done := w.restrictResult(n.low, target, value); !done {
				stack = append(stack, unaryFrame{guard: n.low})
			}
		case 1:
			frame.phase = 2
			if _, done := w.restrictResult(n.high, target, value); !done {
				stack = append(stack, unaryFrame{guard: n.high})
			}
		default:
			low, _ := w.restrictResult(n.low, target, value)
			high, _ := w.restrictResult(n.high, target, value)
			w.restrict[restrictKey{guard: frame.guard, rank: target, value: value}] = w.makeNode(n.rank, low, high)
			stack = stack[:len(stack)-1]
		}
	}
	return w.restrict[restrictKey{guard: root, rank: target, value: value}]
}

func (w *Work) restrictResult(g Guard, target uint64, value bool) (Guard, bool) {
	if isTerminal(g) || w.rank(g) > target {
		return g, true
	}
	result, exists := w.restrict[restrictKey{guard: g, rank: target, value: value}]
	return result, exists
}

// Exists existentially discharges atom from g without enumerating cases.
func (w *Work) Exists(g Guard, atom Atom) Guard {
	if !w.Live() {
		return Guard{}
	}
	w.requireOpen()
	w.require(g)
	rank, exists := w.manager.atoms[atom]
	if !exists {
		return g
	}
	return w.existsNode(g, rank)
}

func (w *Work) existsNode(root Guard, target uint64) Guard {
	if result, done := w.existsResult(root, target); done {
		return result
	}
	if w.exists == nil {
		w.exists = make(map[existsKey]Guard)
	}
	stack := []unaryFrame{{guard: root}}
	for len(stack) != 0 {
		if !w.Live() {
			return Guard{}
		}
		frame := &stack[len(stack)-1]
		if _, done := w.existsResult(frame.guard, target); done {
			stack = stack[:len(stack)-1]
			continue
		}
		n := w.node(frame.guard)
		if n.rank == target {
			w.exists[existsKey{guard: frame.guard, rank: target}] = w.applyNode(orOperation, n.low, n.high)
			stack = stack[:len(stack)-1]
			continue
		}
		switch frame.phase {
		case 0:
			frame.phase = 1
			if _, done := w.existsResult(n.low, target); !done {
				stack = append(stack, unaryFrame{guard: n.low})
			}
		case 1:
			frame.phase = 2
			if _, done := w.existsResult(n.high, target); !done {
				stack = append(stack, unaryFrame{guard: n.high})
			}
		default:
			low, _ := w.existsResult(n.low, target)
			high, _ := w.existsResult(n.high, target)
			w.exists[existsKey{guard: frame.guard, rank: target}] = w.makeNode(n.rank, low, high)
			stack = stack[:len(stack)-1]
		}
	}
	return w.exists[existsKey{guard: root, rank: target}]
}

func (w *Work) existsResult(g Guard, target uint64) (Guard, bool) {
	if isTerminal(g) || w.rank(g) > target {
		return g, true
	}
	result, exists := w.exists[existsKey{guard: g, rank: target}]
	return result, exists
}

func resolvedGuard(g Guard, resolved map[Guard]Guard) (Guard, bool) {
	if isTerminal(g) {
		return g, true
	}
	result, exists := resolved[g]
	return result, exists
}

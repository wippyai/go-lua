package guard

// Not returns the exact complement of g in this candidate.
func (w *Work) Not(g Guard) Guard {
	if !w.Live() {
		return Guard{}
	}
	w.requireOpen()
	w.require(g)
	return w.notNode(g)
}

type unaryFrame struct {
	guard Guard
	phase uint8
}

func (w *Work) notNode(root Guard) Guard {
	if result, done := w.notResult(root, nil); done {
		return result
	}
	if w.not == nil {
		w.not = make(map[Guard]Guard)
	}
	resolved := make(map[Guard]Guard)
	stack := []unaryFrame{{guard: root}}
	for len(stack) != 0 {
		if !w.Live() {
			return Guard{}
		}
		frame := &stack[len(stack)-1]
		if _, done := w.notResult(frame.guard, resolved); done {
			stack = stack[:len(stack)-1]
			continue
		}
		n := w.node(frame.guard)
		switch frame.phase {
		case 0:
			frame.phase = 1
			if _, done := w.notResult(n.low, resolved); !done {
				stack = append(stack, unaryFrame{guard: n.low})
			}
		case 1:
			frame.phase = 2
			if _, done := w.notResult(n.high, resolved); !done {
				stack = append(stack, unaryFrame{guard: n.high})
			}
		default:
			low, _ := w.notResult(n.low, resolved)
			high, _ := w.notResult(n.high, resolved)
			result := w.makeNode(n.rank, low, high)
			w.not[frame.guard] = result
			w.not[result] = frame.guard
			resolved[frame.guard] = result
			stack = stack[:len(stack)-1]
		}
	}
	return resolved[root]
}

func (w *Work) notResult(g Guard, resolved map[Guard]Guard) (Guard, bool) {
	if isTerminal(g) {
		if terminalValue(g) {
			return w.manager.False(), true
		}
		return w.manager.True(), true
	}
	if resolved != nil {
		if result, exists := resolved[g]; exists {
			return result, true
		}
	}
	result, exists := w.not[g]
	return result, exists
}

// And and Or construct exact candidate-local conjunction/disjunction.
func (w *Work) And(left, right Guard) Guard {
	if !w.Live() {
		return Guard{}
	}
	return w.apply(andOperation, left, right)
}
func (w *Work) Or(left, right Guard) Guard {
	if !w.Live() {
		return Guard{}
	}
	return w.apply(orOperation, left, right)
}

type applyFrame struct {
	operation           operation
	left, right         Guard
	lowLeft, lowRight   Guard
	highLeft, highRight Guard
	rank                uint64
	phase               uint8
}

func (w *Work) apply(operation operation, left, right Guard) Guard {
	w.requireOpen()
	w.require(left)
	w.require(right)
	return w.applyNode(operation, left, right)
}

func (w *Work) applyNode(operation operation, left, right Guard) Guard {
	if !w.Live() {
		return Guard{}
	}
	if result, done := w.applyResult(operation, left, right, nil); done {
		return result
	}
	if w.applyCache == nil {
		w.applyCache = make(map[applyKey]Guard)
	}
	resolved := make(map[applyKey]Guard)
	stack := []applyFrame{{operation: operation, left: left, right: right}}
	for len(stack) != 0 {
		if !w.Live() {
			return Guard{}
		}
		frame := &stack[len(stack)-1]
		if _, done := w.applyResult(frame.operation, frame.left, frame.right, resolved); done {
			stack = stack[:len(stack)-1]
			continue
		}
		switch frame.phase {
		case 0:
			frame.rank, frame.lowLeft, frame.lowRight, frame.highLeft, frame.highRight = w.applyChildren(frame.left, frame.right)
			frame.phase = 1
			if _, done := w.applyResult(frame.operation, frame.lowLeft, frame.lowRight, resolved); !done {
				stack = append(stack, applyFrame{operation: frame.operation, left: frame.lowLeft, right: frame.lowRight})
			}
		case 1:
			frame.phase = 2
			if _, done := w.applyResult(frame.operation, frame.highLeft, frame.highRight, resolved); !done {
				stack = append(stack, applyFrame{operation: frame.operation, left: frame.highLeft, right: frame.highRight})
			}
		default:
			low, _ := w.applyResult(frame.operation, frame.lowLeft, frame.lowRight, resolved)
			high, _ := w.applyResult(frame.operation, frame.highLeft, frame.highRight, resolved)
			key := applyKey{operation: frame.operation, left: frame.left, right: frame.right}
			result := w.makeNode(frame.rank, low, high)
			w.applyCache[key] = result
			resolved[key] = result
			stack = stack[:len(stack)-1]
		}
	}
	return resolved[applyKey{operation: operation, left: left, right: right}]
}

func (w *Work) applyResult(operation operation, left, right Guard, resolved map[applyKey]Guard) (Guard, bool) {
	if result, done := terminalApply(w.manager, operation, left, right); done {
		return result, true
	}
	key := applyKey{operation: operation, left: left, right: right}
	if resolved != nil {
		if result, exists := resolved[key]; exists {
			return result, true
		}
	}
	result, exists := w.applyCache[key]
	return result, exists
}

func terminalApply(manager *Manager, operation operation, left, right Guard) (Guard, bool) {
	switch operation {
	case andOperation:
		switch {
		case isTerminal(left) && !terminalValue(left), isTerminal(right) && !terminalValue(right):
			return manager.False(), true
		case isTerminal(left) && terminalValue(left):
			return right, true
		case isTerminal(right) && terminalValue(right):
			return left, true
		case left == right:
			return left, true
		}
	case orOperation:
		switch {
		case isTerminal(left) && terminalValue(left), isTerminal(right) && terminalValue(right):
			return manager.True(), true
		case isTerminal(left) && !terminalValue(left):
			return right, true
		case isTerminal(right) && !terminalValue(right):
			return left, true
		case left == right:
			return left, true
		}
	}
	return Guard{}, false
}

func (w *Work) applyChildren(left, right Guard) (uint64, Guard, Guard, Guard, Guard) {
	leftRank, rightRank := w.rank(left), w.rank(right)
	switch {
	case leftRank < rightRank:
		n := w.node(left)
		return leftRank, n.low, right, n.high, right
	case rightRank < leftRank:
		n := w.node(right)
		return rightRank, left, n.low, left, n.high
	default:
		leftNode, rightNode := w.node(left), w.node(right)
		return leftRank, leftNode.low, rightNode.low, leftNode.high, rightNode.high
	}
}

func (w *Work) cofactor(g Guard, rank uint64) (Guard, Guard) {
	if !isTerminal(g) && w.rank(g) == rank {
		n := w.node(g)
		return n.low, n.high
	}
	return g, g
}

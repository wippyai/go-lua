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
	if result, done := w.notResult(root); done {
		return result
	}
	if w.not == nil {
		w.not = make(map[nodeKey]Guard)
	}
	w.notStack = append(w.notStack[:0], unaryFrame{guard: root})
	for len(w.notStack) != 0 {
		if !w.Live() {
			return Guard{}
		}
		frame := &w.notStack[len(w.notStack)-1]
		if _, done := w.notResult(frame.guard); done {
			w.notStack = w.notStack[:len(w.notStack)-1]
			continue
		}
		n := w.node(frame.guard)
		switch frame.phase {
		case 0:
			frame.phase = 1
			if _, done := w.notResult(n.low); !done {
				w.notStack = append(w.notStack, unaryFrame{guard: n.low})
			}
		case 1:
			frame.phase = 2
			if _, done := w.notResult(n.high); !done {
				w.notStack = append(w.notStack, unaryFrame{guard: n.high})
			}
		default:
			low, _ := w.notResult(n.low)
			high, _ := w.notResult(n.high)
			result := w.makeNode(n.rank, low, high)
			w.not[keyOf(frame.guard)] = result
			// notResult handles terminals directly, so retaining a terminal
			// reverse entry only adds a large key with no lookup benefit.
			if !isTerminal(result) {
				w.not[keyOf(result)] = frame.guard
			}
			w.notStack = w.notStack[:len(w.notStack)-1]
		}
	}
	return w.not[keyOf(root)]
}

func (w *Work) notResult(g Guard) (Guard, bool) {
	if isTerminal(g) {
		if terminalValue(g) {
			return w.manager.False(), true
		}
		return w.manager.True(), true
	}
	result, exists := w.not[keyOf(g)]
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
	if result, done := w.applyResult(operation, left, right); done {
		return result
	}
	if w.applyCache == nil {
		w.applyCache = make(map[applyKey]Guard)
	}
	w.applyStack = append(w.applyStack[:0], applyFrame{operation: operation, left: left, right: right})
	for len(w.applyStack) != 0 {
		if !w.Live() {
			return Guard{}
		}
		frame := &w.applyStack[len(w.applyStack)-1]
		if _, done := w.applyResult(frame.operation, frame.left, frame.right); done {
			w.applyStack = w.applyStack[:len(w.applyStack)-1]
			continue
		}
		switch frame.phase {
		case 0:
			frame.rank, frame.lowLeft, frame.lowRight, frame.highLeft, frame.highRight = w.applyChildren(frame.left, frame.right)
			frame.phase = 1
			if _, done := w.applyResult(frame.operation, frame.lowLeft, frame.lowRight); !done {
				w.applyStack = append(w.applyStack, applyFrame{operation: frame.operation, left: frame.lowLeft, right: frame.lowRight})
			}
		case 1:
			frame.phase = 2
			if _, done := w.applyResult(frame.operation, frame.highLeft, frame.highRight); !done {
				w.applyStack = append(w.applyStack, applyFrame{operation: frame.operation, left: frame.highLeft, right: frame.highRight})
			}
		default:
			low, _ := w.applyResult(frame.operation, frame.lowLeft, frame.lowRight)
			high, _ := w.applyResult(frame.operation, frame.highLeft, frame.highRight)
			key := applyKey{operation: frame.operation, left: keyOf(frame.left), right: keyOf(frame.right)}
			result := w.nodeOrExisting(frame.rank, low, high, frame.left, frame.right)
			w.applyCache[key] = result
			w.applyStack = w.applyStack[:len(w.applyStack)-1]
		}
	}
	return w.applyCache[applyKey{operation: operation, left: keyOf(left), right: keyOf(right)}]
}

func (w *Work) applyResult(operation operation, left, right Guard) (Guard, bool) {
	if result, done := terminalApply(w.manager, operation, left, right); done {
		return result, true
	}
	key := applyKey{operation: operation, left: keyOf(left), right: keyOf(right)}
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

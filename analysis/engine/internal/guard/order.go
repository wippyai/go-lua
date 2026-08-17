package guard

// Compare returns the canonical total order of two sealed guards. It compares
// terminal tag, fixed atom rank, low branch, then high branch. Direct handles
// are an equality fast path only: cross-generation isomorphic BDDs compare
// equal even though their page/slot handles differ.
func (m *Manager) Compare(left, right Guard) (int, bool) {
	if !m.validSealed(left) || !m.validSealed(right) {
		return 0, false
	}
	return m.compareNode(left, right), true
}

// Equivalent is exact semantic BDD equality for sealed guards.
func (m *Manager) Equivalent(left, right Guard) bool {
	comparison, valid := m.Compare(left, right)
	return valid && comparison == 0
}

type comparePair struct{ left, right Guard }

func (m *Manager) compareNode(left, right Guard) int {
	if result, done := m.compareImmediate(left, right); done {
		return result
	}
	stack := []comparePair{{left: left, right: right}}
	seen := make(map[comparePair]struct{})
	for len(stack) != 0 {
		last := len(stack) - 1
		pair := stack[last]
		stack = stack[:last]
		if result, done := m.compareImmediate(pair.left, pair.right); done {
			if result != 0 {
				return result
			}
			continue
		}
		if _, visited := seen[pair]; visited {
			continue
		}
		seen[pair] = struct{}{}
		leftNode, rightNode := m.node(pair.left), m.node(pair.right)
		// LIFO: low decides before high.
		stack = append(stack,
			comparePair{left: leftNode.high, right: rightNode.high},
			comparePair{left: leftNode.low, right: rightNode.low},
		)
	}
	return 0
}

func (m *Manager) compareImmediate(left, right Guard) (int, bool) {
	if left == right {
		return 0, true
	}
	if isTerminal(left) || isTerminal(right) {
		switch {
		case isTerminal(left) && isTerminal(right):
			if !terminalValue(left) && terminalValue(right) {
				return -1, true
			}
			return 1, true
		case isTerminal(left):
			return -1, true
		default:
			return 1, true
		}
	}
	leftRank, rightRank := m.rank(left), m.rank(right)
	if leftRank < rightRank {
		return -1, true
	}
	if leftRank > rightRank {
		return 1, true
	}
	return 0, false
}

// Entails reports whether every valuation satisfying premise satisfies
// conclusion, without allocating a candidate BDD.
func (m *Manager) Entails(premise, conclusion Guard) bool {
	if !m.validSealed(premise) || !m.validSealed(conclusion) {
		return false
	}
	// Directly identical guards have the same exact BDD meaning.  The terminal
	// cases below also settle tautology and contradiction without allocating
	// traversal scratch.  Do not extend this to structurally equal guards: the
	// cross-generation comparison remains the read-only BDD authority.
	if premise == conclusion {
		return true
	}
	if isTerminal(premise) {
		return !terminalValue(premise)
	}
	if isTerminal(conclusion) {
		return terminalValue(conclusion)
	}
	return !m.satisfiable(premise, conclusion, true)
}

type satisfiablePair struct{ left, right Guard }

func (m *Manager) satisfiable(left, right Guard, negateRight bool) bool {
	stack := []satisfiablePair{{left: left, right: right}}
	seen := make(map[satisfiablePair]struct{})
	for len(stack) != 0 {
		last := len(stack) - 1
		pair := stack[last]
		stack = stack[:last]
		if _, visited := seen[pair]; visited {
			continue
		}
		seen[pair] = struct{}{}
		if isTerminal(pair.left) && isTerminal(pair.right) {
			rightValue := terminalValue(pair.right)
			if negateRight {
				rightValue = !rightValue
			}
			if terminalValue(pair.left) && rightValue {
				return true
			}
			continue
		}
		rank := m.rank(pair.left)
		if candidate := m.rank(pair.right); candidate < rank {
			rank = candidate
		}
		leftLow, leftHigh := m.cofactor(pair.left, rank)
		rightLow, rightHigh := m.cofactor(pair.right, rank)
		stack = append(stack,
			satisfiablePair{left: leftLow, right: rightLow},
			satisfiablePair{left: leftHigh, right: rightHigh},
		)
	}
	return false
}

func (m *Manager) node(g Guard) node { return g.page.nodes[g.slot] }

func (m *Manager) cofactor(g Guard, rank uint64) (Guard, Guard) {
	if !isTerminal(g) && m.rank(g) == rank {
		n := m.node(g)
		return n.low, n.high
	}
	return g, g
}

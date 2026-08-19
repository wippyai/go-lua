package guard

// Valid accepts either a sealed predecessor guard or a current local guard.
func (w *Work) Valid(g Guard) bool {
	return w.Open() && w.owns(g)
}

// Compare is the candidate-time canonical order. Its scratch is owned by Work
// and reused across calls; Work is single-writer by contract.
func (w *Work) Compare(left, right Guard) (int, bool) {
	if !w.Live() || !w.Valid(left) || !w.Valid(right) {
		return 0, false
	}
	if result, done := w.compareImmediate(left, right); done {
		return result, true
	}
	if w.compareSeen == nil {
		w.compareSeen = make(map[compareKey]uint64)
	}
	epoch := w.beginRead()
	w.compareStack = append(w.compareStack[:0], comparePair{left: left, right: right})
	for len(w.compareStack) != 0 {
		if !w.Live() {
			return 0, false
		}
		last := len(w.compareStack) - 1
		pair := w.compareStack[last]
		w.compareStack = w.compareStack[:last]
		if result, done := w.compareImmediate(pair.left, pair.right); done {
			if result != 0 {
				return result, true
			}
			continue
		}
		key := compareKey{left: keyOf(pair.left), right: keyOf(pair.right)}
		if w.compareSeen[key] == epoch {
			continue
		}
		w.compareSeen[key] = epoch
		leftNode, rightNode := w.node(pair.left), w.node(pair.right)
		w.compareStack = append(w.compareStack,
			comparePair{left: leftNode.high, right: rightNode.high},
			comparePair{left: leftNode.low, right: rightNode.low},
		)
	}
	return 0, true
}

func (w *Work) compareImmediate(left, right Guard) (int, bool) {
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
	leftRank, rightRank := w.rank(left), w.rank(right)
	if leftRank < rightRank {
		return -1, true
	}
	if leftRank > rightRank {
		return 1, true
	}
	return 0, false
}

func (w *Work) Equivalent(left, right Guard) bool {
	comparison, valid := w.Compare(left, right)
	return valid && comparison == 0
}

func (w *Work) Conflict(left, right Guard) bool {
	if !w.Live() || !w.Valid(left) || !w.Valid(right) {
		return false
	}
	return !w.satisfiable(left, right, false)
}

func (w *Work) Entails(premise, conclusion Guard) bool {
	if !w.Live() || !w.Valid(premise) || !w.Valid(conclusion) {
		return false
	}
	// These are the exact same Boolean identities used by Manager.Entails.
	// Work may read both sealed predecessors and its own open candidate, so
	// the ownership check above remains mandatory; once it succeeds, these
	// cases need no product traversal or reusable seen-map lookup.
	if premise == conclusion {
		return w.Live()
	}
	if isTerminal(premise) {
		return !terminalValue(premise) && w.Live()
	}
	if isTerminal(conclusion) {
		return terminalValue(conclusion) && w.Live()
	}
	return !w.satisfiable(premise, conclusion, true)
}

func (w *Work) satisfiable(left, right Guard, negateRight bool) bool {
	if w.satSeen == nil {
		w.satSeen = make(map[satisfiableKey]uint64)
	} else {
		// The seen set is valid only for this product traversal. Reusing the
		// map's buckets keeps the hot read allocation-free, while clearing the
		// entries prevents one long-lived Work from retaining the union of every
		// pair visited by prior entailment calls.
		clear(w.satSeen)
	}
	epoch := w.beginRead()
	w.satStack = append(w.satStack[:0], satisfiablePair{left: left, right: right})
	for len(w.satStack) != 0 {
		if !w.Live() {
			return false
		}
		last := len(w.satStack) - 1
		pair := w.satStack[last]
		w.satStack = w.satStack[:last]
		key := satisfiableKey{left: keyOf(pair.left), right: keyOf(pair.right)}
		if w.satSeen[key] == epoch {
			continue
		}
		w.satSeen[key] = epoch
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
		rank := w.rank(pair.left)
		if candidate := w.rank(pair.right); candidate < rank {
			rank = candidate
		}
		leftLow, leftHigh := w.cofactor(pair.left, rank)
		rightLow, rightHigh := w.cofactor(pair.right, rank)
		w.satStack = append(w.satStack,
			satisfiablePair{left: leftLow, right: rightLow},
			satisfiablePair{left: leftHigh, right: rightHigh},
		)
	}
	return false
}

func (w *Work) beginRead() uint64 {
	w.readEpoch++
	if w.readEpoch == 0 {
		for pair := range w.compareSeen {
			delete(w.compareSeen, pair)
		}
		for pair := range w.satSeen {
			delete(w.satSeen, pair)
		}
		w.readEpoch++
	}
	return w.readEpoch
}

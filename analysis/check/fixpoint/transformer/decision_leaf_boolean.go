package transformer

func decisionLeafOr(left, right decisionLeaf) (decisionLeaf, error) {
	if left > 1 || right > 1 {
		return 0, errDecisionMalformed
	}
	if left == 1 || right == 1 {
		return 1, nil
	}
	return 0, nil
}

func decisionLeafAnd(left, right decisionLeaf) (decisionLeaf, error) {
	if left > 1 || right > 1 {
		return 0, errDecisionMalformed
	}
	if left == 1 && right == 1 {
		return 1, nil
	}
	return 0, nil
}

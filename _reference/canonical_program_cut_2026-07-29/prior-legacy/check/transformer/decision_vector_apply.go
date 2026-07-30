package transformer

import (
	"context"
	"math"
)

// decisionLeafVectorBinary is the terminal algebra for one correlated carrier.
// A nil leaf vector denotes an operand that is unreachable in this region.
// The result width must equal the carrier width supplied to
// applyVectorUnderCare.
type decisionLeafVectorBinary func(left, right []decisionLeaf) ([]decisionLeaf, error)

type decisionVectorRef uint32

const decisionNoVariable = uint32(math.MaxUint32)

type decisionVectorNode struct {
	width       int
	minVariable uint32
	leaf        decisionRef
	left, right decisionVectorRef
}

type decisionVectorBranchKey struct {
	width       int
	left, right decisionVectorRef
}

// decisionVectorArena is the one canonical representation of an immutable
// vector during a decision transaction. Leaves intern exact decision roots;
// branches intern the fixed balanced segment shape. Consequently equality of
// vector refs is exact vector equality, while cofactoring can retain every
// segment whose top variable is not the selected variable.
type decisionVectorArena struct {
	kernel   *decisionKernel
	nodes    []decisionVectorNode
	leaves   map[decisionRef]decisionVectorRef
	branches map[decisionVectorBranchKey]decisionVectorRef
}

func newDecisionVectorArena(kernel *decisionKernel) *decisionVectorArena {
	return &decisionVectorArena{
		kernel:   kernel,
		nodes:    []decisionVectorNode{{minVariable: decisionNoVariable}},
		leaves:   make(map[decisionRef]decisionVectorRef),
		branches: make(map[decisionVectorBranchKey]decisionVectorRef),
	}
}

func (a *decisionVectorArena) valid(ref decisionVectorRef) bool {
	return a != nil && a.kernel != nil && int(ref) < len(a.nodes)
}

func (a *decisionVectorArena) internLeaf(value decisionRef) (decisionVectorRef, error) {
	if int(value) >= len(a.kernel.nodes) {
		return 0, errDecisionMalformed
	}
	if prior, ok := a.leaves[value]; ok {
		return prior, nil
	}
	variable := decisionNoVariable
	if node := a.kernel.nodes[value]; !node.terminal {
		variable = node.variable
	}
	ref := decisionVectorRef(len(a.nodes))
	a.nodes = append(a.nodes, decisionVectorNode{width: 1, minVariable: variable, leaf: value})
	a.leaves[value] = ref
	return ref, nil
}

func (a *decisionVectorArena) internBranch(left, right decisionVectorRef) (decisionVectorRef, error) {
	if !a.valid(left) || !a.valid(right) || a.nodes[left].width == 0 || a.nodes[right].width == 0 {
		return 0, errDecisionMalformed
	}
	width := a.nodes[left].width + a.nodes[right].width
	key := decisionVectorBranchKey{width: width, left: left, right: right}
	if prior, ok := a.branches[key]; ok {
		return prior, nil
	}
	minimum := a.nodes[left].minVariable
	if rightMinimum := a.nodes[right].minVariable; rightMinimum < minimum {
		minimum = rightMinimum
	}
	ref := decisionVectorRef(len(a.nodes))
	a.nodes = append(a.nodes, decisionVectorNode{
		width: width, minVariable: minimum, left: left, right: right,
	})
	a.branches[key] = ref
	return ref, nil
}

// Build constructs the unique fixed balanced segment vector for values.
func (a *decisionVectorArena) Build(values []decisionRef) (decisionVectorRef, error) {
	if len(values) == 0 {
		return 0, nil
	}
	if len(values) == 1 {
		return a.internLeaf(values[0])
	}
	middle := len(values) / 2
	left, err := a.Build(values[:middle])
	if err != nil {
		return 0, err
	}
	right, err := a.Build(values[middle:])
	if err != nil {
		return 0, err
	}
	return a.internBranch(left, right)
}

// Width returns the number of decision roots represented by ref.
func (a *decisionVectorArena) Width(ref decisionVectorRef) (int, error) {
	if !a.valid(ref) {
		return 0, errDecisionMalformed
	}
	return a.nodes[ref].width, nil
}

// MinVariable returns the least top nonterminal variable of any member.
func (a *decisionVectorArena) MinVariable(ref decisionVectorRef) (uint32, error) {
	if !a.valid(ref) {
		return 0, errDecisionMalformed
	}
	return a.nodes[ref].minVariable, nil
}

// At returns one member without flattening the persistent vector.
func (a *decisionVectorArena) At(ref decisionVectorRef, index int) (decisionRef, error) {
	if !a.valid(ref) || index < 0 || index >= a.nodes[ref].width {
		return 0, errDecisionMalformed
	}
	for a.nodes[ref].width != 1 {
		node := a.nodes[ref]
		leftWidth := a.nodes[node.left].width
		if index < leftWidth {
			ref = node.left
		} else {
			index -= leftWidth
			ref = node.right
		}
	}
	return a.nodes[ref].leaf, nil
}

// Flatten materializes one terminal callback row. Traversal itself remains on
// the persistent representation and therefore does not copy whole vectors.
func (a *decisionVectorArena) Flatten(ref decisionVectorRef) ([]decisionRef, error) {
	width, err := a.Width(ref)
	if err != nil {
		return nil, err
	}
	values := make([]decisionRef, 0, width)
	if width == 0 {
		return values, nil
	}
	stack := []decisionVectorRef{ref}
	for len(stack) != 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		node := a.nodes[current]
		if node.width == 1 {
			values = append(values, node.leaf)
			continue
		}
		stack = append(stack, node.right, node.left)
	}
	return values, nil
}

// SplitAt computes the pointwise low/high cofactor at variable. Only segment
// children whose minimum top variable equals variable are visited; all other
// children are retained by exact identity. Callers select the global minimum,
// so observing a smaller variable is malformed traversal state.
func (a *decisionVectorArena) SplitAt(ref decisionVectorRef, variable uint32) (decisionVectorRef, decisionVectorRef, error) {
	if !a.valid(ref) {
		return 0, 0, errDecisionMalformed
	}
	node := a.nodes[ref]
	if node.width == 0 {
		return ref, ref, nil
	}
	if node.minVariable < variable {
		return 0, 0, errDecisionMalformed
	}
	if node.minVariable != variable {
		return ref, ref, nil
	}
	if node.width == 1 {
		root := a.kernel.nodes[node.leaf]
		if root.terminal || root.variable != variable {
			return 0, 0, errDecisionMalformed
		}
		low, err := a.internLeaf(root.low)
		if err != nil {
			return 0, 0, err
		}
		high, err := a.internLeaf(root.high)
		return low, high, err
	}
	leftLow, leftHigh := node.left, node.left
	rightLow, rightHigh := node.right, node.right
	var err error
	if a.nodes[node.left].minVariable == variable {
		leftLow, leftHigh, err = a.SplitAt(node.left, variable)
		if err != nil {
			return 0, 0, err
		}
	}
	if a.nodes[node.right].minVariable == variable {
		rightLow, rightHigh, err = a.SplitAt(node.right, variable)
		if err != nil {
			return 0, 0, err
		}
	}
	low, err := a.internBranch(leftLow, rightLow)
	if err != nil {
		return 0, 0, err
	}
	high, err := a.internBranch(leftHigh, rightHigh)
	return low, high, err
}

// LiftBranch reconstructs the pointwise decision branch of two equal-width
// vectors. Interning makes reconvergent vectors recover their prior identity.
func (a *decisionVectorArena) LiftBranch(variable uint32, low, high decisionVectorRef) (decisionVectorRef, error) {
	if !a.valid(low) || !a.valid(high) || a.nodes[low].width != a.nodes[high].width {
		return 0, errDecisionMalformed
	}
	if low == high {
		return low, nil
	}
	lowNode, highNode := a.nodes[low], a.nodes[high]
	if lowNode.width == 1 {
		if highNode.width != 1 || lowNode.minVariable <= variable || highNode.minVariable <= variable {
			return 0, errDecisionMalformed
		}
		return a.internLeaf(a.kernel.branch(variable, lowNode.leaf, highNode.leaf))
	}
	if lowNode.width != highNode.width || lowNode.width == 1 || highNode.width == 1 {
		return 0, errDecisionMalformed
	}
	left, err := a.LiftBranch(variable, lowNode.left, highNode.left)
	if err != nil {
		return 0, err
	}
	right, err := a.LiftBranch(variable, lowNode.right, highNode.right)
	if err != nil {
		return 0, err
	}
	return a.internBranch(left, right)
}

type decisionVectorApplyKey struct {
	resultCare          decisionRef
	leftCare, rightCare decisionRef
	left, right         decisionVectorRef
}

// applyNaryUnderCare maps one dependent vector to one decision root. Unlike
// applyVectorUnderCare it does not retain or reconstruct the input members as
// outputs; memoization therefore tracks only exact input cofactors and the
// single semantic result. This is the canonical carrier for pure n-ary nodes
// such as ObjectLiteralPlan after their operands have been quotiented to the
// observations that node can distinguish.
func (k *decisionKernel) applyNaryUnderCare(
	ctx context.Context,
	care decisionRef,
	roots []decisionRef,
	leaves func([]decisionLeaf) (decisionLeaf, error),
) (decisionRef, error) {
	if ctx == nil || leaves == nil || care == decisionFalse || len(roots) == 0 || int(care) >= len(k.nodes) {
		return 0, errDecisionMalformed
	}
	for _, root := range roots {
		if int(root) >= len(k.nodes) {
			return 0, errDecisionMalformed
		}
	}
	vectors := newDecisionVectorArena(k)
	vector, err := vectors.Build(roots)
	if err != nil {
		return 0, err
	}
	type key struct {
		care   decisionRef
		vector decisionVectorRef
	}
	type frame struct {
		key       key
		expanded  bool
		variable  uint32
		low, high key
		lowLive   bool
		highLive  bool
	}
	root := key{care: care, vector: vector}
	memo := make(map[key]decisionRef)
	stack := []frame{{key: root}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if _, done := memo[current.key]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		k.applyOps++
		if k.applyOps&255 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		careNode := k.nodes[current.key.care]
		if careNode.terminal && careNode.leaf > 1 {
			return 0, errDecisionMalformed
		}
		if !current.expanded {
			minimum, minimumErr := vectors.MinVariable(current.key.vector)
			if minimumErr != nil {
				return 0, minimumErr
			}
			current.variable = minimum
			if !careNode.terminal && careNode.variable < current.variable {
				current.variable = careNode.variable
			}
			if current.variable == decisionNoVariable {
				if !careNode.terminal || careNode.leaf != 1 {
					return 0, errDecisionMalformed
				}
				values, flattenErr := vectors.Flatten(current.key.vector)
				if flattenErr != nil || len(values) != len(roots) {
					if flattenErr == nil {
						flattenErr = errDecisionMalformed
					}
					return 0, flattenErr
				}
				terminalLeaves := make([]decisionLeaf, len(values))
				for index, value := range values {
					node := k.nodes[value]
					if !node.terminal {
						return 0, errDecisionMalformed
					}
					terminalLeaves[index] = node.leaf
				}
				leaf, leafErr := leaves(terminalLeaves)
				if leafErr != nil {
					return 0, leafErr
				}
				memo[current.key] = k.terminal(leaf)
				stack = stack[:len(stack)-1]
				continue
			}
			current.expanded = true
			lowCare, highCare := current.key.care, current.key.care
			if !careNode.terminal && careNode.variable == current.variable {
				lowCare, highCare = careNode.low, careNode.high
			}
			lowVector, highVector, splitErr := vectors.SplitAt(current.key.vector, current.variable)
			if splitErr != nil {
				return 0, splitErr
			}
			current.lowLive, current.highLive = lowCare != decisionFalse, highCare != decisionFalse
			current.low = key{care: lowCare, vector: lowVector}
			current.high = key{care: highCare, vector: highVector}
		}
		if current.lowLive {
			if _, done := memo[current.low]; !done {
				stack = append(stack, frame{key: current.low})
				continue
			}
		}
		if current.highLive {
			if _, done := memo[current.high]; !done {
				stack = append(stack, frame{key: current.high})
				continue
			}
		}
		switch {
		case !current.lowLive:
			memo[current.key] = memo[current.high]
		case !current.highLive:
			memo[current.key] = memo[current.low]
		default:
			memo[current.key] = k.branch(current.variable, memo[current.low], memo[current.high])
		}
		stack = stack[:len(stack)-1]
	}
	result, present := memo[root]
	if !present {
		return 0, errDecisionMalformed
	}
	return result, nil
}

// applyVectorUnderCare is the direct multi-output analogue of applyUnderCare.
// It aligns exactly one dependent carrier under resultCare and reconstructs
// its roots bottom-up. It never materializes leaf-region rows and never forms
// a common refinement with any independent carrier.
func (k *decisionKernel) applyVectorUnderCare(
	ctx context.Context,
	resultCare, leftCare, rightCare decisionRef,
	leftRoots, rightRoots []decisionRef,
	leaves decisionLeafVectorBinary,
) ([]decisionRef, error) {
	if ctx == nil || leaves == nil || resultCare == decisionFalse || len(leftRoots) == 0 || len(leftRoots) != len(rightRoots) {
		return nil, errDecisionMalformed
	}
	for _, ref := range append([]decisionRef{resultCare, leftCare, rightCare}, append(append([]decisionRef(nil), leftRoots...), rightRoots...)...) {
		if int(ref) >= len(k.nodes) {
			return nil, errDecisionMalformed
		}
	}
	vectors := newDecisionVectorArena(k)
	leftVector, err := vectors.Build(leftRoots)
	if err != nil {
		return nil, err
	}
	rightVector, err := vectors.Build(rightRoots)
	if err != nil {
		return nil, err
	}
	falseRoots := make([]decisionRef, len(leftRoots))
	falseVector, err := vectors.Build(falseRoots)
	if err != nil {
		return nil, err
	}
	root := decisionVectorApplyKey{
		resultCare: resultCare, leftCare: leftCare, rightCare: rightCare,
		left: leftVector, right: rightVector,
	}
	type frame struct {
		key               decisionVectorApplyKey
		expanded          bool
		variable          uint32
		low, high         decisionVectorApplyKey
		lowLive, highLive bool
	}
	memo := make(map[decisionVectorApplyKey]decisionVectorRef)
	stack := []frame{{key: root}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if _, done := memo[current.key]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		key := current.key
		if int(key.resultCare) >= len(k.nodes) || int(key.leftCare) >= len(k.nodes) || int(key.rightCare) >= len(k.nodes) {
			return nil, errDecisionMalformed
		}
		k.applyOps++
		if k.applyOps&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		resultCareNode := k.nodes[key.resultCare]
		leftCareNode, rightCareNode := k.nodes[key.leftCare], k.nodes[key.rightCare]
		if resultCareNode.terminal && resultCareNode.leaf > 1 || leftCareNode.terminal && leftCareNode.leaf > 1 || rightCareNode.terminal && rightCareNode.leaf > 1 {
			return nil, errDecisionMalformed
		}
		leftLive, rightLive := key.leftCare != decisionFalse, key.rightCare != decisionFalse
		leftWidth, leftWidthErr := vectors.Width(key.left)
		rightWidth, rightWidthErr := vectors.Width(key.right)
		if leftWidthErr != nil || rightWidthErr != nil || leftWidth != len(leftRoots) || rightWidth != len(rightRoots) {
			return nil, errDecisionMalformed
		}
		if !current.expanded {
			current.variable = ^uint32(0)
			for _, node := range []decisionNode{resultCareNode, leftCareNode, rightCareNode} {
				if !node.terminal && node.variable < current.variable {
					current.variable = node.variable
				}
			}
			if leftLive {
				minimum, err := vectors.MinVariable(key.left)
				if err != nil {
					return nil, err
				}
				if minimum < current.variable {
					current.variable = minimum
				}
			}
			if rightLive {
				minimum, err := vectors.MinVariable(key.right)
				if err != nil {
					return nil, err
				}
				if minimum < current.variable {
					current.variable = minimum
				}
			}
			if current.variable == ^uint32(0) {
				if !resultCareNode.terminal || resultCareNode.leaf != 1 || !leftLive && !rightLive {
					return nil, errDecisionMalformed
				}
				var leftLeaves, rightLeaves []decisionLeaf
				if leftLive {
					leftValues, flattenErr := vectors.Flatten(key.left)
					if flattenErr != nil {
						return nil, flattenErr
					}
					leftLeaves = make([]decisionLeaf, len(leftValues))
					for index, ref := range leftValues {
						node := k.nodes[ref]
						if !node.terminal {
							return nil, errDecisionMalformed
						}
						leftLeaves[index] = node.leaf
					}
				}
				if rightLive {
					rightValues, flattenErr := vectors.Flatten(key.right)
					if flattenErr != nil {
						return nil, flattenErr
					}
					rightLeaves = make([]decisionLeaf, len(rightValues))
					for index, ref := range rightValues {
						node := k.nodes[ref]
						if !node.terminal {
							return nil, errDecisionMalformed
						}
						rightLeaves[index] = node.leaf
					}
				}
				resultLeaves, err := leaves(leftLeaves, rightLeaves)
				if err != nil || len(resultLeaves) != len(leftRoots) {
					if err == nil {
						err = errDecisionMalformed
					}
					return nil, err
				}
				result := make([]decisionRef, len(resultLeaves))
				for index, leaf := range resultLeaves {
					result[index] = k.terminal(leaf)
				}
				resultVector, buildErr := vectors.Build(result)
				if buildErr != nil {
					return nil, buildErr
				}
				memo[key] = resultVector
				stack = stack[:len(stack)-1]
				continue
			}
			current.expanded = true
			splitRef := func(ref decisionRef) (decisionRef, decisionRef) {
				node := k.nodes[ref]
				if !node.terminal && node.variable == current.variable {
					return node.low, node.high
				}
				return ref, ref
			}
			splitVector := func(ref decisionVectorRef, live bool) (decisionVectorRef, decisionVectorRef, error) {
				if !live {
					return ref, ref, nil
				}
				return vectors.SplitAt(ref, current.variable)
			}
			resultLow, resultHigh := splitRef(key.resultCare)
			leftCareLow, leftCareHigh := splitRef(key.leftCare)
			rightCareLow, rightCareHigh := splitRef(key.rightCare)
			leftLow, leftHigh, err := splitVector(key.left, leftLive)
			if err != nil {
				return nil, err
			}
			rightLow, rightHigh, err := splitVector(key.right, rightLive)
			if err != nil {
				return nil, err
			}
			current.lowLive, current.highLive = resultLow != decisionFalse, resultHigh != decisionFalse
			if !current.lowLive && !current.highLive {
				return nil, errDecisionMalformed
			}
			current.low = decisionVectorApplyKey{resultCare: resultLow, leftCare: leftCareLow, rightCare: rightCareLow, left: leftLow, right: rightLow}
			current.high = decisionVectorApplyKey{resultCare: resultHigh, leftCare: leftCareHigh, rightCare: rightCareHigh, left: leftHigh, right: rightHigh}
		}
		if current.lowLive {
			if _, done := memo[current.low]; !done {
				stack = append(stack, frame{key: current.low})
				continue
			}
		}
		if current.highLive {
			if _, done := memo[current.high]; !done {
				stack = append(stack, frame{key: current.high})
				continue
			}
		}
		switch {
		case !current.lowLive:
			result, err := vectors.LiftBranch(current.variable, falseVector, memo[current.high])
			if err != nil {
				return nil, err
			}
			memo[key] = result
		case !current.highLive:
			result, err := vectors.LiftBranch(current.variable, memo[current.low], falseVector)
			if err != nil {
				return nil, err
			}
			memo[key] = result
		default:
			result, err := vectors.LiftBranch(current.variable, memo[current.low], memo[current.high])
			if err != nil {
				return nil, err
			}
			memo[key] = result
		}
		stack = stack[:len(stack)-1]
	}
	result, err := vectors.Flatten(memo[root])
	if err != nil || len(result) != len(leftRoots) {
		return nil, errDecisionMalformed
	}
	return result, nil
}

package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// Expr is a closed immutable reduced ordered decision DAG. Node 0 is false
// and node 1 is true; every other node is ITE(Decision, Low, High). It is
// equation declaration data only: carrier later interprets the retained DAG.
// Reduction and canonical node order make equivalent Boolean expressions one
// representation without importing guard atoms or an SMT layer.
type Expr struct {
	nodes []exprNode
	root  uint32
	valid bool
}

type exprNode struct {
	decision Decision
	low      uint32
	high     uint32
}

// ExprNode is one postorder node in a sealed reduced decision DAG. It exists
// for exact carrier-to-equation transport only; ordinary builder APIs still
// use Expr combinators. Low/High use the same 0=false, 1=true, 2+=node
// ordinal convention as Expr.NodeAt.
type ExprNode struct {
	Decision Decision
	Low      uint32
	High     uint32
}

// NewExprDAGWithCheckpoint is NewExprDAG with an evaluator-owned liveness
// probe. A canceled conversion returns no partial equation evidence.
func NewExprDAGWithCheckpoint(rows []ExprNode, root uint32, checkpoint func() bool) (Expr, bool) {
	if root > uint32(len(rows)+1) {
		return Expr{}, false
	}
	// Input row order is transport detail.  Seal only the root-reachable DAG
	// and rebuild its unique low-to-high postorder representation, so a raw
	// ordinal can neither become serialization identity nor hide dead rows.
	state := make([]uint8, len(rows)+2) // 0=new, 1=visiting, 2=done
	ordinal := make([]uint32, len(rows)+2)
	ordinal[0], ordinal[1] = 0, 1
	nodes := make([]exprNode, 0, len(rows))
	unique := make(map[exprNodeKey]uint32, len(rows))
	type frame struct {
		id    uint32
		ready bool
	}
	stack := []frame{{id: root}}
	reachable := 0
	for len(stack) != 0 {
		if checkpoint != nil && !checkpoint() {
			return Expr{}, false
		}
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if current.id < 2 {
			continue
		}
		if current.id > uint32(len(rows)+1) {
			return Expr{}, false
		}
		index := current.id - 2
		if current.ready {
			if state[current.id] != 1 {
				return Expr{}, false
			}
			row := rows[index]
			if !row.Decision.Available() || row.Low == row.High || row.Low > uint32(len(rows)+1) || row.High > uint32(len(rows)+1) {
				return Expr{}, false
			}
			for _, child := range []uint32{row.Low, row.High} {
				if child < 2 {
					continue
				}
				if state[child] != 2 || !lessKey(row.Decision.key, rows[child-2].Decision.key) {
					return Expr{}, false
				}
			}
			low, high := ordinal[row.Low], ordinal[row.High]
			if low == high {
				return Expr{}, false
			}
			key := exprNodeKey{decision: row.Decision.key, low: low, high: high}
			if existing, found := unique[key]; found {
				ordinal[current.id] = existing
			} else {
				ordinal[current.id] = uint32(len(nodes) + 2)
				nodes = append(nodes, exprNode{decision: row.Decision, low: low, high: high})
				unique[key] = ordinal[current.id]
			}
			state[current.id] = 2
			continue
		}
		switch state[current.id] {
		case 2:
			continue
		case 1:
			return Expr{}, false // cycle
		}
		row := rows[index]
		if !row.Decision.Available() || row.Low == row.High || row.Low > uint32(len(rows)+1) || row.High > uint32(len(rows)+1) {
			return Expr{}, false
		}
		state[current.id] = 1
		reachable++
		stack = append(stack, frame{id: current.id, ready: true}, frame{id: row.High}, frame{id: row.Low})
	}
	if reachable != len(rows) {
		return Expr{}, false
	}
	return Expr{nodes: nodes, root: ordinal[root], valid: true}, true
}

func FalseExpr() Expr { return Expr{valid: true} }
func TrueExpr() Expr  { return Expr{root: 1, valid: true} }

func DecisionExpr(decision Decision) (Expr, bool) {
	if !decision.Available() {
		return Expr{}, false
	}
	builder := newExprBuilder()
	root, ok := builder.node(decision, 0, 1)
	if !ok {
		return Expr{}, false
	}
	return builder.freeze(root)
}

func NotExpr(expr Expr) (Expr, bool) {
	return combineExpr(expr, Expr{}, exprNot)
}
func AndExpr(left, right Expr) (Expr, bool) {
	return combineExpr(left, right, exprAnd)
}
func OrExpr(left, right Expr) (Expr, bool) {
	return combineExpr(left, right, exprOr)
}

// Available reports whether expr is a sealed reduced decision DAG. Every
// constructor proves root <= len(nodes)+1 before it ever sets valid, so the
// accessor reads that settled bit instead of re-checking the bound.
func (expr Expr) Available() bool { return expr.valid }
func (expr Expr) IsFalse() bool   { return expr.Available() && expr.root == 0 }
func (expr Expr) IsTrue() bool    { return expr.Available() && expr.root == 1 }

// sameExpr is exact reduced-DAG equality for sealed equation formulas.  It is
// intentionally private: callers retain Expr rather than a second formula
// key, while Batch uses it only to reject two different initialization rows
// for one exact source entity before those rows have canonical identities.
func sameExpr(left, right Expr) bool {
	if !left.Available() || !right.Available() || left.root != right.root || len(left.nodes) != len(right.nodes) {
		return false
	}
	for index := range left.nodes {
		first, second := left.nodes[index], right.nodes[index]
		if first.decision != second.decision || first.low != second.low || first.high != second.high {
			return false
		}
	}
	return true
}

// Decisions returns the exact sorted decision support of the expression.
// A reduced DAG may have multiple nodes at one decision level, so support is
// deduplicated rather than exposing its implementation node multiplicity.
func (expr Expr) Decisions() []Decision {
	if !expr.Available() {
		return nil
	}
	set := make(map[composition.Key]Decision, len(expr.nodes))
	for _, node := range expr.nodes {
		set[node.decision.key] = node.decision
	}
	result := make([]Decision, 0, len(set))
	for _, decision := range set {
		result = append(result, decision)
	}
	sort.Slice(result, func(i, j int) bool { return lessKey(result[i].key, result[j].key) })
	return result
}

// Root returns the immutable DAG root. Edge ordinal 0 is false and 1 is
// true; every ordinal >= 2 names NodeAt(ordinal-2). This is the exact
// symbolic function retained for private lowering, not merely its support.
func (expr Expr) Root() (uint32, bool) {
	if !expr.Available() {
		return 0, false
	}
	return expr.root, true
}

func (expr Expr) NodeCount() int {
	if !expr.Available() {
		return 0
	}
	return len(expr.nodes)
}

// NodeAt returns one immutable ITE node in ordinal order. Low and High use
// Root's terminal/node ordinal convention. No mutable node storage escapes.
func (expr Expr) NodeAt(index int) (Decision, uint32, uint32, bool) {
	if !expr.Available() || index < 0 || index >= len(expr.nodes) {
		return Decision{}, 0, 0, false
	}
	node := expr.nodes[index]
	return node.decision, node.low, node.high, true
}

type exprOp uint8

const (
	exprNot exprOp = iota + 1
	exprAnd
	exprOr
)

func combineExpr(left, right Expr, operation exprOp) (Expr, bool) {
	if !left.Available() || operation != exprNot && !right.Available() {
		return Expr{}, false
	}
	if operation == exprNot {
		builder, roots, ok := importExprs(left)
		if !ok {
			return Expr{}, false
		}
		root, ok := builder.not(roots[0])
		if !ok {
			return Expr{}, false
		}
		return builder.freeze(root)
	}
	builder, roots, ok := importExprs(left, right)
	if !ok {
		return Expr{}, false
	}
	var root uint32
	switch operation {
	case exprAnd:
		root, ok = builder.and(roots[0], roots[1])
	case exprOr:
		root, ok = builder.or(roots[0], roots[1])
	default:
		return Expr{}, false
	}
	if !ok {
		return Expr{}, false
	}
	return builder.freeze(root)
}

type exprNodeKey struct {
	decision composition.Key
	low      uint32
	high     uint32
}

type exprBuilder struct {
	nodes  []exprNode
	unique map[exprNodeKey]uint32
	cache  map[[3]uint32]uint32
}

func newExprBuilder() *exprBuilder {
	return &exprBuilder{unique: make(map[exprNodeKey]uint32), cache: make(map[[3]uint32]uint32)}
}

func (builder *exprBuilder) node(decision Decision, low, high uint32) (uint32, bool) {
	if !decision.Available() || low > uint32(len(builder.nodes)+1) || high > uint32(len(builder.nodes)+1) {
		return 0, false
	}
	if low == high {
		return low, true
	}
	for _, child := range []uint32{low, high} {
		if child >= 2 && !lessKey(decision.key, builder.nodes[child-2].decision.key) {
			return 0, false
		}
	}
	key := exprNodeKey{decision: decision.key, low: low, high: high}
	if existing, ok := builder.unique[key]; ok {
		return existing, true
	}
	result := uint32(len(builder.nodes) + 2)
	builder.nodes = append(builder.nodes, exprNode{decision: decision, low: low, high: high})
	builder.unique[key] = result
	return result, true
}

func (builder *exprBuilder) decision(node uint32) (Decision, bool) {
	if node < 2 || node > uint32(len(builder.nodes)+1) {
		return Decision{}, false
	}
	return builder.nodes[node-2].decision, true
}
func (builder *exprBuilder) branch(node uint32, high bool) (uint32, bool) {
	if node < 2 || node > uint32(len(builder.nodes)+1) {
		return 0, false
	}
	if high {
		return builder.nodes[node-2].high, true
	}
	return builder.nodes[node-2].low, true
}

func (builder *exprBuilder) ite(test, yes, no uint32) (uint32, bool) {
	if test > uint32(len(builder.nodes)+1) || yes > uint32(len(builder.nodes)+1) || no > uint32(len(builder.nodes)+1) {
		return 0, false
	}
	if test == 1 || yes == no {
		return yes, true
	}
	if test == 0 {
		return no, true
	}
	key := [3]uint32{test, yes, no}
	if result, ok := builder.cache[key]; ok {
		return result, true
	}
	decision, ok := builder.decision(test)
	if !ok {
		return 0, false
	}
	for _, node := range []uint32{yes, no} {
		if node < 2 {
			continue
		}
		other, ok := builder.decision(node)
		if !ok {
			return 0, false
		}
		if lessKey(other.key, decision.key) {
			decision = other
		}
	}
	cofactor := func(node uint32, high bool) (uint32, bool) {
		if node < 2 {
			return node, true
		}
		nodeDecision, ok := builder.decision(node)
		if !ok {
			return 0, false
		}
		if nodeDecision == decision {
			return builder.branch(node, high)
		}
		return node, true
	}
	tLow, ok := cofactor(test, false)
	if !ok {
		return 0, false
	}
	tHigh, ok := cofactor(test, true)
	if !ok {
		return 0, false
	}
	yLow, ok := cofactor(yes, false)
	if !ok {
		return 0, false
	}
	yHigh, ok := cofactor(yes, true)
	if !ok {
		return 0, false
	}
	nLow, ok := cofactor(no, false)
	if !ok {
		return 0, false
	}
	nHigh, ok := cofactor(no, true)
	if !ok {
		return 0, false
	}
	low, ok := builder.ite(tLow, yLow, nLow)
	if !ok {
		return 0, false
	}
	high, ok := builder.ite(tHigh, yHigh, nHigh)
	if !ok {
		return 0, false
	}
	result, ok := builder.node(decision, low, high)
	if ok {
		builder.cache[key] = result
	}
	return result, ok
}

func (builder *exprBuilder) not(value uint32) (uint32, bool) { return builder.ite(value, 0, 1) }
func (builder *exprBuilder) and(left, right uint32) (uint32, bool) {
	return builder.ite(left, right, 0)
}
func (builder *exprBuilder) or(left, right uint32) (uint32, bool) { return builder.ite(left, 1, right) }

func importExprs(expressions ...Expr) (*exprBuilder, []uint32, bool) {
	builder := newExprBuilder()
	roots := make([]uint32, len(expressions))
	for expressionIndex, expression := range expressions {
		if !expression.Available() {
			return nil, nil, false
		}
		seen := make(map[uint32]uint32, len(expression.nodes)+2)
		seen[0], seen[1] = 0, 1
		var copyNode func(uint32) (uint32, bool)
		copyNode = func(index uint32) (uint32, bool) {
			if result, ok := seen[index]; ok {
				return result, true
			}
			if index < 2 || index > uint32(len(expression.nodes)+1) {
				return 0, false
			}
			node := expression.nodes[index-2]
			low, ok := copyNode(node.low)
			if !ok {
				return 0, false
			}
			high, ok := copyNode(node.high)
			if !ok {
				return 0, false
			}
			result, ok := builder.node(node.decision, low, high)
			if !ok {
				return 0, false
			}
			seen[index] = result
			return result, true
		}
		root, ok := copyNode(expression.root)
		if !ok {
			return nil, nil, false
		}
		roots[expressionIndex] = root
	}
	return builder, roots, true
}

func (builder *exprBuilder) freeze(root uint32) (Expr, bool) {
	if root > uint32(len(builder.nodes)+1) {
		return Expr{}, false
	}
	// Rebuild reachable nodes in deterministic post-order: low, high, then
	// node. Canonical IDs no longer depend on construction path or map order.
	remap := map[uint32]uint32{0: 0, 1: 1}
	nodes := make([]exprNode, 0, len(builder.nodes))
	var visit func(uint32) (uint32, bool)
	visit = func(index uint32) (uint32, bool) {
		if result, ok := remap[index]; ok {
			return result, true
		}
		if index < 2 || index > uint32(len(builder.nodes)+1) {
			return 0, false
		}
		node := builder.nodes[index-2]
		low, ok := visit(node.low)
		if !ok {
			return 0, false
		}
		high, ok := visit(node.high)
		if !ok {
			return 0, false
		}
		if low == high {
			remap[index] = low
			return low, true
		}
		result := uint32(len(nodes) + 2)
		nodes = append(nodes, exprNode{decision: node.decision, low: low, high: high})
		remap[index] = result
		return result, true
	}
	canonicalRoot, ok := visit(root)
	if !ok {
		return Expr{}, false
	}
	return Expr{nodes: nodes, root: canonicalRoot, valid: true}, true
}

// Package branch declares Numeric's exact primitive equality and integer-order
// truth-branch judgments. Meta/fallback results and generic truth values are
// deliberately outside this child.
package branch

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/domain/numeric"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

type branchOperand struct {
	source     *link.Link
	algebra    *numeric.Algebra
	shard      linkproject.Shard
	binary     keyspace.Term
	branchTerm keyspace.Term
	owner      keyspace.Term
	rawLeft    keyspace.Term
	rawRight   keyspace.Term
	branch     uint8
	branchRoot numeric.Root
	inputKey   numeric.Key
	outputKey  numeric.Key
	left       numeric.Atom
	right      numeric.Atom
	op         kind.BinaryOp
	invert     bool
	truthy     bool
	content    [32]byte
}

// newBranchOperand consumes the already-sealed Binary primitive and its
// normalized comparison projection. Branch identity and bodies come only
// from the Flow BinaryPrimitives projection.
func newBranchOperand(source *link.Link, algebra *numeric.Algebra, shard linkproject.Shard, binary keyspace.Term, branch int) (branchOperand, bool) {
	if source == nil || algebra == nil || !algebra.Valid() || algebra.Link() != source || source.ContentID() != algebra.LinkID() || branch < 0 || branch > 1 {
		return branchOperand{}, false
	}
	project := source.Project()
	if project == nil {
		return branchOperand{}, false
	}
	mounts := project.Mounts()
	shardIndex, shardOK := mounts.Index(shard)
	if !shardOK || uint64(shardIndex+1) > uint64(^uint32(0)) {
		return branchOperand{}, false
	}
	p, present := mounts.Program(shard)
	if !present || p == nil {
		return branchOperand{}, false
	}
	primitives := p.Flow().BinaryPrimitives()
	if !primitives.Available() {
		return branchOperand{}, false
	}
	primitive, retained := primitives.Primitive(binary)
	operation, operationOK := primitive.Operation()
	comparison, comparisonOK := primitive.Comparison()
	resultTerm, sourceOK := primitive.Source()
	if !retained || !operationOK || !comparisonOK || !sourceOK || resultTerm != binary || !p.Flow().Executable().Contains(binary) ||
		!numericBranchOperator(operation.Op) || keyspace.TermFamily(operation.Owner) != keyspace.FamilyBody ||
		keyspace.TermOrdinal(operation.Owner) == 0 || !p.Flow().Executable().Contains(operation.Owner) ||
		keyspace.TermFamily(comparison.Branch) != keyspace.FamilyBranch || keyspace.TermOrdinal(comparison.Branch) == 0 || !p.Flow().Executable().Contains(comparison.Branch) ||
		keyspace.TermFamily(comparison.TrueBody) != keyspace.FamilyBody || keyspace.TermOrdinal(comparison.TrueBody) == 0 ||
		keyspace.TermFamily(comparison.FalseBody) != keyspace.FamilyBody || keyspace.TermOrdinal(comparison.FalseBody) == 0 {
		return branchOperand{}, false
	}
	truthy := branch == 0
	body := comparison.FalseBody
	if truthy {
		body = comparison.TrueBody
	}
	branchRoot, rooted := algebra.RootFor(shard, body)
	inputBody, _, _, firstPositioned := p.Source().Index().Position(comparison.Left)
	secondBody, _, _, secondPositioned := p.Source().Index().Position(comparison.Right)
	first, firstOK := algebra.ScalarFor(shard, inputBody, comparison.Left)
	second, secondOK := algebra.ScalarFor(shard, secondBody, comparison.Right)
	inputRoot, inputRooted := algebra.RootFor(shard, inputBody)
	secondRoot, secondRooted := algebra.RootFor(shard, secondBody)
	inputKey, inputKeyed := algebra.KeyFor(inputRoot)
	outputKey, outputKeyed := algebra.KeyFor(branchRoot)
	left, leftOK := algebra.AtomFor(first)
	right, rightOK := algebra.AtomFor(second)
	if !rooted || !firstPositioned || !secondPositioned || inputBody != operation.Owner || secondBody != operation.Owner || !firstOK || !secondOK ||
		!inputRooted || !secondRooted || inputRoot != secondRoot || !inputKeyed || !outputKeyed || !leftOK || !rightOK {
		return branchOperand{}, false
	}
	if !source.ContentID().Available() {
		return branchOperand{}, false
	}
	content := branchContent(source.ContentID(), uint32(shardIndex+1), binary, uint8(branch))
	return branchOperand{
		source: source, algebra: algebra, shard: shard, binary: binary, branchTerm: comparison.Branch,
		owner: operation.Owner, rawLeft: operation.Left, rawRight: operation.Right,
		branch: uint8(branch), branchRoot: branchRoot, inputKey: inputKey, outputKey: outputKey,
		left: left, right: right, op: operation.Op, invert: comparison.Invert, truthy: truthy, content: content,
	}, true
}

func (operand branchOperand) valid() bool {
	if operand.source == nil || operand.algebra == nil || operand.content == [32]byte{} || operand.algebra.Link() != operand.source || operand.source.ContentID() != operand.algebra.LinkID() {
		return false
	}
	expected, ok := newBranchOperand(operand.source, operand.algebra, operand.shard, operand.binary, int(operand.branch))
	return ok && expected.branchTerm == operand.branchTerm && expected.owner == operand.owner &&
		expected.rawLeft == operand.rawLeft && expected.rawRight == operand.rawRight &&
		expected.branchRoot == operand.branchRoot && expected.inputKey == operand.inputKey && expected.outputKey == operand.outputKey &&
		expected.left == operand.left && expected.right == operand.right && expected.op == operand.op && expected.invert == operand.invert &&
		expected.truthy == operand.truthy && expected.content == operand.content
}

func (operand branchOperand) BranchRoot() (numeric.Root, bool) {
	if !operand.valid() {
		return numeric.Root{}, false
	}
	return operand.branchRoot, true
}

func numericBranchOperator(op kind.BinaryOp) bool {
	return equalityOperator(op) || orderOperator(op)
}

func pairSupported(algebra *numeric.Algebra, key numeric.Key, left, right numeric.Atom) bool {
	pair, ok := algebra.Pair(left, right)
	if !ok {
		return false
	}
	for index := 0; index < algebra.PairCount(key); index++ {
		candidate, present := algebra.PairAt(key, index)
		if !present {
			return false
		}
		if candidate == pair {
			return true
		}
	}
	return false
}

func equalityOperator(op kind.BinaryOp) bool {
	return op == kind.BinaryEqual || op == kind.BinaryNotEqual
}

func orderOperator(op kind.BinaryOp) bool {
	switch op {
	case kind.BinaryLess, kind.BinaryLessEqual, kind.BinaryGreater, kind.BinaryGreaterEqual:
		return true
	default:
		return false
	}
}

func branchContent(linkID keyspace.ContentID, shardOrdinal uint32, binaryTerm keyspace.Term, branch uint8) [32]byte {
	var payload [64]byte
	copy(payload[:32], linkID[:])
	copy(payload[32:40], []byte("num-brn!"))
	binary.BigEndian.PutUint64(payload[40:48], uint64(shardOrdinal))
	binary.BigEndian.PutUint64(payload[48:56], uint64(binaryTerm))
	payload[56] = branch
	return sha256.Sum256(payload[:])
}

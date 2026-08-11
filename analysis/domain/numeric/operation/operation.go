// Package operation declares Numeric's primitive Binary-result judgment.
//
// The judgment is deliberately representation-only.  It records the exact
// runtime numeric representation guaranteed by an already-selected Flow
// BinaryPrimitive; it does not pretend that the finite difference carrier can
// encode multiplication, division, modulo, power, or Lua's wrapping integer
// arithmetic as affine equations.
package operation

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"github.com/wippyai/go-lua/analysis/domain/numeric"
	numericowner "github.com/wippyai/go-lua/analysis/domain/numeric/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// Operand is one exact primitive numeric Binary occurrence. Its three scalar
// occurrences and common Numeric key are reconstructed from Link together,
// so a caller cannot splice a result from another operation.
type Operand struct {
	source              *link.Link
	algebra             *numeric.Algebra
	shard               linkproject.Shard
	binary              keyspace.Term
	owner               keyspace.Term
	leftTerm, rightTerm keyspace.Term
	key                 numeric.Key
	result              numeric.Atom
	left                numeric.Atom
	right               numeric.Atom
	op                  kind.BinaryOp
	content             [32]byte
}

// Key returns the Numeric root selected by this exact primitive occurrence.
// The key remains owner-fenced: malformed or stale operands do not leak a
// usable coordinate to a projection consumer.
func (operand Operand) Key() (numeric.Key, bool) {
	if !operand.valid() {
		return numeric.Key{}, false
	}
	return operand.key, true
}

// NewOperand admits only an existing Flow BinaryPrimitive in the arithmetic or
// bitwise buckets. The primitive row is structural evidence of the Lua
// primitive route; the Rule's Numeric input then proves whether the runtime
// operands actually take that route. Metamethod and error alternatives never
// enter this rule.
func NewOperand(source *link.Link, algebra *numeric.Algebra, shard linkproject.Shard, binary keyspace.Term) (Operand, bool) {
	if source == nil || algebra == nil || !algebra.Valid() || algebra.Link() != source || source.ContentID() != algebra.LinkID() {
		return Operand{}, false
	}
	project := source.Project()
	if project == nil {
		return Operand{}, false
	}
	mounts := project.Mounts()
	shardIndex, shardOK := mounts.Index(shard)
	if !shardOK || uint64(shardIndex+1) > uint64(^uint32(0)) {
		return Operand{}, false
	}
	p, present := mounts.Program(shard)
	if !present || p == nil {
		return Operand{}, false
	}
	primitives := p.Flow().BinaryPrimitives()
	if !primitives.Available() {
		return Operand{}, false
	}
	primitive, retained := primitives.Primitive(binary)
	operation, direct := primitive.Operation()
	resultTerm, sourceOK := primitive.Source()
	if !retained || !direct || !sourceOK || resultTerm != binary || !p.Flow().Executable().Contains(binary) || !numericOperator(operation.Op) ||
		keyspace.TermFamily(operation.Owner) != keyspace.FamilyBody || keyspace.TermOrdinal(operation.Owner) == 0 ||
		operation.Left == 0 || operation.Right == 0 || !p.Flow().Executable().Contains(operation.Owner) {
		return Operand{}, false
	}
	root, rooted := algebra.RootFor(shard, operation.Owner)
	resultScalar, resultScalarOK := algebra.ScalarFor(shard, operation.Owner, resultTerm)
	leftScalar, leftScalarOK := algebra.ScalarFor(shard, operation.Owner, operation.Left)
	rightScalar, rightScalarOK := algebra.ScalarFor(shard, operation.Owner, operation.Right)
	key, keyed := algebra.KeyFor(root)
	result, resultOK := algebra.AtomFor(resultScalar)
	left, leftOK := algebra.AtomFor(leftScalar)
	right, rightOK := algebra.AtomFor(rightScalar)
	linkID := source.ContentID()
	if !rooted || !resultScalarOK || !leftScalarOK || !rightScalarOK || !keyed ||
		!resultOK || !leftOK || !rightOK || !linkID.Available() {
		return Operand{}, false
	}
	return Operand{
		source: source, algebra: algebra, shard: shard, binary: binary, owner: operation.Owner,
		leftTerm: operation.Left, rightTerm: operation.Right, key: key, result: result, left: left, right: right,
		op: operation.Op, content: operandContentID(linkID, uint32(shardIndex+1), binary),
	}, true
}

func (operand Operand) valid() bool {
	if operand.source == nil || operand.algebra == nil || !operand.key.Valid() || !operand.result.Valid() || !operand.left.Valid() || !operand.right.Valid() ||
		operand.content == [32]byte{} || operand.algebra.Link() != operand.source || operand.source.ContentID() != operand.algebra.LinkID() {
		return false
	}
	expected, ok := NewOperand(operand.source, operand.algebra, operand.shard, operand.binary)
	return ok && expected.key == operand.key && expected.result == operand.result && expected.left == operand.left && expected.right == operand.right &&
		expected.owner == operand.owner && expected.leftTerm == operand.leftTerm && expected.rightTerm == operand.rightTerm &&
		expected.op == operand.op && expected.content == operand.content
}

func numericOperator(op kind.BinaryOp) bool {
	switch op {
	case kind.BinaryAdd, kind.BinarySub, kind.BinaryMul, kind.BinaryDiv,
		kind.BinaryIDiv, kind.BinaryMod, kind.BinaryPow,
		kind.BinaryBitAnd, kind.BinaryBitOr, kind.BinaryBitXor,
		kind.BinaryShiftLeft, kind.BinaryShiftRight:
		return true
	default:
		return false
	}
}

func bitwiseOperator(op kind.BinaryOp) bool {
	switch op {
	case kind.BinaryBitAnd, kind.BinaryBitOr, kind.BinaryBitXor, kind.BinaryShiftLeft, kind.BinaryShiftRight:
		return true
	default:
		return false
	}
}

func integerResultOperator(op kind.BinaryOp) bool {
	switch op {
	case kind.BinaryAdd, kind.BinarySub, kind.BinaryMul, kind.BinaryIDiv, kind.BinaryMod:
		return true
	default:
		return bitwiseOperator(op)
	}
}

func operandContentID(linkID keyspace.ContentID, shardOrdinal uint32, binaryTerm keyspace.Term) [32]byte {
	var payload [64]byte
	copy(payload[:32], linkID[:])
	copy(payload[32:40], []byte("num-op!!"))
	binary.BigEndian.PutUint64(payload[40:48], uint64(shardOrdinal))
	binary.BigEndian.PutUint64(payload[48:56], uint64(binaryTerm))
	return sha256.Sum256(payload[:])
}

// Rule owns the one-input Numeric representation transfer.  Its one input
// and output are the same canonical computation root: no second result
// carrier, ad-hoc path, or cross-factor selector is introduced.
type Rule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[numeric.Value, Operand]
	owner    *numericowner.Owner
	read     engine.Read[engine.OrderedCells[numeric.Value]]
	write    engine.Write[numeric.Value]
}

func Declare(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *numericowner.Owner) (*Rule, bool) {
	if composition == nil || owner == nil || owner.Algebra() == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence {
		return nil, false
	}
	declaration := &Rule{semantic: semantic, owner: owner}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[numeric.Value, Operand]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: content, Output: owner.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[numeric.Value, Operand]) bool {
		input, inputOK := rule.InputAt(0)
		read, readOK := engine.ReadFrom(rule, input, owner.ExactRead())
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if inputOK && readOK && writeOK {
			declaration.rule, declaration.read, declaration.write = rule, read, write
		}
		return inputOK && readOK && writeOK
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *Rule) Instance(operand Operand) (*engine.RuleInstance[numeric.Value, Operand], bool) {
	if rule == nil || rule.rule == nil || !validOperand(rule.owner, operand) {
		return nil, false
	}
	ref, ok := rule.owner.Locate(operand.key)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[numeric.Value, Operand]) bool {
		return engine.InstanceRead(binding, rule.read, ref) && engine.InstanceWrite(binding, rule.write, ref)
	})
}

func content(operand Operand) (Operand, [32]byte, bool) {
	return operand, operand.content, operand.valid()
}

func validOperand(owner *numericowner.Owner, operand Operand) bool {
	if owner == nil || owner.Algebra() == nil || operand.algebra != owner.Algebra() {
		return false
	}
	expected, ok := NewOperand(operand.source, owner.Algebra(), operand.shard, operand.binary)
	return ok && expected.key == operand.key && expected.result == operand.result && expected.left == operand.left && expected.right == operand.right &&
		expected.owner == operand.owner && expected.leftTerm == operand.leftTerm && expected.rightTerm == operand.rightTerm &&
		expected.op == operand.op && expected.content == operand.content
}

const numericResult = numeric.MayInteger | numeric.MayFiniteFloat | numeric.MayInfinity | numeric.MayNaN
const floatResult = numeric.MayFiniteFloat | numeric.MayInfinity | numeric.MayNaN

func definitelyNumeric(mask numeric.Eligibility) bool {
	return mask.Valid() && mask&numeric.MayOther == 0
}

// result derives only representation facts that follow from Lua's primitive
// numeric semantics.  In particular it does not manufacture an affine
// equality for wrapping integer arithmetic, floating rounding, division,
// modulo, or power.  Such an equality would be false even though the result's
// representation is known exactly enough for the carrier.
func result(algebra *numeric.Algebra, operand Operand, current numeric.Value) (numeric.Value, bool) {
	if algebra == nil || !validLocalOperand(algebra, operand) || !algebra.Admits(operand.key, current) {
		return numeric.Value{}, false
	}
	// This is the only exact arithmetic equation this child emits.  Both
	// operands are Link-authored integer literals, so the operation's concrete
	// result and the absence of signed overflow are decided cold before any
	// solver row exists.  For every other operation a translation would be
	// unsound: a dynamic integer may wrap, and floats round.
	if value, exact := staticIntegerTranslation(algebra, operand); exact {
		return value, true
	}
	left, leftOK := current.Eligibility(operand.left)
	right, rightOK := current.Eligibility(operand.right)
	if !leftOK || !rightOK {
		return numeric.Value{}, false
	}
	if bitwiseOperator(operand.op) {
		if !left.MustInteger() || !right.MustInteger() {
			return numeric.Value{}, false
		}
		return algebra.EligibilityAt(operand.key, operand.result, numeric.MayInteger)
	}
	if !definitelyNumeric(left) || !definitelyNumeric(right) {
		return numeric.Value{}, false
	}
	if operand.op == kind.BinaryDiv || operand.op == kind.BinaryPow {
		return algebra.EligibilityAt(operand.key, operand.result, floatResult)
	}
	if integerResultOperator(operand.op) && left.MustInteger() && right.MustInteger() {
		return algebra.EligibilityAt(operand.key, operand.result, numeric.MayInteger)
	}
	return algebra.EligibilityAt(operand.key, operand.result, numericResult)
}

// Result replays the existing Numeric primitive-result judgment for a
// projection owner. It is intentionally the same operation Rule reduction,
// not a second arithmetic implementation: consumers must first bind the
// exact Numeric read and then ask this package whether that fact is the
// admitted result representation.
func Result(algebra *numeric.Algebra, operand Operand, current numeric.Value) (numeric.Value, bool) {
	return result(algebra, operand, current)
}

func staticIntegerTranslation(algebra *numeric.Algebra, operand Operand) (numeric.Value, bool) {
	if algebra == nil || operand.source == nil || operand.op != kind.BinaryAdd && operand.op != kind.BinarySub {
		return numeric.Value{}, false
	}
	project := operand.source.Project()
	if project == nil {
		return numeric.Value{}, false
	}
	mounts := project.Mounts()
	if _, shardOK := mounts.Index(operand.shard); !shardOK {
		return numeric.Value{}, false
	}
	p, ok := mounts.Program(operand.shard)
	if !ok || p == nil {
		return numeric.Value{}, false
	}
	primitives := p.Flow().BinaryPrimitives()
	primitive, ok := primitives.Primitive(operand.binary)
	operation, operationOK := primitive.Operation()
	if !primitives.Available() || !ok || !operationOK || operation.Op != operand.op || operation.Owner != operand.owner ||
		operation.Left != operand.leftTerm || operation.Right != operand.rightTerm {
		return numeric.Value{}, false
	}
	left, leftLiteral := sourceIntegerLiteral(p, operand.owner, operand.leftTerm)
	right, rightLiteral := sourceIntegerLiteral(p, operand.owner, operand.rightTerm)
	if !leftLiteral || !rightLiteral {
		return numeric.Value{}, false
	}
	resultScalar, resultOK := algebra.ScalarFor(operand.shard, operand.owner, operand.binary)
	leftScalar, leftOK := algebra.ScalarFor(operand.shard, operand.owner, operand.leftTerm)
	if !resultOK || !leftOK {
		return numeric.Value{}, false
	}
	var delta int64
	var arithmeticOK bool
	switch operand.op {
	case kind.BinaryAdd:
		_, arithmeticOK = checkedAdd(left, right)
		delta = right
	case kind.BinarySub:
		_, arithmeticOK = checkedSub(left, right)
		if right == math.MinInt64 {
			return numeric.Value{}, false
		}
		delta = -right
	default:
		return numeric.Value{}, false
	}
	if !arithmeticOK {
		// Lua wraps the overflowing integer operation.  The finite difference
		// carrier does not model that modular equation, so its general
		// representation-only conclusion remains the precise safe result.
		return numeric.Value{}, false
	}
	resultAtom, resultAtomOK := algebra.AtomFor(resultScalar)
	leftAtom, leftAtomOK := algebra.AtomFor(leftScalar)
	if !resultAtomOK || !leftAtomOK || resultAtom != operand.result || leftAtom != operand.left {
		return numeric.Value{}, false
	}
	return algebra.IntegerTranslation(operand.key, operand.result, operand.left, delta)
}

// sourceIntegerLiteral validates the exact Source literal occurrence before
// its value is used. The term and lexical owner are both checked against the
// returned typed row; no ordinal is trusted as a value lookup.
func sourceIntegerLiteral(p *program.Program, body, term keyspace.Term) (int64, bool) {
	if p == nil || body == 0 || term == 0 {
		return 0, false
	}
	if keyspace.TermFamily(term) != keyspace.FamilyInteger || keyspace.TermOrdinal(term) == 0 {
		return 0, false
	}
	integers := p.Source().Literals().Integers()
	returned, owner, value, ok := integers.At(int(keyspace.TermOrdinal(term) - 1))
	return value, ok && returned == term && owner == body
}

func checkedAdd(left, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right || right < 0 && left < math.MinInt64-right {
		return 0, false
	}
	return left + right, true
}

func checkedSub(left, right int64) (int64, bool) {
	if right > 0 && left < math.MinInt64+right || right < 0 && left > math.MaxInt64+right {
		return 0, false
	}
	return left - right, true
}

func validLocalOperand(algebra *numeric.Algebra, operand Operand) bool {
	return algebra != nil && operand.algebra == algebra && operand.valid()
}

func (rule *Rule) transfer(access engine.Access[numeric.Value, Operand]) bool {
	operand, ok := engine.Operand(access)
	if !ok || !validOperand(rule.owner, operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, readOK := engine.ReadValue(access, row, rule.read)
		if !readOK || cells.Count() != 1 {
			return false
		}
		current, present, cellOK := cells.At(0)
		if !cellOK {
			return false
		}
		if !present {
			return engine.NoCandidate(access, row)
		}
		value, reduced := result(rule.owner.Algebra(), operand, current)
		if !reduced {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, value)
	})
}

func (rule *Rule) check(derivation engine.RuleDerivation[numeric.Value, Operand]) (engine.RuleEvidence, bool) {
	if rule == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, ok := derivation.Operand()
	input, inputOK := derivation.InputAt(0)
	ref, refOK := rule.owner.Locate(operand.key)
	if !ok || !inputOK || input.Guard().Empty() || !validOperand(rule.owner, operand) || !derivation.OperandContentMatches(operand.content) || !refOK ||
		!engine.DerivationReadMatchesRef(derivation, rule.read, ref) {
		return engine.RuleEvidence{}, false
	}
	disposition, dispositionOK := derivation.DispositionAt(0)
	if !dispositionOK || disposition.Guard().Empty() || !disposition.Guard().Same(input.Guard()) {
		return engine.RuleEvidence{}, false
	}
	cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.read)
	if !cellsOK || cells.Count() != 1 {
		return engine.RuleEvidence{}, false
	}
	current, present, cellOK := cells.At(0)
	if !cellOK {
		return engine.RuleEvidence{}, false
	}
	expected, reduced := result(rule.owner.Algebra(), operand, current)
	if !present || !reduced {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !targetOK || !actualOK ||
		!engine.TargetMatchesRef(target, ref) || !rule.owner.Algebra().Equal(actual, expected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

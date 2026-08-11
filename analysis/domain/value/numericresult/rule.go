// Package numericresult declares Value's primitive numeric-result projection.
//
// The Rule is deliberately narrow: Flow must have retained an executable
// arithmetic or bitwise BinaryPrimitive, Numeric must have issued the exact
// operation operand for that primitive, and Value supplies only the existing
// result coordinate.  Comparisons, concatenation, and metamethod routes never
// enter this package.
package numericresult

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/domain/numeric"
	numericoperation "github.com/wippyai/go-lua/analysis/domain/numeric/operation"
	numericowner "github.com/wippyai/go-lua/analysis/domain/numeric/owner"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// Operand is the owner-fenced identity of one primitive Numeric result.  The
// Numeric operation operand is retained intact; Value adds only the existing
// Value coordinates and the exact Source cursor of the Binary occurrence.
// No coordinate is constructed by this type.
type Operand struct {
	schema  *value.Schema
	source  *link.Link
	algebra *numeric.Algebra
	shard   linkproject.Shard

	binary keyspace.Term
	body   keyspace.Term
	offset int
	cursor int

	leftTerm, rightTerm keyspace.Term
	left, right         value.Coordinate
	result              value.Coordinate

	numericOperand numericoperation.Operand
	numericKey     numeric.Key
	content        keyspace.ContentID
}

// NewOperand admits one existing executable arithmetic/bitwise primitive and
// its exact Numeric operation operand.  The source position and all three
// Value coordinates are replayed from the same mounted Program/Link, so a
// result cannot be paired with a foreign or differently positioned operation.
func NewOperand(schema *value.Schema, owner *numericowner.Owner, shard linkproject.Shard, binaryTerm keyspace.Term) (Operand, bool) {
	if schema == nil || owner == nil || owner.Algebra() == nil || schema.Link() == nil || owner.Link() != schema.Link() {
		return Operand{}, false
	}
	source := schema.Link()
	algebra := owner.Algebra()
	return newOperand(schema, source, algebra, shard, binaryTerm)
}

func newOperand(schema *value.Schema, source *link.Link, algebra *numeric.Algebra, shard linkproject.Shard, binaryTerm keyspace.Term) (Operand, bool) {
	if schema == nil || source == nil || algebra == nil || schema.Link() != source || algebra.Link() != source {
		return Operand{}, false
	}
	numericOperand, numericOK := numericoperation.NewOperand(source, algebra, shard, binaryTerm)
	if !numericOK {
		return Operand{}, false
	}
	project := source.Project()
	if project == nil {
		return Operand{}, false
	}
	program, programOK := project.Mounts().Program(shard)
	if !programOK || program == nil {
		return Operand{}, false
	}
	primitives := program.Flow().BinaryPrimitives()
	primitive, primitiveOK := primitives.Primitive(binaryTerm)
	operation, operationOK := primitive.Operation()
	resultTerm, resultOK := primitive.Source()
	if !primitives.Available() || !primitiveOK || !operationOK || !resultOK || resultTerm != binaryTerm ||
		!isPrimitiveNumericOperator(operation.Op) || operation.Owner == 0 || operation.Left == 0 || operation.Right == 0 ||
		!program.Flow().Executable().Contains(binaryTerm) || !program.Flow().Executable().Contains(operation.Owner) {
		return Operand{}, false
	}
	body, offset, cursor, positioned := program.Source().Index().Position(binaryTerm)
	if !positioned || body != operation.Owner || offset < 0 || cursor < 0 {
		return Operand{}, false
	}
	if !sameProgramPosition(program, operation.Owner, operation.Left) || !sameProgramPosition(program, operation.Owner, operation.Right) {
		return Operand{}, false
	}

	resultCoordinate, resultCoordinateOK := valueCoordinate(source, schema, shard, binaryTerm)
	leftCoordinate, leftCoordinateOK := valueCoordinate(source, schema, shard, operation.Left)
	rightCoordinate, rightCoordinateOK := valueCoordinate(source, schema, shard, operation.Right)
	numericKey, numericKeyOK := numericOperand.Key()
	shardIndex, shardIndexOK := project.Mounts().Index(shard)
	content := resultContent(source.ContentID(), uint32(shardIndex+1), binaryTerm, operation.Owner, offset, cursor)
	if !resultCoordinateOK || !leftCoordinateOK || !rightCoordinateOK || !numericKeyOK || !shardIndexOK || !content.Available() {
		return Operand{}, false
	}
	return Operand{
		schema: schema, source: source, algebra: algebra, shard: shard,
		binary: binaryTerm, body: operation.Owner, offset: offset, cursor: cursor,
		leftTerm: operation.Left, rightTerm: operation.Right,
		left: leftCoordinate, right: rightCoordinate, result: resultCoordinate,
		numericOperand: numericOperand, numericKey: numericKey, content: content,
	}, true
}

func valueCoordinate(source *link.Link, schema *value.Schema, shard linkproject.Shard, term keyspace.Term) (value.Coordinate, bool) {
	if source == nil || schema == nil || term == 0 {
		return value.Coordinate{}, false
	}
	boundary := source.Boundary()
	if boundary == nil {
		return value.Coordinate{}, false
	}
	raw, ok := boundary.Values().Of(shard, term)
	if !ok {
		return value.Coordinate{}, false
	}
	coordinate, ok := schema.CoordinateFor(raw)
	return coordinate, ok && coordinate.Valid()
}

func isPrimitiveNumericOperator(op flowkind.BinaryOp) bool {
	switch op {
	case flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul, flowkind.BinaryDiv,
		flowkind.BinaryIDiv, flowkind.BinaryMod, flowkind.BinaryPow,
		flowkind.BinaryBitAnd, flowkind.BinaryBitOr, flowkind.BinaryBitXor,
		flowkind.BinaryShiftLeft, flowkind.BinaryShiftRight:
		return true
	default:
		return false
	}
}

func resultContent(linkID keyspace.ContentID, shard uint32, binaryTerm, body keyspace.Term, offset, cursor int) keyspace.ContentID {
	var payload [80]byte
	copy(payload[:32], linkID[:])
	copy(payload[32:40], []byte("val-num!"))
	binary.BigEndian.PutUint64(payload[40:48], uint64(shard))
	binary.BigEndian.PutUint64(payload[48:56], uint64(binaryTerm))
	binary.BigEndian.PutUint64(payload[56:64], uint64(body))
	binary.BigEndian.PutUint64(payload[64:72], uint64(offset))
	binary.BigEndian.PutUint64(payload[72:80], uint64(cursor))
	return sha256.Sum256(payload[:])
}

// ID returns this Value-family occurrence identity.
func (operand Operand) ID() (keyspace.ContentID, bool) {
	if !operand.valid() {
		return keyspace.ContentID{}, false
	}
	return operand.content, true
}

// Binary returns the exact retained executable Binary term.
func (operand Operand) Binary() (keyspace.Term, bool) {
	if !operand.valid() {
		return 0, false
	}
	return operand.binary, true
}

// NumericKey returns the Numeric root read by the projection.
func (operand Operand) NumericKey() (numeric.Key, bool) {
	if !operand.valid() {
		return numeric.Key{}, false
	}
	return operand.numericKey, true
}

// Coordinates returns the existing Value input/result coordinates associated
// with the primitive's ordered source operands.
func (operand Operand) Coordinates() (result, left, right value.Coordinate, ok bool) {
	if !operand.valid() {
		return value.Coordinate{}, value.Coordinate{}, value.Coordinate{}, false
	}
	return operand.result, operand.left, operand.right, true
}

// SourcePosition returns the exact body/offset/cursor retained for Binary.
func (operand Operand) SourcePosition() (body keyspace.Term, offset, cursor int, ok bool) {
	if !operand.valid() {
		return 0, 0, 0, false
	}
	return operand.body, operand.offset, operand.cursor, true
}

func (operand Operand) valid() bool {
	if operand.source == nil || operand.algebra == nil || operand.source != operand.algebra.Link() ||
		operand.schema == nil || operand.schema.Link() != operand.source ||
		!operand.content.Available() || !operand.numericKey.Valid() || !operand.result.Valid() ||
		!operand.left.Valid() || !operand.right.Valid() || operand.body == 0 || operand.binary == 0 ||
		operand.offset < 0 || operand.cursor < 0 {
		return false
	}
	expected, ok := newOperand(operand.schema, operand.source, operand.algebra, operand.shard, operand.binary)
	return ok && expected.body == operand.body && expected.offset == operand.offset && expected.cursor == operand.cursor &&
		expected.leftTerm == operand.leftTerm && expected.rightTerm == operand.rightTerm && expected.left == operand.left &&
		expected.right == operand.right && expected.result == operand.result && expected.numericKey == operand.numericKey &&
		expected.content == operand.content
}

func sameProgramPosition(program *program.Program, body, term keyspace.Term) bool {
	if program == nil || body == 0 || term == 0 {
		return false
	}
	positionedBody, offset, cursor, ok := program.Source().Index().Position(term)
	return ok && positionedBody == body && offset >= 0 && cursor >= 0 && program.Flow().Executable().Contains(term)
}

// Rule is the one Value-owned Numeric→Value projection Rule.
type Rule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[value.Value, Operand]
	values   *valueowner.Owner
	numerics *numericowner.Owner
	read     engine.Read[engine.OrderedCells[numeric.Value]]
	write    engine.Write[value.Value]
}

func Declare(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, values *valueowner.Owner, numerics *numericowner.Owner) (*Rule, bool) {
	if composition == nil || values == nil || numerics == nil || values.Schema() == nil || numerics.Algebra() == nil ||
		values.Schema().Link() == nil || numerics.Link() != values.Schema().Link() ||
		!semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence {
		return nil, false
	}
	declaration := &Rule{semantic: semantic, values: values, numerics: numerics}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[value.Value, Operand]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: operandContent,
		Output: values.Output(), Inputs: 2,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[value.Value, Operand]) bool {
		valueInput, valueInputOK := rule.InputAt(0)
		numericInput, numericInputOK := rule.InputAt(1)
		read, readOK := engine.ReadFrom(rule, numericInput, numerics.ExactRead())
		carryOK := engine.CarryFrom(rule, valueInput, values.Carry())
		write, writeOK := engine.WriteTo(rule, values.ExactWrite())
		if !valueInputOK || !numericInputOK || !readOK || !carryOK || !writeOK {
			return false
		}
		declaration.rule, declaration.read, declaration.write = rule, read, write
		return true
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

func (rule *Rule) Instance(operand Operand) (*engine.RuleInstance[value.Value, Operand], bool) {
	if rule == nil || rule.rule == nil || rule.values == nil || rule.numerics == nil || !validOperand(rule.values, rule.numerics, operand) {
		return nil, false
	}
	key, keyOK := operand.NumericKey()
	result, _, _, resultOK := operand.Coordinates()
	numericRef, numericRefOK := rule.numerics.Locate(key)
	valueRef, valueRefOK := rule.values.Locate(result)
	if !keyOK || !resultOK || !numericRefOK || !valueRefOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[value.Value, Operand]) bool {
		return engine.InstanceRead(binding, rule.read, numericRef) && engine.InstanceWrite(binding, rule.write, valueRef)
	})
}

func operandContent(operand Operand) (Operand, [32]byte, bool) {
	id, ok := operand.ID()
	return operand, [32]byte(id), ok && id.Available()
}

func validOperand(values *valueowner.Owner, numerics *numericowner.Owner, operand Operand) bool {
	if values == nil || values.Schema() == nil || numerics == nil || numerics.Algebra() == nil || operand.source == nil ||
		values.Schema().Link() != operand.source || numerics.Link() != operand.source || operand.algebra != numerics.Algebra() {
		return false
	}
	expected, ok := NewOperand(values.Schema(), numerics, operand.shard, operand.binary)
	return ok && expected.body == operand.body && expected.offset == operand.offset && expected.cursor == operand.cursor &&
		expected.leftTerm == operand.leftTerm && expected.rightTerm == operand.rightTerm && expected.left == operand.left &&
		expected.right == operand.right && expected.result == operand.result && expected.numericKey == operand.numericKey &&
		expected.content == operand.content
}

func (rule *Rule) transfer(access engine.Access[value.Value, Operand]) bool {
	operand, ok := engine.Operand(access)
	if !ok || !validOperand(rule.values, rule.numerics, operand) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, readOK := engine.ReadValue(access, row, rule.read)
		if !readOK || cells.Count() != 1 {
			return false
		}
		current, present, available := cells.At(0)
		if !available {
			return false
		}
		if !present {
			return engine.NoCandidate(access, row)
		}
		result, reduced := numericoperation.Result(rule.numerics.Algebra(), operand.numericOperand, current)
		if !reduced {
			return engine.NoCandidate(access, row)
		}
		projected, projectedOK := rule.values.Schema().ForRuntimeKinds(runtimekind.Bit(runtimekind.Number))
		if !projectedOK {
			return false
		}
		_ = result // Numeric result admission is the proof; Value stores its Number abstraction.
		return engine.StageValue(access, row, projected)
	})
}

func (rule *Rule) check(derivation engine.RuleDerivation[value.Value, Operand]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.values == nil || rule.numerics == nil || derivation.Rule() != rule.semantic ||
		derivation.InputCount() != 2 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	if !operandOK || !validOperand(rule.values, rule.numerics, operand) {
		return engine.RuleEvidence{}, false
	}
	id, idOK := operand.ID()
	key, keyOK := operand.NumericKey()
	resultCoordinate, _, _, coordinatesOK := operand.Coordinates()
	numericRef, numericRefOK := rule.numerics.Locate(key)
	valueRef, valueRefOK := rule.values.Locate(resultCoordinate)
	if !idOK || !derivation.OperandContentMatches([32]byte(id)) || !keyOK || !coordinatesOK || !numericRefOK || !valueRefOK ||
		!engine.DerivationReadMatchesRef(derivation, rule.read, numericRef) {
		return engine.RuleEvidence{}, false
	}
	for index := 0; index < derivation.InputCount(); index++ {
		input, inputOK := derivation.InputAt(index)
		if !inputOK || input.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
	}
	disposition, dispositionOK := derivation.DispositionAt(0)
	if !dispositionOK || disposition.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	if _, transformed := disposition.CarryTransform(); transformed || disposition.TransformOnly() {
		return engine.RuleEvidence{}, false
	}
	cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.read)
	if !cellsOK || cells.Count() != 1 {
		return engine.RuleEvidence{}, false
	}
	current, present, available := cells.At(0)
	if !available {
		return engine.RuleEvidence{}, false
	}
	if !present {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	_, reduced := numericoperation.Result(rule.numerics.Algebra(), operand.numericOperand, current)
	projected, projectedOK := rule.values.Schema().ForRuntimeKinds(runtimekind.Bit(runtimekind.Number))
	target, targetOK := disposition.TargetAt(0)
	actual, actualOK := disposition.Value()
	if !reduced || !projectedOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 ||
		!targetOK || !actualOK || !engine.TargetMatchesRef(target, valueRef) || !rule.values.Schema().Equal(actual, projected) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

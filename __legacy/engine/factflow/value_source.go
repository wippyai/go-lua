package factflow

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ValueSourceKind classifies where a value-list slot comes from.
type ValueSourceKind uint8

const (
	ValueSourceUnknown ValueSourceKind = iota
	ValueSourceExpression
	ValueSourceCall
	ValueSourceVararg
	ValueSourceNil
	ValueSourcePath
	ValueSourceLiteral
)

// ValueSourceLiteralKind classifies scalar literal sources that do not need an
// expression ref to resolve to a product value.
type ValueSourceLiteralKind uint8

const (
	ValueSourceLiteralInvalid ValueSourceLiteralKind = iota
	ValueSourceLiteralBool
	ValueSourceLiteralInteger
	ValueSourceLiteralNumber
	ValueSourceLiteralString
)

// NoValueSourceIndex marks an index field that does not point at a source,
// target, or result slot.
const NoValueSourceIndex = -1

// ValueSourceShape describes value-list shape flags shared by source kinds.
type ValueSourceShape struct {
	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

// NewValueSourceShape creates a validated value-list shape.
func NewValueSourceShape(final, expanded, adjusted, openTail bool) (ValueSourceShape, bool) {
	shape := ValueSourceShape{
		Final:    final,
		Expanded: expanded,
		Adjusted: adjusted,
		OpenTail: openTail,
	}
	return shape, shape.Valid()
}

// Valid reports whether the shape flags form a supported value-list shape.
func (s ValueSourceShape) Valid() bool {
	if s.Expanded && s.Adjusted {
		return false
	}
	if s.Expanded && !s.Final {
		return false
	}
	if s.OpenTail && (!s.Expanded || !s.Final) {
		return false
	}
	return true
}

// ValueSource describes one value-list slot without retaining source AST.
type ValueSource struct {
	Kind ValueSourceKind

	ExprRef ExprRef
	HasExpr bool

	SourcePoint    cfg.Point
	HasSourcePoint bool

	ExprIndex   int
	TargetIndex int
	ResultIndex int

	CallPoint    cfg.Point
	HasCallPoint bool

	PathKey pathdom.PathKey

	LiteralKind ValueSourceLiteralKind
	Bool        bool
	Int         int64
	Float       float64
	String      string

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

// NewExpressionValueSource creates a value source backed by an expression ref.
func NewExpressionValueSource(expr ExprRef, exprIndex, targetIndex, resultIndex int, shape ValueSourceShape) (ValueSource, bool) {
	source := valueSourceWithShape(ValueSource{
		Kind:        ValueSourceExpression,
		ExprRef:     expr,
		HasExpr:     expr != 0,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
	}, shape)
	return source, source.Valid()
}

// WithSourcePoint records the CFG point that produced an expression-backed
// source. Consumers that resolve a source after later side effects can use this
// point to read the value under the same solved facts that held when the
// expression was evaluated.
func (s ValueSource) WithSourcePoint(point cfg.Point) ValueSource {
	s.SourcePoint = point
	s.HasSourcePoint = point != 0
	return s
}

// NewCallValueSource creates a value source backed by a call result.
func NewCallValueSource(expr ExprRef, exprIndex, targetIndex, resultIndex int, callPoint cfg.Point, shape ValueSourceShape) (ValueSource, bool) {
	source := valueSourceWithShape(ValueSource{
		Kind:         ValueSourceCall,
		ExprRef:      expr,
		HasExpr:      expr != 0,
		ExprIndex:    exprIndex,
		TargetIndex:  targetIndex,
		ResultIndex:  resultIndex,
		CallPoint:    callPoint,
		HasCallPoint: callPoint != 0,
	}, shape)
	return source, source.Valid()
}

// NewVarargValueSource creates a value source backed by a vararg expression.
// Boundary projections may omit expr when the AST source has no factflow ref.
func NewVarargValueSource(expr ExprRef, exprIndex, targetIndex, resultIndex int, shape ValueSourceShape) (ValueSource, bool) {
	source := valueSourceWithShape(ValueSource{
		Kind:        ValueSourceVararg,
		ExprRef:     expr,
		HasExpr:     expr != 0,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
	}, shape)
	return source, source.Valid()
}

// NewPathValueSource creates a value source backed by a point-local path key.
func NewPathValueSource(pathKey pathdom.PathKey, exprIndex, targetIndex, resultIndex int, shape ValueSourceShape) (ValueSource, bool) {
	source := valueSourceWithShape(ValueSource{
		Kind:        ValueSourcePath,
		PathKey:     pathKey,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
	}, shape)
	return source, source.Valid()
}

// NewBoolLiteralValueSource creates a value source backed by a boolean literal.
func NewBoolLiteralValueSource(value bool, exprIndex, targetIndex, resultIndex int, shape ValueSourceShape) (ValueSource, bool) {
	source := valueSourceWithShape(ValueSource{
		Kind:        ValueSourceLiteral,
		LiteralKind: ValueSourceLiteralBool,
		Bool:        value,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
	}, shape)
	return source, source.Valid()
}

// NewIntegerLiteralValueSource creates a value source backed by an integer
// literal.
func NewIntegerLiteralValueSource(value int64, exprIndex, targetIndex, resultIndex int, shape ValueSourceShape) (ValueSource, bool) {
	source := valueSourceWithShape(ValueSource{
		Kind:        ValueSourceLiteral,
		LiteralKind: ValueSourceLiteralInteger,
		Int:         value,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
	}, shape)
	return source, source.Valid()
}

// NewNumberLiteralValueSource creates a value source backed by a floating-point
// numeric literal.
func NewNumberLiteralValueSource(value float64, exprIndex, targetIndex, resultIndex int, shape ValueSourceShape) (ValueSource, bool) {
	source := valueSourceWithShape(ValueSource{
		Kind:        ValueSourceLiteral,
		LiteralKind: ValueSourceLiteralNumber,
		Float:       value,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
	}, shape)
	return source, source.Valid()
}

// NewStringLiteralValueSource creates a value source backed by a string literal.
func NewStringLiteralValueSource(value string, exprIndex, targetIndex, resultIndex int, shape ValueSourceShape) (ValueSource, bool) {
	source := valueSourceWithShape(ValueSource{
		Kind:        ValueSourceLiteral,
		LiteralKind: ValueSourceLiteralString,
		String:      value,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
	}, shape)
	return source, source.Valid()
}

// NewNilValueSource creates a nil-fill value source for a target slot.
func NewNilValueSource(targetIndex int) ValueSource {
	return ValueSource{
		Kind:        ValueSourceNil,
		ExprIndex:   NoValueSourceIndex,
		TargetIndex: targetIndex,
		ResultIndex: NoValueSourceIndex,
	}
}

// NewUnknownValueSource creates an unknown value source for a target slot.
func NewUnknownValueSource(targetIndex int) ValueSource {
	return ValueSource{
		Kind:        ValueSourceUnknown,
		ExprIndex:   NoValueSourceIndex,
		TargetIndex: targetIndex,
		ResultIndex: NoValueSourceIndex,
	}
}

// Valid reports whether the source satisfies the factflow source invariants.
func (s ValueSource) Valid() bool {
	if !s.shape().Valid() {
		return false
	}
	if !s.exprFlagsValid() || !s.callFlagsValid() || !s.sourcePointFlagsValid() {
		return false
	}
	switch s.Kind {
	case ValueSourceUnknown:
		return !s.HasExpr && s.ExprRef == 0 &&
			!s.HasCallPoint && s.CallPoint == 0 &&
			s.PathKey == "" &&
			s.LiteralKind == ValueSourceLiteralInvalid
	case ValueSourceExpression:
		return s.HasExpr && s.ExprRef != 0 &&
			!s.HasCallPoint && s.CallPoint == 0 &&
			s.PathKey == "" &&
			s.LiteralKind == ValueSourceLiteralInvalid
	case ValueSourceCall:
		return s.HasCallPoint && s.CallPoint != 0 &&
			s.ResultIndex >= 0 &&
			s.PathKey == "" &&
			s.LiteralKind == ValueSourceLiteralInvalid
	case ValueSourceVararg:
		return !s.HasCallPoint && s.CallPoint == 0 && s.PathKey == "" && s.LiteralKind == ValueSourceLiteralInvalid
	case ValueSourceNil:
		return s.ExprIndex == NoValueSourceIndex &&
			s.ResultIndex == NoValueSourceIndex &&
			!s.HasExpr &&
			s.ExprRef == 0 &&
			!s.HasCallPoint &&
			s.CallPoint == 0 &&
			s.PathKey == "" &&
			s.LiteralKind == ValueSourceLiteralInvalid &&
			!s.Final &&
			!s.Expanded &&
			!s.Adjusted &&
			!s.OpenTail
	case ValueSourcePath:
		return !s.HasExpr && s.ExprRef == 0 &&
			!s.HasCallPoint && s.CallPoint == 0 &&
			s.PathKey != "" &&
			s.LiteralKind == ValueSourceLiteralInvalid
	case ValueSourceLiteral:
		return !s.HasExpr && s.ExprRef == 0 &&
			!s.HasCallPoint && s.CallPoint == 0 &&
			s.PathKey == "" &&
			s.literalValid()
	default:
		return false
	}
}

func (s ValueSource) literalValid() bool {
	switch s.LiteralKind {
	case ValueSourceLiteralBool:
		return s.Int == 0 && s.Float == 0 && s.String == ""
	case ValueSourceLiteralInteger:
		return !s.Bool && s.Float == 0 && s.String == ""
	case ValueSourceLiteralNumber:
		return !s.Bool && s.Int == 0 && s.String == ""
	case ValueSourceLiteralString:
		return s.Int == 0 && s.Float == 0 && !s.Bool
	default:
		return false
	}
}

func valueSourceWithShape(source ValueSource, shape ValueSourceShape) ValueSource {
	source.Final = shape.Final
	source.Expanded = shape.Expanded
	source.Adjusted = shape.Adjusted
	source.OpenTail = shape.OpenTail
	return source
}

func (s ValueSource) shape() ValueSourceShape {
	return ValueSourceShape{
		Final:    s.Final,
		Expanded: s.Expanded,
		Adjusted: s.Adjusted,
		OpenTail: s.OpenTail,
	}
}

func (s ValueSource) exprFlagsValid() bool {
	return s.HasExpr == (s.ExprRef != 0)
}

func (s ValueSource) callFlagsValid() bool {
	return s.HasCallPoint == (s.CallPoint != 0)
}

func (s ValueSource) sourcePointFlagsValid() bool {
	return s.HasSourcePoint == (s.SourcePoint != 0)
}

func copyValueSources(in []ValueSource) []ValueSource {
	if len(in) == 0 {
		return nil
	}
	out := make([]ValueSource, len(in))
	copy(out, in)
	return out
}

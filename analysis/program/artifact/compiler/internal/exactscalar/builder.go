// Package exactscalar owns the compiler's finite exact-scalar dataflow. It
// consumes only canonical occurrence and function rows and publishes the
// exact summaries plus the private facts needed by arithmetic derivation.
package exactscalar

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

type state struct {
	unknown bool
	values  map[keyspace.LiteralValue]struct{}
}

func (state state) known() bool { return state.unknown || len(state.values) != 0 }

func (state state) exact() (keyspace.LiteralValue, bool) {
	if state.unknown || len(state.values) != 1 {
		return keyspace.LiteralValue{}, false
	}
	for value := range state.values {
		return value, true
	}
	return keyspace.LiteralValue{}, false
}

// Exact returns the one literal proven for id, if the subject is exact.
func (bundle *Bundle) Exact(id identity.ContentID) (keyspace.LiteralValue, bool) {
	if bundle == nil || !id.Available() {
		return keyspace.LiteralValue{}, false
	}
	return bundle.states[id].exact()
}

// Rows returns the canonical exact-scalar summaries.
func (bundle *Bundle) Rows() []programschema.ExactScalarSummary {
	if bundle == nil {
		return nil
	}
	return append([]programschema.ExactScalarSummary(nil), bundle.summaries...)
}

// ReleaseFacts drops the construction-only fixed-point state after the last
// arithmetic consumer. Canonical summary rows remain owned by the bundle for
// publication.
func (bundle *Bundle) ReleaseFacts() {
	if bundle != nil {
		bundle.states = nil
	}
}

type equationKind uint8

const (
	equationCopy equationKind = iota + 1
	equationUnaryNeg
	equationArithmetic
)

type equation struct {
	kind        equationKind
	output      identity.ContentID
	left, right identity.ContentID
	op          flowkind.BinaryOp
}

// Reason identifies one of the exact-scalar admission failures. The parent
// maps these closed reasons into its CompileFailure vocabulary.
type Reason uint8

const (
	ReasonUnavailable Reason = iota + 1
	ReasonValueSourceAppend
	ReasonValues
	ReasonStorageRead
	ReasonStorageBind
)

type Fault struct {
	reason Reason
	row    int
	subrow int
	failed bool
}

func (fault Fault) Failed() bool   { return fault.failed }
func (fault Fault) Reason() Reason { return fault.reason }
func (fault Fault) Row() int       { return fault.row }
func (fault Fault) Subrow() int    { return fault.subrow }

func failure(reason Reason, row, subrow int) Fault {
	return Fault{reason: reason, row: row, subrow: subrow, failed: true}
}

// Input is the complete canonical row boundary needed by exact-scalar
// derivation. No Program, Flow, compiler state, or domain callback crosses it.
type Input struct {
	Occurrences        []programschema.Occurrence
	OccurrencePoints   []programschema.OccurrencePoint
	OccurrenceInputs   []programschema.OccurrenceInput
	FunctionBoundaries []programschema.FunctionBoundary
	FunctionFormals    []programschema.FunctionFormal
	FunctionVarargs    []programschema.FunctionVararg
	FunctionCaptures   []programschema.FunctionCapture
}

// Bundle owns the private fixed-point states and canonical summaries.
type Bundle struct {
	states    map[identity.ContentID]state
	summaries []programschema.ExactScalarSummary
}

func formalAt(input Input, boundary programschema.FunctionBoundary, index int) (programschema.FunctionFormal, bool) {
	if index < 0 || index >= boundary.FormalCount() {
		return programschema.FunctionFormal{}, false
	}
	offset, _, ok := boundary.FormalSpan()
	if !ok || uint64(offset)+uint64(index) >= uint64(len(input.FunctionFormals)) {
		return programschema.FunctionFormal{}, false
	}
	formal := input.FunctionFormals[int(offset)+index]
	return formal, formal.Available()
}

func vararg(input Input, boundary programschema.FunctionBoundary) (programschema.FunctionVararg, bool) {
	if !boundary.HasVararg() {
		return programschema.FunctionVararg{}, false
	}
	offset, count, ok := boundary.VarargSpan()
	if !ok || count != 1 || uint64(offset) >= uint64(len(input.FunctionVarargs)) {
		return programschema.FunctionVararg{}, false
	}
	row := input.FunctionVarargs[offset]
	return row, row.Available()
}

func captureAt(input Input, boundary programschema.FunctionBoundary, index int) (programschema.FunctionCapture, bool) {
	if index < 0 || index >= boundary.CaptureCount() {
		return programschema.FunctionCapture{}, false
	}
	offset, _, ok := boundary.CaptureSpan()
	if !ok || uint64(offset)+uint64(index) >= uint64(len(input.FunctionCaptures)) {
		return programschema.FunctionCapture{}, false
	}
	row := input.FunctionCaptures[int(offset)+index]
	return row, row.Available() && row.InnerBodyID() == boundary.BodyID()
}

func Compile(input Input) (*Bundle, Fault) {
	states := make(map[identity.ContentID]state)
	var equations []equation
	join := func(id identity.ContentID, incoming state) bool {
		if !id.Available() || !incoming.known() {
			return false
		}
		current := states[id]
		changed := incoming.unknown && !current.unknown
		current.unknown = current.unknown || incoming.unknown
		if len(incoming.values) != 0 {
			if current.values == nil {
				current.values = make(map[keyspace.LiteralValue]struct{}, len(incoming.values))
			}
			for value := range incoming.values {
				if _, exists := current.values[value]; !exists {
					current.values[value] = struct{}{}
					changed = true
				}
			}
		}
		states[id] = current
		return changed
	}
	unknown := state{unknown: true}
	exact := func(literal keyspace.LiteralValue) state {
		return state{values: map[keyspace.LiteralValue]struct{}{literal: {}}}
	}

	for _, boundary := range input.FunctionBoundaries {
		for index := 0; index < boundary.FormalCount(); index++ {
			formal, ok := formalAt(input, boundary, index)
			if !ok {
				return nil, failure(ReasonUnavailable, -1, index)
			}
			join(formal.CellID(), unknown)
		}
		if boundary.HasVararg() {
			row, ok := vararg(input, boundary)
			if !ok {
				return nil, failure(ReasonUnavailable, -1, -1)
			}
			join(row.CellID(), unknown)
		}
		for index := 0; index < boundary.CaptureCount(); index++ {
			capture, ok := captureAt(input, boundary, index)
			if !ok {
				return nil, failure(ReasonUnavailable, -1, index)
			}
			join(capture.InnerCellID(), unknown)
			join(capture.OuterCellID(), unknown)
		}
	}

	for index, row := range input.Occurrences {
		if !programschema.OccurrenceDenseAvailable(row, input.OccurrencePoints, input.OccurrenceInputs) {
			return nil, failure(ReasonUnavailable, index, -1)
		}
		switch row.Kind() {
		case programschema.OccurrenceValueSource:
			span, spanOK := programschema.OccurrenceValueSourceSpanID(row, input.OccurrenceInputs)
			family, literal, literalOK := row.Literal()
			if !spanOK {
				return nil, failure(ReasonValueSourceAppend, index, -1)
			}
			if literalOK && (family == keyspace.FamilyInteger || family == keyspace.FamilyFloat) {
				join(row.ID(), exact(literal))
				join(span, exact(literal))
			} else {
				join(row.ID(), unknown)
				join(span, unknown)
			}
		case programschema.OccurrenceValuesMember:
			count, ok := programschema.OccurrenceInputCount(row, input.OccurrenceInputs)
			if !ok || count != 2 {
				return nil, failure(ReasonValues, index, -1)
			}
			span, ok := programschema.OccurrenceInputID(row, input.OccurrenceInputs, 1)
			if !ok {
				return nil, failure(ReasonValues, index, -1)
			}
			equations = append(equations, equation{kind: equationCopy, output: row.ID(), left: span})
		case programschema.OccurrenceStorageRead:
			cell, span, ok := programschema.OccurrenceStorageReadOperands(row, input.OccurrenceInputs)
			if !ok {
				return nil, failure(ReasonStorageRead, index, -1)
			}
			equations = append(equations, equation{kind: equationCopy, output: span, left: cell}, equation{kind: equationCopy, output: row.ID(), left: cell})
		case programschema.OccurrenceStorageBindTransfer, programschema.OccurrenceStorageWrite:
			count, ok := programschema.OccurrenceInputCount(row, input.OccurrenceInputs)
			if !ok || count < 3 {
				return nil, failure(ReasonStorageBind, index, -1)
			}
			from, fromOK := programschema.OccurrenceInputID(row, input.OccurrenceInputs, 1)
			to, toOK := programschema.OccurrenceInputID(row, input.OccurrenceInputs, 2)
			if !fromOK || !toOK {
				return nil, failure(ReasonStorageBind, index, -1)
			}
			equations = append(equations, equation{kind: equationCopy, output: to, left: from})
		case programschema.OccurrenceBinaryArithmetic:
			left, right, op, ok := programschema.OccurrenceBinaryArithmeticOperands(row, input.OccurrenceInputs)
			if !ok {
				return nil, failure(ReasonUnavailable, index, -1)
			}
			equations = append(equations, equation{kind: equationArithmetic, output: row.ID(), left: left, right: right, op: op})
		case programschema.OccurrenceUnary:
			operand, ok := programschema.OccurrenceInputID(row, input.OccurrenceInputs, 0)
			if !ok {
				return nil, failure(ReasonUnavailable, index, -1)
			}
			if flowkind.UnaryOp(row.Code()) == flowkind.UnaryNeg {
				equations = append(equations, equation{kind: equationUnaryNeg, output: row.ID(), left: operand})
			} else {
				join(row.ID(), unknown)
			}
		case programschema.OccurrenceSelect, programschema.OccurrenceValueClaim, programschema.OccurrenceBinaryEquality, programschema.OccurrenceBinaryOrder:
			join(row.ID(), unknown)
		case programschema.OccurrenceIndexRead:
			if count, ok := programschema.OccurrenceInputCount(row, input.OccurrenceInputs); ok && count >= 3 {
				if result, ok := programschema.OccurrenceInputID(row, input.OccurrenceInputs, 2); ok {
					join(result, unknown)
				}
			}
		case programschema.OccurrenceAllocation, programschema.OccurrenceCall:
			join(row.ID(), unknown)
		}
	}

	changed := true
	for changed {
		changed = false
		for _, item := range equations {
			switch item.kind {
			case equationCopy:
				if join(item.output, states[item.left]) {
					changed = true
				}
			case equationUnaryNeg:
				operand := states[item.left]
				if !operand.known() {
					continue
				}
				literal, literalOK := operand.exact()
				result, resultOK := scalar.ExactUnaryNegLiteral(literal)
				if !literalOK || !resultOK {
					if join(item.output, unknown) {
						changed = true
					}
					continue
				}
				if join(item.output, exact(result)) {
					changed = true
				}
			case equationArithmetic:
				left, right := states[item.left], states[item.right]
				if !left.known() || !right.known() {
					continue
				}
				leftLiteral, leftExact := left.exact()
				rightLiteral, rightExact := right.exact()
				if !leftExact || !rightExact {
					if join(item.output, unknown) {
						changed = true
					}
					continue
				}
				result, resultOK := scalar.ExactArithmeticLiteral(leftLiteral, rightLiteral, item.op)
				if !resultOK {
					if join(item.output, unknown) {
						changed = true
					}
					continue
				}
				if join(item.output, exact(result)) {
					changed = true
				}
			}
		}
	}

	summaries := make([]programschema.ExactScalarSummary, 0, len(input.Occurrences)*3)
	seen := make(map[identity.ContentID]struct{}, len(input.Occurrences)*3)
	for index, row := range input.Occurrences {
		if row.Kind() != programschema.OccurrenceBinaryArithmetic {
			continue
		}
		left, right, _, ok := programschema.OccurrenceBinaryArithmeticOperands(row, input.OccurrenceInputs)
		if !ok {
			return nil, failure(ReasonUnavailable, index, -1)
		}
		uses := [...]struct {
			role    programschema.ExactScalarSummaryRole
			subject identity.ContentID
		}{
			{role: programschema.ExactScalarSummaryLeft, subject: left},
			{role: programschema.ExactScalarSummaryRight, subject: right},
			{role: programschema.ExactScalarSummaryResult, subject: row.ID()},
		}
		for _, use := range uses {
			literal, ok := states[use.subject].exact()
			if !ok {
				continue
			}
			body, ok := row.BodyID()
			if !ok {
				return nil, failure(ReasonUnavailable, index, -1)
			}
			summary, ok := programschema.NewExactScalarSummary(row.ID(), use.subject, body, use.role, programschema.SummaryLiteral{Kind: uint8(literal.Kind), Integer: literal.Integer, FloatBits: literal.FloatBits})
			if !ok {
				return nil, failure(ReasonUnavailable, index, -1)
			}
			if _, duplicate := seen[summary.ID()]; duplicate {
				return nil, failure(ReasonUnavailable, index, -1)
			}
			seen[summary.ID()] = struct{}{}
			summaries = append(summaries, summary)
		}
	}
	identity.SortByContentID(summaries, func(row programschema.ExactScalarSummary) identity.ContentID { return row.ID() })
	return &Bundle{states: states, summaries: summaries}, Fault{}
}

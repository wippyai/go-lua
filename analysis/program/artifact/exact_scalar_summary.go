package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
)

// ExactScalarSummaryRole is the closed arithmetic-use position authenticated
// by a Program summary.  It is semantic, not an authored operand ordinal
// supplied later by Link.
type ExactScalarSummaryRole uint8

const (
	ExactScalarSummaryLeft ExactScalarSummaryRole = iota + 1
	ExactScalarSummaryRight
	ExactScalarSummaryResult
)

func (role ExactScalarSummaryRole) Valid() bool {
	return role >= ExactScalarSummaryLeft && role <= ExactScalarSummaryResult
}

// ExactScalarSummaryRow is one reusable Program-owned concrete scalar at an
// exact arithmetic operand or result. It is deliberately narrower than a
// Value-domain fact: the complete Program-local transfer closure must yield
// one scalar at this use. Link can mount the row without solving the Program
// body again, while unrelated authored literals never become global facts.
type ExactScalarSummaryRow struct {
	id         identity.ContentID
	occurrence identity.ContentID
	subject    identity.ContentID
	body       identity.ContentID
	role       ExactScalarSummaryRole
	literal    keyspace.LiteralValue
}

func (artifact *Artifact) ExactScalarSummaryCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.exactScalarSummaries)
}

func (artifact *Artifact) ExactScalarSummaryAt(index int) (ExactScalarSummaryRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.exactScalarSummaries) {
		return ExactScalarSummaryRow{}, false
	}
	row := artifact.exactScalarSummaries[index]
	return row, row.Available()
}

func (row ExactScalarSummaryRow) Available() bool {
	return row.id.Available() && row.occurrence.Available() && row.subject.Available() && row.body.Available() && row.role.Valid() &&
		(row.literal.Kind == keyspace.LiteralInteger || row.literal.Kind == keyspace.LiteralFloat) &&
		row.id == exactScalarSummaryID(row.occurrence, row.subject, row.body, row.role, row.literal)
}

func (row ExactScalarSummaryRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row ExactScalarSummaryRow) OccurrenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.occurrence
}

func (row ExactScalarSummaryRow) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row ExactScalarSummaryRow) SubjectID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.subject
}

func (row ExactScalarSummaryRow) Role() ExactScalarSummaryRole {
	if !row.Available() {
		return 0
	}
	return row.role
}

func (row ExactScalarSummaryRow) Literal() (keyspace.LiteralValue, bool) {
	return row.literal, row.Available()
}

func exactScalarSummaryID(occurrence, subject, body identity.ContentID, role ExactScalarSummaryRole, literal keyspace.LiteralValue) identity.ContentID {
	if !occurrence.Available() || !subject.Available() || !body.Available() || !role.Valid() ||
		(literal.Kind != keyspace.LiteralInteger && literal.Kind != keyspace.LiteralFloat) {
		return identity.ContentID{}
	}
	return digest("analysis/program-artifact/exact-scalar-summary", artifactFormat,
		bytesField(occurrence), bytesField(subject), bytesField(body), uintField(uint64(role)), uintField(uint64(literal.Kind)),
		uintField(uint64(literal.Integer)), uintField(literal.FloatBits))
}

type exactScalarState struct {
	unknown bool
	values  map[keyspace.LiteralValue]struct{}
}

func (state exactScalarState) known() bool { return state.unknown || len(state.values) != 0 }

func (state exactScalarState) exact() (keyspace.LiteralValue, bool) {
	if state.unknown || len(state.values) != 1 {
		return keyspace.LiteralValue{}, false
	}
	for value := range state.values {
		return value, true
	}
	return keyspace.LiteralValue{}, false
}

type exactScalarEquationKind uint8

const (
	exactScalarEquationCopy exactScalarEquationKind = iota + 1
	exactScalarEquationUnaryNeg
	exactScalarEquationArithmetic
)

type exactScalarEquation struct {
	kind        exactScalarEquationKind
	output      identity.ContentID
	left, right identity.ContentID
	op          flowkind.BinaryOp
}

// deriveExactScalarSummariesFailure closes the finite exact-scalar slice of
// the reusable Program transformer.  Storage equations are may-unioned across
// the whole Program; any unknown or differing producer blocks an exact
// summary.  This is intentionally conservative around branches and loops.
func (compiler *compiler) deriveExactScalarSummariesFailure() CompileFailure {
	if compiler == nil || compiler.exactScalarSummaries != nil || compiler.exactScalarStates != nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	states := make(map[identity.ContentID]exactScalarState)
	var equations []exactScalarEquation
	var arithmetic []OccurrenceRow

	join := func(id identity.ContentID, incoming exactScalarState) bool {
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
	unknown := exactScalarState{unknown: true}
	exact := func(literal keyspace.LiteralValue) exactScalarState {
		return exactScalarState{values: map[keyspace.LiteralValue]struct{}{literal: {}}}
	}

	// Formal/capture cells are caller-supplied interface state, never a local
	// constant merely because another local producer happens to share a cell.
	for _, boundary := range compiler.functionBoundaries {
		for _, formal := range boundary.formals {
			join(formal.cell, unknown)
		}
		if boundary.hasVararg {
			join(boundary.vararg.cell, unknown)
		}
		for _, capture := range boundary.captures {
			join(capture.inner, unknown)
			join(capture.outer, unknown)
		}
	}

	for index, row := range compiler.occurrences {
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		switch row.Kind() {
		case OccurrenceValueSource:
			span, spanOK := row.ValueSourceSpanID()
			family, literal, literalOK := row.Literal()
			if !spanOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceValueSourceAppend)
			}
			if literalOK && (family == keyspace.FamilyInteger || family == keyspace.FamilyFloat) {
				join(row.ID(), exact(literal))
				join(span, exact(literal))
			} else {
				join(row.ID(), unknown)
				join(span, unknown)
			}
		case OccurrenceValuesMember:
			if row.InputCount() != 2 {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceValues)
			}
			span, spanOK := row.InputAt(1)
			if !spanOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceValues)
			}
			equations = append(equations, exactScalarEquation{kind: exactScalarEquationCopy, output: row.ID(), left: span})
		case OccurrenceStorageRead:
			cell, span, readOK := row.StorageRead()
			if !readOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
			}
			equations = append(equations,
				exactScalarEquation{kind: exactScalarEquationCopy, output: span, left: cell},
				exactScalarEquation{kind: exactScalarEquationCopy, output: row.ID(), left: cell})
		case OccurrenceStorageBindTransfer, OccurrenceStorageWrite:
			if row.InputCount() < 3 {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
			}
			from, fromOK := row.InputAt(1)
			to, toOK := row.InputAt(2)
			if !fromOK || !toOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
			}
			equations = append(equations, exactScalarEquation{kind: exactScalarEquationCopy, output: to, left: from})
		case OccurrenceBinaryArithmetic:
			left, right, op, arithmeticOK := row.BinaryArithmetic()
			if !arithmeticOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			equations = append(equations, exactScalarEquation{kind: exactScalarEquationArithmetic, output: row.ID(), left: left, right: right, op: op})
			arithmetic = append(arithmetic, row)
		case OccurrenceUnary:
			operand, operandOK := row.InputAt(0)
			if !operandOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if flowkind.UnaryOp(row.Code()) == flowkind.UnaryNeg {
				equations = append(equations, exactScalarEquation{kind: exactScalarEquationUnaryNeg, output: row.ID(), left: operand})
			} else {
				join(row.ID(), unknown)
			}
		case OccurrenceSelect, OccurrenceValueClaim, OccurrenceBinaryEquality, OccurrenceBinaryOrder:
			join(row.ID(), unknown)
		case OccurrenceIndexRead:
			if row.InputCount() >= 3 {
				if result, ok := row.InputAt(2); ok {
					join(result, unknown)
				}
			}
		case OccurrenceAllocation, OccurrenceCall:
			join(row.ID(), unknown)
		}
	}

	changed := true
	for changed {
		changed = false
		for _, equation := range equations {
			switch equation.kind {
			case exactScalarEquationCopy:
				if join(equation.output, states[equation.left]) {
					changed = true
				}
			case exactScalarEquationUnaryNeg:
				operand := states[equation.left]
				if !operand.known() {
					continue
				}
				literal, literalOK := operand.exact()
				result, resultOK := scalar.ExactUnaryNegLiteral(literal)
				if !literalOK || !resultOK {
					if join(equation.output, unknown) {
						changed = true
					}
					continue
				}
				if join(equation.output, exact(result)) {
					changed = true
				}
			case exactScalarEquationArithmetic:
				left, right := states[equation.left], states[equation.right]
				if !left.known() || !right.known() {
					continue
				}
				leftLiteral, leftExact := left.exact()
				rightLiteral, rightExact := right.exact()
				if !leftExact || !rightExact {
					if join(equation.output, unknown) {
						changed = true
					}
					continue
				}
				result, resultOK := scalar.ExactArithmeticLiteral(leftLiteral, rightLiteral, equation.op)
				if !resultOK {
					if join(equation.output, unknown) {
						changed = true
					}
					continue
				}
				if join(equation.output, exact(result)) {
					changed = true
				}
			}
		}
	}

	summaries := make([]ExactScalarSummaryRow, 0, len(arithmetic)*3)
	seen := make(map[identity.ContentID]struct{}, len(arithmetic)*3)
	for index, row := range arithmetic {
		left, right, _, endpointsOK := row.BinaryArithmetic()
		if !endpointsOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		uses := [...]struct {
			role    ExactScalarSummaryRole
			subject identity.ContentID
		}{
			{role: ExactScalarSummaryLeft, subject: left},
			{role: ExactScalarSummaryRight, subject: right},
			{role: ExactScalarSummaryResult, subject: row.ID()},
		}
		for _, use := range uses {
			literal, exactOK := states[use.subject].exact()
			if !exactOK {
				continue
			}
			summary := ExactScalarSummaryRow{occurrence: row.ID(), subject: use.subject, body: row.body, role: use.role, literal: literal}
			summary.id = exactScalarSummaryID(summary.occurrence, summary.subject, summary.body, summary.role, summary.literal)
			if !summary.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if _, duplicate := seen[summary.id]; duplicate {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			seen[summary.id] = struct{}{}
			summaries = append(summaries, summary)
		}
	}
	identity.SortByContentID(summaries, func(row ExactScalarSummaryRow) identity.ContentID { return row.id })
	compiler.exactScalarSummaries = summaries
	compiler.exactScalarStates = states
	return CompileFailure{}
}

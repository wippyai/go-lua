package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

// NumericRepresentation is ProgramArtifact's closed domain-abstract numeric
// carrier. Number means the integer/float union; it is not an exact machine
// representation claim.
type NumericRepresentation uint8

const (
	NumericRepresentationInvalid NumericRepresentation = iota
	NumericRepresentationInteger
	NumericRepresentationFloat
	NumericRepresentationNumber
)

func (representation NumericRepresentation) Valid() bool {
	return representation >= NumericRepresentationInteger && representation <= NumericRepresentationNumber
}

// ArithmeticDivisorProperty is an exact Program-local guard conclusion for
// an arithmetic right operand. None is deliberate withholding.
type ArithmeticDivisorProperty uint8

const (
	ArithmeticDivisorNone ArithmeticDivisorProperty = iota
	ArithmeticDivisorNonzero
	ArithmeticDivisorNonzeroNotMinusOne
)

func (property ArithmeticDivisorProperty) Valid() bool {
	return property <= ArithmeticDivisorNonzeroNotMinusOne
}

func (property ArithmeticDivisorProperty) validFor(op flowkind.BinaryOp) bool {
	if !property.Valid() {
		return false
	}
	return property == ArithmeticDivisorNone || op == flowkind.BinaryIDiv
}

// ArithmeticSummaryRow is one reusable abstract interpretation result for a
// Program-owned arithmetic occurrence. It is derived once from formal/static
// annotations and local transfer closure; Link only mounts the summary.
type ArithmeticSummaryRow struct {
	id, occurrence, body identity.ContentID
	op                   flowkind.BinaryOp
	left, right, result  NumericRepresentation
	divisor              ArithmeticDivisorProperty
}

func (artifact *Artifact) ArithmeticSummaryCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.arithmeticSummaries)
}

func (artifact *Artifact) ArithmeticSummaryAt(index int) (ArithmeticSummaryRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.arithmeticSummaries) {
		return ArithmeticSummaryRow{}, false
	}
	row := artifact.arithmeticSummaries[index]
	return row, row.Available()
}

func (row ArithmeticSummaryRow) Available() bool {
	return row.id.Available() && row.occurrence.Available() && row.body.Available() &&
		flowkind.IsBinaryArithmetic(row.op) && row.left.Valid() && row.right.Valid() && row.result.Valid() && row.divisor.validFor(row.op) &&
		row.id == arithmeticSummaryID(row.occurrence, row.body, row.op, row.left, row.right, row.result, row.divisor)
}

func (row ArithmeticSummaryRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row ArithmeticSummaryRow) OccurrenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.occurrence
}
func (row ArithmeticSummaryRow) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row ArithmeticSummaryRow) Operator() flowkind.BinaryOp {
	if !row.Available() {
		return 0
	}
	return row.op
}
func (row ArithmeticSummaryRow) Representations() (left, right, result NumericRepresentation, ok bool) {
	if !row.Available() {
		return 0, 0, 0, false
	}
	return row.left, row.right, row.result, true
}

func (row ArithmeticSummaryRow) DivisorProperty() ArithmeticDivisorProperty {
	if !row.Available() {
		return ArithmeticDivisorNone
	}
	return row.divisor
}

func arithmeticSummaryID(occurrence, body identity.ContentID, op flowkind.BinaryOp, left, right, result NumericRepresentation, divisor ArithmeticDivisorProperty) identity.ContentID {
	if !occurrence.Available() || !body.Available() || !flowkind.IsBinaryArithmetic(op) || !left.Valid() || !right.Valid() || !result.Valid() || !divisor.validFor(op) {
		return identity.ContentID{}
	}
	return digest("analysis/program-artifact/arithmetic-summary", artifactFormat,
		bytesField(occurrence), bytesField(body), uintField(uint64(op)), uintField(uint64(left)), uintField(uint64(right)), uintField(uint64(result)), uintField(uint64(divisor)))
}

const (
	numericIntegerMask uint8 = 1 << iota
	numericFloatMask
)

type numericSummaryState struct {
	unknown bool
	mask    uint8
}

func (state numericSummaryState) known() bool { return state.unknown || state.mask != 0 }
func (state numericSummaryState) representation() (NumericRepresentation, bool) {
	if state.unknown {
		return NumericRepresentationInvalid, false
	}
	switch state.mask {
	case numericIntegerMask:
		return NumericRepresentationInteger, true
	case numericFloatMask:
		return NumericRepresentationFloat, true
	case numericIntegerMask | numericFloatMask:
		return NumericRepresentationNumber, true
	default:
		return NumericRepresentationInvalid, false
	}
}

type numericSummaryEquationKind uint8

const (
	numericSummaryCopy numericSummaryEquationKind = iota + 1
	numericSummaryArithmetic
)

type numericSummaryEquation struct {
	kind        numericSummaryEquationKind
	output      identity.ContentID
	left, right identity.ContentID
	op          flowkind.BinaryOp
}

type arithmeticGuardMask uint8

const (
	arithmeticGuardExcludesZero arithmeticGuardMask = 1 << iota
	arithmeticGuardExcludesMinusOne
)

func mergeArithmeticGuardFacts(left, right map[identity.ContentID]arithmeticGuardMask) map[identity.ContentID]arithmeticGuardMask {
	merged := make(map[identity.ContentID]arithmeticGuardMask, len(left)+len(right))
	for id, mask := range left {
		merged[id] = mask
	}
	for id, mask := range right {
		merged[id] |= mask
	}
	return merged
}

func intersectArithmeticGuardFacts(left, right map[identity.ContentID]arithmeticGuardMask) map[identity.ContentID]arithmeticGuardMask {
	intersected := make(map[identity.ContentID]arithmeticGuardMask)
	for id, leftMask := range left {
		if mask := leftMask & right[id]; mask != 0 {
			intersected[id] = mask
		}
	}
	return intersected
}

// arithmeticDivisorProperties derives only facts guaranteed by the exact
// Program-owned causal edge entering an arithmetic Body. A true conjunction
// authenticates both operands; false conjunctions and disjunctions deliberately
// yield no fact. Multiple incoming body entries are intersected, so one
// unguarded path withholds the summary rather than turning a may-guard into a
// proof.
func (compiler *compiler) arithmeticDivisorProperties() (map[identity.ContentID]ArithmeticDivisorProperty, CompileFailure) {
	if compiler == nil || compiler.exactScalarStates == nil {
		return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	storageOrigins := make(map[identity.ContentID]identity.ContentID)
	equalities := make(map[identity.ContentID]OccurrenceRow)
	selects := make(map[identity.ContentID]OccurrenceRow)
	claims := make(map[identity.ContentID]identity.ContentID)
	for index, row := range compiler.occurrences {
		if !row.Available() {
			return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		switch row.Kind() {
		case OccurrenceStorageRead:
			cell, span, readOK := row.StorageRead()
			if !readOK {
				return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
			}
			if prior, duplicate := storageOrigins[span]; duplicate && prior != cell {
				return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
			}
			storageOrigins[span] = cell
		case OccurrenceBinaryEquality:
			if _, duplicate := equalities[row.ID()]; duplicate {
				return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			equalities[row.ID()] = row
		case OccurrenceSelect:
			if _, duplicate := selects[row.ID()]; duplicate {
				return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			selects[row.ID()] = row
		case OccurrenceValueClaim:
			operand, operandOK := row.InputAt(0)
			if !operandOK {
				return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if prior, duplicate := claims[row.ID()]; duplicate && prior != operand {
				return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			claims[row.ID()] = operand
		}
	}

	type guardVisit struct {
		id    identity.ContentID
		truth bool
	}
	var conditionFacts func(identity.ContentID, bool, map[guardVisit]bool) map[identity.ContentID]arithmeticGuardMask
	conditionFacts = func(condition identity.ContentID, truth bool, visiting map[guardVisit]bool) map[identity.ContentID]arithmeticGuardMask {
		visit := guardVisit{id: condition, truth: truth}
		if !condition.Available() || visiting[visit] {
			return nil
		}
		visiting[visit] = true
		defer delete(visiting, visit)
		if operand, ok := claims[condition]; ok {
			return conditionFacts(operand, truth, visiting)
		}
		if selectRow, ok := selects[condition]; ok {
			if flowkind.SelectOp(selectRow.Code()) != flowkind.SelectAnd || !truth {
				return nil
			}
			left, leftOK := selectRow.InputAt(0)
			right, rightOK := selectRow.InputAt(1)
			if !leftOK || !rightOK {
				return nil
			}
			return mergeArithmeticGuardFacts(conditionFacts(left, true, visiting), conditionFacts(right, true, visiting))
		}
		equality, ok := equalities[condition]
		if !ok {
			return nil
		}
		left, right, op, equalityOK := equality.BinaryEquality()
		if !equalityOK || truth != (op == flowkind.BinaryNotEqual) {
			return nil
		}
		leftCell, leftStored := storageOrigins[left]
		rightCell, rightStored := storageOrigins[right]
		operand, cell := right, leftCell
		if rightStored && !leftStored {
			operand, cell = left, rightCell
		} else if leftStored == rightStored {
			return nil
		}
		literal, exactOK := compiler.exactScalarStates[operand].exact()
		if !exactOK || literal.Kind != keyspace.LiteralInteger {
			return nil
		}
		var mask arithmeticGuardMask
		switch literal.Integer {
		case 0:
			mask = arithmeticGuardExcludesZero
		case -1:
			mask = arithmeticGuardExcludesMinusOne
		default:
			return nil
		}
		return map[identity.ContentID]arithmeticGuardMask{cell: mask}
	}

	bodyByEntry := make(map[identity.ContentID]identity.ContentID, len(compiler.bodies))
	ambiguousEntry := make(map[identity.ContentID]struct{})
	for bodyIndex, body := range compiler.bodies {
		if !body.Available() {
			return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		for pointIndex := 0; pointIndex < body.EntryPointCount(); pointIndex++ {
			point, pointOK := body.EntryPointAt(pointIndex)
			if !pointOK {
				return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, pointIndex, CompileReasonOccurrenceAttachment)
			}
			if prior, duplicate := bodyByEntry[point]; duplicate && prior != body.ID() {
				ambiguousEntry[point] = struct{}{}
			} else {
				bodyByEntry[point] = body.ID()
			}
		}
	}
	bodyFacts := make(map[identity.ContentID]map[identity.ContentID]arithmeticGuardMask)
	bodyIncoming := make(map[identity.ContentID]bool)
	for edgeIndex, edge := range compiler.environment {
		if !edge.Available() {
			return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		// Same-point structural rows do not enter a Body from a predecessor;
		// they preserve or schedule state already at that entry coordinate and
		// therefore cannot erase a guard carried by the real cross-point edge.
		if edge.From() == edge.To() {
			continue
		}
		body, bodyOK := bodyByEntry[edge.To()]
		_, ambiguous := ambiguousEntry[edge.To()]
		if !bodyOK || ambiguous {
			continue
		}
		facts := map[identity.ContentID]arithmeticGuardMask(nil)
		if condition, conditionOK := edge.ConditionValueSpanID(); conditionOK {
			truth, truthOK := edge.Truth()
			if !truthOK {
				return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
			}
			facts = conditionFacts(condition, truth, make(map[guardVisit]bool))
		}
		if !bodyIncoming[body] {
			bodyFacts[body] = facts
			bodyIncoming[body] = true
		} else {
			bodyFacts[body] = intersectArithmeticGuardFacts(bodyFacts[body], facts)
		}
	}

	properties := make(map[identity.ContentID]ArithmeticDivisorProperty)
	for index, row := range compiler.occurrences {
		if row.Kind() != OccurrenceBinaryArithmetic {
			continue
		}
		_, right, op, arithmeticOK := row.BinaryArithmetic()
		if !arithmeticOK {
			return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if op != flowkind.BinaryIDiv {
			continue
		}
		cell := storageOrigins[right]
		mask := bodyFacts[row.body][cell]
		switch {
		case mask&(arithmeticGuardExcludesZero|arithmeticGuardExcludesMinusOne) == (arithmeticGuardExcludesZero | arithmeticGuardExcludesMinusOne):
			properties[row.ID()] = ArithmeticDivisorNonzeroNotMinusOne
		case mask&arithmeticGuardExcludesZero != 0:
			properties[row.ID()] = ArithmeticDivisorNonzero
		}
	}
	return properties, CompileFailure{}
}

func (compiler *compiler) deriveArithmeticSummariesFailure() CompileFailure {
	if compiler == nil || compiler.arithmeticSummaries != nil || compiler.unarySummaries != nil || compiler.staticInputs == nil || compiler.staticTypeNodes == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	states := make(map[identity.ContentID]numericSummaryState)
	join := func(id identity.ContentID, incoming numericSummaryState) bool {
		if !id.Available() || !incoming.known() {
			return false
		}
		current := states[id]
		updated := numericSummaryState{unknown: current.unknown || incoming.unknown, mask: current.mask | incoming.mask}
		if updated == current {
			return false
		}
		states[id] = updated
		return true
	}
	unknown := numericSummaryState{unknown: true}
	numeric := func(mask uint8) numericSummaryState { return numericSummaryState{mask: mask} }

	types := make(map[identity.ContentID]StaticTypeNodeRow, len(compiler.staticTypeNodes))
	for _, row := range compiler.staticTypeNodes {
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
		}
		types[row.ID()] = row
	}
	var typeMask func(identity.ContentID, map[identity.ContentID]bool) (uint8, bool)
	typeMask = func(id identity.ContentID, visiting map[identity.ContentID]bool) (uint8, bool) {
		row, ok := types[id]
		if !ok || visiting[id] {
			return 0, false
		}
		visiting[id] = true
		defer delete(visiting, id)
		switch row.Kind() {
		case StaticNodePrimitive:
			switch programstatic.PrimitiveKind(row.LiteralKind()) {
			case programstatic.PrimitiveInteger:
				return numericIntegerMask, true
			case programstatic.PrimitiveNumber:
				return numericIntegerMask | numericFloatMask, true
			default:
				return 0, false
			}
		case StaticNodeLiteral:
			switch row.Exact().Kind {
			case keyspace.LiteralInteger:
				return numericIntegerMask, true
			case keyspace.LiteralFloat:
				return numericFloatMask, true
			default:
				return 0, false
			}
		case StaticNodeAlias, StaticNodeReference:
			if row.ChildCount() != 1 {
				return 0, false
			}
			child, childOK := row.ChildAt(0)
			if !childOK {
				return 0, false
			}
			return typeMask(child, visiting)
		case StaticNodeUnion:
			if row.ChildCount() == 0 {
				return 0, false
			}
			var mask uint8
			for index := 0; index < row.ChildCount(); index++ {
				child, childOK := row.ChildAt(index)
				childMask, numericOK := typeMask(child, visiting)
				if !childOK || !numericOK {
					return 0, false
				}
				mask |= childMask
			}
			return mask, mask != 0
		default:
			return 0, false
		}
	}

	for _, input := range compiler.staticInputs {
		if !input.Available() || input.Kind() != StaticInputAnnotation || input.OperandKind() != StaticInputOperandRuntimeSubject {
			continue
		}
		mask, numericOK := typeMask(input.TargetID(), make(map[identity.ContentID]bool))
		if numericOK {
			join(input.OperandSubjectID(), numeric(mask))
		} else {
			join(input.OperandSubjectID(), unknown)
		}
	}

	// Formal Cell and StorageCell are the same interface value in two Program
	// namespaces. Propagate an authored annotation across that exact port; an
	// unannotated port remains unknown caller input.
	for _, boundary := range compiler.functionBoundaries {
		for _, formal := range boundary.formals {
			if formal.declared.Available() {
				mask, numericOK := typeMask(formal.declared, make(map[identity.ContentID]bool))
				if numericOK {
					join(formal.cell, numeric(mask))
					join(formal.storage, numeric(mask))
				} else {
					join(formal.cell, unknown)
					join(formal.storage, unknown)
				}
			}
			cell, storage := states[formal.cell], states[formal.storage]
			switch {
			case cell.known() && storage.known():
				join(formal.cell, storage)
				join(formal.storage, cell)
			case cell.known():
				join(formal.storage, cell)
			case storage.known():
				join(formal.cell, storage)
			default:
				join(formal.cell, unknown)
				join(formal.storage, unknown)
			}
		}
		if boundary.hasVararg {
			join(boundary.vararg.cell, unknown)
		}
		for _, capture := range boundary.captures {
			join(capture.inner, unknown)
			join(capture.outer, unknown)
		}
	}

	equations := make([]numericSummaryEquation, 0, len(compiler.occurrences)*2)
	arithmetic := make([]OccurrenceRow, 0)
	unaries := make([]OccurrenceRow, 0)
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
			mask := uint8(0)
			if literalOK && family == keyspace.FamilyInteger && literal.Kind == keyspace.LiteralInteger {
				mask = numericIntegerMask
			} else if literalOK && family == keyspace.FamilyFloat && literal.Kind == keyspace.LiteralFloat {
				mask = numericFloatMask
			}
			if mask == 0 {
				join(row.ID(), unknown)
				join(span, unknown)
			} else {
				join(row.ID(), numeric(mask))
				join(span, numeric(mask))
			}
		case OccurrenceValuesMember:
			span, spanOK := row.InputAt(1)
			if !spanOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceValues)
			}
			equations = append(equations, numericSummaryEquation{kind: numericSummaryCopy, output: row.ID(), left: span})
		case OccurrenceStorageRead:
			cell, span, readOK := row.StorageRead()
			if !readOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
			}
			equations = append(equations,
				numericSummaryEquation{kind: numericSummaryCopy, output: span, left: cell},
				numericSummaryEquation{kind: numericSummaryCopy, output: row.ID(), left: cell})
		case OccurrenceStorageBindTransfer, OccurrenceStorageWrite:
			from, fromOK := row.InputAt(1)
			to, toOK := row.InputAt(2)
			if !fromOK || !toOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
			}
			equations = append(equations, numericSummaryEquation{kind: numericSummaryCopy, output: to, left: from})
		case OccurrenceSelect:
			left, leftOK := row.InputAt(0)
			right, rightOK := row.InputAt(1)
			if !leftOK || !rightOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			equations = append(equations,
				numericSummaryEquation{kind: numericSummaryCopy, output: row.ID(), left: left},
				numericSummaryEquation{kind: numericSummaryCopy, output: row.ID(), left: right})
		case OccurrenceValueClaim:
			operand, operandOK := row.InputAt(0)
			if !operandOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			equations = append(equations, numericSummaryEquation{kind: numericSummaryCopy, output: row.ID(), left: operand})
		case OccurrenceUnary:
			operand, operandOK := row.InputAt(0)
			if !operandOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if flowkind.UnaryOp(row.Code()) == flowkind.UnaryNeg {
				equations = append(equations, numericSummaryEquation{kind: numericSummaryCopy, output: row.ID(), left: operand})
				unaries = append(unaries, row)
			} else {
				join(row.ID(), unknown)
			}
		case OccurrenceBinaryArithmetic:
			left, right, op, arithmeticOK := row.BinaryArithmetic()
			if !arithmeticOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			equations = append(equations, numericSummaryEquation{kind: numericSummaryArithmetic, output: row.ID(), left: left, right: right, op: op})
			arithmetic = append(arithmetic, row)
		case OccurrenceAllocation, OccurrenceCall, OccurrenceIndexRead, OccurrenceBinaryEquality, OccurrenceBinaryOrder:
			join(row.ID(), unknown)
		}
	}

	changed := true
	for changed {
		changed = false
		for _, equation := range equations {
			switch equation.kind {
			case numericSummaryCopy:
				if join(equation.output, states[equation.left]) {
					changed = true
				}
			case numericSummaryArithmetic:
				left, right := states[equation.left], states[equation.right]
				if !left.known() || !right.known() {
					continue
				}
				if left.unknown || right.unknown || left.mask == 0 || right.mask == 0 {
					if join(equation.output, unknown) {
						changed = true
					}
					continue
				}
				resultMask := numericIntegerMask | numericFloatMask
				switch equation.op {
				case flowkind.BinaryDiv, flowkind.BinaryPow:
					resultMask = numericFloatMask
				case flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul, flowkind.BinaryIDiv, flowkind.BinaryMod:
					if left.mask == numericIntegerMask && right.mask == numericIntegerMask {
						resultMask = numericIntegerMask
					}
				default:
					if join(equation.output, unknown) {
						changed = true
					}
					continue
				}
				if join(equation.output, numeric(resultMask)) {
					changed = true
				}
			}
		}
	}

	divisors, divisorFailure := compiler.arithmeticDivisorProperties()
	compiler.exactScalarStates = nil
	if divisorFailure.Available() {
		return divisorFailure
	}
	summaries := make([]ArithmeticSummaryRow, 0, len(arithmetic))
	for index, row := range arithmetic {
		leftID, rightID, op, _ := row.BinaryArithmetic()
		left, leftOK := states[leftID].representation()
		right, rightOK := states[rightID].representation()
		result, resultOK := states[row.ID()].representation()
		if !leftOK || !rightOK || !resultOK {
			continue
		}
		summary := ArithmeticSummaryRow{occurrence: row.ID(), body: row.body, op: op, left: left, right: right, result: result, divisor: divisors[row.ID()]}
		summary.id = arithmeticSummaryID(summary.occurrence, summary.body, summary.op, summary.left, summary.right, summary.result, summary.divisor)
		if !summary.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		summaries = append(summaries, summary)
	}
	identity.SortByContentID(summaries, func(row ArithmeticSummaryRow) identity.ContentID { return row.id })
	unarySummaries := make([]UnarySummaryRow, 0, len(unaries))
	for index, row := range unaries {
		operandID, operandOK := row.InputAt(0)
		operand, operandRepresentationOK := states[operandID].representation()
		result, resultRepresentationOK := states[row.ID()].representation()
		geometry, geometryOK := compiler.occurrenceSpans[occurrenceLookup{kind: OccurrenceUnary, id: row.ID()}]
		// Each summary names an exact Program-issued finish point rather than
		// making Link infer outputs from the occurrence's undifferentiated point
		// membership set. Multi-context occurrences issue one row per output.
		if !operandOK || !operandRepresentationOK || !resultRepresentationOK || !geometryOK || len(geometry.finish) == 0 {
			continue
		}
		for _, output := range geometry.finish {
			summary := UnarySummaryRow{
				occurrence: row.ID(), body: row.body, point: output, op: flowkind.UnaryOp(row.Code()), operand: operand, result: result,
			}
			summary.id = unarySummaryID(summary.occurrence, summary.body, summary.point, summary.op, summary.operand, summary.result)
			if !summary.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			unarySummaries = append(unarySummaries, summary)
		}
	}
	identity.SortByContentID(unarySummaries, func(row UnarySummaryRow) identity.ContentID { return row.id })
	compiler.arithmeticSummaries = summaries
	compiler.unarySummaries = unarySummaries
	return CompileFailure{}
}

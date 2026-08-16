package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// directBranchCheckFromWIR returns the WIR-owned direct check for a branch
// point. Compound-condition implications live on the branch instruction's WIR
// metadata ranges; this helper intentionally returns only the direct descriptor.
func (l *lowerer) directBranchCheckFromWIR(point cfg.Point) (branchcond.Check, bool) {
	return l.firstDirectBranchCheckFromWIR(point)
}

func (l *lowerer) branchConditionFromWIR(check branchcond.Check) (factflow.BranchCondition, bool) {
	if (check.Kind != branchcond.CheckTruthy && check.Kind != branchcond.CheckFalsy) || check.Path.IsEmpty() {
		return factflow.BranchCondition{}, false
	}
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	source, ok := factflow.NewPathValueSource(check.Path.Key(), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	return factflow.NewBranchCondition(source, check.Kind == branchcond.CheckTruthy)
}

func (l *lowerer) branchConditionAtWIR(point cfg.Point) (factflow.BranchCondition, bool) {
	// A normalized type predicate is an authority boundary. If its descriptor
	// is not sealed and exact, the branch has no publishable scalar condition;
	// falling through to inst.A would incorrectly reinterpret the checked value
	// itself as a Lua truthiness test.
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Check == 0 {
			continue
		}
		check := l.wir.Check(inst.Check)
		if check.Kind == wir.CheckTypeEqual || check.Kind == wir.CheckTypeNot {
			if !l.sealedLuaTypeCheckAuthorized(inst) {
				return factflow.BranchCondition{}, false
			}
		}
	}
	if l.sealedLuaTypeChecks {
		for _, inst := range l.wir.PointInstructions(point) {
			if inst.Check == 0 || inst.Dst.Kind != wir.OperandTemp {
				continue
			}
			check := l.wir.Check(inst.Check)
			if check.Kind != wir.CheckTypeEqual && check.Kind != wir.CheckTypeNot {
				continue
			}
			ref, ok := l.exprRef(wirTempExprRefKey{temp: inst.Dst.Ref})
			if !ok {
				return factflow.BranchCondition{}, false
			}
			shape, ok := factflow.NewValueSourceShape(true, false, true, false)
			if !ok {
				return factflow.BranchCondition{}, false
			}
			source, ok := factflow.NewExpressionValueSource(ref, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, shape)
			if !ok {
				return factflow.BranchCondition{}, false
			}
			return factflow.NewBranchCondition(source, true)
		}
		// Statement-form predicates are normalized directly into OpBranch:
		// unlike expression-form predicates, they have no destination temp to
		// carry the scalar comparison identity. Reconstruct that exact, sealed
		// expression DAG from the canonical WIR check rather than publishing the
		// checked operand as the condition (which would change edge semantics).
		for _, inst := range l.wir.PointInstructions(point) {
			if inst.Op != wir.OpBranch || inst.Check == 0 {
				continue
			}
			check := l.wir.Check(inst.Check)
			if check.Kind != wir.CheckTypeEqual && check.Kind != wir.CheckTypeNot {
				continue
			}
			predicate, ok := l.exprRef(wirSealedLuaTypeBranchExprRefKey{point: point})
			if !ok {
				return factflow.BranchCondition{}, false
			}
			l.addSealedLuaTypeCheckOperation(predicate, inst)
			if _, exact := l.expressionOperations[predicate]; !exact {
				return factflow.BranchCondition{}, false
			}
			shape, ok := factflow.NewValueSourceShape(true, false, true, false)
			if !ok {
				return factflow.BranchCondition{}, false
			}
			source, ok := factflow.NewExpressionValueSource(predicate, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, shape)
			if !ok {
				return factflow.BranchCondition{}, false
			}
			return factflow.NewBranchCondition(source, true)
		}
	}
	if condition, ok := l.structuralLogicalBranchCondition(point); ok {
		return condition, true
	}
	if check, ok := l.firstDirectBranchCheckFromWIR(point); ok {
		if check.Kind == branchcond.CheckTruthy || check.Kind == branchcond.CheckFalsy {
			return l.branchConditionFromWIR(check)
		}
		if condition, exact := l.normalizedLengthBranchCondition(point, check); exact {
			return condition, true
		}
		if condition, exact := l.normalizedIndexInRangeBranchCondition(point, check); exact {
			return condition, true
		}
		if condition, exact := l.normalizedScalarBranchCondition(point, check); exact {
			return condition, true
		}
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpBranch || inst.A.Kind == wir.OperandNone {
			continue
		}
		source, ok := l.valueSourceFromWIROperand(
			inst.A,
			0,
			sourceprovenance.NoSourceIndex,
			true,
			false,
			false,
		)
		if ok {
			source.Adjusted = false
		}
		if !ok {
			return factflow.BranchCondition{}, false
		}
		return factflow.NewBranchCondition(source, true)
	}
	return factflow.BranchCondition{}, false
}

// structuralLogicalBranchCondition recovers the exact left value selected by
// a source-authored short-circuit region. WIR may normalize that guard into a
// path check (for example `not kind` becomes `branch falsy kind`), but the
// region's result-temp owner still certifies the original Boolean producer.
func (l *lowerer) structuralLogicalBranchCondition(point cfg.Point) (factflow.BranchCondition, bool) {
	if l == nil || l.wir == nil {
		return factflow.BranchCondition{}, false
	}
	var condition factflow.BranchCondition
	var found, conflict bool
	l.wir.ForEachStructuralExpressionRegion(func(owner wir.StructuralExpressionOwner, region wir.StructuralExpressionRegion) bool {
		if region.Guard != point || !owner.HasTemp {
			return true
		}
		defs := l.wirTempDefSets()[owner.Temp]
		if len(defs) != 2 {
			conflict = true
			return false
		}
		leftDef, _, _, exact := l.wirLogicalTempDefs(defs)
		if !exact {
			conflict = true
			return false
		}
		source, exact := l.valueSourceFromWIROperand(leftDef.A, 0, factflow.NoValueSourceIndex, true, false, false)
		if !exact {
			conflict = true
			return false
		}
		candidate, exact := factflow.NewBranchCondition(source, true)
		if !exact || found && candidate != condition {
			conflict = true
			return false
		}
		condition, found = candidate, true
		return true
	})
	return condition, found && !conflict
}

type wirSealedLuaTypeBranchExprRefKey struct{ point cfg.Point }

// wirNormalizedScalarBranchExprRefKey owns the Boolean value erased when WIR
// normalizes a statement-form scalar comparison (including nil) directly into
// OpBranch. The check descriptor is the canonical comparison identity;
// rebuilding its pure expression keeps guards and refinements on one producer.
type wirNormalizedScalarBranchExprRefKey struct{ point cfg.Point }

type wirNormalizedLengthValueExprRefKey struct{ point cfg.Point }
type wirNormalizedLengthBranchExprRefKey struct{ point cfg.Point }
type wirNormalizedIndexLengthExprRefKey struct{ point cfg.Point }
type wirNormalizedIndexBranchExprRefKey struct{ point cfg.Point }

// normalizedLengthBranchCondition rebuilds the pure scalar producer erased by
// WIR's CheckLenGe normalization. The check is the canonical authority: the
// value is `#path`, and Negated selects the exact complementary comparison
// `< floor` instead of changing edge polarity or inventing a bound.
func (l *lowerer) normalizedLengthBranchCondition(point cfg.Point, check branchcond.Check) (factflow.BranchCondition, bool) {
	if l == nil || check.Kind != branchcond.CheckLenGe || check.Path.IsEmpty() || check.LenFloor < 0 {
		return factflow.BranchCondition{}, false
	}
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	container, ok := factflow.NewPathValueSource(check.Path.Key(), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	lengthRef, ok := l.exprRef(wirNormalizedLengthValueExprRefKey{point: point})
	if !ok {
		return factflow.BranchCondition{}, false
	}
	lengthOperation, ok := factflow.NewUnaryExpressionOperation("#", container)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	l.expressionOperations[lengthRef] = lengthOperation
	length, ok := factflow.NewExpressionValueSource(lengthRef, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	floor, ok := factflow.NewIntegerLiteralValueSource(check.LenFloor, 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	predicateRef, ok := l.exprRef(wirNormalizedLengthBranchExprRefKey{point: point})
	if !ok {
		return factflow.BranchCondition{}, false
	}
	operator := ">="
	if check.Negated {
		operator = "<"
	}
	predicateOperation, ok := factflow.NewBinaryExpressionOperation(operator, length, floor)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	l.expressionOperations[predicateRef] = predicateOperation
	l.expressionValues[predicateRef] = l.valueFromTypeWithWitness(typ.Boolean)
	predicate, ok := factflow.NewExpressionValueSource(predicateRef, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	return factflow.NewBranchCondition(predicate, true)
}

// normalizedIndexInRangeBranchCondition rebuilds the exact Boolean producer
// erased when WIR normalizes `index <= #array` (or its negation) into an
// index-range proof descriptor.  The proof remains a separate edge fact; this
// expression is solely the source-authored branch value used by the canonical
// ValueTerm/guard path.
func (l *lowerer) normalizedIndexInRangeBranchCondition(point cfg.Point, check branchcond.Check) (factflow.BranchCondition, bool) {
	if l == nil || check.Kind != branchcond.CheckIndexInRange || check.Path.IsEmpty() || check.OtherPath.IsEmpty() {
		return factflow.BranchCondition{}, false
	}
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	index, ok := factflow.NewPathValueSource(check.Path.Key(), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	array, ok := factflow.NewPathValueSource(check.OtherPath.Key(), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	lengthRef, ok := l.exprRef(wirNormalizedIndexLengthExprRefKey{point: point})
	if !ok {
		return factflow.BranchCondition{}, false
	}
	lengthOperation, ok := factflow.NewUnaryExpressionOperation("#", array)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	l.expressionOperations[lengthRef] = lengthOperation
	length, ok := factflow.NewExpressionValueSource(lengthRef, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	predicateRef, ok := l.exprRef(wirNormalizedIndexBranchExprRefKey{point: point})
	if !ok {
		return factflow.BranchCondition{}, false
	}
	operator := "<="
	if check.Negated {
		operator = ">"
	}
	predicateOperation, ok := factflow.NewBinaryExpressionOperation(operator, index, length)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	l.expressionOperations[predicateRef] = predicateOperation
	l.expressionValues[predicateRef] = l.valueFromTypeWithWitness(typ.Boolean)
	predicate, ok := factflow.NewExpressionValueSource(predicateRef, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	return factflow.NewBranchCondition(predicate, true)
}

func (l *lowerer) normalizedScalarBranchCondition(point cfg.Point, check branchcond.Check) (factflow.BranchCondition, bool) {
	if l == nil || check.Path.IsEmpty() ||
		(check.Kind != branchcond.CheckLiteralEqual && check.Kind != branchcond.CheckLiteralNot && check.Kind != branchcond.CheckNil && check.Kind != branchcond.CheckNotNil &&
			check.Kind != branchcond.CheckPathEqual && check.Kind != branchcond.CheckPathNot &&
			check.Kind != branchcond.CheckNumGe && check.Kind != branchcond.CheckNumLe) ||
		(check.Kind == branchcond.CheckPathEqual || check.Kind == branchcond.CheckPathNot) && check.OtherPath.IsEmpty() {
		return factflow.BranchCondition{}, false
	}
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	left, ok := factflow.NewPathValueSource(check.Path.Key(), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	var right factflow.ValueSource
	switch check.Kind {
	case branchcond.CheckNil, branchcond.CheckNotNil:
		right = factflow.NewNilValueSource(factflow.NoValueSourceIndex)
	case branchcond.CheckPathEqual, branchcond.CheckPathNot:
		right, ok = factflow.NewPathValueSource(check.OtherPath.Key(), 0, factflow.NoValueSourceIndex, 0, shape)
	case branchcond.CheckNumGe:
		right, ok = factflow.NewIntegerLiteralValueSource(check.NumFloor, 0, factflow.NoValueSourceIndex, 0, shape)
	case branchcond.CheckNumLe:
		if !check.HasNumCeil {
			return factflow.BranchCondition{}, false
		}
		right, ok = factflow.NewIntegerLiteralValueSource(check.NumCeil, 0, factflow.NoValueSourceIndex, 0, shape)
	default:
		right, ok = literalBranchValueSource(check, shape)
	}
	if !ok && check.Kind != branchcond.CheckNil && check.Kind != branchcond.CheckNotNil {
		return factflow.BranchCondition{}, false
	}
	ref, ok := l.exprRef(wirNormalizedScalarBranchExprRefKey{point: point})
	if !ok {
		return factflow.BranchCondition{}, false
	}
	op := "=="
	switch check.Kind {
	case branchcond.CheckLiteralNot, branchcond.CheckNotNil, branchcond.CheckPathNot:
		op = "~="
	case branchcond.CheckNumGe:
		op = ">="
		if check.Negated {
			op = "<"
		}
	case branchcond.CheckNumLe:
		op = "<="
		if check.Negated {
			op = ">"
		}
	}
	operation, ok := factflow.NewBinaryExpressionOperation(op, left, right)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	l.expressionOperations[ref] = operation
	l.expressionValues[ref] = l.valueFromTypeWithWitness(typ.Boolean)
	source, ok := factflow.NewExpressionValueSource(ref, 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	return factflow.NewBranchCondition(source, true)
}

func literalBranchValueSource(check branchcond.Check, shape factflow.ValueSourceShape) (factflow.ValueSource, bool) {
	literal, ok := check.LiteralValue()
	if !ok {
		return factflow.ValueSource{}, false
	}
	// nil is the one scalar literal represented directly by a type singleton
	// rather than *typ.Literal. Preserve it as the canonical nil source so
	// normalized `x == nil` / `x ~= nil` branches retain their Boolean producer.
	if typ.TypeEquals(literal, typ.Nil) {
		return factflow.NewNilValueSource(factflow.NoValueSourceIndex), true
	}
	scalar, ok := literal.(*typ.Literal)
	if !ok {
		return factflow.ValueSource{}, false
	}
	switch value := scalar.Value.(type) {
	case bool:
		return factflow.NewBoolLiteralValueSource(value, 0, factflow.NoValueSourceIndex, 0, shape)
	case int64:
		return factflow.NewIntegerLiteralValueSource(value, 0, factflow.NoValueSourceIndex, 0, shape)
	case float64:
		return factflow.NewNumberLiteralValueSource(value, 0, factflow.NoValueSourceIndex, 0, shape)
	case string:
		return factflow.NewStringLiteralValueSource(value, 0, factflow.NoValueSourceIndex, 0, shape)
	default:
		return factflow.ValueSource{}, false
	}
}

func (l *lowerer) firstDirectBranchCheckFromWIR(point cfg.Point) (branchcond.Check, bool) {
	var out branchcond.Check
	var found bool
	l.wir.ForEachBranchCheck(point, func(check wir.Check) bool {
		candidate := branchCheckFromWIR(check)
		if candidate.Kind == branchcond.CheckNone || !l.branchCheckAuthorized(candidate) {
			return true
		}
		out = candidate
		found = true
		return false
	})
	return out, found
}

func branchCheckFromWIR(check wir.Check) branchcond.Check {
	return branchcond.Check{
		Kind:             branchcond.CheckKind(check.Kind),
		Path:             check.Path,
		OtherPath:        check.OtherPath,
		TypeName:         check.TypeName,
		Literal:          check.Literal,
		LiteralString:    check.LiteralString,
		LenFloor:         check.LenFloor,
		NumFloor:         check.NumFloor,
		NumCeil:          check.NumCeil,
		HasNumCeil:       check.HasNumCeil,
		NumCeilNegated:   check.NumCeilNegated,
		Negated:          check.Negated,
		ProducerPoint:    check.ProducerPoint,
		HasProducerPoint: check.HasProducerPoint,
	}
}

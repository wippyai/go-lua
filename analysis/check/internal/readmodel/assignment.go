package readmodel

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ForEachAssignment visits annotated local assignments in deterministic RPO
// order. It intentionally exposes only solved values and target contracts; the
// public readmodel owns report planning and obligation passes own judgment
// emission.
func (r Reader) ForEachAssignment(visit func(Assignment) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		if fact, ok := r.result.LocalAssignment(point); ok {
			if !r.forEachLocalAssignment(point, fact, visit, &visited) {
				return true
			}
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			if !r.forEachOrdinaryAssignment(point, fact, visit, &visited) {
				return true
			}
		}
	}
	return visited
}

// ForEachOptionalAssignmentTarget visits writes whose container may be nil.
func (r Reader) ForEachOptionalAssignmentTarget(visit func(OptionalAssignmentTarget) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	return r.result.ForEachOptionalAssignmentTargetOccurrence(func(fact body.OrdinaryAssignmentFact, occ body.OptionalAssignmentTargetOccurrence) bool {
		target := OptionalAssignmentTarget{
			Point:          occ.Point,
			ContainerLabel: occ.ContainerLabel,
			TargetLabel:    occ.TargetLabel,
			TargetKey:      r.assignmentTargetKeyForOrdinary(occ.Point, fact) + ":optional-target",
			ContainerType:  occ.ContainerType,
			ContainerSpan:  sourceSpanFromBody(occ.ContainerSpan),
			TargetSpan:     sourceSpanFromBody(occ.TargetSpan),
		}
		visited = true
		if !visit(target) {
			return true
		}
		return true
	}) || visited
}

func (r Reader) forEachLocalAssignment(point cfg.Point, fact body.LocalAssignmentFact, visit func(Assignment) bool, visited *bool) bool {
	if fact.Type == nil || (fact.Expr == nil && fact.Source.Kind == sourceprovenance.SourceExpression) {
		return true
	}
	if fact.Expr == nil && len(fact.Exprs) == 0 {
		// A declaration without an initializer defers the annotated contract
		// to later assignments; flow-sensitive use analysis owns the implicit
		// nil state. Only nil-filled targets of an initialized multi-assignment
		// remain assignment obligations.
		return true
	}
	expected, ok := r.result.LocalAssignmentExpectedType(point, fact)
	if !ok || !readapi.ObligationTypeReportable(expected) {
		return true
	}
	if entry, ok := r.assignmentObjectLiteralEntry(point, fact, expected); ok {
		*visited = true
		return visit(entry)
	}
	value, ok := r.localAssignmentSourceValue(point, fact)
	if !ok {
		return true
	}
	presentation := body.LocalAssignmentPresentationFor(fact)
	t, _ := r.assignmentSourceTypeForPresenceProof(point, r.result.AssignmentSourceReadProvenPresent(point, fact.Expr), value)
	missingField, missingFieldOK := assignmentMissingRequired(point, r, fact, expected)
	if missingFieldOK {
		if shape, shapeOK := r.assignmentObjectLiteralShapeType(point, fact); shapeOK {
			t = shape
		}
	}
	assignment := Assignment{
		Point:              point,
		TargetLabel:        fact.Name,
		SourceLabel:        presentation.SourceLabel,
		TargetKey:          assignmentTargetKey(fact),
		SourceKey:          r.result.AssignmentSourcePathKey(fact.Expr),
		SourceIndexedRead:  body.AssignmentSourceIndexedRead(fact.Expr),
		Value:              value,
		ValueHash:          r.ValueHash(value),
		TypeWithPresence:   t,
		Expected:           expected,
		ExpectedLabel:      presentation.ExpectedLabel,
		ExpectedSource:     readapi.AssignmentExpectedDeclared,
		SourceSpan:         sourceSpanFromBody(presentation.SourceSpan),
		DeclarationSpan:    sourceSpanFromBody(presentation.DeclarationSpan),
		NilableAccesses:    assignmentNilableAccessEvidenceFromBody(r.result.AssignmentNilableAccessEvidence(point, fact.Expr)),
		SourceContributors: assignmentSourceContributorsFromBody(r.result.AssignmentSourceContributions(point, fact.Expr)),
		CallInvalidations:  assignmentCallInvalidationsFromBody(r.result.AssignmentCallInvalidations(point, fact.Expr)),
		CallResult:         r.assignmentCallResultSource(fact.Source),
		UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
		ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
	}
	var missingFieldType typ.Type
	if missingFieldOK {
		missingFieldType, _ = body.ExpectedTypeAtSegments(expected, []segment.Segment{{Kind: segment.SegmentField, Name: missingField}})
	}
	interfaceMismatch, hasInterfaceMismatch := subtype.RecordInterfaceMismatch(t, expected)
	assignmentPlan := readapi.AssignmentCheckPlan{
		Assignment:               assignment,
		ValueAdmissible:          r.ValueProofAdmissible(value, expected),
		ValueProvenMismatch:      r.ValueWitnessProvenMismatch(value, expected),
		MissingRequiredField:     missingField,
		MissingRequiredFieldType: missingFieldType,
		IsSubtype:                r.IsSubtype,
	}
	if hasInterfaceMismatch {
		switch interfaceMismatch.Kind {
		case subtype.InterfaceMismatchMissingMethod:
			assignmentPlan.MissingRequiredMethod = interfaceMismatch.Method.Name
			assignmentPlan.MissingRequiredMethodType = interfaceMismatch.Expected
		case subtype.InterfaceMismatchMethodType:
			assignmentPlan.MethodMismatchName = interfaceMismatch.Method.Name
			assignmentPlan.MethodMismatchExpected = interfaceMismatch.Expected
			assignmentPlan.MethodMismatchActual = interfaceMismatch.Actual
		}
	}
	assignment.Check = readapi.PlanAssignmentCheck(assignmentPlan)
	*visited = true
	return visit(assignment)
}

func (r Reader) localAssignmentSourceValue(point cfg.Point, fact body.LocalAssignmentFact) (product.Value, bool) {
	var value product.Value
	var ok bool
	value, ok = r.SourceValue(point, fact.Source)
	if ok {
		return r.result.WithMemberReadNilWitness(point, fact.Expr, value), true
	}
	if r.callResultSourceUnderSupplied(fact.Source) {
		if reg := r.result.Registry(); reg != nil {
			return product.Absent(reg), true
		}
	}
	if fact.Source.Kind == sourceprovenance.SourceExpression && fact.Expr != nil {
		if value, ok = r.result.ExpressionValueBeforeBoundary(point, fact.Expr); ok {
			return r.result.WithMemberReadNilWitness(point, fact.Expr, value), true
		}
	}
	if !ok {
		if fact.Expr != nil {
			value, ok = r.result.ExpressionValueBeforeBoundary(point, fact.Expr)
			if ok {
				return r.result.WithMemberReadNilWitness(point, fact.Expr, value), true
			}
		}
		return product.Value{}, false
	}
	return r.result.WithMemberReadNilWitness(point, fact.Expr, value), true
}

func assignmentNilableAccessEvidenceFromBody(evidence []body.NilableAccessEvidence) []readapi.NilableAccessEvidence {
	if len(evidence) == 0 {
		return nil
	}
	out := make([]readapi.NilableAccessEvidence, 0, len(evidence))
	for _, item := range evidence {
		out = append(out, readapi.NilableAccessEvidence{
			Label:  item.Label,
			Access: item.Access,
			Span:   sourceSpanFromBody(item.Span),
		})
	}
	return out
}

func assignmentSourceContributorsFromBody(contributors []body.AssignmentSourceContribution) []readapi.AssignmentSourceContribution {
	if len(contributors) == 0 {
		return nil
	}
	out := make([]readapi.AssignmentSourceContribution, 0, len(contributors))
	for _, contribution := range contributors {
		out = append(out, readapi.AssignmentSourceContribution{
			RootLabel: contribution.RootLabel,
			ReadLabel: contribution.ReadLabel,
			Type:      contribution.Type,
			Span:      sourceSpanFromBody(contribution.Span),
		})
	}
	return out
}

func assignmentCallInvalidationsFromBody(invalidations []body.AssignmentCallInvalidation) []readapi.AssignmentCallInvalidation {
	if len(invalidations) == 0 {
		return nil
	}
	out := make([]readapi.AssignmentCallInvalidation, 0, len(invalidations))
	for _, invalidation := range invalidations {
		out = append(out, readapi.AssignmentCallInvalidation{
			CallLabel:        invalidation.CallLabel,
			ReadLabel:        invalidation.ReadLabel,
			InvalidatedLabel: invalidation.InvalidatedLabel,
			Span:             sourceSpanFromBody(invalidation.Span),
		})
	}
	return out
}

func (r Reader) forEachOrdinaryAssignment(point cfg.Point, fact body.OrdinaryAssignmentFact, visit func(Assignment) bool, visited *bool) bool {
	if fact.Target == nil || fact.Value == nil {
		return true
	}
	target, ok := r.ordinaryAssignmentTarget(point, fact)
	if !ok {
		return true
	}
	expected := r.ordinaryWritableTargetType(target.Type, target.TargetValue, target.HasValue, target.Declared)
	if !readapi.ObligationTypeReportable(expected) {
		return true
	}
	value, ok := r.ordinaryAssignmentSourceValue(point, fact)
	if !ok {
		return true
	}
	presentation := body.OrdinaryAssignmentPresentationFor(fact)
	if literal, ok := r.result.ObjectLiteral(fact.Value); ok {
		if entry, ok := r.assignmentObjectLiteralEntryCandidate(point, literal, expected, assignmentObjectEntryTarget{
			Label:          presentation.TargetLabel,
			Key:            r.assignmentTargetKeyForOrdinary(point, fact),
			ExpectedSpan:   sourceSpanFromBody(presentation.TargetSpan),
			ExpectedSource: readapi.AssignmentExpectedDynamicTarget,
		}); ok {
			*visited = true
			return visit(entry)
		}
	}
	t, _ := r.assignmentSourceTypeForPresenceProof(point, r.result.AssignmentSourceReadProvenPresent(point, fact.Value), value)
	if r.inferredReplacementAccepted(point, target, expected, t) {
		return true
	}
	assignment := Assignment{
		Point:              point,
		TargetLabel:        presentation.TargetLabel,
		SourceLabel:        presentation.SourceLabel,
		TargetKey:          r.assignmentTargetKeyForOrdinary(point, fact),
		SourceKey:          r.result.AssignmentSourcePathKey(fact.Value),
		SourceIndexedRead:  body.AssignmentSourceIndexedRead(fact.Value),
		Value:              value,
		ValueHash:          r.ValueHash(value),
		TypeWithPresence:   t,
		Expected:           expected,
		ExpectedSource:     ordinaryAssignmentExpectedSource(presentation.DynamicTarget),
		SourceSpan:         sourceSpanFromBody(presentation.SourceSpan),
		DeclarationSpan:    sourceSpanFromBody(presentation.TargetSpan),
		NilableAccesses:    assignmentNilableAccessEvidenceFromBody(r.result.AssignmentNilableAccessEvidence(point, fact.Value)),
		SourceContributors: assignmentSourceContributorsFromBody(r.result.AssignmentSourceContributions(point, fact.Value)),
		CallInvalidations:  assignmentCallInvalidationsFromBody(r.result.AssignmentCallInvalidations(point, fact.Value)),
		UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
		ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
	}
	assignment.Check = readapi.PlanAssignmentCheck(readapi.AssignmentCheckPlan{
		Assignment:          assignment,
		ValueAdmissible:     r.ValueProofAdmissible(value, expected),
		ValueProvenMismatch: r.ValueWitnessProvenMismatch(value, expected),
		IsSubtype:           r.IsSubtype,
	})
	if assignment.Check.Admissible {
		return true
	}
	*visited = true
	return visit(assignment)
}

func ordinaryAssignmentExpectedSource(dynamic bool) readapi.AssignmentExpectedSource {
	if dynamic {
		return readapi.AssignmentExpectedDynamicTarget
	}
	return readapi.AssignmentExpectedDeclared
}

func (r Reader) ordinaryAssignmentSourceValue(point cfg.Point, fact body.OrdinaryAssignmentFact) (product.Value, bool) {
	if fact.Source.Kind == sourceprovenance.SourceExpression && fact.Value != nil {
		if value, ok := r.result.ExpressionValueBeforeBoundary(point, fact.Value); ok {
			return r.result.WithMemberReadNilWitness(point, fact.Value, value), true
		}
	}
	if t, ok := body.LiteralExpressionType(fact.Value); ok && r.result != nil && r.result.Registry() != nil {
		base := typevalue.FromType(r.result.Registry(), t)
		return typevalue.WithWitness(r.result.Registry(), base, t), true
	}
	if value, ok := r.SourceValue(point, fact.Source); ok {
		return r.result.WithMemberReadNilWitness(point, fact.Value, value), true
	}
	if fact.Value != nil {
		if value, ok := r.result.ExpressionValueBeforeBoundary(point, fact.Value); ok {
			return r.result.WithMemberReadNilWitness(point, fact.Value, value), true
		}
	}
	return product.Value{}, false
}

func (r Reader) assignmentSourceType(point cfg.Point, value product.Value) (typ.Type, bool) {
	if fn, ok := r.result.FunctionValueTypeForValueAtBoundary(point, value); ok {
		return fn, true
	}
	return r.ValueTypeWithPresence(value)
}

func (r Reader) assignmentSourceTypeForPresenceProof(point cfg.Point, provenPresent bool, value product.Value) (typ.Type, bool) {
	t, ok := r.assignmentSourceType(point, value)
	if !ok || t == nil || !provenPresent {
		return t, ok
	}
	if withoutNil := body.ProjectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
		return withoutNil, true
	}
	return t, true
}

func assignmentTargetKey(fact body.LocalAssignmentFact) string {
	if fact.HasSymbol && fact.Symbol != 0 {
		return "sym:" + strconv.FormatUint(uint64(fact.Symbol), 10)
	}
	return "local:" + fact.Name + ":" + strconv.Itoa(fact.Index)
}

func (r Reader) assignmentTargetKeyForOrdinary(point cfg.Point, fact body.OrdinaryAssignmentFact) string {
	if fact.HasPath && !fact.Path.IsEmpty() {
		return "path:" + fact.Path.String()
	}
	if r.result != nil {
		if key := r.result.OrdinaryAssignmentTargetPathKey(fact); key != "" {
			return key
		}
	}
	return "ordinary:" + strconv.Itoa(int(point)) + ":" + strconv.Itoa(fact.Index)
}

func (r Reader) ordinaryAssignmentTarget(point cfg.Point, fact body.OrdinaryAssignmentFact) (body.OrdinaryAssignmentTargetType, bool) {
	target, ok := r.result.OrdinaryAssignmentTargetTypeAt(point, fact)
	if !ok {
		return body.OrdinaryAssignmentTargetType{}, false
	}
	return target, true
}

func (r Reader) ordinaryAssignmentTargetType(point cfg.Point, fact body.OrdinaryAssignmentFact) (typ.Type, bool) {
	target, ok := r.ordinaryAssignmentTarget(point, fact)
	if !ok {
		return nil, false
	}
	return r.ordinaryWritableTargetType(target.Type, target.TargetValue, target.HasValue, target.Declared), true
}

func (r Reader) ordinaryWritableTargetType(current typ.Type, targetValue product.Value, hasValue bool, declared bool) typ.Type {
	if current == nil {
		return nil
	}
	if hasValue {
		if family, familyOK := r.FullVariantOriginType(targetValue); familyOK && family != nil && r.IsSubtype(current, family) {
			return family
		}
	}
	if !declared {
		if base, ok := body.TypeFamilyBase(current); ok && base != nil {
			return base
		}
	}
	return current
}

func (r Reader) inferredReplacementAccepted(_ cfg.Point, target body.OrdinaryAssignmentTargetType, expected, actual typ.Type) bool {
	if expected == nil || actual == nil {
		return false
	}
	if target.Declared {
		return false
	}
	if _, ok := unwrap.Annotated(expected).(*typ.Function); ok {
		_, actualOK := unwrap.Annotated(actual).(*typ.Function)
		return actualOK
	}
	if _, ok := unwrap.Annotated(expected).(*typ.Record); ok {
		return inferredRecordReplacementAccepted(actual)
	}
	return false
}

func inferredRecordReplacementAccepted(actual typ.Type) bool {
	switch t := unwrap.Annotated(actual).(type) {
	case *typ.Record:
		return true
	case *typ.Optional:
		return false
	case *typ.Union:
		if len(t.Members) == 0 {
			return false
		}
		for _, member := range t.Members {
			if !inferredRecordReplacementAccepted(member) {
				return false
			}
		}
		return true
	default:
		return typetable.IsLike(t)
	}
}

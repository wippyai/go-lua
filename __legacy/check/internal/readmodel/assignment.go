package readmodel

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
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
	refutedTargets := make(map[string]assignmentCascadeCause)
	refutedDynamicRoots := make(map[string]assignmentCascadeCause)
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		if fact, ok := r.result.LocalAssignment(point); ok {
			if !r.forEachLocalAssignment(point, fact, visit, &visited, refutedTargets, refutedDynamicRoots) {
				return true
			}
		}
		if fact, ok := r.result.OrdinaryAssignment(point); ok {
			if !r.forEachOrdinaryAssignment(point, fact, visit, &visited, refutedTargets, refutedDynamicRoots) {
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
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		write, ok := r.result.LoweredAssignmentWrite(point)
		if !ok {
			continue
		}
		target, ok := r.optionalAssignmentTarget(point, write)
		if !ok {
			continue
		}
		visited = true
		if !visit(target) {
			return true
		}
	}
	return visited
}

func (r Reader) optionalAssignmentTarget(point cfg.Point, write body.LoweredAssignmentWrite) (OptionalAssignmentTarget, bool) {
	if write.Target.IsEmpty() {
		return OptionalAssignmentTarget{}, false
	}
	container, containerType, ok := r.optionalAssignmentContainer(point, write)
	if !ok {
		return OptionalAssignmentTarget{}, false
	}
	return OptionalAssignmentTarget{
		Point:          point,
		ContainerLabel: r.displayPath(container),
		TargetLabel:    r.displayPathCanonical(write.Target),
		TargetKey:      "path:" + string(write.Target.Key()) + ":optional-target",
		ContainerType:  containerType,
		ContainerSpan:  sourceSpanFromBody(write.ContainerSpan),
		TargetSpan:     sourceSpanFromBody(write.Span),
	}, true
}

func (r Reader) optionalAssignmentContainer(point cfg.Point, write body.LoweredAssignmentWrite) (bodyPath pathdom.Path, containerType typ.Type, ok bool) {
	candidates := optionalAssignmentContainerCandidates(write)
	for _, candidate := range candidates {
		t, typeOK := r.optionalAssignmentContainerType(point, candidate)
		if typeOK {
			return candidate, t, true
		}
	}
	return pathdom.Path{}, nil, false
}

func optionalAssignmentContainerCandidates(write body.LoweredAssignmentWrite) []pathdom.Path {
	var out []pathdom.Path
	add := func(candidate pathdom.Path) {
		if candidate.IsEmpty() {
			return
		}
		for _, existing := range out {
			if existing.Equal(candidate) {
				return
			}
		}
		out = append(out, candidate)
	}
	if write.HasContainer {
		add(write.Container)
	}
	for i := len(write.Target.Segments) - 1; i >= 0; i-- {
		prefix := write.Target.RootOnly().AppendSegments(write.Target.Segments[:i])
		add(prefix)
	}
	return out
}

func (r Reader) optionalAssignmentContainerType(point cfg.Point, container pathdom.Path) (typ.Type, bool) {
	value, valueOK := r.result.PathValueBeforeBoundary(point, container)
	if valueOK && presence.Equal(product.PresenceOf(value), presence.Present()) {
		return nil, false
	}
	var containerType typ.Type
	var ok bool
	if valueOK {
		containerType, ok = r.ValueTypeWithPresence(value)
	}
	if !ok || containerType == nil {
		containerType, ok = r.result.DeclaredPathTypeAt(point, container, true)
	}
	if !ok || containerType == nil ||
		typ.IsAny(containerType) ||
		typ.IsUnknown(containerType) ||
		typ.IsNever(containerType) ||
		!typevalue.ProjectionHasNil(containerType) {
		return nil, false
	}
	if r.result.PathProvenPresentBeforeBoundary(point, container) ||
		r.result.DominatingRequiredMemberReadProvesPathPresent(point, container) {
		return nil, false
	}
	return containerType, true
}

func (r Reader) forEachLocalAssignment(
	point cfg.Point,
	fact body.LocalAssignmentFact,
	visit func(Assignment) bool,
	visited *bool,
	refutedTargets map[string]assignmentCascadeCause,
	refutedDynamicRoots map[string]assignmentCascadeCause,
) bool {
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
		entry.CascadeFromRefuted = r.assignmentIsCascadeFromRefuted(entry, refutedTargets, refutedDynamicRoots)
		r.recordRefutedAssignmentCascade(entry, refutedTargets, refutedDynamicRoots)
		*visited = true
		return visit(entry)
	}
	value, ok := r.localAssignmentSourceValue(point, fact)
	if !ok {
		return true
	}
	presentation := body.LocalAssignmentPresentationFor(fact)
	t, _ := r.assignmentSourceTypeForPresenceProof(point, r.result.AssignmentSourceReadProvenPresent(point, fact.Expr), fact.Source, value)
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
		SourceIndexedRead:  r.result.AssignmentSourceIndexedReadAt(point, fact.Expr),
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
		RuntimeValidated:   r.ValueHasRuntimeValidationProof(value),
	}
	var missingFieldType typ.Type
	if missingFieldOK {
		missingFieldType, _ = luatypeprojection.ExpectedTypeAtSegments(expected, []segment.Segment{{Kind: segment.SegmentField, Name: missingField}})
	}
	interfaceMismatch, hasInterfaceMismatch := r.result.RecordInterfaceMismatch(t, expected)
	assignmentPlan := readapi.AssignmentCheckPlan{
		Assignment:               assignment,
		ValueAdmissible:          r.ValueProofAdmissible(value, expected),
		ValueProvenMismatch:      r.ValueWitnessProvenMismatch(value, expected),
		MayBeNil:                 assignmentTopLikeNilableAccess(assignment.TypeWithPresence, assignment.NilableAccesses),
		MissingRequiredField:     missingField,
		MissingRequiredFieldType: missingFieldType,
		IsSubtype:                r.IsSubtype,
	}
	if hasInterfaceMismatch {
		switch interfaceMismatch.Kind {
		case body.InterfaceMismatchMissingMethod:
			assignmentPlan.MissingRequiredMethod = interfaceMismatch.MethodName
			assignmentPlan.MissingRequiredMethodType = interfaceMismatch.Expected
		case body.InterfaceMismatchMethodType:
			assignmentPlan.MethodMismatchName = interfaceMismatch.MethodName
			assignmentPlan.MethodMismatchExpected = interfaceMismatch.Expected
			assignmentPlan.MethodMismatchActual = interfaceMismatch.Actual
		}
	}
	assignment.Check = readapi.PlanAssignmentCheck(assignmentPlan)
	assignment.CascadeFromRefuted = r.assignmentIsCascadeFromRefuted(assignment, refutedTargets, refutedDynamicRoots)
	r.recordRefutedAssignmentCascade(assignment, refutedTargets, refutedDynamicRoots)
	*visited = true
	return visit(assignment)
}

func (r Reader) localAssignmentSourceValue(point cfg.Point, fact body.LocalAssignmentFact) (product.Value, bool) {
	if call, ok := fact.Expr.(*ast.FuncCallExpr); ok && r.result.UnresolvedStaticCalleeCall(call) {
		return product.Value{}, false
	}
	var value product.Value
	var ok bool
	explanation, explanationOK := r.result.LocalAssignmentSourceValueForExplanationAtBoundary(point, fact.Source)
	lowered, loweredOK := r.result.LocalAssignmentSourceValueAtBoundary(point, fact.Source)
	generic, genericOK := r.SourceValue(point, fact.Source)
	switch {
	case loweredOK && genericOK:
		value = r.result.PreferredLocalAssignmentSourceValue(lowered, generic)
		ok = true
	case loweredOK:
		value, ok = lowered, true
	case genericOK:
		value, ok = generic, true
	}
	if ok {
		if explanationOK && r.result.ExplanationValueShouldReplaceAssignmentSource(value, explanation) {
			value = explanation
		}
		value = r.result.WithMemberReadNilWitness(point, fact.Expr, value)
		return value, true
	}
	if explanationOK {
		value = explanation
		value = r.result.WithMemberReadNilWitness(point, fact.Expr, value)
		return value, true
	}
	if r.callResultSourceUnderSupplied(fact.Source) {
		if reg := r.result.Registry(); reg != nil {
			return product.Absent(reg), true
		}
	}
	if fact.Source.Kind == sourceprovenance.SourceExpression && fact.Expr != nil {
		if value, ok = r.result.ExpressionValueBeforeBoundary(point, fact.Expr); ok {
			value = r.result.WithMemberReadNilWitness(point, fact.Expr, value)
			return value, true
		}
	}
	if !ok {
		if fact.Expr != nil {
			value, ok = r.result.ExpressionValueBeforeBoundary(point, fact.Expr)
			if ok {
				value = r.result.WithMemberReadNilWitness(point, fact.Expr, value)
				return value, true
			}
		}
		return product.Value{}, false
	}
	value = r.result.WithMemberReadNilWitness(point, fact.Expr, value)
	return value, true
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

func (r Reader) forEachOrdinaryAssignment(
	point cfg.Point,
	fact body.OrdinaryAssignmentFact,
	visit func(Assignment) bool,
	visited *bool,
	refutedTargets map[string]assignmentCascadeCause,
	refutedDynamicRoots map[string]assignmentCascadeCause,
) bool {
	if fact.Target == nil || fact.Value == nil {
		return true
	}
	target, ok := r.ordinaryAssignmentTarget(point, fact)
	if !ok {
		return true
	}
	expected := r.result.OrdinaryWritableTargetType(target.Type, target.TargetValue, target.HasValue, target.Declared)
	if !readapi.ObligationTypeReportable(expected) {
		return true
	}
	value, ok := r.ordinaryAssignmentSourceValue(point, fact)
	if !ok {
		return true
	}
	presentation := body.OrdinaryAssignmentPresentationFor(fact)
	t, _ := r.assignmentSourceTypeForPresenceProof(point, r.result.AssignmentSourceReadProvenPresent(point, fact.Value), fact.Source, value)
	if ordinaryDynamicNilDeletionAccepted(presentation.DynamicTarget, target.NilDeletionAllowed, t, r.ValueHasUntrustedTopOrigin(value)) {
		return true
	}
	if r.inferredReplacementAccepted(point, target, expected, t) {
		return true
	}
	if write, ok := r.result.LoweredAssignmentWrite(point); ok {
		if literal, ok := r.result.ObjectLiteralViewForSource(write.Source); ok {
			if entry, ok := r.assignmentObjectLiteralEntryCandidate(point, literal, expected, assignmentObjectEntryTarget{
				Label:          presentation.TargetLabel,
				Key:            r.assignmentTargetKeyForOrdinary(point, fact),
				ExpectedSpan:   sourceSpanFromBody(presentation.TargetSpan),
				ExpectedSource: readapi.AssignmentExpectedDynamicTarget,
			}); ok {
				entry.CascadeFromRefuted = r.assignmentIsCascadeFromRefuted(entry, refutedTargets, refutedDynamicRoots)
				r.recordRefutedAssignmentCascade(entry, refutedTargets, refutedDynamicRoots)
				*visited = true
				return visit(entry)
			}
		}
	}
	assignment := Assignment{
		Point:              point,
		TargetLabel:        presentation.TargetLabel,
		SourceLabel:        presentation.SourceLabel,
		TargetKey:          r.assignmentTargetKeyForOrdinary(point, fact),
		SourceKey:          r.result.AssignmentSourcePathKey(fact.Value),
		SourceIndexedRead:  r.result.AssignmentSourceIndexedReadAt(point, fact.Value),
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
		RuntimeValidated:   r.ValueHasRuntimeValidationProof(value),
	}
	assignment.Check = readapi.PlanAssignmentCheck(readapi.AssignmentCheckPlan{
		Assignment:          assignment,
		ValueAdmissible:     r.ValueProofAdmissible(value, expected) || r.dynamicTargetReadProofAdmissible(point, fact, value),
		ValueProvenMismatch: r.ValueWitnessProvenMismatch(value, expected),
		MayBeNil:            assignmentTopLikeNilableAccess(assignment.TypeWithPresence, assignment.NilableAccesses),
		IsSubtype:           r.IsSubtype,
	})
	assignment.CascadeFromRefuted = r.assignmentIsCascadeFromRefuted(assignment, refutedTargets, refutedDynamicRoots)
	r.recordRefutedAssignmentCascade(assignment, refutedTargets, refutedDynamicRoots)
	if assignment.Check.Admissible {
		return true
	}
	*visited = true
	return visit(assignment)
}

type assignmentCascadeCause struct {
	point cfg.Point
	span  readapi.SourceSpan
}

func (r Reader) assignmentIsCascadeFromRefuted(assignment Assignment, refutedTargets map[string]assignmentCascadeCause, refutedDynamicRoots map[string]assignmentCascadeCause) bool {
	if assignment.Check.Admissible {
		return false
	}
	if assignment.SourceLabel == "" || assignment.TargetLabel == assignment.SourceLabel {
		return false
	}
	if cause, ok := refutedTargets[assignment.SourceLabel]; ok && assignmentCascadeCausePrecedes(cause, assignment) {
		return true
	}
	if assignmentSourceCoveredByDynamicRoot(assignment.SourceKey, refutedDynamicRoots, assignment) {
		return true
	}
	return false
}

func (r Reader) recordRefutedAssignmentCascade(assignment Assignment, refutedTargets map[string]assignmentCascadeCause, refutedDynamicRoots map[string]assignmentCascadeCause) {
	if !assignment.Check.ProvenMismatch || assignment.TargetLabel == "" {
		return
	}
	cause := assignmentCascadeCause{point: assignment.Point, span: assignment.SourceSpan}
	refutedTargets[assignment.TargetLabel] = cause
	if root, ok := assignmentRefutedDynamicTargetRoot(assignment); ok {
		refutedDynamicRoots[root] = cause
	}
}

func assignmentRefutedDynamicTargetRoot(assignment Assignment) (string, bool) {
	if assignment.ExpectedSource != readapi.AssignmentExpectedDynamicTarget || assignment.TargetKey == "" {
		return "", false
	}
	return assignment.TargetKey, true
}

func assignmentCascadeCausePrecedes(cause assignmentCascadeCause, item Assignment) bool {
	if cause.point < item.Point {
		return true
	}
	if item.SourceSpan.StartLine == 0 || cause.span.StartLine == 0 {
		return false
	}
	return spanBefore(cause.span, item.SourceSpan)
}

func assignmentSourceCoveredByDynamicRoot(sourceKey string, roots map[string]assignmentCascadeCause, item Assignment) bool {
	if sourceKey == "" || len(roots) == 0 {
		return false
	}
	for root, cause := range roots {
		if !assignmentCascadeCausePrecedes(cause, item) {
			continue
		}
		if sourceKey == root || strings.HasPrefix(sourceKey, root+".") || strings.HasPrefix(sourceKey, root+"[") {
			return true
		}
	}
	return false
}

func spanBefore(a, b readapi.SourceSpan) bool {
	if a.StartLine != b.StartLine {
		return a.StartLine < b.StartLine
	}
	return a.StartCol < b.StartCol
}

func assignmentTopLikeNilableAccess(t typ.Type, accesses []readapi.NilableAccessEvidence) bool {
	return len(accesses) != 0 && (t == nil || typ.IsAny(t) || typ.IsUnknown(t))
}

func (r Reader) dynamicTargetReadProofAdmissible(point cfg.Point, fact body.OrdinaryAssignmentFact, value product.Value) bool {
	sourcePath, ok := r.result.ExpressionPath(fact.Value)
	return ok && !r.ValueHasUntrustedTopOrigin(value) && r.result.AssignmentSourcePathMatchesDynamicTargetRead(point, sourcePath)
}

func ordinaryAssignmentExpectedSource(dynamic bool) readapi.AssignmentExpectedSource {
	if dynamic {
		return readapi.AssignmentExpectedDynamicTarget
	}
	return readapi.AssignmentExpectedDeclared
}

func ordinaryDynamicNilDeletionAccepted(dynamicTarget bool, allowed bool, actual typ.Type, untrustedTopOrigin bool) bool {
	return dynamicTarget && allowed && !untrustedTopOrigin && actual != nil && typ.TypeEquals(actual, typ.Nil)
}

func (r Reader) ordinaryAssignmentSourceValue(point cfg.Point, fact body.OrdinaryAssignmentFact) (product.Value, bool) {
	if fact.Source.Kind == sourceprovenance.SourceExpression && fact.Value != nil {
		if value, ok := r.result.ExpressionValueBeforeBoundary(point, fact.Value); ok {
			value = r.result.WithMemberReadNilWitness(point, fact.Value, value)
			return value, true
		}
	}
	if t, ok := body.LiteralExpressionType(fact.Value); ok && r.result != nil && r.result.Registry() != nil {
		base := typevalue.FromType(r.result.Registry(), t)
		return typevalue.WithWitness(r.result.Registry(), base, t), true
	}
	if value, ok := r.SourceValue(point, fact.Source); ok {
		value = r.result.WithMemberReadNilWitness(point, fact.Value, value)
		return value, true
	}
	if fact.Value != nil {
		if value, ok := r.result.ExpressionValueBeforeBoundary(point, fact.Value); ok {
			value = r.result.WithMemberReadNilWitness(point, fact.Value, value)
			return value, true
		}
	}
	return product.Value{}, false
}

func (r Reader) assignmentSourceType(point cfg.Point, value product.Value) (typ.Type, bool) {
	if fn, ok := r.result.FunctionValueTypeForValue(value); ok {
		return fn, true
	}
	return r.ValueTypeWithPresence(value)
}

func (r Reader) assignmentSourceTypeForPresenceProof(point cfg.Point, provenPresent bool, source sourceprovenance.ASTSource, value product.Value) (typ.Type, bool) {
	expr := source.Expr
	t, ok := r.assignmentSourceType(point, value)
	if (!ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t)) && r.result != nil && expr != nil {
		if declared, declaredOK := r.result.DeclaredExpressionTypeAt(point, expr); declaredOK &&
			declared != nil &&
			!typ.IsAny(declared) &&
			!typ.IsUnknown(declared) {
			t = declared
			ok = true
		}
	}
	if !ok || t == nil || !provenPresent ||
		(!presence.Equal(product.PresenceOf(value), presence.Present()) && !r.assignmentSourceIsIndexedReadAt(point, source)) {
		return t, ok
	}
	if withoutNil := body.ProjectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
		return withoutNil, true
	}
	return t, true
}

func (r Reader) assignmentSourceIsIndexedReadAt(point cfg.Point, source sourceprovenance.ASTSource) bool {
	return source.Kind == sourceprovenance.SourceExpression &&
		source.Expr != nil &&
		r.result != nil &&
		r.result.AssignmentSourceIndexedReadAt(point, source.Expr)
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
	return r.result.OrdinaryWritableTargetType(target.Type, target.TargetValue, target.HasValue, target.Declared), true
}

func (r Reader) inferredReplacementAccepted(point cfg.Point, target body.OrdinaryAssignmentTargetType, expected, actual typ.Type) bool {
	return r.result.InferredReplacementAccepted(point, target, expected, actual)
}

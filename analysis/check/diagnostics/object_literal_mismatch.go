package diagnostics

import (
	"fmt"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// closedRecordUnionArms returns the closed record arms of a union want. A table
// literal assigned to such a union must satisfy at least one arm in full, so the
// admissibility check ranges over arms rather than projecting fields across the
// union (which would conflate field types from arms the literal never matches).
func closedRecordUnionArms(want typ.Type) ([]*typ.Record, bool) {
	union, ok := transparentExpectedType(want).(*typ.Union)
	if !ok {
		return nil, false
	}
	arms := make([]*typ.Record, 0, len(union.Members))
	for _, member := range union.Members {
		record, ok := closedRecord(member)
		if !ok {
			return nil, false
		}
		arms = append(arms, record)
	}
	if len(arms) < 2 {
		return nil, false
	}
	return arms, true
}

func objectLiteralAdmissibleToAnyArm(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, arms []*typ.Record, fact semantics.ObjectLiteralFact, env guardEnv) bool {
	for _, arm := range arms {
		if objectLiteralAdmissibleToRecord(result, resolver, point, arm, fact, env) {
			return true
		}
	}
	return false
}

// objectLiteralAdmissibleToRecord reports whether the literal could satisfy one
// closed record arm: every present entry matches the arm's expected type and no
// required field is missing. It mirrors the per-arm decisions the single-record
// path makes, so a literal that clears every arm check stays accepted.
func objectLiteralAdmissibleToRecord(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, record *typ.Record, fact semantics.ObjectLiteralFact, env guardEnv) bool {
	for _, entry := range fact.Entries {
		expected, ok := expectedTypeAtSegments(record, entry.Suffix.Segments)
		if !ok {
			continue
		}
		if _, mismatch := objectLiteralEntryMismatchType(result, resolver, point, entry, expected, env); mismatch {
			return false
		}
	}
	if _, ok := missingRequiredRecordField(record, fact); ok {
		return false
	}
	return true
}

func missingRequiredRecordField(want typ.Type, fact semantics.ObjectLiteralFact) (typ.Field, bool) {
	record, ok := closedRecord(want)
	if !ok {
		return typ.Field{}, false
	}
	present := make(map[string]struct{}, len(fact.Entries))
	for _, entry := range fact.Entries {
		if len(entry.Suffix.Segments) != 1 {
			continue
		}
		seg := entry.Suffix.Segments[0]
		if seg.Kind == segment.SegmentField && seg.Name != "" {
			present[seg.Name] = struct{}{}
		}
	}
	for _, field := range record.Fields {
		if field.Optional || unwrap.IsOptionalLike(field.Type) {
			continue
		}
		if _, ok := present[field.Name]; ok {
			continue
		}
		return field, true
	}
	return typ.Field{}, false
}

func closedRecord(t typ.Type) (*typ.Record, bool) {
	record, ok := transparentExpectedType(t).(*typ.Record)
	if !ok || record == nil || record.Open {
		return nil, false
	}
	return record, true
}

type objectLiteralTypeMismatch struct {
	expr             ast.Expr
	got              typ.Type
	want             typ.Type
	suffix           string
	segments         []segment.Segment
	missingField     string
	missingMethod    typ.Method
	unionArmEvidence []diagnostic.Evidence
	readBoundary     boundaryValueReader
}

// missingMemberEvidence returns the missing-required-member evidence for a
// mismatch raised because the literal omits a required field or interface
// method, or nil when the mismatch is an ordinary member type mismatch.
func (m objectLiteralTypeMismatch) missingMemberEvidence() []diagnostic.Evidence {
	switch {
	case m.missingField != "":
		return []diagnostic.Evidence{{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    ast.SpanOf(m.expr),
			Message: missingRequiredFieldEvidence(m.missingField),
		}}
	case m.missingMethod.Name != "":
		return []diagnostic.Evidence{{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    ast.SpanOf(m.expr),
			Message: missingRequiredMethodTypeEvidence(m.want, m.missingMethod),
		}}
	default:
		return nil
	}
}

func objectLiteralMemberMismatch(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr, want typ.Type, env guardEnv) (objectLiteralTypeMismatch, bool) {
	if result == nil || expr == nil || want == nil {
		return objectLiteralTypeMismatch{}, false
	}
	fact, ok := result.ObjectLiteral(expr)
	if !ok {
		return objectLiteralTypeMismatch{}, false
	}
	if arms, ok := closedRecordUnionArms(want); ok {
		if objectLiteralAdmissibleToAnyArm(result, resolver, point, arms, fact, env) {
			return objectLiteralTypeMismatch{}, false
		}
		if mismatch, ok, handled := objectLiteralMemberMismatchInLiteralConsistentArm(result, resolver, point, fact, arms, env); handled {
			return mismatch, ok
		}
		for _, arm := range arms {
			if mismatch, ok := objectLiteralMemberMismatchInFact(result, resolver, point, fact, arm, env); ok {
				return mismatch, true
			}
		}
		return objectLiteralTypeMismatch{expr: expr, got: objectLiteralType(want, fact), want: want, unionArmEvidence: objectLiteralUnionArmEvidence(result, resolver, point, fact, arms, env)}, true
	}
	return objectLiteralMemberMismatchInFact(result, resolver, point, fact, want, env)
}

func objectLiteralMemberMismatchWithValueSources(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr, want typ.Type, env guardEnv, literal factflow.ObjectLiteral) (objectLiteralTypeMismatch, bool) {
	if result == nil || expr == nil || want == nil {
		return objectLiteralTypeMismatch{}, false
	}
	fact, ok := result.ObjectLiteral(expr)
	if !ok {
		return objectLiteralTypeMismatch{}, false
	}
	sourceForSegments := objectLiteralEntrySourceResolver(literal)
	if arms, ok := closedRecordUnionArms(want); ok {
		if objectLiteralAdmissibleToAnyArmWithSources(result, resolver, point, arms, fact, env, sourceForSegments) {
			return objectLiteralTypeMismatch{}, false
		}
		if mismatch, ok, handled := objectLiteralMemberMismatchInLiteralConsistentArmWithSources(result, resolver, point, fact, arms, env, sourceForSegments); handled {
			return mismatch, ok
		}
		for _, arm := range arms {
			if mismatch, ok := objectLiteralMemberMismatchInFactWithSources(result, resolver, point, fact, arm, env, sourceForSegments); ok {
				return mismatch, true
			}
		}
		return objectLiteralTypeMismatch{expr: expr, got: objectLiteralType(want, fact), want: want, unionArmEvidence: objectLiteralUnionArmEvidence(result, resolver, point, fact, arms, env)}, true
	}
	return objectLiteralMemberMismatchInFactWithSources(result, resolver, point, fact, want, env, sourceForSegments)
}

type objectLiteralEntrySourceResolverFunc func([]segment.Segment) (factflow.ValueSource, bool)

func objectLiteralEntrySourceResolver(literal factflow.ObjectLiteral) objectLiteralEntrySourceResolverFunc {
	view := literal.View()
	return func(segments []segment.Segment) (factflow.ValueSource, bool) {
		var out factflow.ValueSource
		found := false
		view.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			if slices.Equal(entry.SuffixSegmentsView(), segments) {
				out = entry.Source()
				found = true
				return false
			}
			return true
		})
		return out, found
	}
}

func objectLiteralAdmissibleToAnyArmWithSources(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, arms []*typ.Record, fact semantics.ObjectLiteralFact, env guardEnv, sourceForSegments objectLiteralEntrySourceResolverFunc) bool {
	for _, arm := range arms {
		if objectLiteralAdmissibleToRecordWithSources(result, resolver, point, arm, fact, env, sourceForSegments) {
			return true
		}
	}
	return false
}

func objectLiteralAdmissibleToRecordWithSources(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, record *typ.Record, fact semantics.ObjectLiteralFact, env guardEnv, sourceForSegments objectLiteralEntrySourceResolverFunc) bool {
	for _, entry := range fact.Entries {
		expected, ok := expectedTypeAtSegments(record, entry.Suffix.Segments)
		if !ok {
			continue
		}
		source, hasSource := sourceForSegments(entry.Suffix.Segments)
		if _, _, mismatch := objectLiteralEntryMismatchTypeWithSource(result, resolver, point, entry, expected, env, source, hasSource); mismatch {
			return false
		}
	}
	if _, ok := missingRequiredRecordField(record, fact); ok {
		return false
	}
	return true
}

func objectLiteralMemberMismatchInLiteralConsistentArm(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.ObjectLiteralFact, arms []*typ.Record, env guardEnv) (objectLiteralTypeMismatch, bool, bool) {
	sawConsistentArm := false
	for _, arm := range arms {
		consistent, sawLiteral := objectLiteralArmConsistentWithLiteralEntries(result, fact, arm)
		if !sawLiteral || !consistent {
			continue
		}
		sawConsistentArm = true
		if mismatch, ok := objectLiteralMemberMismatchInFact(result, resolver, point, fact, arm, env); ok {
			return mismatch, true, true
		}
	}
	if sawConsistentArm {
		return objectLiteralTypeMismatch{}, false, true
	}
	return objectLiteralTypeMismatch{}, false, false
}

func objectLiteralMemberMismatchInLiteralConsistentArmWithSources(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.ObjectLiteralFact, arms []*typ.Record, env guardEnv, sourceForSegments objectLiteralEntrySourceResolverFunc) (objectLiteralTypeMismatch, bool, bool) {
	sawConsistentArm := false
	for _, arm := range arms {
		consistent, sawLiteral := objectLiteralArmConsistentWithLiteralEntries(result, fact, arm)
		if !sawLiteral || !consistent {
			continue
		}
		sawConsistentArm = true
		if mismatch, ok := objectLiteralMemberMismatchInFactWithSources(result, resolver, point, fact, arm, env, sourceForSegments); ok {
			return mismatch, true, true
		}
	}
	if sawConsistentArm {
		return objectLiteralTypeMismatch{}, false, true
	}
	return objectLiteralTypeMismatch{}, false, false
}

func objectLiteralArmConsistentWithLiteralEntries(result *body.Result, fact semantics.ObjectLiteralFact, arm *typ.Record) (consistent bool, sawLiteral bool) {
	for _, entry := range fact.Entries {
		got, ok := valueexpr.LiteralType(entry.Value)
		if !ok {
			continue
		}
		expected, ok := expectedTypeAtSegments(arm, entry.Suffix.Segments)
		if !ok {
			continue
		}
		sawLiteral = true
		if clearMismatch(result, got, expected) {
			return false, true
		}
	}
	return true, sawLiteral
}

func objectLiteralUnionArmEvidence(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.ObjectLiteralFact, arms []*typ.Record, env guardEnv) []diagnostic.Evidence {
	if len(arms) == 0 {
		return nil
	}
	out := make([]diagnostic.Evidence, 0, len(arms))
	for i, arm := range arms {
		message, span, ok := objectLiteralUnionArmRejection(result, resolver, point, fact, arm, env)
		if !ok {
			continue
		}
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustRefuted,
			Span:    span,
			Message: fmt.Sprintf("union arm %d (%s) rejected: %s", i+1, formatType(arm), message),
		})
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func objectLiteralUnionArmRejection(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.ObjectLiteralFact, arm *typ.Record, env guardEnv) (string, ast.Span, bool) {
	if field, ok := missingRequiredRecordField(arm, fact); ok {
		fieldPath := segment.FormatSegments([]segment.Segment{{Kind: segment.SegmentField, Name: field.Name}})
		if alias, aliasSpan, aliasOK := objectLiteralStaticStringIndexForField(fact, field.Name); aliasOK {
			return fmt.Sprintf("requires %s; literal provides %s instead", fieldPath, alias), aliasSpan, true
		}
		return fmt.Sprintf("missing required field %s", fieldPath), ast.SpanOf(fact.Expr), true
	}
	for _, entry := range fact.Entries {
		expected, ok := expectedTypeAtSegments(arm, entry.Suffix.Segments)
		if !ok {
			continue
		}
		got, ok := objectLiteralEntryMismatchType(result, resolver, point, entry, expected, env)
		if !ok {
			continue
		}
		return fmt.Sprintf("%s is %s, not %s", segment.FormatSegments(entry.Suffix.Segments), formatType(got), formatType(expected)), ast.SpanOf(entry.Value), true
	}
	return "", ast.SpanOf(fact.Expr), false
}

func objectLiteralStaticStringIndexForField(fact semantics.ObjectLiteralFact, name string) (string, ast.Span, bool) {
	for _, entry := range fact.Entries {
		if len(entry.Suffix.Segments) != 1 {
			continue
		}
		seg := entry.Suffix.Segments[0]
		if seg.Kind != segment.SegmentIndexString || seg.Name != name {
			continue
		}
		return segment.FormatSegments(entry.Suffix.Segments), ast.SpanOf(entry.Value), true
	}
	return "", ast.Span{}, false
}

func objectLiteralMemberMismatchInFact(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.ObjectLiteralFact, want typ.Type, env guardEnv) (objectLiteralTypeMismatch, bool) {
	return objectLiteralMemberMismatchInFactWithSources(result, resolver, point, fact, want, env, nil)
}

func objectLiteralMemberMismatchInFactWithSources(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.ObjectLiteralFact, want typ.Type, env guardEnv, sourceForSegments objectLiteralEntrySourceResolverFunc) (objectLiteralTypeMismatch, bool) {
	for _, entry := range fact.Entries {
		expected, ok := expectedTypeAtSegments(want, entry.Suffix.Segments)
		if !ok {
			continue
		}
		if _, ok := result.ObjectLiteral(entry.Value); ok {
			if mismatch, hasMismatch := objectLiteralMemberMismatch(result, resolver, point, entry.Value, expected, env); hasMismatch {
				return mismatch, true
			}
			continue
		}
		var source factflow.ValueSource
		var hasSource bool
		if sourceForSegments != nil {
			source, hasSource = sourceForSegments(entry.Suffix.Segments)
		}
		got, readBoundary, ok := objectLiteralEntryMismatchTypeWithSource(result, resolver, point, entry, expected, env, source, hasSource)
		if !ok {
			continue
		}
		return objectLiteralTypeMismatch{expr: entry.Value, got: got, want: expected, suffix: segment.FormatSegments(entry.Suffix.Segments), segments: entry.Suffix.Segments, readBoundary: readBoundary}, true
	}
	if field, ok := missingRequiredRecordField(want, fact); ok {
		return objectLiteralTypeMismatch{expr: fact.Expr, got: objectLiteralType(want, fact), want: want, missingField: field.Name}, true
	}
	if method, ok := missingRequiredInterfaceMethod(want, fact); ok {
		return objectLiteralTypeMismatch{expr: fact.Expr, got: objectLiteralType(want, fact), want: want, missingMethod: method}, true
	}
	return objectLiteralTypeMismatch{}, false
}

func missingRequiredInterfaceMethod(want typ.Type, fact semantics.ObjectLiteralFact) (typ.Method, bool) {
	iface, ok := transparentExpectedType(want).(*typ.Interface)
	if !ok || iface == nil || len(iface.Methods) == 0 {
		return typ.Method{}, false
	}
	present := make(map[string]struct{}, len(fact.Entries))
	for _, entry := range fact.Entries {
		if len(entry.Suffix.Segments) != 1 {
			continue
		}
		seg := entry.Suffix.Segments[0]
		if seg.Kind == segment.SegmentField && seg.Name != "" {
			present[seg.Name] = struct{}{}
		}
	}
	for _, method := range iface.Methods {
		if _, ok := present[method.Name]; ok {
			continue
		}
		return method, true
	}
	return typ.Method{}, false
}

func objectLiteralEntryMismatchType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, entry semantics.ObjectEntryFact, expected typ.Type, env guardEnv) (typ.Type, bool) {
	got, _, ok := objectLiteralEntryMismatchTypeWithSource(result, resolver, point, entry, expected, env, factflow.ValueSource{}, false)
	return got, ok
}

func objectLiteralEntryMismatchTypeWithSource(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, entry semantics.ObjectEntryFact, expected typ.Type, env guardEnv, source factflow.ValueSource, hasSource bool) (typ.Type, boundaryValueReader, bool) {
	if expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || refinement.ContainsFreeTypeParam(expected) {
		return nil, nil, false
	}
	if got, ok := valueexpr.LiteralType(entry.Value); ok && clearMismatch(result, got, expected) {
		return got, boundaryValueFromExpr(entry.Value), true
	}
	if got, ok := result.FunctionValueTypeAtBoundary(point, entry.Value); ok && clearMismatch(result, got, expected) {
		return got, boundaryValueFromExpr(entry.Value), true
	}
	if fn, ok := entry.Value.(*ast.FunctionExpr); ok {
		if got, ok := lowerFunctionExprType(fn, resolver); ok && clearMismatch(result, got, expected) {
			return got, boundaryValueFromExpr(entry.Value), true
		}
	}
	if nestedFact, ok := result.ObjectLiteral(entry.Value); ok {
		mismatch, hasMismatch := objectLiteralMemberMismatch(result, resolver, point, entry.Value, expected, env)
		if !hasMismatch {
			return nil, nil, false
		}
		if mismatch.got != nil {
			return mismatch.got, mismatch.readBoundary, true
		}
		return objectLiteralType(expected, nestedFact), boundaryValueFromExpr(entry.Value), true
	}
	if env.provesRuntimeType(result, point, entry.Value, expected) {
		return nil, nil, false
	}
	if hasSource {
		if got, readBoundary, ok := objectLiteralEntrySourceBoundaryType(result, point, source); ok {
			if clearMismatch(result, got, expected) {
				return got, readBoundary, true
			}
			return nil, nil, false
		}
	}
	if got, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(entry.Value); ok &&
		got != nil &&
		!topLikeType(got) &&
		!refinement.ContainsFreeTypeParam(got) {
		if clearMismatch(result, got, expected) {
			return got, boundaryValueFromExpr(entry.Value), true
		}
		return nil, nil, false
	}
	if got, ok := declaredPathType(result, resolver, entry.Value); ok && !topLikeType(got) && !refinement.ContainsFreeTypeParam(got) {
		readBoundary := boundaryValueFromASTSource(entry.Source)
		if boundaryProofTypeMismatch(result, point, got, expected, readBoundary) {
			return got, readBoundary, true
		}
	}
	if got, ok := untrustedTopLikeExpressionTypeAt(result, resolver, point, entry.Value); ok && clearMismatch(result, got, expected) {
		return got, boundaryValueFromExpr(entry.Value), true
	}
	query := newDiagnosticQuery(result)
	if hasSource {
		value, ok := query.ValueSourceForExplanationAtBoundary(point, source)
		if ok {
			readBoundary := boundaryValueFromValueSource(source)
			if query.ValueHasUntrustedTopOrigin(value) {
				if query.ValueProofAdmissible(value, expected) {
					return nil, nil, false
				}
				if got, ok := untrustedTopLikeExpressionTypeAt(result, resolver, point, entry.Value); ok {
					return got, readBoundary, true
				}
				if got, ok := query.ValueType(value); ok {
					return got, readBoundary, true
				}
				return typ.Any, readBoundary, true
			}
			if got, ok := query.ValueType(value); ok && clearMismatch(result, got, expected) {
				if topLike, ok := untrustedTopLikeExpressionType(result, resolver, entry.Value); ok {
					return topLike, readBoundary, true
				}
				return got, readBoundary, true
			}
		}
	}
	value, ok := query.SourceValue(point, entry.Source)
	if !ok {
		return nil, nil, false
	}
	if query.ValueHasUntrustedTopOrigin(value) {
		if query.ValueProofAdmissible(value, expected) {
			return nil, nil, false
		}
		if got, ok := declaredPathType(result, resolver, entry.Value); ok && !topLikeType(got) && !refinement.ContainsFreeTypeParam(got) {
			return got, boundaryValueFromASTSource(entry.Source), true
		}
		if got, ok := untrustedTopLikeExpressionTypeAt(result, resolver, point, entry.Value); ok {
			return got, boundaryValueFromASTSource(entry.Source), true
		}
		if got, ok := query.ValueType(value); ok {
			return got, boundaryValueFromASTSource(entry.Source), true
		}
		return typ.Any, boundaryValueFromASTSource(entry.Source), true
	}
	if got, ok := query.ValueType(value); ok && clearMismatch(result, got, expected) {
		if topLike, ok := untrustedTopLikeExpressionType(result, resolver, entry.Value); ok {
			return topLike, boundaryValueFromASTSource(entry.Source), true
		}
		return got, boundaryValueFromASTSource(entry.Source), true
	}
	return nil, nil, false
}

func objectLiteralEntrySourceBoundaryType(result *body.Result, point cfg.Point, source factflow.ValueSource) (typ.Type, boundaryValueReader, bool) {
	if result == nil {
		return nil, nil, false
	}
	query := newDiagnosticQuery(result)
	value, ok := query.SourceValueAtBoundary(point, source)
	if !ok {
		return nil, nil, false
	}
	if query.ValueHasUntrustedTopOrigin(value) {
		return nil, nil, false
	}
	got, ok := query.ValueTypeWithPresence(value)
	if !ok || got == nil || topLikeType(got) || refinement.ContainsFreeTypeParam(got) {
		return nil, nil, false
	}
	return got, boundaryValueFromValueSource(source), true
}

// objectLiteralType synthesizes the literal's structural type for a whole-literal
// union mismatch message. Entry values whose literal type is known contribute
// that type; the rest fall back to the union's own field type so the report names
// the literal's shape rather than an opaque value.
func objectLiteralType(want typ.Type, fact semantics.ObjectLiteralFact) typ.Type {
	builder := typetable.NewRecord()
	for _, entry := range fact.Entries {
		if len(entry.Suffix.Segments) != 1 {
			continue
		}
		seg := entry.Suffix.Segments[0]
		if (seg.Kind != segment.SegmentField && seg.Kind != segment.SegmentIndexString) || seg.Name == "" {
			continue
		}
		fieldType, ok := valueexpr.LiteralType(entry.Value)
		if !ok {
			if expected, ok := expectedTypeAtSegments(want, entry.Suffix.Segments); ok {
				fieldType = expected
			} else {
				fieldType = typ.Unknown
			}
		}
		switch seg.Kind {
		case segment.SegmentField:
			builder = builder.Field(seg.Name, fieldType)
		case segment.SegmentIndexString:
			builder = builder.StaticStringIndex(seg.Name, fieldType)
		}
	}
	return builder.Build()
}

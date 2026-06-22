package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
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
			return false
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
		for _, arm := range arms {
			if mismatch, ok := objectLiteralMemberMismatchInFact(result, resolver, point, fact, arm, env); ok {
				return mismatch, true
			}
		}
		return objectLiteralTypeMismatch{expr: expr, got: objectLiteralType(want, fact), want: want, unionArmEvidence: objectLiteralUnionArmEvidence(result, resolver, point, fact, arms, env)}, true
	}
	return objectLiteralMemberMismatchInFact(result, resolver, point, fact, want, env)
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
	for _, entry := range fact.Entries {
		expected, ok := expectedTypeAtSegments(want, entry.Suffix.Segments)
		if !ok {
			continue
		}
		got, ok := objectLiteralEntryMismatchType(result, resolver, point, entry, expected, env)
		if !ok {
			continue
		}
		return objectLiteralTypeMismatch{expr: entry.Value, got: got, want: expected, suffix: segment.FormatSegments(entry.Suffix.Segments), segments: entry.Suffix.Segments}, true
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
	if expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || refinement.ContainsFreeTypeParam(expected) {
		return nil, false
	}
	if got, ok := valueexpr.LiteralType(entry.Value); ok && clearMismatch(result, got, expected) {
		return got, true
	}
	if got, ok := result.FunctionValueTypeAtBoundary(point, entry.Value); ok && clearMismatch(result, got, expected) {
		return got, true
	}
	if fn, ok := entry.Value.(*ast.FunctionExpr); ok {
		if got, ok := lowerFunctionExprType(fn, resolver); ok && clearMismatch(result, got, expected) {
			return got, true
		}
	}
	if env.provesRuntimeType(result, point, entry.Value, expected) {
		return nil, false
	}
	reader := readmodel.New(result)
	value, ok := reader.SourceValue(point, entry.Source)
	if !ok {
		return nil, false
	}
	if got, ok := reader.ValueType(value); ok && clearMismatch(result, got, expected) {
		return got, true
	}
	if reader.ValueHasUntrustedTopOrigin(value) {
		if reader.ValueProofAdmissible(value, expected) {
			return nil, false
		}
		if got, ok := reader.ValueType(value); ok {
			return got, true
		}
		return typ.Any, true
	}
	return nil, false
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

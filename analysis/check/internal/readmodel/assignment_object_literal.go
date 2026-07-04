package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/sourcebridge"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func assignmentMissingRequired(point cfg.Point, r Reader, fact body.LocalAssignmentFact, expected typ.Type) (string, bool) {
	literal, ok := r.assignmentObjectLiteral(point, fact)
	if !ok {
		return "", false
	}
	return body.MissingRequiredRecordField(expected, func(name string) bool {
		for _, entry := range literal.Entries {
			if len(entry.Suffix.Segments) != 1 {
				continue
			}
			seg := entry.Suffix.Segments[0]
			if seg.Kind == segment.SegmentField && seg.Name == name {
				return true
			}
		}
		return false
	})
}

func (r Reader) assignmentObjectLiteral(point cfg.Point, fact body.LocalAssignmentFact) (body.ObjectLiteralFact, bool) {
	literal, ok := r.result.ObjectLiteral(fact.Source.Expr)
	if !ok {
		literal, ok = r.result.ObjectLiteral(fact.Expr)
	}
	return literal, ok
}

func (r Reader) assignmentObjectLiteralShapeType(point cfg.Point, fact body.LocalAssignmentFact) (typ.Type, bool) {
	literal, ok := r.assignmentObjectLiteral(point, fact)
	if !ok {
		return nil, false
	}
	return body.ObjectLiteralShapeType(literal, func(entry body.ObjectEntryFact) (typ.Type, bool) {
		value, valueOK := r.assignmentObjectLiteralEntryValue(point, entry)
		t, ok := r.assignmentObjectLiteralEntryType(point, entry, value, valueOK)
		if !ok || t == nil {
			return nil, false
		}
		return t, true
	})
}

func (r Reader) assignmentObjectLiteralEntry(point cfg.Point, fact body.LocalAssignmentFact, expected typ.Type) (Assignment, bool) {
	literal, ok := r.assignmentObjectLiteral(point, fact)
	if !ok {
		return Assignment{}, false
	}
	presentation := body.LocalAssignmentPresentationFor(fact)
	return r.assignmentObjectLiteralEntryCandidate(point, literal, expected, assignmentObjectEntryTarget{
		Label:          fact.Name,
		Key:            assignmentTargetKey(fact),
		ExpectedSpan:   sourceSpanFromBody(presentation.DeclarationSpan),
		ExpectedSource: readapi.AssignmentExpectedDeclared,
	})
}

type assignmentObjectEntryTarget struct {
	Label          string
	Key            string
	ExpectedSpan   SourceSpan
	ExpectedSource readapi.AssignmentExpectedSource
}

func (r Reader) assignmentObjectLiteralEntryCandidate(point cfg.Point, literal body.ObjectLiteralFact, expected typ.Type, target assignmentObjectEntryTarget) (Assignment, bool) {
	for _, entry := range literal.Entries {
		entryExpected, ok := body.ExpectedConstructorEntryType(expected, entry.Suffix.Segments)
		if !ok || !readapi.ObligationTypeReportable(entryExpected) {
			continue
		}
		value, valueOK := r.assignmentObjectLiteralEntryValue(point, entry)
		t, typeOK := r.assignmentObjectLiteralEntryType(point, entry, value, valueOK)
		if !typeOK {
			continue
		}
		untrustedTopOrigin := valueOK && r.ValueHasUntrustedTopOrigin(value)
		explicitTopOrigin := valueOK && r.ValueHasExplicitTopOrigin(value)
		valueAdmissible := valueOK && r.ValueProofAdmissible(value, entryExpected)
		valueProvenMismatch := valueOK && r.ValueWitnessProvenMismatch(value, entryExpected)
		if t == nil || valueAdmissible || (r.IsSubtype(t, entryExpected) && !untrustedTopOrigin) {
			continue
		}
		targetLabel := target.Label + segment.FormatSegments(entry.Suffix.Segments)
		sourceLabel := entry.ValueLabel
		if sourceLabel == "" {
			sourceLabel = body.AssignmentSourceLabel(entry.Value)
		}
		if sourceLabel == "" {
			sourceLabel = targetLabel
		}
		assignment := Assignment{
			Point:              point,
			TargetLabel:        targetLabel,
			SourceLabel:        sourceLabel,
			TargetKey:          target.Key + ":" + segment.FormatSegments(entry.Suffix.Segments),
			Value:              value,
			ValueHash:          assignmentValueHash(r, value, valueOK),
			TypeWithPresence:   t,
			Expected:           entryExpected,
			ExpectedSource:     target.ExpectedSource,
			SourceSpan:         sourceSpanFromBody(entry.ValueSpan),
			DeclarationSpan:    target.ExpectedSpan,
			UntrustedTopOrigin: untrustedTopOrigin,
			ExplicitTopOrigin:  explicitTopOrigin,
		}
		assignment.Check = readapi.PlanAssignmentCheck(readapi.AssignmentCheckPlan{
			Assignment:          assignment,
			ValueAdmissible:     valueAdmissible,
			ValueProvenMismatch: valueProvenMismatch,
		})
		return assignment, true
	}
	return Assignment{}, false
}

func (r Reader) assignmentObjectLiteralEntryType(point cfg.Point, entry body.ObjectEntryFact, value product.Value, valueOK bool) (typ.Type, bool) {
	if t, ok := body.LiteralExpressionType(entry.Value); ok {
		return t, true
	}
	if r.result != nil && entry.Value != nil {
		if t, ok := r.result.ExpressionTypeBeforeBoundary(point, entry.Value); ok && t != nil {
			return t, true
		}
	}
	if !valueOK {
		return nil, false
	}
	if t, ok := r.result.ObjectLiteralEntryType(value); ok {
		return t, true
	}
	if !r.result.ObjectLiteralEntryHasUntrustedTopOrigin(value) {
		return nil, false
	}
	if projected, ok := r.ValueTypeWithPresence(value); ok && projected != nil {
		return projected, true
	}
	return typ.Unknown, true
}

func (r Reader) assignmentObjectLiteralEntryValue(point cfg.Point, entry body.ObjectEntryFact) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	if entry.Value != nil {
		if value, ok := r.result.ExpressionValueBeforeBoundary(point, entry.Value); ok {
			return r.result.WithMemberReadNilWitness(point, entry.Value, value), true
		}
	}
	value, ok := r.SourceValue(point, entry.Source)
	if loweredSource, loweredOK := sourcebridge.ValueSourceFromASTSource(entry.Source); loweredOK {
		if before, beforeOK := r.result.SourceValueBeforeBoundary(point, loweredSource); beforeOK {
			value, ok = before, true
		}
	}
	if !ok {
		return product.Value{}, false
	}
	return value, true
}

func assignmentValueHash(r Reader, value product.Value, ok bool) uint64 {
	if !ok {
		return 0
	}
	return r.ValueHash(value)
}

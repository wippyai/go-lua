package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func assignmentMissingRequired(point cfg.Point, r Reader, fact body.LocalAssignmentFact, expected typ.Type) (string, bool) {
	literal, ok := r.assignmentObjectLiteralView(point)
	if !ok {
		return "", false
	}
	return assignmentObjectLiteralMissingRequired(literal, expected)
}

func (r Reader) assignmentObjectLiteralShapeType(point cfg.Point, fact body.LocalAssignmentFact) (typ.Type, bool) {
	literal, ok := r.assignmentObjectLiteralView(point)
	if !ok {
		return nil, false
	}
	return r.assignmentObjectLiteralShapeTypeForView(point, literal)
}

func (r Reader) assignmentObjectLiteralEntry(point cfg.Point, fact body.LocalAssignmentFact, expected typ.Type) (Assignment, bool) {
	literal, ok := r.assignmentObjectLiteralView(point)
	if !ok {
		return Assignment{}, false
	}
	presentation := body.LocalAssignmentPresentationFor(fact)
	parentType, _ := r.assignmentObjectLiteralShapeTypeForView(point, literal)
	return r.assignmentObjectLiteralEntryCandidate(point, literal, expected, assignmentObjectEntryTarget{
		Label:          fact.Name,
		Key:            body.AssignmentTargetKey(fact),
		ExpectedSpan:   sourceSpanFromBody(presentation.DeclarationSpan),
		ExpectedSource: readapi.AssignmentExpectedDeclared,
		ParentContext: readapi.AssignmentParentContext{
			SourceLabel:     "assigned value",
			TargetLabel:     fact.Name,
			SourceType:      parentType,
			Expected:        expected,
			SourceSpan:      sourceSpanFromBody(presentation.SourceSpan),
			DeclarationSpan: sourceSpanFromBody(presentation.DeclarationSpan),
		},
	})
}

func (r Reader) assignmentObjectLiteralView(point cfg.Point) (factflow.ObjectLiteralView, bool) {
	if r.result == nil {
		return factflow.ObjectLiteralView{}, false
	}
	fact, ok := r.result.LoweredLocalAssignment(point)
	if !ok {
		return factflow.ObjectLiteralView{}, false
	}
	return r.result.ObjectLiteralViewForSource(fact.Source())
}

type assignmentObjectEntryTarget struct {
	Label          string
	Key            string
	ExpectedSpan   SourceSpan
	ExpectedSource readapi.AssignmentExpectedSource
	ParentContext  readapi.AssignmentParentContext
}

func (r Reader) assignmentObjectLiteralEntryCandidate(point cfg.Point, literal factflow.ObjectLiteralView, expected typ.Type, target assignmentObjectEntryTarget) (Assignment, bool) {
	var out Assignment
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		entryExpected, ok := body.ExpectedConstructorEntryType(expected, entry.SuffixSegmentsView())
		if !ok || !readapi.ObligationTypeReportable(entryExpected) {
			return true
		}
		value, valueOK := r.assignmentObjectEntryValue(point, entry)
		t, typeOK := r.assignmentObjectEntryType(value, valueOK)
		if !typeOK {
			return true
		}
		untrustedTopOrigin := valueOK && r.ValueHasUntrustedTopOrigin(value)
		explicitTopOrigin := valueOK && r.ValueHasExplicitTopOrigin(value)
		valueAdmissible := valueOK && r.ValueProofAdmissible(value, entryExpected)
		provenMismatch := valueOK && r.ValueWitnessProvenMismatch(value, entryExpected)
		if t == nil || valueAdmissible || (r.IsSubtype(t, entryExpected) && !untrustedTopOrigin) {
			return true
		}
		if !provenMismatch && !untrustedTopOrigin {
			provenMismatch = assignmentObjectLiteralProvenMismatch(r, t, entryExpected)
		}
		targetLabel := target.Label + segment.FormatSegments(entry.SuffixSegmentsView())
		sourceLabel := entry.ValueLabel()
		if sourceLabel == "" {
			sourceLabel = targetLabel
		}
		out = Assignment{
			Point:              point,
			TargetLabel:        targetLabel,
			SourceLabel:        sourceLabel,
			TargetKey:          target.Key + ":" + segment.FormatSegments(entry.SuffixSegmentsView()),
			Value:              value,
			ValueHash:          assignmentValueHash(r, value, valueOK),
			TypeWithPresence:   t,
			Expected:           entryExpected,
			ExpectedSource:     target.ExpectedSource,
			SourceSpan:         sourceSpanFromFactflow(entry.ValueSpan()),
			DeclarationSpan:    target.ExpectedSpan,
			ParentContext:      target.ParentContext,
			UntrustedTopOrigin: untrustedTopOrigin,
			ExplicitTopOrigin:  explicitTopOrigin,
			RuntimeValidated:   valueOK && r.ValueHasRuntimeValidationProof(value),
		}
		out.Check = readapi.PlanAssignmentCheck(readapi.AssignmentCheckPlan{
			Assignment:          out,
			ValueAdmissible:     valueAdmissible,
			ValueProvenMismatch: provenMismatch,
		})
		return false
	})
	if out.Expected == nil {
		return Assignment{}, false
	}
	return out, true
}

func (r Reader) assignmentObjectLiteralShapeTypeForView(point cfg.Point, literal factflow.ObjectLiteralView) (typ.Type, bool) {
	builder := typetable.NewConstructorBuilder()
	seen := false
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		constructorPath, ok := body.ConstructorPathFromSegments(entry.SuffixSegmentsView())
		if !ok {
			return true
		}
		value, valueOK := r.assignmentObjectEntryValue(point, entry)
		t, ok := r.assignmentObjectEntryType(value, valueOK)
		if !ok || t == nil {
			return true
		}
		if !builder.Add(constructorPath, t) {
			seen = false
			return false
		}
		seen = true
		return true
	})
	if !seen {
		return nil, false
	}
	return builder.Build()
}

func assignmentObjectLiteralMissingRequired(literal factflow.ObjectLiteralView, expected typ.Type) (string, bool) {
	return body.MissingRequiredRecordField(expected, func(name string) bool {
		has := false
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			if field, ok := segment.DirectFieldName(entry.SuffixSegmentsView()); ok && field == name {
				has = true
				return false
			}
			return true
		})
		return has
	})
}

func (r Reader) assignmentObjectEntryValue(point cfg.Point, entry factflow.ObjectEntryView) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	source := entry.Source()
	if value, ok := r.result.SourceValueBeforeBoundary(point, source); ok {
		return value, true
	}
	return r.result.SourceValueForExplanationAtBoundary(point, source)
}

func (r Reader) assignmentObjectEntryType(value product.Value, valueOK bool) (typ.Type, bool) {
	if !valueOK {
		return nil, false
	}
	if r.result != nil {
		if t, ok := r.result.ObjectLiteralEntryType(value); ok {
			return t, true
		}
	}
	if !r.ValueHasUntrustedTopOrigin(value) {
		return r.ValueTypeWithPresence(value)
	}
	if projected, ok := r.ValueTypeWithPresence(value); ok && projected != nil {
		return projected, true
	}
	return typ.Unknown, true
}

func assignmentObjectLiteralProvenMismatch(r Reader, actual, expected typ.Type) bool {
	if actual == nil || expected == nil || typ.IsAny(actual) || typ.IsUnknown(actual) || typ.IsNever(actual) {
		return false
	}
	return !r.IsSubtype(actual, expected)
}

func assignmentValueHash(r Reader, value product.Value, ok bool) uint64 {
	if !ok {
		return 0
	}
	return r.ValueHash(value)
}

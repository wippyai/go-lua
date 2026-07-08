package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func assignmentMissingRequired(point cfg.Point, r Reader, fact body.LocalAssignmentFact, expected typ.Type) (string, bool) {
	literal, ok := r.assignmentObjectLiteralView(point)
	if !ok {
		return "", false
	}
	return body.ObjectLiteralMissingRequired(literal, expected)
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
	if r.result == nil {
		return Assignment{}, false
	}
	var out Assignment
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		entryExpected, ok := luatypeprojection.ExpectedConstructorEntryType(expected, entry.SuffixSegmentsView())
		if !ok || !readapi.ObligationTypeReportable(entryExpected) {
			return true
		}
		proof := r.result.ObjectLiteralEntryProofAt(point, entry, entryExpected)
		if !proof.HasType {
			return true
		}
		if proof.Type == nil || proof.Admissible || (proof.Assignable && !proof.UntrustedTop) {
			return true
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
			Value:              proof.Value,
			ValueHash:          assignmentValueHash(r, proof.Value, proof.HasValue),
			TypeWithPresence:   proof.Type,
			Expected:           entryExpected,
			ExpectedSource:     target.ExpectedSource,
			SourceSpan:         sourceSpanFromFactflow(entry.ValueSpan()),
			DeclarationSpan:    target.ExpectedSpan,
			ParentContext:      target.ParentContext,
			UntrustedTopOrigin: proof.UntrustedTop,
			ExplicitTopOrigin:  proof.ExplicitTop,
			RuntimeValidated:   proof.RuntimeValidated,
		}
		out.Check = readapi.PlanAssignmentCheck(readapi.AssignmentCheckPlan{
			Assignment:          out,
			ValueAdmissible:     proof.Admissible,
			ValueProvenMismatch: proof.ProvenMismatch,
		})
		return false
	})
	if out.Expected == nil {
		return Assignment{}, false
	}
	return out, true
}

func (r Reader) assignmentObjectLiteralShapeTypeForView(point cfg.Point, literal factflow.ObjectLiteralView) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return r.result.ObjectLiteralShapeTypeAt(point, literal)
}

func assignmentValueHash(r Reader, value product.Value, ok bool) uint64 {
	if !ok {
		return 0
	}
	return r.ValueHash(value)
}

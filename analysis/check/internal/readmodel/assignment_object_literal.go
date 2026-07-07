package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func assignmentMissingRequired(point cfg.Point, r Reader, fact body.LocalAssignmentFact, expected typ.Type) (string, bool) {
	return r.result.AssignmentObjectLiteralMissingRequired(point, fact, expected)
}

func (r Reader) assignmentObjectLiteralShapeType(point cfg.Point, fact body.LocalAssignmentFact) (typ.Type, bool) {
	return r.result.AssignmentObjectLiteralShapeType(point, fact)
}

func (r Reader) assignmentObjectLiteralEntry(point cfg.Point, fact body.LocalAssignmentFact, expected typ.Type) (Assignment, bool) {
	proof, ok := r.result.LocalAssignmentObjectLiteralEntryProof(point, fact, expected)
	if !ok {
		return Assignment{}, false
	}
	return r.assignmentFromObjectLiteralProof(proof, readapi.AssignmentExpectedDeclared), true
}

type assignmentObjectEntryTarget struct {
	Label          string
	Key            string
	ExpectedSpan   SourceSpan
	ExpectedSource readapi.AssignmentExpectedSource
	ParentContext  readapi.AssignmentParentContext
}

func (r Reader) assignmentObjectLiteralEntryCandidate(point cfg.Point, literal body.ObjectLiteralFact, expected typ.Type, target assignmentObjectEntryTarget) (Assignment, bool) {
	proof, ok := r.result.ObjectLiteralAssignmentEntryProof(point, literal, expected, body.ObjectLiteralAssignmentTarget{
		Label:        target.Label,
		Key:          target.Key,
		ExpectedSpan: sourceSpanToBody(target.ExpectedSpan),
		ParentContext: body.ObjectLiteralAssignmentParentContext{
			SourceLabel:     target.ParentContext.SourceLabel,
			TargetLabel:     target.ParentContext.TargetLabel,
			SourceType:      target.ParentContext.SourceType,
			Expected:        target.ParentContext.Expected,
			SourceSpan:      sourceSpanToBody(target.ParentContext.SourceSpan),
			DeclarationSpan: sourceSpanToBody(target.ParentContext.DeclarationSpan),
		},
	})
	if !ok {
		return Assignment{}, false
	}
	return r.assignmentFromObjectLiteralProof(proof, target.ExpectedSource), true
}

func (r Reader) assignmentFromObjectLiteralProof(proof body.ObjectLiteralAssignmentEntryProof, expectedSource readapi.AssignmentExpectedSource) Assignment {
	assignment := Assignment{
		Point:              proof.Point,
		TargetLabel:        proof.TargetLabel,
		SourceLabel:        proof.SourceLabel,
		TargetKey:          proof.TargetKey,
		Value:              proof.Value,
		ValueHash:          assignmentValueHash(r, proof.Value, proof.ValueOK),
		TypeWithPresence:   proof.TypeWithPresence,
		Expected:           proof.Expected,
		ExpectedSource:     expectedSource,
		SourceSpan:         sourceSpanFromBody(proof.SourceSpan),
		DeclarationSpan:    sourceSpanFromBody(proof.DeclarationSpan),
		ParentContext:      assignmentParentContextFromBody(proof.ParentContext),
		UntrustedTopOrigin: proof.UntrustedTopOrigin,
		ExplicitTopOrigin:  proof.ExplicitTopOrigin,
		RuntimeValidated:   proof.RuntimeValidated,
	}
	assignment.Check = readapi.PlanAssignmentCheck(readapi.AssignmentCheckPlan{
		Assignment:          assignment,
		ValueAdmissible:     proof.ValueAdmissible,
		ValueProvenMismatch: proof.ProvenMismatch,
	})
	return assignment
}

func assignmentParentContextFromBody(ctx body.ObjectLiteralAssignmentParentContext) readapi.AssignmentParentContext {
	return readapi.AssignmentParentContext{
		SourceLabel:     ctx.SourceLabel,
		TargetLabel:     ctx.TargetLabel,
		SourceType:      ctx.SourceType,
		Expected:        ctx.Expected,
		SourceSpan:      sourceSpanFromBody(ctx.SourceSpan),
		DeclarationSpan: sourceSpanFromBody(ctx.DeclarationSpan),
	}
}

func sourceSpanToBody(span SourceSpan) body.SourceSpan {
	return body.SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func assignmentValueHash(r Reader, value product.Value, ok bool) uint64 {
	if !ok {
		return 0
	}
	return r.ValueHash(value)
}

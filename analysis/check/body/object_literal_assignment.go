package body

import (
	"github.com/wippyai/go-lua/analysis/check/internal/sourcebridge"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type ObjectLiteralAssignmentParentContext struct {
	SourceLabel     string
	TargetLabel     string
	SourceType      typ.Type
	Expected        typ.Type
	SourceSpan      SourceSpan
	DeclarationSpan SourceSpan
}

type ObjectLiteralAssignmentTarget struct {
	Label         string
	Key           string
	ExpectedSpan  SourceSpan
	ParentContext ObjectLiteralAssignmentParentContext
}

type ObjectLiteralAssignmentEntryProof struct {
	Point              cfg.Point
	TargetLabel        string
	SourceLabel        string
	TargetKey          string
	Value              product.Value
	ValueOK            bool
	TypeWithPresence   typ.Type
	Expected           typ.Type
	SourceSpan         SourceSpan
	DeclarationSpan    SourceSpan
	ParentContext      ObjectLiteralAssignmentParentContext
	UntrustedTopOrigin bool
	ExplicitTopOrigin  bool
	RuntimeValidated   bool
	ValueAdmissible    bool
	ProvenMismatch     bool
}

func (r *Result) AssignmentObjectLiteral(point cfg.Point, fact LocalAssignmentFact) (ObjectLiteralFact, bool) {
	if r == nil {
		return ObjectLiteralFact{}, false
	}
	literal, ok := r.ObjectLiteral(fact.Source.Expr)
	if !ok {
		literal, ok = r.ObjectLiteral(fact.Expr)
	}
	return literal, ok
}

func (r *Result) AssignmentObjectLiteralShapeType(point cfg.Point, fact LocalAssignmentFact) (typ.Type, bool) {
	literal, ok := r.AssignmentObjectLiteral(point, fact)
	if !ok {
		return nil, false
	}
	return ObjectLiteralShapeType(literal, func(entry ObjectEntryFact) (typ.Type, bool) {
		value, valueOK := r.assignmentObjectLiteralEntryValue(point, entry)
		t, ok := r.assignmentObjectLiteralEntryType(point, entry, value, valueOK)
		if !ok || t == nil {
			return nil, false
		}
		return t, true
	})
}

func (r *Result) AssignmentObjectLiteralMissingRequired(point cfg.Point, fact LocalAssignmentFact, expected typ.Type) (string, bool) {
	literal, ok := r.AssignmentObjectLiteral(point, fact)
	if !ok {
		return "", false
	}
	return MissingRequiredRecordField(expected, func(name string) bool {
		for _, entry := range literal.Entries {
			if field, ok := segment.DirectFieldName(entry.Suffix.Segments); ok && field == name {
				return true
			}
		}
		return false
	})
}

func (r *Result) LocalAssignmentObjectLiteralEntryProof(point cfg.Point, fact LocalAssignmentFact, expected typ.Type) (ObjectLiteralAssignmentEntryProof, bool) {
	literal, ok := r.AssignmentObjectLiteral(point, fact)
	if !ok {
		return ObjectLiteralAssignmentEntryProof{}, false
	}
	presentation := LocalAssignmentPresentationFor(fact)
	parentType, _ := r.AssignmentObjectLiteralShapeType(point, fact)
	return r.ObjectLiteralAssignmentEntryProof(point, literal, expected, ObjectLiteralAssignmentTarget{
		Label:        fact.Name,
		Key:          AssignmentTargetKey(fact),
		ExpectedSpan: presentation.DeclarationSpan,
		ParentContext: ObjectLiteralAssignmentParentContext{
			SourceLabel:     "assigned value",
			TargetLabel:     fact.Name,
			SourceType:      parentType,
			Expected:        expected,
			SourceSpan:      presentation.SourceSpan,
			DeclarationSpan: presentation.DeclarationSpan,
		},
	})
}

func (r *Result) ObjectLiteralAssignmentEntryProof(point cfg.Point, literal ObjectLiteralFact, expected typ.Type, target ObjectLiteralAssignmentTarget) (ObjectLiteralAssignmentEntryProof, bool) {
	if r == nil {
		return ObjectLiteralAssignmentEntryProof{}, false
	}
	for _, entry := range literal.Entries {
		entryExpected, ok := ExpectedConstructorEntryType(expected, entry.Suffix.Segments)
		if !ok || !objectLiteralAssignmentTypeReportable(entryExpected) {
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
		provenMismatch := valueOK && r.ValueWitnessProvenMismatch(value, entryExpected)
		if t == nil || valueAdmissible || (r.IsSubtype(t, entryExpected) && !untrustedTopOrigin) {
			continue
		}
		if !provenMismatch && !untrustedTopOrigin {
			provenMismatch = objectLiteralAssignmentProvenMismatch(r, t, entryExpected)
		}
		targetLabel := target.Label + segment.FormatSegments(entry.Suffix.Segments)
		sourceLabel := entry.ValueLabel
		if sourceLabel == "" {
			sourceLabel = AssignmentSourceLabel(entry.Value)
		}
		if sourceLabel == "" {
			sourceLabel = targetLabel
		}
		return ObjectLiteralAssignmentEntryProof{
			Point:              point,
			TargetLabel:        targetLabel,
			SourceLabel:        sourceLabel,
			TargetKey:          target.Key + ":" + segment.FormatSegments(entry.Suffix.Segments),
			Value:              value,
			ValueOK:            valueOK,
			TypeWithPresence:   t,
			Expected:           entryExpected,
			SourceSpan:         entry.ValueSpan,
			DeclarationSpan:    target.ExpectedSpan,
			ParentContext:      target.ParentContext,
			UntrustedTopOrigin: untrustedTopOrigin,
			ExplicitTopOrigin:  explicitTopOrigin,
			RuntimeValidated:   valueOK && r.ValueHasRuntimeValidationProof(value),
			ValueAdmissible:    valueAdmissible,
			ProvenMismatch:     provenMismatch,
		}, true
	}
	return ObjectLiteralAssignmentEntryProof{}, false
}

func objectLiteralAssignmentTypeReportable(t typ.Type) bool {
	return t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}

func objectLiteralAssignmentProvenMismatch(r *Result, actual, expected typ.Type) bool {
	if r == nil || actual == nil || expected == nil || typ.IsAny(actual) || typ.IsUnknown(actual) || typ.IsNever(actual) {
		return false
	}
	return !r.IsSubtype(actual, expected)
}

func (r *Result) assignmentObjectLiteralEntryType(point cfg.Point, entry ObjectEntryFact, value product.Value, valueOK bool) (typ.Type, bool) {
	if t, ok := LiteralExpressionType(entry.Value); ok {
		return t, true
	}
	if r != nil && entry.Value != nil {
		if t, ok := r.ExpressionTypeBeforeBoundary(point, entry.Value); ok && t != nil {
			return t, true
		}
	}
	if !valueOK {
		return nil, false
	}
	if t, ok := r.ObjectLiteralEntryType(value); ok {
		return t, true
	}
	if !r.ObjectLiteralEntryHasUntrustedTopOrigin(value) {
		return nil, false
	}
	if projected, ok := r.ValueTypeWithPresence(value); ok && projected != nil {
		return projected, true
	}
	return typ.Unknown, true
}

func (r *Result) assignmentObjectLiteralEntryValue(point cfg.Point, entry ObjectEntryFact) (product.Value, bool) {
	if r == nil {
		return product.Value{}, false
	}
	value, ok := r.objectEntrySourceValue(point, entry.Source)
	if loweredSource, loweredOK := sourcebridge.ValueSourceFromASTSource(entry.Source); loweredOK {
		if before, beforeOK := r.SourceValueBeforeBoundary(point, loweredSource); beforeOK {
			if !ok || r.ValueHasUntrustedTopOrigin(before) || !r.ValueHasUntrustedTopOrigin(value) {
				value, ok = before, true
			}
		}
	}
	if ok {
		return r.objectLiteralEntryDeclaredTopValue(point, entry, value), true
	}
	if entry.Value != nil {
		if value, ok := r.ExpressionValueBeforeBoundary(point, entry.Value); ok {
			value = r.WithMemberReadNilWitness(point, entry.Value, value)
			return r.objectLiteralEntryDeclaredTopValue(point, entry, value), true
		}
	}
	if !ok {
		return product.Value{}, false
	}
	return r.objectLiteralEntryDeclaredTopValue(point, entry, value), true
}

func (r *Result) objectEntrySourceValue(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	switch source.Kind {
	case sourceprovenance.SourceExpression:
		if source.Expr == nil {
			return product.Value{}, false
		}
		if value, ok := r.ExpressionValueAtBoundary(point, source.Expr); ok {
			if !r.valueHasReadableType(value) {
				if p, ok := r.ExpressionPath(source.Expr); ok && !p.IsEmpty() {
					if pathValue, pathOK := r.PathValueAtBoundary(point, p); pathOK {
						return pathValue, true
					}
				}
			}
			return value, true
		}
		if value, ok := r.LocalAssignmentSourceValueForExplanationAtBoundary(point, source); ok {
			return value, true
		}
		if p, ok := r.ExpressionPath(source.Expr); ok && !p.IsEmpty() {
			return r.PathValueAtBoundary(point, p)
		}
		return product.Value{}, false
	case sourceprovenance.SourceCall:
		if source.Expr != nil && source.ResultIndex == 0 {
			if value, ok := r.ExpressionValueAtBoundary(point, source.Expr); ok {
				return value, true
			}
		}
		if value, ok := r.LocalAssignmentSourceValueForExplanationAtBoundary(point, source); ok {
			return value, true
		}
		valueSource, ok := sourcebridge.ValueSourceFromASTSource(source)
		if !ok {
			return product.Value{}, false
		}
		return r.SourceValueForExplanationAtBoundary(point, valueSource)
	case sourceprovenance.SourceVararg, sourceprovenance.SourceNil, sourceprovenance.SourceUnknown:
		if value, ok := r.LocalAssignmentSourceValueForExplanationAtBoundary(point, source); ok {
			return value, true
		}
		valueSource, ok := sourcebridge.ValueSourceFromASTSource(source)
		if !ok {
			return product.Value{}, false
		}
		return r.SourceValueForExplanationAtBoundary(point, valueSource)
	default:
		return product.Value{}, false
	}
}

func (r *Result) objectLiteralEntryDeclaredTopValue(point cfg.Point, entry ObjectEntryFact, value product.Value) product.Value {
	if r == nil || entry.Value == nil || r.ValueHasUntrustedTopOrigin(value) {
		return value
	}
	declared, ok := r.DeclaredExpressionTypeAt(point, entry.Value)
	if !ok || declared == nil || (!typ.IsAny(declared) && !typ.IsUnknown(declared)) {
		return value
	}
	top, ok := r.valueFromType(declared)
	if !ok {
		top = value
	}
	return product.Set(r.Registry(), top, evidence.Key, evidence.ExplicitTop())
}

func (r *Result) valueFromType(t typ.Type) (product.Value, bool) {
	if r == nil || r.typeValues == nil || t == nil || r.Registry() == nil {
		return product.Value{}, false
	}
	return typevalue.WithWitness(r.Registry(), r.typeValues.FromType(r.Registry(), t), t), true
}

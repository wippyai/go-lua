package readmodel

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ForEachReturn visits returned expressions with explicit declared return
// contracts in deterministic RPO order.
func (r Reader) ForEachReturn(visit func(Return) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	expectedValues := r.result.ReturnTypeValues()
	if len(expectedValues) == 0 {
		return false
	}
	expectedSpans := sourceSpansFromBody(r.result.FunctionReturnTypeSpansForFunction())
	visited := false
	return r.result.ForEachReturnValueOccurrence(func(occ body.ReturnValueOccurrence) bool {
		if occ.Index >= len(expectedValues) {
			return true
		}
		ret, ok := r.returnObjectLiteralEntry(occ.Point, occ.Index, occ.Source, expectedValues[occ.Index], expectedSpans)
		if !ok {
			ret, ok = r.returnValue(occ, expectedValues[occ.Index], expectedSpans)
		}
		if !ok {
			return true
		}
		visited = true
		if !visit(ret) {
			return true
		}
		return true
	}) || visited
}

func (r Reader) returnObjectLiteralEntry(point cfg.Point, index int, source sourceprovenance.ASTSource, expectedValue product.Value, expectedSpans []SourceSpan) (Return, bool) {
	if source.Kind != sourceprovenance.SourceExpression || source.Expr == nil {
		return Return{}, false
	}
	expected, ok := r.ValueType(expectedValue)
	if !ok || expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || typ.IsNever(expected) || refinement.ContainsFreeTypeParam(expected) {
		return Return{}, false
	}
	literal, ok := r.result.ObjectLiteral(source.Expr)
	if !ok {
		return Return{}, false
	}
	for _, entry := range literal.Entries {
		entryExpected, ok := body.ExpectedTypeAtSegments(expected, entry.Suffix.Segments)
		if !ok || !readapi.ObligationTypeReportable(entryExpected) {
			continue
		}
		value, ok := r.SourceValue(point, entry.Source)
		if !ok {
			continue
		}
		actual, _ := r.ValueTypeWithPresence(value)
		untrustedTopOrigin := r.ValueHasUntrustedTopOrigin(value)
		if actual == nil || (r.IsSubtype(actual, entryExpected) && !untrustedTopOrigin) {
			continue
		}
		label := returnExpectedLabel(index) + segment.FormatSegments(entry.Suffix.Segments)
		sourceLabel := entry.ValueLabel
		if sourceLabel == "" {
			sourceLabel = label
		}
		ret := Return{
			Point:              point,
			Index:              index,
			Value:              value,
			ValueHash:          r.ValueHash(value),
			TypeWithPresence:   actual,
			Expected:           entryExpected,
			ExpectedLabel:      label,
			SourceLabel:        sourceLabel,
			SourceSpan:         sourceSpanFromSemantic(entry.ValueSpan),
			DeclarationSpan:    readmodelSourceSpanAt(expectedSpans, index),
			UntrustedTopOrigin: untrustedTopOrigin,
			ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
		}
		ret.Check = readapi.PlanReturnCheck(readapi.ReturnCheckPlan{
			Return:              ret,
			ValueAdmissible:     r.ValueProofAdmissible(value, entryExpected),
			ValueProvenMismatch: r.ValueWitnessProvenMismatch(value, entryExpected),
			IsSubtype:           r.IsSubtype,
		})
		return ret, true
	}
	return Return{}, false
}

func (r Reader) returnValue(occ body.ReturnValueOccurrence, expectedValue product.Value, expectedSpans []SourceSpan) (Return, bool) {
	expected, ok := r.ValueType(expectedValue)
	if !ok || expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || typ.IsNever(expected) || refinement.ContainsFreeTypeParam(expected) {
		return Return{}, false
	}
	value, ok := r.SourceValue(occ.Point, occ.Source)
	if !ok {
		return Return{}, false
	}
	actual, _ := r.ValueTypeWithPresence(value)
	ret := Return{
		Point:              occ.Point,
		Index:              occ.Index,
		Value:              value,
		ValueHash:          r.ValueHash(value),
		TypeWithPresence:   actual,
		Expected:           expected,
		ExpectedLabel:      returnExpectedLabel(occ.Index),
		SourceLabel:        occ.SourceLabel,
		SourceSpan:         sourceSpanFromBody(occ.SourceSpan),
		DeclarationSpan:    readmodelSourceSpanAt(expectedSpans, occ.Index),
		UntrustedTopOrigin: r.ValueHasUntrustedTopOrigin(value),
		ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
	}
	ret.Check = readapi.PlanReturnCheck(readapi.ReturnCheckPlan{
		Return:              ret,
		ValueAdmissible:     r.ValueProofAdmissible(value, expected),
		ValueProvenMismatch: r.ValueWitnessProvenMismatch(value, expected),
		IsSubtype:           r.IsSubtype,
	})
	return ret, true
}

func returnExpectedLabel(index int) string {
	return "returned value " + strconv.Itoa(index+1)
}

func readmodelSourceSpanAt(spans []SourceSpan, index int) SourceSpan {
	if index < 0 || index >= len(spans) {
		return SourceSpan{}
	}
	return spans[index]
}

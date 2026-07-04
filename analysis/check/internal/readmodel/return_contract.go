package readmodel

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
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
		ret, ok := r.returnObjectLiteralEntry(occ, expectedValues[occ.Index], expectedSpans)
		if !ok {
			ret, ok = r.returnDeclaredObjectLiteralEntry(occ, expectedValues[occ.Index], expectedSpans)
		}
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

func (r Reader) returnObjectLiteralEntry(occ body.ReturnValueOccurrence, expectedValue product.Value, expectedSpans []SourceSpan) (Return, bool) {
	source := occ.Source
	if source.Kind != sourceprovenance.SourceExpression || source.Expr == nil {
		return Return{}, false
	}
	expected, ok := r.ValueTypeWithPresence(expectedValue)
	if !ok || expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || typ.IsNever(expected) || refinement.ContainsFreeTypeParam(expected) {
		return Return{}, false
	}
	literal, ok := r.result.ObjectLiteral(source.Expr)
	if !ok {
		return Return{}, false
	}
	rootValue, rootValueOK := r.SourceValue(occ.Point, source)
	for _, entry := range literal.Entries {
		entryExpected, ok := body.ExpectedTypeAtSegments(expected, entry.Suffix.Segments)
		if !ok || !readapi.ObligationTypeReportable(entryExpected) {
			continue
		}
		value, ok := r.SourceValue(occ.Point, entry.Source)
		if !ok {
			continue
		}
		actual, _ := r.ValueTypeWithPresence(value)
		untrustedTopOrigin := r.ValueHasUntrustedTopOrigin(value)
		if actual == nil || (r.IsSubtype(actual, entryExpected) && !untrustedTopOrigin) {
			continue
		}
		label := returnExpectedLabel(occ.Index) + segment.FormatSegments(entry.Suffix.Segments)
		sourceLabel := entry.ValueLabel
		if sourceLabel == "" {
			sourceLabel = label
		}
		ret := Return{
			Point:              occ.Point,
			Index:              occ.Index,
			Value:              value,
			ValueHash:          r.ValueHash(value),
			TypeWithPresence:   actual,
			Expected:           entryExpected,
			ExpectedLabel:      label,
			SourceLabel:        sourceLabel,
			SourceSpan:         sourceSpanFromBody(entry.ValueSpan),
			DeclarationSpan:    readmodelSourceSpanAt(expectedSpans, occ.Index),
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
	if rootValueOK {
		return r.returnSemanticObjectLiteralMissingRequired(occ.Point, occ.Index, literal, rootValue, expected, sourceSpanFromBody(occ.SourceSpan), expectedSpans)
	}
	return Return{}, false
}

func (r Reader) returnDeclaredObjectLiteralEntry(occ body.ReturnValueOccurrence, expectedValue product.Value, expectedSpans []SourceSpan) (Return, bool) {
	source := occ.Source
	if r.result == nil || source.Kind != sourceprovenance.SourceExpression || source.Expr == nil {
		return Return{}, false
	}
	if !occ.HasPath || occ.SourcePath.IsEmpty() || occ.SourcePath.Symbol == 0 {
		return Return{}, false
	}
	declaration, ok := r.result.DominatingPathRootDeclarationSource(occ.Point, occ.SourcePath)
	if !ok {
		return Return{}, false
	}
	literal, ok := r.result.ObjectLiteralViewForSource(declaration.Source)
	if !ok {
		return Return{}, false
	}
	rootValue, rootValueOK := r.SourceValue(occ.Point, source)
	return r.returnLoweredObjectLiteralEntry(occ.Point, occ.Index, literal, rootValue, rootValueOK, sourceSpanFromBody(occ.SourceSpan), expectedValue, expectedSpans)
}

func (r Reader) returnLoweredObjectLiteralEntry(point cfg.Point, index int, literal factflow.ObjectLiteralView, rootValue product.Value, rootValueOK bool, sourceSpan SourceSpan, expectedValue product.Value, expectedSpans []SourceSpan) (Return, bool) {
	expected, ok := r.ValueTypeWithPresence(expectedValue)
	if !ok || expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || typ.IsNever(expected) || refinement.ContainsFreeTypeParam(expected) {
		return Return{}, false
	}
	var out Return
	found := false
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		entryExpected, ok := body.ExpectedTypeAtSegments(expected, entry.SuffixSegmentsView())
		if !ok || !readapi.ObligationTypeReportable(entryExpected) {
			return true
		}
		value, ok := r.result.SourceValueForExplanationAtBoundary(point, entry.Source())
		if !ok {
			return true
		}
		actual, _ := r.ValueTypeWithPresence(value)
		untrustedTopOrigin := r.ValueHasUntrustedTopOrigin(value)
		if actual == nil || (r.IsSubtype(actual, entryExpected) && !untrustedTopOrigin) {
			return true
		}
		label := returnExpectedLabel(index) + segment.FormatSegments(entry.SuffixSegmentsView())
		sourceLabel := entry.ValueLabel()
		if sourceLabel == "" {
			sourceLabel = label
		}
		out = Return{
			Point:              point,
			Index:              index,
			Value:              value,
			ValueHash:          r.ValueHash(value),
			TypeWithPresence:   actual,
			Expected:           entryExpected,
			ExpectedLabel:      label,
			SourceLabel:        sourceLabel,
			SourceSpan:         sourceSpanFromFactflow(entry.ValueSpan()),
			DeclarationSpan:    readmodelSourceSpanAt(expectedSpans, index),
			UntrustedTopOrigin: untrustedTopOrigin,
			ExplicitTopOrigin:  r.ValueHasExplicitTopOrigin(value),
		}
		out.Check = readapi.PlanReturnCheck(readapi.ReturnCheckPlan{
			Return:              out,
			ValueAdmissible:     r.ValueProofAdmissible(value, entryExpected),
			ValueProvenMismatch: r.ValueWitnessProvenMismatch(value, entryExpected),
			IsSubtype:           r.IsSubtype,
		})
		found = true
		return false
	})
	if found {
		return out, true
	}
	if rootValueOK {
		return r.returnLoweredObjectLiteralMissingRequired(point, index, literal, rootValue, expected, sourceSpan, expectedSpans)
	}
	return Return{}, false
}

func (r Reader) returnSemanticObjectLiteralMissingRequired(point cfg.Point, index int, literal body.ObjectLiteralFact, rootValue product.Value, expected typ.Type, sourceSpan SourceSpan, expectedSpans []SourceSpan) (Return, bool) {
	field, ok := body.MissingRequiredRecordField(expected, func(name string) bool {
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
	if !ok {
		return Return{}, false
	}
	return r.returnMissingRequiredField(point, index, rootValue, expected, sourceSpan, expectedSpans, field)
}

func (r Reader) returnLoweredObjectLiteralMissingRequired(point cfg.Point, index int, literal factflow.ObjectLiteralView, rootValue product.Value, expected typ.Type, sourceSpan SourceSpan, expectedSpans []SourceSpan) (Return, bool) {
	field, ok := body.MissingRequiredRecordField(expected, func(name string) bool {
		has := false
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			segments := entry.SuffixSegmentsView()
			if len(segments) != 1 {
				return true
			}
			seg := segments[0]
			if seg.Kind == segment.SegmentField && seg.Name == name {
				has = true
				return false
			}
			return true
		})
		return has
	})
	if !ok {
		return Return{}, false
	}
	return r.returnMissingRequiredField(point, index, rootValue, expected, sourceSpan, expectedSpans, field)
}

func (r Reader) returnMissingRequiredField(point cfg.Point, index int, rootValue product.Value, expected typ.Type, sourceSpan SourceSpan, expectedSpans []SourceSpan, field string) (Return, bool) {
	actual, _ := r.ValueTypeWithPresence(rootValue)
	if actual == nil {
		return Return{}, false
	}
	fieldType, ok := body.ExpectedTypeAtSegments(expected, []segment.Segment{{Kind: segment.SegmentField, Name: field}})
	if !ok || !readapi.ObligationTypeReportable(fieldType) {
		return Return{}, false
	}
	label := returnExpectedLabel(index) + "." + field
	ret := Return{
		Point:            point,
		Index:            index,
		Value:            rootValue,
		ValueHash:        r.ValueHash(rootValue),
		TypeWithPresence: actual,
		Expected:         fieldType,
		ExpectedLabel:    label,
		SourceLabel:      returnExpectedLabel(index),
		SourceSpan:       sourceSpan,
		DeclarationSpan:  readmodelSourceSpanAt(expectedSpans, index),
	}
	ret.Check = readapi.PlanReturnCheck(readapi.ReturnCheckPlan{
		Return:                   ret,
		MissingRequiredField:     field,
		MissingRequiredFieldType: fieldType,
		IsSubtype:                r.IsSubtype,
	})
	return ret, true
}

func (r Reader) returnValue(occ body.ReturnValueOccurrence, expectedValue product.Value, expectedSpans []SourceSpan) (Return, bool) {
	expected, ok := r.ValueTypeWithPresence(expectedValue)
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

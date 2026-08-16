package readmodel

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/refinement"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
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
	expectedSpans := sourceSpansFromBody(body.FunctionReturnTypeSpans(r.result.Function()))
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
	if source.Kind != sourceprovenance.SourceExpression || source.Expr == nil || !occ.HasLowered {
		return Return{}, false
	}
	if sourceprovenance.ConcreteRuntimeCastSource(source) {
		return Return{}, false
	}
	literal, ok := r.result.ObjectLiteralViewForSource(occ.Lowered)
	if !ok {
		return Return{}, false
	}
	rootValue, rootValueOK := r.result.SourceValueForExplanationAtBoundary(occ.Point, occ.Lowered)
	if rootValueOK {
		rootValue = r.returnSourceValueWithNilWitness(occ, rootValue)
	} else {
		rootValue, rootValueOK = r.SourceValue(occ.Point, source)
	}
	return r.returnLoweredObjectLiteralEntry(
		occ.Point,
		occ.Index,
		literal,
		rootValue,
		rootValueOK,
		occ.SourcePath,
		sourceSpanFromBody(occ.SourceSpan),
		expectedValue,
		expectedSpans,
		true,
	)
}

func (r Reader) returnDeclaredObjectLiteralEntry(occ body.ReturnValueOccurrence, expectedValue product.Value, expectedSpans []SourceSpan) (Return, bool) {
	source := occ.Source
	if r.result == nil || source.Kind != sourceprovenance.SourceExpression || source.Expr == nil {
		return Return{}, false
	}
	if !occ.HasPath || occ.SourcePath.IsEmpty() || occ.SourcePath.Symbol == 0 || len(occ.SourcePath.Segments) != 0 {
		return Return{}, false
	}
	if kind, ok := r.result.SymbolKind(occ.SourcePath.Symbol); ok && kind == symbol.Param {
		return Return{}, false
	}
	declaration, ok := r.result.DominatingPathRootDeclarationSource(occ.Point, occ.SourcePath)
	if !ok {
		return Return{}, false
	}
	if declarationPath, ok := r.result.ValueSourcePath(declaration.Source); ok && declarationPath.Symbol != occ.SourcePath.Symbol {
		return Return{}, false
	}
	literal, ok := r.result.ObjectLiteralViewForSource(declaration.Source)
	if !ok {
		return Return{}, false
	}
	rootValue, rootValueOK := r.SourceValue(occ.Point, source)
	return r.returnLoweredObjectLiteralEntry(
		occ.Point,
		occ.Index,
		literal,
		rootValue,
		rootValueOK,
		occ.SourcePath,
		sourceSpanFromBody(occ.SourceSpan),
		expectedValue,
		expectedSpans,
		!sourceprovenance.ConcreteRuntimeCastSource(source),
	)
}

func (r Reader) returnLoweredObjectLiteralEntry(point cfg.Point, index int, literal factflow.ObjectLiteralView, rootValue product.Value, rootValueOK bool, rootPath pathdom.Path, sourceSpan SourceSpan, expectedValue product.Value, expectedSpans []SourceSpan, reportMissingRequired bool) (Return, bool) {
	expected, ok := r.ValueTypeWithPresence(expectedValue)
	if !ok || expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || typ.IsNever(expected) || refinement.ContainsFreeTypeParam(expected) {
		return Return{}, false
	}
	objectExpected := r.loweredObjectLiteralExpectedType(point, literal, expected)
	var out Return
	found := false
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		entryExpected, ok := luatypeprojection.ExpectedTypeAtSegments(objectExpected, entry.SuffixSegmentsView())
		if !ok || !readapi.ObligationTypeReportable(entryExpected) {
			return true
		}
		value, ok := r.loweredObjectEntryValue(point, rootPath, entry)
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
			Point:               point,
			Index:               index,
			Value:               value,
			ValueHash:           r.ValueHash(value),
			TypeWithPresence:    actual,
			Expected:            entryExpected,
			ExpectedLabel:       label,
			SourceLabel:         sourceLabel,
			SourceIndexedRead:   false,
			SourceSpan:          sourceSpanFromFactflow(entry.ValueSpan()),
			DeclarationSpan:     readmodelSourceSpanAt(expectedSpans, index),
			UntrustedTopOrigin:  untrustedTopOrigin,
			ExplicitTopOrigin:   r.ValueHasExplicitTopOrigin(value),
			BodyParamObligation: r.result.HasBodyOwnedParamObligations(),
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
	if rootValueOK && reportMissingRequired {
		if rootActual, actualOK := r.ValueTypeWithPresence(rootValue); actualOK && rootActual != nil &&
			r.IsSubtype(rootActual, expected) &&
			r.ValueProofAdmissible(rootValue, expected) &&
			r.returnRootPathHasRequiredStaticMembers(point, rootPath, expected) &&
			!r.ValueHasUntrustedTopOrigin(rootValue) {
			return r.returnObjectLiteralSuccess(point, index, rootValue, rootActual, expected, sourceSpan, expectedSpans), true
		}
		if ret, ok := r.returnLoweredObjectLiteralMissingRequired(point, index, literal, rootValue, objectExpected, sourceSpan, expectedSpans); ok {
			return ret, true
		}
		return r.returnObjectLiteralSuccess(point, index, rootValue, objectExpected, expected, sourceSpan, expectedSpans), true
	}
	return Return{}, false
}

func (r Reader) loweredObjectLiteralExpectedType(point cfg.Point, literal factflow.ObjectLiteralView, expected typ.Type) typ.Type {
	if selected, ok := luatypeprojection.ExpectedObjectLiteralRecordCached(r.result.TypeValues(), expected, func(name string) (typ.Type, bool) {
		var out typ.Type
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			if out != nil {
				return false
			}
			segments := entry.SuffixSegmentsView()
			if field, ok := segment.DirectFieldName(segments); !ok || field != name {
				return true
			}
			value, ok := r.result.SourceValueForExplanationAtBoundary(point, entry.Source())
			if !ok {
				return false
			}
			out, _ = r.ValueTypeWithPresence(value)
			return false
		})
		return out, out != nil
	}); ok {
		return selected
	}
	return expected
}

func (r Reader) loweredObjectEntryValue(point cfg.Point, rootPath pathdom.Path, entry factflow.ObjectEntryView) (product.Value, bool) {
	if value, ok := r.loweredExpressionSourceValue(entry.Source()); ok {
		return value, true
	}
	if value, ok := r.loweredObjectEntryStaticMemberValue(point, rootPath, entry.SuffixSegmentsView()); ok {
		return value, true
	}
	return r.result.SourceValueForExplanationAtBoundary(point, entry.Source())
}

func (r Reader) loweredExpressionSourceValue(source factflow.ValueSource) (product.Value, bool) {
	if r.result == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr || source.ExprRef == 0 {
		return product.Value{}, false
	}
	if p, ok := r.result.ExpressionPathRef(source.ExprRef); ok && !p.IsEmpty() {
		return product.Value{}, false
	}
	value, ok := r.result.ExpressionValueRef(source.ExprRef)
	if !ok || !r.valueHasReadableType(value) {
		return product.Value{}, false
	}
	return value, true
}

func (r Reader) loweredObjectEntryStaticMemberValue(point cfg.Point, rootPath pathdom.Path, suffix []segment.Segment) (product.Value, bool) {
	if r.result == nil || rootPath.IsEmpty() || len(suffix) == 0 {
		return product.Value{}, false
	}
	memberPath := rootPath.AppendSegments(suffix)
	if memberPath.IsEmpty() {
		return product.Value{}, false
	}
	st, ok := r.result.StateAtBoundary(point)
	if !ok {
		return product.Value{}, false
	}
	ks := r.result.KeySpace()
	if ks == nil {
		return product.Value{}, false
	}
	return st.ReadPathStaticMember(ks, memberPath.Key())
}

func (r Reader) returnObjectLiteralSuccess(point cfg.Point, index int, rootValue product.Value, actual typ.Type, expected typ.Type, sourceSpan SourceSpan, expectedSpans []SourceSpan) Return {
	ret := Return{
		Point:               point,
		Index:               index,
		Value:               rootValue,
		ValueHash:           r.ValueHash(rootValue),
		TypeWithPresence:    actual,
		Expected:            expected,
		ExpectedLabel:       returnExpectedLabel(index),
		SourceLabel:         returnExpectedLabel(index),
		SourceSpan:          sourceSpan,
		DeclarationSpan:     readmodelSourceSpanAt(expectedSpans, index),
		BodyParamObligation: r.result.HasBodyOwnedParamObligations(),
	}
	ret.Check = readapi.PlanReturnCheck(readapi.ReturnCheckPlan{
		Return:          ret,
		ValueAdmissible: true,
		IsSubtype:       r.IsSubtype,
	})
	return ret
}

func (r Reader) returnLoweredObjectLiteralMissingRequired(point cfg.Point, index int, literal factflow.ObjectLiteralView, rootValue product.Value, expected typ.Type, sourceSpan SourceSpan, expectedSpans []SourceSpan) (Return, bool) {
	field, ok := luatypeprojection.MissingRequiredRecordField(expected, func(name string) bool {
		has := false
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			segments := entry.SuffixSegmentsView()
			if field, ok := segment.DirectFieldName(segments); ok && field == name {
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
	fieldType, ok := luatypeprojection.ExpectedTypeAtSegments(expected, []segment.Segment{{Kind: segment.SegmentField, Name: field}})
	if !ok || !readapi.ObligationTypeReportable(fieldType) {
		return Return{}, false
	}
	label := returnExpectedLabel(index) + "." + field
	ret := Return{
		Point:               point,
		Index:               index,
		Value:               rootValue,
		ValueHash:           r.ValueHash(rootValue),
		TypeWithPresence:    actual,
		Expected:            fieldType,
		ExpectedLabel:       label,
		SourceLabel:         returnExpectedLabel(index),
		SourceIndexedRead:   false,
		SourceSpan:          sourceSpan,
		DeclarationSpan:     readmodelSourceSpanAt(expectedSpans, index),
		BodyParamObligation: r.result.HasBodyOwnedParamObligations(),
	}
	ret.Check = readapi.PlanReturnCheck(readapi.ReturnCheckPlan{
		Return:                   ret,
		MissingRequiredField:     field,
		MissingRequiredFieldType: fieldType,
		IsSubtype:                r.IsSubtype,
	})
	return ret, true
}

func (r Reader) returnRootPathHasRequiredStaticMembers(point cfg.Point, rootPath pathdom.Path, expected typ.Type) bool {
	if r.result == nil || rootPath.IsEmpty() || expected == nil {
		return false
	}
	st, ok := r.result.StateAtBoundary(point)
	if !ok {
		return false
	}
	ks := r.result.KeySpace()
	if ks == nil {
		return false
	}
	_, missing := luatypeprojection.MissingRequiredRecordField(expected, func(name string) bool {
		memberPath := rootPath.Append(segment.Segment{Kind: segment.SegmentField, Name: name})
		return r.returnRootPathHasRequiredMemberProof(point, st, ks, memberPath, expected, name)
	})
	return !missing
}

func (r Reader) returnRootPathHasRequiredMemberProof(point cfg.Point, st state.State, ks *keyspace.KeySpace, memberPath pathdom.Path, expected typ.Type, name string) bool {
	if memberPath.IsEmpty() {
		return false
	}
	if _, ok := st.ReadPathStaticMember(ks, memberPath.Key()); ok {
		return true
	}
	fieldType, ok := luatypeprojection.ExpectedTypeAtSegments(expected, []segment.Segment{{Kind: segment.SegmentField, Name: name}})
	if !ok || fieldType == nil {
		return false
	}
	if _, ok := unwrap.Annotated(fieldType).(*typ.Function); !ok {
		return false
	}
	key, ok := factflow.CalleePathKeyFromPath(memberPath)
	if !ok {
		return false
	}
	fn, ok := r.result.FunctionValueTypeForCalleePath(key)
	return ok && fn != nil && r.IsSubtype(fn, fieldType)
}

func (r Reader) returnValue(occ body.ReturnValueOccurrence, expectedValue product.Value, expectedSpans []SourceSpan) (Return, bool) {
	expected, ok := r.ValueTypeWithPresence(expectedValue)
	if !ok || expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || typ.IsNever(expected) || refinement.ContainsFreeTypeParam(expected) {
		return Return{}, false
	}
	value, ok := r.returnOccurrenceValue(occ)
	if !ok {
		return Return{}, false
	}
	actual, _ := r.returnActualType(occ, value)
	if projected, projectedOK := r.returnObjectLiteralProjectedActualType(occ, actual, expected); projectedOK {
		actual = projected
	}
	ret := Return{
		Point:               occ.Point,
		Index:               occ.Index,
		Value:               value,
		ValueHash:           r.ValueHash(value),
		TypeWithPresence:    actual,
		Expected:            expected,
		ExpectedLabel:       returnExpectedLabel(occ.Index),
		SourceLabel:         occ.SourceLabel,
		SourceIndexedRead:   r.returnSourceIndexedReadAt(occ),
		SourceSpan:          sourceSpanFromBody(occ.SourceSpan),
		DeclarationSpan:     readmodelSourceSpanAt(expectedSpans, occ.Index),
		Nilability:          r.nilabilityProvenance(occ.Point, occ.Source.Expr, value),
		UntrustedTopOrigin:  r.ValueHasUntrustedTopOrigin(value),
		ExplicitTopOrigin:   r.ValueHasExplicitTopOrigin(value),
		BodyParamObligation: r.result.HasBodyOwnedParamObligations(),
	}
	ret.Check = readapi.PlanReturnCheck(readapi.ReturnCheckPlan{
		Return:              ret,
		ValueAdmissible:     r.ValueProofAdmissible(value, expected),
		ValueProvenMismatch: r.ValueWitnessProvenMismatch(value, expected),
		IsSubtype:           r.IsSubtype,
	})
	return ret, true
}

func (r Reader) returnObjectLiteralProjectedActualType(occ body.ReturnValueOccurrence, actual typ.Type, expected typ.Type) (typ.Type, bool) {
	if r.result == nil ||
		occ.Source.Kind != sourceprovenance.SourceExpression ||
		occ.Source.Expr == nil ||
		!occ.HasLowered ||
		expected == nil {
		return nil, false
	}
	if _, ok := r.result.ObjectLiteralViewForSource(occ.Lowered); !ok {
		return nil, false
	}
	if actual != nil && r.IsSubtype(actual, expected) {
		return nil, false
	}
	projected, ok := r.result.ExpressionTypeBeforeBoundary(occ.Point, occ.Source.Expr)
	if !ok || projected == nil || typ.IsAny(projected) || typ.IsUnknown(projected) || typ.IsNever(projected) {
		return nil, false
	}
	if !r.IsSubtype(projected, expected) {
		return nil, false
	}
	return projected, true
}

func (r Reader) returnActualType(occ body.ReturnValueOccurrence, value product.Value) (typ.Type, bool) {
	actual, ok := r.ValueTypeWithPresence(value)
	if ok && actual != nil && !typ.IsAny(actual) && !typ.IsUnknown(actual) {
		return actual, true
	}
	if r.result == nil || occ.Source.Kind != sourceprovenance.SourceExpression || occ.Source.Expr == nil {
		return actual, ok
	}
	if projected, projectedOK := r.result.ExpressionTypeBeforeBoundary(occ.Point, occ.Source.Expr); projectedOK &&
		projected != nil &&
		!typ.IsAny(projected) &&
		!typ.IsUnknown(projected) {
		return projected, true
	}
	declared, declaredOK := r.result.DeclaredExpressionTypeAt(occ.Point, occ.Source.Expr)
	if !declaredOK || declared == nil || typ.IsAny(declared) || typ.IsUnknown(declared) {
		return actual, ok
	}
	return declared, true
}

func (r Reader) returnOccurrenceValue(occ body.ReturnValueOccurrence) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	if occ.HasLowered {
		if value, ok := r.result.SourceValueForExplanationAtBoundary(occ.Point, occ.Lowered); ok &&
			r.returnLoweredSourceValueCanReplaceSlot(value) {
			return r.returnSourceValueWithNilWitness(occ, value), true
		}
	}
	if occ.Source.Kind == sourceprovenance.SourceExpression && occ.Source.Expr != nil {
		if value, ok := r.result.ExpressionValueAtBoundary(occ.Point, occ.Source.Expr); ok {
			return r.returnSourceValueWithNilWitness(occ, value), true
		}
	}
	if occ.HasLowered {
		if value, ok := r.result.SourceValueForExplanationAtBoundary(occ.Point, occ.Lowered); ok {
			return r.returnSourceValueWithNilWitness(occ, value), true
		}
	}
	value, ok := r.SourceValue(occ.Point, occ.Source)
	if !ok {
		return product.Value{}, false
	}
	return r.returnSourceValueWithNilWitness(occ, value), true
}

func (r Reader) returnLoweredSourceValueCanReplaceSlot(value product.Value) bool {
	return r.valueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value)
}

func (r Reader) returnSourceValueWithNilWitness(occ body.ReturnValueOccurrence, value product.Value) product.Value {
	if r.result == nil || occ.Source.Kind != sourceprovenance.SourceExpression || occ.Source.Expr == nil {
		return value
	}
	if sourceprovenance.ConcreteRuntimeCastSource(occ.Source) {
		return value
	}
	if r.returnSourceIndexedReadAt(occ) && r.result.Registry() != nil {
		value = product.WithPresence(r.result.Registry(), value, presence.Maybe())
	}
	return r.result.WithMemberReadNilWitness(occ.Point, occ.Source.Expr, value)
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

func (r Reader) returnSourceIndexedReadAt(occ body.ReturnValueOccurrence) bool {
	if r.result == nil || occ.Source.Kind != sourceprovenance.SourceExpression || occ.Source.Expr == nil {
		return false
	}
	if r.result.AssignmentSourceIndexedReadAt(occ.Point, occ.Source.Expr) {
		return true
	}
	inner, ok := sourceprovenance.ProofInner(occ.Source.Expr)
	return ok && inner != nil && inner != occ.Source.Expr &&
		r.result.AssignmentSourceIndexedReadAt(occ.Point, inner)
}

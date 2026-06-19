package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

type objectLiteralExprReader interface {
	ObjectLiteralExpr(factflow.ExprRef) (factflow.ObjectLiteral, bool)
}

type expressionPathRefReader interface {
	ExpressionPathRef(factflow.ExprRef) (pathdom.Path, bool)
}

func projectReturnParamPathAliases(result ResultReader) []summary.ReturnParamPathAlias {
	params := parameterValuePaths(result)
	sourceReader, hasSources := result.(returnValueSourceReader)
	objectReader, hasObjects := result.(objectLiteralExprReader)
	pathReader, hasPaths := result.(expressionPathRefReader)
	if len(params) == 0 || !hasSources || !hasObjects || !hasPaths {
		return nil
	}
	var out []summary.ReturnParamPathAlias
	for _, returnPoint := range result.ReturnPoints() {
		sources, ok := sourceReader.ReturnValueSources(returnPoint)
		if !ok {
			continue
		}
		for returnIndex, source := range sources {
			out = append(out, projectReturnSourceParamAliases(
				returnIndex,
				nil,
				source,
				params,
				result,
				objectReader,
				pathReader,
				nil,
			)...)
		}
	}
	return out
}

func projectReturnSourceParamAliases(
	returnIndex int,
	prefix []segment.Segment,
	source factflow.ValueSource,
	params []pathdom.Path,
	result ResultReader,
	objectReader objectLiteralExprReader,
	pathReader expressionPathRefReader,
	active map[factflow.ExprRef]bool,
) []summary.ReturnParamPathAlias {
	if returnIndex < 0 || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return nil
	}
	lit, ok := objectReader.ObjectLiteralExpr(source.ExprRef)
	if !ok {
		return nil
	}
	if active[source.ExprRef] {
		return nil
	}
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[source.ExprRef] = true
	defer delete(active, source.ExprRef)

	var out []summary.ReturnParamPathAlias
	for _, entry := range lit.Entries() {
		entrySource := entry.Source()
		memberSegments := appendSegments(prefix, entry.Suffix().Segments)
		memberKey, ok := heapidentity.StaticMemberSuffixKey(memberSegments)
		if !ok {
			continue
		}
		if entrySource.Kind == factflow.ValueSourceExpression && entrySource.HasExpr {
			if sourcePath, ok := pathReader.ExpressionPathRef(entrySource.ExprRef); ok {
				if placeholder, ok := returnAliasPlaceholderPath(sourcePath, params, result); ok {
					out = append(out, summary.ReturnParamPathAlias{
						ReturnIndex: returnIndex,
						Member:      memberKey,
						Source:      placeholder.Key(),
					})
				}
			}
			out = append(out, projectReturnSourceParamAliases(
				returnIndex,
				memberSegments,
				entrySource,
				params,
				result,
				objectReader,
				pathReader,
				active,
			)...)
		}
	}
	return out
}

func returnAliasPlaceholderPath(
	sourcePath pathdom.Path,
	params []pathdom.Path,
	result ResultReader,
) (pathdom.Path, bool) {
	index, ok := returnAliasParamIndex(sourcePath, params)
	if !ok {
		return pathdom.Path{}, false
	}
	if returnAliasParamReassigned(index, params, result) {
		return pathdom.Path{}, false
	}
	return pathdom.NewPlaceholder(index).AppendSegments(sourcePath.Segments), true
}

func returnAliasParamIndex(sourcePath pathdom.Path, params []pathdom.Path) (int, bool) {
	if sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return 0, false
	}
	for i, param := range params {
		if param.Symbol == sourcePath.Symbol {
			return i, true
		}
	}
	return 0, false
}

func returnAliasParamReassigned(index int, params []pathdom.Path, result ResultReader) bool {
	if index < 0 || index >= len(params) {
		return true
	}
	slot := key.SymbolValue(params[index].Symbol)
	if slot == "" {
		return true
	}
	reassignedReader, ok := result.(reassignedParameterValueSlotReader)
	if !ok {
		return false
	}
	_, reassigned := reassignedReader.ReassignedParameterValueSlots()[slot]
	return reassigned
}

func appendSegments(prefix, suffix []segment.Segment) []segment.Segment {
	if len(prefix) == 0 {
		return append([]segment.Segment(nil), suffix...)
	}
	out := make([]segment.Segment, 0, len(prefix)+len(suffix))
	out = append(out, prefix...)
	out = append(out, suffix...)
	return out
}

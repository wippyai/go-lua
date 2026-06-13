package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func projectReturnConditionParamRefinements(
	reg *axis.Registry,
	result ResultReader,
) []ReturnConditionParamRefinement {
	sourceReader, ok := result.(returnValueSourceReader)
	if !ok {
		return nil
	}
	conditionReader, ok := result.(expressionConditionReader)
	if !ok {
		return nil
	}
	params := normalReturnParamPaths(result)
	if len(params) == 0 {
		return nil
	}
	var out []ReturnConditionParamRefinement
	for _, point := range result.ReturnPoints() {
		sources, ok := sourceReader.ReturnValueSources(point)
		if !ok {
			continue
		}
		for returnIndex, source := range sources {
			if !returnConditionSource(source) || !source.HasExpr {
				continue
			}
			condition, ok := conditionReader.ExpressionCondition(source.ExprRef)
			if !ok {
				continue
			}
			out = appendReturnConditionParamRefinements(reg, out, returnIndex, true, condition.RefinementsForValue(true), params)
			out = appendReturnConditionParamRefinements(reg, out, returnIndex, false, condition.RefinementsForValue(false), params)
		}
	}
	return normalizeReturnConditionParamRefinements(reg, out)
}

func returnConditionSource(source factflow.ValueSource) bool {
	if source.Kind == factflow.ValueSourceExpression {
		return true
	}
	return source.Kind == factflow.ValueSourceCall && source.ResultIndex == 0
}

func appendReturnConditionParamRefinements(
	reg *axis.Registry,
	out []ReturnConditionParamRefinement,
	returnIndex int,
	returnValue bool,
	refinements []factflow.PostconditionRefinement,
	params []path.Path,
) []ReturnConditionParamRefinement {
	for _, refinement := range refinements {
		target, ok := paramPlaceholderPath(refinement.TargetPath(), params)
		if !ok {
			continue
		}
		value, ok := refinement.Value().Constraint()
		if !ok || !usefulReturnConditionValue(reg, value) {
			continue
		}
		out = append(out, ReturnConditionParamRefinement{
			ReturnIndex: returnIndex,
			ReturnValue: returnValue,
			Target:      target,
			Value:       value,
		})
	}
	return out
}

func paramPlaceholderPath(target path.Path, params []path.Path) (path.Path, bool) {
	if target.IsEmpty() {
		return path.Path{}, false
	}
	for i, param := range params {
		if param.Symbol == 0 || target.Symbol != param.Symbol {
			continue
		}
		out := path.NewPlaceholder(i)
		for _, seg := range target.Segments {
			out = out.Append(seg)
		}
		return out, true
	}
	return path.Path{}, false
}

package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func projectReturnConditionParamRefinements(
	reg *axis.Registry,
	result ResultReader,
) []summary.ReturnConditionParamRefinement {
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
	var out []summary.ReturnConditionParamRefinement
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
	return out
}

func returnConditionSource(source factflow.ValueSource) bool {
	if source.Kind == factflow.ValueSourceExpression {
		return true
	}
	return source.Kind == factflow.ValueSourceCall && source.ResultIndex == 0
}

func appendReturnConditionParamRefinements(
	reg *axis.Registry,
	out []summary.ReturnConditionParamRefinement,
	returnIndex int,
	returnValue bool,
	refinements []factflow.PostconditionRefinement,
	params []path.Path,
) []summary.ReturnConditionParamRefinement {
	_ = reg
	for _, refinement := range refinements {
		target, ok := paramPlaceholderPath(refinement.TargetPath(), params)
		if !ok {
			continue
		}
		value, ok := refinement.Value().Constraint()
		if !ok {
			continue
		}
		out = append(out, summary.ReturnConditionParamRefinement{
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

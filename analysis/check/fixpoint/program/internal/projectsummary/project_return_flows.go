package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func projectReturnFlows(result ResultReader) []summary.ReturnFlow {
	params := parameterValuePaths(result)
	sourceReader, hasSources := result.(returnValueSourceReader)
	pathReader, hasPaths := result.(expressionPathRefReader)
	if len(params) == 0 || !hasSources || !hasPaths {
		return nil
	}
	points := result.ReturnPoints()
	if len(points) == 0 {
		return nil
	}
	sourcesByPoint := make([][]factflow.ValueSource, 0, len(points))
	arity := 0
	for _, point := range points {
		sources, ok := sourceReader.ReturnValueSources(point)
		if !ok {
			return nil
		}
		if len(sources) > arity {
			arity = len(sources)
		}
		sourcesByPoint = append(sourcesByPoint, sources)
	}
	if arity == 0 {
		return nil
	}
	var out []summary.ReturnFlow
	for returnIndex := 0; returnIndex < arity; returnIndex++ {
		var candidate summary.ReturnFlow
		candidateSet := false
		mixed := false
		for _, sources := range sourcesByPoint {
			if returnIndex >= len(sources) {
				mixed = true
				break
			}
			flow, ok := returnFlowFromSource(returnIndex, sources[returnIndex], params, result, pathReader)
			if !ok {
				mixed = true
				break
			}
			if !candidateSet {
				candidate = flow
				candidateSet = true
				continue
			}
			if !returnFlowsSame(candidate, flow) {
				mixed = true
				break
			}
		}
		if !mixed && candidateSet {
			out = append(out, candidate)
		}
	}
	return out
}

func returnFlowFromSource(
	returnIndex int,
	source factflow.ValueSource,
	params []pathdom.Path,
	result ResultReader,
	pathReader expressionPathRefReader,
) (summary.ReturnFlow, bool) {
	sourcePath, ok := valueSourcePath(result, pathReader, source)
	if !ok || sourcePath.Symbol == 0 {
		return summary.ReturnFlow{}, false
	}
	placeholder, ok := returnAliasPlaceholderPath(sourcePath, params, result)
	if !ok {
		return summary.ReturnFlow{}, false
	}
	param := placeholder.PlaceholderIndex()
	if param < 0 {
		return summary.ReturnFlow{}, false
	}
	if len(placeholder.Segments) == 0 {
		return summary.ReturnFlow{
			ReturnIndex: returnIndex,
			Kind:        summary.ReturnFlowParam,
			Param:       param,
		}, true
	}
	return summary.ReturnFlow{
		ReturnIndex: returnIndex,
		Kind:        summary.ReturnFlowParamMember,
		Param:       param,
		Path:        append([]segment.Segment(nil), placeholder.Segments...),
	}, true
}

func returnFlowsSame(a, b summary.ReturnFlow) bool {
	if a.ReturnIndex != b.ReturnIndex || a.Kind != b.Kind || a.Param != b.Param || len(a.Path) != len(b.Path) {
		return false
	}
	for i := range a.Path {
		if a.Path[i] != b.Path[i] {
			return false
		}
	}
	return true
}

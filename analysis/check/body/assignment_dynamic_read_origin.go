package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// AssignmentSourceMatchesDynamicTargetRead reports whether an ordinary dynamic
// write stores back a value proven to have been read from the same container/key
// pair. This is the canonical proof for self-writes such as:
//
//	for key, value in pairs(item) do
//	    item[key] = value
//	end
func (r *Result) AssignmentSourceMatchesDynamicTargetRead(point cfg.Point, fact OrdinaryAssignmentFact) bool {
	if r == nil || r.visibility == nil || fact.Value == nil {
		return false
	}
	write, ok := r.DynamicIndexWrite(point)
	if !ok {
		return false
	}
	sourcePath, ok := r.ExpressionPath(fact.Value)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return false
	}
	sourceKey, ok := r.visibility.StateKeyAt(point, sourcePath)
	if !ok {
		return false
	}
	keyPath, ok := dynamicIndexWriteKeyPath(r, write)
	if !ok {
		return false
	}
	keyStateKey, ok := r.visibility.StateKeyAt(point, keyPath)
	if !ok {
		return false
	}
	containerKey, ok := r.visibility.VisibleKeyspaceKeyAt(point, write.TablePathRef())
	if !ok {
		return false
	}
	for _, origin := range r.stateRead(point).DynamicIndexReadOriginsForValue(sourceKey) {
		if origin.Container == containerKey && origin.Key == keyStateKey {
			return true
		}
	}
	return false
}

func dynamicIndexWriteKeyPath(r *Result, write factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	source := write.KeySource()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return pathdom.Path{}, false
	}
	return r.facts.ExpressionPathRef(source.ExprRef)
}

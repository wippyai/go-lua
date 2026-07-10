package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func markStableReturnHeapObjects(
	reg *axis.Registry,
	result ResultReader,
	objects map[identity.ID]heapidentity.TableObject,
	returns []product.Value,
) map[identity.ID]heapidentity.TableObject {
	if reg == nil || result == nil || len(objects) == 0 || len(returns) == 0 {
		return objects
	}
	out := objects
	cloned := false
	for index, value := range returns {
		id, ok := identityvalue.ExactID(reg, value)
		if !ok {
			continue
		}
		object, ok := out[id]
		if !ok || object.StableShape() {
			continue
		}
		if !returnSlotStableAtEveryReturn(reg, result, index, id) {
			continue
		}
		if !cloned {
			out = heapidentity.CloneMap(objects)
			cloned = true
		}
		out[id] = object.WithStableShape()
	}
	return out
}

func returnSlotStableAtEveryReturn(reg *axis.Registry, result ResultReader, index int, id identity.ID) bool {
	if index < 0 || id == (identity.ID{}) {
		return false
	}
	sources, ok := result.(returnValueSourceReader)
	if !ok {
		return false
	}
	stable, ok := result.(stableShapeSourceReader)
	if !ok {
		return false
	}
	seen := false
	for _, point := range result.ReturnPoints() {
		if projectedReturnPointUnreachable(reg, result, point) {
			continue
		}
		values, ok := sources.ReturnValueSources(point)
		if !ok || index >= len(values) {
			return false
		}
		value, ok := sourceValueForStableReturnPoint(result, point, values[index])
		if !ok {
			return false
		}
		got, ok := identityvalue.ExactID(reg, value)
		if !ok || got != id {
			return false
		}
		if !stable.SourceHasStableShapeBeforeBoundary(point, values[index]) {
			return false
		}
		seen = true
	}
	return seen
}

func sourceValueForStableReturnPoint(result ResultReader, point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	reader, ok := result.(sourceValueBeforeBoundaryReader)
	if !ok {
		return product.Value{}, false
	}
	return reader.SourceValueBeforeBoundary(point, source)
}

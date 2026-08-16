package allocation

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/value"
)

type operand struct {
	result     *value.AllocationResult
	key        heap.Key
	coordinate value.Coordinate
	fresh      value.Value
	digest     [32]byte
}

func allocationOperandContentForSchema(schema *value.Schema, candidate operand) (operand, [32]byte, bool) {
	if schema == nil || candidate.result == nil || !candidate.result.Owns(schema) {
		return operand{}, [32]byte{}, false
	}
	key, keyOK := candidate.result.Key()
	coordinate, coordinateOK := candidate.result.Coordinate()
	fresh, freshOK := candidate.result.Fresh()
	id, idOK := candidate.result.KeyID()
	digest := [32]byte(id)
	if !keyOK || !coordinateOK || !freshOK || !idOK || digest == [32]byte{} || candidate.key != key || candidate.coordinate != coordinate ||
		!schema.Same(candidate.fresh, fresh) || candidate.digest != digest {
		return operand{}, [32]byte{}, false
	}
	return candidate, digest, true
}

func allocationOperandForSchema(schema *value.Schema, key heap.Key) (operand, bool) {
	if schema == nil {
		return operand{}, false
	}
	result, ok := schema.AllocationResultFor(key)
	if !ok {
		return operand{}, false
	}
	coordinate, coordinateOK := result.Coordinate()
	fresh, freshOK := result.Fresh()
	id, idOK := result.KeyID()
	digest := [32]byte(id)
	if !coordinateOK || !freshOK || !idOK || digest == [32]byte{} {
		return operand{}, false
	}
	return operand{result: result, key: key, coordinate: coordinate, fresh: fresh, digest: digest}, true
}

func allocationResultForSchema(schema *value.Schema, key heap.Key) (value.Coordinate, value.Value, bool) {
	if schema == nil {
		return value.Coordinate{}, value.Value{}, false
	}
	result, ok := schema.AllocationResultFor(key)
	if !ok {
		return value.Coordinate{}, value.Value{}, false
	}
	coordinate, coordinateOK := result.Coordinate()
	fresh, freshOK := result.Fresh()
	if !coordinateOK || !freshOK || schema.Equal(fresh, schema.Default()) {
		return value.Coordinate{}, value.Value{}, false
	}
	return coordinate, fresh, true
}

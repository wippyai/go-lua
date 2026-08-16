package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/identity"
)

type result struct {
	coordinate value.Coordinate
	fact       value.Value
	absent     bool
}

func globalContentForSchema(schema *value.Schema) func(identity.ContentID) (identity.ContentID, [32]byte, bool) {
	return func(binding identity.ContentID) (identity.ContentID, [32]byte, bool) {
		if schema == nil || !binding.Available() {
			return identity.ContentID{}, [32]byte{}, false
		}
		receipt, resultOK := schema.GlobalBootstrapResultForID(binding)
		id, idOK := receipt.ID()
		if !resultOK || !idOK {
			return identity.ContentID{}, [32]byte{}, false
		}
		return binding, [32]byte(id), true
	}
}

func globalResultForSchema(schema *value.Schema, binding identity.ContentID) (result, bool) {
	if schema == nil {
		return result{}, false
	}
	receipt, ok := schema.GlobalBootstrapResultForID(binding)
	if !ok {
		return result{}, false
	}
	coordinate, coordinateOK := receipt.Coordinate()
	if !coordinateOK {
		return result{}, false
	}
	if receipt.Absent() {
		return result{coordinate: coordinate, absent: true}, true
	}
	fact, factOK := receipt.Fact()
	if !factOK {
		return result{}, false
	}
	return result{coordinate: coordinate, fact: fact}, true
}

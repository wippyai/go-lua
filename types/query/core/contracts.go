package core

import (
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// PreserveFunctionContracts retains proved behavior when a known value is
// viewed through a compatible signature. Unknown values supply no guarantees.
func PreserveFunctionContracts(view, actual typ.Type) typ.Type {
	return preserveFunctionContracts(view, actual, 0)
}

func preserveFunctionContracts(view, actual typ.Type, depth int) typ.Type {
	if view == nil || actual == nil || depth >= typ.DefaultRecursionDepth {
		return view
	}
	switch target := unwrap.Alias(view).(type) {
	case *typ.Function:
		if spec := commonFunctionSpec(actual, depth+1); spec != nil && subtype.IsSubtype(actual, target) {
			return target.WithSpec(spec)
		}
	case *typ.Record:
		result := target
		for _, field := range target.Fields {
			source, ok := Field(actual, field.Name)
			if !ok {
				continue
			}
			enriched := preserveFunctionContracts(field.Type, source, depth+1)
			if enriched != field.Type {
				field.Type = enriched
				result = result.WithField(field)
			}
		}
		if result != target {
			return result
		}
	}
	return view
}

// A union or overload view only retains behavior shared by every alternative.
func commonFunctionSpec(actual typ.Type, depth int) typ.SpecInfo {
	if depth >= typ.DefaultRecursionDepth {
		return nil
	}
	var members []typ.Type
	switch source := unwrap.Alias(actual).(type) {
	case *typ.Function:
		return source.Spec
	case *typ.Intersection:
		members = source.Members
	case *typ.Union:
		members = source.Members
	default:
		return nil
	}
	var common typ.SpecInfo
	for i, member := range members {
		spec := commonFunctionSpec(member, depth+1)
		if spec == nil {
			return nil
		}
		if i == 0 {
			common = spec
		} else if !common.Equals(spec) {
			return nil
		}
	}
	return common
}

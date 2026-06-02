package callbackenv

import (
	"cmp"
	"slices"

	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
)

// GlobalName is an external contract name, not a solver identity. Canonical
// facts lower it to cfg.SymbolID before storing callback environment facts.
type GlobalName = globalenv.Name

// OverlayBinding is one callback-scoped external global name and its type.
type OverlayBinding = globalenv.TypeBinding

// Overlay is a deterministic, normalized set of callback environment bindings.
type Overlay = globalenv.TypeOverlay

// ParamOverlay attaches a callback environment overlay to a callback parameter.
type ParamOverlay struct {
	ParamIndex int
	Overlay    Overlay
}

// Overlays is a deterministic, normalized set of callback parameter overlays.
type Overlays []ParamOverlay

// OverlayFromContractMap normalizes a contract EnvOverlay map into the domain
// carrier. Raw string-keyed maps are accepted only at the contract boundary.
func OverlayFromContractMap(env map[string]typ.Type) Overlay {
	return globalenv.TypeOverlayFromMap(env)
}

// OverlaysFromFunction normalizes a function contract's callback environment
// overlays into the domain carrier. Function signatures are a boundary format;
// analysis-facing callback overlay state is the sorted Overlays carrier.
func OverlaysFromFunction(t typ.Type) Overlays {
	return OverlaysFromContractSpec(contract.ExtractSpec(t))
}

// OverlaysFromContractSpec normalizes callback EnvOverlay maps from a contract
// spec into the domain carrier.
func OverlaysFromContractSpec(spec *contract.Spec) Overlays {
	if spec == nil || len(spec.Callbacks) == 0 {
		return nil
	}
	var out Overlays
	for idx, cb := range spec.Callbacks {
		if cb == nil || len(cb.EnvOverlay) == 0 {
			continue
		}
		if overlay := OverlayFromContractMap(cb.EnvOverlay); len(overlay) > 0 {
			out = JoinProducts(out, Overlays{{
				ParamIndex: idx,
				Overlay:    overlay,
			}})
		}
	}
	return out
}

// MergeOverlay joins two normalized overlays by external global name.
func MergeOverlay(base, overlay Overlay) Overlay {
	return globalenv.MergeTypeOverlay(base, overlay)
}

// MergeContractOverlay joins two external contract EnvOverlay maps at the
// boundary without exposing map-shaped overlay state to the analysis domain.
func MergeContractOverlay(base, overlay map[string]typ.Type) map[string]typ.Type {
	return MergeOverlay(OverlayFromContractMap(base), OverlayFromContractMap(overlay)).ToContractMap()
}

// MergeIntoContractOverlay joins a domain overlay into an external contract map.
func MergeIntoContractOverlay(base map[string]typ.Type, overlay Overlay) map[string]typ.Type {
	return MergeOverlay(OverlayFromContractMap(base), overlay).ToContractMap()
}

// JoinProducts joins callback overlays keyed by callback parameter index.
func JoinProducts(base, overlay Overlays) Overlays {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	byParam := make(map[int]Overlay, len(base)+len(overlay))
	for _, param := range base {
		if len(param.Overlay) == 0 {
			continue
		}
		byParam[param.ParamIndex] = MergeOverlay(nil, param.Overlay)
	}
	for _, param := range overlay {
		if len(param.Overlay) == 0 {
			continue
		}
		byParam[param.ParamIndex] = MergeOverlay(byParam[param.ParamIndex], param.Overlay)
	}
	if len(byParam) == 0 {
		return nil
	}
	indices := make([]int, 0, len(byParam))
	for idx := range byParam {
		indices = append(indices, idx)
	}
	slices.Sort(indices)
	out := make(Overlays, 0, len(indices))
	for _, idx := range indices {
		out = append(out, ParamOverlay{ParamIndex: idx, Overlay: byParam[idx]})
	}
	return out
}

// ForParam returns the overlay for paramIndex.
func (o Overlays) ForParam(paramIndex int) (Overlay, bool) {
	idx, ok := slices.BinarySearchFunc(o, paramIndex, func(param ParamOverlay, target int) int {
		return cmp.Compare(param.ParamIndex, target)
	})
	if !ok {
		return nil, false
	}
	return o[idx].Overlay, true
}

func overlaysFromMutableMap(input map[int]map[GlobalName]typ.Type) Overlays {
	if len(input) == 0 {
		return nil
	}
	var out Overlays
	for paramIdx, overlay := range input {
		if len(overlay) == 0 {
			continue
		}
		bindings := make(Overlay, 0, len(overlay))
		for name, t := range overlay {
			if name == "" || t == nil {
				continue
			}
			bindings = append(bindings, OverlayBinding{Name: name, Type: t})
		}
		out = append(out, ParamOverlay{ParamIndex: paramIdx, Overlay: MergeOverlay(nil, bindings)})
	}
	return JoinProducts(nil, out)
}

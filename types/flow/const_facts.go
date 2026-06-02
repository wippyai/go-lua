package flow

import "github.com/wippyai/go-lua/types/cfg"

// ConstValueAtSym returns a constant value for sym at point p from immutable
// flow inputs. It returns nil for unknown constants and absent facts.
func (in *Inputs) ConstValueAtSym(p cfg.Point, sym cfg.SymbolID) *ConstValue {
	if in == nil || sym == 0 || in.ConstValues == nil {
		return nil
	}
	atPoints := in.ConstValues[sym]
	if atPoints == nil {
		return nil
	}
	val := atPoints[p]
	if val != nil && val.Kind == ConstUnknown {
		return nil
	}
	return val
}

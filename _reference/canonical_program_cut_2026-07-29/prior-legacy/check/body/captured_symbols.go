package body

import "github.com/wippyai/go-lua/analysis/symbol"

// CapturedSymbols returns the stable symbol identities captured directly by
// this body in binder order. The detached IDs are boundary metadata; callers
// cannot recover binder or syntax authority from them.
func (r *Result) CapturedSymbols() []symbol.ID {
	if r == nil || r.bindings == nil || r.function == nil {
		return nil
	}
	captures := r.bindings.DirectCaptures(r.function)
	out := make([]symbol.ID, 0, len(captures))
	seen := make(map[symbol.ID]struct{}, len(captures))
	for _, capture := range captures {
		if capture.Captured == 0 {
			continue
		}
		if _, duplicate := seen[capture.Captured]; duplicate {
			continue
		}
		seen[capture.Captured] = struct{}{}
		out = append(out, capture.Captured)
	}
	return out
}

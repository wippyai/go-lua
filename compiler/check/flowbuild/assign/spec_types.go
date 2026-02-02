package assign

import (
	"github.com/wippyai/go-lua/compiler/check/api"
)

// MergeSpecTypes merges base and override, with override taking precedence.
// This is used to build per-operation overlay types during extraction.
func MergeSpecTypes(base, override api.SpecTypes) api.SpecTypes {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(api.SpecTypes, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

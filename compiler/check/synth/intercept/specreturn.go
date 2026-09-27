package intercept

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// SpecReturnOverride matches declared FieldEquals return cases against inline tables.
// Type-based matching for variable arguments lives in transform.ApplySpecReturnCases.
type SpecReturnOverride struct {
	Phase api.Phase
}

// Override computes a spec return type override for a call.
// Returns nil if no override applies.
func (s *SpecReturnOverride) Override(fnType typ.Type, args []ast.Expr) typ.Type {
	if fnType == nil {
		return nil
	}
	if s.Phase != api.PhaseScopeCompute && s.Phase != api.PhaseNarrowing {
		return nil
	}

	fn := ResolveSpecFunction(fnType)
	if fn == nil || fn.Spec == nil {
		return nil
	}

	spec, ok := fn.Spec.(*contract.Spec)
	if !ok || spec == nil {
		return nil
	}

	return transform.ReturnTypeFromSpec(spec, args)
}

// ApplyOverride applies a spec return override to call result types.
// If override is non-nil, replaces the first return type.
func ApplyOverride(types []typ.Type, override typ.Type) []typ.Type {
	if override == nil || len(types) == 0 {
		return types
	}

	result := make([]typ.Type, len(types))
	copy(result, types)
	result[0] = override
	return result
}

// ResolveSpecFunction extracts the function type from a potentially wrapped type.
// Handles aliases, generics, and instantiated types.
func ResolveSpecFunction(t typ.Type) *typ.Function {
	if t == nil {
		return nil
	}

	t = unwrap.Alias(t)

	if g, ok := t.(*typ.Generic); ok {
		t = g.Body
	}

	if inst, ok := t.(*typ.Instantiated); ok {
		resolved, err := core.ResolveInstantiated(inst)
		if err == nil && resolved != nil {
			t = resolved
		}
	}

	return unwrap.Function(t)
}

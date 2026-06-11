package gradual

import (
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SoftPolicy controls how soft-placeholder detection behaves.
type SoftPolicy struct {
	// AllowEmptyRecord treats {} as a soft placeholder when true.
	AllowEmptyRecord bool
}

// SoftAnnotationPolicy treats empty records as non-soft (annotation semantics).
var SoftAnnotationPolicy = SoftPolicy{}

// SoftPlaceholderPolicy treats empty records as soft (placeholder semantics).
var SoftPlaceholderPolicy = SoftPolicy{AllowEmptyRecord: true}

// IsSoft reports whether a type should be treated as a soft placeholder under policy.
func IsSoft(t typ.Type, policy SoftPolicy) bool {
	state := getSoftPruneState()
	defer putSoftPruneState(state)
	return isSoftMemo(t, typ.NewGuard(), policy, state.softMemo)
}

func isSoftMemo(t typ.Type, guard recursion.Guard, policy SoftPolicy, memo map[typ.Type]bool) bool {
	if t == nil {
		return false
	}
	if cached, ok := memo[t]; ok {
		return cached
	}
	node := unwrapTransparentSoft(t)
	if node != t {
		if cached, ok := memo[node]; ok {
			memo[t] = cached
			return cached
		}
	}

	soft := isSoftNode(node, guard, policy, memo)
	memo[t] = soft
	if node != t {
		memo[node] = soft
	}
	return soft
}

func isSoftNode(t typ.Type, guard recursion.Guard, policy SoftPolicy, memo map[typ.Type]bool) bool {
	if t == nil {
		return false
	}
	node := unwrapTransparentSoft(t)
	switch node.(type) {
	case *typ.Alias, *typ.Optional, *typ.Array, *typ.Map, *typ.ReadonlyMap, *typ.Record, *typ.Union:
		// recurse below
	default:
		return node.Kind().IsPlaceholder()
	}

	next, ok := guard.Enter(node)
	if !ok {
		return false
	}

	switch tt := node.(type) {
	case *typ.Alias:
		return isSoftMemo(tt.Target, next, policy, memo)
	case *typ.Optional:
		return isSoftMemo(tt.Inner, next, policy, memo)
	case *typ.Array:
		return isSoftMemo(tt.Element, next, policy, memo)
	case *typ.Map:
		return isSoftMemo(tt.Key, next, policy, memo) && isSoftMemo(tt.Value, next, policy, memo)
	case *typ.ReadonlyMap:
		return isSoftMemo(tt.Key, next, policy, memo) && isSoftMemo(tt.Value, next, policy, memo)
	case *typ.Record:
		if tt.Open && len(tt.Fields) == 0 && !tt.HasMapComponent() {
			return true
		}
		if len(tt.Fields) == 0 && !tt.HasMapComponent() {
			return policy.AllowEmptyRecord
		}
		if tt.HasMapComponent() && len(tt.Fields) == 0 {
			return isSoftMemo(tt.MapKey, next, policy, memo) && isSoftMemo(tt.MapValue, next, policy, memo)
		}
		return false
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, m := range tt.Members {
			if !isSoftMemo(m, next, policy, memo) {
				return false
			}
		}
		return true
	}
	return false
}

func unwrapTransparentSoft(t typ.Type) typ.Type {
	for {
		ann, ok := t.(*typ.Annotated)
		if !ok {
			return t
		}
		if ann.Inner == nil || ann.Inner == t {
			return t
		}
		t = ann.Inner
	}
}

func isSoftWithMemo(t typ.Type, policy SoftPolicy, memo map[typ.Type]bool) bool {
	return isSoftMemo(t, typ.NewGuard(), policy, memo)
}

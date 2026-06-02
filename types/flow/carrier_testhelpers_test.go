package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// liftFlowValues builds the AbstractValue abstract-state carrier from a typ.Type
// map literal, mirroring the admission boundary (setValue's FromType lift) so
// white-box tests can seed the stable values store with concrete types.
func liftFlowValues(in map[string]typ.Type) pathValueMap {
	out := make(pathValueMap, len(in))
	for key, t := range in {
		out[constraint.PathKey(key)] = liftFlowValue(t)
	}
	return out
}

// liftPointFlowValues builds the point-sensitive mutable-state carrier from a
// typ.Type map literal.
func liftPointFlowValues(in map[cfg.Point]map[string]typ.Type) map[cfg.Point]pathValueMap {
	out := make(map[cfg.Point]pathValueMap, len(in))
	for p, m := range in {
		out[p] = liftFlowValues(m)
	}
	return out
}

// storedFlowType projects the stored carrier for key back to a typ.Type at the
// egress boundary, returning nil when the key is absent.
func storedFlowType(s *Solution, key string) typ.Type {
	av, ok := s.values[constraint.PathKey(key)]
	if !ok {
		return nil
	}
	return av.ProjectValue()
}

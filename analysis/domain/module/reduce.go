package module

import (
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// BeginInit applies Module's half of one cache-entry-derived ModuleInit
// generation. It consumes only the predecessor Cold alternative and writes
// the exact Recent pending generation, preserving every unrelated may
// alternative. The sibling Suspension initiation patch is deliberately not an
// input to this reduction.
func (schema Schema) BeginInit(current Value, key Key, generation linkmodule.ModuleInitGeneration) (Value, bool) {
	owner, support, ok := key.support()
	if !ok || owner != schema.owner || !schema.owns(current) || !schema.Admits(key, current) || current.top || !current.cold {
		return Value{}, false
	}
	_, predecessor, _, _, generationOK := owner.source.Module().Generations().Entry(generation)
	if !generationOK || predecessor != support.coordinate || !containsGeneration(owner.source, support.pending, generation) {
		return Value{}, false
	}
	pending, pendingOK := normalizedPending(owner.source, append(append([]pendingSite(nil), current.pending...), pendingSite{site: generation, role: materialization.Recent}))
	if !pendingOK {
		return Value{}, false
	}
	return Value{owner: owner, pending: pending, ready: append([]readySite(nil), current.ready...)}, true
}

// RestoreCold applies the Throw or matching Cancel outcome of one already
// committed pending generation.  Liveness/consumption belongs to Suspension;
// the caller must establish that premise before invoking this cache-only
// reduction.  A stale generation cannot restore Cold because it is not an
// exact current pending alternative.
func (schema Schema) RestoreCold(current Value, key Key, generation linkmodule.ModuleInitGeneration, role materialization.Role) (Value, bool) {
	owner, _, ok := key.support()
	if !ok || owner != schema.owner || !schema.owns(current) || !schema.Admits(key, current) || current.top || !pendingRoleValid(role) || !containsPending(current.pending, generation, role) {
		return Value{}, false
	}
	pending := make([]pendingSite, 0, len(current.pending)-1)
	removed := false
	for _, candidate := range current.pending {
		if !removed && candidate.site == generation && candidate.role == role {
			removed = true
			continue
		}
		pending = append(pending, candidate)
	}
	if !removed {
		return Value{}, false
	}
	return Value{owner: owner, cold: true, pending: pending, ready: append([]readySite(nil), current.ready...)}, true
}

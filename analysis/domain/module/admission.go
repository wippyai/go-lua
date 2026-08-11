package module

// Admits reports whether value inhabits the one exact sealed cache
// coordinate. Pending retains only a compact external Suspension reference;
// this check validates that reference's structural support, not lifecycle
// liveness or consumption. Key-local Pending and Ready support remains solely
// Schema authority; foreign values and coordinates fail closed.
func (schema Schema) Admits(key Key, value Value) bool {
	owner, support, keyOK := key.support()
	if !keyOK || owner != schema.owner || !schema.owns(value) {
		return false
	}
	if value.top {
		return true
	}
	for _, pending := range value.pending {
		if !pendingRoleValid(pending.role) || !containsGeneration(schema.owner.source, support.pending, pending.site) {
			return false
		}
	}
	for _, ready := range value.ready {
		if !containsReadyForSite(schema.owner.source, support.ready, ready.site, ready.subject) {
			return false
		}
	}
	return true
}

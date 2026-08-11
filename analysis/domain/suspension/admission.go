package suspension

// Admits reports whether value inhabits the exact sealed continuation
// occurrence. The Value relation stays keyless; only its retained subjects
// require key-local support validation.
func (schema Schema) Admits(key Key, value Value) bool {
	owner, support, keyOK := key.support()
	if !keyOK || owner != schema.owner || !schema.owns(value) {
		return false
	}
	if value.top {
		return true
	}
	for _, lifecycle := range value.lifecycles {
		if !lifecycle.role.Valid() {
			return false
		}
		for _, retention := range lifecycle.retained {
			if !schema.owner.containsAtom(support.retained, retainedAtom{kind: retention.subject.kind, value: retention.subject.value}) {
				return false
			}
		}
	}
	return true
}

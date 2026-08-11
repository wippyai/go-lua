package suspension

import "github.com/wippyai/go-lua/analysis/domain/materialization"

// ConsumeLive is Suspension's lifecycle-only reduction for one exact
// generation age.  It turns live alternatives at that age into consumed
// alternatives, retaining the exact private/shared subject classification.
// No cache, heap, or resumer state is accepted here; those are explicit
// inputs of their owning two-input boundary Rules.
func (schema Schema) ConsumeLive(current Value, key Key, role materialization.Role) (Value, bool) {
	owner, _, ok := key.support()
	if !ok || owner != schema.owner || !schema.owns(current) || !schema.Admits(key, current) || current.top || !role.Valid() {
		return Value{}, false
	}
	lifecycles := cloneLifecycles(current.lifecycles)
	changed := false
	for index := range lifecycles {
		if lifecycles[index].role != role || !lifecycles[index].live {
			continue
		}
		lifecycles[index].live = false
		lifecycles[index].consumed = true
		changed = true
	}
	if !changed {
		return Value{}, false
	}
	return schema.normalize(lifecycles), true
}

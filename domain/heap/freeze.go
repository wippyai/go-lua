package heap

import "github.com/wippyai/go-lua/domain/materialization"

// ShallowFreezeBranches is the normal-successor image of one exact
// table.freeze transition. A missing normal branch means that every admitted
// predecessor world contains zero objects at the selected allocation root;
// callers must preserve that distinction from an invalid transition.
//
// The transition is deliberately root-local. Contained objects are retained
// unchanged and must be proved frozen independently by Placement's recursive
// Heap-graph closure.
type ShallowFreezeBranches struct {
	owner  *schema
	root   uint32
	normal Value
	has    bool
}

func (branches ShallowFreezeBranches) valid() bool {
	if branches.owner == nil {
		return false
	}
	root, rootOK := branches.owner.rootAt(branches.root)
	if !rootOK || root.kind != RootAllocation {
		return false
	}
	if !branches.has {
		return branches.normal.owner == nil
	}
	return branches.normal.valid() && branches.normal.owner == branches.owner
}

// Normal returns the normal successor only for the exact allocation key the
// transition consumed. Keeping the coordinate proof on the branch prevents a
// homogeneous Heap Value from being replayed at another compatible root.
func (branches ShallowFreezeBranches) Normal(key Key) (Value, bool) {
	if !branches.valid() || !branches.has || !key.valid() || key.owner != branches.owner || key.slot != branches.root {
		return Value{}, false
	}
	return branches.normal, true
}

// ShallowFreeze maps the selected allocation's exact Recent object to
// FrozenFrozen while preserving its complete shape, metatable, partition and
// containment state. Summary objects are not changed: a Summary reference is
// not one exact runtime object and therefore cannot authorize this strong
// transition.
//
// Top remains a conservative Top normal successor; it never becomes frozen
// evidence. Zero worlds have no normal successor. The operation is immutable,
// idempotent, and distributes over Heap's finite world union.
func (schema Schema) ShallowFreeze(value Value, reference Reference) (ShallowFreezeBranches, bool) {
	key, role, referenceOK := reference.Key()
	if !schema.valid() || !schema.owns(value) || !referenceOK || reference.owner != schema.owner || role != materialization.Recent || !key.valid() || key.owner != schema.owner || key.Kind() != RootAllocation || !schema.Admits(key, value) {
		return ShallowFreezeBranches{}, false
	}
	if value.top {
		branches := ShallowFreezeBranches{owner: schema.owner, root: key.slot, normal: value, has: true}
		return branches, branches.valid()
	}
	hasNormal, changed := false, false
	for _, world := range value.worlds {
		switch world.kind {
		case WorldZero:
			changed = true
		case WorldOne, WorldMany:
			hasNormal = true
			if !world.recent.valid() || world.recent.owner != schema.owner {
				return ShallowFreezeBranches{}, false
			}
			if world.recent.frozen != FrozenFrozen {
				changed = true
			}
		default:
			return ShallowFreezeBranches{}, false
		}
	}
	if !hasNormal {
		branches := ShallowFreezeBranches{owner: schema.owner, root: key.slot}
		return branches, branches.valid()
	}
	if !changed {
		branches := ShallowFreezeBranches{owner: schema.owner, root: key.slot, normal: value, has: true}
		return branches, branches.valid()
	}
	worlds := make([]World, 0, len(value.worlds))
	for _, world := range value.worlds {
		if world.kind == WorldZero {
			continue
		}
		recent := world.recent
		recent.frozen = FrozenFrozen
		next, ok := rawWorldWithObject(world, materialization.Recent, recent)
		if !ok {
			return ShallowFreezeBranches{}, false
		}
		worlds = append(worlds, next)
	}
	normal, ok := schema.Relation(key, worlds...)
	if !ok {
		return ShallowFreezeBranches{}, false
	}
	branches := ShallowFreezeBranches{owner: schema.owner, root: key.slot, normal: normal, has: true}
	return branches, branches.valid()
}

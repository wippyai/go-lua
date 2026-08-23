package heap

import "github.com/wippyai/go-lua/analysis/identity"

// allocationFormDirectoryCount is the extent of the sealed per-form
// directories. It is AllocationForm's own extent, so a new constructor form
// gets its directory by declaring the form and nothing else.
const allocationFormDirectoryCount = int(AllocationFormFinalOpen) + 1

// sealAllocationFormDirectory publishes one dense global ordinal per Program
// allocation root, grouped by the constructor form the root was sealed with.
//
// The directory is Link-global on purpose. An allocation occurrence is
// addressed by its mount and its occurrence identity, but the ordinal it
// resolves to is a coordinate of the whole sealed Heap: two mounts of the same
// module occupy distinct ordinals, and every reader of this schema reads the
// same ordering. A per-mount directory cannot express that, because it has no
// row for the mount it is not.
//
// Fresh Target roots are absent by construction: they carry no sealed Program
// constructor form, so they belong to no form directory.
func (owner *schema) sealAllocationFormDirectory() bool {
	if owner == nil || owner.allocationFormOrdinal != nil {
		return false
	}
	count := int(owner.programRootCount)
	if count < 0 || count > len(owner.roots) {
		return false
	}
	ordinals := make([]uint32, count)
	for index := 0; index < count; index++ {
		row := owner.roots[index]
		if row.kind != RootAllocation || !row.allocation.form.Valid() {
			continue
		}
		form := int(row.allocation.form)
		if form <= 0 || form >= allocationFormDirectoryCount {
			return false
		}
		if len(owner.allocationFormRoots[form]) >= int(^uint32(0)) {
			return false
		}
		owner.allocationFormRoots[form] = append(owner.allocationFormRoots[form], uint32(index+1))
		ordinals[index] = uint32(len(owner.allocationFormRoots[form]))
	}
	owner.allocationFormOrdinal = ordinals
	return true
}

func (schema Schema) formDirectory(form AllocationForm) ([]uint32, bool) {
	if !schema.valid() || !form.Valid() || int(form) >= allocationFormDirectoryCount || schema.owner.allocationFormOrdinal == nil {
		return nil, false
	}
	return schema.owner.allocationFormRoots[form], true
}

func (schema Schema) formCount(form AllocationForm) int {
	directory, ok := schema.formDirectory(form)
	if !ok {
		return 0
	}
	return len(directory)
}

func (schema Schema) formAt(form AllocationForm, index int) (Key, bool) {
	directory, ok := schema.formDirectory(form)
	if !ok || index < 0 || index >= len(directory) {
		return Key{}, false
	}
	return Key{owner: schema.owner, slot: directory[index]}, true
}

func (schema Schema) formOrdinal(form AllocationForm, key Key) (uint32, bool) {
	directory, directoryOK := schema.formDirectory(form)
	if !directoryOK || !schema.OwnsKey(key) || key.Kind() != RootAllocation || key.slot == 0 ||
		int(key.slot) > len(schema.owner.allocationFormOrdinal) {
		return 0, false
	}
	ordinal := schema.owner.allocationFormOrdinal[key.slot-1]
	if ordinal == 0 || int(ordinal) > len(directory) || directory[ordinal-1] != key.slot {
		return 0, false
	}
	return ordinal - 1, true
}

func (schema Schema) formForMountedOccurrence(form AllocationForm, module, occurrence identity.ContentID) (Key, bool) {
	key, keyOK := schema.AllocationRootForMountedOccurrence(module, occurrence)
	if !keyOK {
		return Key{}, false
	}
	if _, ok := schema.formOrdinal(form, key); !ok {
		return Key{}, false
	}
	return key, true
}

// ClosedAllocationCount is the dense census of scalar closed table
// constructors this Link seals.
func (schema Schema) ClosedAllocationCount() int {
	return schema.formCount(AllocationFormClosed)
}

// ClosedAllocationAt indexes that directory in sealed Key order.
func (schema Schema) ClosedAllocationAt(index int) (Key, bool) {
	return schema.formAt(AllocationFormClosed, index)
}

// ClosedAllocationOrdinal is the exact inverse of ClosedAllocationAt.
func (schema Schema) ClosedAllocationOrdinal(key Key) (uint32, bool) {
	return schema.formOrdinal(AllocationFormClosed, key)
}

// ClosedAllocationForMountedOccurrence resolves one mounted Program
// allocation occurrence to its closed constructor root, refusing an
// occurrence whose sealed form is not closed.
func (schema Schema) ClosedAllocationForMountedOccurrence(module, occurrence identity.ContentID) (Key, bool) {
	return schema.formForMountedOccurrence(AllocationFormClosed, module, occurrence)
}

// EmptyAllocationCount is the dense census of empty constructors this Link
// seals.
func (schema Schema) EmptyAllocationCount() int {
	return schema.formCount(AllocationFormEmpty)
}

// EmptyAllocationAt indexes that directory in sealed Key order.
func (schema Schema) EmptyAllocationAt(index int) (Key, bool) {
	return schema.formAt(AllocationFormEmpty, index)
}

// EmptyAllocationOrdinal is the exact inverse of EmptyAllocationAt.
func (schema Schema) EmptyAllocationOrdinal(key Key) (uint32, bool) {
	return schema.formOrdinal(AllocationFormEmpty, key)
}

// EmptyAllocationForMountedOccurrence resolves one mounted Program allocation
// occurrence to its empty constructor root, refusing an occurrence whose
// sealed form is not empty.
func (schema Schema) EmptyAllocationForMountedOccurrence(module, occurrence identity.ContentID) (Key, bool) {
	return schema.formForMountedOccurrence(AllocationFormEmpty, module, occurrence)
}

// ClosedAllocation is the destination projection of one closed-allocation
// candidate: the constructor writes the very root it is a candidate of. The
// projection refuses any other root, so the coordinate cannot be read off a
// key that this relation does not contain.
func (key Key) ClosedAllocation() (Key, bool) {
	return key.allocationOfForm(AllocationFormClosed)
}

// EmptyAllocation is the same destination projection for the empty
// constructor directory.
func (key Key) EmptyAllocation() (Key, bool) {
	return key.allocationOfForm(AllocationFormEmpty)
}

func (key Key) allocationOfForm(form AllocationForm) (Key, bool) {
	if !key.valid() || key.Kind() != RootAllocation {
		return Key{}, false
	}
	if _, ok := (Schema{owner: key.owner}).formOrdinal(form, key); !ok {
		return Key{}, false
	}
	return key, true
}

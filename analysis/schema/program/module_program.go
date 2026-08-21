package programschema

func (row Program) ModuleImportCount() (int, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return 0, false
	}
	return ModuleImportFamily().Count(&row.Frozen, catalog)
}
func (row Program) ModuleImportAt(index int) (ModuleImport, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return ModuleImport{}, false
	}
	return ModuleImportFamily().At(&row.Frozen, catalog, index)
}
func (row Program) ModuleRequestCount() (int, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return 0, false
	}
	return ModuleRequestFamily().Count(&row.Frozen, catalog)
}
func (row Program) ModuleRequestAt(index int) (ModuleRequest, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return ModuleRequest{}, false
	}
	return ModuleRequestFamily().At(&row.Frozen, catalog, index)
}
func (row Program) ModuleEntryCount() (int, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return 0, false
	}
	return ModuleEntryFamily().Count(&row.Frozen, catalog)
}
func (row Program) ModuleEntryAt(index int) (ModuleEntry, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return ModuleEntry{}, false
	}
	return ModuleEntryFamily().At(&row.Frozen, catalog, index)
}
func (row Program) ModuleEntryRootCellAt(index int) (ModuleEntryRootCell, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return ModuleEntryRootCell{}, false
	}
	return ModuleEntryRootCellFamily().At(&row.Frozen, catalog, index)
}
func (row Program) ModuleEntryRootFunctionAt(index int) (ModuleEntryRootFunction, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return ModuleEntryRootFunction{}, false
	}
	return ModuleEntryRootFunctionFamily().At(&row.Frozen, catalog, index)
}
func (row Program) ModuleEntryMemberAt(index int) (ModuleEntryMember, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return ModuleEntryMember{}, false
	}
	return ModuleEntryMemberFamily().At(&row.Frozen, catalog, index)
}

func (row Program) ModuleRequestFor(importIndex int) (ModuleRequest, bool) {
	parent, ok := row.ModuleImportAt(importIndex)
	if !ok {
		return ModuleRequest{}, false
	}
	offset, count, spanOK := parent.RequestSpan()
	if !spanOK || count != 1 {
		return ModuleRequest{}, false
	}
	request, requestOK := row.ModuleRequestAt(int(offset))
	return request, requestOK && request.ImportID() == parent.ID()
}

func (row Program) ModuleEntryForReturnOrdinal(returnOrdinal uint32) (ModuleEntry, bool) {
	count, ok := row.ModuleEntryCount()
	if !ok || returnOrdinal == 0 {
		return ModuleEntry{}, false
	}
	var previous uint32
	for index := 0; index < count; index++ {
		entry, held := row.ModuleEntryAt(index)
		ordinal, ordinalOK := entry.ReturnOrdinal()
		if !held || !ordinalOK || ordinal <= previous {
			return ModuleEntry{}, false
		}
		if ordinal == returnOrdinal {
			return entry, true
		}
		if ordinal > returnOrdinal {
			return ModuleEntry{}, false
		}
		previous = ordinal
	}
	return ModuleEntry{}, false
}

func (row Program) ModuleEntryRootCellFor(entryIndex, position int) (ModuleEntryRootCell, bool) {
	entry, ok := row.ModuleEntryAt(entryIndex)
	rootWidth, widthOK := entry.RootWidth()
	if !ok || !widthOK || position < 0 || uint64(position) >= uint64(rootWidth) {
		return ModuleEntryRootCell{}, false
	}
	offset, count, spanOK := entry.RootCellSpan()
	if !spanOK {
		return ModuleEntryRootCell{}, false
	}
	var result ModuleEntryRootCell
	var previous uint32
	for childIndex := uint32(0); childIndex < count; childIndex++ {
		child, childOK := row.ModuleEntryRootCellAt(int(offset + childIndex))
		childPosition := child.Position()
		if !childOK || child.EntryID() != entry.ID() || childPosition >= rootWidth || childIndex != 0 && childPosition <= previous {
			return ModuleEntryRootCell{}, false
		}
		if childPosition == uint32(position) {
			result = child
		}
		previous = childPosition
	}
	return result, result.Available()
}
func (row Program) ModuleEntryRootFunctionFor(entryIndex, position int) (ModuleEntryRootFunction, bool) {
	entry, ok := row.ModuleEntryAt(entryIndex)
	rootWidth, widthOK := entry.RootWidth()
	if !ok || !widthOK || position < 0 || uint64(position) >= uint64(rootWidth) {
		return ModuleEntryRootFunction{}, false
	}
	offset, count, spanOK := entry.RootFunctionSpan()
	if !spanOK {
		return ModuleEntryRootFunction{}, false
	}
	var result ModuleEntryRootFunction
	var previous uint32
	for childIndex := uint32(0); childIndex < count; childIndex++ {
		child, childOK := row.ModuleEntryRootFunctionAt(int(offset + childIndex))
		childPosition := child.Position()
		if !childOK || child.EntryID() != entry.ID() || childPosition >= rootWidth || childIndex != 0 && childPosition <= previous {
			return ModuleEntryRootFunction{}, false
		}
		if childPosition == uint32(position) {
			result = child
		}
		previous = childPosition
	}
	return result, result.Available()
}
func (row Program) ModuleEntryMemberFor(entryIndex, childIndex int) (ModuleEntryMember, bool) {
	entry, ok := row.ModuleEntryAt(entryIndex)
	if !ok || childIndex < 0 {
		return ModuleEntryMember{}, false
	}
	offset, count, spanOK := entry.MemberSpan()
	if !spanOK || uint64(childIndex) >= uint64(count) {
		return ModuleEntryMember{}, false
	}
	child, childOK := row.ModuleEntryMemberAt(int(offset) + childIndex)
	rootWidth, widthOK := entry.RootWidth()
	return child, childOK && widthOK && child.EntryID() == entry.ID() && child.Position() < rootWidth
}

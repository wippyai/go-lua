package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// ModuleRowsLawVersion is part of the pinned Artifact identity preimage. The
// Program publication owns the module rows and their child planes, so the
// version travels with the schema identity writer rather than with a consumer
// or a compiler-local projection.
const ModuleRowsLawVersion uint64 = 1

// WriteModuleIdentityFields replays the canonical Module portion of the
// Artifact identity preimage from the sealed Program publication. Span
// offsets are storage layout and are intentionally omitted; span widths,
// original positions, row identities, parent identities, and every optional
// presence bit are semantic and are committed here.
//
// The writer validates the publication's dense geometry while replaying it.
// Seal validation owns the cross-family joins and publication gate; this
// method nevertheless fails closed for an unavailable row or an out-of-range
// span so an invalid publication can never acquire an identity.
func (row Program) WriteModuleIdentityFields(writer identity.IdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	importCount, importsPublished := ModuleImportFamily().Count(&row.Frozen, catalog)
	requestCount, requestsPublished := ModuleRequestFamily().Count(&row.Frozen, catalog)
	entryCount, entriesPublished := ModuleEntryFamily().Count(&row.Frozen, catalog)
	rootCellCount, rootCellsPublished := ModuleEntryRootCellFamily().Count(&row.Frozen, catalog)
	rootFunctionCount, rootFunctionsPublished := ModuleEntryRootFunctionFamily().Count(&row.Frozen, catalog)
	memberCount, membersPublished := ModuleEntryMemberFamily().Count(&row.Frozen, catalog)
	if !importsPublished || !requestsPublished || !entriesPublished || !rootCellsPublished || !rootFunctionsPublished || !membersPublished {
		return false
	}
	if !writer.WriteUint(ModuleRowsLawVersion) ||
		!writer.WriteUint(uint64(importCount)) || !writer.WriteUint(uint64(requestCount)) ||
		!writer.WriteUint(uint64(entryCount)) || !writer.WriteUint(uint64(rootCellCount)) ||
		!writer.WriteUint(uint64(memberCount)) || !writer.WriteUint(uint64(rootFunctionCount)) {
		return false
	}

	requestCursor := uint32(0)
	seenImports := make(map[identity.ContentID]struct{}, importCount)
	seenRequests := make(map[identity.ContentID]struct{}, requestCount)
	for index := 0; index < importCount; index++ {
		importRow, held := ModuleImportFamily().At(&row.Frozen, catalog, index)
		offset, width, spanOK := importRow.RequestSpan()
		alias, hasAlias := importRow.AliasID()
		if !held || !importRow.Available() || !spanOK || width != 1 || offset != requestCursor ||
			uint64(offset)+uint64(width) > uint64(requestCount) {
			return false
		}
		if _, duplicate := seenImports[importRow.ID()]; duplicate {
			return false
		}
		seenImports[importRow.ID()] = struct{}{}
		if !writer.WriteContentID(importRow.ID()) || !writer.WriteContentID(importRow.CallID()) ||
			!writer.WriteBool(hasAlias) || !writer.WriteContentID(alias) || !writer.WriteUint(uint64(width)) {
			return false
		}
		request, requestHeld := ModuleRequestFamily().At(&row.Frozen, catalog, int(offset))
		if !requestHeld || !request.Available() || request.ImportID() != importRow.ID() {
			return false
		}
		if _, duplicate := seenRequests[request.ID()]; duplicate {
			return false
		}
		seenRequests[request.ID()] = struct{}{}
		if !writer.WriteContentID(request.ID()) || !writer.WriteContentID(request.ImportID()) ||
			!writer.WriteContentID(request.ValueID()) || !writer.WriteUint(uint64(request.Key())) {
			return false
		}
		requestCursor += width
	}
	if requestCursor != uint32(requestCount) {
		return false
	}

	rootCellCursor, rootFunctionCursor, memberCursor := uint32(0), uint32(0), uint32(0)
	seenEntries := make(map[identity.ContentID]struct{}, entryCount)
	seenRootCellIDs := make(map[identity.ContentID]struct{}, rootCellCount)
	seenRootFunctionIDs := make(map[identity.ContentID]struct{}, rootFunctionCount)
	seenMemberIDs := make(map[identity.ContentID]struct{}, memberCount)
	var previousReturn uint32
	for index := 0; index < entryCount; index++ {
		entry, held := ModuleEntryFamily().At(&row.Frozen, catalog, index)
		returnOrdinal, returnOrdinalOK := entry.ReturnOrdinal()
		rootWidth, rootWidthOK := entry.RootWidth()
		rootCellOffset, rootCells, rootCellsOK := entry.RootCellSpan()
		rootFunctionOffset, rootFunctions, rootFunctionsOK := entry.RootFunctionSpan()
		memberOffset, members, membersOK := entry.MemberSpan()
		if !held || !entry.Available() || !returnOrdinalOK || !rootWidthOK || !rootCellsOK || !rootFunctionsOK || !membersOK ||
			rootCellOffset != rootCellCursor || rootFunctionOffset != rootFunctionCursor || memberOffset != memberCursor ||
			returnOrdinal <= previousReturn ||
			uint64(rootCellOffset)+uint64(rootCells) > uint64(rootCellCount) ||
			uint64(rootFunctionOffset)+uint64(rootFunctions) > uint64(rootFunctionCount) ||
			uint64(memberOffset)+uint64(members) > uint64(memberCount) {
			return false
		}
		if _, duplicate := seenEntries[entry.ID()]; duplicate {
			return false
		}
		seenEntries[entry.ID()] = struct{}{}
		if !writer.WriteContentID(entry.ID()) || !writer.WriteContentID(entry.ReturnID()) ||
			!writer.WriteUint(uint64(returnOrdinal)) || !writer.WriteUint(uint64(rootWidth)) ||
			!writer.WriteUint(uint64(rootCells)) || !writer.WriteUint(uint64(rootFunctions)) ||
			!writer.WriteUint(uint64(members)) {
			return false
		}

		var previousCellPosition uint32
		cellPositionKnown := false
		for position := uint32(0); position < rootCells; position++ {
			child, childHeld := ModuleEntryRootCellFamily().At(&row.Frozen, catalog, int(rootCellOffset+position))
			original := child.Position()
			if !childHeld || !child.Available() || child.EntryID() != entry.ID() || original >= rootWidth {
				return false
			}
			if cellPositionKnown && original <= previousCellPosition {
				return false
			}
			cellPositionKnown, previousCellPosition = true, original
			if _, duplicate := seenRootCellIDs[child.ID()]; duplicate {
				return false
			}
			seenRootCellIDs[child.ID()] = struct{}{}
			if !writer.WriteContentID(child.ID()) || !writer.WriteContentID(child.EntryID()) ||
				!writer.WriteContentID(child.CellID()) || !writer.WriteUint(uint64(original)) {
				return false
			}
		}
		var previousMemberPosition uint32
		memberPositionKnown := false
		memberFields := make(map[identity.ContentID]struct{}, members)
		for position := uint32(0); position < members; position++ {
			member, memberHeld := ModuleEntryMemberFamily().At(&row.Frozen, catalog, int(memberOffset+position))
			value, hasValue := member.ValueID()
			original := member.Position()
			if !memberHeld || !member.Available() || member.EntryID() != entry.ID() || original >= rootWidth {
				return false
			}
			if memberPositionKnown && original < previousMemberPosition {
				return false
			}
			memberPositionKnown, previousMemberPosition = true, original
			if _, duplicate := seenMemberIDs[member.ID()]; duplicate {
				return false
			}
			seenMemberIDs[member.ID()] = struct{}{}
			if member.ParentID() != member.TableID() {
				if _, earlier := memberFields[member.ParentID()]; !earlier {
					return false
				}
			}
			memberFields[member.FieldID()] = struct{}{}
			if !writer.WriteContentID(member.ID()) || !writer.WriteContentID(member.FieldID()) ||
				!writer.WriteContentID(member.ParentID()) || !writer.WriteBool(hasValue) ||
				!writer.WriteContentID(value) || !writer.WriteContentID(member.EntryID()) ||
				!writer.WriteContentID(member.TableID()) || !writer.WriteUint(uint64(member.Suffix())) ||
				!writer.WriteUint(uint64(original)) {
				return false
			}
		}
		var previousFunctionPosition uint32
		functionPositionKnown := false
		for position := uint32(0); position < rootFunctions; position++ {
			child, childHeld := ModuleEntryRootFunctionFamily().At(&row.Frozen, catalog, int(rootFunctionOffset+position))
			original := child.Position()
			if !childHeld || !child.Available() || child.EntryID() != entry.ID() || original >= rootWidth {
				return false
			}
			if functionPositionKnown && original <= previousFunctionPosition {
				return false
			}
			functionPositionKnown, previousFunctionPosition = true, original
			if _, duplicate := seenRootFunctionIDs[child.ID()]; duplicate {
				return false
			}
			seenRootFunctionIDs[child.ID()] = struct{}{}
			if !writer.WriteContentID(child.ID()) || !writer.WriteContentID(child.EntryID()) ||
				!writer.WriteContentID(child.FunctionID()) || !writer.WriteUint(uint64(original)) {
				return false
			}
		}
		previousReturn = returnOrdinal
		rootCellCursor = rootCellOffset + rootCells
		rootFunctionCursor = rootFunctionOffset + rootFunctions
		memberCursor = memberOffset + members
	}
	return rootCellCursor == uint32(rootCellCount) && rootFunctionCursor == uint32(rootFunctionCount) && memberCursor == uint32(memberCount)
}

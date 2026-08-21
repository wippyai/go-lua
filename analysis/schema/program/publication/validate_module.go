package publication

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
)

// validateSealModule authenticates the canonical Module planes as one dense
// publication. All span cursors are scratch locals: the sealed Artifact keeps
// the schema families only and does not retain a second module index.
func (validator *validator) validateSealModule(state *validationState) bool {
	if validator == nil || state == nil {
		return false
	}
	importCount, importsPublished := programschema.ModuleImportFamily().Count(&validator.frozen, validator.catalog)
	requestCount, requestsPublished := programschema.ModuleRequestFamily().Count(&validator.frozen, validator.catalog)
	entryCount, entriesPublished := programschema.ModuleEntryFamily().Count(&validator.frozen, validator.catalog)
	rootCellCount, rootCellsPublished := programschema.ModuleEntryRootCellFamily().Count(&validator.frozen, validator.catalog)
	rootFunctionCount, rootFunctionsPublished := programschema.ModuleEntryRootFunctionFamily().Count(&validator.frozen, validator.catalog)
	memberCount, membersPublished := programschema.ModuleEntryMemberFamily().Count(&validator.frozen, validator.catalog)
	if !importsPublished || !requestsPublished || !entriesPublished || !rootCellsPublished || !rootFunctionsPublished || !membersPublished {
		return false
	}

	// These maps are seal-time witnesses only. They authenticate joins into
	// already-owned Program families without becoming retained parallel state.
	functionIDs := make(map[identity.ContentID]struct{})
	functionCount, functionPublished := programschema.FunctionBoundaryFamily().Count(&validator.frozen, validator.catalog)
	if !functionPublished {
		return false
	}
	for index := 0; index < functionCount; index++ {
		function, held := programschema.FunctionBoundaryFamily().At(&validator.frozen, validator.catalog, index)
		if !held || !function.Available() {
			return false
		}
		functionIDs[function.ID()] = struct{}{}
	}
	allocationIDs := make(map[identity.ContentID]struct{})
	fieldIDs := make(map[identity.ContentID]struct{})
	allocationCount, allocationsPublished := heapallocation.AllocationFamily().Count(&validator.frozen, validator.catalog)
	fieldCount, fieldsPublished := heapallocation.FieldFamily().Count(&validator.frozen, validator.catalog)
	if !allocationsPublished || !fieldsPublished {
		return false
	}
	for index := 0; index < allocationCount; index++ {
		allocation, held := heapallocation.AllocationFamily().At(&validator.frozen, validator.catalog, index)
		if !held || !allocation.Available() {
			return false
		}
		allocationIDs[allocation.ID()] = struct{}{}
	}
	for index := 0; index < fieldCount; index++ {
		field, held := heapallocation.FieldFamily().At(&validator.frozen, validator.catalog, index)
		if !held || !field.Available() {
			return false
		}
		fieldIDs[field.ID()] = struct{}{}
	}
	cellIDs := make(map[identity.ContentID]struct{})
	lifecycleView, lifecycleOK := validator.lifecycle, validator.lifecycle.Available()
	if !lifecycleOK {
		return false
	}
	lifetimeCount, lifetimesPublished := lifecycleView.StorageCellLifetimeCount()
	if !lifetimesPublished {
		return false
	}
	for index := 0; index < lifetimeCount; index++ {
		lifetime, held := lifecycleView.StorageCellLifetimeAt(index)
		if !held || !lifetime.Available() {
			return false
		}
		cellIDs[lifetime.ID()] = struct{}{}
	}

	seenImports := make(map[identity.ContentID]struct{}, importCount)
	seenRequests := make(map[identity.ContentID]struct{}, requestCount)
	requestCursor := uint32(0)
	for index := 0; index < importCount; index++ {
		importRow, held := programschema.ModuleImportFamily().At(&validator.frozen, validator.catalog, index)
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
		if _, callKnown := state.callRows[importRow.CallID()]; !callKnown {
			return false
		}
		if hasAlias {
			if _, cellKnown := cellIDs[alias]; !cellKnown {
				return false
			}
		}
		request, requestHeld := programschema.ModuleRequestFamily().At(&validator.frozen, validator.catalog, int(offset))
		if !requestHeld || !request.Available() || request.ImportID() != importRow.ID() {
			return false
		}
		if _, duplicate := seenRequests[request.ID()]; duplicate {
			return false
		}
		seenRequests[request.ID()] = struct{}{}
		if sourceIDs := state.occurrenceRows[programschema.OccurrenceValueSource]; sourceIDs != nil {
			if _, sourceKnown := sourceIDs[request.ValueID()]; !sourceKnown {
				return false
			}
		} else {
			return false
		}
		requestCursor += width
	}
	if requestCursor != uint32(requestCount) {
		return false
	}

	seenEntries := make(map[identity.ContentID]struct{}, entryCount)
	seenRootCellIDs := make(map[identity.ContentID]struct{}, rootCellCount)
	seenRootFunctionIDs := make(map[identity.ContentID]struct{}, rootFunctionCount)
	seenMemberIDs := make(map[identity.ContentID]struct{}, memberCount)
	rootCellCursor, rootFunctionCursor, memberCursor := uint32(0), uint32(0), uint32(0)
	previousReturn := uint32(0)
	for index := 0; index < entryCount; index++ {
		entry, held := programschema.ModuleEntryFamily().At(&validator.frozen, validator.catalog, index)
		returnOrdinal, returnOrdinalOK := entry.ReturnOrdinal()
		rootWidth, rootWidthOK := entry.RootWidth()
		rootCellOffset, rootCells, rootCellsOK := entry.RootCellSpan()
		rootFunctionOffset, rootFunctions, rootFunctionsOK := entry.RootFunctionSpan()
		memberOffset, members, membersOK := entry.MemberSpan()
		if !held || !entry.Available() || !returnOrdinalOK || !rootWidthOK || !rootCellsOK || !rootFunctionsOK || !membersOK ||
			returnOrdinal <= previousReturn || rootCellOffset != rootCellCursor || rootFunctionOffset != rootFunctionCursor || memberOffset != memberCursor ||
			uint64(rootCellOffset)+uint64(rootCells) > uint64(rootCellCount) ||
			uint64(rootFunctionOffset)+uint64(rootFunctions) > uint64(rootFunctionCount) ||
			uint64(memberOffset)+uint64(members) > uint64(memberCount) {
			return false
		}
		if _, duplicate := seenEntries[entry.ID()]; duplicate {
			return false
		}
		seenEntries[entry.ID()] = struct{}{}
		// ModuleEntry.ReturnID is the canonical Program Outcome identity, not
		// the separate Return-boundary occurrence identity. Authenticate it
		// against the already-validated Outcome plane and retain only Returns.
		outcomeIndex, present := state.outcomeRows[entry.ReturnID()]
		outcome, outcomeHeld := programschema.OutcomeFamily().At(&validator.frozen, validator.catalog, outcomeIndex)
		if !present || !outcomeHeld || outcome.Kind() != programschema.OutcomeReturn {
			return false
		}
		var previousCellPosition uint32
		cellPositionKnown := false
		for position := uint32(0); position < rootCells; position++ {
			child, childHeld := programschema.ModuleEntryRootCellFamily().At(&validator.frozen, validator.catalog, int(rootCellOffset+position))
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
			if _, cellKnown := cellIDs[child.CellID()]; !cellKnown {
				return false
			}
		}
		memberFields := make(map[identity.ContentID]struct{}, members)
		var previousMemberPosition uint32
		memberPositionKnown := false
		for position := uint32(0); position < members; position++ {
			member, memberHeld := programschema.ModuleEntryMemberFamily().At(&validator.frozen, validator.catalog, int(memberOffset+position))
			original := member.Position()
			value, hasValue := member.ValueID()
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
			if _, fieldKnown := fieldIDs[member.FieldID()]; !fieldKnown {
				return false
			}
			if _, tableKnown := allocationIDs[member.TableID()]; !tableKnown {
				return false
			}
			if member.ParentID() == member.TableID() {
				// A first-level member is owned by the table root named by its
				// TableID. No earlier field is required in this case.
			} else if _, earlier := memberFields[member.ParentID()]; !earlier {
				return false
			}
			if hasValue {
				if _, functionKnown := functionIDs[value]; !functionKnown {
					return false
				}
			}
			memberFields[member.FieldID()] = struct{}{}
		}
		var previousFunctionPosition uint32
		functionPositionKnown := false
		for position := uint32(0); position < rootFunctions; position++ {
			child, childHeld := programschema.ModuleEntryRootFunctionFamily().At(&validator.frozen, validator.catalog, int(rootFunctionOffset+position))
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
			if _, functionKnown := functionIDs[child.FunctionID()]; !functionKnown {
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

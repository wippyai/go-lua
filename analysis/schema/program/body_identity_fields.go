package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
)

// FunctionBoundaryLawVersion is part of the pinned CompileKey and Artifact
// identity preimages. Version 4 adds the owner-issued inner and outer storage
// cell identities of every capture; version 3 committed only callable cells.
// The Program schema owns the version with the boundary rows it describes.
const FunctionBoundaryLawVersion uint64 = 4

// WriteBodyIdentityFields appends the historical Body, callable interface,
// call-target, and Outcome segment of the Artifact identity. It writes into
// the caller's existing identity stream; it does not create a second digest.
func (row Program) WriteBodyIdentityFields(writer identity.IdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	bodyCount, bodiesPublished := BodyFamily().Count(&row.Frozen, catalog)
	if !bodiesPublished || !writer.WriteUint(uint64(bodyCount)) {
		return false
	}
	for index := 0; index < bodyCount; index++ {
		body, held := BodyFamily().At(&row.Frozen, catalog, index)
		entryOffset, entryCount, entriesOK := body.EntrySpan()
		rootOffset, rootCount, rootsOK := body.RootSpan()
		outcomeOffset, outcomeCount, outcomesOK := body.OutcomeSpan()
		function, _ := body.FunctionContextID()
		formal, _ := body.CallFormalID()
		if !held || !entriesOK || !rootsOK || !outcomesOK ||
			!writer.WriteContentID(body.ID()) || !writer.WriteContentID(body.ContextID()) ||
			!writer.WriteContentID(body.EntryID()) || !writer.WriteBool(body.Callable()) ||
			!writer.WriteContentID(function) || !writer.WriteContentID(formal) || !writer.WriteUint(uint64(entryCount)) {
			return false
		}
		for position := uint32(0); position < entryCount; position++ {
			point, childHeld := BodyEntryFamily().At(&row.Frozen, catalog, int(entryOffset+position))
			if !childHeld || point.BodyID() != body.ID() || !writer.WriteContentID(point.PointID()) {
				return false
			}
		}
		if !writer.WriteUint(uint64(rootCount)) {
			return false
		}
		for position := uint32(0); position < rootCount; position++ {
			root, childHeld := BodyRootFamily().At(&row.Frozen, catalog, int(rootOffset+position))
			if !childHeld || root.BodyID() != body.ID() || !writer.WriteContentID(root.ID()) || !writer.WriteUint(uint64(root.Family())) {
				return false
			}
		}
		if !writer.WriteUint(uint64(outcomeOffset)) || !writer.WriteUint(uint64(outcomeOffset+outcomeCount)) {
			return false
		}
	}

	boundaryCount, boundariesPublished := FunctionBoundaryFamily().Count(&row.Frozen, catalog)
	formalCount, formalsPublished := FunctionFormalFamily().Count(&row.Frozen, catalog)
	varargCount, varargsPublished := FunctionVarargFamily().Count(&row.Frozen, catalog)
	captureCount, capturesPublished := FunctionCaptureFamily().Count(&row.Frozen, catalog)
	if !boundariesPublished || !formalsPublished || !varargsPublished || !capturesPublished ||
		!writer.WriteUint(FunctionBoundaryLawVersion) || !writer.WriteUint(uint64(boundaryCount)) {
		return false
	}
	for index := 0; index < boundaryCount; index++ {
		boundary, held := FunctionBoundaryFamily().At(&row.Frozen, catalog, index)
		formalOffset, formalWidth, formalSpanOK := boundary.FormalSpan()
		varargOffset, varargWidth, varargSpanOK := boundary.VarargSpan()
		captureOffset, captureWidth, captureSpanOK := boundary.CaptureSpan()
		if !held || !boundary.Available() || !formalSpanOK || !varargSpanOK || !captureSpanOK ||
			uint64(formalOffset)+uint64(formalWidth) > uint64(formalCount) ||
			uint64(varargOffset)+uint64(varargWidth) > uint64(varargCount) ||
			uint64(captureOffset)+uint64(captureWidth) > uint64(captureCount) ||
			!writer.WriteContentID(boundary.ID()) || !writer.WriteContentID(boundary.BodyID()) ||
			!writer.WriteContentID(boundary.BodyContextID()) || !writer.WriteContentID(boundary.EntryID()) ||
			!writer.WriteContentID(boundary.CallFormalID()) || !writer.WriteUint(uint64(formalWidth)) {
			return false
		}
		for position := uint32(0); position < formalWidth; position++ {
			port, portHeld := FunctionFormalFamily().At(&row.Frozen, catalog, int(formalOffset+position))
			declared, _ := port.DeclaredStaticTypeID()
			formalPosition, positionOK := port.Position()
			if !portHeld || !port.Available() || !positionOK ||
				!writer.WriteContentID(port.ID()) || !writer.WriteContentID(port.CellID()) ||
				!writer.WriteContentID(port.StorageCellID()) || !writer.WriteContentID(declared) ||
				!writer.WriteUint(uint64(formalPosition)) {
				return false
			}
		}
		varargID, varargCell := identity.ContentID{}, identity.ContentID{}
		if varargWidth == 1 {
			vararg, varargHeld := FunctionVarargFamily().At(&row.Frozen, catalog, int(varargOffset))
			if !varargHeld || !vararg.Available() {
				return false
			}
			varargID, varargCell = vararg.ID(), vararg.CellID()
		}
		if !writer.WriteBool(varargWidth == 1) || !writer.WriteContentID(varargID) ||
			!writer.WriteContentID(varargCell) || !writer.WriteUint(uint64(captureWidth)) {
			return false
		}
		for position := uint32(0); position < captureWidth; position++ {
			capture, captureHeld := FunctionCaptureFamily().At(&row.Frozen, catalog, int(captureOffset+position))
			capturePosition, positionOK := capture.Position()
			if !captureHeld || !capture.Available() || !positionOK ||
				!writer.WriteContentID(capture.ID()) || !writer.WriteContentID(capture.InnerCellID()) ||
				!writer.WriteContentID(capture.OuterCellID()) || !writer.WriteContentID(capture.InnerStorageCellID()) ||
				!writer.WriteContentID(capture.OuterStorageCellID()) || !writer.WriteContentID(capture.InnerBodyID()) ||
				!writer.WriteContentID(capture.OuterBodyID()) || !writer.WriteUint(uint64(capturePosition)) {
				return false
			}
		}
	}

	targetCount, targetsPublished := calltarget.Family().Count(&row.Frozen, catalog)
	if !targetsPublished || !writer.WriteUint(uint64(targetCount)) {
		return false
	}
	for index := 0; index < targetCount; index++ {
		target, held := calltarget.Family().At(&row.Frozen, catalog, index)
		if !held || !writer.WriteContentID(target.AllocationID()) || !writer.WriteContentID(target.BodyID()) ||
			!writer.WriteContentID(target.ContextID()) || !writer.WriteContentID(target.FunctionID()) ||
			!writer.WriteContentID(target.FormalID()) {
			return false
		}
	}

	outcomeCount, outcomesPublished := OutcomeFamily().Count(&row.Frozen, catalog)
	if !outcomesPublished || !writer.WriteUint(uint64(outcomeCount)) {
		return false
	}
	for index := 0; index < outcomeCount; index++ {
		outcome, held := OutcomeFamily().At(&row.Frozen, catalog, index)
		returnOffset, returnCount, returnsOK := outcome.ReturnValueSpan()
		pointOffset, pointCount, pointsOK := outcome.PointSpan()
		target, hasTarget := outcome.TargetID()
		propagation, hasPropagation := outcome.PropagationID()
		if !held || !returnsOK || !pointsOK ||
			!writer.WriteContentID(outcome.ID()) || !writer.WriteContentID(outcome.BodyID()) ||
			!writer.WriteUint(uint64(outcome.Kind())) || !writer.WriteBool(hasTarget) ||
			!writer.WriteContentID(target) || !writer.WriteBool(hasPropagation) ||
			!writer.WriteContentID(propagation) || !writer.WriteUint(uint64(returnOffset)) ||
			!writer.WriteUint(uint64(returnOffset+returnCount)) || !writer.WriteUint(uint64(pointCount)) {
			return false
		}
		for position := uint32(0); position < pointCount; position++ {
			point, childHeld := OutcomePointFamily().At(&row.Frozen, catalog, int(pointOffset+position))
			if !childHeld || point.OutcomeID() != outcome.ID() || !writer.WriteContentID(point.PointID()) {
				return false
			}
		}
	}
	returnValueCount, returnsPublished := OutcomeReturnValueFamily().Count(&row.Frozen, catalog)
	if !returnsPublished || !writer.WriteUint(uint64(returnValueCount)) {
		return false
	}
	for index := 0; index < returnValueCount; index++ {
		value, held := OutcomeReturnValueFamily().At(&row.Frozen, catalog, index)
		if !held || !writer.WriteContentID(value.ValuesID()) {
			return false
		}
	}
	return true
}

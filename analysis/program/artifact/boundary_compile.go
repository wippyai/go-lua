package artifact

import "github.com/wippyai/go-lua/analysis/identity"

// copyBoundaryRowsFailure captures Residence's three structural boundary
// families while the exact transformer proofs are live. Rows contain only
// stable parent IDs and are reusable across mounted Link instances.
func (compiler *compiler) copyBoundaryRowsFailure() CompileFailure {
	rows := make([]BoundaryRow, 0)
	for bodyIndex := 0; bodyIndex < compiler.input.BodyCount(); bodyIndex++ {
		body, bodyOK := compiler.input.BodyAt(bodyIndex)
		if !bodyOK || !compiler.input.OwnsBody(body) {
			return compileFailure(CompileStageOccurrences, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		if !body.Executable() {
			continue
		}
		if function, functionOK := body.Function(); functionOK {
			functionID, functionIDOK := compiler.input.FunctionID(function)
			if !functionIDOK || !functionID.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
			}
			for captureIndex := 0; captureIndex < function.CaptureCount(); captureIndex++ {
				id, _, _, _, _, captureOK := compiler.input.FunctionCaptureAt(function, captureIndex)
				if !captureOK || !id.Available() || uint64(captureIndex) > uint64(^uint32(0)) {
					return compileFailure(CompileStageOccurrences, CompileRowBody, bodyIndex, captureIndex, CompileReasonOccurrenceUnavailable)
				}
				rows = append(rows, BoundaryRow{kind: BoundaryCapture, id: id, owner: functionID, position: uint32(captureIndex), eligible: true})
			}
		}
		bodyBoundary, bodyBoundaryOK := compiler.input.Flow().FunctionBoundaries().ResolveBodyContextID(body.ContextID())
		returned, returnedOK := compiler.input.Flow().BodyReturns().ForBody(bodyBoundary)
		returnSite, returnSiteOK := returned.Outcome()
		if bodyBoundaryOK && returnedOK && returnSiteOK {
			id := returnSite.ContextID()
			if !id.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowBody, bodyIndex, -1, CompileReasonOccurrenceUnavailable)
			}
			rows = append(rows, BoundaryRow{kind: BoundaryReturn, id: id, owner: body.ContextID(), eligible: true})
		}
	}
	assigns := compiler.input.Flow().Authored().Storage().Assigns()
	for assignmentIndex := 0; assignmentIndex < assigns.Count(); assignmentIndex++ {
		assignment, assignmentOK := compiler.storageAssignmentAt(assignmentIndex)
		if !assignmentOK || !assignment.id.Available() || !assignment.context.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, assignmentIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		for _, write := range assignment.transfers {
			if !write.id.Available() || !write.cell.Available() || !write.predecessor.Available() || !write.route.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, assignmentIndex, write.position, CompileReasonOccurrenceUnavailable)
			}
			// Assignment scratch rows are built from Flow/Source while the
			// Program proof is live; only the body context and scalar write ID
			// cross into the generic boundary column.
			rows = append(rows, BoundaryRow{kind: BoundaryStore, id: write.id, owner: assignment.context, eligible: write.eligible})
		}
	}
	radixBoundaryRows(rows)
	compiler.boundaries = rows
	return CompileFailure{}
}

func radixBoundaryRows(rows []BoundaryRow) {
	if len(rows) < 2 {
		return
	}
	// The ID radix establishes the exact lexicographic order; a final stable
	// kind pass makes the complete key (kind, ContentID) without comparison
	// callbacks or string conversion.
	identity.SortByContentID(rows, func(row BoundaryRow) identity.ContentID { return row.id })
	var kindCounts [BoundaryReturn + 1]int
	for _, row := range rows {
		kindCounts[row.kind]++
	}
	offset := 0
	for kind, count := range kindCounts {
		kindCounts[kind], offset = offset, offset+count
	}
	work := make([]BoundaryRow, len(rows))
	for _, row := range rows {
		bucket := row.kind
		work[kindCounts[bucket]] = row
		kindCounts[bucket]++
	}
	copy(rows, work)
}

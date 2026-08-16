package programartifact

import "github.com/wippyai/go-lua/program/keyspace"

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
		if function, functionOK := body.TransformerFunction(); functionOK {
			functionID := function.ContextID()
			if !functionID.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
			}
			for captureIndex := 0; captureIndex < function.CaptureCount(); captureIndex++ {
				capture, captureOK := function.CaptureAt(captureIndex)
				id := capture.ContextID()
				if !captureOK || !compiler.input.OwnsCapture(capture) || !id.Available() || uint64(captureIndex) > uint64(^uint32(0)) {
					return compileFailure(CompileStageOccurrences, CompileRowBody, bodyIndex, captureIndex, CompileReasonOccurrenceUnavailable)
				}
				rows = append(rows, BoundaryRow{kind: BoundaryCapture, id: id, owner: functionID, position: uint32(captureIndex), eligible: true})
			}
		}
		if returned, returnedOK := body.Return(); returnedOK {
			id := returned.ContextID()
			if !id.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowBody, bodyIndex, -1, CompileReasonOccurrenceUnavailable)
			}
			rows = append(rows, BoundaryRow{kind: BoundaryReturn, id: id, owner: body.ContextID(), eligible: true})
		}
	}
	for assignmentIndex := 0; assignmentIndex < compiler.input.StorageAssignmentCount(); assignmentIndex++ {
		assignment, assignmentOK := compiler.input.StorageAssignmentAt(assignmentIndex)
		if !assignmentOK || !compiler.input.OwnsStorageAssignment(assignment) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, assignmentIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		for position := 0; position < assignment.TransferCount(); position++ {
			write, writeOK := assignment.TransferAt(position)
			if !writeOK {
				// Assignment width covers both storage Cells and index Lenses,
				// and may also exceed a source Values row's fixed prefix. Only
				// an owner-issued Cell transfer belongs to BoundaryStore. Lens
				// writes are emitted once by copyIndexAccess as RawSet rows.
				continue
			}
			id := write.ContextID()
			body, bodyOK := write.Body()
			if !compiler.input.OwnsStorageWriteOccurrence(write) || !bodyOK || !id.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, assignmentIndex, position, CompileReasonOccurrenceUnavailable)
			}
			rows = append(rows, BoundaryRow{kind: BoundaryStore, id: id, owner: body.ContextID(), eligible: write.Eligible()})
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
	// LSD passes over the fixed-width ContentID bytes establish the exact
	// lexicographic ID order. A final tiny kind pass makes the complete key
	// (kind, ContentID) without comparison callbacks or string conversion.
	work := make([]BoundaryRow, len(rows))
	src, dst := rows, work
	for byteIndex := len(keyspace.ContentID{}) - 1; byteIndex >= 0; byteIndex-- {
		var counts [256]int
		for _, row := range src {
			counts[row.id[byteIndex]]++
		}
		offset := 0
		for bucket, count := range counts {
			counts[bucket], offset = offset, offset+count
		}
		for _, row := range src {
			bucket := row.id[byteIndex]
			dst[counts[bucket]] = row
			counts[bucket]++
		}
		src, dst = dst, src
	}
	var kindCounts [BoundaryReturn + 1]int
	for _, row := range src {
		kindCounts[row.kind]++
	}
	offset := 0
	for kind, count := range kindCounts {
		kindCounts[kind], offset = offset, offset+count
	}
	for _, row := range src {
		bucket := row.kind
		dst[kindCounts[bucket]] = row
		kindCounts[bucket]++
	}
	copy(rows, dst)
}

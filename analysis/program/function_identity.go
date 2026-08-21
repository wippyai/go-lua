package program

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/functionboundary"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/internal/framing"
)

// FunctionID issues the root-fenced identity of one existing callable
// Function boundary. Program owns the root fence; the boundary context is
// Flow's already-sealed callable coordinate. No caller may rebuild this
// equation from a copied ProgramID or an authored Function term.
func (program *Program) FunctionID(boundary functionboundary.Boundary) (identity.ContentID, bool) {
	if program == nil || !program.Available() || !boundary.Available() {
		return identity.ContentID{}, false
	}
	view := program.Flow()
	boundaries := view.FunctionBoundaries()
	bodyTerm, bodyTermOK := boundary.Body()
	body, bodyOK := boundaries.ForBody(bodyTerm)
	context := boundary.ContextID()
	if !boundaries.OwnsFunction(boundary) || !bodyTermOK || !bodyOK || !boundaries.OwnsBody(body) || !body.Available() || !context.Available() {
		return identity.ContentID{}, false
	}

	hash := sha256.New()
	var writer framing.Writer
	programID := program.ContentID()
	if writer.Reset(hash, "program/transformer/function", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(programID[:]) != nil || writer.Bytes(context[:]) != nil || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

// FunctionCaptureIDs issues the root-fenced identity and scalar component
// identities of one existing closure capture. Flow owns the inner/outer Body
// proof and path-based cell equations; Program owns the final equation because
// it commits the exact root-fenced StorageCell identities as well.
func (program *Program) FunctionCaptureIDs(boundary functionboundary.Boundary, index int) (captureID, innerID, outerID, innerStorageID, outerStorageID, innerBodyID, outerBodyID identity.ContentID, ok bool) {
	if program == nil || !program.Available() || !boundary.Available() || index < 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	view := program.Flow()
	pair, pairOK := boundary.CaptureAt(index)
	if !pairOK || pair.Inner == 0 || pair.Outer == 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	innerID, outerID, innerBodyID, outerBodyID, cellsOK := view.FunctionCaptureCells(boundary, index)
	if !cellsOK {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	cells := view.Authored().Storage().Cells()
	if _, _, _, innerCellOK := cells.Get(pair.Inner); !innerCellOK ||
		keyspace.TermFamily(pair.Inner) != keyspace.FamilyCell || keyspace.TermOrdinal(pair.Inner) == 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	if _, _, _, outerCellOK := cells.Get(pair.Outer); !outerCellOK ||
		keyspace.TermFamily(pair.Outer) != keyspace.FamilyCell || keyspace.TermOrdinal(pair.Outer) == 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	programID := program.ContentID()
	var innerStorageOK, outerStorageOK bool
	innerStorageID, innerStorageOK = lifecycle.StorageCellIdentity(programID, pair.Inner)
	outerStorageID, outerStorageOK = lifecycle.StorageCellIdentity(programID, pair.Outer)
	if !innerStorageOK || !outerStorageOK || !innerStorageID.Available() || !outerStorageID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/transformer/capture", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(innerBodyID[:]) != nil || writer.Bytes(outerBodyID[:]) != nil || writer.Uint(uint64(index)) != nil ||
		writer.Bytes(innerID[:]) != nil || writer.Bytes(outerID[:]) != nil || writer.Bytes(innerStorageID[:]) != nil ||
		writer.Bytes(outerStorageID[:]) != nil || writer.Finish() != nil {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	copy(captureID[:], hash.Sum(nil))
	return captureID, innerID, outerID, innerStorageID, outerStorageID, innerBodyID, outerBodyID, captureID.Available()
}

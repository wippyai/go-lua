package compiler

import (
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/rowidentity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
)

// copyStorageCellLifetimesFailure publishes the neutral storage ownership
// proof consumed by mounted Value/Placement domains. The compiler only emits
// facts available from canonical Flow ownership:
//
//   - a local Cell owned by the parentless module Body is module-owned;
//   - a local Cell owned by another proven Body is frame-local;
//   - a global Cell remains Unknown until a mounted Host mapping proves
//     cross-context authority.
//
// In particular, this pass never treats the CellGlobal spelling as proof of
// process-global storage. Unknown is safer than silently leaving a retained
// allocation Stack or inventing a Shared owner.
func (compiler *compiler) copyStorageCellLifetimesFailure() CompileFailure {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	programID := compiler.key.ProgramID()
	if !programID.Available() {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}

	view := compiler.input.Flow()
	cells := view.Authored().Storage().Cells()
	bodyForest := view.Body()
	// A local Cell participating in a closure capture is not proven to die
	// with the Body that introduced it.  Flow currently publishes the capture
	// edge, but does not publish a non-escape certificate for the captured
	// storage itself.  Keep those cells Unknown until such a proof exists;
	// treating them as Frame would allow a retained closure to point at a
	// reclaimed frame allocation.
	captured := make(map[keyspace.Term]struct{})
	functions := view.Authored().Functions()
	for functionIndex := 0; functionIndex < functions.Count(); functionIndex++ {
		function, functionOK := functions.At(functionIndex)
		if !functionOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, functionIndex, -1, CompileReasonBodyUnavailable)
		}
		captureCount, captureCountOK := functions.CaptureCount(function)
		if !captureCountOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, functionIndex, -1, CompileReasonBodyUnavailable)
		}
		for captureIndex := 0; captureIndex < captureCount; captureIndex++ {
			inner, outer, captureOK := functions.CaptureAt(function, captureIndex)
			if !captureOK {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, functionIndex, captureIndex, CompileReasonBodyUnavailable)
			}
			captured[inner] = struct{}{}
			captured[outer] = struct{}{}
		}
	}
	var entry keyspace.Term
	entryOK := false
	if bodyForest != nil {
		entry, entryOK = bodyForest.Entry()
	}
	compiler.publication.Lifecycle.StorageCellLifetimes = make([]lifecycle.StorageCellLifetime, 0, cells.Count())
	for index := 0; index < cells.Count(); index++ {
		term, termOK := cells.At(index)
		if !termOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		kind, body, _, cellOK := cells.Get(term)
		if !cellOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		cellID, idOK := rowidentity.StorageCellID(programID, view, term)
		if !idOK || !cellID.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}

		lifetime := lifecycle.StorageLifetimeUnknown
		switch kind {
		case authored.CellLocal:
			// The exact module entry Body is the only lexical owner proven to
			// outlive a frame. A valid non-entry Body is frame-local; malformed
			// or unavailable Body evidence stays Unknown.
			if entryOK && body == entry {
				lifetime = lifecycle.StorageLifetimeModule
			} else if _, escapes := captured[term]; escapes {
				// Capture membership is positive escape evidence, not a
				// lifetime classification.  Without an explicit non-escape
				// proof, preserve Unknown for both sides of the edge.
				lifetime = lifecycle.StorageLifetimeUnknown
			} else if bodyForest != nil {
				if _, validBody := bodyForest.Activation(body); validBody {
					lifetime = lifecycle.StorageLifetimeFrame
				}
			}
		case authored.CellGlobal:
			// Host/global authority is Link-owned and is intentionally joined
			// later by Value. CellGlobal alone is not process-global proof.
			lifetime = lifecycle.StorageLifetimeUnknown
		default:
			lifetime = lifecycle.StorageLifetimeUnknown
		}
		row, rowOK := lifecycle.NewStorageCellLifetime(cellID, lifetime)
		if !rowOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		compiler.storageCellLifetimes = append(compiler.storageCellLifetimes, row)
	}
	return CompileFailure{}
}

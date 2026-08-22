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
//   - a local Cell participating in a proven closure capture is closure-owned
//     retained storage, because its environment must outlive the introducing
//     frame without becoming module-entry state;
//   - a global Cell remains Unknown until a mounted Host mapping proves
//     cross-context authority.
//
// Unknown is therefore an authenticated semantic state for an authored global
// whose external owner has not yet been joined. It is never a default for a
// malformed Cell, unavailable Body, or invalid capture relation.
func (compiler *compiler) copyStorageCellLifetimesFailure() CompileFailure {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	programID := compiler.key.ProgramID()
	if !programID.Available() {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}

	view := compiler.input.Flow()
	if !view.ContentID().Available() || !view.Authored().ContentID().Available() {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	cells := view.Authored().Storage().Cells()
	bodyForest := view.Body()
	if bodyForest == nil {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	entry, entryOK := bodyForest.Entry()
	if !entryOK || entry == 0 {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	validBody := func(body keyspace.Term) bool {
		if body == entry {
			return true
		}
		_, ok := bodyForest.Activation(body)
		return ok
	}
	// A capture is positive evidence that the closure environment, rather
	// than the introducing frame, owns the captured storage. The environment
	// remains within the mounted module until a later publication/return rule
	// proves a stronger boundary, so Closure is the least sound class in this
	// neutral vocabulary. This avoids both an unsound Frame result and an
	// unauthenticated Unknown result without conflating the closure with the
	// module entry owner.
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
			innerKind, innerBody, innerKey, innerOK := cells.Get(inner)
			outerKind, outerBody, outerKey, outerOK := cells.Get(outer)
			if !innerOK || !outerOK || inner == outer ||
				innerKind != authored.CellLocal || outerKind != authored.CellLocal ||
				innerKey != 0 || outerKey != 0 || innerBody == 0 || outerBody == 0 {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, functionIndex, captureIndex, CompileReasonBodyUnavailable)
			}
			if !validBody(innerBody) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, functionIndex, captureIndex, CompileReasonBodyUnavailable)
			}
			if !validBody(outerBody) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, functionIndex, captureIndex, CompileReasonBodyUnavailable)
			}
			captured[inner] = struct{}{}
			captured[outer] = struct{}{}
		}
	}
	compiler.publication.Lifecycle.StorageCellLifetimes = make([]lifecycle.StorageCellLifetime, 0, cells.Count())
	for index := 0; index < cells.Count(); index++ {
		term, termOK := cells.At(index)
		if !termOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		kind, body, key, cellOK := cells.Get(term)
		if !cellOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		if kind != authored.CellLocal && kind != authored.CellGlobal {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		cellID, idOK := rowidentity.StorageCellID(programID, view, term)
		if !idOK || !cellID.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}

		switch kind {
		case authored.CellLocal:
			if key != 0 || body == 0 {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
			}
			if !validBody(body) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
			}
			lifetime := lifecycle.StorageLifetimeFrame
			if body == entry {
				lifetime = lifecycle.StorageLifetimeModule
			} else if _, escapes := captured[term]; escapes {
				lifetime = lifecycle.StorageLifetimeClosure
			}
			row, rowOK := lifecycle.NewStorageCellLifetime(cellID, lifetime)
			if !rowOK {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
			}
			compiler.publication.Lifecycle.StorageCellLifetimes = append(compiler.publication.Lifecycle.StorageCellLifetimes, row)
		case authored.CellGlobal:
			if body != 0 || key == 0 {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
			}
			// Host/global authority is Link-owned and is intentionally joined
			// later by Value. CellGlobal alone is not process-global proof.
			row, rowOK := lifecycle.NewStorageCellLifetime(cellID, lifecycle.StorageLifetimeUnknown)
			if !rowOK {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
			}
			compiler.publication.Lifecycle.StorageCellLifetimes = append(compiler.publication.Lifecycle.StorageCellLifetimes, row)
		default:
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
	}
	return CompileFailure{}
}

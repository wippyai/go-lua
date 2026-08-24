package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/rowidentity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
)

// copyStorageCellLifetimesFailure publishes the neutral storage ownership
// proof consumed by mounted Value/Placement domains. The compiler only emits
// facts available from canonical Flow ownership:
//
//   - every local Cell is frame-local unless another owner is proven;
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
	if compiler.bodyBoundary == nil {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	// BodyBoundary has already authenticated every lexical capture and issued
	// the exact storage identities consumed here. Lifetime must not reopen
	// authored Functions and reconstruct that relation from Cell terms.
	captures := compiler.bodyBoundary.FunctionCaptures()
	captured := make(map[identity.ContentID]struct{}, len(captures)*2)
	for captureIndex, capture := range captures {
		inner := capture.InnerStorageCellID()
		outer := capture.OuterStorageCellID()
		if !capture.Available() || !inner.Available() || !outer.Available() || inner == outer {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, captureIndex, -1, CompileReasonBodyUnavailable)
		}
		captured[inner] = struct{}{}
		captured[outer] = struct{}{}
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
			lifetime := lifecycle.StorageLifetimeFrame
			if _, escapes := captured[cellID]; escapes {
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

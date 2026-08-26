package arithmetic

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// SourceAChangedDelta is the fixture's owner-issued source transition from an
// exact committed root. It reuses the already-bound SeedSource worker and the
// same Apply/Publish doors used to create Base; it does not construct a second
// writer or expose a mutable state alternative. SourceA keeps its join address
// and changes its two arithmetic operands, so the scheduled dependency stays
// the exact arithmetic expression while one affected output genuinely
// changes. The caller supplies the predecessor root because a Later delta is
// meaningful only after the predecessor dependency has been fully materialized.
func (fixture Fixture) SourceAChangedDelta(base database.Version) (database.Delta, bool) {
	if !fixture.mounted.Available() || !base.Available() || fixture.seedSource == nil || !fixture.declaration.Schema.Available() {
		return database.Delta{}, false
	}
	if !base.Mounted().Same(fixture.mounted) || !base.Fence().Same(fixture.mounted.RuntimeFence()) || base.MountedDigest() != fixture.mounted.Digest() || base.ArrangementDigest() != fixture.mounted.Arrangement().Digest() {
		return database.Delta{}, false
	}
	if len(fixture.declaration.Signatures) < 2 {
		return database.Delta{}, false
	}
	operation := fixture.declaration.Signatures[1]
	if !operation.Available() || operation.Identity() != fixture.declaration.IDs.SeedSource {
		return database.Delta{}, false
	}
	scope, ok := fixture.mounted.Scope(fixture.declaration.IDs.Scope)
	if !ok || !scope.Available() {
		return database.Delta{}, false
	}
	token, ok := fixture.mounted.ScopeToken(scope)
	if !ok || !token.Available() {
		return database.Delta{}, false
	}
	addressA, ok := fixture.mounted.IssueValue(fixture.declaration.IDs.Type, derive("address/a"))
	if !ok {
		return database.Delta{}, false
	}
	changedA, ok := fixture.mounted.IssueValue(fixture.declaration.IDs.Type, derive("address/z"))
	if !ok {
		return database.Delta{}, false
	}
	outputs := operation.Outputs()
	if len(outputs) != 3 || outputs[0].Denominator != fixture.declaration.Sources || outputs[1].Denominator != fixture.declaration.Sources || outputs[2].Denominator != fixture.declaration.Sources {
		return database.Delta{}, false
	}
	values := []binding.ValueToken{addressA, changedA, changedA}
	denominatorWitness, witnessOK := fixture.mounted.Denominator(fixture.declaration.Sources)
	if !witnessOK {
		return database.Delta{}, false
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		return database.Delta{}, false
	}
	proposals := make([]binding.Proposal, 0, len(outputs))
	for index, output := range outputs {
		cell, cellOK := fixture.mounted.IssueCell(denominatorWitness, scope, output.Column, fixture.declaration.IDs.SourceA)
		if !cellOK {
			return database.Delta{}, false
		}
		proposal, proposalOK := binding.NewProposal(cell, values[index], presence)
		if !proposalOK {
			return database.Delta{}, false
		}
		proposals = append(proposals, proposal)
	}
	fixture.seedSource.operation = operation
	fixture.seedSource.proposals = map[binding.ScopeToken][]binding.Proposal{token: proposals}
	provenance, ok := arithmeticProvenance(fixture.mounted, operation)
	if !ok {
		return database.Delta{}, false
	}
	application, ok := apply.Apply(fixture.mounted, operation.Identity(), scope, provenance, binding.NewOwnerNamedDestination(operation.Outputs()[0].Relation))
	if !ok || !application.Available() || application.Outcome().Code != outcome.Produced {
		return database.Delta{}, false
	}
	scratch := store.NewReadScratch(fixture.view.Manager())
	if scratch == nil || !scratch.Available() {
		return database.Delta{}, false
	}
	door, ok := publish.New(fixture.mounted, fixture.view)
	if !ok || !door.Available() {
		return database.Delta{}, false
	}
	settlement := door.Publish(base, scratch, application, witness.WideningPermit{})
	if !settlement.Available() || !settlement.Changed() {
		return database.Delta{}, false
	}
	delta, ok := settlement.Delta()
	if !ok || !delta.Available() || !delta.Base().Same(base) || !delta.Next().Same(settlement.Next()) {
		return database.Delta{}, false
	}
	return delta, true
}

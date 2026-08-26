package targetfixture

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/transaction"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type seedWorker struct {
	proposals map[binding.ScopeToken][]binding.Proposal
}

func (value *seedWorker) Evaluate(frame binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	if value == nil || buffer == nil || !frame.Available() || frame.Len() != 0 {
		return outcome.Result{}
	}
	proposals, ok := value.proposals[frame.Scope()]
	if !ok {
		return outcome.Result{Code: outcome.NoSelection}
	}
	for _, proposal := range proposals {
		if !buffer.Append(proposal) {
			return outcome.Result{}
		}
	}
	return outcome.Result{Code: outcome.Produced}
}

type seedBinding struct {
	operation signature.Signature
	worker    *seedWorker
}

func (value seedBinding) Signature() signature.Signature { return value.operation }

func (value seedBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return value.worker, value.worker != nil
}

type seedFactory struct{ binding seedBinding }

func (value seedFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() || operation.Digest() != value.binding.operation.Digest() {
		return nil, false
	}
	return value.binding, true
}

func newSeeds(t Probe, initials []Initial) ([]binding.Factory, []*seedWorker) {
	t.Helper()
	factories := make([]binding.Factory, len(initials))
	workers := make([]*seedWorker, len(initials))
	for index, initial := range initials {
		worker := &seedWorker{proposals: make(map[binding.ScopeToken][]binding.Proposal)}
		workers[index] = worker
		factories[index] = seedFactory{binding: seedBinding{operation: initial.Operation, worker: worker}}
	}
	return factories, workers
}

func publishSeeds(t Probe, mounted witness.Mounted, view geometry.Geometry, base database.Version, issuer binding.Issuer, initials []Initial, workers []*seedWorker) database.Version {
	t.Helper()
	if len(initials) != len(workers) {
		t.Fatal("target fixture seed shape")
	}
	if len(initials) == 0 {
		return base
	}
	door, ok := publish.New(mounted, view)
	if !ok || !door.Available() {
		t.Fatal("target fixture seed publication door")
	}
	scratch := store.NewReadScratch(view.Manager())
	if scratch == nil || !scratch.Available() {
		t.Fatal("target fixture seed scratch")
	}
	for index, initial := range initials {
		scope, ok := mounted.Scope(initial.Scope)
		if !ok {
			t.Fatalf("target fixture seed %d scope", index)
		}
		scopeToken, ok := mounted.ScopeToken(scope)
		if !ok {
			t.Fatalf("target fixture seed %d scope token", index)
		}
		cells, ok := initial.Cells(issuer)
		if !ok || len(cells) == 0 {
			t.Fatalf("target fixture seed %d cells", index)
		}
		proposals := make([]binding.Proposal, 0, len(cells))
		witnesses := make(map[model.DenominatorRef]binding.DenominatorWitness)
		for cellIndex, cell := range cells {
			output, outputOK := initial.Operation.OutputFor(cell.Row.Relation(), cell.Column)
			if !outputOK || cell.Denominator != output.Denominator || !cell.Denominator.Available() || !cell.Row.Available() || cell.Row.Relation() != cell.Denominator.Relation() || !cell.Column.Available() || cell.Column.Relation() != cell.Denominator.Relation() || !cell.Presence.Available() {
				t.Fatalf("target fixture seed %d cell %d declaration", index, cellIndex)
			}
			denominatorWitness, witnessOK := witnesses[cell.Denominator]
			if !witnessOK {
				denominatorWitness, witnessOK = mounted.Denominator(cell.Denominator)
				if !witnessOK {
					t.Fatalf("target fixture seed %d cell %d witness", index, cellIndex)
				}
				witnesses[cell.Denominator] = denominatorWitness
			}
			destination, ok := mounted.IssueCell(denominatorWitness, scope, cell.Column, cell.Row)
			if !ok {
				t.Fatalf("target fixture seed %d cell %d destination", index, cellIndex)
			}
			proposal, ok := binding.NewProposal(destination, cell.Value, cell.Presence)
			if !ok {
				t.Fatalf("target fixture seed %d cell %d proposal", index, cellIndex)
			}
			proposals = append(proposals, proposal)
		}
		workers[index].proposals[scopeToken] = proposals
		provenance, ok := targetInitialProvenance(mounted, initial.Operation)
		if !ok {
			t.Fatalf("target fixture seed %d lineage", index)
		}
		application, ok := apply.Apply(mounted, initial.Operation.Identity(), scope, provenance, binding.NewOwnerNamedDestination(initial.Operation.Outputs()[0].Relation))
		if !ok || !application.Available() || application.Outcome().Code != outcome.Produced {
			t.Fatalf("target fixture seed %d apply", index)
		}
		settlement := door.Publish(base, scratch, application, witness.WideningPermit{})
		if !settlement.Available() || !settlement.Changed() {
			fmt.Printf("debug publish available=%v changed=%v\\n", settlement.Available(), settlement.Changed())
			return base
			delta, deltaOK := settlement.Delta()
			proposals, proposalsOK := application.Proposals()
			proposalLen := -1
			if proposalsOK {
				proposalLen = proposals.Len()
			}
			batch, batchOK := transaction.NewSubmissionBatch(application, witness.WideningPermit{}, nil)
			prepared, preparedOK := transaction.Prepare(base, view, scratch, batch)
			t.Fatalf("target fixture seed %d publish available=%t changed=%t outcome=%v app=%t proposals=%t len=%d lineage=%t delta=%t deltaOK=%t batch=%t prepared=%t preparedOK=%t", index, settlement.Available(), settlement.Changed(), application.Outcome().Code, application.Available(), proposalsOK, proposalLen, application.Lineage().Available(), delta.Available(), deltaOK, batchOK, prepared.Available(), preparedOK)
		}
		base = settlement.Next()
	}
	return base
}

func targetInitialProvenance(mounted witness.Mounted, operation signature.Signature) (model.LineageRef, bool) {
	if !mounted.Available() || !operation.Available() || operation.OutputLen() == 0 {
		return model.LineageRef{}, false
	}
	authority, ok := mounted.Lineage()
	if !ok || authority == nil {
		return model.LineageRef{}, false
	}
	var result model.LineageRef
	for _, output := range operation.Outputs() {
		if !output.Denominator.Available() {
			return model.LineageRef{}, false
		}
		ref, refOK := mounted.DenominatorLineage(output.Denominator)
		if !refOK {
			return model.LineageRef{}, false
		}
		if !result.Available() {
			result = ref
			continue
		}
		result, ok = authority.Join(result, ref)
		if !ok {
			return model.LineageRef{}, false
		}
	}
	return result, result.Available()
}

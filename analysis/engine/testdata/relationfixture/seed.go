package testfixture

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// commitRows is the only fixture database writer. It publishes through the
// public application/door/transaction boundary so operator tests exercise the
// same immutable root and lineage fences as a real mounted solve.
func commitRows(t TB, mounted witness.Mounted, door publish.Door, base database.Version, scratch *store.ReadScratch, worker *worker, operation signature.Identity, first, second witness.Scope) (database.Version, database.Delta) {
	t.Helper()
	var committed database.Delta
	provenance, provenanceOK := worker.provenance(mounted, operation)
	if !provenanceOK {
		t.Fatalf("worker provenance")
	}
	for index, scope := range []witness.Scope{first, second} {
		if index > 0 && scope.Same(first) {
			// A BoundedMany application carries both rows in one production
			// transaction. The duplicate scope arguments retain the helper's
			// row-pair shape without publishing a second successor.
			continue
		}
		if worker == nil || len(worker.operation.Outputs()) == 0 {
			t.Fatalf("operation output")
		}
		application, ok := apply.Apply(mounted, operation, scope, provenance, binding.NewOwnerNamedDestination(worker.operation.Outputs()[0].Relation))
		if !ok || !application.Available() || application.Outcome().Code != outcome.Produced {
			t.Fatalf("apply row")
		}
		settlement := door.Publish(base, scratch, application, witness.WideningPermit{})
		if !settlement.Available() || !settlement.Changed() {
			t.Fatalf("publish row")
		}
		delta, deltaOK := settlement.Delta()
		if !deltaOK {
			t.Fatalf("publish delta")
		}
		committed = delta
		base = settlement.Next()
	}
	return base, committed
}

func prepareWorker(t TB, mounted witness.Mounted, worker *worker, operation signature.Signature, denominator model.DenominatorRef, first, second witness.Scope, firstRow, secondRow model.RowID, columns []model.ColumnID, values [2][4]binding.ValueToken) {
	t.Helper()
	if worker == nil {
		t.Fatal("nil worker")
	}
	if worker.proposals == nil {
		worker.proposals = make(map[binding.ScopeToken][]binding.Proposal)
	}
	denominatorWitness, witnessOK := mounted.Denominator(denominator)
	if !witnessOK {
		t.Fatal("worker denominator witness")
	}
	for index, scope := range []witness.Scope{first, second} {
		token, ok := mounted.ScopeToken(scope)
		if !ok {
			t.Fatal("worker scope token")
		}
		row := firstRow
		if index == 1 {
			row = secondRow
		}
		proposals := make([]binding.Proposal, len(columns))
		for columnIndex, column := range columns {
			cell, cellOK := mounted.IssueCell(denominatorWitness, scope, column, row)
			if !cellOK {
				t.Fatal("worker cell")
			}
			proposal, proposalOK := binding.NewProposal(cell, values[index][columnIndex], mustPresence(t))
			if !proposalOK {
				t.Fatal("worker proposal")
			}
			proposals[columnIndex] = proposal
		}
		worker.proposals[token] = append(worker.proposals[token], proposals...)
	}
	worker.operation = operation
}

func (worker *worker) provenance(mounted witness.Mounted, operation signature.Identity) (model.LineageRef, bool) {
	if worker == nil || !mounted.Available() || !operation.Available() {
		return model.LineageRef{}, false
	}
	bound, ok := mounted.Binding(operation)
	if !ok || bound == nil {
		return model.LineageRef{}, false
	}
	// Use the exact closed-world provenance admitted for the operation's output
	// denominator. Fixtures must redeem mount-issued evidence just like the
	// runtime Apply bridge; minting a same-owner or synthetic source atom here
	// would test a path no production caller is allowed to use.
	return operationProvenance(mounted, bound.Signature())
}

// operationProvenance redeems all explicit output denominator witnesses. A
// fixture follows the same no-fallback rule as production Apply: there is no
// operation-wide destination authority to consult.
func operationProvenance(mounted witness.Mounted, operation signature.Signature) (model.LineageRef, bool) {
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

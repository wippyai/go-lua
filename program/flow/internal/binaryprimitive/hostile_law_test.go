package binaryprimitive

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/program/flow/internal/causal"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// These are deliberately the two narrow row-reader seams used by the
// production pass. They let hostile laws supply malformed relation rows
// without puncturing authored or causal owner encapsulation.
type hostileBranchRow struct {
	term                keyspace.Term
	owner, condition    keyspace.Term
	whenTrue, whenFalse keyspace.Term
}

type hostileBranches struct{ rows []hostileBranchRow }

func (branches hostileBranches) Count() int { return len(branches.rows) }

func (branches hostileBranches) At(index int) (keyspace.Term, bool) {
	if index < 0 || index >= len(branches.rows) {
		return 0, false
	}
	return branches.rows[index].term, true
}

func (branches hostileBranches) Get(term keyspace.Term) (owner, condition, whenTrue, whenFalse keyspace.Term, ok bool) {
	for index := range branches.rows {
		row := branches.rows[index]
		if row.term == term {
			return row.owner, row.condition, row.whenTrue, row.whenFalse, true
		}
	}
	return 0, 0, 0, 0, false
}

type hostileSuccessors struct {
	binary keyspace.Term
	rows   []causal.Successor
}

func (successors hostileSuccessors) Count(from keyspace.Term) int {
	if from != successors.binary {
		return 0
	}
	return len(successors.rows)
}

func (successors hostileSuccessors) At(from keyspace.Term, index int) (causal.Successor, bool) {
	if from != successors.binary || index < 0 || index >= len(successors.rows) {
		return causal.Successor{}, false
	}
	return successors.rows[index], true
}

func hostileComparisonFixture() (binary keyspace.Term, comparison Comparison, rows []causal.Successor) {
	binary = keyspace.MakeTerm(keyspace.FamilyBinary, 14)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	trueBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	falseBody := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	left := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	right := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	comparison = Comparison{Branch: branch, TrueBody: trueBody, FalseBody: falseBody, Left: left, Right: right}
	rows = []causal.Successor{
		{From: binary, To: trueBody, Decision: branch, Truth: true, Arm: causal.BoundaryLocal},
		{From: binary, To: falseBody, Decision: branch, Truth: false, Arm: causal.BoundaryLocal},
	}
	return binary, comparison, rows
}

func TestBinaryPrimitiveRejectsMissingDuplicateAndMalformedCausalTruthArms(t *testing.T) {
	binary, comparison, valid := hostileComparisonFixture()
	cases := []struct {
		name string
		rows []causal.Successor
	}{
		{name: "missing-false", rows: valid[:1]},
		{name: "duplicate-true", rows: []causal.Successor{valid[0], valid[0]}},
		{name: "wrong-decision", rows: []causal.Successor{valid[0], {From: binary, To: comparison.FalseBody, Truth: false, Arm: causal.BoundaryLocal}}},
		{name: "wrong-body", rows: []causal.Successor{valid[0], {From: binary, To: comparison.TrueBody, Decision: comparison.Branch, Truth: false, Arm: causal.BoundaryLocal}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			reader := hostileSuccessors{binary: binary, rows: test.rows}
			if err := validateCausalSuccessorArms(reader, binary, comparison); err == nil {
				t.Fatal("malformed causal truth matrix was admitted")
			}
		})
	}
}

func TestBinaryPrimitiveRejectsDuplicateBranchPerBinaryThroughBranchReader(t *testing.T) {
	fixture := openBinaryPrimitiveFixture(t, flowkind.BinaryEqual)
	candidateResult, err := candidates.Seal(fixture.sourceView.Identity(), fixture.flow, fixture.executable, fixture.staticID, fixture.moduleID)
	if err != nil {
		t.Fatalf("candidates.Seal: %v", err)
	}
	result, err := Seal(fixture.sourceView, fixture.flow, candidateResult, fixture.causal, fixture.staticID, fixture.moduleID)
	if err != nil {
		t.Fatalf("binaryprimitive.Seal: %v", err)
	}
	branchTerm := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	secondBranchTerm := keyspace.MakeTerm(keyspace.FamilyBranch, 2)
	branchOwner, condition, whenTrue, whenFalse, ok := fixture.flow.Control().Branches().Get(branchTerm)
	if !ok {
		t.Fatal("real authored Branch row is unavailable")
	}
	branches := hostileBranches{rows: []hostileBranchRow{
		{term: branchTerm, owner: branchOwner, condition: condition, whenTrue: whenTrue, whenFalse: whenFalse},
		{term: secondBranchTerm, owner: branchOwner, condition: condition, whenTrue: whenTrue, whenFalse: whenFalse},
	}}
	if err := sealBranchComparisons(result, branches, fixture.causal); err == nil {
		t.Fatal("duplicate Branch condition was admitted")
	}
}

func TestBinaryPrimitiveProductionSealStillUsesConcreteCausalReader(t *testing.T) {
	fixture := openBinaryPrimitiveFixture(t, flowkind.BinaryEqual)
	candidateResult, err := candidates.Seal(fixture.sourceView.Identity(), fixture.flow, fixture.executable, fixture.staticID, fixture.moduleID)
	if err != nil {
		t.Fatalf("candidates.Seal: %v", err)
	}
	result, err := Seal(fixture.sourceView, fixture.flow, candidateResult, fixture.causal, fixture.staticID, fixture.moduleID)
	if err != nil {
		t.Fatalf("binaryprimitive.Seal: %v", err)
	}
	primitive, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 14))
	if !ok {
		t.Fatal("concrete production causal reader did not publish primitive")
	}
	if _, ok := primitive.Comparison(); !ok {
		t.Fatal("concrete production causal reader did not publish comparison")
	}
}

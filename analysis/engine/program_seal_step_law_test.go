package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// TestARuntimeAssemblyRefusalPublishesItsStep is the law the program seal was
// missing.
//
// Every other construction stage publishes the predicate that failed beside
// the stage, so a caller can tell a malformed member row from a malformed
// query row. The program seal published the stage alone: a table disagreement
// anywhere inside the runtime assembly - the member rows, the query rows, the
// per-Factor execution family directory - arrived as "the program did not
// seal", which is the one thing the caller already knew. Each step must now
// be its own boundary, and none of them may collapse onto the bare stage.
func TestARuntimeAssemblyRefusalPublishesItsStep(t *testing.T) {
	bare := ProgramStageFailure(ProgramSealStageProgramSeal)
	if !bare.Available() {
		t.Fatal("the bare program-seal stage is not a failure")
	}
	seen := map[identity.ContentID]topologyConstructionStep{}
	for _, step := range []topologyConstructionStep{
		topologyConstructionStepDeclarationShape,
		topologyConstructionStepBinding,
		topologyConstructionStepMemberGroup,
		topologyConstructionStepMemberRow,
		topologyConstructionStepQueryRow,
		topologyConstructionStepDirectory,
		topologyConstructionStepSchedule,
	} {
		refusal := refuseProgramSeal(step)
		if !refusal.Available() || refusal.Stage() != ProgramSealStageProgramSeal || refusal.Step() != step {
			t.Fatalf("step %d does not close a program-seal refusal: %+v", step, refusal)
		}
		failure := refusal.Failure()
		if !failure.Available() || failure.Family != SolveFailureFamilyCompile || failure.Disposition != schema.DispositionMalformed {
			t.Fatalf("step %d publishes %+v, want a malformed compile-family failure", step, failure)
		}
		if failure.Site == bare.Site {
			t.Fatalf("step %d collapsed onto the bare program-seal stage", step)
		}
		if previous, duplicate := seen[failure.Site]; duplicate {
			t.Fatalf("steps %d and %d publish one boundary", previous, step)
		}
		seen[failure.Site] = step
		stage, named := ProgramSealStageOf(failure)
		if !named || stage != ProgramSealStageProgramSeal {
			t.Fatalf("step %d does not recover its stage: %d/%t", step, stage, named)
		}
	}
}

// TestAnInstallerLevelDisagreementSurfacesTheDirectoryStep pins WHICH step the
// installer path takes.
//
// The per-Factor execution family directory is what an installer is asked
// through, so an installer that refuses stops that directory and nothing
// earlier. A refusal there must arrive as the directory step, distinct from a
// malformed member row or a malformed query row, because those are the two
// things a reader would otherwise have to rule out by hand - which is exactly
// what a stale emitted table cost.
func TestAnInstallerLevelDisagreementSurfacesTheDirectoryStep(t *testing.T) {
	directory := refuseProgramSeal(topologyConstructionStepDirectory).Failure()
	member := refuseProgramSeal(topologyConstructionStepMemberRow).Failure()
	query := refuseProgramSeal(topologyConstructionStepQueryRow).Failure()
	if directory.Site == member.Site || directory.Site == query.Site {
		t.Fatal("the family directory is not distinguishable from a member or query row")
	}
	program := &runtimeProgram{
		memberTable:      []memberRow{{}},
		factorTable:      []factorRecord{{}},
		factorOwners:     []runtimeFactor{nil},
		programSealed:    true,
		generatedPresent: true,
	}
	// A Factor with no bound owner cannot answer the read side its rules join
	// through, which is the same directory boundary an installer refusal
	// stops, one step earlier in the same pass.
	if _, refusal, built := buildGeneratedExecutionProgram(program); built {
		t.Fatal("an execution program built over an unbound Factor owner")
	} else if !refusal.Available() || refusal.Stage() != ProgramSealStageProgramSeal {
		t.Fatalf("the execution build published %+v, want a program-seal refusal", refusal)
	} else if refusal.Failure().Site == ProgramStageFailure(ProgramSealStageProgramSeal).Site {
		t.Fatal("the execution build collapsed onto the bare program-seal stage")
	}
}

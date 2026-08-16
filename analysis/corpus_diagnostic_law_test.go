package analysis

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target/profile"
	"github.com/wippyai/go-lua/program/testfixture"
)

// Keep the first corpus fixture that exercises an unconditional branch in a
// direct receipt diagnostic lane, so an assembly regression reports its
// closed phase rather than only the census' public incomplete status.
func TestCorpusAlwaysTrueGuardReceiptLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusReceiptLaw(t, "advice/always-true-guard")
	result, report, status, diagnostics := solveAlwaysTrueGuardReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() {
		t.Fatalf("policy solve = %v result=%t report=%t identity=%v/%v diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), diagnostics)
	}
	if report.FindingCount() != 1 {
		t.Fatalf("always-true report count=%d failure=%d, want 1", report.FindingCount(), report.CollectionFailure())
	}
	if report.CollectionFailure() != DiagnosticCollectionOK {
		t.Fatalf("always-true report collection failure=%d, want %d", report.CollectionFailure(), DiagnosticCollectionOK)
	}
	finding, ok := report.FindingAt(0)
	location, locationOK := finding.Location()
	line, column := location.Start()
	if !ok || !locationOK || finding.Code() != DiagnosticCodeAlwaysTrueGuard || finding.Severity() != FindingSeverityHint || finding.Message() != "condition is proven always true" || finding.Help() != "Remove the guard or move the guarded code out of the branch." || location.File() != "main.lua" || line != 2 || column != 4 {
		t.Fatalf("finding is not exact always-true evidence: finding=%+v location=%+v", finding, location)
	}
	for index := 0; index < report.FindingCount(); index++ {
		candidate, candidateOK := report.FindingAt(index)
		candidateLocation, candidateLocationOK := candidate.Location()
		candidateLine, _ := candidateLocation.Start()
		if candidateOK && candidateLocationOK && candidateLocation.File() == "main.lua" && candidateLine == 9 {
			t.Fatal("ordinary boolean parameter at main.lua:9 received an always-true finding")
		}
	}
}

func TestCorpusAlwaysTrueGuardRedundantGuardMatrixLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusReceiptLaw(t, "advice/redundant-guard")
	result, report, status, diagnostics := solveAlwaysTrueGuardReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() {
		t.Fatalf("redundant-guard solve = %v result=%t report=%t identity=%v/%v diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), diagnostics)
	}
	assertAlwaysTrueGuardLocations(t, report, map[uint32]uint32{3: 8, 22: 8}, map[uint32]uint32{13: 8})
}

func TestCorpusAlwaysFalseGuardReceiptLaw(t *testing.T) {
	for _, test := range []struct {
		project string
		line    uint32
	}{
		{project: "native/truthy-false-literal-is-falsy", line: 5},
		{project: "native/branch-always-not-taken", line: 5},
	} {
		t.Run(test.project, func(t *testing.T) {
			plan, baseline, baselineDiagnostics, _ := testCorpusReceiptLaw(t, test.project)
			offResult, offReport, offStatus, offDiagnostics := plan.SolveWithReport(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256}, DiagnosticPolicy{})
			if offStatus != AnalyzeComplete || offResult == nil || offReport != nil || offResult.ContentID() != baseline.ContentID() || offDiagnostics.ObservationAttach != engine.ReceiptObservationAttachFailureNone || !reflect.DeepEqual(offDiagnostics.Engine, baselineDiagnostics.Engine) {
				t.Fatalf("policy-off false guard changed inference = status=%v result=%t report=%t identity=%v/%v diagnostics=%+v", offStatus, offResult != nil, offReport != nil, offResult.ContentID(), baseline.ContentID(), offDiagnostics)
			}
			result, report, status, diagnostics := solveGuardPolarityReport(plan)
			if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != DiagnosticCollectionOK || report.FindingCount() != 1 {
				t.Fatalf("false guard report = status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
			}
			finding, findingOK := report.FindingAt(0)
			location, locationOK := finding.Location()
			line, column := location.Start()
			if !findingOK || !locationOK || finding.Code() != DiagnosticCodeAlwaysFalseGuard || finding.Severity() != FindingSeverityHint || finding.Message() != "condition is proven always false" || finding.Help() != "Remove the unreachable branch or invert the guard." || location.File() != "main.lua" || line != test.line || column != 8 {
				t.Fatalf("false guard finding is not exact: finding=%+v location=%+v", finding, location)
			}
		})
	}
}

func TestCorpusAlwaysTrueGuardOptionalValueFenceLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusReceiptLaw(t, "native/truthy-optional-is-dynamic")
	result, report, status, diagnostics := solveGuardPolarityReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != DiagnosticCollectionOK || report.FindingCount() != 0 {
		t.Fatalf("optional-value report = status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
}

// Missing generic evidence is a report-collection failure, never an excuse to
// infer either guard polarity or to change the completed inference result.
func TestCorpusGuardPolarityMissingEvidenceSuppressesBothRulesLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusReceiptLaw(t, "native/truthy-false-literal-is-falsy")
	if plan == nil || plan.state == nil || plan.state.resultReceipt == nil {
		t.Fatal("false guard fixture did not retain its detached result receipt")
	}
	receipt := plan.state.resultReceipt
	original := receipt.pointObservations
	receipt.pointObservations = make(map[artifactResultPoint][]compiledObservation)
	t.Cleanup(func() { receipt.pointObservations = original })

	result, report, status, diagnostics := solveGuardPolarityReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != DiagnosticCollectionSubjectQueryAbsent || report.FindingCount() != 0 || diagnostics.Reason != AnalyzeDiagnosticReasonNone {
		t.Fatalf("missing guard evidence escaped as a polarity: status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
}

func TestCorpusAlwaysTrueGuardPolicyDisabledIdentityLaw(t *testing.T) {
	plan, baseline, baselineDiagnostics, _ := testCorpusReceiptLaw(t, "advice/always-true-guard")
	result, report, status, diagnostics := plan.SolveWithReport(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256}, DiagnosticPolicy{})
	if status != AnalyzeComplete || result == nil || report != nil || result.ContentID() != baseline.ContentID() || !reflect.DeepEqual(diagnostics.Engine, baselineDiagnostics.Engine) {
		t.Fatalf("disabled-policy solve = %v result=%t report=%t identity=%v/%v diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), diagnostics)
	}
}

// Type-reference absence is a ProgramArtifact static observation. This law
// proves its public report stays detached from the inference result and does
// not accidentally enable the separately installed adjacent value-reference
// collector.
func TestCorpusUnresolvedTypeReferenceStaticReceiptLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusReceiptLaw(t, "semantic/unresolved-reference-diagnostics-evidence-chain")
	offResult, offReport, offStatus, offDiagnostics := plan.SolveWithReport(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256}, DiagnosticPolicy{})
	if offStatus != AnalyzeComplete || offResult == nil || offReport != nil || offResult.ContentID() != baseline.ContentID() {
		t.Fatalf("disabled unresolved-type policy changed result = status=%v result=%t report=%t identity=%v/%v diagnostics=%+v", offStatus, offResult != nil, offReport != nil, offResult.ContentID(), baseline.ContentID(), offDiagnostics)
	}
	result, report, status, diagnostics := solveUnresolvedTypeReferenceReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != DiagnosticCollectionOK || report.FindingCount() != 1 {
		t.Fatalf("unresolved-type static report = status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
	finding, findingOK := report.FindingAt(0)
	location, locationOK := finding.Location()
	line, column := location.Start()
	if !findingOK || !locationOK || finding.Code() != DiagnosticCodeUnresolvedTypeReference || finding.Severity() != FindingSeverityError || location.File() != "main.lua" || line != 5 || column != 10 || finding.Message() != "unknown type LocalPoint" || finding.Help() != "Declare the type in scope" {
		t.Fatalf("unresolved-type finding is not exact: finding=%+v location=%+v", finding, location)
	}
	evidence, evidenceOK := finding.EvidenceAt(0)
	label, labelOK := finding.LabelAt(0)
	if !evidenceOK || evidence.Detail() != "no type named LocalPoint is declared in this scope" || !labelOK || label.Text() != "unknown type" {
		t.Fatalf("unresolved-type public evidence/label lost: evidence=%+v/%t label=%+v/%t", evidence, evidenceOK, label, labelOK)
	}
	rendered, renderOK := finding.RenderSource("main.lua", "if true then\n    type LocalPoint = {x: number}\nend\n\nlocal p: LocalPoint = {x = 1}\nlocal total = missing_count + 1\n")
	if !renderOK || rendered != "error[type.reference.unresolved]: unknown type LocalPoint\n--> main.lua:5:10\n5 | local p: LocalPoint = {x = 1}\nbecause:\n1. proven: no type named LocalPoint is declared in this scope\nhelp: Declare the type in scope" {
		t.Fatalf("unresolved-type render changed: %q/%t", rendered, renderOK)
	}
	for index := 0; index < report.FindingCount(); index++ {
		candidate, candidateOK := report.FindingAt(index)
		candidateLocation, candidateLocationOK := candidate.Location()
		candidateLine, _ := candidateLocation.Start()
		if candidateOK && candidateLocationOK && (candidate.Code() == DiagnosticCodeUnresolvedValueReference || candidateLine == 6) {
			t.Fatal("disabled value-reference row escaped the unresolved-type collector")
		}
	}
}

func TestCorpusUnresolvedValueReferenceStaticReceiptLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusReceiptLaw(t, "semantic/unresolved-reference-diagnostics-evidence-chain")
	result, report, status, diagnostics := solveUnresolvedValueReferenceReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != DiagnosticCollectionOK || report.FindingCount() != 1 {
		t.Fatalf("unresolved-value static report = status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
	finding, findingOK := report.FindingAt(0)
	location, locationOK := finding.Location()
	line, column := location.Start()
	if !findingOK || !locationOK || finding.Code() != DiagnosticCodeUnresolvedValueReference || finding.Severity() != FindingSeverityError ||
		location.File() != "main.lua" || line != 6 || column != 15 || finding.Message() != "unknown value missing_count" || finding.Help() != "Declare the value" {
		t.Fatalf("unresolved-value finding is not exact: finding=%+v location=%+v", finding, location)
	}
	evidence, evidenceOK := finding.EvidenceAt(0)
	label, labelOK := finding.LabelAt(0)
	if !evidenceOK || evidence.Detail() != "no value named missing_count is declared, predeclared, imported, or configured global in this scope" || !labelOK || label.Text() != "unknown value" {
		t.Fatalf("unresolved-value public evidence/label lost: evidence=%+v/%t label=%+v/%t", evidence, evidenceOK, label, labelOK)
	}
	rendered, renderOK := finding.RenderSource("main.lua", "if true then\n    type LocalPoint = {x: number}\nend\n\nlocal p: LocalPoint = {x = 1}\nlocal total = missing_count + 1\n")
	if !renderOK || rendered != "error[value.reference.unresolved]: unknown value missing_count\n--> main.lua:6:15\n6 | local total = missing_count + 1\nbecause:\n1. proven: no value named missing_count is declared, predeclared, imported, or configured global in this scope\nhelp: Declare the value" {
		t.Fatalf("unresolved-value render changed: %q/%t", rendered, renderOK)
	}
}

// Program owns the target-independent binder fact that an authored global
// read was implicit. Link alone knows whether its mounted Target supplies that
// name. A configured global must therefore remain in the reusable artifact
// while disappearing from the mount-qualified diagnostic receipt.
func TestConfiguredGlobalSuppressesProgramUnresolvedValueCandidateAtLinkLaw(t *testing.T) {
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, configured := contract.InitialBinding("require"); !configured {
		t.Fatal("canonical Target profile does not configure require")
	}
	linked := mustLink(t, "return require", contract)
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil || plan.state.resultReceipt == nil {
		t.Fatalf("configured-global compile = %v/%t diagnostics=%+v", status, plan != nil, diagnostics)
	}
	defer plan.Close()

	programCandidates := 0
	for _, mount := range plan.state.artifacts.mounts {
		for index := 0; index < mount.artifact.DiagnosticObservationCount(); index++ {
			row, rowOK := mount.artifact.DiagnosticObservationAt(index)
			payload, payloadOK := row.UnresolvedValueReference()
			name, nameOK := payload.Name()
			if rowOK && payloadOK {
				if !nameOK || name != "require" {
					t.Fatalf("configured-global Program candidate name = %q/%t, want require", name, nameOK)
				}
				programCandidates++
			}
		}
	}
	if programCandidates != 1 {
		t.Fatalf("configured-global Program candidates = %d, want 1", programCandidates)
	}
	for _, observation := range plan.state.resultReceipt.staticObservations {
		if observation.kind == programartifact.DiagnosticObservationValueReferenceUnresolved {
			t.Fatal("configured global escaped Link absence filtering")
		}
	}

	result, report, solveStatus, solveDiagnostics := solveUnresolvedValueReferenceReport(plan)
	if solveStatus != AnalyzeComplete || result == nil || report == nil || report.CollectionFailure() != DiagnosticCollectionOK || report.FindingCount() != 0 {
		t.Fatalf("configured-global diagnostic solve = %v result=%t report=%t failure=%d findings=%d diagnostics=%+v", solveStatus, result != nil, report != nil, report.CollectionFailure(), report.FindingCount(), solveDiagnostics)
	}
}

func TestCorpusUnresolvedTypeReferenceDisabledStaticKindLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusReceiptLaw(t, "semantic/unresolved-reference-diagnostics-evidence-chain")
	result, report, status, diagnostics := solveAlwaysTrueGuardReport(plan)
	reportAvailable, collectionFailure := report != nil && report.Available(), DiagnosticCollectionSubjectQueryAbsent
	if report != nil {
		collectionFailure = report.CollectionFailure()
	}
	if status != AnalyzeComplete || result == nil || report == nil || !reportAvailable || collectionFailure != DiagnosticCollectionOK || result.ContentID() != baseline.ContentID() {
		t.Fatalf("disabled static collector changed always-true report = status=%v result=%t report=%t available=%t failure=%d identity=%v/%v diagnostics=%+v", status, result != nil, report != nil, reportAvailable, collectionFailure, result.ContentID(), baseline.ContentID(), diagnostics)
	}
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		if findingOK && finding.Code() == DiagnosticCodeUnresolvedTypeReference {
			t.Fatal("known static type row emitted while its policy was disabled")
		}
	}
}

func TestCorpusAlwaysTrueGuardDuplicateMountReportLaw(t *testing.T) {
	shared := planLawProgram(t, `if true then
	return 1
end
return 0`)
	linked := planLawMountedLink(t, []linkproject.Module{{Name: "left", Program: shared}, {Name: "right", Program: shared}})
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("duplicate diagnostic compile = %v/%v diagnostics=%+v", status, plan, diagnostics)
	}
	artifacts := plan.state.artifacts
	if len(artifacts.mounts) != 2 || len(artifacts.byProgram) != 1 ||
		artifacts.mounts[0].artifact != artifacts.mounts[1].artifact ||
		artifacts.mounts[0].moduleKey == artifacts.mounts[1].moduleKey {
		t.Fatal("duplicate diagnostic mounts did not reuse one artifact with distinct substitutions")
	}
	baseline, baselineStatus, baselineDiagnostics := plan.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256})
	result, report, solveStatus, solveDiagnostics := solveAlwaysTrueGuardReport(plan)
	bodyCount, findingCount, collectionFailure := 0, 0, DiagnosticCollectionSubjectQueryAbsent
	if result != nil {
		bodyCount = result.BodyCount()
	}
	if report != nil {
		findingCount, collectionFailure = report.FindingCount(), report.CollectionFailure()
	}
	wantBodies := 2 * artifacts.mounts[0].artifact.BodyCount()
	if baselineStatus != AnalyzeComplete || baseline == nil || solveStatus != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || result.BodyCount() != wantBodies || report.CollectionFailure() != DiagnosticCollectionOK || report.FindingCount() != 2 {
		t.Fatalf("duplicate diagnostic solve = baseline:%v/%t report:%v/%t bodies=%d findings=%d failure=%d baseline-diagnostics=%+v diagnostics=%+v", baselineStatus, baseline != nil, solveStatus, report != nil, bodyCount, findingCount, collectionFailure, baselineDiagnostics, solveDiagnostics)
	}
	ids, subjects := make(map[[32]byte]struct{}, 2), make(map[[32]byte]struct{}, 2)
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		id, idOK := finding.ID()
		subject, subjectOK := finding.SubjectID()
		location, locationOK := finding.Location()
		line, _ := location.Start()
		if !findingOK || !idOK || !subjectOK || !locationOK || location.File() != "plan-law.lua" || line != 1 {
			t.Fatalf("duplicate mount finding[%d] lost exact mounted identity/location", index)
		}
		ids[id] = struct{}{}
		subjects[subject] = struct{}{}
	}
	if len(ids) != 2 || len(subjects) != 2 {
		t.Fatalf("duplicate mounts collapsed finding/subject identity: findings=%d subjects=%d", len(ids), len(subjects))
	}
}

func solveAlwaysTrueGuardReport(plan *Plan) (*Result, *DiagnosticReport, AnalyzeStatus, AnalyzeDiagnostics) {
	return plan.SolveWithReport(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256}, DiagnosticPolicy{Enabled: []DiagnosticRule{DiagnosticRuleAlwaysTrueGuard}})
}

func solveGuardPolarityReport(plan *Plan) (*Result, *DiagnosticReport, AnalyzeStatus, AnalyzeDiagnostics) {
	return plan.SolveWithReport(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256}, DiagnosticPolicy{Enabled: []DiagnosticRule{DiagnosticRuleAlwaysTrueGuard, DiagnosticRuleAlwaysFalseGuard}})
}

func solveUnresolvedTypeReferenceReport(plan *Plan) (*Result, *DiagnosticReport, AnalyzeStatus, AnalyzeDiagnostics) {
	return plan.SolveWithReport(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256}, DiagnosticPolicy{Enabled: []DiagnosticRule{DiagnosticRuleUnresolvedTypeReference}})
}

func solveUnresolvedValueReferenceReport(plan *Plan) (*Result, *DiagnosticReport, AnalyzeStatus, AnalyzeDiagnostics) {
	return plan.SolveWithReport(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256}, DiagnosticPolicy{Enabled: []DiagnosticRule{DiagnosticRuleUnresolvedValueReference}})
}

func assertAlwaysTrueGuardLocations(t *testing.T, report *DiagnosticReport, expected, forbidden map[uint32]uint32) {
	t.Helper()
	if report == nil || report.CollectionFailure() != DiagnosticCollectionOK || report.FindingCount() != len(expected) {
		t.Fatalf("always-true matrix = report=%t failure=%d findings=%d, want %d", report != nil, report.CollectionFailure(), report.FindingCount(), len(expected))
	}
	found := make(map[uint32]uint32, report.FindingCount())
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		location, locationOK := finding.Location()
		line, column := location.Start()
		if !findingOK || !locationOK || finding.Code() != DiagnosticCodeAlwaysTrueGuard || finding.Severity() != FindingSeverityHint || location.File() != "main.lua" {
			t.Fatalf("finding[%d] is not an exact always-true row: finding=%+v location=%+v", index, finding, location)
		}
		if _, duplicate := found[line]; duplicate {
			t.Fatalf("duplicate always-true finding at main.lua:%d", line)
		}
		found[line] = column
	}
	for line, column := range expected {
		if found[line] != column {
			t.Fatalf("missing always-true finding at main.lua:%d:%d; got %v", line, column, found)
		}
	}
	for line, column := range forbidden {
		if found[line] == column {
			t.Fatalf("forbidden always-true finding at main.lua:%d:%d", line, column)
		}
	}
}

func TestCorpusAnnotationArrayInferredReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "core/annotation-array-inferred")
}

func TestCorpusControlForIPairsReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "core/control-for-ipairs")
}

func TestCorpusNativeRecordNestedParentChildEdgesReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "native/record-nested-parent-child-edges")
}

func TestCorpusNativeSendSafeFreshIsolatedMoveReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "native/sendsafe-fresh-isolated-move")
}

func TestCorpusNativeAliasDistinctFreshAllocationsReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "native/alias-distinct-fresh-allocations")
}

func TestCorpusNativeCaptureEnvReentrantBeginReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "native/capture-env-reentrant-begin")
}

func TestCorpusNativePublicationMonotonicityAddedUseReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "native/publication-monotonicity-added-use")
}

func TestCorpusNativeEffectPureCalleeNoSafepointReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "native/effect-pure-callee-no-safepoint")
}

func TestCorpusNativeTostringNumericRebindAwareReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "native/tostring-numeric-rebind-aware")
}

func TestCorpusCoreControlRepeatUntilReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "core/control-repeat-until")
}

func TestCorpusCoreTonumberSkipsOptionalReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "core/tonumber-skips-optional")
}

func TestCorpusRegressionGenericNestedInstantiateReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "regression/generic-nested-instantiate")
}

func TestCorpusRegressionGradualTypingAdversarialReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "regression/gradual-typing-adversarial")
}

func TestCorpusRegressionDeadlockCompilerLuaReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "regression/deadlock-compiler-lua")
}

func TestCorpusSoundnessRecurrenceExitArmReceiptLaw(t *testing.T) {
	testCorpusReceiptLaw(t, "soundness/recurrence-exit-arm")
}

func testCorpusReceiptLaw(t *testing.T, name string) (*Plan, *Result, AnalyzeDiagnostics, *link.Link) {
	t.Helper()
	project, err := testfixture.FrozenCorpusProject(name)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatal(err)
	}
	plan, compiled, compileDiagnostics := CompileWithDiagnostics(linked)
	if compiled != CompileComplete || plan == nil {
		t.Fatalf("compile=%v artifact=%s heap=%s schedule=%s diagnostics=%+v", compiled, planLawArtifactFailure(linked), planLawHeapSealFailure(linked), planLawArtifactScheduleRow(linked, compileDiagnostics.ReceiptScheduleOrdinal), compileDiagnostics)
	}
	result, status, diagnostics := plan.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256})
	if status != AnalyzeComplete || result == nil {
		t.Fatalf("solve=%v result=%v binding=%s diagnostics=%+v", status, result != nil, corpusAlwaysTrueGuardBindingFailure(plan, linked), diagnostics)
	}
	return plan, result, diagnostics, linked
}

func corpusAlwaysTrueGuardBindingFailure(plan *Plan, source *link.Link) string {
	if plan == nil || plan.state == nil || source == nil || source.ContentID() != plan.state.sourceID || plan.state.artifacts == nil {
		return "state"
	}
	binding, bindingFailure, valueFailure, allocationFailure := plan.state.newProgramBinding(source)
	if bindingFailure != ProgramBindingFailureNone {
		return "binding:" + bindingFailure.String() + ":" + valueFailure.String() + ":" + allocationFailure.String()
	}
	if failure := corpusAlwaysTrueGuardEffectBodyReceiptFailure(plan, binding); failure != "" {
		return "effect-body:" + failure
	}
	return "complete"
}

func corpusAlwaysTrueGuardEffectBodyReceiptFailure(plan *Plan, binding *programBinding) string {
	if plan == nil || plan.state == nil || plan.state.artifacts == nil || binding == nil || binding.effectBody == nil {
		return "state"
	}
	valueIDs, heapIDs, witness, witnessOK := linkBootstrapWitness(plan.state, binding)
	if !witnessOK {
		return "bootstrap"
	}
	mounts := make([]engine.MountedArtifactReceipt, 0, len(plan.state.artifacts.mounts))
	for _, mount := range plan.state.artifacts.mounts {
		receipt, receiptOK := newEngineArtifactScalarReceipt(mount.template, mount.roles, binding)
		if !receiptOK {
			return "receipt"
		}
		mounted, mountedOK := engine.NewMountedArtifactReceipt(receipt, mount.moduleKey)
		if !mountedOK {
			return "mount"
		}
		mounts = append(mounts, mounted)
	}
	assembly, _, assemblyOK := engine.BeginMountedArtifactReceiptAssemblyWithFailure(binding.binding, mounts, witness)
	if !assemblyOK {
		return "assembly"
	}
	defer assembly.Abort()
	if !binding.attachLinkBootstrapRules(assembly, valueIDs, heapIDs) {
		return "bootstrap-rules"
	}
	for _, mount := range plan.state.artifacts.mounts {
		issuer, issuerOK := binding.effectBody.ForMount(mount.moduleKey)
		if !issuerOK {
			return "issuer"
		}
		for index := 0; index < mount.artifact.RuleOccurrenceCount(programartifact.RuleRoleEffectBody); index++ {
			row, rowOK := mount.artifact.RuleOccurrenceAt(programartifact.RuleRoleEffectBody, index)
			if !rowOK {
				return "artifact-row"
			}
			if _, receiptOK := issuer.ReceiptForOccurrence(row.ID()); !receiptOK {
				return "occurrence"
			}
			for pointIndex := 0; pointIndex < row.PointCount(); pointIndex++ {
				point, pointOK := row.PointAt(pointIndex)
				if !pointOK {
					return "point"
				}
				if _, failure := binding.effectBody.AttachMountedOccurrenceWithFailure(assembly, mount.moduleKey, point, row.ID()); failure != 0 {
					return "attach-" + failure.String()
				}
			}
		}
	}
	if !assembly.SealSources() {
		return "seal-" + binding.effectBody.FinalizationFailure().String()
	}
	return ""
}

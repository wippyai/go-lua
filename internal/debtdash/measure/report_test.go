package measure

import "testing"

// TestMeasureFixture exercises Measure against a synthetic testdata tree
// (testdata/fixture), never the live repository, so the expected numbers
// are exact and independent of anything else changing in this codebase.
func TestMeasureFixture(t *testing.T) {
	report, err := Measure("testdata/fixture")
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}

	wantAreas := []AreaLOC{
		{Name: "areaa", LOC: LOC{NonTest: 43, Test: 14}},
		{Name: "areab", LOC: LOC{NonTest: 16, Test: 5}},
		{Name: "composite", LOC: LOC{NonTest: 9, Test: 0}},
	}
	if len(report.DomainAreas) != len(wantAreas) {
		t.Fatalf("DomainAreas = %+v, want %+v", report.DomainAreas, wantAreas)
	}
	for i, want := range wantAreas {
		if report.DomainAreas[i] != want {
			t.Errorf("DomainAreas[%d] = %+v, want %+v", i, report.DomainAreas[i], want)
		}
	}

	wantDomainTotal := LOC{NonTest: 68, Test: 19}
	if report.DomainTotal != wantDomainTotal {
		t.Errorf("DomainTotal = %+v, want %+v", report.DomainTotal, wantDomainTotal)
	}

	if want := (LOC{NonTest: 22, Test: 0}); report.EngineLOC != want {
		t.Errorf("EngineLOC = %+v, want %+v", report.EngineLOC, want)
	}
	if want := (LOC{NonTest: 19, Test: 0}); report.SchemaLOC != want {
		t.Errorf("SchemaLOC = %+v, want %+v", report.SchemaLOC, want)
	}
	if want := (LOC{NonTest: 5, Test: 5}); report.AnalysisRest != want {
		t.Errorf("AnalysisRest = %+v, want %+v", report.AnalysisRest, want)
	}
	if want := (LOC{NonTest: 5, Test: 0}); report.InternalLOC != want {
		t.Errorf("InternalLOC = %+v, want %+v", report.InternalLOC, want)
	}
	if want := (LOC{NonTest: 3, Test: 0}); report.CmdLOC != want {
		t.Errorf("CmdLOC = %+v, want %+v", report.CmdLOC, want)
	}

	if report.GeneratedFiles != 4 {
		t.Errorf("GeneratedFiles = %d, want 4", report.GeneratedFiles)
	}
	if report.GeneratedLOC != 20 {
		t.Errorf("GeneratedLOC = %d, want 20", report.GeneratedLOC)
	}

	if report.ResidueFiles != 4 {
		t.Errorf("ResidueFiles = %d, want 4", report.ResidueFiles)
	}
	if report.ResidueOccurrences != 6 {
		t.Errorf("ResidueOccurrences = %d, want 6", report.ResidueOccurrences)
	}

	if report.FamilyFiles != 2 {
		t.Errorf("FamilyFiles = %d, want 2", report.FamilyFiles)
	}
	if report.HotRuleFiles != 1 {
		t.Errorf("HotRuleFiles = %d, want 1", report.HotRuleFiles)
	}
	if report.RegistrationFiles != 1 {
		t.Errorf("RegistrationFiles = %d, want 1", report.RegistrationFiles)
	}
	if report.SchemaFragmentFiles != 1 {
		t.Errorf("SchemaFragmentFiles = %d, want 1", report.SchemaFragmentFiles)
	}

	if report.ScheduledDeathRows != 4 {
		t.Errorf("ScheduledDeathRows = %d, want 4", report.ScheduledDeathRows)
	}

	if report.RuleTemplatesGenerated != 2 {
		t.Errorf("RuleTemplatesGenerated = %d, want 2", report.RuleTemplatesGenerated)
	}
	if report.RuleTemplatesLegacy != 3 {
		t.Errorf("RuleTemplatesLegacy = %d, want 3", report.RuleTemplatesLegacy)
	}

	if report.EmittedDomainFiles != 2 {
		t.Errorf("EmittedDomainFiles = %d, want 2", report.EmittedDomainFiles)
	}

	if report.TotalTestFuncs != 5 {
		t.Errorf("TotalTestFuncs = %d, want 5", report.TotalTestFuncs)
	}
	if report.LawTestFuncs != 2 {
		t.Errorf("LawTestFuncs = %d, want 2", report.LawTestFuncs)
	}
	if report.LawTestFiles != 1 {
		t.Errorf("LawTestFiles = %d, want 1", report.LawTestFiles)
	}

	if report.ExportedEngine != 4 {
		t.Errorf("ExportedEngine = %d, want 4", report.ExportedEngine)
	}
	if report.ExportedSchema != 2 {
		t.Errorf("ExportedSchema = %d, want 2", report.ExportedSchema)
	}
}

// TestMeasureMissingRoot confirms a worktree missing some of the areas
// debt-dashboard measures (an older commit, or a partial fixture) is
// treated as zero for that area rather than an error.
func TestMeasureMissingRoot(t *testing.T) {
	report, err := Measure("testdata/does-not-exist")
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	if len(report.DomainAreas) != 0 {
		t.Errorf("DomainAreas = %+v, want empty", report.DomainAreas)
	}
	if report.DomainTotal != (LOC{}) {
		t.Errorf("DomainTotal = %+v, want zero", report.DomainTotal)
	}
	if report.ExportedEngine != 0 || report.ExportedSchema != 0 {
		t.Errorf("ExportedEngine/Schema = %d/%d, want 0/0", report.ExportedEngine, report.ExportedSchema)
	}
}

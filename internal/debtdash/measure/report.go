package measure

import "path/filepath"

// Report is one commit's debt-dashboard measurement.
type Report struct {
	DomainAreas []AreaLOC
	DomainTotal LOC

	EngineLOC    LOC
	SchemaLOC    LOC
	AnalysisRest LOC
	InternalLOC  LOC
	CmdLOC       LOC

	GeneratedFiles int
	GeneratedLOC   int

	ResidueFiles       int
	ResidueOccurrences int

	FamilyFiles         int
	HotRuleFiles        int
	RegistrationFiles   int
	SchemaFragmentFiles int

	ScheduledDeathRows int

	RuleTemplatesGenerated int
	RuleTemplatesLegacy    int

	EmittedDomainFiles int

	TotalTestFuncs int
	LawTestFuncs   int
	LawTestFiles   int

	ExportedEngine int
	ExportedSchema int
}

// Measure walks root - a checked-out worktree path, real or a testdata
// fixture - and computes every debt-dashboard metric. It performs no git
// operations and runs no build or test commands.
func Measure(root string) (Report, error) {
	domainDir := filepath.Join(root, "domain")
	analysisDir := filepath.Join(root, "analysis")
	engineDir := filepath.Join(analysisDir, "engine")
	schemaDir := filepath.Join(analysisDir, "schema")
	internalDir := filepath.Join(root, "internal")
	cmdDir := filepath.Join(root, "cmd")

	var (
		r   Report
		err error
	)

	r.DomainAreas, r.DomainTotal, err = domainAreas(domainDir)
	if err != nil {
		return Report{}, err
	}

	r.EngineLOC, err = locInDir(engineDir)
	if err != nil {
		return Report{}, err
	}
	r.SchemaLOC, err = locInDir(schemaDir)
	if err != nil {
		return Report{}, err
	}
	analysisTotal, err := locInDir(analysisDir)
	if err != nil {
		return Report{}, err
	}
	r.AnalysisRest = analysisTotal.Sub(r.EngineLOC).Sub(r.SchemaLOC)

	r.InternalLOC, err = locInDir(internalDir)
	if err != nil {
		return Report{}, err
	}
	r.CmdLOC, err = locInDir(cmdDir)
	if err != nil {
		return Report{}, err
	}

	r.GeneratedFiles, r.GeneratedLOC, err = generatedStats([]string{domainDir, analysisDir})
	if err != nil {
		return Report{}, err
	}

	r.ResidueFiles, r.ResidueOccurrences, err = residueStats(domainDir)
	if err != nil {
		return Report{}, err
	}

	r.FamilyFiles, err = countFilesByName(domainDir, "family.go")
	if err != nil {
		return Report{}, err
	}
	r.HotRuleFiles, err = countFilesByName(domainDir, "hot_rule.go")
	if err != nil {
		return Report{}, err
	}
	r.RegistrationFiles, err = countFilesByName(domainDir, "registration.go")
	if err != nil {
		return Report{}, err
	}
	r.SchemaFragmentFiles, err = countFilesByName(domainDir, "schema_fragment.go")
	if err != nil {
		return Report{}, err
	}

	r.ScheduledDeathRows, err = scheduledDeathRows(root)
	if err != nil {
		return Report{}, err
	}

	r.RuleTemplatesGenerated, r.RuleTemplatesLegacy, err = ruleTemplatesStats(domainDir)
	if err != nil {
		return Report{}, err
	}

	r.EmittedDomainFiles, err = emittedContentFiles(domainDir)
	if err != nil {
		return Report{}, err
	}

	r.TotalTestFuncs, r.LawTestFuncs, err = testFuncCounts(root)
	if err != nil {
		return Report{}, err
	}
	r.LawTestFiles, err = countFilesWithSuffix(root, "_law_test.go")
	if err != nil {
		return Report{}, err
	}

	r.ExportedEngine, err = exportedSymbolCount(engineDir)
	if err != nil {
		return Report{}, err
	}
	r.ExportedSchema, err = exportedSymbolCount(schemaDir)
	if err != nil {
		return Report{}, err
	}

	return r, nil
}

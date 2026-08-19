// construction_shape_law_test.go is the shape law that gates the constructor
// migration. It enumerates the dying construction design by signature class,
// never by name, and it counts down: every violation the tree carries today is
// pinned, the pinned sets are shrink-only, and each clause flips to its
// absolute form once its pins are empty.
//
// The clauses are:
//
//	(a) mutating-admission ban - no engine method hands a caller a mutation of
//	    construction state behind a decision;
//	(b) sealed-input mint - the sole Solver mint takes sealed values only;
//	(c) no exported builder parameter - code outside the engine cannot hold or
//	    drive a construction handle;
//	(d) root-one-call - the analysis root reaches engine construction through
//	    exactly one call whose parameter types are sealed by (b)'s criterion.
//
// A clause is complete when its pin list is empty. The law is complete when
// all four are: at that point (a) and (c) assert zero, (d) asserts the single
// sealed entry, and every symbol in dyingConstructionSymbols is gone.
package engine

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"
)

// constructionStep names the migration step that removes a pin:
//
//	1 - the sealed construction input value the root hands the engine;
//	2 - the single engine construction entry that consumes it;
//	3 - deletion of the builder and attach spine named in
//	    dyingConstructionSymbols;
//	4 - this law in its absolute form, which carries no pin.
type constructionStep int

const (
	stepSealedInput   constructionStep = 1
	stepSingleEntry   constructionStep = 2
	stepSpineDeletion constructionStep = 3
)

// constructionPin is one violation the tree carries today. symbol is the
// declaration that violates the clause, via the construction handle it reaches
// through, count how many times the site repeats, and step the migration step
// that removes it.
type constructionPin struct {
	symbol string
	via    string
	count  int
	step   constructionStep
}

// observation is one violation the law measured.
type observation struct {
	file   string
	symbol string
	via    string
	count  int
}

func observationKey(symbol, via string) string { return symbol + " via " + via }

func (pin constructionPin) key() string { return observationKey(pin.symbol, pin.via) }

func (found observation) key() string { return observationKey(found.symbol, found.via) }

// constructionMutableAllowlist names engine types that match the structural
// construction-state test but are not state of the program construction this
// migration replaces. The list is shrink-only and every entry carries the
// reason the type sits off the construction path.
var constructionMutableAllowlist = map[string]string{
	"Activation":                    "solve-time activation row, written while a revision runs",
	"ClosedRefs":                    "rule-surface refs a declaration builds and seals inside one surface placement",
	"RuleSourceTransaction":         "scoped surface-placement transaction: it lives inside one admit call and is never retained",
	"SchemaBuilder":                 "schema declaration plane, sealed before any program construction opens",
	"Selection":                     "solve-time selection cursor",
	"SolveReport":                   "solve-time diagnostic accumulator",
	"Solver":                        "the sealed product of construction; solving writes it, building does not",
	"bindingRuleRow":                "builder-local row value; the builder methods that accumulate it are what clause (a) counts",
	"bindingRuleRowDraft":           "builder-local row draft; the builder methods that accumulate it are what clause (a) counts",
	"boundFactor":                   "assembled runtime factor row, written while a revision runs",
	"boundRule":                     "assembled runtime rule row, written while a revision runs",
	"compiledActivationRule":        "assembled runtime activation row, written while a revision runs",
	"executorEpoch":                 "solve-time epoch",
	"keyRow":                        "sealed directory row",
	"orderedCellsRecord":            "sealed schema cell index",
	"pointQueue":                    "solve-time work queue",
	"preparedSelectedFactorOverlay": "solve-time overlay staged for one epoch",
	"producerEpoch":                 "solve-time epoch",
	"productSession":                "solve-time product session",
	"regionEpoch":                   "solve-time epoch",
	"ruleAdmissionTicket":           "one admission's scoped ticket, issued and consumed inside a single admit call",
	"ruleRow":                       "builder-local row value; the builder methods that accumulate it are what clause (a) counts",
	"runtimeBinding":                "assembled runtime binding, written while a revision runs",
	"schemaBindingState":            "schema declaration plane, sealed before any program construction opens",
	"schemaCompletionDraft":         "schema declaration draft",
	"schemaFactorBindingCell":       "schema declaration cell",
	"schemaFormDraft":               "schema declaration draft",
	"schemaQueryDraft":              "schema declaration draft",
	"schemaRuleDraft":               "schema declaration draft",
	"schemaTokenCell":               "schema declaration cell",
	"semanticDirectory":             "sealed semantic directory the construction reads",
	"solveBoundary":                 "solve-time boundary",
	"solveDiagnosticRestartSample":  "solve-time diagnostic sample",
	"solveDiagnosticState":          "solve-time diagnostic state",
	"solvedPublication":             "solve-time publication row",
	"solverRuntime":                 "the assembled runtime the mint installs: solving writes it, construction hands it over sealed",
	"stagedRouteSink":               "solve-time route sink",
	"typedOutput":                   "solve-time typed output cursor",
	"typedReadSession":              "solve-time read session",
	"typedStagedSelectionSession":   "solve-time staged selection session",
}

// sealedMintParameterTypes is the closed allowlist of named types the sole
// Solver mint, and the root's single construction entry, may take. The law
// checks each listed engine type really is sealed: no exported method of it
// writes its own state.
var sealedMintParameterTypes = map[string]string{
	"solverRuntime": "the assembled runtime the mint installs and never mutates",
}

// dyingConstructionSymbols is the step-3 deletion checklist, snapshotted by
// name so that a rename between now and the cut cannot hide a symbol. A method
// is named as Receiver.Method, so that renaming either half is a
// disappearance. present records whether the tree declares the symbol at
// snapshot time.
//
// While any pin remains, a present symbol that disappears is a rename, not a
// removal, and the law fails on it. An absent symbol must stay absent. When
// the pins are empty every symbol here must be gone.
type dyingConstructionSymbol struct {
	name    string
	present bool
	note    string
}

var dyingConstructionSymbols = []dyingConstructionSymbol{
	{name: "BindingTopologyBuilder", present: true, note: "the builder itself"},
	{name: "BindingTopology", present: true, note: "the topology receipt the builder commits"},
	{name: "RuleProgramAttach", present: true, note: "the erased construction join domains implement"},
	{name: "RuleImplementation.AdmitMounted", present: false, note: "carried by the declared-issuance cut"},
	{name: "RuleImplementation.AdmitLink", present: false, note: "carried by the declared-issuance cut"},
	{name: "RuleProgramAttach.AdmitMounted", present: false, note: "carried by the declared-issuance cut"},
	{name: "RuleProgramAttach.AdmitLink", present: false, note: "carried by the declared-issuance cut"},
	{name: "AssembleMountedProgram", present: true, note: "the mounted program assembly entry"},
	{name: "BeginProgramConstruction", present: true, note: "the first-construction ledger entry"},
	{name: "ProgramConstruction", present: true, note: "the attach ledger handle"},
	{name: "queryReceipts", present: true, note: "the composite query receipt pass"},
	{name: "artifactReceiptTopology", present: false, note: "already carried by the receipt cut"},
	{name: "artifactMountReceipt", present: false, note: "already carried by the receipt cut"},
	{name: "linkBootstrapReceipt", present: false, note: "already carried by the receipt cut"},
	{name: "BindingTopologyBuilder.Abort", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.AddActivationRule", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.AddActivationRuleFromDraft", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.AddRule", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.AddRuleFromDraft", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.AdmitLinkRuleOccurrence", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.AdmitMountedRuleOccurrence", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.AdmitMountedRuleOperand", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.Commit", present: false, note: "carried by the constructor cut"},
	{name: "BindingTopologyBuilder.CommitFailure", present: false, note: "carried by the constructor cut"},
	{name: "BindingTopologyBuilder.CommitObservationTopology", present: false, note: "carried by the constructor cut"},
	{name: "BindingTopologyBuilder.IssueRuleSourceWithSurfaces", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.IssueRuleSourceWithSurfacesWithFailure", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.QueueLinkRuleFinalizer", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.QueueMountedQueryBatch", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.QueueMountedRuleFinalizer", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.SealFailure", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.SealSources", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.abort", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addActivationCandidate", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addDirectActivationCandidate", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addEnvironmentEdge", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addFactorEdge", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addPoint", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addRow", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addSemanticActivation", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addSemanticPoint", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addSemanticQuery", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addSemanticRule", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.addSummary", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.admitAt", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.admitFrom", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.admitOperand", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.admitSite", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.claimMountedSelectedSurface", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.claimMountedSurface", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.commit", present: false, note: "carried by the constructor cut"},
	{name: "BindingTopologyBuilder.currentRuleFinalizerFailure", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.currentRuleSourceSealFailure", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.drainQueryBatches", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.drainRuleFinalizers", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.issueDirectActivationCandidate", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.issueEnvironmentEdge", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.issueFactorEdge", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.issueMaterialization", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.issuePointRow", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.issueQueryRow", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.issueRuleRow", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.issueRuleSurfaceSource", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.lockPhase", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.lockSourcesOpen", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.lockTopologyOpen", present: true, note: "builder method"},
	{name: "BindingTopologyBuilder.lowerArtifactRows", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.queueRuleFinalizer", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.recordRuleFinalizerFailure", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.recordRuleSourceSealFailure", present: false, note: "carried by the declared-issuance cut"},
	{name: "BindingTopologyBuilder.sealSources", present: true, note: "builder method"},
}

// dyingSymbolSearchRoots are the trees the existence check reads, relative to
// the repository root.
var dyingSymbolSearchRoots = []string{"analysis/engine/", "domain/composite/"}

// rootConstructionFiles are the analysis root files clause (d) reads.
var rootConstructionFiles = []string{"analysis/analyze.go", "analysis/compile.go"}

// mutatingAdmissionPins is clause (a)'s countdown.
var mutatingAdmissionPins = []constructionPin{
	{symbol: "BindingTopology.releaseArtifact", via: "BindingTopology", count: 1, step: stepSpineDeletion},
	{symbol: "ExactQueryImplementation.bindConstruction", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "ProgramConstruction.Close", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "ProgramMember.ownedBy", via: "BindingTopology", count: 1, step: stepSpineDeletion},
	{symbol: "RuleImplementation.AttachLinkMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "RuleImplementation.AttachMountedMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "SummaryQueryImplementation.bindConstruction", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
}

// exportedBuilderParamPins is clause (c)'s countdown.
var exportedBuilderParamPins = []constructionPin{
	{symbol: "AttachActivationMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachExactQuery", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachLinkRuleMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachMountedActivationMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachMountedExact", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachMountedExactObservation", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachMountedRuleMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachMountedSummary", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachMountedSummaryObservationWithFailure", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachRuleExactObservation", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachRuleExactObservationWithFailure", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachRuleMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachRuleSummaryObservationWithFailure", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "AttachSummaryQuery", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "BindingTopology.Graph", via: "BindingTopology", count: 1, step: stepSpineDeletion},
	{symbol: "CommittedProgramFrom", via: "BindingTopology", count: 1, step: stepSpineDeletion},
	{symbol: "ProgramConstruction.Close", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "ProgramConstruction.HasMountedRuleMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "ProgramConstruction.MountedCallStage", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "ProgramConstruction.QueryPublicationKey", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "ProgramConstruction.Seal", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "RuleImplementation.AttachLinkMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "RuleImplementation.AttachMountedMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "RuleProgramAttach.AttachLinkMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
	{symbol: "RuleProgramAttach.AttachMountedMember", via: "ProgramConstruction", count: 1, step: stepSingleEntry},
}

// rootConstructionCallPins is clause (d)'s countdown. Every call the root
// makes into engine construction beyond the single sealed entry is pinned
// here; the clause completes when one sealed call is all that is left.
var rootConstructionCallPins = []constructionPin{
	{symbol: "assembleCommittedProgram -> engine.AssembleMountedProgram", via: "analysis/analyze.go", count: 1, step: stepSingleEntry},
	{symbol: "beginRuntimeConstruction -> engine.BeginProgramConstruction", via: "analysis/analyze.go", count: 1, step: stepSingleEntry},
	{symbol: "buildRuntimeSolver -> anadiag.AttachBranchValues", via: "analysis/analyze.go", count: 1, step: stepSingleEntry},
	{symbol: "buildRuntimeSolver -> binding.AttachQueries", via: "analysis/analyze.go", count: 1, step: stepSingleEntry},
	{symbol: "buildRuntimeSolver -> compilation.Close", via: "analysis/analyze.go", count: 3, step: stepSingleEntry},
	{symbol: "buildRuntimeSolver -> compilation.Seal", via: "analysis/analyze.go", count: 1, step: stepSingleEntry},
	{symbol: "buildRuntimeSolver -> rules.AttachLinkMembers", via: "analysis/analyze.go", count: 1, step: stepSingleEntry},
	{symbol: "buildRuntimeSolver -> rules.AttachMountedMembers", via: "analysis/analyze.go", count: 1, step: stepSingleEntry},
}

// TestConstructionMutatingAdmissionBan is clause (a). A mutating admission is
// a method that answers bool or (T, bool) and either takes somebody else's
// construction handle or writes its own receiver's construction state. Both
// hand a caller a mutation behind a decision, which is the shape a sealed
// input replaces.
func TestConstructionMutatingAdmissionBan(t *testing.T) {
	shape := loadConstructionShape(t)
	t.Logf("construction-mutable engine types: %s", strings.Join(shape.mutableNames(), ", "))
	t.Logf("builder-driving engine interfaces: %s", strings.Join(shape.drivingNames(), ", "))
	enforceCountdown(t, "(a) mutating admission", mutatingAdmissionPins, shape.mutatingAdmissions())
}

// TestSealedSolverMintTakesSealedInputs is clause (b). The engine mints a
// Solver in exactly one place and that mint takes sealed values: types the
// allowlist names and whose exported methods write nothing.
func TestSealedSolverMintTakesSealedInputs(t *testing.T) {
	shape := loadConstructionShape(t)
	mints := shape.solverMints()
	if len(mints) != 1 {
		t.Fatalf("the engine must mint a Solver in exactly one place; found %d: %s", len(mints), strings.Join(mints, ", "))
	}
	mint := mints[0]
	t.Logf("sole Solver mint: %s", mint)
	signature := shape.functionSignature(mint)
	if signature == nil {
		t.Fatalf("mint %s carries no signature", mint)
	}
	params := signature.Params()
	for index := 0; index < params.Len(); index++ {
		named := shape.namedParameter(params.At(index).Type())
		if named == "" {
			continue
		}
		if _, listed := sealedMintParameterTypes[named]; !listed {
			t.Errorf("mint %s takes %s, which sealedMintParameterTypes does not name; a mint input is a sealed value or it is not an input", mint, named)
			continue
		}
		if mutators := shape.exportedMutators(named); len(mutators) != 0 {
			t.Errorf("mint input %s is not sealed: exported methods %s write its own state", named, strings.Join(mutators, ", "))
		}
	}
	for named := range sealedMintParameterTypes {
		if !shape.takesParameter(signature, named) {
			t.Errorf("sealedMintParameterTypes names %s, which mint %s does not take; the allowlist is shrink-only, so remove it", named, mint)
		}
	}
}

// TestNoExportedConstructionParameters is clause (c). No exported engine
// declaration may take a construction handle, and no exported method may hang
// off one: either way code outside the engine drives a builder.
func TestNoExportedConstructionParameters(t *testing.T) {
	shape := loadConstructionShape(t)
	enforceCountdown(t, "(c) exported construction parameter", exportedBuilderParamPins, shape.exportedConstructionParams())
}

// TestRootReachesConstructionThroughOneCall is clause (d). The analysis root
// drives engine construction through exactly one call, and that call's
// parameter types are sealed by (b)'s criterion. Until then every call it
// makes is pinned, so the count can only fall.
func TestRootReachesConstructionThroughOneCall(t *testing.T) {
	shape := loadConstructionShape(t)
	observed := shape.rootConstructionCalls(t)
	sealed, others := shape.partitionSealedEntry(observed)
	for _, entry := range sealed {
		t.Logf("(d) sealed entry: %s", entry.symbol)
	}
	if len(sealed) > 1 {
		names := make([]string, 0, len(sealed))
		for _, entry := range sealed {
			names = append(names, entry.symbol)
		}
		sort.Strings(names)
		t.Errorf("the root reaches engine construction through %d sealed entries (%s); exactly one is the law", len(sealed), strings.Join(names, ", "))
	}
	enforceCountdown(t, "(d) root construction call", rootConstructionCallPins, others)
	if len(rootConstructionCallPins) != 0 {
		return
	}
	if len(sealed) != 1 {
		t.Errorf("clause (d) has no pins left, so the root must reach engine construction through exactly one sealed call; it makes %d", len(sealed))
	}
}

// TestDyingConstructionSymbolsCannotHide is the step-3 deletion checklist. It
// holds the dying names still, so that a rename cannot retire a pin without
// retiring the design.
func TestDyingConstructionSymbolsCannotHide(t *testing.T) {
	declared := declaredSymbolNames(t)
	pins := len(mutatingAdmissionPins) + len(exportedBuilderParamPins) + len(rootConstructionCallPins)
	for _, symbol := range dyingConstructionSymbols {
		present := declared[symbol.name]
		switch {
		case symbol.present && !present && pins != 0:
			t.Errorf("%s left the tree while %d construction pins still stand: a rename hides a symbol, it does not remove it. Remove the design, drop its pins, then drop this entry", symbol.name, pins)
		case symbol.present && !present:
			t.Logf("%s is gone and no pin remains: the cut carried it", symbol.name)
		case !symbol.present && present:
			t.Errorf("%s was already deleted at snapshot time and the tree declares it again", symbol.name)
		case pins == 0 && present:
			t.Errorf("%s still exists with no construction pin left; the cut deletes every symbol on this checklist", symbol.name)
		}
	}
}

// enforceCountdown is the shrink-only comparison every clause shares. An
// observation the pins do not name fails the law, and a pin no observation
// meets fails it too, so the pinned set tracks the tree exactly and can only
// fall. With no pins left the clause reads as its absolute form: any
// observation at all is a violation.
func enforceCountdown(t *testing.T, clause string, pins []constructionPin, observed []observation) {
	t.Helper()
	pinned := make(map[string]constructionPin, len(pins))
	for _, pin := range pins {
		if _, duplicate := pinned[pin.key()]; duplicate {
			t.Fatalf("%s pins %s twice", clause, pin.key())
		}
		pinned[pin.key()] = pin
	}
	met := make(map[string]bool, len(observed))
	for _, found := range observed {
		pin, ok := pinned[found.key()]
		if !ok {
			t.Errorf("%s: %s in %s reaches construction state through %s and no pin names it; dissolve it, or pin it with the step that removes it", clause, found.symbol, found.file, found.via)
			continue
		}
		met[found.key()] = true
		if found.count != pin.count {
			t.Errorf("%s: %s occurs %d times in %s and its pin records %d; the pins are shrink-only", clause, found.symbol, found.count, found.file, pin.count)
		}
	}
	for key, pin := range pinned {
		if met[key] {
			continue
		}
		t.Errorf("%s: pin %s (step %d) meets nothing in the tree; the list is shrink-only, so remove it in the change that dissolved it", clause, key, pin.step)
	}
}

// constructionShape is the type-checked engine package and the structural
// classification the clauses read off it.
type constructionShape struct {
	pkg     *packages.Package
	decls   map[string]*types.Named
	mutable map[string]bool
	driving map[string]bool
}

var enginePackageOnce = sync.OnceValues(loadEnginePackage)

func loadEnginePackage() (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
	}
	loaded, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, err
	}
	for _, pkg := range loaded {
		if pkg.Types != nil && pkg.TypesInfo != nil && len(pkg.Syntax) > 0 {
			return pkg, nil
		}
	}
	return nil, errNoTypedEnginePackage{}
}

type errNoTypedEnginePackage struct{}

func (errNoTypedEnginePackage) Error() string {
	return "the engine package loaded without type information"
}

func loadConstructionShape(t *testing.T) *constructionShape {
	t.Helper()
	pkg, err := enginePackageOnce()
	if err != nil {
		t.Fatalf("load engine package: %v", err)
	}
	shape := &constructionShape{
		pkg:     pkg,
		decls:   map[string]*types.Named{},
		mutable: map[string]bool{},
		driving: map[string]bool{},
	}
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		if named, ok := scope.Lookup(name).Type().(*types.Named); ok {
			shape.decls[name] = named
		}
	}
	writes := shape.stateWriters()
	for name, named := range shape.decls {
		structured, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		if _, allowed := constructionMutableAllowlist[name]; allowed {
			continue
		}
		if shape.mutableFields(structured) && writes[name] {
			shape.mutable[name] = true
		}
	}
	for name, named := range shape.decls {
		iface, ok := named.Underlying().(*types.Interface)
		if !ok {
			continue
		}
		for index := 0; index < iface.NumMethods(); index++ {
			if shape.signatureCarriesMutable(iface.Method(index).Type().(*types.Signature)) != "" {
				shape.driving[name] = true
				break
			}
		}
	}
	for name := range constructionMutableAllowlist {
		if _, declared := shape.decls[name]; !declared {
			t.Errorf("constructionMutableAllowlist names %s, which the engine no longer declares; the list is shrink-only, so remove it", name)
		}
	}
	return shape
}

// mutableFields reports whether a struct carries unexported state a
// constructor accumulates: a map, a slice, an integer counter, or a handle to
// another engine-declared struct. The test over-approximates on purpose; a
// type that matches but is not construction state is named, with its reason,
// in constructionMutableAllowlist.
func (shape *constructionShape) mutableFields(structured *types.Struct) bool {
	for index := 0; index < structured.NumFields(); index++ {
		field := structured.Field(index)
		if field.Exported() {
			continue
		}
		switch under := field.Type().Underlying().(type) {
		case *types.Map, *types.Slice:
			return true
		case *types.Basic:
			if under.Info()&types.IsInteger != 0 {
				return true
			}
		case *types.Pointer:
			if shape.enginePointee(under) != "" {
				return true
			}
		case *types.Struct:
			if named, ok := field.Type().(*types.Named); ok && shape.owns(named) {
				return true
			}
		}
	}
	return false
}

// enginePointee names the engine-declared struct a pointer addresses.
func (shape *constructionShape) enginePointee(pointer *types.Pointer) string {
	named, ok := pointer.Elem().(*types.Named)
	if !ok || !shape.owns(named) {
		return ""
	}
	if _, structured := named.Underlying().(*types.Struct); !structured {
		return ""
	}
	return named.Obj().Name()
}

func (shape *constructionShape) owns(named *types.Named) bool {
	object := named.Obj()
	return object != nil && object.Pkg() != nil && object.Pkg().Path() == shape.pkg.Types.Path()
}

// stateWriters maps an engine type name to whether the package writes state
// through a value of that type: an assignment, an increment, a delete, or a
// clear rooted at the receiver or at a parameter naming the type. Both holders
// count, because a ledger the package mutates through a parameter is state a
// caller accumulates exactly as a receiver is.
//
// A mutation the method performs inside another package, through a handle it
// borrows out of its own state, is invisible here. Those methods are covered
// by clause (c) when they are exported and by the step-3 checklist when they
// are not.
func (shape *constructionShape) stateWriters() map[string]bool {
	writes := map[string]bool{}
	for _, file := range shape.pkg.Syntax {
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			for holder, name := range functionHolders(function) {
				if bodyWritesThrough(function.Body, holder) {
					writes[name] = true
				}
			}
		}
	}
	return writes
}

// functionHolders maps each receiver and parameter identifier of a function to
// the bare type name it holds.
func functionHolders(function *ast.FuncDecl) map[string]string {
	holders := map[string]string{}
	if function.Recv != nil && len(function.Recv.List) == 1 && len(function.Recv.List[0].Names) == 1 {
		if name := receiverTypeName(function.Recv.List[0].Type); name != "" {
			holders[function.Recv.List[0].Names[0].Name] = name
		}
	}
	if function.Type.Params == nil {
		return holders
	}
	for _, param := range function.Type.Params.List {
		name := receiverTypeName(param.Type)
		if name == "" {
			continue
		}
		for _, ident := range param.Names {
			holders[ident.Name] = name
		}
	}
	return holders
}

// receiverTypeName is the bare type name an expression addresses, with the
// pointer and any type arguments stripped.
func receiverTypeName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(typed.X)
	case *ast.IndexExpr:
		return receiverTypeName(typed.X)
	case *ast.IndexListExpr:
		return receiverTypeName(typed.X)
	case *ast.Ident:
		return typed.Name
	}
	return ""
}

// bodyWritesThrough reports whether a body writes into the named identifier's
// own state rather than merely reading it. Rebinding the identifier is not a
// write.
func bodyWritesThrough(body *ast.BlockStmt, holder string) bool {
	written := false
	ast.Inspect(body, func(node ast.Node) bool {
		if written {
			return false
		}
		switch statement := node.(type) {
		case *ast.AssignStmt:
			for _, target := range statement.Lhs {
				if writesTarget(target, holder) {
					written = true
				}
			}
		case *ast.IncDecStmt:
			if writesTarget(statement.X, holder) {
				written = true
			}
		case *ast.CallExpr:
			ident, ok := statement.Fun.(*ast.Ident)
			if !ok || (ident.Name != "delete" && ident.Name != "clear") || len(statement.Args) == 0 {
				return true
			}
			if rootIdent(statement.Args[0]) == holder {
				written = true
			}
		}
		return true
	})
	return written
}

func writesTarget(target ast.Expr, holder string) bool {
	switch target.(type) {
	case *ast.SelectorExpr, *ast.IndexExpr, *ast.StarExpr:
		return rootIdent(target) == holder
	}
	return false
}

func rootIdent(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return rootIdent(typed.X)
	case *ast.IndexExpr:
		return rootIdent(typed.X)
	case *ast.IndexListExpr:
		return rootIdent(typed.X)
	case *ast.StarExpr:
		return rootIdent(typed.X)
	case *ast.ParenExpr:
		return rootIdent(typed.X)
	case *ast.CallExpr:
		return rootIdent(typed.Fun)
	}
	return ""
}

// signatureCarriesMutable names the construction handle a signature takes: a
// pointer to a construction-mutable engine struct, or a builder-driving engine
// interface.
func (shape *constructionShape) signatureCarriesMutable(signature *types.Signature) string {
	params := signature.Params()
	for index := 0; index < params.Len(); index++ {
		if carried := shape.mutableCarrier(params.At(index).Type()); carried != "" {
			return carried
		}
	}
	return ""
}

func (shape *constructionShape) mutableCarrier(typ types.Type) string {
	switch typed := typ.(type) {
	case *types.Pointer:
		if name := shape.enginePointee(typed); name != "" && shape.mutable[name] {
			return name
		}
	case *types.Named:
		name := typed.Obj().Name()
		if shape.owns(typed) && shape.driving[name] {
			return name
		}
	}
	return ""
}

func (shape *constructionShape) mutableNames() []string { return sortedNames(shape.mutable) }
func (shape *constructionShape) drivingNames() []string { return sortedNames(shape.driving) }

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// answersDecision reports whether results are bool or (T, bool).
func answersDecision(signature *types.Signature) bool {
	results := signature.Results()
	if results.Len() == 0 || results.Len() > 2 {
		return false
	}
	basic, ok := results.At(results.Len() - 1).Type().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

// mutatingAdmissions is clause (a)'s measurement.
func (shape *constructionShape) mutatingAdmissions() []observation {
	var found []observation
	for _, file := range shape.pkg.Syntax {
		path := shape.fileName(file.Package)
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			signature, ok := shape.declaredSignature(function)
			if !ok || !answersDecision(signature) {
				continue
			}
			receiver := receiverTypeName(function.Recv.List[0].Type)
			symbol := receiver + "." + function.Name.Name
			if carried := shape.signatureCarriesMutable(signature); carried != "" {
				found = append(found, observation{file: path, symbol: symbol, via: carried, count: 1})
				continue
			}
			if shape.mutable[receiver] && writesOwnState(function) {
				found = append(found, observation{file: path, symbol: symbol, via: receiver, count: 1})
			}
		}
	}
	sortObservations(found)
	return found
}

// writesOwnState reports whether this one method writes through its receiver.
func writesOwnState(function *ast.FuncDecl) bool {
	if function.Body == nil || function.Recv == nil || len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 {
		return false
	}
	return bodyWritesThrough(function.Body, function.Recv.List[0].Names[0].Name)
}

// exportedConstructionParams is clause (c)'s measurement. An exported method
// on an exported construction handle counts too: the receiver is a parameter,
// and holding one is the drive the clause bans.
func (shape *constructionShape) exportedConstructionParams() []observation {
	var found []observation
	for _, file := range shape.pkg.Syntax {
		path := shape.fileName(file.Package)
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || !function.Name.IsExported() {
				continue
			}
			signature, ok := shape.declaredSignature(function)
			if !ok {
				continue
			}
			symbol, receiver := function.Name.Name, ""
			if function.Recv != nil && len(function.Recv.List) == 1 {
				receiver = receiverTypeName(function.Recv.List[0].Type)
				if !ast.IsExported(receiver) {
					continue
				}
				symbol = receiver + "." + symbol
			}
			carried := shape.signatureCarriesMutable(signature)
			if carried == "" && shape.mutable[receiver] {
				carried = receiver
			}
			if carried == "" {
				continue
			}
			found = append(found, observation{file: path, symbol: symbol, via: carried, count: 1})
		}
	}
	for name, named := range shape.decls {
		iface, ok := named.Underlying().(*types.Interface)
		if !ok || !ast.IsExported(name) {
			continue
		}
		for index := 0; index < iface.NumMethods(); index++ {
			method := iface.Method(index)
			carried := shape.signatureCarriesMutable(method.Type().(*types.Signature))
			if carried == "" {
				continue
			}
			found = append(found, observation{file: shape.fileName(named.Obj().Pos()), symbol: name + "." + method.Name(), via: carried, count: 1})
		}
	}
	sortObservations(found)
	return found
}

func (shape *constructionShape) declaredSignature(function *ast.FuncDecl) (*types.Signature, bool) {
	object := shape.pkg.TypesInfo.Defs[function.Name]
	if object == nil {
		return nil, false
	}
	signature, ok := object.Type().(*types.Signature)
	return signature, ok
}

func (shape *constructionShape) functionSignature(name string) *types.Signature {
	function, ok := shape.pkg.Types.Scope().Lookup(name).(*types.Func)
	if !ok {
		return nil
	}
	signature, _ := function.Type().(*types.Signature)
	return signature
}

// solverMints are the functions that construct a Solver value. The engine
// admits exactly one.
func (shape *constructionShape) solverMints() []string {
	mints := map[string]bool{}
	for _, file := range shape.pkg.Syntax {
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				composite, ok := node.(*ast.CompositeLit)
				if !ok || composite.Type == nil {
					return true
				}
				if receiverTypeName(composite.Type) == "Solver" {
					mints[function.Name.Name] = true
				}
				return true
			})
		}
	}
	return sortedNames(mints)
}

// namedParameter is the engine-declared type name a parameter carries, with
// the pointer, slice and array wrappers stripped.
func (shape *constructionShape) namedParameter(typ types.Type) string {
	switch typed := typ.(type) {
	case *types.Pointer:
		return shape.namedParameter(typed.Elem())
	case *types.Slice:
		return shape.namedParameter(typed.Elem())
	case *types.Array:
		return shape.namedParameter(typed.Elem())
	case *types.Named:
		if !shape.owns(typed) {
			return ""
		}
		return typed.Obj().Name()
	}
	return ""
}

func (shape *constructionShape) takesParameter(signature *types.Signature, name string) bool {
	params := signature.Params()
	for index := 0; index < params.Len(); index++ {
		if shape.namedParameter(params.At(index).Type()) == name {
			return true
		}
	}
	return false
}

// exportedMutators names the exported methods of an engine type that write the
// type's own state. A sealed value has none.
func (shape *constructionShape) exportedMutators(name string) []string {
	mutators := map[string]bool{}
	for _, file := range shape.pkg.Syntax {
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 || !function.Name.IsExported() {
				continue
			}
			if receiverTypeName(function.Recv.List[0].Type) != name {
				continue
			}
			if writesOwnState(function) {
				mutators[function.Name.Name] = true
			}
		}
	}
	return sortedNames(mutators)
}

// sealedParameters reports whether every parameter of a signature is a sealed
// value: an engine type the allowlist names and no exported method mutates, or
// a type the engine does not declare at all.
func (shape *constructionShape) sealedParameters(signature *types.Signature) bool {
	params := signature.Params()
	for index := 0; index < params.Len(); index++ {
		typ := params.At(index).Type()
		if carried := shape.mutableCarrier(typ); carried != "" {
			return false
		}
		named := shape.namedParameter(typ)
		if named == "" {
			continue
		}
		if _, listed := sealedMintParameterTypes[named]; !listed {
			return false
		}
		if len(shape.exportedMutators(named)) != 0 {
			return false
		}
	}
	return true
}

// constructionEntries are the exported engine functions the root can reach
// construction through: those that take a construction handle and those that
// produce one.
func (shape *constructionShape) constructionEntries() map[string]bool {
	entries := map[string]bool{}
	scope := shape.pkg.Types.Scope()
	for _, name := range scope.Names() {
		function, ok := scope.Lookup(name).(*types.Func)
		if !ok || !function.Exported() {
			continue
		}
		signature, ok := function.Type().(*types.Signature)
		if !ok {
			continue
		}
		if shape.signatureCarriesMutable(signature) != "" || shape.producesConstruction(signature) {
			entries[name] = true
		}
	}
	return entries
}

// producesConstruction reports whether a signature hands back a construction
// handle or the committed program a construction seals.
func (shape *constructionShape) producesConstruction(signature *types.Signature) bool {
	results := signature.Results()
	for index := 0; index < results.Len(); index++ {
		pointer, ok := results.At(index).Type().(*types.Pointer)
		if !ok {
			continue
		}
		name := shape.enginePointee(pointer)
		if name == "" {
			continue
		}
		if shape.mutable[name] || name == "CommittedProgram" {
			return true
		}
	}
	return false
}

// partitionSealedEntry splits the root's construction calls into the ones that
// could be the single sealed entry and the rest.
func (shape *constructionShape) partitionSealedEntry(observed []observation) (sealed, others []observation) {
	entries := shape.constructionEntries()
	for _, found := range observed {
		name, direct := strings.CutPrefix(found.symbol, "engine.")
		if !direct || !entries[name] {
			others = append(others, found)
			continue
		}
		signature := shape.functionSignature(name)
		if signature != nil && shape.sealedParameters(signature) && found.count == 1 {
			sealed = append(sealed, found)
			continue
		}
		others = append(others, found)
	}
	return sealed, others
}

// rootConstructionCalls is clause (d)'s measurement: every call the analysis
// root makes into engine construction, whether it names an engine entry
// directly or passes the construction handle one handed back.
func (shape *constructionShape) rootConstructionCalls(t *testing.T) []observation {
	t.Helper()
	entries := shape.constructionEntries()
	roots := rootConstructionFileSet(t)
	handleFuncs := rootHandleFunctions(roots, shape)
	counted := map[string]*observation{}
	for _, root := range roots {
		engineName, imported := engineImportName(root.file)
		for _, decl := range root.file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			handles := map[string]bool{}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if assignment, ok := node.(*ast.AssignStmt); ok {
					recordHandleAssignment(assignment, handleFuncs, engineName, imported, shape, handles)
					return true
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee := calleeName(call.Fun)
				if callee == "" {
					return true
				}
				drives := false
				if imported && strings.HasPrefix(callee, engineName+".") && entries[strings.TrimPrefix(callee, engineName+".")] {
					callee = "engine." + strings.TrimPrefix(callee, engineName+".")
					drives = true
				}
				if handles[rootIdent(call.Fun)] {
					drives = true
				}
				for _, argument := range call.Args {
					if handles[rootIdent(argument)] {
						drives = true
					}
				}
				if !drives {
					return true
				}
				key := root.path + " " + function.Name.Name + " " + callee
				if existing, seen := counted[key]; seen {
					existing.count++
					return true
				}
				counted[key] = &observation{file: root.path, symbol: function.Name.Name + " -> " + callee, via: root.path, count: 1}
				return true
			})
		}
	}
	found := make([]observation, 0, len(counted))
	for _, entry := range counted {
		found = append(found, *entry)
	}
	sortObservations(found)
	return found
}

// recordHandleAssignment marks the identifiers a statement binds to a
// construction handle the root just obtained. Only the result positions that
// carry a handle bind one, so the failure value a construction entry returns
// beside it stays an ordinary value.
func recordHandleAssignment(assignment *ast.AssignStmt, handleFuncs map[string][]int, engineName string, imported bool, shape *constructionShape, handles map[string]bool) {
	if len(assignment.Rhs) != 1 {
		return
	}
	call, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok {
		return
	}
	callee := calleeName(call.Fun)
	positions := handleFuncs[lastSegment(callee)]
	if imported && strings.HasPrefix(callee, engineName+".") {
		if signature := shape.functionSignature(strings.TrimPrefix(callee, engineName+".")); signature != nil {
			positions = shape.handleResultPositions(signature)
		}
	}
	for _, position := range positions {
		if position >= len(assignment.Lhs) {
			continue
		}
		if ident, ok := assignment.Lhs[position].(*ast.Ident); ok && ident.Name != "_" {
			handles[ident.Name] = true
		}
	}
}

// handleResultPositions are the result positions of a signature that hand back
// a construction handle.
func (shape *constructionShape) handleResultPositions(signature *types.Signature) []int {
	var positions []int
	results := signature.Results()
	for index := 0; index < results.Len(); index++ {
		pointer, ok := results.At(index).Type().(*types.Pointer)
		if !ok {
			continue
		}
		if name := shape.enginePointee(pointer); name != "" && shape.mutable[name] {
			positions = append(positions, index)
		}
	}
	return positions
}

// rootHandleFunctions names the root's own functions that hand back an engine
// construction handle, with the result positions that carry it.
func rootHandleFunctions(roots []productionFile, shape *constructionShape) map[string][]int {
	names := map[string][]int{}
	for _, root := range roots {
		engineName, imported := engineImportName(root.file)
		if !imported {
			continue
		}
		for _, decl := range root.file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Type.Results == nil {
				continue
			}
			position := 0
			for _, result := range function.Type.Results.List {
				width := len(result.Names)
				if width == 0 {
					width = 1
				}
				if handleResultType(result.Type, engineName, shape) {
					for offset := 0; offset < width; offset++ {
						names[function.Name.Name] = append(names[function.Name.Name], position+offset)
					}
				}
				position += width
			}
		}
	}
	return names
}

// handleResultType reports whether a result expression names a pointer to an
// engine construction handle.
func handleResultType(expr ast.Expr, engineName string, shape *constructionShape) bool {
	pointer, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok || rootIdent(selector) != engineName {
		return false
	}
	return shape.mutable[selector.Sel.Name]
}

func calleeName(fun ast.Expr) string {
	switch typed := fun.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		if base := calleeName(typed.X); base != "" {
			return base + "." + typed.Sel.Name
		}
		return typed.Sel.Name
	case *ast.IndexExpr:
		return calleeName(typed.X)
	case *ast.IndexListExpr:
		return calleeName(typed.X)
	case *ast.ParenExpr:
		return calleeName(typed.X)
	}
	return ""
}

func lastSegment(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}

func rootConstructionFileSet(t *testing.T) []productionFile {
	t.Helper()
	wanted := make(map[string]bool, len(rootConstructionFiles))
	for _, path := range rootConstructionFiles {
		wanted[path] = true
	}
	var roots []productionFile
	for _, file := range productionFiles(t) {
		if wanted[file.path] {
			roots = append(roots, file)
			delete(wanted, file.path)
		}
	}
	for path := range wanted {
		t.Fatalf("clause (d) reads %s and the tree does not carry it", path)
	}
	return roots
}

// declaredSymbolNames is every name the engine and composite trees declare:
// types, functions, methods and struct fields. The checklist reads it to tell
// a deletion from a rename.
func declaredSymbolNames(t *testing.T) map[string]bool {
	t.Helper()
	declared := map[string]bool{}
	for _, file := range productionFiles(t) {
		inRoot := false
		for _, root := range dyingSymbolSearchRoots {
			if strings.HasPrefix(file.path, root) {
				inRoot = true
				break
			}
		}
		if !inRoot {
			continue
		}
		for _, decl := range file.file.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				declared[typed.Name.Name] = true
				if typed.Recv != nil && len(typed.Recv.List) == 1 {
					if receiver := receiverTypeName(typed.Recv.List[0].Type); receiver != "" {
						declared[receiver+"."+typed.Name.Name] = true
					}
				}
			case *ast.GenDecl:
				for _, spec := range typed.Specs {
					switch specified := spec.(type) {
					case *ast.TypeSpec:
						declared[specified.Name.Name] = true
						declareInterfaceMethods(specified, declared)
					case *ast.ValueSpec:
						for _, name := range specified.Names {
							declared[name.Name] = true
						}
					}
				}
			}
		}
	}
	return declared
}

func declareInterfaceMethods(specified *ast.TypeSpec, declared map[string]bool) {
	iface, ok := specified.Type.(*ast.InterfaceType)
	if !ok || iface.Methods == nil {
		return
	}
	for _, method := range iface.Methods.List {
		for _, name := range method.Names {
			declared[name.Name] = true
			declared[specified.Name.Name+"."+name.Name] = true
		}
	}
}

func sortObservations(found []observation) {
	sort.Slice(found, func(i, j int) bool {
		if found[i].symbol != found[j].symbol {
			return found[i].symbol < found[j].symbol
		}
		return found[i].via < found[j].via
	})
}

func (shape *constructionShape) fileName(pos token.Pos) string {
	position := shape.pkg.Fset.Position(pos)
	if position.Filename == "" {
		return "?"
	}
	parts := strings.Split(position.Filename, "/")
	return parts[len(parts)-1]
}

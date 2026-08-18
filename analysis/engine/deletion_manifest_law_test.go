package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

// deletionManifestFiles is the compile/receipt spine: every file listed here is
// deleted whole at the receipt flash cut. The list is shrink-only. An entry may
// be removed only once the file is gone from disk; adding an entry is a
// deliberate edit of this constant.
//
// The manifest names files in the engine flat root only; engineSourceFiles
// skips directories, so a surviving package under analysis/engine is out of
// scope by construction. analysis/engine/rows is such a package: it holds
// the ProgramArtifact declaration surface that outlives the cut, and
// artifact_receipt.go keeps only the lowering that dies with the receipt.
var deletionManifestFiles = []string{
	"activation_candidate_issuer.go",
	"artifact_receipt.go",
	"receipt_observation.go",
	"receipt_query_admission.go",
	"receipt_rule_admission.go",
	"schema_surface_receipt.go",
	"semantic_directory.go",
	"solver_compiler.go",
	"structural_schedule_certificate.go",
	"structural_witness.go",
}

// deletionReplacementFiles carry declarations that the cut replaces rather
// than deletes outright. The list exists so their deletion is tracked with the
// cut.
var deletionReplacementFiles = []string{}

// pinnedRuntimeCompileEdge is one surviving solver-side reference into a
// declaration owned by the deletion manifest. Every edge here dissolves at the
// cut; the set is shrink-only.
type pinnedRuntimeCompileEdge struct {
	from       string
	identifier string
	target     string
	note       string
}

var pinnedRuntimeCompileEdges = []pinnedRuntimeCompileEdge{
	{
		from:       "operand_resolver.go",
		identifier: "ReceiptAssembly",
		target:     "solver_compiler.go",
		note:       "the operand resolver installs against the compile-side assembly, owned by the operand resolver migration",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "AddActivationRuleFromDraft",
		target:     "receipt_rule_admission.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "AddExactRead",
		target:     "receipt_rule_admission.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "AddMountedActivationCandidate",
		target:     "activation_candidate_issuer.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "AddRead",
		target:     "receipt_rule_admission.go or solver_compiler.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "AdmitMountedRuleOccurrence",
		target:     "receipt_rule_admission.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "BeginMountedActivationRuleAdmission",
		target:     "receipt_rule_admission.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "BeginReceiptRuleRow",
		target:     "receipt_rule_admission.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "CompleteMountedActivationCandidates",
		target:     "activation_candidate_issuer.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "QueueMountedRuleFinalizer",
		target:     "solver_compiler.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "ReceiptAssembly",
		target:     "solver_compiler.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "ReceiptReadPart",
		target:     "receipt_rule_admission.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_activation_admit.go",
		identifier: "RuleSourceTransaction",
		target:     "receipt_rule_admission.go",
		note:       "activation admit still runs through the compile-side assembly, owned by the construction-plane activation admit",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "AddAnchoredRouteWrite",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "AddCarry",
		target:     "receipt_rule_admission.go or solver_compiler.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "AddRead",
		target:     "receipt_rule_admission.go or solver_compiler.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "AddRuleFromDraft",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "AddWrite",
		target:     "receipt_rule_admission.go or solver_compiler.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "AdmitLinkRuleOccurrence",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "AdmitMountedRuleOccurrence",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "admitRuleScope",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "AnchoredSelectedReadSurface",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "BeginReceiptRuleRow",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "ReceiptAssembly",
		target:     "solver_compiler.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "ReceiptCarryPart",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "ReceiptReadPart",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "ReceiptWritePart",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "RuleOccurrenceReceipt",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "RuleSourceTransaction",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "runtime_program_admit.go",
		identifier: "RuleSurfaceSourceReceipt",
		target:     "receipt_rule_admission.go",
		note:       "rule admission still runs through the compile-side transaction, owned by the runtime program admit migration",
	},
	{
		from:       "rule_surface.go",
		identifier: "mountedSelectedSurfaceAnchor",
		target:     "solver_compiler.go",
		note:       "RuleReadSurface.anchor and RuleWriteSurface.anchor carry the assembly-owned anchor",
	},
	{
		from:       "rule_surface.go",
		identifier: "bindingSummarySurfaceReceipt",
		target:     "schema_surface_receipt.go",
		note:       "ruleSummaryMapping.receipt is typed by the compile-side summary surface interface",
	},
	{
		from:       "runtime_program_construction.go",
		identifier: "LinkRuleMember",
		target:     "solver_compiler.go",
		note:       "link member lookup on the committed graph handle, owned by the runtime program construction migration",
	},
	{
		from:       "runtime_program_construction.go",
		identifier: "MountedActivationMember",
		target:     "solver_compiler.go",
		note:       "activation member lookup on the committed graph handle, owned by the runtime program construction migration",
	},
	{
		from:       "runtime_program_construction.go",
		identifier: "MountedNativeCallStageReceipt",
		target:     "artifact_receipt.go",
		note:       "native Call stage lookup on the committed graph handle, owned by the runtime program construction migration",
	},
	{
		from:       "runtime_program_construction.go",
		identifier: "MountedRuleMember",
		target:     "solver_compiler.go",
		note:       "mounted member lookup on the committed graph handle, owned by the runtime program construction migration",
	},
	{
		from:       "runtime_program_construction.go",
		identifier: "Query",
		target:     "receipt_query_admission.go",
		note:       "query row lookup on the committed graph handle, owned by the runtime program construction migration",
	},
	{
		from:       "runtime_program_construction.go",
		identifier: "ReceiptGraph",
		target:     "solver_compiler.go",
		note:       "first construction still opens against the committed graph handle; this single edge carries nine method declarations, because runtime_program_construction.go hangs directProgramGraphValid, directProgramGraphState, directProgramGraphValue, directProgramTopology, directPublishedQueryKeys, directMountedRuleMember, directLinkRuleMember, directMountedActivationMember and directQuery off the dying *ReceiptGraph receiver, and the cut re-homes all nine",
	},
	{
		from:       "runtime_program_construction.go",
		identifier: "RuleMember",
		target:     "artifact_receipt.go or solver_compiler.go",
		note:       "member lookup by identity on the committed graph handle, owned by the runtime program construction migration",
	},
	{
		from:       "schema_factor_binding.go",
		identifier: "AddExactRead",
		target:     "receipt_rule_admission.go",
		note:       "exact read admission on the compile-side transaction, owned by the schema factor binding migration",
	},
	{
		from:       "schema_factor_binding.go",
		identifier: "AddExactWrite",
		target:     "receipt_rule_admission.go",
		note:       "exact write admission on the compile-side transaction, owned by the schema factor binding migration",
	},
	{
		from:       "schema_factor_binding.go",
		identifier: "RuleSourceTransaction",
		target:     "receipt_rule_admission.go",
		note:       "the factor admission hooks are typed by the compile-side transaction, owned by the schema factor binding migration",
	},
	{
		from:       "runtime_program_construction.go",
		identifier: "publishedQueryKeys",
		target:     "solver_compiler.go",
		note:       "addressed query keys are read off the committed graph handle, owned by the runtime program construction migration",
	},
}

func TestDeletionManifestPinsRuntimeCompileEdges(t *testing.T) {
	manifest := manifestFileSet(t)
	index := manifestDeclarations(t, manifest)
	pinned := make(map[string]pinnedRuntimeCompileEdge, len(pinnedRuntimeCompileEdges))
	for _, edge := range pinnedRuntimeCompileEdges {
		key := edge.from + " -> " + edge.identifier
		if _, duplicate := pinned[key]; duplicate {
			t.Fatalf("duplicate pinned edge %s", key)
		}
		pinned[key] = edge
	}

	observed := map[string]bool{}
	shadowed := map[string]bool{}
	for _, name := range engineSourceFiles(t) {
		if manifest[name] {
			continue
		}
		file := parseEngineFile(t, name)
		for competitor := range competingMethodNames(t, enginePackageDir) {
			if _, owned := index.methods[competitor]; owned {
				shadowed[competitor] = true
			}
		}
		for _, reference := range engineReferences(t, file, index) {
			key := name + " -> " + reference.identifier
			observed[key] = true
			edge, ok := pinned[key]
			if !ok {
				t.Errorf("unpinned runtime->compile edge %s (declared in %s); dissolve it or pin it in pinnedRuntimeCompileEdges", key, reference.target)
				continue
			}
			if edge.target != reference.target {
				t.Errorf("pinned edge %s records target %s but %s is declared in %s", key, edge.target, reference.identifier, reference.target)
			}
		}
	}

	for key, edge := range pinned {
		switch {
		case observed[key]:
		case shadowedPin(t, enginePackageDir, index, edge.identifier):
			t.Logf("pinned edge %s names a method the resolver cannot attribute here, so the pin stands on its note and the cut lane confirms it by hand", key)
		default:
			t.Errorf("stale pinned edge %s no longer exists; the list is shrink-only, so remove it", key)
		}
	}
	names := make([]string, 0, len(shadowed))
	for name := range shadowed {
		names = append(names, name)
	}
	sort.Strings(names)
	t.Logf("manifest method names a declaration package engine reaches also owns, and which are therefore invisible to this law: %s", strings.Join(names, ", "))
}

// pinnedExternalConsumerEdge is one reference from a production file outside
// package engine into an exported declaration the deletion manifest owns.
// consumer is the path relative to the repository root, symbol the referenced
// declaration, target the manifest file that declares it.
type pinnedExternalConsumerEdge struct {
	consumer string
	symbol   string
	target   string
}

// pinnedExternalConsumerEdges is the countdown to the receipt cut: its length
// is the number of references the rest of the tree still holds into the
// deletion manifest, and the cut lands when it reaches zero. The list is
// shrink-only. A reference this list does not name fails the law and is either
// dissolved or pinned; a pinned entry whose reference is gone fails the law and
// is removed in the change that dissolved it.
//
// The domain migration dissolves these edges continuously, so entries go stale
// as it lands. That is the intended behaviour: whoever lands a migration step
// deletes the entries it dissolved, and the countdown drops in the same change.
var pinnedExternalConsumerEdges = []pinnedExternalConsumerEdge{
	{consumer: "analysis/analyze.go", symbol: "ReceiptGraph", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_diagnostic_plan.go", symbol: "ReceiptGraph", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_diagnostic_plan.go", symbol: "ReceiptObservationAttachFailure", target: "receipt_observation.go"},
	{consumer: "analysis/artifact_diagnostic_plan.go", symbol: "ReceiptObservationAttachFailureArguments", target: "receipt_observation.go"},
	{consumer: "analysis/artifact_diagnostic_plan.go", symbol: "ReceiptObservationAttachFailureNone", target: "receipt_observation.go"},
	{consumer: "analysis/artifact_diagnostic_plan.go", symbol: "ReceiptObservationAttachFailurePoint", target: "receipt_observation.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "ArtifactScalarReceipt", target: "artifact_receipt.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "AssembleMountedArtifactReceipt", target: "artifact_receipt.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "BindRole", target: "artifact_receipt.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "LinkBootstrapCatalog", target: "structural_witness.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "LinkBootstrapPoint", target: "structural_witness.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "LinkBootstrapWitness", target: "structural_witness.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "LinkCapability", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "MountedArtifactReceipt", target: "artifact_receipt.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "MountedCapability", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "MountedQueryBatch", target: "receipt_query_admission.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "NewArtifactScalarBinding", target: "artifact_receipt.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "NewArtifactScalarReceipt", target: "artifact_receipt.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "NewLinkBootstrapWitnessByCapability", target: "structural_witness.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "NewMountedArtifactReceipt", target: "artifact_receipt.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "Phase", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "QueueMountedQueryBatch", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "ReceiptAssembly", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "ReceiptAssemblyFailure", target: "artifact_receipt.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "ReceiptAssemblyFailureNone", target: "artifact_receipt.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "ReceiptCommitFailure", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "ReceiptGraph", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "ReceiptSealFailure", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "ReceiptSealFailureArtifactRows", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "ReleaseArtifactReceipt", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "ScheduleOrdinal", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "SealFailure", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_plan.go", symbol: "SealSources", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_query_plan.go", symbol: "AddMountedExactQuery", target: "receipt_query_admission.go"},
	{consumer: "analysis/artifact_query_plan.go", symbol: "AddMountedSummaryQuery", target: "receipt_query_admission.go"},
	{consumer: "analysis/artifact_query_plan.go", symbol: "MountedQueryBatch", target: "receipt_query_admission.go"},
	{consumer: "analysis/artifact_query_plan.go", symbol: "Query", target: "receipt_query_admission.go"},
	{consumer: "analysis/artifact_query_plan.go", symbol: "ReceiptGraph", target: "solver_compiler.go"},
	{consumer: "analysis/artifact_rule_plan.go", symbol: "ReceiptAssembly", target: "solver_compiler.go"},
	{consumer: "domain/composite/publication/branch_value_observation.go", symbol: "AttachMountedSummaryObservationWithFailure", target: "receipt_observation.go"},
	{consumer: "domain/composite/publication/branch_value_observation.go", symbol: "MatchesID", target: "receipt_observation.go"},
	{consumer: "domain/composite/publication/branch_value_observation.go", symbol: "ReceiptObservation", target: "receipt_observation.go"},
	{consumer: "domain/composite/publication/branch_value_observation.go", symbol: "ReceiptObservationAttachFailure", target: "receipt_observation.go"},
	{consumer: "domain/composite/publication/branch_value_observation.go", symbol: "ReceiptObservationAttachFailureArguments", target: "receipt_observation.go"},
	{consumer: "domain/composite/publication/branch_value_observation.go", symbol: "ReceiptObservationAttachFailureNone", target: "receipt_observation.go"},
	{consumer: "domain/composite/publication/direct_allocation_membership.go", symbol: "AttachMountedSummaryObservationWithFailure", target: "receipt_observation.go"},
	{consumer: "domain/composite/publication/direct_allocation_membership.go", symbol: "MatchesID", target: "receipt_observation.go"},
	{consumer: "domain/composite/publication/direct_allocation_membership.go", symbol: "ReceiptObservation", target: "receipt_observation.go"},
	{consumer: "domain/composite/publication/direct_allocation_membership.go", symbol: "ReceiptObservationAttachFailureNone", target: "receipt_observation.go"},
	{consumer: "domain/effect/callsite/occurrence.go", symbol: "MountedNativeCallStage", target: "artifact_receipt.go"},
	{consumer: "domain/effect/callsite/occurrence.go", symbol: "MountedNativeCallStageReceipt", target: "artifact_receipt.go"},
	{consumer: "domain/effect/callsite/occurrence.go", symbol: "Stage", target: "artifact_receipt.go"},
	{consumer: "domain/effect/callsite/publication_transition.go", symbol: "AttachRuleExactObservation", target: "receipt_observation.go"},
	{consumer: "domain/effect/callsite/publication_transition.go", symbol: "MatchesID", target: "receipt_observation.go"},
	{consumer: "domain/effect/callsite/publication_transition.go", symbol: "MountedNativeCallStageReceipt", target: "artifact_receipt.go"},
	{consumer: "domain/effect/callsite/publication_transition.go", symbol: "ReceiptObservation", target: "receipt_observation.go"},
	{consumer: "domain/effect/callsite/publication_transition.go", symbol: "RuleMember", target: "artifact_receipt.go or solver_compiler.go"},
	{consumer: "domain/effect/callsite/publication_transition.go", symbol: "Stage", target: "artifact_receipt.go"},
	{consumer: "domain/value/owner/hot_owner.go", symbol: "AddSummaryRead", target: "receipt_rule_admission.go"},
	{consumer: "domain/value/owner/hot_owner.go", symbol: "RuleSourceTransaction", target: "receipt_rule_admission.go"},
}


func TestDeletionManifestExternalConsumersOnlyShrink(t *testing.T) {
	index := manifestDeclarations(t, manifestFileSet(t))
	pinned := make(map[string]pinnedExternalConsumerEdge, len(pinnedExternalConsumerEdges))
	for _, edge := range pinnedExternalConsumerEdges {
		key := edge.consumer + " -> " + edge.symbol
		if _, duplicate := pinned[key]; duplicate {
			t.Fatalf("duplicate pinned external consumer edge %s", key)
		}
		pinned[key] = edge
	}
	if !sort.SliceIsSorted(pinnedExternalConsumerEdges, func(i, j int) bool {
		left, right := pinnedExternalConsumerEdges[i], pinnedExternalConsumerEdges[j]
		if left.consumer != right.consumer {
			return left.consumer < right.consumer
		}
		return left.symbol < right.symbol
	}) {
		t.Errorf("pinnedExternalConsumerEdges must stay sorted by consumer then symbol for reviewable shrinkage")
	}

	observed := map[string]bool{}
	consumers := map[string]bool{}
	for _, consumer := range productionFiles(t) {
		alias, ok := engineImportName(consumer.file)
		if !ok {
			continue
		}
		for _, reference := range consumerReferences(t, consumer, alias, index) {
			key := consumer.path + " -> " + reference.identifier
			observed[key] = true
			consumers[consumer.path] = true
			edge, pinnedOK := pinned[key]
			if !pinnedOK {
				t.Errorf("new external consumer edge into the deletion manifest; dissolve it or pin it in pinnedExternalConsumerEdges:\n\t{consumer: %q, symbol: %q, target: %q},", consumer.path, reference.identifier, reference.target)
				continue
			}
			if edge.target != reference.target {
				t.Errorf("pinned external consumer edge %s records target %s but %s is declared in %s", key, edge.target, reference.identifier, reference.target)
			}
		}
	}

	shadowedPins := 0
	for key, edge := range pinned {
		switch {
		case observed[key]:
		case shadowedPin(t, path.Dir(edge.consumer), index, edge.symbol):
			shadowedPins++
			consumers[edge.consumer] = true
			t.Logf("pinned external consumer edge %s names a method the resolver cannot attribute in %s, so the pin holds its place in the countdown and the cut lane confirms it by hand", key, path.Dir(edge.consumer))
		default:
			t.Errorf("stale pinned external consumer edge %s no longer exists; the list is shrink-only, so remove it", key)
		}
	}
	if shadowedPins > 0 {
		t.Logf("%d of the pinned edges rest on their note rather than on a resolved reference", shadowedPins)
	}
	t.Logf("receipt cut countdown: %d external consumer edges across %d files", len(pinnedExternalConsumerEdges), len(consumers))
}

// TestDomainAndSchemaAdmissionHooksNameNoReceiptConstruction states the
// construction-retype floor: domain and schema production admission surfaces
// name neither ReceiptAssembly nor ReceiptGraph. Remaining receipt edges are
// the analysis-root construction spine.
func TestDomainAndSchemaAdmissionHooksNameNoReceiptConstruction(t *testing.T) {
	index := manifestDeclarations(t, manifestFileSet(t))
	visitedDomain, visitedSchema := 0, 0
	for _, consumer := range productionFiles(t) {
		switch {
		case strings.HasPrefix(consumer.path, "domain/"):
			visitedDomain++
		case strings.HasPrefix(consumer.path, "analysis/schema/"):
			visitedSchema++
		default:
			continue
		}
		alias, imported := engineImportName(consumer.file)
		if !imported {
			continue
		}
		for _, reference := range consumerReferences(t, consumer, alias, index) {
			if reference.identifier == "ReceiptAssembly" || reference.identifier == "ReceiptGraph" {
				t.Errorf("%s admission hook names %s", consumer.path, reference.identifier)
			}
		}
	}
	if visitedDomain == 0 || visitedSchema == 0 {
		t.Fatalf("admission walk visited domain=%d schema=%d", visitedDomain, visitedSchema)
	}
}

// deletionManifestTestFiles is the test-side countdown to the receipt cut:
// every engine test file that names a declaration the deletion manifest owns,
// and therefore dies or is rewritten with it. The list is shrink-only, and an
// entry comes off in the commit that deletes the file it names.
//
// The law is a biconditional. A listed file that reaches no manifest
// declaration is stale, because its subjects survive the cut. An unlisted file
// that reaches one is a receipt-coupled test appearing unnoticed, and is either
// dissolved or listed.
var deletionManifestTestFiles = []string{
	"artifact_bootstrap_transport_law_test.go",
	"artifact_native_call_stage_receipt_law_test.go",
	"artifact_predecessor_input_law_test.go",
	"artifact_scalar_builder_law_test.go",
	"identity_framing_laws_test.go",
	"operand_resolver_law_test.go",
	"receipt_assembly_lifecycle_law_test.go",
	"receipt_exact_observation_law_test.go",
	"receipt_observation_matches_id_law_test.go",
	"receipt_semantic_directory_law_test.go",
	"receipt_solver_fallback_law_test.go",
	"runtime_executor_law_test.go",
	"runtime_program_revision_law_test.go",
	"runtime_program_table_law_test.go",
	"runtime_region_discharge_law_test.go",
	"semantic_directory_law_test.go",
	"snapshot_materialize_law_test.go",
	"solve_law_test.go",
	"stable_query_authority_law_test.go",
	"state_borrow_law_test.go",
	"structural_schedule_stage_rank_law_test.go",
	"structural_witness_capability_law_test.go",
	"value_model_bench_test.go",
}

func TestDeletionManifestCoversItsTestSide(t *testing.T) {
	manifest := manifestFileSet(t)
	index := manifestDeclarations(t, manifest)
	listed := make(map[string]bool, len(deletionManifestTestFiles))
	for _, name := range deletionManifestTestFiles {
		if listed[name] {
			t.Errorf("duplicate manifest test entry %s", name)
		}
		listed[name] = true
		if _, err := os.Stat(filepath.Join(engineRoot(t), name)); err != nil {
			t.Errorf("manifest test entry %s is not on disk; remove the entry in the same change that deletes the file: %v", name, err)
		}
	}
	if !sort.StringsAreSorted(deletionManifestTestFiles) {
		t.Errorf("deletionManifestTestFiles must stay sorted for reviewable shrinkage")
	}

	coupled := 0
	for _, name := range engineTestFiles(t) {
		references := engineReferences(t, parseEngineFile(t, name), index)
		switch {
		case len(references) > 0:
			coupled++
			if !listed[name] {
				t.Errorf("engine test file %s reaches manifest declaration %s (declared in %s) without being listed; dissolve the coupling or add it to deletionManifestTestFiles:\n\t%q,", name, references[0].identifier, references[0].target, name)
			}
		case listed[name]:
			t.Errorf("stale manifest test entry %s reaches no manifest declaration; its subjects survive the cut, so remove it", name)
		}
	}
	t.Logf("receipt cut test-side countdown: %d engine test files coupled to the deletion manifest", coupled)
}

func engineTestFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(engineRoot(t))
	if err != nil {
		t.Fatalf("read engine root: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestDeletionManifestIsShrinkOnly(t *testing.T) {
	root := engineRoot(t)
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, deletionManifestFiles...), deletionReplacementFiles...) {
		if seen[name] {
			t.Errorf("duplicate manifest entry %s", name)
		}
		seen[name] = true
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("manifest entry %s is not on disk; remove the entry in the same change that deletes the file: %v", name, err)
		}
	}
	if !sort.StringsAreSorted(deletionManifestFiles) {
		t.Errorf("deletionManifestFiles must stay sorted for reviewable shrinkage")
	}
	for _, name := range deletionReplacementFiles {
		for _, manifestName := range deletionManifestFiles {
			if name == manifestName {
				t.Errorf("%s is listed as both a deletion and a replacement file", name)
			}
		}
	}
}

func engineRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("engine root: %v", err)
	}
	return root
}

func manifestFileSet(t *testing.T) map[string]bool {
	t.Helper()
	manifest := make(map[string]bool, len(deletionManifestFiles))
	for _, name := range deletionManifestFiles {
		manifest[name] = true
	}
	return manifest
}

func engineSourceFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(engineRoot(t))
	if err != nil {
		t.Fatalf("read engine root: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func parseEngineFile(t *testing.T, name string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(engineRoot(t), name), nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return file
}

// modulePath is this module, and enginePackagePath the import path a consumer
// names to reach this package.
const (
	modulePath        = "github.com/wippyai/go-lua"
	enginePackagePath = modulePath + "/analysis/engine"
)

// productionSourceRoots are the trees that hold the engine's consumers,
// relative to the repository root.
var productionSourceRoots = []string{"analysis", "domain"}

// productionFile is one parsed production source file, addressed by its
// slash-separated path relative to the repository root.
type productionFile struct {
	path string
	file *ast.File
}

// productionCorpus parses every production file under productionSourceRoots
// once per test binary; both laws read the same parse.
var productionCorpus = sync.OnceValues(loadProductionCorpus)

func loadProductionCorpus() ([]productionFile, error) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []productionFile
	for _, source := range productionSourceRoots {
		walkErr := filepath.WalkDir(filepath.Join(root, source), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			parsed, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if parseErr != nil {
				return parseErr
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			files = append(files, productionFile{path: filepath.ToSlash(relative), file: parsed})
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, nil
}

func productionFiles(t *testing.T) []productionFile {
	t.Helper()
	files, err := productionCorpus()
	if err != nil {
		t.Fatalf("parse production corpus: %v", err)
	}
	return files
}

// engineImportName returns the local name this file gives the engine package.
// A blank or dot import carries no selector, so it reports no name.
func engineImportName(file *ast.File) (string, bool) {
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != enginePackagePath {
			continue
		}
		if spec.Name == nil {
			return "engine", true
		}
		if spec.Name.Name == "_" || spec.Name.Name == "." {
			return "", false
		}
		return spec.Name.Name, true
	}
	return "", false
}

// manifestSymbolIndex indexes the declarations the deletion manifest owns.
// packageLevel maps a package-scope name to its manifest file. methods maps a
// receiver method name to every manifest file declaring it, because one name
// can name a method on several manifest types.
type manifestSymbolIndex struct {
	packageLevel map[string]string
	methods      map[string][]string
}

// declaringFile reports the manifest files that own name. A method several
// manifest types share resolves to all of them, so the pin records the
// attribution the resolver can actually make.
func (index manifestSymbolIndex) declaringFile(name string) string {
	if file, ok := index.packageLevel[name]; ok {
		return file
	}
	return strings.Join(index.methods[name], " or ")
}

// manifestDeclarations indexes every top-level declaration owned by the
// deletion manifest, methods included, so a reference from a surviving file is
// visible as a name lookup.
func manifestDeclarations(t *testing.T, manifest map[string]bool) manifestSymbolIndex {
	t.Helper()
	index := manifestSymbolIndex{
		packageLevel: map[string]string{},
		methods:      map[string][]string{},
	}
	for _, name := range engineSourceFiles(t) {
		if !manifest[name] {
			continue
		}
		file := parseEngineFile(t, name)
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				if typed.Recv != nil {
					index.methods[typed.Name.Name] = append(index.methods[typed.Name.Name], name)
					continue
				}
				index.packageLevel[typed.Name.Name] = name
			case *ast.GenDecl:
				for _, spec := range typed.Specs {
					switch specific := spec.(type) {
					case *ast.TypeSpec:
						index.packageLevel[specific.Name.Name] = name
					case *ast.ValueSpec:
						for _, ident := range specific.Names {
							index.packageLevel[ident.Name] = name
						}
					}
				}
			}
		}
	}
	delete(index.packageLevel, "_")
	delete(index.methods, "_")
	for name, files := range index.methods {
		sort.Strings(files)
		index.methods[name] = slices.Compact(files)
	}
	return index
}

// enginePackageDir is the engine package, relative to the repository root.
const enginePackageDir = "analysis/engine"

// packageCompetitors maps each package under productionSourceRoots to the
// names a selector can call on a value that package owns, the deletion
// manifest's own files excluded: concrete methods, interface methods, and
// struct fields of function type. A package absorbs the callables of every
// module package its files import, because the type a field carries is named
// where the field is declared and called anywhere in the owning package.
//
// One import level bounds the absorption. A type reached only through a field
// of an indirectly imported package stays outside it, so a name it owns still
// reads as an edge. That direction is deliberate: an unbounded absorption
// hides live references, which is the blindness these laws exist to close,
// while a bounded one at worst pins an edge that a reader can dismiss.
var packageCompetitors = sync.OnceValues(loadPackageCompetitors)

func loadPackageCompetitors() (map[string]map[string]bool, error) {
	files, err := productionCorpus()
	if err != nil {
		return nil, err
	}
	manifest := make(map[string]bool, len(deletionManifestFiles))
	for _, name := range deletionManifestFiles {
		manifest[name] = true
	}
	callables := map[string]map[string]bool{}
	imports := map[string]map[string]bool{}
	for _, production := range files {
		directory := path.Dir(production.path)
		if directory == enginePackageDir && manifest[path.Base(production.path)] {
			continue
		}
		if callables[directory] == nil {
			callables[directory] = map[string]bool{}
			imports[directory] = map[string]bool{}
		}
		for name := range selectorCallables(production.file) {
			callables[directory][name] = true
		}
		for _, spec := range production.file.Imports {
			imported := strings.Trim(spec.Path.Value, `"`)
			if strings.HasPrefix(imported, modulePath+"/") {
				imports[directory][strings.TrimPrefix(imported, modulePath+"/")] = true
			}
		}
	}
	competitors := make(map[string]map[string]bool, len(callables))
	for directory, own := range callables {
		competing := make(map[string]bool, len(own))
		for name := range own {
			competing[name] = true
		}
		for imported := range imports[directory] {
			for name := range callables[imported] {
				competing[name] = true
			}
		}
		competitors[directory] = competing
	}
	return competitors, nil
}

// selectorCallables returns the names this file declares that a selector can
// call: concrete methods, interface methods, and struct fields of function
// type.
func selectorCallables(file *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, decl := range file.Decls {
		if function, ok := decl.(*ast.FuncDecl); ok && function.Recv != nil {
			names[function.Name.Name] = true
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.InterfaceType:
			if typed.Methods == nil {
				return true
			}
			for _, method := range typed.Methods.List {
				for _, ident := range method.Names {
					names[ident.Name] = true
				}
			}
		case *ast.StructType:
			if typed.Fields == nil {
				return true
			}
			for _, field := range typed.Fields.List {
				if _, ok := field.Type.(*ast.FuncType); !ok {
					continue
				}
				for _, ident := range field.Names {
					names[ident.Name] = true
				}
			}
		}
		return true
	})
	return names
}

// shadowedPin reports whether a pinned edge names a manifest method that a
// declaration this package reaches also owns. The resolver cannot attribute
// such a call either way, so the pin stays: dropping it would shorten the
// countdown on a name collision rather than on a dissolved reference.
func shadowedPin(t *testing.T, packageDir string, index manifestSymbolIndex, symbol string) bool {
	t.Helper()
	if _, owned := index.methods[symbol]; !owned {
		return false
	}
	return competingMethodNames(t, packageDir)[symbol]
}

// competingMethodNames returns the names a file in this package can call on a
// value some declaration outside the deletion manifest owns. A manifest method
// sharing such a name carries no attribution here, because a selector names a
// method through its receiver's type, which these laws do not resolve.
func competingMethodNames(t *testing.T, packageDir string) map[string]bool {
	t.Helper()
	competitors, err := packageCompetitors()
	if err != nil {
		t.Fatalf("index package competitors: %v", err)
	}
	return competitors[packageDir]
}

// manifestReference is one resolved use of a manifest-owned declaration.
type manifestReference struct {
	identifier string
	target     string
}

// engineReferences returns the manifest-owned declarations a surviving file in
// package engine names: package-scope identifiers and method calls.
func engineReferences(t *testing.T, file *ast.File, index manifestSymbolIndex) []manifestReference {
	t.Helper()
	found := map[string]bool{}
	for _, name := range packageLevelReferences(file, index.packageLevel) {
		found[name] = true
	}
	for _, name := range methodReferences(file, index.methods, competingMethodNames(t, enginePackageDir)) {
		found[name] = true
	}
	return sortedReferences(found, index)
}

// consumerReferences returns the exported manifest-owned declarations a
// production file outside package engine names: package-scope declarations
// reached through the engine import alias, and methods called by name.
func consumerReferences(t *testing.T, consumer productionFile, alias string, index manifestSymbolIndex) []manifestReference {
	t.Helper()
	file := consumer.file
	found := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok || qualifier.Name != alias || !ast.IsExported(selector.Sel.Name) {
			return true
		}
		if _, declared := index.packageLevel[selector.Sel.Name]; declared {
			found[selector.Sel.Name] = true
		}
		return true
	})
	for _, name := range methodReferences(file, index.methods, competingMethodNames(t, path.Dir(consumer.path))) {
		if ast.IsExported(name) {
			found[name] = true
		}
	}
	return sortedReferences(found, index)
}

func sortedReferences(found map[string]bool, index manifestSymbolIndex) []manifestReference {
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	references := make([]manifestReference, 0, len(names))
	for _, name := range names {
		references = append(references, manifestReference{identifier: name, target: index.declaringFile(name)})
	}
	return references
}

// packageLevelReferences returns the sorted set of manifest-declared names that
// this file uses as package-level identifiers. Selector fields, struct and
// interface member names, composite-literal field keys, and every binding
// occurrence are excluded so only true package-scope uses are reported.
func packageLevelReferences(file *ast.File, declared map[string]string) []string {
	bound := boundIdentifiers(file)
	found := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok || bound[ident] {
			return true
		}
		if _, declaredName := declared[ident.Name]; !declaredName {
			return true
		}
		found[ident.Name] = true
		return true
	})
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// methodReferences returns the sorted set of manifest-declared method names
// this file calls. A method is named through its receiver's type, which these
// laws do not compute, so matching is by selector name in call position,
// narrowed by competing: a name a declaration this file reaches also owns
// carries no attribution and is skipped. Two approximations remain. A method
// value that is passed rather than called reads as no edge, and a call on a
// standard library or third-party receiver stays outside competing's bound.
func methodReferences(file *ast.File, methods map[string][]string, competing map[string]bool) []string {
	imports := importNames(file)
	found := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := calledSelector(call.Fun)
		if !ok {
			return true
		}
		if qualifier, ok := selector.X.(*ast.Ident); ok && imports[qualifier.Name] {
			return true
		}
		if competing[selector.Sel.Name] {
			return true
		}
		if _, declared := methods[selector.Sel.Name]; declared {
			found[selector.Sel.Name] = true
		}
		return true
	})
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// calledSelector unwraps the callee of a call expression down to the selector
// that names the callee, generic instantiation included.
func calledSelector(expr ast.Expr) (*ast.SelectorExpr, bool) {
	switch typed := expr.(type) {
	case *ast.SelectorExpr:
		return typed, true
	case *ast.IndexExpr:
		return calledSelector(typed.X)
	case *ast.IndexListExpr:
		return calledSelector(typed.X)
	case *ast.ParenExpr:
		return calledSelector(typed.X)
	default:
		return nil, false
	}
}

// importNames returns the local name of every import in this file, so a
// package-qualified selector is not read as a method call.
func importNames(file *ast.File) map[string]bool {
	names := make(map[string]bool, len(file.Imports))
	for _, spec := range file.Imports {
		if spec.Name != nil {
			names[spec.Name.Name] = true
			continue
		}
		path := strings.Trim(spec.Path.Value, `"`)
		names[path[strings.LastIndex(path, "/")+1:]] = true
	}
	return names
}

func boundIdentifiers(file *ast.File) map[*ast.Ident]bool {
	bound := map[*ast.Ident]bool{}
	markNames := func(list *ast.FieldList) {
		if list == nil {
			return
		}
		for _, field := range list.List {
			for _, ident := range field.Names {
				bound[ident] = true
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.SelectorExpr:
			bound[typed.Sel] = true
		case *ast.StructType:
			markNames(typed.Fields)
		case *ast.InterfaceType:
			markNames(typed.Methods)
		case *ast.FuncType:
			markNames(typed.TypeParams)
			markNames(typed.Params)
			markNames(typed.Results)
		case *ast.FuncDecl:
			bound[typed.Name] = true
			markNames(typed.Recv)
		case *ast.TypeSpec:
			bound[typed.Name] = true
			markNames(typed.TypeParams)
		case *ast.ValueSpec:
			for _, ident := range typed.Names {
				bound[ident] = true
			}
		case *ast.AssignStmt:
			if typed.Tok != token.DEFINE {
				return true
			}
			for _, expr := range typed.Lhs {
				if ident, ok := expr.(*ast.Ident); ok {
					bound[ident] = true
				}
			}
		case *ast.LabeledStmt:
			bound[typed.Label] = true
		case *ast.CompositeLit:
			if !structuralCompositeLiteral(typed.Type) {
				return true
			}
			for _, elt := range typed.Elts {
				pair, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if ident, ok := pair.Key.(*ast.Ident); ok {
					bound[ident] = true
				}
			}
		}
		return true
	})
	return bound
}

// structuralCompositeLiteral reports whether the literal keys are struct field
// names rather than map or slice index expressions.
func structuralCompositeLiteral(expr ast.Expr) bool {
	switch typed := expr.(type) {
	case nil:
		return true
	case *ast.Ident, *ast.SelectorExpr, *ast.StructType:
		return true
	case *ast.IndexExpr:
		return structuralCompositeLiteral(typed.X)
	case *ast.IndexListExpr:
		return structuralCompositeLiteral(typed.X)
	case *ast.StarExpr:
		return structuralCompositeLiteral(typed.X)
	default:
		return false
	}
}

package architecture

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/wippyai/go-lua"

type listedPackage struct {
	ImportPath string
	Name       string
	Imports    []string
}

func TestLowerLayerImportBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		banned   []string
	}{
		{
			name:     "ir stays below lua check and lua parser ast",
			patterns: []string{modulePath + "/analysis/ir/..."},
			banned: []string{
				modulePath + "/analysis/lua",
				modulePath + "/analysis/check",
				modulePath + "/compiler/ast",
				modulePath + "/compiler/parse",
				modulePath + "/compiler/source",
			},
		},
		{
			name:     "cfgbuild stays before transferfacts and check",
			patterns: []string{modulePath + "/analysis/lua/cfgbuild"},
			banned: []string{
				modulePath + "/analysis/lua/transferfacts",
				modulePath + "/analysis/check",
			},
		},
		{
			name:     "transferfacts stays before check fixpoint",
			patterns: []string{modulePath + "/analysis/lua/transferfacts"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/check/fixpoint",
			},
		},
		{
			name:     "lua moduleidentity stays below check and engine",
			patterns: []string{modulePath + "/analysis/lua/moduleidentity"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine",
			},
		},
		{
			name:     "engine signature effect lowering stays lua and check free",
			patterns: []string{modulePath + "/analysis/engine/effectlowering"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name:     "engine sourcevalue stays read-model only",
			patterns: []string{modulePath + "/analysis/engine/sourcevalue"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine/factapply",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name:     "engine visibility stays generic",
			patterns: []string{modulePath + "/analysis/engine/visibility"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine/factflow",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name:     "engine callboundary stays boundary schema only",
			patterns: []string{modulePath + "/analysis/engine/callboundary"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine/factapply",
				modulePath + "/analysis/engine/factflow",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
				modulePath + "/compiler",
			},
		},
		{
			name:     "check body stays below fixpoint owners",
			patterns: []string{modulePath + "/analysis/check/body"},
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
				modulePath + "/analysis/check/fixpoint",
			},
		},
		{
			name:     "engine factflow stays syntax check type and state independent",
			patterns: []string{modulePath + "/analysis/engine/factflow"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine/state",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
			},
		},
		{
			name:     "engine callproducer stays projection only",
			patterns: []string{modulePath + "/analysis/engine/callproducer"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine/factapply",
				modulePath + "/analysis/engine/state",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
				modulePath + "/compiler",
			},
		},
		{
			name:     "engine transfer stays generic",
			patterns: []string{modulePath + "/analysis/engine/transfer"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
			},
		},
		{
			name:     "engine solve stays generic",
			patterns: []string{modulePath + "/analysis/engine/solve"},
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
			},
		},
		{
			name:     "engine state tree stays below syntax type check and lua",
			patterns: []string{modulePath + "/analysis/engine/state/..."},
			banned: []string{
				modulePath + "/__old",
				modulePath + "/analysis/check",
				modulePath + "/analysis/ir/cfg",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
				modulePath + "/compiler",
				"go/ast",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, dep := range productionDeps(t, tt.patterns...) {
				for _, banned := range tt.banned {
					if forbiddenImport(dep, banned, false) {
						t.Fatalf("%s imports forbidden dependency %q", strings.Join(tt.patterns, " "), dep)
					}
				}
			}
		})
	}
}

func TestLowLevelLeafImportBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		banned   []string
	}{
		{
			name:     "type stays independent from domain values and paths",
			patterns: []string{modulePath + "/analysis/type/..."},
			banned: []string{
				modulePath + "/analysis/domain/value",
				modulePath + "/analysis/domain/path",
			},
		},
		{
			name:     "domain path stays independent from type and domain values",
			patterns: []string{modulePath + "/analysis/domain/path/..."},
			banned: []string{
				modulePath + "/analysis/type",
				modulePath + "/analysis/domain/value",
			},
		},
		{
			name: "domain value axis and product stay below type lua and check",
			patterns: []string{
				modulePath + "/analysis/domain/value/axis",
				modulePath + "/analysis/domain/value/product",
			},
			banned: []string{
				modulePath + "/analysis/type",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/check",
			},
		},
		{
			name: "type aware value bridge packages stay below engine lua check and compiler",
			patterns: []string{
				modulePath + "/analysis/domain/value/identityvalue",
				modulePath + "/analysis/domain/value/refinement",
				modulePath + "/analysis/domain/value/typevalue",
				modulePath + "/analysis/domain/value/variant",
			},
			banned: []string{
				modulePath + "/analysis/engine",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/check",
				modulePath + "/compiler",
			},
		},
		{
			name:     "channel select type schema stays domain engine check lua and compiler free",
			patterns: []string{modulePath + "/analysis/type/channelselect"},
			banned: []string{
				modulePath + "/analysis/domain",
				modulePath + "/analysis/engine",
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name:     "module signature schema stays engine check lua and compiler free",
			patterns: []string{modulePath + "/analysis/module/signature"},
			banned: []string{
				modulePath + "/analysis/engine",
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, dep := range productionDeps(t, tt.patterns...) {
				for _, banned := range tt.banned {
					if forbiddenImport(dep, banned, false) {
						t.Fatalf("%s imports forbidden dependency %q", strings.Join(tt.patterns, " "), dep)
					}
				}
			}
		})
	}
}

func TestWIRImportBoundaries(t *testing.T) {
	for _, dep := range productionImports(t, modulePath+"/analysis/ir/wir") {
		for _, banned := range []string{
			modulePath + "/analysis/symbol",
			modulePath + "/analysis/lua",
			modulePath + "/analysis/check",
			modulePath + "/compiler/ast",
			modulePath + "/compiler/parse",
			modulePath + "/compiler/source",
			"go/ast",
		} {
			if forbiddenImport(dep, banned, false) {
				t.Fatalf("wir imports forbidden dependency %q", dep)
			}
		}
	}
}

func TestRequiredSemanticSurfacesExist(t *testing.T) {
	required := []string{
		modulePath + "/analysis/check/contract",
		modulePath + "/analysis/check/judgment",
		modulePath + "/analysis/check/obligation/pass",
		modulePath + "/analysis/domain/value/axis/assertion",
		modulePath + "/analysis/domain/value/axis/escape",
		modulePath + "/analysis/domain/value/axis/evidence",
		modulePath + "/analysis/domain/value/axis/identity",
		modulePath + "/analysis/domain/value/axis/presence",
		modulePath + "/analysis/domain/value/axis/runtimekind",
		modulePath + "/analysis/domain/value/axis/typewitness",
		modulePath + "/analysis/domain/value/axis/variantorigin",
		modulePath + "/analysis/domain/value/identityvalue",
		modulePath + "/analysis/domain/value/proof",
		modulePath + "/analysis/domain/placement",
		modulePath + "/analysis/domain/effect",
		modulePath + "/analysis/domain/effect/capability",
		modulePath + "/analysis/domain/effect/control",
		modulePath + "/analysis/domain/effect/dispatch",
		modulePath + "/analysis/domain/effect/iteration",
		modulePath + "/analysis/domain/effect/mutation",
		modulePath + "/analysis/domain/effect/ownership",
		modulePath + "/analysis/domain/effect/postcondition",
		modulePath + "/analysis/domain/effect/returns",
	}

	for _, pkg := range required {
		if got := productionPackages(t, pkg); len(got) != 1 {
			t.Fatalf("required package %s resolved to %d packages", pkg, len(got))
		}
	}
}

func TestJudgmentImportBoundaries(t *testing.T) {
	for _, dep := range productionDeps(t, modulePath+"/analysis/check/judgment/...") {
		for _, banned := range []string{
			modulePath + "/analysis/check/diagnostics",
			modulePath + "/analysis/engine/state",
			modulePath + "/analysis/lua",
			modulePath + "/compiler",
			"go/ast",
		} {
			if forbiddenImport(dep, banned, false) {
				t.Fatalf("judgment packages import forbidden dependency %q", dep)
			}
		}
	}
}

func TestCanonicalContractImportBoundaries(t *testing.T) {
	for _, dep := range productionDeps(t, modulePath+"/analysis/check/contract/...") {
		for _, banned := range []string{
			modulePath + "/analysis/check/diagnostics",
			modulePath + "/analysis/engine/state",
			modulePath + "/analysis/lua",
			modulePath + "/compiler",
			"go/ast",
		} {
			if forbiddenImport(dep, banned, false) {
				t.Fatalf("contract packages import forbidden dependency %q", dep)
			}
		}
	}
}

func TestPublicReadmodelImportBoundaries(t *testing.T) {
	for _, dep := range productionDeps(t, modulePath+"/analysis/check/readmodel/...") {
		for _, banned := range []string{
			modulePath + "/analysis/check/body",
			modulePath + "/analysis/check/diagnostics",
			modulePath + "/analysis/check/internal/readmodel",
			modulePath + "/analysis/engine/state",
			modulePath + "/analysis/lua",
			modulePath + "/compiler",
			"go/ast",
		} {
			if forbiddenImport(dep, banned, false) {
				t.Fatalf("public readmodel packages import forbidden dependency %q", dep)
			}
		}
	}
}

func TestInternalReadmodelAvoidsSyntaxSemanticReachIns(t *testing.T) {
	for _, dep := range productionImports(t, modulePath+"/analysis/check/internal/readmodel") {
		for _, banned := range []string{
			modulePath + "/analysis/check/diagnostics",
			modulePath + "/analysis/lua/bind",
			modulePath + "/analysis/type/typecall",
			modulePath + "/compiler/ast",
			"go/ast",
		} {
			if forbiddenImport(dep, banned, true) {
				t.Fatalf("internal readmodel directly imports forbidden dependency %q", dep)
			}
		}
	}
}

func TestCallContractOwnsGenericInferencePathSemantics(t *testing.T) {
	owner := filepath.Join("..", "check", "internal", "callcontract", "callcontract.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func InferenceContributionKey",
		"func InferencePathKey",
		"func InferenceContributionMatchesSegments",
		"func InferenceContributionHasSegmentPrefix",
		"func InferencePathStepMatchesSegment",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain generic inference path owner marker %q", owner, want)
		}
	}

	readmodel := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	readmodelContent, err := os.ReadFile(readmodel)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"func inferenceContributionKey",
		"func inferencePathKey",
		"func inferenceContributionMatchesSegments",
		"func inferenceContributionHasSegmentPrefix",
		"func inferenceContributionValueSegments",
		"func inferencePathStepMatchesSegment",
		"case callcontract.InferencePathField:",
		"case callcontract.InferencePathStaticString:",
		"case callcontract.InferencePathStaticInt:",
	} {
		if bytes.Contains(readmodelContent, []byte(banned)) {
			t.Fatalf("%s contains generic inference path semantic marker %q; callcontract owns this vocabulary", readmodel, banned)
		}
	}
}

func TestCallContractOwnsGenericInferenceConflictSemantics(t *testing.T) {
	owner := filepath.Join("..", "check", "internal", "callcontract", "callcontract.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func InferenceTypesConflict",
		"func PlanGenericInferenceConflicts",
		"func InferenceParamSetContains",
		"func SameInferenceParam",
		"func InferenceParamName",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain generic inference conflict owner marker %q", owner, want)
		}
	}

	readmodel := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	readmodelContent, err := os.ReadFile(readmodel)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"type callArgumentConstraintViolation struct",
		"func callArgumentConstraintViolations",
		"func genericInferenceHasDistinctTypes",
		"func genericInferenceTypesConflict",
		"func genericInferenceLiteralFamiliesCompatible",
		"func genericInferenceContributionTypes",
		"func (r Reader) genericInferenceContributions",
		"paramsByIndex := make(map[int][]*typ.TypeParam)",
		"func inferenceParamSetContains",
		"func sameInferenceParam",
		"func inferenceParamName",
		"typelit.FamilyBase",
		"typelit.MergeFamilyBases",
		"callcontract.InstantiatedArgumentAssignable",
	} {
		if bytes.Contains(readmodelContent, []byte(banned)) {
			t.Fatalf("%s contains generic inference conflict semantic marker %q; callcontract owns this vocabulary", readmodel, banned)
		}
	}
}

func TestPublicReadmodelOwnsGenericInferenceContributionSpanSelection(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type GenericInferenceContributionSpanPlan struct",
		"type GenericInferenceContributionSpanCandidate struct",
		"func PlanGenericInferenceContributionSpan",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain generic inference contribution span owner marker %q", owner, want)
		}
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"bestLen := -1",
		"if bestLen >= 0",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains generic inference contribution span selection marker %q; public readmodel owns this policy", internal, banned)
		}
	}
}

func TestTypeProjectionOwnsObjectLiteralExpectedMemberProjection(t *testing.T) {
	owner := filepath.Join("..", "lua", "typeprojection", "object_literal_expected.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func ExpectedTypeAtSegments",
		"func MissingRequiredRecordField",
		"func expectedRecordTypeAtSegments",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain object-literal expected member projection marker %q", owner, want)
		}
	}

	readmodel := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	readmodelContent, err := os.ReadFile(readmodel)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"func typeAtSegments",
		"func recordTypeAtSegments",
		"func missingRequiredObjectLiteralField",
		"func closedRecordType",
		"GetStaticStringIndex(seg.Name)",
		"GetStaticIntIndex(int64(seg.Index))",
	} {
		if bytes.Contains(readmodelContent, []byte(banned)) {
			t.Fatalf("%s contains object-literal expected member projection marker %q; typeprojection owns this vocabulary", readmodel, banned)
		}
	}
}

func TestValueProofImportBoundaries(t *testing.T) {
	for _, dep := range productionDeps(t, modulePath+"/analysis/domain/value/proof") {
		for _, banned := range []string{
			modulePath + "/analysis/check",
			modulePath + "/analysis/engine",
			modulePath + "/analysis/lua",
			modulePath + "/compiler",
			"go/ast",
		} {
			if forbiddenImport(dep, banned, false) {
				t.Fatalf("value proof package imports forbidden dependency %q", dep)
			}
		}
	}
}

func TestValueProofOwnsBoundaryTypeProjection(t *testing.T) {
	owner := filepath.Join("..", "domain", "value", "proof", "proof.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func ConcreteBoundaryType",
		"func WitnessTypeForPresence",
		"func TypeWithBoundaryPresence",
		"func ScalarRuntimeKindType",
		"func RuntimeKindType",
		"func RefineTypeByRuntimeKindSet",
		"func ProjectionWithoutNil",
		"func (r Reader) RuntimeKindReducedType",
		"func (r Reader) VariantOriginType",
		"func (r Reader) FullVariantOriginType",
		"func (r Reader) RefineDeclaredType",
		"func (r Reader) NarrowDeclaredByOrigin",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain value-proof projection owner marker %q", owner, want)
		}
	}

	readmodel := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	readmodelContent, err := os.ReadFile(readmodel)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"func concreteBoundaryType",
		"func witnessTypeForPresence",
		"func typeWithBoundaryPresence",
		"func scalarRuntimeKindType",
		"func runtimeKindType",
		"func runtimeKindEvidenceUnion",
		"func refineTypeByRuntimeKindSet",
		"func projectionWithoutNil",
		"normalize.UnionForEvidence",
		"subst.ExpandInstantiated",
		"unwrap.NormalizeNil",
		"typetable.PresentReadonlyEntryValue",
		"runtimekindof.RestrictTypeToRuntimeKind",
		"variantorigin.Key",
		"variant.FullFamilyType",
	} {
		if bytes.Contains(readmodelContent, []byte(banned)) {
			t.Fatalf("%s contains boundary type projection marker %q; value/proof owns this vocabulary", readmodel, banned)
		}
	}
}

func TestPublicReadmodelOwnsCallArgumentLabelFormatting(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func CallArgumentMemberLabel",
		"func ExpectedLabelWithSuffix",
		"func CallArgumentExpectedLabelSuffix",
		"func CallArgumentMayBeNilMismatch",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain call-argument label owner marker %q", owner, want)
		}
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"func callArgumentMemberLabel",
		"func expectedLabelWithSuffix",
		"func callArgumentExpectedLabelSuffix",
		"func callArgumentMayBeNilMismatch",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains call-argument label helper %q; public readmodel owns this vocabulary", internal, banned)
		}
	}
}

func TestPublicReadmodelOwnsCallArgumentMismatchSubjectSelection(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type CallArgumentMismatchSubjectPlan struct",
		"type CallArgumentMismatchCandidate struct",
		"func PlanCallArgumentMismatchSubject",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain call-argument mismatch subject owner marker %q", owner, want)
		}
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"func (r Reader) refineCallArgumentMismatchSubject",
		"CallArgumentMismatch{Kind: CallArgumentMismatchMissingRequiredField",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains call-argument mismatch subject selection marker %q; public readmodel owns this policy", internal, banned)
		}
	}
}

func TestPublicReadmodelOwnsCallCalleeReportTypeSelection(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ownerContent, []byte("func CallCalleeDeclaredTypeMoreInformative")) {
		t.Fatalf("%s does not contain call-callee declared-type selection owner marker", owner)
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(internalContent, []byte("func calleeDeclaredTypeMoreInformative")) {
		t.Fatalf("%s contains call-callee declared-type selection helper; public readmodel owns this report vocabulary", internal)
	}
}

func TestPublicReadmodelOwnsCallArgumentReportPlanning(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type CallArgumentReportPlan struct",
		"type IndexedCallArgumentObligation struct",
		"func PlanCallArgumentReports",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain call-argument report planning owner marker %q", owner, want)
		}
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"func callArgumentsByIndex",
		"reported := make(map[int]struct{})",
		"CallArgumentReportGenericConflict",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains call-argument report planning marker %q; public readmodel owns report precedence", internal, banned)
		}
	}
}

func TestPublicReadmodelOwnsCallOutcomeParamObligationReportability(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ownerContent, []byte("func ObligationTypeReportable")) {
		t.Fatalf("%s does not contain call-outcome parameter obligation reportability owner marker", owner)
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"refinement.ContainsFreeTypeParam(t)",
		"typ.IsAny(t) || typ.IsUnknown(t)",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains call-outcome parameter obligation reportability marker %q; public readmodel owns this policy", internal, banned)
		}
	}
}

func TestPublicReadmodelOwnsCallArgumentProofCombination(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type CallArgumentProofPlan struct",
		"func CallArgumentProofAdmissible",
		"func CallArgumentWitnessProvenMismatch",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain call-argument proof combination owner marker %q", owner, want)
		}
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"func (r Reader) callArgumentProofAdmissible",
		"func (r Reader) callArgumentWitnessProvenMismatch",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains call-argument proof-combination helper %q; public readmodel owns this policy", internal, banned)
		}
	}
}

func TestPublicReadmodelOwnsCallArgumentCheckPlanning(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type CallArgumentCheckPlan struct",
		"func PlanCallArgumentCheck",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain call-argument check planning owner marker %q", owner, want)
		}
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"labelSuffix :=",
		"CallArgumentMayBeNilMismatch",
		"CallArgumentProofAdmissible",
		"CallArgumentWitnessProvenMismatch",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains call-argument check planning marker %q; public readmodel owns check assembly", internal, banned)
		}
	}
}

func TestPublicReadmodelOwnsCallArityReportPlanning(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type CallArityReportPlan struct",
		"ParameterSpans []SourceSpan",
		"ArgumentSpans  []SourceSpan",
		"func PlanCallArityReport",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain call-arity report planning owner marker %q", owner, want)
		}
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"CallArityReportTooFew",
		"CallArityReportTooMany",
		"declarationIndex :=",
		"callArgumentSpan(site, fixed)",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains call-arity report planning marker %q; public readmodel owns arity report planning", internal, banned)
		}
	}
}

func TestPublicReadmodelOwnsCallCalleeReportPlanning(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type CallCalleeReportPlan struct",
		"CallSpan",
		"func PlanCallCalleeReport",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain call-callee report planning owner marker %q", owner, want)
		}
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"CallCalleeReportMayBeNil",
		"CallCalleeReportNotCallable",
		"typevalue.TypeIncludesNil(t)",
		"span.StartLine == 0",
		"name = \"call target\"",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains call-callee report planning marker %q; public readmodel owns callee report planning", internal, banned)
		}
	}
}

func TestPublicReadmodelOwnsCallContractSourceFormatting(t *testing.T) {
	owner := filepath.Join("..", "check", "readmodel", "api.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type CallContractSource struct",
		"type CallContractSourceKind uint8",
		"func (s CallContractSource) ParameterLabel",
		"func (s CallContractSource) ParameterSpan",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain call-contract source owner marker %q", owner, want)
		}
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"type callContractSource struct",
		"type callContractSourceKind uint8",
		"func (s callContractSource) ParameterLabel",
		"func (s callContractSource) ParameterSpan",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains call-contract source formatting marker %q; public readmodel owns this vocabulary", internal, banned)
		}
	}
}

func TestObligationPassImportsStayReadmodelOnly(t *testing.T) {
	imports := productionImports(t, modulePath+"/analysis/check/obligation/pass")
	required := map[string]bool{
		modulePath + "/analysis/check/readmodel": false,
		modulePath + "/analysis/check/judgment":  false,
	}
	for _, dep := range imports {
		if _, ok := required[dep]; ok {
			required[dep] = true
		}
	}
	for dep, seen := range required {
		if !seen {
			t.Fatalf("obligation pass missing required direct dependency %q", dep)
		}
	}
	for _, dep := range productionDeps(t, modulePath+"/analysis/check/obligation/pass") {
		for _, banned := range []string{
			modulePath + "/analysis/check/body",
			modulePath + "/analysis/check/diagnostics",
			modulePath + "/analysis/check/internal/readmodel",
			modulePath + "/analysis/engine/state",
			modulePath + "/analysis/lua",
			modulePath + "/compiler",
			"go/ast",
		} {
			if forbiddenImport(dep, banned, false) {
				t.Fatalf("obligation pass depends on forbidden dependency %q", dep)
			}
		}
	}
}

func TestDiagnosticsSiblingProducersDoNotCallDirectCallProducerMethods(t *testing.T) {
	root := filepath.Join("..", "check", "diagnostics")
	banned := []byte("directCallContract(")
	bannedMethod := []byte(".directFunctionCall(")
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Base(path) == "direct_call.go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(content, banned) && bytes.Contains(content, bannedMethod) {
			t.Fatalf("%s calls direct-call producer methods; shared contract diagnostics must use a neutral helper or judgment path", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeletedDirectFunctionContractSchemaDoesNotRegrow(t *testing.T) {
	assertDiagnosticsFilesAbsent(t,
		"direct_function_contract.go",
		"direct_function_contract_receiver.go",
	)
	assertDiagnosticsMarkersAbsent(t, []string{
		"type directFunctionContract struct",
		"type directCallParam struct",
		"type directCallResult struct",
		"func lowerDirectFunctionContract",
		"func lowerDirectFunctionContractInResultScope",
		"func lowerDirectFunctionType",
		"func lowerDirectCallResult",
		"func lowerDirectCallParam",
	})
}

func TestDeletedDirectFunctionContractResolutionDoesNotRegrow(t *testing.T) {
	assertDiagnosticsMarkersAbsent(t, []string{
		"func currentDirectFunctionContract",
		"func currentFunctionValueContract",
		"func lossyImplicitSelfMemberFallback",
	})
}

func TestDeletedDirectCallArgumentSourceResolutionDoesNotRegrow(t *testing.T) {
	assertDiagnosticsMarkersAbsent(t, []string{
		"type directCallArgumentSourceTypeResolution struct",
		"func resolveDirectCallArgumentSourceType",
		"func boundaryCallArgumentReaderType",
		"func directCallArgumentFlowExpressionType",
		"func directCallArgumentContractSourceType",
		"func boundaryCallArgumentReader",
	})
}

func TestDeletedDirectFunctionContractReceiverBindingDoesNotRegrow(t *testing.T) {
	assertDiagnosticsFilesAbsent(t, "direct_function_contract_receiver.go")
	assertDiagnosticsMarkersAbsent(t, []string{
		"func bindDirectCallReceiver",
		"func directCallDeclaredReceiverType",
		"func directCallContractHasUnboundReceiverSlot",
		"func contextIndependentImplicitSelfArgument",
		"func implicitSelfArgumentInResultTree",
	})
}

func assertDiagnosticsFilesAbsent(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join("..", "check", "diagnostics", name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("%s exists; deleted legacy diagnostics owner file must not regrow", path)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func assertDiagnosticsMarkersAbsent(t *testing.T, markers []string) {
	t.Helper()
	root := filepath.Join("..", "check", "diagnostics")
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, marker := range markers {
			if bytes.Contains(content, []byte(marker)) {
				t.Fatalf("%s contains deleted legacy marker %q", path, marker)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCallContractOwnsReceiverUsabilitySemantics(t *testing.T) {
	owner := filepath.Join("..", "check", "internal", "callcontract", "callcontract.go")
	ownerContent, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func ReceiverTypeUsable",
		"func BindReceiver",
	} {
		if !bytes.Contains(ownerContent, []byte(want)) {
			t.Fatalf("%s does not contain receiver owner marker %q", owner, want)
		}
	}

	internal := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	internalContent, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"func receiverTypeUsable",
		"func (r Reader) bindCallReceiver",
		"callcontract.ParamConsumesReceiver",
		"BindFirstParameter",
	} {
		if bytes.Contains(internalContent, []byte(banned)) {
			t.Fatalf("%s contains receiver-call semantic marker %q; callcontract owns this receiver-call semantic", internal, banned)
		}
	}
}

func TestDeletedDirectCallDefinitionDiscoveryDoesNotRegrow(t *testing.T) {
	assertDiagnosticsFilesAbsent(t, "direct_call_definitions.go")
	assertDiagnosticsMarkersAbsent(t, []string{
		"type directCallDefinitionCache struct",
		"func newDirectCallDefinitionCache",
		"func directCallDefinitions",
		"func computeDirectCallDefinitions",
		"func directFunctionExprFromExpr",
	})
}

func TestDeletedDirectCallFunctionLiteralContextDoesNotRegrow(t *testing.T) {
	assertDiagnosticsFilesAbsent(t, "direct_call_function_literal.go")
	assertDiagnosticsMarkersAbsent(t, []string{
		"func functionLiteralArgumentContextuallyChecked",
		"func functionLiteralTypeAdmitsContext",
		"func topLikeFunctionPlaceholder",
		"func placeholderFunctionLiteralTypeAdmitsContext",
		"func functionLiteralHasExplicitParamTypes",
		"func unwrapFunctionLiteralArgument",
	})
}

func TestDeletedDirectCallCalleeDiagnosticsDoNotRegrow(t *testing.T) {
	assertDiagnosticsMarkersAbsent(t, []string{
		"func (p directCallContract) possiblyNilCallee",
		"func (p directCallContract) invalidDominatingCalleeDeclarationWouldReport",
		"func calleeFlowType",
		"func boundaryMaybeNilCalleeType",
		"func directPossiblyNilCalleeDiagnostic",
		"func directCalleeDiagnostic",
		"func directNotCallableDiagnostic",
	})
}

func TestDeletedDirectCallArgumentDiagnosticsDoNotRegrow(t *testing.T) {
	assertDiagnosticsFilesAbsent(t, "direct_call_argument_diagnostic.go")
	assertDiagnosticsMarkersAbsent(t, []string{
		"func tooFewArgsDiagnostic",
		"func tooManyArgsDiagnostic",
		"func argTypeDiagnostic",
		"func argProofBoundaryDiagnostic",
		"func objectLiteralArgTypeDiagnostic",
		"func argTypeDiagnosticEnvelope",
		"func argTypeDiagnosticEnvelopeWithSubject",
		"func directCallArgumentSpan",
		"func directCallDeclarationEvidenceSpan",
	})
}

func TestDeletedDirectCallSiteHelpersDoNotRegrow(t *testing.T) {
	assertDiagnosticsFilesAbsent(t, "direct_call_site.go")
	assertDiagnosticsMarkersAbsent(t, []string{
		"func callMemberAccessInfoForSite",
		"func directCallDisplayName",
		"func displayPath",
	})
}

func TestDeletedDirectCallArgumentSemanticsDoNotRegrow(t *testing.T) {
	assertDiagnosticsFilesAbsent(t, "direct_call_argument_semantics.go")
	assertDiagnosticsMarkersAbsent(t, []string{
		"func objectLiteralMemberMismatchForCallArgument",
		"func displayAliasDescribesFlowType",
		"func containsTypeParamSyntax",
		"func declaredArgumentExprType",
		"func directCallArgumentDisplayType",
		"func genericObjectLiteralArgTypeMismatch",
		"func genericObjectLiteralMissingFieldEvidence",
	})
}

func TestFixpointProgramUsesSemanticBoundaryReads(t *testing.T) {
	root := filepath.Join("..", "check", "fixpoint", "program")
	banned := []byte("SourceValueForExplanationAtBoundary")
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(content, banned) {
			t.Fatalf("%s uses SourceValueForExplanationAtBoundary; semantic fixed-point code must use SourceValueAtBoundary", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNormalReturnFactPipelineHasPositiveLaneOwners(t *testing.T) {
	required := []struct {
		path     string
		contains []string
	}{
		{
			path: filepath.Join("..", "engine", "callboundary", "path_bindings.go"),
			contains: []string{
				"type PathBindings struct",
				"func (b PathBindings) Substitute",
				"func ReturnSlotIndex",
				"func PathRootedInReturnSlots",
			},
		},
		{
			path: filepath.Join("..", "engine", "callboundary", "normal_return_lanes.go"),
			contains: []string{
				"type NormalReturnFactLane struct",
				"var normalReturnFactLanes",
				"func (f NormalReturnFacts) FilterPaths",
			},
		},
		{
			path: filepath.Join("..", "engine", "callboundary", "normal_return_facts.go"),
			contains: []string{
				"for _, lane := range normalReturnFactLanes",
			},
		},
		{
			path: filepath.Join("..", "check", "fixpoint", "summary", "normal_return_fact_lanes.go"),
			contains: []string{
				"type normalReturnSummaryLane struct",
				"var normalReturnSummaryLanes",
			},
		},
		{
			path: filepath.Join("..", "check", "fixpoint", "summary", "normal_return_facts.go"),
			contains: []string{
				"for _, lane := range normalReturnSummaryLanes",
			},
		},
		{
			path: filepath.Join("..", "check", "fixpoint", "program", "internal", "projectsummary", "normal_return_project_lanes.go"),
			contains: []string{
				"var normalReturnProjectLanes = callboundary.BindNormalReturnFactLanes",
			},
		},
		{
			path: filepath.Join("..", "check", "fixpoint", "program", "internal", "projectsummary", "boundary_path_projector.go"),
			contains: []string{
				"type boundaryPathProjector struct",
				"func (p boundaryPathProjector) StatePath",
				"func (p boundaryPathProjector) RelConstraintFact",
			},
		},
		{
			path: filepath.Join("..", "check", "fixpoint", "program", "internal", "projectsummary", "project_normal_return_facts.go"),
			contains: []string{
				"for _, lane := range normalReturnProjectLanes",
			},
		},
		{
			path: filepath.Join("..", "engine", "factapply", "normal_return_apply_lanes.go"),
			contains: []string{
				"var normalReturnApplyLanes = callboundary.BindNormalReturnFactLanes",
				"var normalReturnApplyLanes",
				"type normalReturnApplyContext struct",
			},
		},
		{
			path: filepath.Join("..", "engine", "factapply", "call_outcome_apply.go"),
			contains: []string{
				"applyNormalReturnFactPhase(normalApply, normalReturnApplyBeforeParamFacts, out)",
				"applyNormalReturnFactPhase(normalApply, normalReturnApplyAfterParamFacts, out)",
				"applyNormalReturnFactPhase(normalApply, normalReturnApplyAfterParamRelations, out)",
			},
		},
		{
			path: filepath.Join("..", "engine", "factapply", "call_return_slot_facts.go"),
			contains: []string{
				"return facts.FilterPaths",
			},
		},
		{
			path: filepath.Join("..", "check", "fixpoint", "program", "internal", "callresult", "provider.go"),
			contains: []string{
				".DropFactsTouchingPaths(",
			},
		},
	}
	for _, item := range required {
		content, err := os.ReadFile(item.path)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range item.contains {
			if !bytes.Contains(content, []byte(want)) {
				t.Fatalf("%s does not contain positive normal-return lane owner marker %q", item.path, want)
			}
		}
	}
}

func TestPathAddressPipelineHasPositiveOwner(t *testing.T) {
	required := []struct {
		path     string
		contains []string
	}{
		{
			path: filepath.Join("..", "engine", "visibility", "address.go"),
			contains: []string{
				"type Address struct",
				"type StateKeyForm",
				"func AddressAt",
				"func (a Address) VisibleStateKey",
				"func (a Address) VisibleLocalKeyspaceKey",
				"func (a Address) RootOrVisibleStateKey",
				"func (a Address) StructuralStateKey",
				"func (a Address) ForEachStateKey",
				"func (a Address) StateKeys",
				"func (a Address) ForEachKeyspaceKey",
				"func (a Address) KeyspaceKeys",
				"func KeyspaceKeyFromStateKey",
			},
		},
		{
			path: filepath.Join("..", "engine", "visibility", "resolver.go"),
			contains: []string{
				"return AddressAt(resolver, point, path).RootOrVisibleStateKey()",
			},
		},
		{
			path: filepath.Join("..", "engine", "factapply", "path_fact_apply.go"),
			contains: []string{
				"visibility.AddressAt(resolver, point, path).VisibleStateKey()",
				"visibility.AddressAt(resolver, point, path).VisibleKeyspaceKey()",
			},
		},
		{
			path: filepath.Join("..", "engine", "factapply", "dynamic_index_write.go"),
			contains: []string{
				"visibility.AddressAt(resolver, point, tablePath).ForEachStateKey(",
				"visibility.AddressAt(resolver, point, tablePath).RootOrVisibleKeyspaceKey()",
			},
		},
		{
			path: filepath.Join("..", "engine", "factapply", "call_outcome_paths.go"),
			contains: []string{
				"visibility.AddressAt(resolver, point, targetPath).RootOrVisibleKeyspaceKey()",
			},
		},
		{
			path: filepath.Join("..", "engine", "factapply", "roots.go"),
			contains: []string{
				"visibility.AddressAt(resolver, point, tablePath).ForEachKeyspaceKey(",
			},
		},
		{
			path: filepath.Join("..", "check", "body", "internal", "readexpr", "readexpr.go"),
			contains: []string{
				"visibility.AddressAt(config.Visibility, point, p).ForEachStateKey(",
			},
		},
	}
	for _, item := range required {
		content, err := os.ReadFile(item.path)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range item.contains {
			if !bytes.Contains(content, []byte(want)) {
				t.Fatalf("%s does not contain positive path-address owner marker %q", item.path, want)
			}
		}
	}
}

func TestStateLaneCatalogHasPositiveOwner(t *testing.T) {
	required := []struct {
		path     string
		contains []string
	}{
		{
			path: filepath.Join("..", "engine", "state", "lane_catalog.go"),
			contains: []string{
				"type LaneCatalog struct",
				"func (c LaneCatalog) LaneSet() LaneSet",
				"func (c LaneCatalog) DomainWithLaneSet",
				"func (c LaneCatalog) TryDomainWithLaneSet",
				"func (c LaneCatalog) selectSpecs",
			},
		},
		{
			path: filepath.Join("..", "engine", "state", "lane_set.go"),
			contains: []string{
				"type LaneSet struct",
				"func NewLaneSet",
				"func CloneLanes",
				"func DefaultLaneSet",
				"func (s LaneSet) Without",
			},
		},
		{
			path: filepath.Join("..", "engine", "state", "lane_ops.go"),
			contains: []string{
				"type laneSpec struct",
				"func domainFromLaneSpecs",
				"for _, lane := range lanes",
				"func stateLane[T any]",
			},
		},
		{
			path: filepath.Join("..", "engine", "state", "lane_registry.go"),
			contains: []string{
				"var defaultLaneCatalog = newLaneCatalog([]laneSpec{",
				"laneValuesBit            = defaultLaneCatalog.mustLaneBit(LaneValues)",
				"laneDiffRelationsBit     = defaultLaneCatalog.mustLaneBit(LaneDiffRelations)",
			},
		},
		{
			path: filepath.Join("..", "engine", "state", "domain.go"),
			contains: []string{
				"return defaultLaneCatalog.Domain(reg)",
				"return defaultLaneCatalog.TryDomainWithLaneSet(reg, lanes)",
				"return TryDomainWithLaneSet(reg, NewLaneSet(lanes...))",
			},
		},
	}
	for _, item := range required {
		content, err := os.ReadFile(item.path)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range item.contains {
			if !bytes.Contains(content, []byte(want)) {
				t.Fatalf("%s does not contain positive state-lane owner marker %q", item.path, want)
			}
		}
	}
}

func TestConstraintExprReferencesHavePositiveBindingOwner(t *testing.T) {
	owner := filepath.Join("..", "domain", "constraint", "expr", "binding_ref.go")
	content, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type bindingRef struct",
		"func (r bindingRef) Key",
		"func (r bindingRef) Substitute",
		"func (r bindingRef) Eval",
	} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("%s does not contain positive binding-ref owner marker %q", owner, want)
		}
	}

	refFile := filepath.Join("..", "domain", "constraint", "expr", "expr_ref.go")
	content, err = os.ReadFile(refFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		`+".len"`,
		`"param["`,
		`"ret["`,
		`strconv.Itoa`,
	} {
		if bytes.Contains(content, []byte(banned)) {
			t.Fatalf("%s contains reference-key spelling %q; bindingRef owns this boundary", refFile, banned)
		}
	}
}

func TestValueIdentityCarrierHasPositiveOwner(t *testing.T) {
	owner := filepath.Join("..", "domain", "value", "identityvalue", "identityvalue.go")
	content, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func ExactID",
		"func HasExact",
		"func WithExact",
	} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("%s does not contain positive identity carrier owner marker %q", owner, want)
		}
	}

	oldOwner := filepath.Join("..", "engine", "sourcevalue", "path_read.go")
	content, err = os.ReadFile(oldOwner)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"func ExactIdentityID",
		"func HasExactIdentity",
	} {
		if bytes.Contains(content, []byte(banned)) {
			t.Fatalf("%s contains identity carrier helper %q; identityvalue owns this boundary", oldOwner, banned)
		}
	}
}

func TestStateOwnsPlacementQualifiedValueIdentityQueries(t *testing.T) {
	owner := filepath.Join("..", "engine", "state", "placement.go")
	content, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func (s State) PlacementOfValue",
		"func (s State) ValueHasStackLocalExactIdentity",
		"func (s State) ValueHasLocalExclusiveExactIdentity",
	} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("%s does not contain placement-qualified identity owner marker %q", owner, want)
		}
	}

	readmodel := filepath.Join("..", "check", "internal", "readmodel", "readmodel.go")
	content, err = os.ReadFile(readmodel)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		"identityvalue.ExactID",
		"identityvalue.HasExact",
		"ReadPlacement(id)",
		"valueHasExactIdentityWithPlacement",
		"placement.Stack ||",
	} {
		if bytes.Contains(content, []byte(banned)) {
			t.Fatalf("%s contains placement-qualified identity helper %q; state owns this boundary", readmodel, banned)
		}
	}
}

func TestDiagnosticQueryHasPositiveProjectionOwner(t *testing.T) {
	owner := filepath.Join("..", "check", "diagnostics", "judgment_producers.go")
	content, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func judgmentContext",
		"func judgmentContextWithParents",
		"func reachableJudgmentContext",
		"func reachableCallJudgmentContext",
		"readmodel.NewWithParents(result, parents...)",
		"internalreadmodel.NewWithParent(result, parent)",
	} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("%s does not contain judgment producer owner marker %q", owner, want)
		}
	}

	root := filepath.Join("..", "check", "diagnostics")
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() ||
			!strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") ||
			filepath.Base(path) == "judgment_producers.go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, banned := range []string{
			`"github.com/wippyai/go-lua/analysis/check/internal/readmodel"`,
			`"github.com/wippyai/go-lua/analysis/domain/value/typevalue"`,
			`"github.com/wippyai/go-lua/analysis/type/table"`,
			"readmodel.New",
		} {
			if bytes.Contains(content, []byte(banned)) {
				t.Fatalf("%s contains diagnostic projection bypass %q; diagnosticQuery owns this boundary", path, banned)
			}
		}
		assertNoDirectDiagnosticBoundaryReads(t, path, content)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExportManifestSourceProjectionHasPositiveOwner(t *testing.T) {
	sourceType := filepath.Join("..", "check", "exportmanifest", "source_type.go")
	content, err := os.ReadFile(sourceType)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"github.com/wippyai/go-lua/analysis/check/internal/readmodel"`,
		"func exportValueType",
		"readmodel.New(result).ValueType(value)",
	} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("%s does not contain positive export source projection owner marker %q", sourceType, want)
		}
	}
	for _, path := range []string{
		sourceType,
		filepath.Join("..", "check", "exportmanifest", "object_type.go"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{
			`"github.com/wippyai/go-lua/analysis/domain/value/typevalue"`,
			"typevalue.TypeOf",
		} {
			if bytes.Contains(content, []byte(banned)) {
				t.Fatalf("%s contains export boundary projection bypass %q; exportValueType/readmodel owns this boundary", path, banned)
			}
		}
	}
}

func TestProjectSummaryReturnSlotProjectionHasPositiveOwner(t *testing.T) {
	owner := filepath.Join("..", "check", "fixpoint", "program", "internal", "projectsummary", "return_slot_projection.go")
	content, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type returnSlotProjection struct",
		"func newReturnSlotProjection",
		"func (p returnSlotProjection) Sources",
		"func (p returnSlotProjection) Value",
		"SourceValueAtBoundary",
	} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("%s does not contain positive return-slot projection owner marker %q", owner, want)
		}
	}

	for _, path := range []string{
		filepath.Join("..", "check", "fixpoint", "program", "internal", "projectsummary", "project_return_presence_relations.go"),
		filepath.Join("..", "check", "fixpoint", "program", "internal", "projectsummary", "project_return_condition_slots.go"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{
			"returnSourceValueReader",
			"SourceValueAtBoundary",
		} {
			if bytes.Contains(content, []byte(banned)) {
				t.Fatalf("%s contains return-slot boundary read %q; returnSlotProjection owns this boundary", path, banned)
			}
		}
	}
}

func assertNoDirectDiagnosticBoundaryReads(t *testing.T, path string, content []byte) {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, content, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	owned := map[string]struct{}{
		"SourceValueForExplanationAtBoundary": {},
		"SourceValueAtBoundary":               {},
		"SourceValueBeforeBoundary":           {},
		"ExpressionValueAtBoundary":           {},
		"ExpressionValueBeforeBoundary":       {},
		"PathValueAtBoundary":                 {},
		"PathValueBeforeBoundary":             {},
	}
	ast.Inspect(file, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, banned := owned[sel.Sel.Name]; !banned || diagnosticQueryReceiver(sel.X) {
			return true
		}
		pos := fileSet.Position(sel.Pos())
		t.Fatalf("%s:%d uses direct diagnostic boundary read %s; diagnosticQuery owns this boundary", path, pos.Line, sel.Sel.Name)
		return false
	})
}

func diagnosticQueryReceiver(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name == "query"
	case *ast.SelectorExpr:
		return x.Sel.Name == "query"
	case *ast.CallExpr:
		if fn, ok := x.Fun.(*ast.Ident); ok && fn.Name == "newDiagnosticQuery" {
			return true
		}
	}
	return false
}

func TestValueAxisLeafDirectImportBoundaries(t *testing.T) {
	baseAllowed := allowSet(
		modulePath+"/analysis/domain/value/axis",
		modulePath+"/analysis/internal/hash",
	)
	// runtimekindof is the bridge leaf between the runtimekind axis and the type
	// lattice; it converts kinds to and from types, so it imports runtimekind and
	// the type packages it materializes.
	runtimeKindOfAllowed := copyAllowSet(baseAllowed,
		modulePath+"/analysis/domain/value/axis/runtimekind",
		modulePath+"/analysis/type/kind",
		modulePath+"/analysis/type/normalize",
		modulePath+"/analysis/type/subst",
		modulePath+"/analysis/type/typ",
		modulePath+"/analysis/type/unwrap",
	)
	// typewitness carries type evidence and reduces against the runtimekind axis,
	// so beyond the type packages it imports runtimekind and the runtimekindof
	// bridge for its reduced-product rule.
	typeWitnessAllowed := copyAllowSet(baseAllowed,
		modulePath+"/analysis/domain/value/axis/runtimekind",
		modulePath+"/analysis/domain/value/axis/runtimekindof",
		modulePath+"/analysis/type/literal",
		modulePath+"/analysis/type/normalize",
		modulePath+"/analysis/type/refinement",
		modulePath+"/analysis/type/typ",
		modulePath+"/analysis/type/unwrap",
	)

	for _, pkg := range productionPackages(t, modulePath+"/analysis/domain/value/axis/...") {
		if pkg.ImportPath == modulePath+"/analysis/domain/value/axis" {
			continue
		}
		allowed := baseAllowed
		switch pkg.ImportPath {
		case modulePath + "/analysis/domain/value/axis/typewitness":
			allowed = typeWitnessAllowed
		case modulePath + "/analysis/domain/value/axis/runtimekindof":
			allowed = runtimeKindOfAllowed
		}
		assertModuleImportsAllowed(t, pkg.ImportPath, pkg.Imports, allowed)
	}
}

func TestPlacementDomainDirectImportBoundary(t *testing.T) {
	allowed := allowSet(
		modulePath+"/analysis/domain/lattice",
		modulePath+"/analysis/internal/hash",
	)
	assertModuleImportsAllowed(
		t,
		modulePath+"/analysis/domain/placement",
		productionImports(t, modulePath+"/analysis/domain/placement"),
		allowed,
	)
}

func TestDomainValuePackageDirectImportBoundaries(t *testing.T) {
	t.Run("product imports only the presence axis leaf", func(t *testing.T) {
		for _, imp := range productionImports(t, modulePath+"/analysis/domain/value/product") {
			if strings.HasPrefix(imp, modulePath+"/analysis/domain/value/axis/") &&
				imp != modulePath+"/analysis/domain/value/axis/presence" {
				t.Fatalf("product imports non-core axis leaf %q", imp)
			}
			for _, banned := range []string{
				modulePath + "/analysis/domain/effect",
				modulePath + "/analysis/domain/value/refinement",
				modulePath + "/analysis/domain/value/typevalue",
				modulePath + "/analysis/domain/value/variant",
				modulePath + "/analysis/engine",
				modulePath + "/analysis/type",
				modulePath + "/compiler",
			} {
				if forbiddenImport(imp, banned, false) {
					t.Fatalf("product imports forbidden dependency %q", imp)
				}
			}
		}
	})

	t.Run("variant stays independent from value products and axes", func(t *testing.T) {
		for _, dep := range productionDeps(t, modulePath+"/analysis/domain/value/variant/...") {
			for _, banned := range []string{
				modulePath + "/analysis/domain/value/axis",
				modulePath + "/analysis/domain/value/product",
			} {
				if forbiddenImport(dep, banned, false) {
					t.Fatalf("variant imports forbidden dependency %q", dep)
				}
			}
		}
	})

	t.Run("typevalue imports only approved axis leaves", func(t *testing.T) {
		allowedLeaves := allowSet(
			modulePath+"/analysis/domain/value/axis/evidence",
			modulePath+"/analysis/domain/value/axis/presence",
			modulePath+"/analysis/domain/value/axis/runtimekind",
			modulePath+"/analysis/domain/value/axis/runtimekindof",
			modulePath+"/analysis/domain/value/axis/typewitness",
			modulePath+"/analysis/domain/value/axis/variantorigin",
		)
		assertValuePackageAxisImports(t, modulePath+"/analysis/domain/value/typevalue", allowedLeaves)
	})

	t.Run("identityvalue imports only identity carrier leaves", func(t *testing.T) {
		allowedLeaves := allowSet(
			modulePath+"/analysis/domain/value/axis/identity",
			modulePath+"/analysis/domain/value/axis/presence",
		)
		assertValuePackageAxisImports(t, modulePath+"/analysis/domain/value/identityvalue", allowedLeaves)
	})

	t.Run("refinement imports only proof axis leaves", func(t *testing.T) {
		allowedLeaves := allowSet(
			modulePath+"/analysis/domain/value/axis/typewitness",
			modulePath+"/analysis/domain/value/axis/variantorigin",
		)
		assertValuePackageAxisImports(t, modulePath+"/analysis/domain/value/refinement", allowedLeaves)
	})
}

func TestEffectPackagesStayValueAndEngineFree(t *testing.T) {
	for _, dep := range productionDeps(t, modulePath+"/analysis/domain/effect/...") {
		for _, banned := range []string{
			modulePath + "/analysis/check",
			modulePath + "/analysis/domain/value",
			modulePath + "/analysis/engine",
			modulePath + "/analysis/lua",
			modulePath + "/compiler",
		} {
			if forbiddenImport(dep, banned, false) {
				t.Fatalf("effect packages import forbidden dependency %q", dep)
			}
		}
	}
}

func TestEngineStateCompositionImportBoundaries(t *testing.T) {
	allowed := allowSet(
		modulePath+"/analysis/engine/dynamicindex",
		modulePath+"/analysis/engine/state/channelselectfact",
		modulePath+"/analysis/engine/state/effectdelta",
		modulePath+"/analysis/engine/state/escapeevent",
		modulePath+"/analysis/engine/state/heapidentity",
		modulePath+"/analysis/engine/state/lenbound",
		modulePath+"/analysis/engine/state/numbound",
		modulePath+"/analysis/engine/state/pathevidence",
	)
	for _, imp := range productionImports(t, modulePath+"/analysis/engine/state") {
		if strings.HasPrefix(imp, modulePath+"/analysis/engine/") {
			if _, ok := allowed[imp]; !ok {
				t.Fatalf("state root imports non-composition engine dependency %q", imp)
			}
		}
	}
}

func TestEngineStateLeafDirectImportBoundaries(t *testing.T) {
	leafAllowed := map[string]map[string]struct{}{
		modulePath + "/analysis/engine/state/channelselectfact": {},
		modulePath + "/analysis/engine/state/effectdelta":       {},
		modulePath + "/analysis/engine/state/escapeevent":       {},
		modulePath + "/analysis/engine/state/heapidentity": copyAllowSet(nil,
			modulePath+"/analysis/engine/dynamicindex",
		),
		modulePath + "/analysis/engine/state/internal/floor": {},
		modulePath + "/analysis/engine/state/lenbound": copyAllowSet(nil,
			modulePath+"/analysis/engine/state/internal/floor",
		),
		modulePath + "/analysis/engine/state/numbound": copyAllowSet(nil,
			modulePath+"/analysis/engine/state/internal/floor",
		),
		modulePath + "/analysis/engine/state/pathevidence": {},
	}

	for leaf, allowed := range leafAllowed {
		for _, imp := range productionImports(t, leaf) {
			if !strings.HasPrefix(imp, modulePath+"/analysis/engine/") {
				continue
			}
			if _, ok := allowed[imp]; !ok {
				t.Fatalf("%s imports non-leaf engine dependency %q", leaf, imp)
			}
		}
	}
}

func TestCheckSplitDirectImportBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		banned  []string
		exactly bool
	}{
		{
			name: "program does not import public check facade exactly",
			pkg:  modulePath + "/analysis/check/fixpoint/program",
			banned: []string{
				modulePath + "/analysis/check",
			},
			exactly: true,
		},
		{
			name: "program does not import diagnostics or fixture harnesses",
			pkg:  modulePath + "/analysis/check/fixpoint/program",
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
				modulePath + "/analysis/check/checktest",
			},
		},
		{
			name: "sourcevalue does not directly import any analysis/type packages",
			pkg:  modulePath + "/analysis/engine/sourcevalue",
			banned: []string{
				modulePath + "/analysis/type",
			},
		},
		{
			name: "factapply does not directly import call outcome policy or type refinement internals",
			pkg:  modulePath + "/analysis/engine/factapply",
			banned: []string{
				modulePath + "/analysis/engine/calloutcome",
				modulePath + "/analysis/type/subtype",
				modulePath + "/analysis/type/access",
				modulePath + "/analysis/type/unwrap",
			},
		},
		{
			name: "calloutcome does not directly import factapply after payload extraction",
			pkg:  modulePath + "/analysis/engine/calloutcome",
			banned: []string{
				modulePath + "/analysis/engine/factapply",
			},
		},
		{
			name: "callpayload remains a neutral outcome payload package",
			pkg:  modulePath + "/analysis/engine/callpayload",
			banned: []string{
				modulePath + "/analysis/engine/factapply",
				modulePath + "/analysis/engine/calloutcome",
				modulePath + "/analysis/check",
				modulePath + "/analysis/lua",
				modulePath + "/compiler",
			},
		},
		{
			name: "placementplan stays projection-only",
			pkg:  modulePath + "/analysis/check/placementplan",
			banned: []string{
				modulePath + "/analysis/check/checktest",
				modulePath + "/analysis/check/diagnostics",
				modulePath + "/analysis/check/exportmanifest",
				modulePath + "/analysis/lua",
				modulePath + "/analysis/type",
				modulePath + "/compiler",
			},
		},
		{
			name: "value refinement remains below engine and check layers",
			pkg:  modulePath + "/analysis/domain/value/refinement",
			banned: []string{
				modulePath + "/analysis/check",
				modulePath + "/analysis/engine",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, imp := range productionImports(t, tt.pkg) {
				for _, banned := range tt.banned {
					if forbiddenImport(imp, banned, tt.exactly) {
						t.Fatalf("%s imports forbidden dependency %q", tt.pkg, imp)
					}
				}
			}
		})
	}
}

func TestCheckCorePackagesDoNotImportDiagnosticsOrFixpoint(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		banned  []string
	}{
		{
			name:    "readmodel stays below diagnostics fixpoint and checktest",
			pattern: modulePath + "/analysis/check/internal/readmodel",
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
				modulePath + "/analysis/check/fixpoint",
				modulePath + "/analysis/check/checktest",
			},
		},
		{
			name:    "body stays below fixpoint and diagnostics",
			pattern: modulePath + "/analysis/check/body",
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
				modulePath + "/analysis/check/fixpoint",
			},
		},
		{
			name:    "program stays below diagnostics",
			pattern: modulePath + "/analysis/check/fixpoint/program",
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
			},
		},
		{
			name:    "summary stays below diagnostics",
			pattern: modulePath + "/analysis/check/fixpoint/summary",
			banned: []string{
				modulePath + "/analysis/check/diagnostics",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, dep := range productionDeps(t, tt.pattern) {
				for _, banned := range tt.banned {
					if forbiddenImport(dep, banned, false) {
						t.Fatalf("%s imports forbidden dependency %q", tt.pattern, dep)
					}
				}
			}
		})
	}
}

func TestReadExprLeafDoesNotImportHigherCheckLayers(t *testing.T) {
	pkg := modulePath + "/analysis/check/body/internal/readexpr"
	banned := []string{
		modulePath + "/analysis/check/body",
		modulePath + "/analysis/check/diagnostics",
		modulePath + "/analysis/check/fixpoint",
		modulePath + "/analysis/check/checktest",
	}
	for _, imp := range productionImports(t, pkg) {
		for _, bannedImport := range banned {
			if forbiddenImport(imp, bannedImport, false) {
				t.Fatalf("%s imports forbidden dependency %q", pkg, imp)
			}
		}
	}
}

func TestActiveCheckTreeHasNoPipelinePackages(t *testing.T) {
	for _, pkg := range productionPackages(t, modulePath+"/analysis/check/...") {
		if pkg.Name == "pipeline" {
			t.Fatalf("%s uses forbidden package name %q", pkg.ImportPath, pkg.Name)
		}
	}
}

func TestLuaProductionPackagesDoNotImportEngineReadBoundaries(t *testing.T) {
	banned := []string{
		modulePath + "/analysis/engine/sourcevalue",
		modulePath + "/analysis/engine/state",
		modulePath + "/analysis/engine/visibility",
	}
	visibilityAdapter := modulePath + "/analysis/lua/visibilityfacts"

	for _, pkg := range productionPackages(t, modulePath+"/analysis/lua/...") {
		for _, imp := range pkg.Imports {
			for _, bannedImport := range banned {
				if pkg.ImportPath == visibilityAdapter && bannedImport == modulePath+"/analysis/engine/visibility" {
					continue
				}
				if forbiddenImport(imp, bannedImport, false) {
					t.Fatalf("%s imports forbidden dependency %q", pkg.ImportPath, imp)
				}
			}
		}
	}
}

func productionPackages(t *testing.T, patterns ...string) []listedPackage {
	t.Helper()

	args := append([]string{"list", "-json"}, patterns...)
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []listedPackage
	for {
		var pkg listedPackage
		err := dec.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

func productionDeps(t *testing.T, patterns ...string) []string {
	t.Helper()

	args := append([]string{"list", "-deps", "-json"}, patterns...)
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var deps []string
	for {
		var pkg listedPackage
		err := dec.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		deps = append(deps, pkg.ImportPath)
	}
	return deps
}

func productionImports(t *testing.T, pattern string) []string {
	t.Helper()

	args := []string{"list", "-json", pattern}
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}

	var pkg listedPackage
	if err := json.Unmarshal(out, &pkg); err != nil {
		t.Fatalf("decode go list output: %v", err)
	}
	return pkg.Imports
}

func allowSet(imports ...string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(imports))
	for _, imp := range imports {
		allowed[imp] = struct{}{}
	}
	return allowed
}

func copyAllowSet(base map[string]struct{}, imports ...string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(base)+len(imports))
	for imp := range base {
		allowed[imp] = struct{}{}
	}
	for _, imp := range imports {
		allowed[imp] = struct{}{}
	}
	return allowed
}

func assertModuleImportsAllowed(t *testing.T, pkg string, imports []string, allowed map[string]struct{}) {
	t.Helper()

	for _, imp := range imports {
		if !strings.HasPrefix(imp, modulePath+"/") {
			continue
		}
		if _, ok := allowed[imp]; !ok {
			t.Fatalf("%s imports forbidden dependency %q", pkg, imp)
		}
	}
}

func assertValuePackageAxisImports(t *testing.T, pkg string, allowedLeaves map[string]struct{}) {
	t.Helper()

	for _, imp := range productionImports(t, pkg) {
		if !strings.HasPrefix(imp, modulePath+"/analysis/domain/value/axis/") {
			continue
		}
		if _, ok := allowedLeaves[imp]; !ok {
			t.Fatalf("%s imports unapproved axis leaf %q", pkg, imp)
		}
	}
}

func forbiddenImport(dep, banned string, exactly bool) bool {
	if exactly {
		return dep == banned
	}
	return dep == banned || strings.HasPrefix(dep, banned+"/")
}

package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

func TestFrontDraftWireOwnershipStaysDisplaced(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/...")
	owned := map[string]bool{
		filepath.Join(loader.root, "analysis", "check", "fixpoint", "front", "wire_codec.go"): true,
	}
	forbiddenProtocols := []string{
		"front/branch-predicate/v1/",
		"front/branch-evidence/v1/",
		"front/branch-diff/v1/",
		"provider/module/v1/",
		"effect.lifecycle.channel/",
		"effect.lifecycle.channel.display/",
		"effect.lifecycle.resource/",
	}
	forbiddenPayloads := []string{"scalar/", "shape/"}
	owners := []fenceOwnedType{
		loader.ownedType(modulePath+"/analysis/check/fixpoint/front", "BranchPredicateWire"),
		loader.ownedType(modulePath+"/analysis/check/fixpoint/front", "BranchDiffWire"),
		loader.ownedType(modulePath+"/analysis/check/fixpoint/front", "BranchChainPathWire"),
		loader.ownedType(modulePath+"/analysis/check/fixpoint/front", "BranchChainCheckWire"),
		loader.ownedType(modulePath+"/analysis/check/fixpoint/front", "BranchChainWire"),
		loader.ownedType(modulePath+"/analysis/check/fixpoint/front", "ModuleProviderWire"),
		loader.ownedType(modulePath+"/analysis/check/fixpoint/shapefact", "Payload"),
		loader.ownedType(modulePath+"/analysis/check/fixpoint/shapefact", "Scalar"),
		loader.ownedType(modulePath+"/analysis/check/fixpoint/shapefact", "Claim"),
		loader.ownedType(modulePath+"/analysis/domain/typestate", "Publication"),
	}
	for _, meta := range loader.modulePackages("/analysis/check/") {
		typed := loader.load(meta)
		for _, construction := range fenceDisplacedRepresentationConstructions(typed, owners) {
			t.Errorf("%s", construction)
		}
		switch meta.ImportPath {
		case modulePath + "/analysis/check/fixpoint/shapefact",
			modulePath + "/analysis/check/fixpoint/factkey":
			continue
		}
		for _, construction := range fenceSemanticTexts(typed, forbiddenProtocols, owned, false) {
			t.Errorf("%s reconstructs a codec-owned wire/payload/lifecycle value", construction)
		}
		for _, construction := range fenceSemanticTexts(typed, forbiddenPayloads, owned, true) {
			t.Errorf("%s reconstructs a codec-owned wire/payload/lifecycle value", construction)
		}
		for _, classification := range fenceDecodeTargetClassifications(typed) {
			t.Errorf("%s classifies a payload through DecodeTarget; switch on shapefact.Decode", classification)
		}
	}
}

func TestRepresentationFenceRejectsAliasesAndHelperResults(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/...")
	tests := []struct {
		name   string
		owner  fenceOwnedType
		source string
	}{
		{
			name:  "wire defined mirror",
			owner: loader.ownedType(modulePath+"/analysis/check/fixpoint/front", "ModuleProviderWire"),
			source: `package engine
import "` + modulePath + `/analysis/check/fixpoint/front"
type localProviderWire front.ModuleProviderWire
`,
		},
		{
			name:  "wire helper result",
			owner: loader.ownedType(modulePath+"/analysis/check/fixpoint/front", "ModuleProviderWire"),
			source: "package engine\n" +
				"type localProviderWire struct {\n" +
				"\tModule string `json:\"module\"`\n" +
				"\tSuffix string `json:\"suffix,omitempty\"`\n" +
				"}\n" +
				"func localProvider() localProviderWire { return localProviderWire{} }\n" +
				"var duplicatedProvider = localProvider()\n",
		},
		{
			name:  "payload defined mirror",
			owner: loader.ownedType(modulePath+"/analysis/check/fixpoint/shapefact", "Payload"),
			source: `package engine
import "` + modulePath + `/analysis/check/fixpoint/shapefact"
type localPayload shapefact.Payload
`,
		},
		{
			name:  "payload defined mirror helper",
			owner: loader.ownedType(modulePath+"/analysis/check/fixpoint/shapefact", "Scalar"),
			source: `package engine
import "` + modulePath + `/analysis/check/fixpoint/shapefact"
type localScalar shapefact.Scalar
func localPayload() localScalar { return localScalar{} }
var duplicatedPayload = localPayload()
`,
		},
		{
			name:  "lifecycle defined mirror",
			owner: loader.ownedType(modulePath+"/analysis/domain/typestate", "Publication"),
			source: `package engine
import "` + modulePath + `/analysis/domain/typestate"
type localLifecyclePublication typestate.Publication
`,
		},
		{
			name:  "lifecycle defined mirror helper",
			owner: loader.ownedType(modulePath+"/analysis/domain/typestate", "Publication"),
			source: `package engine
import "` + modulePath + `/analysis/domain/typestate"
type localLifecyclePublication typestate.Publication
func localLifecycle() localLifecyclePublication { return localLifecyclePublication{} }
var duplicatedLifecycle = localLifecycle()
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			typed := loader.source(modulePath+"/analysis/check/engine", test.source)
			if constructions := fenceDisplacedRepresentationConstructions(typed, []fenceOwnedType{test.owner}); len(constructions) == 0 {
				t.Fatal("type-based representation fence accepted displaced construction")
			}
		})
	}
}

func TestProtocolConstructionFenceRejectsSemanticEvasions(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/...")
	tests := []struct {
		name      string
		forbidden string
		source    string
	}{
		{
			name:      "rescan4 wire local alias",
			forbidden: "front/branch-predicate/v1/",
			source: `package engine
import "strings"
func mutation(value string) bool {
	wireRoot := "front/"
	return strings.HasPrefix(value, wireRoot+"branch-predicate/v1/")
}`,
		},
		{
			name:      "wire typed helper result",
			forbidden: "front/branch-evidence/v1/",
			source: `package engine
import "strings"
type wireRootText string
func wireRoot() wireRootText { return wireRootText("front/") }
func mutation(value string) bool {
	return strings.HasPrefix(value, string(wireRoot())+"branch-evidence/v1/")
}`,
		},
		{
			name:      "wire strings join",
			forbidden: "provider/module/v1/",
			source: `package engine
import "strings"
func mutation(value string) bool {
	prefix := strings.Join([]string{"provider", "module", "v1", ""}, "/")
	return strings.HasPrefix(value, prefix)
}`,
		},
		{
			name:      "rescan5 wire strings builder",
			forbidden: "front/branch-predicate/v1/",
			source: `package engine
import "strings"
func mutation(value string) bool {
	var wire strings.Builder
	wire.WriteString("front/")
	wire.WriteString("branch-predicate/v1/")
	return strings.HasPrefix(value, wire.String())
}`,
		},
		{
			name:      "rescan4 lifecycle local alias",
			forbidden: "effect.lifecycle.resource/",
			source: `package engine
import "strings"
func mutation(value string) bool {
	lifecycleRoot := "effect.lifecycle."
	return strings.HasPrefix(value, lifecycleRoot+"resource/")
}`,
		},
		{
			name:      "lifecycle helper result",
			forbidden: "effect.lifecycle.channel/",
			source: `package engine
import "strings"
func lifecycleRoot() string { return "effect.lifecycle." }
func mutation(value string) bool {
	return strings.HasPrefix(value, lifecycleRoot()+"channel/")
}`,
		},
		{
			name:      "lifecycle sprintf result",
			forbidden: "effect.lifecycle.channel.display/",
			source: `package engine
import (
	"fmt"
	"strings"
)
func mutation(value string) bool {
	return strings.HasPrefix(value, fmt.Sprintf("%s%s", "effect.lifecycle.", "channel.display/"))
}`,
		},
		{
			name:      "payload byte composite",
			forbidden: "scalar/",
			source: `package engine
func mutation() []byte {
	return append([]byte{'s', 'c', 'a', 'l', 'a', 'r', '/'}, []byte("number/1")...)
}`,
		},
		{
			name:      "payload typed helper result",
			forbidden: "shape/",
			source: `package engine
type payloadBytes []byte
func payloadRoot() payloadBytes { return payloadBytes("shape/") }
func mutation() []byte { return append(payloadRoot(), []byte("target/v1/value")...) }
`,
		},
		{
			name:      "payload strings join",
			forbidden: "scalar/",
			source: `package engine
import "strings"
func mutation() string { return strings.Join([]string{"scalar", "resource", "id"}, "/") }
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			typed := loader.source(modulePath+"/analysis/check/engine", test.source)
			prefixOnly := test.forbidden == "scalar/" || test.forbidden == "shape/"
			if found := fenceSemanticTexts(typed, []string{test.forbidden}, nil, prefixOnly); len(found) == 0 {
				t.Fatalf("semantic construction fence accepted %q", test.forbidden)
			}
		})
	}
}

func wireFenceRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil && !info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find repository go.mod")
		}
		directory = parent
	}
}

func TestBranchKernelSurfacesMalformedPredicateWire(t *testing.T) {
	operation := equation.BoundEquation{
		Target: equation.Coordinate{Name: "branch"},
		Operands: []equation.BoundOperand{{
			Role:  equation.MustOperandRole("predicate"),
			Value: []byte("front/branch-predicate/v1/{"),
		}},
	}
	if _, err := branchSelectionKernel(operation, equation.Partition{}); err == nil {
		t.Fatal("malformed predicate wire was silently treated as absent semantics")
	}
}

func TestBranchNumericConsumerSurfacesMalformedDifferenceWire(t *testing.T) {
	operation := equation.BoundEquation{Operands: []equation.BoundOperand{{
		Role:  equation.MustOperandRole("difference-00000000"),
		Value: []byte("front/branch-diff/v1/{"),
	}}}
	if _, _, err := branchNumericTruth(operation, equation.Partition{}); err == nil {
		t.Fatal("malformed difference wire was silently treated as no relation")
	}
}

func TestEngineAdmissionSurfacesMalformedModuleProviderWire(t *testing.T) {
	compilation := front.Compilation{Artifact: equation.Artifact{Equations: []equation.Equation{{
		Target: equation.Coordinate{Name: "call"},
		Operands: []equation.Operand{{
			Role: equation.MustOperandRole("provider"),
			Term: equation.ClosedTerm([]byte("provider/module/v1/not-base64")),
		}},
	}}}}
	if _, _, err := evaluateCheck(compilation, equation.EntryBinding{}, nil, nil); err == nil {
		t.Fatal("malformed module-provider wire was silently treated as an unresolved provider")
	}
}

package engine

import (
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypedOperandRolesStayDisplaced(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/...")
	for _, path := range []string{
		modulePath + "/analysis/check/engine",
		modulePath + "/analysis/check/fixpoint/front",
		modulePath + "/analysis/check/exporter",
	} {
		meta, ok := loader.metas[path]
		if !ok {
			t.Fatalf("semantic role fence package %s not found", path)
		}
		for _, parser := range fenceRawRoleParsers(loader.load(meta)) {
			t.Errorf("%s reconstructs an OperandRole display value with a string parser", parser)
		}
	}
}

func TestTypedOperandRoleFenceRejectsAssignmentEvasions(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/...")
	compileFailures := map[string]string{
		"direct string cast": `package engine
import "` + modulePath + `/analysis/check/fixpoint/equation"
var regression = equation.OperandRole("result-00000000")
`,
		"implicit string assignment": `package engine
import "` + modulePath + `/analysis/check/fixpoint/equation"
var regression equation.OperandRole = "result-00000000"
`,
		"named string conversion": `package engine
import "` + modulePath + `/analysis/check/fixpoint/equation"
type roleText string
func resurrect(operand equation.Operand) roleText {
	return roleText(operand.Role)
}`,
	}
	for name, source := range compileFailures {
		t.Run(name, func(t *testing.T) {
			if err := loader.sourceError(modulePath+"/analysis/check/engine", source); err == nil {
				t.Fatal("opaque role boundary compiled a string construction or conversion")
			}
		})
	}

	tests := map[string]string{
		"rescan4 two-stage display alias": `package engine
import (
	"strings"
	"` + modulePath + `/analysis/check/fixpoint/equation"
)
func resurrect(operand equation.Operand) bool {
	text := operand.Role.Wire()
	alias := text
	return strings.HasPrefix(alias, "result-")
}`,
		"typed role then display": `package engine
import (
	"strings"
	"` + modulePath + `/analysis/check/fixpoint/equation"
)
func resurrect(operand equation.Operand) bool {
	role := operand.Role
	text := role.Wire()
	return strings.TrimPrefix(text, "result-") != text
}`,
		"helper returns role display": `package engine
import (
	"strings"
	"` + modulePath + `/analysis/check/fixpoint/equation"
)
func roleText(role equation.OperandRole) string { return role.Wire() }
func resurrect(operand equation.Operand) bool {
	text := roleText(operand.Role)
	return strings.HasPrefix(text, "result-")
}`,
		"parser function alias": `package engine
import (
	"strings"
	"` + modulePath + `/analysis/check/fixpoint/equation"
)
func resurrect(operand equation.Operand) bool {
	parse := strings.HasPrefix
	text := operand.Role.Wire()
	return parse(text, "result-")
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			typed := loader.source(modulePath+"/analysis/check/engine", source)
			if parsers := fenceRawRoleParsers(typed); len(parsers) == 0 {
				t.Fatal("semantic role fence accepted raw role reconstruction")
			}
		})
	}
}

func TestChannelSelectConsumersStayOnFrontOperands(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/engine")
	meta := loader.metas[modulePath+"/analysis/check/engine"]
	typed := loader.load(meta)
	if found := fenceReachableStringParserLiterals(typed, "channelSelectCoverageConsumer", ".channel"); len(found) != 0 {
		t.Fatalf("channel-select consumer reconstructs path topology by suffix: %v", found)
	}
	source, err := os.ReadFile(filepath.Join(meta.Dir, "channel_select_consumers.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"compilation.WIR", "ForEachIfChainDescriptor", "BranchChecks("} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("channel-select consumer resurrected %q", forbidden)
		}
	}
}

func TestChannelSelectFenceRejectsTopologyAndSuffixReconstruction(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/engine")
	tests := map[string]string{
		"rescan4 split suffix": `package engine
import "strings"
func channelSelectCoverageConsumer(path string) bool {
	return strings.TrimSuffix(path, "."+"channel") != path
}`,
		"suffix local alias": `package engine
import "strings"
func channelSelectCoverageConsumer(path string) bool {
	suffix := ".channel"
	return strings.HasSuffix(path, suffix)
}`,
		"suffix helper": `package engine
import "strings"
func channelSuffix() string { return "."+"channel" }
func channelSelectCoverageConsumer(path string) bool {
	return strings.TrimSuffix(path, channelSuffix()) != path
}`,
		"suffix join": `package engine
import "strings"
func channelSelectCoverageConsumer(path string) bool {
	suffix := strings.Join([]string{"", "channel"}, ".")
	return strings.HasSuffix(path, suffix)
}`,
		"parser function alias": `package engine
import "strings"
func channelSelectCoverageConsumer(path string) bool {
	parse := strings.TrimSuffix
	suffix := ".channel"
	return parse(path, suffix) != path
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			typed := loader.source(modulePath+"/analysis/check/engine", source)
			if found := fenceReachableStringParserLiterals(typed, "channelSelectCoverageConsumer", ".channel"); len(found) == 0 {
				t.Fatal("semantic channel fence accepted suffix reconstruction")
			}
		})
	}
}

func TestNativeRecognitionStaysBinderAndRegistryOwned(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/fixpoint/front")
	meta := loader.metas[modulePath+"/analysis/check/fixpoint/front"]
	typed := loader.load(meta)
	if found := fenceReachableRawASTNameComparisons(typed, "nativeBoundStdlibCall"); len(found) != 0 {
		t.Fatalf("native recognition compares raw AST names: %v", found)
	}
	source, err := os.ReadFile(filepath.Join(meta.Dir, "native_operations.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"nativeDirectCall(", "nativeMemberCall("} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("native recognition resurrected %q", forbidden)
		}
	}
	for _, required := range []string{"GlobalIdentity(", "RegistryIdentity("} {
		if !strings.Contains(string(source), required) {
			t.Fatalf("native recognition omitted %q", required)
		}
	}
}

func TestNativeRecognitionFenceRejectsSemanticNameEvasions(t *testing.T) {
	loader := newFencePackageLoader(t, "./analysis/check/fixpoint/front")
	t.Run("consumer requires registry identity by type", func(t *testing.T) {
		meta := loader.metas[modulePath+"/analysis/check/fixpoint/front"]
		typed := loader.load(meta)
		function, ok := typed.pkg.Scope().Lookup("nativeBoundStdlibCall").(*types.Func)
		if !ok {
			t.Fatal("nativeBoundStdlibCall is absent")
		}
		signature, _ := function.Type().(*types.Signature)
		if signature == nil || signature.Params().Len() != 3 ||
			!fenceNamedType(signature.Params().At(2).Type(), modulePath+"/analysis/module/signaturelookup", "Identity") {
			t.Fatal("nativeBoundStdlibCall accepts source text instead of opaque registry identity")
		}
	})

	t.Run("rescan5 fmt.Sprint before pcall", func(t *testing.T) {
		source := `package front
import (
	"fmt"
	"` + modulePath + `/analysis/module/signaturelookup"
	luaast "` + modulePath + `/compiler/ast"
)
func consume(identity signaturelookup.Identity) {}
func mutation(id *luaast.IdentExpr) {
	consume(fmt.Sprint(id.Value))
}`
		if err := loader.sourceError(modulePath+"/analysis/check/fixpoint/front", source); err == nil {
			t.Fatal("native consumer accepted raw formatted AST text in place of registry identity")
		}
	})

	tests := map[string]string{
		"rescan4 split constant": `package front
import luaast "` + modulePath + `/compiler/ast"
func nativeBoundStdlibCall(id *luaast.IdentExpr) bool {
	return id.Value == "p"+"call"
}`,
		"two-stage AST name alias": `package front
import luaast "` + modulePath + `/compiler/ast"
func nativeBoundStdlibCall(id *luaast.IdentExpr) bool {
	text := id.Value
	alias := text
	return alias == "pcall"
}`,
		"helper returns AST name": `package front
import luaast "` + modulePath + `/compiler/ast"
func rawName(id *luaast.IdentExpr) string { return id.Value }
func nativeBoundStdlibCall(id *luaast.IdentExpr) bool {
	return rawName(id) == "pcall"
}`,
		"typed AST name alias": `package front
import luaast "` + modulePath + `/compiler/ast"
type nativeName string
func nativeBoundStdlibCall(id *luaast.IdentExpr) bool {
	text := nativeName(id.Value)
	return text == nativeName("pcall")
}`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			typed := loader.source(modulePath+"/analysis/check/fixpoint/front", source)
			if found := fenceReachableRawASTNameComparisons(typed, "nativeBoundStdlibCall"); len(found) == 0 {
				t.Fatal("semantic native-recognition fence accepted raw AST name comparison")
			}
		})
	}
}

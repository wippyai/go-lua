package manifest

import (
	"strings"
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func TestProjectMethodSelectsCallableExportAndRekeysEffects(t *testing.T) {
	create := typ.Func().Param("name", typ.String).Returns(typ.Number).Build()
	remove := typ.Func().Param("id", typ.Number).Returns(typ.Boolean).Build()
	m := New("shared/body")
	m.SetExport(typetable.NewRecord().Field("create", create).Field("remove", remove).Build())
	m.DefineFunctionSignature("shared/body.create", signature.Function{Type: create})
	m.DefineFunctionSignature("shared/body.remove", signature.Function{Type: remove})
	m.DefineCallbackPhaseRegistration("shared/body.create", 0, "before")

	got, err := m.ProjectMethod("entry:create", "create")
	if err != nil {
		t.Fatalf("ProjectMethod: %v", err)
	}
	if got.Path != "entry:create" || !typ.TypeEquals(got.Export, create) {
		t.Fatalf("projection = path %q export %v", got.Path, got.Export)
	}
	if _, ok := got.FunctionSignatures["entry:create"]; !ok || len(got.FunctionSignatures) != 1 {
		t.Fatalf("projected signatures = %#v", got.FunctionSignatures)
	}
	if len(got.CallbackPhaseRegistrations) != 1 || got.CallbackPhaseRegistrations[0].Function != "entry:create" {
		t.Fatalf("projected callbacks = %#v", got.CallbackPhaseRegistrations)
	}
	if !typ.TypeEquals(m.Export, typetable.NewRecord().Field("create", create).Field("remove", remove).Build()) || len(m.FunctionSignatures) != 2 {
		t.Fatal("projection mutated the shared module manifest")
	}
}

func TestProjectMethodRejectsMissingMethod(t *testing.T) {
	m := New("shared/body")
	m.SetExport(typetable.NewRecord().Field("present", typ.Func().Build()).Build())
	_, err := m.ProjectMethod("entry:missing", "missing")
	if err == nil || !strings.Contains(err.Error(), `no exported method "missing"`) {
		t.Fatalf("missing method error = %v", err)
	}
}

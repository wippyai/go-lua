package checktest

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

func TestAmbientGlobalManifestIntersectionExposesMethodsAndFields(t *testing.T) {
	osManifest := manifest.New("os")
	osFields := typetable.NewRecord().
		Field("platform", typ.String).
		Build()
	osMethods := typ.NewInterface("os", []typ.Method{
		{Name: "time", Type: typ.Func().Returns(typ.Number).Build()},
	})
	osManifest.SetExport(typeexpr.Intersection(osFields, osMethods))

	result := Check(`
local now: number = os.time()
local platform: string = os.platform
`, WithStdlib(), WithManifest("os", osManifest), WithGlobals("os"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want ambient manifest global methods and fields accepted", result.Diagnostics)
	}
}

func TestAmbientGlobalManifestNestedFieldExposesInterfaceMethods(t *testing.T) {
	processManifest := manifest.New("process")
	registryFields := typetable.NewRecord().
		Field("LOCAL", typ.Number).
		Build()
	registryMethods := typ.NewInterface("process.registry", []typ.Method{
		{Name: "lookup", Type: typ.Func().Param("name", typ.String).Returns(typ.String).Build()},
	})
	processManifest.SetExport(typetable.NewRecord().
		Field("registry", typeexpr.Intersection(registryFields, registryMethods)).
		Build())

	result := Check(`
local pid: string = process.registry.lookup("service")
local scope: number = process.registry.LOCAL
`, WithStdlib(), WithManifest("process", processManifest), WithGlobals("process"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nested ambient manifest methods and fields accepted", result.Diagnostics)
	}
}

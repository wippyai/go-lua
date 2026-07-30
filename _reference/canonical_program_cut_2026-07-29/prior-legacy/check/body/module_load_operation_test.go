package body

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/module/importlookup"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestPreparedPlanCompilesDynamicModuleLoadAsModuleOnlyExternal(t *testing.T) {
	stmts, err := parse.ParseString(`local module_name = "alpha"; return require(module_name)`, "module-load-plan.lua")
	if err != nil {
		t.Fatal(err)
	}
	alphaOld := manifest.New("alpha")
	alphaOld.SetExport(typ.String)
	zeta := manifest.New("zeta")
	zeta.SetExport(typ.Any)
	alphaEffective := manifest.New("alpha")
	alphaEffective.SetExport(typ.Number)
	reg := standard.Registry()
	config := Config{
		Registry: reg, TypeValues: typevalue.NewCache(), Globals: []string{"require"},
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("module-load-plan")),
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{alphaOld, zeta, alphaEffective}},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: Globals(config)})
	prepared, err := PrepareBoundChunk(stmts, bindings, config)
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.OperationPlan()
	var pointFound bool
	for rawPoint := 0; rawPoint < plan.PointCount(); rawPoint++ {
		point := cfg.Point(rawPoint)
		operation, ok := plan.ModuleLoadOperation(point)
		if !ok {
			continue
		}
		pointFound = true
		site, ok := plan.Facts().CallSiteView(point)
		if !ok {
			t.Fatalf("module operation point %d has no call site", point)
		}
		argument, ok := site.ArgumentSourceAt(0)
		if !ok || operation.Argument() != argument || operation.Argument().Kind == factflow.ValueSourceLiteral {
			t.Fatalf("module argument = %#v, site argument %#v/%v; want exact dynamic source", operation.Argument(), argument, ok)
		}
		exports := operation.Exports()
		if len(exports) != 2 || exports[0].Path != "alpha" || exports[1].Path != "zeta" {
			t.Fatalf("effective export table = %#v", exports)
		}
		alpha, _ := operation.LookupExport("alpha")
		alphaType, exact := typevalue.TypeOf(reg, alpha.Value)
		if !exact || !typ.TypeEquals(alphaType, typ.Number) || !alpha.PostReturnAuthority {
			t.Fatalf("effective alpha export = %v exact=%v authority=%v", alphaType, exact, alpha.PostReturnAuthority)
		}
		zeta, _ := operation.LookupExport("zeta")
		if zeta.PostReturnAuthority {
			t.Fatal("any export incorrectly claimed post-return authority")
		}
		surface, ok := plan.CallSurface()
		if !ok {
			t.Fatal("call surface unavailable")
		}
		surfaceSite, ok := surface.Site(point)
		if !ok || surfaceSite.Target.Kind() != operationplan.CallSurfaceTargetExternal {
			t.Fatalf("module-only site = %#v/%v", surfaceSite, ok)
		}
		if _, present := surfaceSite.Target.ExternalOperation(); present {
			t.Fatal("module-only site fabricated signature content")
		}
		if moduleID, present := surfaceSite.Target.ModuleLoadContentID(); !present || moduleID != operation.ContentID() {
			t.Fatal("module-only external site lost Plan-owned operation identity")
		}
	}
	if !pointFound {
		t.Fatal("prepared plan omitted dynamic module-load operation")
	}
}

func TestPreparedPlanKeepsSignatureBeforeModuleLoadSupplement(t *testing.T) {
	stmts, err := parse.ParseString(`return require("alpha")`, "module-load-composite.lua")
	if err != nil {
		t.Fatal(err)
	}
	alpha := manifest.New("alpha")
	alpha.SetExport(typ.String)
	reg := standard.Registry()
	config := Config{
		Registry: reg, TypeValues: typevalue.NewCache(),
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("module-load-composite")),
		Signatures:    signaturelookup.Source{IncludeStdlib: true},
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{alpha}},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: Globals(config)})
	prepared, err := PrepareBoundChunk(stmts, bindings, config)
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.OperationPlan()
	surface, ok := plan.CallSurface()
	if !ok {
		t.Fatal("call surface unavailable")
	}
	for _, site := range surface.Sites() {
		signatureOperation, hasSignature := plan.SignatureCallOperation(site.Point)
		moduleOperation, hasModule := plan.ModuleLoadOperation(site.Point)
		if !hasModule {
			continue
		}
		if !hasSignature || !site.Target.MatchesExternalOperation(signatureOperation) || !site.Target.MatchesModuleLoadOperation(moduleOperation) {
			t.Fatalf("composite require point %d lost producer: signature=%v module=%v target=%#v", site.Point, hasSignature, hasModule, site.Target)
		}
		if moduleOperation.ResultIndex() != 0 {
			t.Fatalf("module result index = %d, want zero", moduleOperation.ResultIndex())
		}
		return
	}
	t.Fatal("prepared plan omitted composite require operation")
}

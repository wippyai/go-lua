package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestModuleLoadOperationOwnsCanonicalTableAndExactDynamicSource(t *testing.T) {
	reg := standard.Registry()
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	argument, ok := factflow.NewPathValueSource("7:module_name", 3, 0, 0, shape)
	if !ok {
		t.Fatal("dynamic path source rejected")
	}
	input := []ModuleLoadExport{
		{Path: "zeta", Value: typevalue.FromType(reg, typ.Any)},
		{Path: "alpha", Value: typevalue.String(reg), PostReturnAuthority: true},
	}
	operation, ok := NewModuleLoadOperation(reg, argument, input)
	if !ok {
		t.Fatal("module-load operation rejected")
	}
	input[0].Path = "mutated"
	if operation.Argument() != argument || operation.Argument().Kind == factflow.ValueSourceLiteral {
		t.Fatalf("argument = %#v, want exact dynamic source %#v", operation.Argument(), argument)
	}
	exports := operation.Exports()
	if len(exports) != 2 || exports[0].Path != "alpha" || exports[1].Path != "zeta" ||
		!exports[0].PostReturnAuthority || exports[1].PostReturnAuthority {
		t.Fatalf("exports = %#v, want canonical table with conditional authority", exports)
	}
	exports[0].Path = "escaped"
	if item, ok := operation.LookupExport("alpha"); !ok || item.Path != "alpha" {
		t.Fatalf("operation table escaped through Exports: %#v/%v", item, ok)
	}

	reordered, ok := NewModuleLoadOperation(reg, argument, []ModuleLoadExport{
		{Path: "alpha", Value: typevalue.String(reg), PostReturnAuthority: true},
		{Path: "zeta", Value: typevalue.FromType(reg, typ.Any)},
	})
	if !ok || reordered.ContentID() != operation.ContentID() {
		t.Fatal("input ordering changed canonical module-load identity")
	}
	otherSource, _ := factflow.NewPathValueSource("8:module_name", 3, 0, 0, shape)
	different, ok := NewModuleLoadOperation(reg, otherSource, operation.Exports())
	if !ok || different.ContentID() == operation.ContentID() {
		t.Fatal("exact argument source was absent from operation identity")
	}
}

func TestModuleLoadOperationResolvesEvaluatedArgumentWithoutCallback(t *testing.T) {
	reg := standard.Registry()
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	source, _ := factflow.NewPathValueSource("7:module_name", 0, 0, 0, shape)
	operation, ok := NewModuleLoadOperation(reg, source, []ModuleLoadExport{
		{Path: "strong", Value: typevalue.String(reg), PostReturnAuthority: true},
		{Path: "weak", Value: typevalue.FromType(reg, typ.Any)},
	})
	if !ok {
		t.Fatal("module operation rejected")
	}
	strong, ok := operation.ResolveArgument(reg, typevalue.LiteralString(reg, "strong"))
	if !ok || !strong.Matches(operation) || strong.ResultIndex() != ModuleLoadResultIndex ||
		!strong.PostReturnAuthority() || !product.Equal(reg, strong.Value(), typevalue.String(reg)) {
		t.Fatalf("strong resolution = %#v/%v", strong, ok)
	}
	weak, ok := operation.ResolveArgument(reg, typevalue.LiteralString(reg, "weak"))
	if !ok || !weak.Matches(operation) || weak.PostReturnAuthority() {
		t.Fatalf("weak resolution = %#v/%v", weak, ok)
	}
	for name, argument := range map[string]product.Value{
		"missing":    typevalue.LiteralString(reg, "missing"),
		"nonliteral": typevalue.String(reg),
		"nonstr":     typevalue.FromType(reg, typ.Number),
	} {
		if result, resolved := operation.ResolveArgument(reg, argument); resolved || result.Matches(operation) {
			t.Fatalf("%s argument resolved: %#v/%v", name, result, resolved)
		}
	}
}

func TestPlanOwnsDetachedModuleLoadOperation(t *testing.T) {
	reg := standard.Registry()
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	argument, _ := factflow.NewStringLiteralValueSource("alpha", 0, 0, 0, shape)
	operation, ok := NewModuleLoadOperation(reg, argument, []ModuleLoadExport{{
		Path: "alpha", Value: typevalue.FromType(reg, typ.Number), PostReturnAuthority: true,
	}})
	if !ok {
		t.Fatal("module-load operation rejected")
	}
	plan := New(testCallSurfaceGraph(4), factflow.FactsInput{}).WithModuleLoads(map[cfg.Point]ModuleLoadOperation{1: operation, 2: operation})
	first, ok := plan.ModuleLoadOperation(1)
	if !ok || !first.ContentID().Available() {
		t.Fatalf("point 1 operation = %#v/%v", first, ok)
	}
	firstExports := first.Exports()
	firstExports[0].Path = "escaped"
	second, ok := plan.ModuleLoadOperation(2)
	if !ok {
		t.Fatal("point 2 operation missing")
	}
	if item, found := second.LookupExport("alpha"); !found || item.Path != "alpha" {
		t.Fatalf("plan-owned operation escaped: %#v/%v", item, found)
	}
	if _, ok := plan.ModuleLoadOperation(3); ok {
		t.Fatal("absent point published a module-load operation")
	}
}

func TestPlanSharesOneModuleLoadExportTableAuthorityAcrossSites(t *testing.T) {
	reg := standard.Registry()
	table, ok := NewModuleLoadExportTable(reg, []ModuleLoadExport{
		{Path: "beta", Value: typevalue.String(reg), PostReturnAuthority: true},
		{Path: "alpha", Value: typevalue.FromType(reg, typ.Number), PostReturnAuthority: true},
	})
	if !ok || table.authority == nil {
		t.Fatal("shared export table rejected")
	}
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	firstArgument, _ := factflow.NewPathValueSource("7:first", 0, 0, 0, shape)
	secondArgument, _ := factflow.NewPathValueSource("8:second", 0, 0, 0, shape)
	thirdArgument, _ := factflow.NewPathValueSource("9:third", 0, 0, 0, shape)
	firstInput, ok := NewModuleLoadOperationWithTable(firstArgument, table)
	if !ok {
		t.Fatal("first table-backed operation rejected")
	}
	secondInput, ok := NewModuleLoadOperationWithTable(secondArgument, table)
	if !ok {
		t.Fatal("second table-backed operation rejected")
	}
	thirdInput, ok := NewModuleLoadOperation(reg, thirdArgument, table.Exports())
	if !ok || thirdInput.table.authority == table.authority {
		t.Fatal("independently allocated third operation was not independent before Plan ownership")
	}
	plan := New(testCallSurfaceGraph(5), factflow.FactsInput{}).WithModuleLoads(map[cfg.Point]ModuleLoadOperation{
		1: firstInput, 2: secondInput, 3: thirdInput,
	})
	first, _ := plan.ModuleLoadOperation(1)
	second, _ := plan.ModuleLoadOperation(2)
	third, _ := plan.ModuleLoadOperation(3)
	if first.table.authority != table.authority || second.table.authority != table.authority ||
		first.table.authority != second.table.authority || third.table.authority != first.table.authority {
		t.Fatal("Plan cloned the full export table per module-load site")
	}
	if first.ContentID() == second.ContentID() || first.ExportTable().ContentID() != second.ExportTable().ContentID() {
		t.Fatal("per-site argument identity or shared table identity was lost")
	}
	target, ok := NewModuleLoadCallSurfaceTarget(first)
	if !ok {
		t.Fatal("module-only call-surface target rejected")
	}
	if targetID, present := target.ModuleLoadContentID(); !present || targetID != first.ContentID() {
		t.Fatalf("call surface identity = %x/%v, want %x", targetID, present, first.ContentID())
	}
}

func TestCallSurfacePreservesModuleOnlyAndCompositeExternalProducers(t *testing.T) {
	reg := standard.Registry()
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	argument, _ := factflow.NewStringLiteralValueSource("alpha", 0, 0, 0, shape)
	moduleOperation, ok := NewModuleLoadOperation(reg, argument, []ModuleLoadExport{{
		Path: "alpha", Value: typevalue.String(reg), PostReturnAuthority: true,
	}})
	if !ok {
		t.Fatal("module operation rejected")
	}
	moduleOnly, ok := NewModuleLoadCallSurfaceTarget(moduleOperation)
	if !ok || moduleOnly.Kind() != CallSurfaceTargetExternal || !moduleOnly.MatchesModuleLoadOperation(moduleOperation) {
		t.Fatalf("module-only target = %#v/%v", moduleOnly, ok)
	}
	if _, signaturePresent := moduleOnly.ExternalOperation(); signaturePresent {
		t.Fatal("module-only target fabricated a signature operation")
	}

	signatureOperation, ok := NewSignatureCallOperation(signature.Function{Type: typ.Func().Param("path", typ.String).Returns(typ.Any).Build()})
	if !ok {
		t.Fatal("signature operation rejected")
	}
	composite, ok := NewCompositeExternalCallSurfaceTarget(signatureOperation, moduleOperation)
	if !ok || !composite.MatchesExternalOperation(signatureOperation) || !composite.MatchesModuleLoadOperation(moduleOperation) {
		t.Fatalf("composite target lost a producer: %#v/%v", composite, ok)
	}
	ownedSignature, _ := composite.ExternalOperation()
	moduleID, hasModule := composite.ModuleLoadContentID()
	if ownedSignature.Signature().Type == nil || !hasModule || moduleID != moduleOperation.ContentID() {
		t.Fatal("composite producer descriptors are incomplete")
	}
}

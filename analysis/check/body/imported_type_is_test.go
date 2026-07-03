package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPrepareImportedTypeIsMemberCalleePublishesResultSlots(t *testing.T) {
	reg := standard.Registry()
	appError := typetable.NewRecord().
		Field("code", typ.String).
		Field("message", typ.String).
		Build()
	errorsManifest := manifest.New("errors")
	errorsManifest.Types["AppError"] = appError
	errorsManifest.SetExport(typetable.NewRecord().
		StaticStringIndex("AppError", typ.NewMeta(appError)).
		Build())

	prepared, err := PrepareChunk(
		parseChunk(t, `
local errors = require("errors")
local raw: any = {}
local validated, err = errors.AppError:is(raw)
if err == nil and validated then
	local code: string = validated.code
end
`),
		Config{
			Registry: reg,
			Globals:  []string{"require"},
			ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{
				errorsManifest,
			}},
			ModuleTypes: typelookup.Source{Manifests: []*manifest.Manifest{errorsManifest}},
		},
	)
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}

	result := solvePreparedForTest(t, prepared, SolveConfig{})
	callPoint := requireCallSiteByCalleePath(t, result, "errors.AppError.is")
	values := prepared.facts.CallResultValues(callPoint)
	if len(values) != 2 {
		t.Fatalf("call result values = %d, want 2: %#v", len(values), values)
	}
	gotType, ok := typevalue.TypeOf(reg, values[0].Value())
	wantType := typ.MaterializeOptional(appError)
	if !ok || !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("type witness = %v/%v, want %v", gotType, ok, wantType)
	}
	if got := product.PresenceOf(values[0].Value()); !presence.Equal(got, presence.Maybe()) {
		t.Fatalf("value presence = %s, want maybe before the success branch proves it present", got)
	}
	if got := product.Get(reg, values[0].Value(), assertion.Key); !got.Has(assertion.RuntimeClaim) {
		t.Fatalf("assertion = %s, want runtime validation proof", got)
	}

	codePoint, codeExpr := requireLocalAssignmentExprByName(t, result, "code")
	codeBefore, ok := result.ExpressionValueBeforeBoundary(codePoint, codeExpr)
	if !ok {
		t.Fatal("ExpressionValueBeforeBoundary(validated.code) returned false")
	}
	codeBeforeType, ok := typevalue.TypeOf(reg, codeBefore)
	if !ok || !typ.TypeEquals(codeBeforeType, typ.String) {
		t.Fatalf("validated.code before-boundary type = %v/%v, want string", codeBeforeType, ok)
	}
	codeValue, ok := result.ExpressionValueAtBoundary(codePoint, codeExpr)
	if !ok {
		t.Fatal("ExpressionValueAtBoundary(validated.code) returned false")
	}
	codeType, ok := typevalue.TypeOf(reg, codeValue)
	if !ok || !typ.TypeEquals(codeType, typ.String) {
		t.Fatalf("validated.code type = %v/%v, want string", codeType, ok)
	}
}

func requireCallSiteByCalleePath(t *testing.T, result *Result, want string) cfg.Point {
	t.Helper()
	for _, point := range result.Graph().RPO() {
		site, ok := result.CallSite(point)
		if !ok {
			continue
		}
		if site.CalleePathRef().String() == want {
			return point
		}
	}
	t.Fatalf("call site %q not found", want)
	return 0
}

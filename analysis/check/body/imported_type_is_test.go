package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
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
	for _, point := range result.Graph().RPO() {
		if site, ok := result.CallSite(point); ok {
			t.Logf("call point=%d ctx=%d callee=%s results=%d fixed=%d", point, site.Context(), site.CalleePathRef().String(), len(site.ResultTargets()), len(result.facts.CallResultValues(point)))
		}
	}
	callPoint := requireCallSiteByCalleePath(t, result, "errors.AppError.is")
	if result.callOutcome != nil {
		if site, ok := result.facts.CallSiteView(callPoint); ok {
			outcome := result.callOutcome(
				transfer.NodeContext{Graph: result.Graph(), Registry: reg, Point: callPoint, Node: result.Graph().Node(callPoint), Read: result.stateRead},
				site,
				result.stateRead(callPoint),
				result.stateRead,
			)
			t.Logf("outcome results=%d authority=%v post=%v", len(outcome.Results), outcome.PostReturnAuthority, outcome.HasPostReturnEvidence())
			for _, result := range outcome.Results {
				resultType, resultOK := typevalue.TypeOf(reg, result.Value)
				t.Logf("outcome result index=%d type=%v/%v presence=%s evidence=%s assertion=%s identity=%s bottom=%v top=%v",
					result.Index,
					resultType,
					resultOK,
					product.PresenceOf(result.Value),
					product.Get(reg, result.Value, evidence.Key),
					product.Get(reg, result.Value, assertion.Key),
					product.Get(reg, result.Value, identity.Key),
					product.Equal(reg, result.Value, product.Bottom(reg)),
					product.Equal(reg, result.Value, product.Top()))
			}
		}
	}
	values := prepared.facts.CallResultValues(callPoint)
	if len(values) != 2 {
		t.Fatalf("call result values = %d, want 2: %#v", len(values), values)
	}
	resultValues := result.facts.CallResultValues(callPoint)
	t.Logf("prepared fixed results=%d result fixed results=%d", len(values), len(resultValues))
	gotType, ok := typevalue.TypeOf(reg, values[0].Value())
	t.Logf("fixed result index=0 type=%v/%v presence=%s evidence=%s assertion=%s identity=%s",
		gotType,
		ok,
		product.PresenceOf(values[0].Value()),
		product.Get(reg, values[0].Value(), evidence.Key),
		product.Get(reg, values[0].Value(), assertion.Key),
		product.Get(reg, values[0].Value(), identity.Key))
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
	if st, ok := result.boundaryStateAt(callPoint); ok {
		slot := st.ReadReturnSlot(reg, 0)
		slotType, slotOK := typevalue.TypeOf(reg, slot)
		t.Logf("call return slot 0 after materialization type=%v/%v bottom=%v top=%v value=%v",
			slotType, slotOK, product.Equal(reg, slot, product.Bottom(reg)), product.Equal(reg, slot, product.Top()), slot)
	}
	for _, point := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || fact.Name != "validated" {
			continue
		}
		if st, ok := result.boundaryStateAt(point); ok && fact.HasSymbol {
			local := st.ReadValue(reg, key.SymbolValue(fact.Symbol))
			localType, localOK := typevalue.TypeOf(reg, local)
			t.Logf("validated local after assignment type=%v/%v bottom=%v top=%v value=%v",
				localType, localOK, product.Equal(reg, local, product.Bottom(reg)), product.Equal(reg, local, product.Top()), local)
		}
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

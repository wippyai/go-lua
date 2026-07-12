package body

import (
	"os"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

func TestCalleeValueProviderReturnsIdentityRichPathWithoutTypeWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7)
	target := symbol.ID(41)
	calleePath := pathdom.NewPath(target, "state").Field("async")

	builder := visibility.NewBuilder()
	version := builder.Define(point, target, "state")
	resolver := visibility.NewResolver(builder.Build())
	pathKey := resolver.KeyForVersion(target, version.ID, calleePath.Segments)
	fnID := identity.LuaFunction(99)
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	value = product.Set(reg, value, identity.Key, identity.Singleton(fnID))
	in := state.State{}.WritePathKey(reg, resolver.KeySpace(), pathKey, value)
	provider := calleeValueProvider(reg, factflow.NewFacts(factflow.FactsInput{}), resolver, nil, typevalue.NewCache(), nil, nil, nil)

	got, ok := provider(transfer.NodeContext{Point: point, Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: calleePath,
	}).View(), in, nil)
	if !ok {
		t.Fatal("callee value returned !ok")
	}
	gotID, ok := product.Get(reg, got, identity.Key).ID()
	if !ok || gotID != fnID {
		t.Fatalf("callee identity = %s/%v, want %s", gotID, ok, fnID)
	}
}

func TestImportOutcomesUsesTypeValueWitnessBoundary(t *testing.T) {
	src, err := os.ReadFile("import_outcomes.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, forbidden := range []string{
		"axis/typewitness",
		"func witnessedType",
		"func hasTypeWitness",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("import_outcomes.go reaches through the type-witness lane directly: found %q", forbidden)
		}
	}
}

func TestAnnotatedRequireLocalSourceUsesManifestExportReturnSlot(t *testing.T) {
	reg := standard.Registry()
	exportType := typetable.NewRecord().Field("run", typ.Func().Build()).Build()
	m := manifest.New("pkg")
	m.SetExport(exportType)

	result, err := CheckChunk(parseChunk(t, `
		local pkg: {run: () -> ()}? = require("pkg")
	`), Config{
		Registry: reg,
		Globals:  []string{"require"},
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
			Manifests:     []*manifest.Manifest{m},
		},
		ModuleExports: importlookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	point, _ := requireLocalAssignmentExprByName(t, result, "pkg")
	fact, ok := result.LocalAssignment(point)
	if !ok {
		t.Fatal("missing local assignment for pkg")
	}
	value, ok := result.LocalAssignmentSourceValueAtBoundary(point, fact.Source)
	if !ok {
		t.Fatal("missing source value for annotated require assignment")
	}
	gotType, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(gotType, exportType) {
		t.Fatalf("annotated require source type = %v/%v, want manifest export %v", gotType, ok, exportType)
	}
	exprValue, ok := result.ExpressionValueAtBoundary(point, fact.Expr)
	if !ok {
		t.Fatal("missing expression value for annotated require assignment")
	}
	exprType, ok := typevalue.TypeOf(reg, exprValue)
	if !ok || !typ.TypeEquals(exprType, exportType) {
		t.Fatalf("annotated require expression type = %v/%v, want manifest export %v", exprType, ok, exportType)
	}
}

func TestCalleeValueProviderPrefersTypedStaticMethodOverWeakExactPath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8)
	receiver := symbol.ID(42)
	receiverPath := pathdom.NewPath(receiver, "service")
	methodPath := receiverPath.Field("run")

	builder := visibility.NewBuilder()
	version := builder.Define(point, receiver, "service")
	resolver := visibility.NewResolver(builder.Build())
	methodKey := resolver.KeyForVersion(receiver, version.ID, methodPath.Segments)
	methodType := typ.Func().Returns(typ.Number).Build()
	staticValue := typevalue.WithWitness(reg, typevalue.FromType(reg, methodType), methodType)
	weakValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	weakValue = product.Set(reg, weakValue, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	weakValue = product.Set(reg, weakValue, identity.Key, identity.Singleton(identity.LuaFunction(100)))
	in := state.State{}.
		WritePathStaticMember(resolver.KeySpace(), methodKey, staticValue).
		WritePathKey(reg, resolver.KeySpace(), methodKey, weakValue)
	provider := calleeValueProvider(reg, factflow.NewFacts(factflow.FactsInput{}), resolver, nil, typevalue.NewCache(), nil, nil, nil)

	got, ok := provider(transfer.NodeContext{Point: point, Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		ReceiverPath:    receiverPath,
		HasReceiverPath: true,
		MethodPath:      methodPath,
		HasMethodPath:   true,
		MethodName:      "run",
	}).View(), in, nil)
	if !ok {
		t.Fatal("callee value returned !ok")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, methodType) {
		t.Fatalf("callee method type = %v/%v, want %v (value %#v)", gotType, ok, methodType, got)
	}
}

func TestCallableValueOutcomeUsesTypedStaticMethodOverWeakExactPath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9)
	receiver := symbol.ID(43)
	result := symbol.ID(44)
	receiverPath := pathdom.NewPath(receiver, "service")
	resultPath := pathdom.NewPath(result, "out")
	methodPath := receiverPath.Field("run")

	builder := visibility.NewBuilder()
	version := builder.Define(point, receiver, "service")
	builder.Define(point, result, "out")
	resolver := visibility.NewResolver(builder.Build())
	methodKey := resolver.KeyForVersion(receiver, version.ID, methodPath.Segments)
	methodType := typ.Func().Returns(typ.Number).Build()
	staticValue := typevalue.WithWitness(reg, typevalue.FromType(reg, methodType), methodType)
	weakValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	weakValue = product.Set(reg, weakValue, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	weakValue = product.Set(reg, weakValue, identity.Key, identity.Singleton(identity.LuaFunction(101)))
	in := state.State{}.
		WritePathStaticMember(resolver.KeySpace(), methodKey, staticValue).
		WritePathKey(reg, resolver.KeySpace(), methodKey, weakValue)
	calleeValue := calleeValueProvider(reg, factflow.NewFacts(factflow.FactsInput{}), resolver, nil, typevalue.NewCache(), nil, nil, nil)
	provider := effectlowering.CallableValueOutcomeProvider(effectlowering.CallableValueOutcomeProviderConfig{
		CalleeValue: effectlowering.CalleeValueFunc(calleeValue),
		Callable:    typecall.Callable,
	})

	got := provider(transfer.NodeContext{Point: point, Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		ReceiverPath:    receiverPath,
		HasReceiverPath: true,
		MethodPath:      methodPath,
		HasMethodPath:   true,
		MethodName:      "run",
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result, resultPath),
		},
	}).View(), in, nil)

	if len(got.Results) != 1 || got.Results[0].Index != 0 {
		t.Fatalf("callable outcome results = %#v, want one result slot", got.Results)
	}
	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("callable outcome return type = %v/%v, want number (outcome %#v)", gotType, ok, got)
	}
}

func TestExplicitAnyReceiverMethodResultSurvivesMultiReturnAssignment(t *testing.T) {
	reg := standard.Registry()
	result, err := CheckChunk(parseChunk(t, `
local provider_instance: any = nil
local raw_result, err = (provider_instance :: any):structured_output({})
if err then
	local message = err:message()
end
`), Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var messagePoint cfg.Point
	var receiverSource factflow.ValueSource
	for _, point := range result.Graph().RPO() {
		fact, ok := result.SourceCall(point)
		if !ok || fact.Method != "message" {
			continue
		}
		site, ok := result.CallSiteView(point)
		if !ok {
			t.Fatalf("message call has no lowered call site at %d", point)
		}
		var hasReceiver bool
		receiverSource, hasReceiver = site.ReceiverSource()
		if !hasReceiver {
			t.Fatalf("message call has no receiver source at %d", point)
		}
		messagePoint = point
		break
	}
	if messagePoint == 0 {
		t.Fatal("missing err:message call")
	}
	receiverValue, ok := result.SourceValueAtBoundary(messagePoint, receiverSource)
	if !ok {
		t.Fatalf("message receiver source did not resolve at %d", messagePoint)
	}
	if ev := product.Get(reg, receiverValue, evidence.Key); !ev.IsExplicitTop() {
		t.Fatalf("message receiver evidence = %s, want explicit top from upstream :: any call", ev)
	}
	outcome, ok := result.CallOutcomeAt(messagePoint)
	if !ok || len(outcome.Results) == 0 {
		t.Fatalf("message outcome = %#v/%v, want result", outcome, ok)
	}
	if ev := product.Get(reg, outcome.Results[0].Value, evidence.Key); !ev.IsExplicitTop() {
		t.Fatalf("message result evidence = %s, want explicit top inherited from receiver", ev)
	}
}

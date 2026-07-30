package body

import (
	"os"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
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

func TestExplicitAnyReceiverProviderDoesNotClaimReceiverForNonMethodCall(t *testing.T) {
	provider := explicitAnyReceiverMethodOutcomeProvider(standard.Registry(), typevalue.NewCache())
	site := factflow.NewCallSite(factflow.CallSiteConfig{}).View()
	prepared := testPrepareCallOutcome(t, provider, transfer.NodeContext{Registry: standard.Registry()}, site)
	if len(prepared.Capability().FieldRoles()) != 0 {
		t.Fatalf("non-method provider fields = %#v, want empty", prepared.Capability().FieldRoles())
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
		Callable: typecall.Callable,
	})
	ctx := transfer.NodeContext{Point: point, Registry: reg}
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		ReceiverPath:    receiverPath,
		HasReceiverPath: true,
		MethodPath:      methodPath,
		HasMethodPath:   true,
		MethodName:      "run",
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result, resultPath),
		},
	}).View()
	callee, ok := calleeValue(ctx, site, in, nil)
	if !ok {
		t.Fatal("callee operand did not resolve")
	}
	input := sealedBodyCallInput(t, provider, ctx, site, in, callpayload.CallOutcomeValueOperands{Callee: callee, HasCallee: true})
	got := testEvaluateCallOutcome(t, provider, ctx, site, input)

	if len(got.Results) != 1 || got.Results[0].Index != 0 {
		t.Fatalf("callable outcome results = %#v, want one result slot", got.Results)
	}
	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("callable outcome return type = %v/%v, want number (outcome %#v)", gotType, ok, got)
	}
}

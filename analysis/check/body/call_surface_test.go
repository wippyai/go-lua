package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestPreparedPlanSealsIndependentLexicalCallSurface(t *testing.T) {
	stmts, err := parse.ParseString(`
local function leaf(value: number): number
  return value
end
local function caller(value: number): number
  local result = leaf(value)
  return result
end
return caller(1)
`, "call-surface.lua")
	if err != nil {
		t.Fatal(err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	var leaf, caller *ast.FunctionExpr
	for _, origin := range bindings.FunctionOrigins() {
		switch bindings.Name(origin.TargetSymbol) {
		case "leaf":
			leaf = origin.Func
		case "caller":
			caller = origin.Func
		}
	}
	if leaf == nil || caller == nil {
		t.Fatal("bound functions missing")
	}
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("call-surface-test"))
	reg := standard.Registry()
	prepared, err := PrepareBoundFunction(caller, bindings, Config{Registry: reg, TypeValues: typevalue.NewCache(), UnitNamespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	surface, ok := prepared.OperationPlan().CallSurface()
	if !ok || !surface.Complete() || surface.Owner() != prepared.StableLexicalBodyID() || len(surface.Sites()) != 1 {
		t.Fatalf("prepared call surface = %#v/%v", surface, ok)
	}
	functionSymbol, ok := bindings.FunctionSymbol(leaf)
	if !ok {
		t.Fatal("leaf function symbol missing")
	}
	want := lexicalidentity.FunctionBody(namespace, uint64(functionSymbol))
	got, lexical := surface.Sites()[0].Target.LexicalBody()
	if !lexical || got != want || surface.Sites()[0].Target.Kind() != operationplan.CallSurfaceTargetLexical {
		t.Fatalf("lexical target = %x/%v kind=%v, want %x; closures=%#v", got, lexical, surface.Sites()[0].Target.Kind(), want, bindings.LocalFunctionUseClosures())
	}
}

func TestPreparedPlanExplicitlyRejectsUnclosedDynamicCall(t *testing.T) {
	stmts, err := parse.ParseString(`
local function caller(fn: any, value: number): number
  local result = fn(value)
  return result
end
return caller(function(value) return value end, 1)
`, "rejected-call-surface.lua")
	if err != nil {
		t.Fatal(err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	var caller *ast.FunctionExpr
	for _, origin := range bindings.FunctionOrigins() {
		if bindings.Name(origin.TargetSymbol) == "caller" {
			caller = origin.Func
			break
		}
	}
	if caller == nil {
		t.Fatal("caller missing")
	}
	reg := standard.Registry()
	prepared, err := PrepareBoundFunction(caller, bindings, Config{Registry: reg, TypeValues: typevalue.NewCache(), UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("rejected-call-surface"))})
	if err != nil {
		t.Fatal(err)
	}
	surface, ok := prepared.OperationPlan().CallSurface()
	if !ok || len(surface.Sites()) != 1 || surface.Sites()[0].Target.Kind() != operationplan.CallSurfaceTargetRejected {
		t.Fatalf("dynamic call surface = %#v/%v", surface, ok)
	}
}

func TestPreparedPlanCallSurfaceReusesSealedExternalOperations(t *testing.T) {
	json := manifest.New("json")
	json.DefineFunctionSignature("json.decode", signature.Function{Type: typ.Func().Param("src", typ.String).Returns(typ.Number).Build()})
	for _, test := range []struct {
		name          string
		source        string
		config        Config
		wantIntrinsic bool
	}{
		{
			name:   "native stdlib",
			source: `print("value")`,
			config: Config{Signatures: signaturelookup.Source{IncludeStdlib: true}},
		},
		{
			name:          "sealed Lua type intrinsic",
			source:        `local value = type("value") return value`,
			config:        Config{Signatures: signaturelookup.Source{IncludeStdlib: true}},
			wantIntrinsic: true,
		},
		{
			name:   "imported signature",
			source: `local json = require("json") local value = json.decode("{}") return value`,
			config: Config{Globals: []string{"require"}, Signatures: signaturelookup.Source{IncludeStdlib: true, Manifests: []*manifest.Manifest{json}}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stmts, err := parse.ParseString(test.source, test.name+".lua")
			if err != nil {
				t.Fatal(err)
			}
			test.config.Registry = standard.Registry()
			test.config.TypeValues = typevalue.NewCache()
			test.config.UnitNamespace = lexicalidentity.UnitNamespaceFromContent([]byte(test.name))
			bindings := bind.BindChunk(stmts, bind.Options{Globals: Globals(test.config)})
			prepared, err := PrepareBoundChunk(stmts, bindings, test.config)
			if err != nil {
				t.Fatal(err)
			}
			plan := prepared.OperationPlan()
			surface, ok := plan.CallSurface()
			if !ok || len(surface.Sites()) == 0 {
				t.Fatalf("external surface unavailable: %#v/%v", surface, ok)
			}
			externals, intrinsics := 0, 0
			for _, site := range surface.Sites() {
				operation, resolved := plan.SignatureCallOperation(site.Point)
				content, external := site.Target.ExternalContentID()
				if !resolved {
					if site.Target.Kind() != operationplan.CallSurfaceTargetRejected {
						t.Fatalf("unresolved point %d was not explicitly rejected", site.Point)
					}
					continue
				}
				if !external || site.Target.Kind() != operationplan.CallSurfaceTargetExternal || content != operation.ContentID() {
					t.Fatalf("point %d external identity drift: kind=%v external=%v content=%x want=%x", site.Point, site.Target.Kind(), external, content, operation.ContentID())
				}
				externals++
				if _, intrinsic := operation.Intrinsic(); intrinsic {
					intrinsics++
				}
			}
			if externals == 0 || (test.wantIntrinsic && intrinsics == 0) {
				t.Fatalf("external/intrinsic calls = %d/%d", externals, intrinsics)
			}
		})
	}
}

func TestPreparedCallSurfaceNeverHidesWIRCallsMissingFacts(t *testing.T) {
	stmts, err := parse.ParseString(`return nil`, "hidden-call.lua")
	if err != nil {
		t.Fatal(err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("hidden-call"))
	owner := lexicalidentity.RootBody(namespace)
	body := wir.NewBody("hidden-call")
	body.Emit(wir.Instruction{Op: wir.OpCall, Point: 1})
	surface := sealPreparedCallSurface(bindings, body, factflow.NewFacts(factflow.FactsInput{}), nil, owner, namespace, 2)
	if !surface.Complete() || len(surface.Sites()) != 1 || surface.Sites()[0].Point != 1 || surface.Sites()[0].Target.Kind() != operationplan.CallSurfaceTargetRejected {
		t.Fatalf("factless WIR call disappeared: %#v", surface)
	}

	for name, instructions := range map[string][]wir.Instruction{
		"out of range": {{Op: wir.OpCall, Point: cfg.Point(100)}},
		"duplicate point": {
			{Op: wir.OpCall, Point: 1},
			{Op: wir.OpCall, Point: 1},
		},
	} {
		t.Run(name, func(t *testing.T) {
			malformed := wir.NewBody(name)
			for _, instruction := range instructions {
				malformed.Emit(instruction)
			}
			got := sealPreparedCallSurface(bindings, malformed, factflow.NewFacts(factflow.FactsInput{}), nil, owner, namespace, 2)
			if got.Complete() || got.Digest().Available() {
				t.Fatalf("malformed WIR census remained authoritative: %#v", got)
			}
		})
	}
}

func TestPreparedCallSurfaceSealsOnlyExactExternalMethodShape(t *testing.T) {
	stmts, err := parse.ParseString(`return nil`, "method-call.lua")
	if err != nil {
		t.Fatal(err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("method-call"))
	owner := lexicalidentity.RootBody(namespace)
	body := wir.NewBody("method-call")
	receiver := pathdom.NewPath(11, "value")
	method := body.InternConst(wir.Const{Kind: wir.ConstString, Str: "gsub"})
	body.Emit(wir.Instruction{Op: wir.OpCall, Point: 1, Call: wir.CallInfo{
		Receiver: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(receiver))}, Method: method,
	}})
	operation, ok := operationplan.NewSignatureCallOperation(signature.Function{Type: typ.Func().Build()})
	if !ok {
		t.Fatal("signature operation rejected")
	}
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	receiverSource, _ := factflow.NewPathValueSource(receiver.Key(), 0, 0, 0, shape)
	exact := factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: receiver.Field("gsub"), CalleeMemberAccess: true,
		ReceiverPath: receiver, HasReceiverPath: true, MethodPath: receiver.Field("gsub"), HasMethodPath: true, MethodName: "gsub",
		ReceiverSource: receiverSource, HasReceiverSource: true,
	})
	for _, test := range []struct {
		name       string
		site       factflow.CallSite
		operations map[cfg.Point]operationplan.SignatureCallOperation
		want       operationplan.CallSurfaceTargetKind
	}{
		{name: "exact", site: exact, operations: map[cfg.Point]operationplan.SignatureCallOperation{1: operation}, want: operationplan.CallSurfaceTargetExternal},
		{name: "unsealed", site: exact, want: operationplan.CallSurfaceTargetRejected},
		{name: "receiver mismatch", site: factflow.NewCallSite(factflow.CallSiteConfig{
			CalleePath: pathdom.NewPath(12, "other").Field("gsub"), CalleeMemberAccess: true,
			ReceiverPath: pathdom.NewPath(12, "other"), HasReceiverPath: true, MethodPath: pathdom.NewPath(12, "other").Field("gsub"), HasMethodPath: true, MethodName: "gsub",
			ReceiverSource: receiverSource, HasReceiverSource: true,
		}), operations: map[cfg.Point]operationplan.SignatureCallOperation{1: operation}, want: operationplan.CallSurfaceTargetRejected},
		{name: "name mismatch", site: factflow.NewCallSite(factflow.CallSiteConfig{
			CalleePath: receiver.Field("match"), CalleeMemberAccess: true,
			ReceiverPath: receiver, HasReceiverPath: true, MethodPath: receiver.Field("match"), HasMethodPath: true, MethodName: "match",
			ReceiverSource: receiverSource, HasReceiverSource: true,
		}), operations: map[cfg.Point]operationplan.SignatureCallOperation{1: operation}, want: operationplan.CallSurfaceTargetRejected},
	} {
		t.Run(test.name, func(t *testing.T) {
			surface := sealPreparedCallSurface(bindings, body, factflow.NewFacts(factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{1: test.site}}), test.operations, owner, namespace, 2)
			if !surface.Complete() || len(surface.Sites()) != 1 || surface.Sites()[0].Target.Kind() != test.want {
				t.Fatalf("method surface = %#v, want complete target kind %v", surface, test.want)
			}
		})
	}
}

func TestPreparedCallSurfaceRejectsDynamicExternalMethodReceiver(t *testing.T) {
	stmts, err := parse.ParseString(`return nil`, "dynamic-method-call.lua")
	if err != nil {
		t.Fatal(err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("dynamic-method-call"))
	body := wir.NewBody("dynamic-method-call")
	body.Emit(wir.Instruction{Op: wir.OpCall, Point: 1, Call: wir.CallInfo{Receiver: wir.Operand{Kind: wir.OperandTemp, Ref: 1}, Method: body.InternConst(wir.Const{Kind: wir.ConstString, Str: "gsub"})}})
	operation, ok := operationplan.NewSignatureCallOperation(signature.Function{Type: typ.Func().Build()})
	if !ok {
		t.Fatal("signature operation rejected")
	}
	surface := sealPreparedCallSurface(bindings, body, factflow.NewFacts(factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{1: factflow.NewCallSite(factflow.CallSiteConfig{MethodName: "gsub", CalleeMemberAccess: true})}}), map[cfg.Point]operationplan.SignatureCallOperation{1: operation}, lexicalidentity.RootBody(namespace), namespace, 2)
	if !surface.Complete() || surface.Sites()[0].Target.Kind() != operationplan.CallSurfaceTargetRejected {
		t.Fatalf("dynamic method surface = %#v, want rejected", surface)
	}
}

func TestPreparedCallSurfaceExternalTargetRequiresExactWIRCallee(t *testing.T) {
	stmts, err := parse.ParseString(`return nil`, "external-callee-authority.lua")
	if err != nil {
		t.Fatal(err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("external-callee-authority"))
	owner := lexicalidentity.RootBody(namespace)
	actual := pathdom.NewPath(11, "actual")
	body := wir.NewBody("external-callee-authority")
	body.Emit(wir.Instruction{Op: wir.OpCall, Point: 1, Call: wir.CallInfo{
		Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(actual))},
	}})
	operation, ok := operationplan.NewSignatureCallOperation(signature.Function{Type: typ.Func().Build()})
	if !ok {
		t.Fatal("signature operation rejected")
	}

	for _, test := range []struct {
		name string
		site factflow.CallSite
		want operationplan.CallSurfaceTargetKind
	}{
		{
			name: "exact",
			site: factflow.NewCallSite(factflow.CallSiteConfig{CalleeSymbol: actual.Symbol, CalleePath: actual}),
			want: operationplan.CallSurfaceTargetExternal,
		},
		{
			name: "mismatched callee",
			site: factflow.NewCallSite(factflow.CallSiteConfig{
				CalleeSymbol: 12,
				CalleePath:   pathdom.NewPath(12, "stale"),
			}),
			want: operationplan.CallSurfaceTargetRejected,
		},
		{
			name: "missing callee",
			site: factflow.NewCallSite(factflow.CallSiteConfig{}),
			want: operationplan.CallSurfaceTargetRejected,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			surface := sealPreparedCallSurface(bindings, body, factflow.NewFacts(factflow.FactsInput{
				CallSites: map[cfg.Point]factflow.CallSite{1: test.site},
			}), map[cfg.Point]operationplan.SignatureCallOperation{1: operation}, owner, namespace, 2)
			if !surface.Complete() || len(surface.Sites()) != 1 || surface.Sites()[0].Target.Kind() != test.want {
				t.Fatalf("external surface = %#v, want complete target kind %v", surface, test.want)
			}
		})
	}
}

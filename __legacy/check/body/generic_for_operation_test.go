package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestGenericForMethodIteratorComesFromCanonicalSignatureCallDescriptor(t *testing.T) {
	stmts := parseChunk(t, `
local function scan(s: string): string
    for word in s:gmatch("%a+") do
        return word
    end
    return ""
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	origins := bindings.FunctionOrigins()
	if len(origins) != 1 || origins[0].Func == nil {
		t.Fatalf("function origins = %d, want scan", len(origins))
	}
	prepared, err := PrepareBoundFunction(origins[0].Func, bindings, Config{
		Registry: standard.Registry(), Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for point := cfg.Point(0); int(point) < prepared.operationPlan.PointCount(); point++ {
		generic, ok := prepared.operationPlan.GenericForOperation(point)
		if !ok {
			continue
		}
		source, ok := generic.ProtocolSource(0)
		if !ok || !source.HasCallPoint {
			t.Fatalf("generic-for point %d has no exact call producer", point)
		}
		call, ok := prepared.operationPlan.SignatureCallOperation(source.CallPoint)
		if !ok {
			site, _ := prepared.facts.CallSiteView(source.CallPoint)
			receiver, _ := site.ReceiverPath()
			signatures := signaturelookup.Source{IncludeStdlib: true}
			sig, lookedUp := signatures.Lookup("string." + site.MethodName())
			refined, refinedOK := effectlowering.RefineStaticStringMethodSignature(prepared.registry, sig, site)
			iter, iterOK := iteration.ActiveIterator(refined.Effect.Labels)
			_, operationOK := operationplan.NewSignatureCallOperation(refined)
			t.Fatalf("generic-for point %d source call %d has no canonical descriptor (method=%q receiver=%v boundary=%t guarded=%t written=%t lookup=%t refine=%t iterator=%#v/%t operation=%t context=%v)",
				point, source.CallPoint, site.MethodName(), receiver,
				exactBoundaryStringMethodReceiver(prepared.registry, bindings, prepared.operationPlan, site),
				exactGuardedStringMethodReceiver(prepared.registry, bindings, prepared.cfg.Graph, prepared.facts, source.CallPoint, site),
				bindings.HasWrite(receiver.Symbol), lookedUp, refinedOK, iter, iterOK, operationOK, site.Context())
		}
		if _, ok := iteration.ActiveIterator(call.Signature().Effect.Labels); ok {
			t.Fatalf("source call %d function-valued iterator was misclassified as a collection iterator", source.CallPoint)
		}
		if _, ok := effectlowering.CallableIteratorSignature(call.Signature()); !ok || !generic.CallableIterator() {
			t.Fatalf("generic-for callable iterator = %t/%t, want canonical function-result descriptor", ok, generic.CallableIterator())
		}
		if got, ok := generic.Iterator(); ok {
			t.Fatalf("generic-for collection iterator = %#v/%t, want callable protocol", got, ok)
		}
		found++
	}
	if found != 1 {
		t.Fatalf("generic-for operations = %d, want 1", found)
	}
	shape := transformer.Shape{
		Params:   uint32(len(prepared.operationPlan.BoundaryParams())),
		Captures: uint32(len(prepared.operationPlan.BoundaryCaptures())),
		Globals:  uint32(len(prepared.operationPlan.BoundaryGlobals())),
	}
	if _, err := transformer.NewPlanCompiler().Prepare(prepared.registry, prepared.cfg.Graph, prepared.operationPlan, shape); err != nil {
		t.Fatalf("canonical callable-iterator plan did not compile: %v", err)
	}
}

func TestGenericForAnnotatedLocalMethodIteratorComesFromCanonicalSignatureCallDescriptor(t *testing.T) {
	stmts := parseChunk(t, `
local s: string = "hello world"
for word in s:gmatch("%a+") do
	break
end
`)
	prepared, err := PrepareChunk(stmts, Config{
		Registry: standard.Registry(), Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for point := cfg.Point(0); int(point) < prepared.operationPlan.PointCount(); point++ {
		generic, ok := prepared.operationPlan.GenericForOperation(point)
		if !ok {
			continue
		}
		source, ok := generic.ProtocolSource(0)
		if !ok || !source.HasCallPoint {
			t.Fatalf("generic-for point %d has no exact call producer", point)
		}
		call, ok := prepared.operationPlan.SignatureCallOperation(source.CallPoint)
		if !ok {
			t.Fatalf("generic-for point %d source call %d has no canonical descriptor", point, source.CallPoint)
		}
		if _, ok := effectlowering.CallableIteratorSignature(call.Signature()); !ok || !generic.CallableIterator() {
			t.Fatalf("generic-for callable iterator = %t/%t, want canonical function-result descriptor", ok, generic.CallableIterator())
		}
		return
	}
	t.Fatal("generic-for operation missing")
}

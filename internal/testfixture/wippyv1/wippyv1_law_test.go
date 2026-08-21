package wippyv1_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/testfixture"
	"github.com/wippyai/go-lua/internal/testfixture/wippyv1"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
)

// TestProductionHostSurfaceSealsIntoOneTarget states the first thing the
// checker owes a real deployment: the whole declared host surface of a running
// Wippy node seals into one canonical Target without a single provider needing
// to be simplified for it.
func TestProductionHostSurfaceSealsIntoOneTarget(t *testing.T) {
	target, err := wippyv1.Target()
	if err != nil {
		t.Fatalf("seal wippy v1 host target: %v", err)
	}
	if !target.ContentID().Available() {
		t.Fatal("sealed wippy v1 target has no content identity")
	}
	for _, module := range wippyv1.Modules() {
		t.Run(module.Name, func(t *testing.T) {
			declaration := module.Declaration()
			if declaration.Path != module.Name {
				t.Fatalf("module path = %q, want %q", declaration.Path, module.Name)
			}
			if declaration.ErrorType == nil {
				t.Fatal("module declares no error type, so its trailing optional result carries no value/error correlation")
			}
			for member := range declaration.FunctionSignatures {
				binding := memberBinding(module.Name, member)
				if _, ok := target.Operations.Lookup(binding); !ok {
					t.Fatalf("declared member %s.%s is not a target operation", module.Name, member)
				}
			}
		})
	}
}

// TestProductionHostSurfaceDeclaresNoTransfer records what the production
// manifests state about cross-actor payloads: nothing. Every process member
// that forwards a variadic payload into another actor - send, the four spawn
// variants, exec, upgrade - is declared as an ordinary call. A checker that
// wants to admit or reject a payload on those calls has no declared evidence
// to read, and this test fails the moment that changes, which is the moment
// the send-safety question becomes answerable from the manifest.
func TestProductionHostSurfaceDeclaresNoTransfer(t *testing.T) {
	target, err := wippyv1.Target()
	if err != nil {
		t.Fatalf("seal wippy v1 host target: %v", err)
	}
	for _, member := range []string{"send", "spawn", "spawn_monitored", "spawn_linked", "spawn_linked_monitored", "exec", "upgrade"} {
		t.Run(member, func(t *testing.T) {
			operation, ok := target.Operations.Lookup(memberBinding("process", member))
			if !ok {
				t.Fatalf("process.%s is not a target operation", member)
			}
			if got := target.Operations.TransferCount(operation); got != 0 {
				t.Fatalf("process.%s publishes %d transfers; the v1 manifest declares none, so the target invented them", member, got)
			}
			if got := target.Operations.EffectCount(operation); got != 0 {
				t.Fatalf("process.%s publishes %d publication effects; the v1 manifest declares none", member, got)
			}
			if got := target.Operations.FormalEffectCount(operation); got != 0 {
				t.Fatalf("process.%s publishes %d formal effects; the v1 manifest declares none", member, got)
			}
		})
	}
}

// TestProductionHostSurfaceSurvivesTheWireCodec seals the transcribed surface
// through the module-boundary codec that carries a manifest between compiled
// modules. A declaration a running node exchanges must decode to the same
// declaration it encoded; a manifest that only works in-process is not a
// module boundary.
func TestProductionHostSurfaceSurvivesTheWireCodec(t *testing.T) {
	for _, module := range wippyv1.Modules() {
		t.Run(module.Name, func(t *testing.T) {
			encoded, err := manifestwire.Encode(module.Declaration())
			if err != nil {
				t.Fatalf("encode %s: %v", module.Name, err)
			}
			decoded, err := manifestwire.Decode(encoded)
			if err != nil {
				t.Fatalf("decode %s: %v", module.Name, err)
			}
			again, err := manifestwire.Encode(decoded)
			if err != nil {
				t.Fatalf("re-encode %s: %v", module.Name, err)
			}
			if !bytes.Equal(encoded, again) {
				t.Fatalf("%s manifest is not stable across the wire codec", module.Name)
			}
			if len(decoded.FunctionSignatures) != len(module.Declaration().FunctionSignatures) {
				t.Fatalf("%s decoded %d signatures, declared %d", module.Name,
					len(decoded.FunctionSignatures), len(module.Declaration().FunctionSignatures))
			}
		})
	}
}

// TestProductionHostSurfaceLinksAnOrdinaryProgram closes the harvest: a piece
// of ordinary Wippy application code - open a store, answer an http request,
// message another actor - links against the sealed production surface through
// the same lowering and admission path any analyzed module takes.
func TestProductionHostSurfaceLinksAnOrdinaryProgram(t *testing.T) {
	target, err := wippyv1.Target()
	if err != nil {
		t.Fatalf("seal wippy v1 host target: %v", err)
	}
	source := []byte(`
local handle, open_err = store.get("main")
if open_err ~= nil then
    return false
end

local request, request_err = http.request()
if request_err ~= nil then
    return false
end

local key = request:param("id")
local encoded = json.encode({ path = request:path() })
handle:set("last", encoded)
process.send(process.registry.lookup("worker"), "job", key)
handle:release()
return true
`)
	linked, err := testfixture.SealSource(target, "main.lua", source)
	if err != nil {
		t.Fatalf("link program against wippy v1 host target: %v", err)
	}
	if got := linked.Project().Mounts().Count(); got != 1 {
		t.Fatalf("linked mount count = %d, want 1", got)
	}
}

func memberBinding(module string, member string) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{module},
		Member:    strings.Split(member, "."),
	}
}

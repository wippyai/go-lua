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

// TestProductionHostSurfaceDeclaresOnlyAuthenticatedTransfers proves that
// process.send owns its cross-actor semantics in the V1 manifest while no
// generic forwarding fallback invents the same semantics for other members.
func TestProductionHostSurfaceDeclaresOnlyAuthenticatedTransfers(t *testing.T) {
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
			want := 0
			if member == "send" {
				want = 1
			}
			if got := target.Operations.TransferCount(operation); got != want {
				t.Fatalf("process.%s transfer count = %d, want %d", member, got, want)
			}
			if got := target.Operations.EffectCount(operation); got != want {
				t.Fatalf("process.%s publication effect count = %d, want %d", member, got, want)
			}
			if got := target.Operations.FormalEffectCount(operation); got != want {
				t.Fatalf("process.%s formal effect count = %d, want %d", member, got, want)
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

package publicationfreeze

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

func publicationFreezeSchemaSemantic(seed byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xF1, seed})
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("publication freeze schema semantic key")
	}
	return key
}

func TestPublicationFreezeSchemaDeclaresCallValueHeapRoute(t *testing.T) {
	builder := engine.NewSchema()
	calls, callsOK := callowner.DeclareSchema(builder, publicationFreezeSchemaSemantic(1))
	values, valuesOK := valueowner.DeclareSchema(builder, publicationFreezeSchemaSemantic(2), publicationFreezeSchemaSemantic(3), publicationFreezeSchemaSemantic(4))
	heap, heapOK := heapowner.DeclareSchema(builder, publicationFreezeSchemaSemantic(5), publicationFreezeSchemaSemantic(6))
	fragment, fragmentOK := DeclareSchema(builder, publicationFreezeSchemaSemantic(7), publicationFreezeSchemaSemantic(8), values, calls, heap)
	sealed, sealedOK := builder.Seal()
	if !callsOK || !valuesOK || !heapOK || !fragmentOK || !sealedOK || sealed == nil || fragment.RuleSlot() == nil {
		t.Fatalf("publication freeze schema declaration calls=%t values=%t heap=%t fragment=%t sealed=%t", callsOK, valuesOK, heapOK, fragmentOK, sealedOK)
	}
}

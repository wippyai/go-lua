package publicationescape

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

func publicationEscapeSchemaSemantic(seed byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xE7, seed})
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("publication escape schema semantic key")
	}
	return key
}

func TestPublicationEscapeSchemaDeclaresCallValuePlacementRoute(t *testing.T) {
	builder := engine.NewSchema()
	calls, callsOK := callowner.DeclareSchema(builder, publicationEscapeSchemaSemantic(1))
	values, valuesOK := valueowner.DeclareSchema(builder, publicationEscapeSchemaSemantic(2), publicationEscapeSchemaSemantic(3), publicationEscapeSchemaSemantic(4))
	placement, placementOK := placementowner.DeclareSchema(builder, publicationEscapeSchemaSemantic(5), publicationEscapeSchemaSemantic(6))
	fragment, fragmentOK := DeclareSchema(builder, publicationEscapeSchemaSemantic(7), publicationEscapeSchemaSemantic(8), values, calls, placement)
	sealed, sealedOK := builder.Seal()
	if !callsOK || !valuesOK || !placementOK || !fragmentOK || !sealedOK || sealed == nil || fragment.RuleSlot() == nil {
		t.Fatalf("publication escape schema declaration calls=%t values=%t placement=%t fragment=%t sealed=%t", callsOK, valuesOK, placementOK, fragmentOK, sealedOK)
	}
}

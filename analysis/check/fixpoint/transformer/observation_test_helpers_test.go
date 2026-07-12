package transformer

import (
	engineobservation "github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func testObservationBody(seed byte) lexicalidentity.StableLexicalBodyID {
	var out lexicalidentity.StableLexicalBodyID
	out[0] = seed
	return out
}

func testObservationAnchor(kind engineobservation.Kind, line, ordinal uint32) engineobservation.Occurrence {
	return engineobservation.Occurrence{Point: wir.DebugPointID{Ordinal: line, Phase: wir.DebugPhaseAfter}, Kind: kind, Slot: ordinal}
}

package relcompile

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

func TestDestinationCarrierRejectsWriterFact(t *testing.T) {
	writerKey := carrier.Key("writer/key")
	writerFact := carrier.Key("writer/fact")

	projection := ProjectionBinding{Result: writerKey}
	publication := Publication{Result: writerKey}
	if !destinationUsesWriterKeyCarrier(projection, publication) {
		t.Fatal("canonical writer key carrier was rejected")
	}

	projection.Result = writerFact
	if destinationUsesWriterKeyCarrier(projection, publication) {
		t.Fatal("writer fact carrier was accepted as the destination row key")
	}
}

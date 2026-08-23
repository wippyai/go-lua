package value

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/plane"
)

// The payload every published value summary carries for the two fixtures
// below. A layout digest pins what the declaration is; these pin what the
// declaration produces, so a change in the walk that leaves the layout intact
// is caught here rather than by a consumer that can no longer read a stored
// answer.
const (
	summaryPresentPayloadPin = "558689a172cb39380af5f4e967cdca266ffaf1e0f8f64c53c44a059e1a63c403"
	summaryAbsentPayloadPin  = "07c3d687f26e16798b5657eda43e3dc23a67e2e56d64016caa804546bcc2366c"
)

// TestSummaryPublicationPayloadIsPinned states that the wire image of a value
// summary is a function of the declaration alone. The family authors a
// projection and the plane driver writes it; neither may change the bytes a
// stored answer was written as.
func TestSummaryPublicationPayloadIsPinned(t *testing.T) {
	firstID := summaryCodecID(33)
	secondID := summaryCodecID(67)
	schema := summaryCodecSchema(firstID, secondID)
	present := ValueSummaryObservation{
		Values: []Value{
			{schema: schema, image: []uint64{2, 5}},
			{schema: schema, top: true},
		},
		Present: []bool{true, true}, Rows: 1, Valid: true, owner: schema,
	}
	absent := ValueSummaryObservation{
		Values: make([]Value, 2), Present: []bool{false, false}, Valid: true, owner: schema,
	}
	for _, pinned := range []struct {
		name        string
		observation ValueSummaryObservation
		length      int
		pin         string
	}{
		{"present", present, 188, summaryPresentPayloadPin},
		{"absent", absent, 172, summaryAbsentPayloadPin},
	} {
		_, _, payload, ok := plane.Publish(summaryResultTestLayout, summaryPublicationProjection, pinned.observation)
		if !ok {
			t.Fatalf("the %s fixture did not publish", pinned.name)
		}
		if len(payload) != pinned.length {
			t.Fatalf("%s payload = %d bytes, want the pinned %d", pinned.name, len(payload), pinned.length)
		}
		digest := sha256.Sum256(payload)
		if got := hex.EncodeToString(digest[:]); got != pinned.pin {
			t.Fatalf("%s payload digest = %s, want the pinned image %s", pinned.name, got, pinned.pin)
		}
	}
}

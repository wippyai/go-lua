package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func publicationObservationLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

// An observation address is framed from the mounted call and selected-call
// occurrence. Missing identity material cannot acquire a fallback address,
// and another occurrence cannot preserve the same observation frame.
func TestPublicationObservationIdentityFrameLaw(t *testing.T) {
	mount := publicationObservationLawID(1)
	occurrence := publicationObservationLawID(3)
	otherOccurrence := publicationObservationLawID(2)

	first, firstOK := publicationObservationID(mount, occurrence)
	second, secondOK := publicationObservationID(mount, otherOccurrence)
	if !firstOK || !secondOK || first == second {
		t.Fatal("publication occurrence frames collapsed")
	}
	if _, ok := publicationObservationID(mount, identity.ContentID{}); ok {
		t.Fatal("missing publication identity acquired a fallback observation address")
	}
}

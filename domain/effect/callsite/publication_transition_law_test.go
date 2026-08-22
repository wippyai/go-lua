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

// An observation address is framed from the mounted call, selected-call
// occurrence, and exact execution Context. Missing identity material cannot
// acquire a fallback address, and another occurrence cannot preserve the same
// observation frame.
func TestPublicationObservationIdentityFrameLaw(t *testing.T) {
	mount := publicationObservationLawID(1)
	occurrence := publicationObservationLawID(3)
	otherOccurrence := publicationObservationLawID(2)
	context := publicationObservationLawID(5)

	first, firstOK := publicationObservationID(mount, occurrence, context)
	second, secondOK := publicationObservationID(mount, otherOccurrence, context)
	if !firstOK || !secondOK || first == second {
		t.Fatal("publication occurrence frames collapsed")
	}
	if _, ok := publicationObservationID(mount, identity.ContentID{}, context); ok {
		t.Fatal("missing publication identity acquired a fallback observation address")
	}
}

func TestPublicationObservationIdentityIsContextQualified(t *testing.T) {
	mount := publicationObservationLawID(11)
	occurrence := publicationObservationLawID(13)
	firstContext := publicationObservationLawID(17)
	secondContext := publicationObservationLawID(19)

	first, firstOK := publicationObservationID(mount, occurrence, firstContext)
	second, secondOK := publicationObservationID(mount, occurrence, secondContext)
	if !firstOK || !secondOK || first == second {
		t.Fatal("publication observations in distinct contexts aliased")
	}
	if _, ok := publicationObservationID(mount, occurrence, identity.ContentID{}); ok {
		t.Fatal("zero context acquired a publication observation address")
	}
}

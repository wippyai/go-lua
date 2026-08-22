package dispatch

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func callDispatchObservationLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

// A dispatch observation address is the owner-qualified mounted application
// occurrence and exact Context. Every coordinate changes the row, and no
// incomplete tuple receives a fallback address.
func TestCallDispatchObservationIdentityFrameLaw(t *testing.T) {
	linkID := callDispatchObservationLawID(1)
	applicationID := callDispatchObservationLawID(3)
	mount := callDispatchObservationLawID(5)
	occurrence := callDispatchObservationLawID(7)
	contextID := callDispatchObservationLawID(9)

	first, firstOK := callDispatchObservationID(linkID, applicationID, mount, occurrence, contextID)
	if !firstOK || !first.Available() {
		t.Fatal("complete dispatch observation identity was refused")
	}
	coordinates := []struct {
		name                                          string
		link, application, mount, occurrence, context identity.ContentID
	}{
		{name: "link", link: callDispatchObservationLawID(11), application: applicationID, mount: mount, occurrence: occurrence, context: contextID},
		{name: "application", link: linkID, application: callDispatchObservationLawID(13), mount: mount, occurrence: occurrence, context: contextID},
		{name: "mount", link: linkID, application: applicationID, mount: callDispatchObservationLawID(15), occurrence: occurrence, context: contextID},
		{name: "occurrence", link: linkID, application: applicationID, mount: mount, occurrence: callDispatchObservationLawID(17), context: contextID},
		{name: "context", link: linkID, application: applicationID, mount: mount, occurrence: occurrence, context: callDispatchObservationLawID(19)},
	}
	for _, coordinate := range coordinates {
		t.Run(coordinate.name, func(t *testing.T) {
			other, otherOK := callDispatchObservationID(coordinate.link, coordinate.application, coordinate.mount, coordinate.occurrence, coordinate.context)
			if !otherOK || !other.Available() || other == first {
				t.Fatalf("dispatch observation coordinate %s aliased", coordinate.name)
			}
		})
	}
}

func TestCallDispatchObservationIdentityRefusesIncompleteOwnerTuple(t *testing.T) {
	complete := []identity.ContentID{
		callDispatchObservationLawID(1),
		callDispatchObservationLawID(3),
		callDispatchObservationLawID(5),
		callDispatchObservationLawID(7),
		callDispatchObservationLawID(9),
	}
	for index := range complete {
		coordinates := append([]identity.ContentID(nil), complete...)
		coordinates[index] = identity.ContentID{}
		if _, ok := callDispatchObservationID(coordinates[0], coordinates[1], coordinates[2], coordinates[3], coordinates[4]); ok {
			t.Fatalf("zero dispatch observation coordinate index %d acquired a fallback address", index)
		}
	}
}

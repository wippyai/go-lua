package observation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func lawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

// Every owner coordinate contributes to the sealed row address.
func TestIDIdentityFrameLaw(t *testing.T) {
	linkID := lawID(1)
	applicationID := lawID(3)
	mount := lawID(5)
	occurrence := lawID(7)
	contextID := lawID(9)

	first, firstOK := ID(linkID, applicationID, mount, occurrence, contextID)
	if !firstOK || !first.Available() {
		t.Fatal("complete dispatch observation identity was refused")
	}
	coordinates := []struct {
		name                                          string
		link, application, mount, occurrence, context identity.ContentID
	}{
		{name: "link", link: lawID(11), application: applicationID, mount: mount, occurrence: occurrence, context: contextID},
		{name: "application", link: linkID, application: lawID(13), mount: mount, occurrence: occurrence, context: contextID},
		{name: "mount", link: linkID, application: applicationID, mount: lawID(15), occurrence: occurrence, context: contextID},
		{name: "occurrence", link: linkID, application: applicationID, mount: mount, occurrence: lawID(17), context: contextID},
		{name: "context", link: linkID, application: applicationID, mount: mount, occurrence: occurrence, context: lawID(19)},
	}
	for _, coordinate := range coordinates {
		t.Run(coordinate.name, func(t *testing.T) {
			other, otherOK := ID(coordinate.link, coordinate.application, coordinate.mount, coordinate.occurrence, coordinate.context)
			if !otherOK || !other.Available() || other == first {
				t.Fatalf("dispatch observation coordinate %s aliased", coordinate.name)
			}
		})
	}
}

// A missing owner coordinate is not a row identity and never becomes one by
// substitution or fallback.
func TestIDRefusesIncompleteOwnerTuple(t *testing.T) {
	complete := []identity.ContentID{lawID(1), lawID(3), lawID(5), lawID(7), lawID(9)}
	for index := range complete {
		coordinates := append([]identity.ContentID(nil), complete...)
		coordinates[index] = identity.ContentID{}
		if _, ok := ID(coordinates[0], coordinates[1], coordinates[2], coordinates[3], coordinates[4]); ok {
			t.Fatalf("zero dispatch observation coordinate index %d acquired a fallback address", index)
		}
	}
}

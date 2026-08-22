package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

func boundaryKey(value byte) composition.Key {
	var id composition.ID
	id[0] = value
	return composition.Key{ID: id, Version: 1}
}

func boundaryContext(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func boundaryDecision(t testing.TB, value byte) Decision {
	t.Helper()
	decision, ok := NewDecision(boundaryKey(value))
	if !ok {
		t.Fatal("decision")
	}
	return decision
}

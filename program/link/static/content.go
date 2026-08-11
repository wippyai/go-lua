package static

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Version 2 includes every identity-bearing Static row coordinate.  Version 1
// omitted StaticInput kind/target and qualified consumer shard, so retaining
// that digest would allow distinct relations to alias.
const contentVersion = 2

func staticContentID(targetID, projectID keyspace.ContentID, component *Component) (id keyspace.ContentID) {
	if component == nil || !targetID.Available() || !projectID.Available() {
		return keyspace.ContentID{}
	}
	h := sha256.New()
	var w canonical.Writer
	if w.Reset(h, "program/link/static", contentVersion) != nil ||
		w.Record(1) != nil || w.Bytes(targetID[:]) != nil ||
		w.Bytes(projectID[:]) != nil ||
		!writeStaticRows(&w, component) || w.Finish() != nil {
		return keyspace.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}

package static

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// contentVersion changes only when the fixed, owned Static semantic codec
// changes. Query indexes, containment scratch, and cross-owner geometry are
// deliberately outside this authored identity.
const contentVersion = 3

// Cold is the post-build snapshot of authored Static identity. It deliberately
// retains only the value observed by consumers, never the Component graph.
type Cold struct{ contentID keyspace.ContentID }

func (component *Component) Cold() Cold {
	if component == nil {
		return Cold{}
	}
	return Cold{contentID: component.contentID}
}

func (cold Cold) ContentID() keyspace.ContentID { return cold.contentID }

// contentID coordinates exactly the nine typed authored verticals. Each
// vertical owns its own scalar order and never hashes query derivatives.
func contentID(component *Component) (id keyspace.ContentID) {
	if component == nil {
		return keyspace.ContentID{}
	}
	hash := sha256.New()
	var writer canonical.Writer
	if writer.Reset(hash, "program/static", contentVersion) != nil ||
		writeArtifactContent(&writer,
			component.types, component.references, component.declarations,
			component.signatures, component.contracts, component.effectRows, component.operators,
			component.operands, component.publications) != nil ||
		writer.Finish() != nil {
		return keyspace.ContentID{}
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}

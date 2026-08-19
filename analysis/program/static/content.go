package static

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// contentVersion changes only when the fixed, owned Static semantic codec
// changes. Query indexes, containment scratch, and cross-owner geometry are
// deliberately outside this authored identity.
const contentVersion = 4

// Cold is the post-build snapshot of authored Static identity. It deliberately
// retains only the value observed by consumers, never the Component graph.
type Cold struct{ contentID identity.ContentID }

// ContentID returns the sealed authored Static identity.
func (component *Component) ContentID() identity.ContentID {
	if component == nil {
		return identity.ContentID{}
	}
	return component.contentID
}

func (component *Component) Cold() Cold {
	if component == nil {
		return Cold{}
	}
	return Cold{contentID: component.contentID}
}

func (cold Cold) ContentID() identity.ContentID { return cold.contentID }

// contentID coordinates exactly the eight typed authored verticals. Each
// vertical owns its own scalar order and never hashes query derivatives.
func contentID(component *Component) (id identity.ContentID) {
	if component == nil {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/static", contentVersion) != nil ||
		writeArtifactContent(&writer,
			component.types, component.references, component.declarations,
			component.signatures, component.contracts, component.operators,
			component.operands, component.publications) != nil ||
		writer.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

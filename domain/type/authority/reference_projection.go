package typeauthority

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/kind"
)

// ReferenceProjection is the one immutable semantic row issued for a static
// Program reference. Consumers read its scalar columns; they never resolve or
// re-walk the underlying type graph.
type ReferenceProjection struct {
	owner    *Authority
	ref      StaticTypeRef
	semantic identity.ContentID
	root     kind.Kind
	may      runtimekind.Set
	name     string
	open     bool
}

func bindReferenceProjection(owner *Authority, ref StaticTypeRef, artifact *artifactAuthority) (ReferenceProjection, RuntimeInput, bool) {
	if owner == nil || artifact == nil || !ref.Valid() {
		return ReferenceProjection{}, RuntimeInput{}, false
	}
	issued, ok := artifact.referenceProjection(ref.NodeID())
	if !ok {
		return ReferenceProjection{}, RuntimeInput{}, false
	}
	projection := ReferenceProjection{
		owner: owner, ref: ref, semantic: issued.semantic, root: issued.root,
		may: issued.may, name: issued.name, open: issued.open,
	}
	if projection.open {
		return projection, RuntimeInput{}, projection.valid()
	}
	if !issued.graph.Valid() {
		return ReferenceProjection{}, RuntimeInput{}, false
	}
	input := RuntimeInput{authority: owner, graph: issued.graph}
	inputIdentity, inputOK := input.CanonicalIdentity()
	return projection, input, projection.valid() && inputOK && inputIdentity == projection.semantic
}

func (projection ReferenceProjection) valid() bool {
	if projection.owner == nil || !projection.ref.Valid() || !projection.semantic.Available() ||
		!projection.may.Valid() {
		return false
	}
	return true
}

func (projection ReferenceProjection) SemanticIdentity() (identity.ContentID, bool) {
	return projection.semantic, projection.valid()
}

func (projection ReferenceProjection) RootKind() (kind.Kind, bool) {
	return projection.root, projection.valid()
}

func (projection ReferenceProjection) MayRuntimeKinds() (runtimekind.Set, bool) {
	return projection.may, projection.valid()
}

func (projection ReferenceProjection) Name() (string, bool) {
	return projection.name, projection.valid()
}

func (projection ReferenceProjection) Open() bool {
	return projection.valid() && projection.open
}

func (projection ReferenceProjection) ClosedInput() (RuntimeInput, bool) {
	if !projection.valid() || projection.open {
		return RuntimeInput{}, false
	}
	return projection.owner.runtimeInput(projection.ref, projection.semantic)
}

// Projection returns the row authenticated by one authority-issued reference.
func (a *Authority) Projection(ref StaticTypeRef) (ReferenceProjection, bool) {
	selector, ok := a.Lookup(ref)
	if !ok {
		return ReferenceProjection{}, false
	}
	entry, ok := a.entry(selector)
	return entry.projection, ok && entry.ref == ref && entry.projection.valid()
}

// ProjectionByReferenceID admits a Program reference ID and returns its one
// sealed semantic row without exposing the graph or an intermediate selector.
func (a *Authority) ProjectionByReferenceID(referenceID identity.ContentID) (ReferenceProjection, bool) {
	if a == nil || !referenceID.Available() {
		return ReferenceProjection{}, false
	}
	selector, ok := a.byReferenceID[referenceID]
	if !ok {
		return ReferenceProjection{}, false
	}
	entry, ok := a.entry(selector)
	return entry.projection, ok && entry.ref.NodeID() == referenceID && entry.projection.valid()
}

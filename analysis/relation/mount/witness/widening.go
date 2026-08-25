package witness

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

// WideningPermit is the opaque, mount-owned proof for one certified
// recurrence head.  It carries only the stable logical endpoints and the
// certificate evidence identity; callers cannot turn it back into a plan
// reference or manufacture one for an unadmitted pair.
type WideningPermit struct {
	dependency model.DependencyID
	relation   model.RelationID
	evidence   identity.ContentID
}

// Available reports whether this permit carries a complete certified proof.
func (permit WideningPermit) Available() bool {
	return permit.dependency.Available() && permit.relation.Available() && permit.evidence.Available()
}

// Dependency returns the certified dependency endpoint.
func (permit WideningPermit) Dependency() model.DependencyID {
	if !permit.Available() {
		return model.DependencyID{}
	}
	return permit.dependency
}

// Relation returns the certified recurrence-head relation endpoint.
func (permit WideningPermit) Relation() model.RelationID {
	if !permit.Available() {
		return model.RelationID{}
	}
	return permit.relation
}

// Evidence returns the immutable certificate identity authenticating this
// permit.
func (permit WideningPermit) Evidence() identity.ContentID {
	if !permit.Available() {
		return identity.ContentID{}
	}
	return permit.evidence
}

// newWideningPermit translates a checked plan head once at the admission
// boundary.  The resulting value is independent of schema/plan types and is
// the only widening representation retained by Mounted.
func newWideningPermit(head plan.WideningHead) (WideningPermit, bool) {
	if !head.Available() || !head.Dependency().Available() || !head.Relation().Available() {
		return WideningPermit{}, false
	}
	dependency := head.Dependency().ID()
	relation := head.Relation().ID()
	if !dependency.Available() || !relation.Available() || !head.Digest().Available() {
		return WideningPermit{}, false
	}
	return WideningPermit{dependency: dependency, relation: relation, evidence: head.Digest()}, true
}

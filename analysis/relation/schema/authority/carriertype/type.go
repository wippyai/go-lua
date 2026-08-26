package carriertype

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/authority"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

// Type projects one exact owner-issued carrier authority into the relation
// model's TypeID namespace.
//
// A carrier authority is not self-authenticating through its exported key and
// capability alone: those fields can be copied or changed while retaining an
// old private ID. Replaying issuance from the exact Owner.Entry proves both
// the owner fence and the complete declaration. The resulting TypeID carries
// the Owner.ID owner and the carrier Authority.ID content verbatim; no type
// name, Go representation, or capability mapping participates in its
// identity.
func Type(owner authority.Owner, value carrier.Authority) (model.TypeID, bool) {
	if !owner.Available() || !value.Available() || !value.Issued() {
		return model.TypeID{}, false
	}

	// carrier.Issue accepts only an unissued authority. Reconstructing this
	// declaration is intentional: it asks the carrier package to recompute the
	// private issuance proof instead of trusting the supplied Authority.ID.
	raw := carrier.Authority{Carrier: value.Carrier, Capability: value.Capability}
	expected, ok := carrier.Issue(owner.Entry, raw)
	if !ok || expected.ID() != value.ID() {
		return model.TypeID{}, false
	}

	ownerID := owner.ID()
	if !ownerID.Available() {
		return model.TypeID{}, false
	}
	content := identity.ContentID(value.ID())
	typeID, ok := model.IssueTypeID(ownerID, content)
	if !ok || !typeID.Available() || typeID.Owner() != ownerID || typeID.Content() != content {
		return model.TypeID{}, false
	}
	return typeID, true
}

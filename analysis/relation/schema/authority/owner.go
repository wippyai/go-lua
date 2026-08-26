package authority

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Owner is the exact sealed declaration entry that owns one attachment.
// Token is supplied by the owner (for example, an axis EntryID, a query
// Registration.EntryID, or a composite Entry.ID). This package never derives
// or replaces that token from Entry.
type Owner struct {
	Entry schema.EntryReference
	Token identity.ContentID
}

// NewOwner validates one owner fence without deriving an identity from the
// entry's authored label. The returned value is immutable because both fields
// are values, not references to caller-owned storage.
func NewOwner(entry schema.EntryReference, token identity.ContentID) (Owner, bool) {
	if !entry.Available() || !token.Available() {
		return Owner{}, false
	}
	if _, ok := model.IssueOwnerID(token); !ok {
		return Owner{}, false
	}
	return Owner{Entry: entry, Token: token}, true
}

// Available reports whether this owner carries an exact non-zero fence.
func (owner Owner) Available() bool {
	return owner.Entry.Available() && owner.Token.Available()
}

// ID returns the model owner identity carried by Token. An invalid owner
// returns the unavailable zero identity.
func (owner Owner) ID() model.OwnerID {
	if !owner.Available() {
		return model.OwnerID{}
	}
	value, ok := model.IssueOwnerID(owner.Token)
	if !ok {
		return model.OwnerID{}
	}
	return value
}

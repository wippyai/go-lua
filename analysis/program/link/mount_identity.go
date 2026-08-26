package link

import (
	"github.com/wippyai/go-lua/analysis/identity"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// linkMountIdentityDomain separates the Link-issued mounted-instance identity
// from every other identity derived from the same Link or Project authorities.
const linkMountIdentityDomain = "analysis/program/link/mount/v1"

// MountID issues the identity of one exact Project mount in this Link.
// Project.ModuleKey authenticates the Shard owner before the Link identity and
// mount key enter the single full-width derivation.  Certificates are runtime
// admission evidence and are intentionally not part of this identity.
func (l *Link) MountID(shard linkproject.Shard) (identity.MountID, bool) {
	if l == nil || l.project == nil {
		return identity.MountID{}, false
	}
	linkID := l.ContentID()
	if !linkID.Available() {
		return identity.MountID{}, false
	}
	moduleKey, ok := l.project.ModuleKey(shard)
	if !ok || !moduleKey.Available() {
		return identity.MountID{}, false
	}
	derived, ok := identity.DeriveContentID(linkMountIdentityDomain, linkID[:], moduleKey[:])
	if !ok {
		return identity.MountID{}, false
	}
	return identity.MountID(derived), true
}

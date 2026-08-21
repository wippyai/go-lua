package lifecycle

import "github.com/wippyai/go-lua/analysis/identity"

// StorageLifetime is the neutral, compiler-owned lifetime classification for
// one authored storage Cell. Unknown is intentionally a sealed state: an
// authored global is not promoted to process Shared storage until a mounted
// Host authority proves cross-context ownership.
type StorageLifetime uint8

const (
	StorageLifetimeInvalid StorageLifetime = iota
	StorageLifetimeFrame
	StorageLifetimeModule
	StorageLifetimeGlobal
	StorageLifetimeExternal
	StorageLifetimeUnknown
)

func (lifetime StorageLifetime) Valid() bool {
	return lifetime >= StorageLifetimeFrame && lifetime <= StorageLifetimeUnknown
}

// String returns the stable schema spelling used by diagnostics and laws.
func (lifetime StorageLifetime) String() string {
	switch lifetime {
	case StorageLifetimeFrame:
		return "frame"
	case StorageLifetimeModule:
		return "module"
	case StorageLifetimeGlobal:
		return "global"
	case StorageLifetimeExternal:
		return "external"
	case StorageLifetimeUnknown:
		return "unknown"
	default:
		return "lifetime(invalid)"
	}
}

// StorageCellLifetime is one canonical storage-cell identity and its neutral
// lifetime proof. The identity is the root-fenced StorageCellIdentity for the
// exact Program, rather than a Flow Cell term a mounted consumer must rebuild.
type StorageCellLifetime struct {
	id       identity.ContentID
	lifetime StorageLifetime
}

func NewStorageCellLifetime(id identity.ContentID, lifetime StorageLifetime) (StorageCellLifetime, bool) {
	row := StorageCellLifetime{id: id, lifetime: lifetime}
	return row, row.Available()
}

func (row StorageCellLifetime) Available() bool {
	return row.id.Available() && row.lifetime.Valid()
}

func (row StorageCellLifetime) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row StorageCellLifetime) Lifetime() StorageLifetime {
	if !row.Available() {
		return StorageLifetimeInvalid
	}
	return row.lifetime
}

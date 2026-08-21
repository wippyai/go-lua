package modulecomposition

import "github.com/wippyai/go-lua/analysis/identity"

// CacheIngress records the resolved import's ingress between its source and
// target cache/root identities. It stores only stable join identities; the
// exact request was validated against the canonical Program at construction.
type CacheIngress struct {
	id, link, importID, requestID    identity.ContentID
	sourceModuleKey, targetModuleKey identity.ContentID
	fromRootID, toRootID, actorID    identity.ContentID
	representativeInstanceID         identity.ContentID
}

// NewCacheIngress constructs one cache ingress from a resolved import. The
// source/target module keys and request identity are copied from that witness
// into the row's stable join, so a caller cannot provide a second geometry.
func NewCacheIngress(importRow ResolvedImport, fromRootID, toRootID, actorID, representativeInstanceID identity.ContentID) (CacheIngress, bool) {
	if !importRow.Available() || !fromRootID.Available() || !toRootID.Available() || !actorID.Available() ||
		!representativeInstanceID.Available() {
		return CacheIngress{}, false
	}
	row := CacheIngress{
		link: importRow.LinkID(), importID: importRow.ID(), requestID: importRow.RequestID(),
		sourceModuleKey: importRow.SourceModuleKey(), targetModuleKey: importRow.TargetModuleKey(),
		fromRootID: fromRootID, toRootID: toRootID, actorID: actorID,
		representativeInstanceID: representativeInstanceID,
	}
	row.id = cacheIngressID(row)
	return row, row.Available()
}

func (row CacheIngress) Available() bool {
	return row.id.Available() && row.link.Available() && row.importID.Available() && row.requestID.Available() &&
		row.sourceModuleKey.Available() && row.targetModuleKey.Available() && row.fromRootID.Available() && row.toRootID.Available() &&
		row.actorID.Available() && row.representativeInstanceID.Available() &&
		row.id == cacheIngressID(row)
}
func (row CacheIngress) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row CacheIngress) LinkID() identity.ContentID {
	if row.Available() {
		return row.link
	}
	return identity.ContentID{}
}
func (row CacheIngress) ImportID() identity.ContentID {
	if row.Available() {
		return row.importID
	}
	return identity.ContentID{}
}
func (row CacheIngress) RequestID() identity.ContentID {
	if row.Available() {
		return row.requestID
	}
	return identity.ContentID{}
}
func (row CacheIngress) SourceModuleKey() identity.ContentID {
	if row.Available() {
		return row.sourceModuleKey
	}
	return identity.ContentID{}
}
func (row CacheIngress) TargetModuleKey() identity.ContentID {
	if row.Available() {
		return row.targetModuleKey
	}
	return identity.ContentID{}
}
func (row CacheIngress) FromRootID() identity.ContentID {
	if row.Available() {
		return row.fromRootID
	}
	return identity.ContentID{}
}
func (row CacheIngress) ToRootID() identity.ContentID {
	if row.Available() {
		return row.toRootID
	}
	return identity.ContentID{}
}
func (row CacheIngress) ActorID() identity.ContentID {
	if row.Available() {
		return row.actorID
	}
	return identity.ContentID{}
}
func (row CacheIngress) RepresentativeInstanceID() identity.ContentID {
	if row.Available() {
		return row.representativeInstanceID
	}
	return identity.ContentID{}
}

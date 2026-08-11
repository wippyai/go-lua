package project

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

const (
	applicationIdentityFormat  uint64 = 0x6c696e6b2d617070 // "link-app"
	applicationIdentityVersion uint64 = 2
	applicationIdentityWords          = 22
	moduleIdentityFormat              = "program/link/project/module"
	moduleIdentityVersion             = 1
)

// ApplicationID derives the stable identity of one exact Project
// Application.  The preimage contains only Project's canonical application
// relation digest and the typed occurrence discriminator; it never includes
// the enclosing Link.ContentID or any sibling component.
func (c *Component) ApplicationID(application Application) (keyspace.ContentID, bool) {
	if c == nil || c.authority == nil || !c.authority.applicationContentID.Available() {
		return keyspace.ContentID{}, false
	}
	row, ok := (Applications{authority: c.authority}).application(application)
	if !ok {
		return keyspace.ContentID{}, false
	}
	return applicationID(c.authority.applicationContentID, row)
}

// ModuleKey derives the stable identity of one exact Project mount.  The
// digest is mount-local and excludes Target, Link, and all dependent Link
// component relations.
func (c *Component) ModuleKey(shard Shard) (keyspace.ContentID, bool) {
	if c == nil || c.authority == nil || !c.authority.mountContentID.Available() || shard.authority != c.authority || shard.ordinal == 0 || uint64(shard.ordinal) > uint64(len(c.authority.mounts)) {
		return keyspace.ContentID{}, false
	}
	mount := c.authority.mounts[shard.ordinal-1]
	if mount.program == nil || !mount.id.Available() || mount.program.ContentID() != mount.id {
		return keyspace.ContentID{}, false
	}
	var id keyspace.ContentID
	h := sha256.New()
	var w canonical.Writer
	if w.Reset(h, moduleIdentityFormat, moduleIdentityVersion) != nil ||
		w.Record(1) != nil || w.Bytes(c.authority.mountContentID[:]) != nil ||
		w.Uint(uint64(shard.ordinal)) != nil || w.Bytes(mount.id[:]) != nil || w.Finish() != nil {
		return keyspace.ContentID{}, false
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}, false
	}
	return id, id.Available()
}

func applicationID(relationID keyspace.ContentID, row applicationRow) (id keyspace.ContentID, ok bool) {
	if !relationID.Available() || row.kind == 0 || row.shard == 0 || row.term == 0 {
		return keyspace.ContentID{}, false
	}
	var payload [32 + applicationIdentityWords*8]byte
	copy(payload[:32], relationID[:])
	words := payload[32:]
	put := func(index int, value uint64) {
		binary.BigEndian.PutUint64(words[index*8:(index+1)*8], value)
	}
	put(0, applicationIdentityFormat)
	put(1, applicationIdentityVersion)
	put(7, uint64(row.kind))
	put(8, uint64(row.shard))
	put(9, uint64(row.term))
	put(17, uint64(row.slot))
	id = sha256.Sum256(payload[:])
	return id, id.Available()
}

package static

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/authority"
)

// TypeValueSeeds is Static's read-only finite observation of executable
// TypeValue occurrences.  It is deliberately not embedded in Authority: the
// occurrence relation has its own owner-fenced handles and cannot be used to
// manufacture Static values or Runtime rows.
type TypeValueSeeds struct{ authority *Authority }

type TypeValueSeed struct {
	owner *Authority
	index uint32
}

type typeValueRow struct {
	valueID identity.ContentID
	name    string
	root    identity.ContentID
	inner   typeauthority.RuntimeInner
	exact   bool
	id      identity.ContentID
}

func (a *Authority) TypeValueSeeds() TypeValueSeeds { return TypeValueSeeds{authority: a} }

func (s TypeValueSeeds) Count() int {
	if s.authority == nil {
		return 0
	}
	return len(s.authority.typeValues)
}

func (s TypeValueSeeds) At(index int) (TypeValueSeed, bool) {
	if s.authority == nil || index < 0 || index >= len(s.authority.typeValues) {
		return TypeValueSeed{}, false
	}
	return TypeValueSeed{owner: s.authority, index: uint32(index)}, true
}

func (s TypeValueSeeds) Name(seed TypeValueSeed) (string, bool) {
	row, ok := s.row(seed)
	return row.name, ok && row.name != ""
}

// RootIdentity is the Static-issued semantic representative key: primitives
// share by primitive kind, and named objects only within their exact mounted
// lexical activation.  TypeValue consumes it without rescanning Program.
func (s TypeValueSeeds) RootIdentity(seed TypeValueSeed) (identity.ContentID, bool) {
	row, ok := s.row(seed)
	return row.root, ok && row.root.Available()
}

// ValueIdentity is the detached Boundary identity for the occurrence. It is
// the only source coordinate that downstream TypeValue needs after Static
// sealing; no live Boundary Value crosses this surface.
func (s TypeValueSeeds) ValueIdentity(seed TypeValueSeed) (identity.ContentID, bool) {
	row, ok := s.row(seed)
	return row.valueID, ok && row.valueID.Available()
}

func (s TypeValueSeeds) ExactInner(seed TypeValueSeed) (typeauthority.RuntimeInner, bool) {
	row, ok := s.row(seed)
	return row.inner, ok && row.exact
}

func (s TypeValueSeeds) row(seed TypeValueSeed) (typeValueRow, bool) {
	if s.authority == nil || seed.owner != s.authority || uint64(seed.index) >= uint64(len(s.authority.typeValues)) {
		return typeValueRow{}, false
	}
	return s.authority.typeValues[seed.index], true
}

// Runtime is a read-only structural observation owned by Static.  Runtime
// receives no TypeValue occurrence rows and callers cannot supply one.
func (a *Authority) Runtime() (*typeauthority.Runtime, bool) {
	if a == nil || a.runtime == nil || a.runtime.LinkID() != a.linkID {
		return nil, false
	}
	return a.runtime, true
}

func staticTypeValueRowID(sourceID identity.ContentID, runtime *typeauthority.Runtime, result Value, row typeValueRow) (identity.ContentID, bool) {
	if !sourceID.Available() || !row.valueID.Available() || !row.root.Available() {
		return identity.ContentID{}, false
	}
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/typevalue-row\\x00\\x02"))
	_, _ = h.Write(sourceID[:])
	_, _ = h.Write(row.valueID[:])
	_, _ = h.Write(row.root[:])
	resultID, ok := staticResultIdentity(result)
	if !ok {
		return identity.ContentID{}, false
	}
	_, _ = h.Write(resultID[:])
	if row.exact {
		if runtime == nil {
			return identity.ContentID{}, false
		}
		_, _ = h.Write([]byte{1})
		inner, ok := runtime.Identity(row.inner)
		if !ok {
			return identity.ContentID{}, false
		}
		_, _ = h.Write(inner[:])
	} else {
		_, _ = h.Write([]byte{0})
	}
	var id identity.ContentID
	copy(id[:], h.Sum(nil))
	return id, id.Available()
}

// Package schema owns the declaration vocabulary shared by every schema
// surface. It deliberately does not own sealing: the seal subsystem lives in
// the child package schema/seal and imports this package, never the other way
// around.
package schema

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// Key is the authored identity of one entry within its surface. A key is a
// construction input; callers derive an EntryID before an identity crosses a
// verdict boundary.
type Key string

func (key Key) Available() bool { return key != "" }

// SurfaceKind is the closed declaration-surface catalog. The ordinal order is
// also the dependency order used by schema/seal: a surface may resolve entries
// on lower ordinals while it is being sealed, but not entries on a surface at
// or above its own ordinal.
type SurfaceKind uint8

const (
	SurfaceKindInvalid SurfaceKind = iota
	SurfaceKindStructure
	SurfaceKindAxis
	// SurfaceKindIssuance owns the declarative construction machine used to
	// place authored Program occurrences. It follows axes and precedes rules.
	SurfaceKindIssuance
	SurfaceKindRule
	SurfaceKindDiagnostic
	SurfaceKindComposite
	SurfaceKindDenominator
	SurfaceKindQuery
	SurfaceKindObservation

	// SurfaceKindLimit is the stable exclusive upper bound of the catalog. It
	// is exported so the child seal package and independent tooling can size
	// fixed catalogs without duplicating the ordinal count.
	SurfaceKindLimit
)

func (kind SurfaceKind) Available() bool {
	return kind > SurfaceKindInvalid && kind < SurfaceKindLimit
}

// EntryID is the stable content identity of one entry. It is derived from a
// contributor and authored key, so it survives declaration reordering.
type EntryID identity.ContentID

func (id EntryID) Available() bool { return identity.ContentID(id).Available() }

// NewEntryID derives one entry identity. Surfaces use it to name entries in a
// verdict without exposing authored text.
func NewEntryID(kind SurfaceKind, key Key) EntryID {
	if !kind.Available() || !key.Available() {
		return EntryID{}
	}
	hash := sha256.New()
	var framingBytes [8]byte
	binary.BigEndian.PutUint64(framingBytes[:], uint64(kind))
	if _, err := hash.Write(framingBytes[:]); err != nil {
		return EntryID{}
	}
	binary.BigEndian.PutUint64(framingBytes[:], uint64(len(key)))
	if _, err := hash.Write(framingBytes[:]); err != nil {
		return EntryID{}
	}
	if _, err := hash.Write([]byte(key)); err != nil {
		return EntryID{}
	}
	var id EntryID
	copy(id[:], hash.Sum(nil))
	return id
}

// LawID names an admission law. Root sealing laws are owned by schema/seal;
// surface packages allocate their ordinals above seal.SurfaceLawFloor.
type LawID uint16

// Disposition is the universal, kind-agnostic outcome vocabulary. It is the
// rendered part of a verdict; the contributor, entry, and law remain
// identities.
type Disposition uint8

const (
	DispositionAccepted Disposition = iota
	DispositionMalformed
	DispositionDuplicate
	DispositionIncomplete
)

func (disposition Disposition) String() string {
	switch disposition {
	case DispositionMalformed:
		return "malformed"
	case DispositionDuplicate:
		return "duplicate"
	case DispositionIncomplete:
		return "incomplete"
	default:
		return "accepted"
	}
}

// SealFailure is the public verdict of a rejected declaration table. The seal
// subsystem fills Schema once a rejection is attributable to a completed
// content stream; failures raised while the table is still being assembled
// leave it empty.
type SealFailure struct {
	Schema      identity.ContentID
	Contributor SurfaceKind
	Entry       EntryID
	Law         LawID
	Disposition Disposition
}

func (failure SealFailure) Available() bool { return failure.Law != 0 }

// EntryReference names one declaration on another surface. It is a
// schema-neutral declaration value: surfaces may carry their own typed meaning
// alongside it, while schema/seal only validates this common target. The
// authored key remains construction data and is never rendered in a
// SealFailure.
type EntryReference struct {
	Surface SurfaceKind
	Key     Key
}

func (reference EntryReference) Available() bool {
	return reference.Surface.Available() && reference.Key.Available()
}

// Declared distinguishes an absent optional reference from one whose fields
// are malformed. A surface may use that distinction when declaring optional
// relation arms; the seal subsystem still validates every declared reference.
func (reference EntryReference) Declared() bool { return reference != (EntryReference{}) }

// EntryReferences is the reusable ordered collection declaration for a row's
// cross-surface references. Ordering is declaration content, not identity.
type EntryReferences []EntryReference

func (references EntryReferences) Available() bool {
	for _, reference := range references {
		if !reference.Available() {
			return false
		}
	}
	return true
}

// Clone makes the transient copy used by schema/seal before invoking a
// surface's user-owned Seal method. It is intentionally nil-preserving.
func (references EntryReferences) Clone() EntryReferences {
	if references == nil {
		return nil
	}
	return append(EntryReferences(nil), references...)
}

// ReferenceProvider is the explicit root-owned hook through which a surface
// or entry exposes its common cross-surface declarations to schema/seal. The
// seal subsystem samples it once before invoking user-owned Seal.
type ReferenceProvider interface {
	References() EntryReferences
}

// EntryReferenceProvider is the explicit alternate hook for an entry that
// already uses References for a domain-local purpose. It has the same snapshot
// and validation semantics as ReferenceProvider.
type EntryReferenceProvider interface {
	EntryReferences() EntryReferences
}

// Entry is one declared row of any surface. The concrete record type remains
// in its surface package; the root only asks for identity, admissibility, and
// canonical declared content.
type Entry interface {
	Key() Key
	EntryAvailable() bool
	// EntryContent writes the canonical bytes of the entry's declared data to
	// the framing stream owned by schema/seal. Function values and other
	// executable hooks are not content; the declarative fields that select them
	// are.
	EntryContent(content *framing.Writer) error
}

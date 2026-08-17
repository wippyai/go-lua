package equation

import (
	"encoding/binary"
	"encoding/hex"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
	"github.com/wippyai/go-lua/analysis/schema"
)

// SealFailureFamily names the equation authority that refused one seal. It is
// the whole rendered classification a caller receives; the exact boundary
// reached inside that family travels beside it as an opaque site identity, so
// equation refines its own boundaries without moving public vocabulary.
type SealFailureFamily uint8

const (
	SealFailureFamilyNone SealFailureFamily = iota
	// Immutable source and target row admission of one Batch.
	SealFailureFamilySource
	// Topology seal: assembly, reissue, receipts, and operand realms.
	SealFailureFamilyTopology
	// Graph compilation of one sealed topology spec.
	SealFailureFamilyCompile
	// Canonical graph and topology key derivation.
	SealFailureFamilyIdentity
)

func (family SealFailureFamily) String() string {
	switch family {
	case SealFailureFamilySource:
		return "source"
	case SealFailureFamilyTopology:
		return "topology"
	case SealFailureFamilyCompile:
		return "compile"
	case SealFailureFamilyIdentity:
		return "identity"
	default:
		return "none"
	}
}

// SealFailure is equation's whole seal-failure vocabulary: the authority family
// a caller can act on, the universal schema disposition, and the opaque
// identity of the exact internal boundary. Site is a framed digest, so equation
// keeps full internal precision without any internal name crossing the API.
type SealFailure struct {
	Family      SealFailureFamily
	Disposition schema.Disposition
	Site        identity.ContentID
}

// Available reports whether this value classifies a failure.
func (failure SealFailure) Available() bool { return failure.Family != SealFailureFamilyNone }

// String renders the family, the disposition, and the leading bytes of the site
// digest. The prefix separates boundaries within a family without naming any of
// them.
func (failure SealFailure) String() string {
	if !failure.Available() {
		return "none"
	}
	return failure.Family.String() + "/" + failure.Disposition.String() + "@" + hex.EncodeToString(failure.Site[:4])
}

// Ordinals projects the boundary onto two scalars for a caller that frames it
// beside its own coordinates: the family and the leading site digest bytes. Two
// boundaries share both only if their site digests collide.
func (failure SealFailure) Ordinals() (uint64, uint64) {
	return uint64(failure.Family), binary.BigEndian.Uint64(failure.Site[:8])
}

const (
	sealFailureSiteDomain  = "analysis/engine/equation/seal-failure-site"
	sealFailureSiteVersion = 1
)

// sealRefused names one internal boundary whose fence rejected its input.
func sealRefused(family SealFailureFamily, site string) SealFailure {
	return sealFailureAt(family, schema.DispositionMalformed, site)
}

// The receipt compiler raises these three boundaries on equation's behalf: its
// own precondition and batch-identity checks guard a source seal it is about to
// perform, and its topology-input check guards a sealed Topology it did not
// receive. Exposing the exact values, rather than the minting constructor,
// keeps the site vocabulary closed to equation.
var (
	SealFailureSourcePrecondition  = sealRefused(SealFailureFamilySource, "precondition")
	SealFailureSourceBatchIdentity = sealRefused(SealFailureFamilySource, "batch-identity")
	SealFailureTopologyInput       = sealRefused(SealFailureFamilyTopology, "input")
)

// sealFailureAt mints the framed site identity. The family and the site name
// both enter the preimage, so two boundaries never share an identity.
func sealFailureAt(family SealFailureFamily, disposition schema.Disposition, site string) SealFailure {
	var writer canonical.DigestWriter
	if writer.Reset(sealFailureSiteDomain, sealFailureSiteVersion) != nil ||
		writer.Uint(uint64(family)) != nil || writer.Bytes([]byte(site)) != nil || writer.Finish() != nil {
		return SealFailure{Family: family, Disposition: disposition}
	}
	return SealFailure{Family: family, Disposition: disposition, Site: identity.ContentID(writer.Sum())}
}

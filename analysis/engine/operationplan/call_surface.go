package operationplan

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// CallSurfaceTargetKind identifies the authority which resolved a call target.
// Values are semantic encoding tags and must remain stable.
type CallSurfaceTargetKind uint8

const (
	CallSurfaceTargetLexical CallSurfaceTargetKind = iota + 1
	CallSurfaceTargetExternal
	CallSurfaceTargetRejected
)

// CallSurfaceTarget is one sealed call-target classification. Its fields are
// private so a consumer cannot construct a mixed lexical/external identity.
type CallSurfaceTarget struct {
	kind     CallSurfaceTargetKind
	lexical  lexicalidentity.StableLexicalBodyID
	external SignatureCallOperation
	residue  callSurfaceResidue
}

// callSurfaceResidueKind is intentionally private until semantic/program owns
// a consumer. Values are canonical encoding tags, not scheduling authority.
type callSurfaceResidueKind uint8

const (
	callSurfaceResidueUnresolved callSurfaceResidueKind = iota + 1
	callSurfaceResidueExternal
)

type callSurfaceHintKind uint8

const (
	callSurfaceHintNone callSurfaceHintKind = iota + 1
	callSurfaceHintPath
	callSurfaceHintMethod
	callSurfaceHintTemporary
	callSurfaceHintExternalContent
)

type callSurfaceTargetHint struct {
	kind       callSurfaceHintKind
	path       pathdom.PathKey
	member     string
	temporary  uint32
	externalID signature.ContentID
}

func (h callSurfaceTargetHint) valid() bool {
	switch h.kind {
	case callSurfaceHintNone:
		return h.path == "" && h.member == "" && h.temporary == 0 && !h.externalID.Available()
	case callSurfaceHintPath:
		return h.path != "" && h.member == "" && h.temporary == 0 && !h.externalID.Available()
	case callSurfaceHintMethod:
		return h.path != "" && h.member != "" && h.temporary == 0 && !h.externalID.Available()
	case callSurfaceHintTemporary:
		// WIR temporaries are zero-based. The hint kind, rather than the
		// numeric payload, distinguishes temporary zero from no hint.
		return h.path == "" && h.member == "" && !h.externalID.Available()
	case callSurfaceHintExternalContent:
		return h.path == "" && h.member == "" && h.temporary == 0 && h.externalID.Available()
	default:
		return false
	}
}

type callSurfaceResidue struct {
	kind callSurfaceResidueKind
	hint callSurfaceTargetHint
}

func (r callSurfaceResidue) valid() bool {
	return (r.kind == callSurfaceResidueUnresolved || r.kind == callSurfaceResidueExternal) && r.hint.valid()
}

// NewLexicalCallSurfaceTarget binds a call to one stable lexical body.
func NewLexicalCallSurfaceTarget(body lexicalidentity.StableLexicalBodyID) (CallSurfaceTarget, bool) {
	if body == (lexicalidentity.StableLexicalBodyID{}) {
		return CallSurfaceTarget{}, false
	}
	return CallSurfaceTarget{kind: CallSurfaceTargetLexical, lexical: body}, true
}

// NewExternalCallSurfaceTarget binds a call to canonical external signature
// content. A sealed intrinsic, including Lua type, remains part of the owned
// operation rather than being reconstructed from a name by a consumer.
func NewExternalCallSurfaceTarget(operation SignatureCallOperation) (CallSurfaceTarget, bool) {
	if !operation.valid() {
		return CallSurfaceTarget{}, false
	}
	content := operation.ContentID()
	return CallSurfaceTarget{kind: CallSurfaceTargetExternal, external: operation.clone(), residue: callSurfaceResidue{
		kind: callSurfaceResidueExternal,
		hint: callSurfaceTargetHint{kind: callSurfaceHintExternalContent, externalID: content},
	}}, true
}

func unresolvedCallSurfaceTarget(hint callSurfaceTargetHint) CallSurfaceTarget {
	residue := callSurfaceResidue{kind: callSurfaceResidueUnresolved, hint: hint}
	if !residue.valid() {
		residue = callSurfaceResidue{kind: callSurfaceResidueUnresolved, hint: callSurfaceTargetHint{kind: callSurfaceHintNone}}
	}
	return CallSurfaceTarget{kind: CallSurfaceTargetRejected, residue: residue}
}

// RejectedCallSurfaceTarget explicitly records a call which the resolution
// authority could not classify. Rejected calls remain in the complete census.
func RejectedCallSurfaceTarget() CallSurfaceTarget {
	return unresolvedCallSurfaceTarget(callSurfaceTargetHint{kind: callSurfaceHintNone})
}

// RejectedPathCallSurfaceTarget records a stable lexical path hint without
// claiming that the runtime value is a known function.
func RejectedPathCallSurfaceTarget(path pathdom.PathKey) CallSurfaceTarget {
	return unresolvedCallSurfaceTarget(callSurfaceTargetHint{kind: callSurfaceHintPath, path: path})
}

// RejectedMethodCallSurfaceTarget records method-call syntax without claiming
// a lexical or external target.
func RejectedMethodCallSurfaceTarget(receiver pathdom.PathKey, method string) CallSurfaceTarget {
	return unresolvedCallSurfaceTarget(callSurfaceTargetHint{kind: callSurfaceHintMethod, path: receiver, member: method})
}

// RejectedTemporaryCallSurfaceTarget records a WIR-local dynamic callee hint.
func RejectedTemporaryCallSurfaceTarget(temporary uint32) CallSurfaceTarget {
	return unresolvedCallSurfaceTarget(callSurfaceTargetHint{kind: callSurfaceHintTemporary, temporary: temporary})
}

func (t CallSurfaceTarget) Kind() CallSurfaceTargetKind { return t.kind }

// LexicalBody returns the stable lexical target when Kind is lexical.
func (t CallSurfaceTarget) LexicalBody() (lexicalidentity.StableLexicalBodyID, bool) {
	if t.kind != CallSurfaceTargetLexical || t.lexical == (lexicalidentity.StableLexicalBodyID{}) {
		return lexicalidentity.StableLexicalBodyID{}, false
	}
	return t.lexical, true
}

// ExternalContentID returns the full-width canonical signature identity when
// Kind is external.
func (t CallSurfaceTarget) ExternalContentID() (signature.ContentID, bool) {
	if t.kind != CallSurfaceTargetExternal || !t.external.valid() {
		return signature.ContentID{}, false
	}
	return t.external.ContentID(), true
}

// ExternalOperation returns an owned external descriptor. It preserves sealed
// intrinsic identity without exposing mutable signature storage.
func (t CallSurfaceTarget) ExternalOperation() (SignatureCallOperation, bool) {
	if t.kind != CallSurfaceTargetExternal || !t.external.valid() {
		return SignatureCallOperation{}, false
	}
	return t.external.clone(), true
}

// MatchesExternalOperation reports whether t owns exactly the same resolved
// signature-call descriptor as operation. Consumers use this narrow seam
// instead of reconstructing descriptor equality from names or digests.
func (t CallSurfaceTarget) MatchesExternalOperation(operation SignatureCallOperation) bool {
	return t.kind == CallSurfaceTargetExternal && t.external.valid() && operation.valid() && t.external.equal(operation)
}

func (t CallSurfaceTarget) valid() bool {
	switch t.kind {
	case CallSurfaceTargetLexical:
		return t.lexical != (lexicalidentity.StableLexicalBodyID{}) && !t.external.valid() && !t.residue.valid()
	case CallSurfaceTargetExternal:
		return t.lexical == (lexicalidentity.StableLexicalBodyID{}) && t.external.valid() && t.residue.valid() && t.residue.kind == callSurfaceResidueExternal
	case CallSurfaceTargetRejected:
		return t.lexical == (lexicalidentity.StableLexicalBodyID{}) && !t.external.valid() && t.residue.valid() && t.residue.kind == callSurfaceResidueUnresolved
	default:
		return false
	}
}

// CallSurfaceSite classifies the single statically extracted call at Point.
type CallSurfaceSite struct {
	Point  cfg.Point
	Target CallSurfaceTarget
}

// CallSurfaceDigest is the full-width canonical identity of a complete call
// census. It includes owner, CFG point count, every point, target namespace,
// and target content identity.
type CallSurfaceDigest [sha256.Size]byte

func (d CallSurfaceDigest) Available() bool { return d != CallSurfaceDigest{} }

// CallSurface is an immutable, canonically ordered census of every call in one
// lexical body. Complete means the classified site points matched the
// independently extracted call point set exactly; rejected targets do not make
// a census partial. This surface deliberately does not claim shared-resource
// dependencies: capture/global/heap/allocation/effect edges require a separate
// transaction-owned authority and must never be inferred from call syntax.
type CallSurface struct {
	owner      lexicalidentity.StableLexicalBodyID
	pointCount int
	sites      []CallSurfaceSite
	digest     CallSurfaceDigest
	complete   bool
}

// WithCallSurface binds a complete lowering-owned call census to the plan.
// Ownership and CFG width must agree with the already attached observation
// identity. Invalid or independently prepared surfaces clear the authority so
// downstream compositional admission fails closed.
func (p *Plan) WithCallSurface(surface CallSurface) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.callSurface = CallSurface{}
	if !surface.Complete() || !surface.Digest().Available() ||
		p.observationBody == (lexicalidentity.StableLexicalBodyID{}) ||
		surface.Owner() != p.observationBody || surface.PointCount() != p.PointCount() {
		return &out
	}
	out.callSurface = surface
	return &out
}

// CallSurface returns the immutable complete call census owned by this plan.
// The bool is false when preparation could not certify the independent census.
func (p *Plan) CallSurface() (CallSurface, bool) {
	if p == nil || !p.callSurface.Complete() || !p.callSurface.Digest().Available() ||
		p.callSurface.Owner() != p.observationBody || p.callSurface.PointCount() != p.PointCount() {
		return CallSurface{}, false
	}
	return p.callSurface, true
}

// SealCallSurface validates and owns a complete call census. extractedCallPoints
// must come from the extraction authority rather than the classifier. Exact
// point-set equality prevents a classifier from substituting an unrelated CFG
// point for a missed call while preserving the expected cardinality. At most
// one call may occupy a CFG point.
func SealCallSurface(
	owner lexicalidentity.StableLexicalBodyID,
	pointCount int,
	extractedCallPoints []cfg.Point,
	sites []CallSurfaceSite,
) (CallSurface, error) {
	if owner == (lexicalidentity.StableLexicalBodyID{}) {
		return CallSurface{}, errors.New("operationplan: call surface has no lexical owner")
	}
	if pointCount < 2 {
		return CallSurface{}, fmt.Errorf("operationplan: call surface point count %d is below the CFG minimum", pointCount)
	}
	extracted := append([]cfg.Point(nil), extractedCallPoints...)
	sort.Slice(extracted, func(i, j int) bool { return extracted[i] < extracted[j] })
	for index, point := range extracted {
		if uint64(point) >= uint64(pointCount) {
			return CallSurface{}, fmt.Errorf("operationplan: extracted call point %d outside point count %d", point, pointCount)
		}
		if index != 0 && extracted[index-1] == point {
			return CallSurface{}, fmt.Errorf("operationplan: extracted call point %d is duplicated", point)
		}
	}
	if len(extracted) != len(sites) {
		return CallSurface{}, fmt.Errorf("operationplan: call surface count mismatch: extracted=%d classified=%d", len(extracted), len(sites))
	}

	owned := append([]CallSurfaceSite(nil), sites...)
	sort.Slice(owned, func(i, j int) bool { return owned[i].Point < owned[j].Point })
	for index, site := range owned {
		if uint64(site.Point) >= uint64(pointCount) {
			return CallSurface{}, fmt.Errorf("operationplan: call surface point %d outside point count %d", site.Point, pointCount)
		}
		if !site.Target.valid() {
			return CallSurface{}, fmt.Errorf("operationplan: call surface point %d has invalid target", site.Point)
		}
		if index != 0 && owned[index-1].Point == site.Point {
			return CallSurface{}, fmt.Errorf("operationplan: call surface point %d is duplicated", site.Point)
		}
		if extracted[index] != site.Point {
			return CallSurface{}, fmt.Errorf("operationplan: classified call point %d does not match extracted call point %d", site.Point, extracted[index])
		}
	}

	digest := digestCallSurface(owner, pointCount, owned)
	if !digest.Available() { // SHA-256 cannot practically be zero; retain a strict publication invariant.
		return CallSurface{}, errors.New("operationplan: call surface digest unavailable")
	}
	return CallSurface{owner: owner, pointCount: pointCount, sites: owned, digest: digest, complete: true}, nil
}

func (s CallSurface) Owner() lexicalidentity.StableLexicalBodyID { return s.owner }
func (s CallSurface) PointCount() int                            { return s.pointCount }
func (s CallSurface) Complete() bool                             { return s.complete }
func (s CallSurface) Digest() CallSurfaceDigest                  { return s.digest }

// Sites returns the canonical point-sorted census. Target payloads expose no
// mutable storage; external descriptors are cloned by ExternalOperation.
func (s CallSurface) Sites() []CallSurfaceSite {
	return append([]CallSurfaceSite(nil), s.sites...)
}

// Site returns the classification at point without scanning the surface.
func (s CallSurface) Site(point cfg.Point) (CallSurfaceSite, bool) {
	index := sort.Search(len(s.sites), func(index int) bool { return s.sites[index].Point >= point })
	if index == len(s.sites) || s.sites[index].Point != point {
		return CallSurfaceSite{}, false
	}
	return s.sites[index], true
}

func digestCallSurface(owner lexicalidentity.StableLexicalBodyID, pointCount int, sites []CallSurfaceSite) CallSurfaceDigest {
	hash := sha256.New()
	writeCallSurfaceBytes(hash, []byte("wippy.operationplan.call-surface.v2"))
	writeCallSurfaceBytes(hash, owner[:])
	writeCallSurfaceUint64(hash, uint64(pointCount))
	writeCallSurfaceUint64(hash, uint64(len(sites)))
	for _, site := range sites {
		writeCallSurfaceUint64(hash, uint64(site.Point))
		_, _ = hash.Write([]byte{byte(site.Target.kind)})
		switch site.Target.kind {
		case CallSurfaceTargetLexical:
			writeCallSurfaceBytes(hash, site.Target.lexical[:])
		case CallSurfaceTargetExternal:
			content := site.Target.external.ContentID()
			writeCallSurfaceBytes(hash, content[:])
			intrinsic, _ := site.Target.external.Intrinsic()
			_, _ = hash.Write([]byte{byte(intrinsic)})
		case CallSurfaceTargetRejected:
		}
		if site.Target.residue.valid() {
			_, _ = hash.Write([]byte{byte(site.Target.residue.kind), byte(site.Target.residue.hint.kind)})
			digestCallSurfaceHint(hash, site.Target.residue.hint)
		}
	}
	var out CallSurfaceDigest
	copy(out[:], hash.Sum(nil))
	return out
}

func digestCallSurfaceHint(hash callSurfaceWriter, hint callSurfaceTargetHint) {
	switch hint.kind {
	case callSurfaceHintPath:
		writeCallSurfaceBytes(hash, []byte(hint.path))
	case callSurfaceHintMethod:
		writeCallSurfaceBytes(hash, []byte(hint.path))
		writeCallSurfaceBytes(hash, []byte(hint.member))
	case callSurfaceHintTemporary:
		writeCallSurfaceUint64(hash, uint64(hint.temporary))
	case callSurfaceHintExternalContent:
		writeCallSurfaceBytes(hash, hint.externalID[:])
	case callSurfaceHintNone:
	}
}

type callSurfaceWriter interface{ Write([]byte) (int, error) }

func writeCallSurfaceBytes(writer callSurfaceWriter, value []byte) {
	writeCallSurfaceUint64(writer, uint64(len(value)))
	_, _ = writer.Write(value)
}

func writeCallSurfaceUint64(writer callSurfaceWriter, value uint64) {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	_, _ = writer.Write(raw[:])
}

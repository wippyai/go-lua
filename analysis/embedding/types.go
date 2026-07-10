package embedding

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
)

// DocumentScheme identifies the namespace used by a DocumentID. The value is
// deliberately small and closed at this first pinned schema: host-specific
// information belongs in OpaqueKey rather than a new identity shape.
type DocumentScheme string

const (
	DocumentSchemeFile     DocumentScheme = "file"
	DocumentSchemeRegistry DocumentScheme = "registry"
	DocumentSchemeMem      DocumentScheme = "mem"
)

// DocumentID names one logical document across edits and provider revisions.
// It is comparable and its equality intentionally excludes every revision,
// digest, overlay version, and resolver view.
type DocumentID struct {
	Scheme    DocumentScheme
	OpaqueKey string
}

// FileDocument, RegistryDocument, and MemDocument construct IDs in the three
// initial namespaces. Registry callers should include an explicit source-slot
// discriminator in key (for example, entry-id plus logical source field).
func FileDocument(key string) DocumentID {
	return DocumentID{Scheme: DocumentSchemeFile, OpaqueKey: key}
}

func RegistryDocument(key string) DocumentID {
	return DocumentID{Scheme: DocumentSchemeRegistry, OpaqueKey: key}
}

func MemDocument(key string) DocumentID {
	return DocumentID{Scheme: DocumentSchemeMem, OpaqueKey: key}
}

// Valid reports whether id is a supported non-empty stable document identity.
func (id DocumentID) Valid() bool {
	return id.OpaqueKey != "" && (id.Scheme == DocumentSchemeFile || id.Scheme == DocumentSchemeRegistry || id.Scheme == DocumentSchemeMem)
}

func (id DocumentID) String() string {
	if id.Scheme == "" {
		return id.OpaqueKey
	}
	return string(id.Scheme) + ":" + id.OpaqueKey
}

// Digest is a stable SHA-256 content-address. Semantic consumers verify it
// from bytes instead of trusting a provider-provided value.
type Digest [sha256.Size]byte

func DigestBytes(data []byte) Digest { return sha256.Sum256(data) }

func (d Digest) String() string { return hex.EncodeToString(d[:]) }

func (d Digest) IsZero() bool { return d == Digest{} }

func (d Digest) MarshalText() ([]byte, error) {
	out := make([]byte, hex.EncodedLen(len(d)))
	hex.Encode(out, d[:])
	return out, nil
}

// ByteSpan is a half-open byte range in the exact bytes identified by a
// SourceLocation.ContentDigest. LSP UTF-16 positions are a frontend projection
// and must be calculated from that exact snapshot.
type ByteSpan struct {
	StartByte int
	EndByte   int
}

func (s ByteSpan) Valid() bool { return s.StartByte >= 0 && s.EndByte >= s.StartByte }

// SourceLocation binds a source range to both stable document identity and the
// exact content snapshot in which the range was produced. ByteSpan is
// canonical; line/column values are an optional display convenience retained
// for existing compiler and terminal diagnostics.
type SourceLocation struct {
	Document      DocumentID
	ContentDigest Digest
	Span          ByteSpan
	StartLine     int
	StartColumn   int
	EndLine       int
	EndColumn     int
}

func (l SourceLocation) Valid() bool {
	return l.Document.Valid() && !l.ContentDigest.IsZero() && l.Span.Valid()
}

// SourceSnapshot is an immutable provider result. ProviderRevision is an
// opaque fence for a provider or overlay; it has no semantic identity. The
// checker recomputes ContentDigest from Content before solving.
type SourceSnapshot struct {
	Document         DocumentID
	ProviderRevision string
	ContentDigest    Digest
	Content          []byte
}

// Clone returns a value whose Content does not share mutable backing storage.
func (s SourceSnapshot) Clone() SourceSnapshot {
	s.Content = append([]byte(nil), s.Content...)
	return s
}

// Verify recomputes the content digest and validates the snapshot identity.
// A zero ContentDigest is normalized to the computed digest for convenient
// host construction; a non-zero mismatched digest is rejected.
func (s SourceSnapshot) Verify() (SourceSnapshot, error) {
	if !s.Document.Valid() {
		return SourceSnapshot{}, fmt.Errorf("embedding: invalid document id %q", s.Document)
	}
	computed := DigestBytes(s.Content)
	if !s.ContentDigest.IsZero() && s.ContentDigest != computed {
		return SourceSnapshot{}, errors.New("embedding: source snapshot content digest does not match content")
	}
	s.ContentDigest = computed
	return s.Clone(), nil
}

// UnitID is a stable identity for one independently resolved analysis unit.
type UnitID string

// WorkspaceViewID identifies a host-owned resolution view. It is distinct
// from a document identity and can incorporate roots, overlay namespace,
// registry snapshot, and resolution policy.
type WorkspaceViewID string

// UnitImport records a resolver-selected dependency. Imports are explicit
// materialized edges: source providers never resolve module names.
type UnitImport struct {
	Alias          string
	TargetUnit     UnitID
	ManifestDigest Digest
}

// UnitPlan is one fully resolved unit in a ResolutionSnapshot. Sources are
// document identities; their exact bytes are supplied separately as immutable
// SourceSnapshots before the engine is invoked.
type UnitPlan struct {
	ID         UnitID
	ModulePath string
	Entry      DocumentID
	Sources    []DocumentID
	Imports    []UnitImport
}

// Clone returns a detached plan and canonicalizes no host policy.
func (p UnitPlan) Clone() UnitPlan {
	p.Sources = append([]DocumentID(nil), p.Sources...)
	p.Imports = append([]UnitImport(nil), p.Imports...)
	return p
}

// ResolutionSnapshot freezes the resolver view from which a host materializes
// all UnitPlan sources. ViewDigest fences the complete graph/view, not one
// source revision.
type ResolutionSnapshot struct {
	View       WorkspaceViewID
	ViewDigest Digest
	Units      []UnitPlan
}

func (s ResolutionSnapshot) Clone() ResolutionSnapshot {
	s.Units = append([]UnitPlan(nil), s.Units...)
	for i := range s.Units {
		s.Units[i] = s.Units[i].Clone()
	}
	return s
}

// DocumentLabeler is an optional display projection. Labels never participate
// in document equality, source digests, unit digests, or cache keys.
type DocumentLabeler interface {
	Label(DocumentID) string
}

// StaticLabels is a convenient immutable label projection for hosts that have
// already materialized document labels.
type StaticLabels map[DocumentID]string

func (l StaticLabels) Label(id DocumentID) string { return l[id] }

// SortedDocuments returns IDs in deterministic scheme/key order.
func SortedDocuments(items map[DocumentID]SourceSnapshot) []DocumentID {
	out := make([]DocumentID, 0, len(items))
	for id := range items {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scheme != out[j].Scheme {
			return out[i].Scheme < out[j].Scheme
		}
		return out[i].OpaqueKey < out[j].OpaqueKey
	})
	return out
}

// SolveSeq is a session-local publication ordinal. It is deliberately distinct
// from BodyInputDigest and is never a runtime/deployment identity.
type SolveSeq uint64

// BodyInputDigest identifies the fully consumed inputs for one solved body.
// The current engine's body hash is 64-bit; the named type prevents it from
// being confused with a session SolveSeq.
type BodyInputDigest uint64

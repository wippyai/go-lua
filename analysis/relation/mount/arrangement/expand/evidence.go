// Package expand owns only the physical evidence redeemed by a mounted
// algebra.Expand.  The logical operator and its C/P/R contract belong to the
// schema packages; this child package deliberately carries no second logical
// vocabulary.
package expand

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

const evidenceDigestDomain = "analysis/relation/mount/arrangement/expand/evidence/v2"

// Vector is the owner-issued semantic vector for one C→P correspondence.
// Candidate is the nominal identity of the C row, Publisher is the exact
// nominal identity of the paired P row, and Keys are the ordered identities
// of rows in R. RowID (rather than a bare content digest) keeps C and P
// relation namespaces explicit; Correlation in the logical contract is not
// treated as row correspondence. It contains no dense ordinal, physical
// address, callback, or geometry.
type Vector struct {
	candidate model.RowID
	publisher model.RowID
	keys      []identity.ContentID
}

// NewVector freezes one owner response. A non-nil empty key slice is a valid
// closed empty vector; nil is unavailable so an absent response cannot be
// confused with an authenticated empty extent.
func NewVector(candidate, publisher model.RowID, keys []identity.ContentID) (Vector, bool) {
	if !candidate.Available() || !publisher.Available() || keys == nil {
		return Vector{}, false
	}
	copyOf := append([]identity.ContentID(nil), keys...)
	for index, key := range copyOf {
		if !key.Available() {
			return Vector{}, false
		}
		for _, prior := range copyOf[:index] {
			if prior == key {
				return Vector{}, false
			}
		}
	}
	return Vector{candidate: candidate, publisher: publisher, keys: copyOf}, true
}

func (vector Vector) Available() bool {
	if !vector.candidate.Available() || !vector.publisher.Available() || vector.keys == nil {
		return false
	}
	for index, key := range vector.keys {
		if !key.Available() {
			return false
		}
		for _, prior := range vector.keys[:index] {
			if prior == key {
				return false
			}
		}
	}
	return true
}

// Candidate returns the owner-issued subject identity.
func (vector Vector) Candidate() model.RowID {
	if !vector.Available() {
		return model.RowID{}
	}
	return vector.candidate
}

// Publisher returns the paired P-row identity authenticated by the owner.
func (vector Vector) Publisher() model.RowID {
	if !vector.Available() {
		return model.RowID{}
	}
	return vector.publisher
}

// KeyCount returns the sealed vector width without allocating.
func (vector Vector) KeyCount() int {
	if !vector.Available() {
		return 0
	}
	return len(vector.keys)
}

// KeyAt returns one owner-issued semantic row identity in authored order.
func (vector Vector) KeyAt(index int) (identity.ContentID, bool) {
	if !vector.Available() || index < 0 || index >= len(vector.keys) {
		return identity.ContentID{}, false
	}
	return vector.keys[index], true
}

// Evidence is the immutable, runtime-ready expansion catalogue. KeyIDs are
// retained as owner-issued ValueTokens under the mount runtime fence. Raw C/P
// identities are seal-only evidence: they are validated and committed to the
// digest, while runtime rows retain only C and the issued R-key tokens.
type Evidence struct {
	fence       binding.Fence
	contract    model.ExpandContract
	keyType     model.TypeID
	rows        []vector
	byCandidate map[model.RowID]int
	byKey       map[evidenceKey][]int
	digest      identity.ContentID
}

type evidenceKey struct {
	typeID  model.TypeID
	content identity.ContentID
}

type vector struct {
	candidate model.RowID
	keys      []binding.ValueToken
}

// Freeze validates owner evidence and issues all row tokens exactly once.
// The key TypeID is the sealed type of the Expand reader column; callers may
// not supply a per-row type or a second codec authority.
func Freeze(fence binding.Fence, issuer binding.Issuer, contract model.ExpandContract, keyType model.TypeID, raw []Vector) (Evidence, bool) {
	if !fence.Available() || !issuer.Available() || !issuer.Fence().Same(fence) || !contract.Available() || !keyType.Available() || raw == nil {
		return Evidence{}, false
	}
	// Expand is mounted only after placement has attached its exact port scope.
	// The owner vector's order and sparse extent are intrinsic to the frozen
	// evidence; runtime does not interpret a second delivery policy.
	if !contract.Scope().Available() {
		return Evidence{}, false
	}
	rows := make([]vector, len(raw))
	byCandidate := make(map[model.RowID]int, len(raw))
	byKey := make(map[evidenceKey][]int)
	seenPublisher := make(map[model.RowID]struct{}, len(raw))
	parts := make([][]byte, 0, 4+len(raw))
	contractDigest := contract.Digest()
	if !contractDigest.Available() {
		return Evidence{}, false
	}
	parts = append(parts, contentBytes(contractDigest), nominalBytes(keyType.Owner().Content(), keyType.Content()))
	for index, source := range raw {
		if !source.Available() {
			return Evidence{}, false
		}
		if source.candidate.Relation() != contract.Candidate() || source.publisher.Relation() != contract.Publisher() {
			return Evidence{}, false
		}
		if _, duplicate := byCandidate[source.candidate]; duplicate {
			return Evidence{}, false
		}
		if _, duplicate := seenPublisher[source.publisher]; duplicate {
			return Evidence{}, false
		}
		seenPublisher[source.publisher] = struct{}{}
		keys := make([]binding.ValueToken, len(source.keys))
		for keyIndex, content := range source.keys {
			token, ok := issuer.IssueValue(keyType, content)
			if !ok || !token.Available() || !token.ValidFor(fence) {
				return Evidence{}, false
			}
			keys[keyIndex] = token
		}
		// Publisher remains in the sealed raw evidence and in the digest above,
		// but it is not runtime state: Expand never reads or wakes on P.
		rows[index] = vector{candidate: source.candidate, keys: keys}
		byCandidate[source.candidate] = index
		for _, token := range keys {
			key := evidenceKey{typeID: token.Type(), content: token.Opaque()}
			byKey[key] = append(byKey[key], index)
		}
		parts = append(parts, vectorDigest(source.candidate, source.publisher, keys))
	}
	digest, ok := identity.DeriveContentID(evidenceDigestDomain, parts...)
	if !ok {
		return Evidence{}, false
	}
	result := Evidence{fence: fence, contract: contract, keyType: keyType, rows: rows, byCandidate: byCandidate, byKey: byKey, digest: digest}
	return result, result.Available()
}

// Available is an O(1) sealed-catalogue check. Freeze performs all row,
// duplicate, token, and digest validation before publishing Evidence.
func (evidence Evidence) Available() bool {
	return evidence.fence.Available() && evidence.contract.Available() && evidence.keyType.Available() && evidence.digest.Available() && evidence.rows != nil && evidence.byCandidate != nil && evidence.byKey != nil && len(evidence.rows) == len(evidence.byCandidate)
}

func (evidence Evidence) Fence() binding.Fence {
	if !evidence.Available() {
		return binding.Fence{}
	}
	return evidence.fence
}

func (evidence Evidence) Contract() model.ExpandContract {
	if !evidence.Available() {
		return model.ExpandContract{}
	}
	return evidence.contract
}

func (evidence Evidence) KeyType() model.TypeID {
	if !evidence.Available() {
		return model.TypeID{}
	}
	return evidence.keyType
}

func (evidence Evidence) Digest() identity.ContentID {
	if !evidence.Available() {
		return identity.ContentID{}
	}
	return evidence.digest
}

// ValidFor reports whether this frozen owner evidence belongs to the exact
// mounted fence.  The catalog is redeemed only after this check; a matching
// logical contract with a different mount is not reusable evidence.
func (evidence Evidence) ValidFor(fence binding.Fence) bool {
	return evidence.Available() && fence.Available() && evidence.fence.Same(fence)
}

// Entry associates one logical Expand node with its already-frozen owner
// evidence.  The expression identity is the only lookup coordinate; no
// ordinal, geometry, or callback crosses the arrangement boundary.
type Entry struct {
	Expression identity.ContentID
	Evidence   Evidence
}

type catalogEntry struct {
	expression identity.ContentID
	evidence   Evidence
}

// Catalog is the immutable mount-time Expand evidence directory.  It is a
// physical witness catalogue, not a second algebra or contract vocabulary.
// NewCatalog validates all entries and computes one deterministic digest.
type Catalog struct {
	entries      []catalogEntry
	byExpression map[identity.ContentID]int
	digest       identity.ContentID
}

const catalogDigestDomain = evidenceDigestDomain + "/catalog"

func NewCatalog(entries []Entry) (Catalog, bool) {
	if entries == nil {
		entries = []Entry{}
	}
	ordered := append([]Entry(nil), entries...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return bytes.Compare(ordered[left].Expression[:], ordered[right].Expression[:]) < 0
	})
	result := Catalog{entries: make([]catalogEntry, len(ordered)), byExpression: make(map[identity.ContentID]int, len(ordered))}
	parts := make([][]byte, 0, 1+len(ordered))
	for index, entry := range ordered {
		if !entry.Expression.Available() || !entry.Evidence.Available() {
			return Catalog{}, false
		}
		if _, duplicate := result.byExpression[entry.Expression]; duplicate {
			return Catalog{}, false
		}
		result.entries[index] = catalogEntry{expression: entry.Expression, evidence: entry.Evidence}
		result.byExpression[entry.Expression] = index
		parts = append(parts, contentBytes(entry.Expression), contentBytes(entry.Evidence.Digest()))
	}
	digest, ok := identity.DeriveContentID(catalogDigestDomain, parts...)
	if !ok {
		return Catalog{}, false
	}
	result.digest = digest
	return result, result.Available()
}

// EmptyCatalog is the valid closed empty evidence set for certificates with
// no Expand node.
func EmptyCatalog() Catalog {
	result, _ := NewCatalog([]Entry{})
	return result
}

func (catalog Catalog) Available() bool {
	return catalog.entries != nil && catalog.byExpression != nil && catalog.digest.Available() && len(catalog.entries) == len(catalog.byExpression)
}

func (catalog Catalog) Digest() identity.ContentID {
	if !catalog.Available() {
		return identity.ContentID{}
	}
	return catalog.digest
}

// Expressions returns the sealed logical Expand node identities in canonical
// order. Arrangement uses this closed-world view to reject an evidence entry
// that is not consumed by the checked certificate instead of silently
// accepting an overcomplete mount catalog.
func (catalog Catalog) Expressions() []identity.ContentID {
	if !catalog.Available() {
		return nil
	}
	result := make([]identity.ContentID, len(catalog.entries))
	for index, entry := range catalog.entries {
		result[index] = entry.expression
	}
	return result
}

func (catalog Catalog) At(expression identity.ContentID) (Evidence, bool) {
	if !catalog.Available() || !expression.Available() {
		return Evidence{}, false
	}
	index, ok := catalog.byExpression[expression]
	if !ok || index < 0 || index >= len(catalog.entries) {
		return Evidence{}, false
	}
	entry := catalog.entries[index]
	return entry.evidence, entry.evidence.Available()
}

// Vectors returns a defensive view of the candidates and their already-issued
// tokens. Runtime callers can use VectorAt for allocation-free lookup.
func (evidence Evidence) Vectors() []VectorView {
	if !evidence.Available() {
		return nil
	}
	result := make([]VectorView, len(evidence.rows))
	for index, row := range evidence.rows {
		result[index] = VectorView{candidate: row.candidate, keys: append([]binding.ValueToken(nil), row.keys...)}
	}
	return result
}

// VectorAt redeems the owner-issued vector for one candidate identity.
func (evidence Evidence) VectorAt(candidate model.RowID) (VectorView, bool) {
	if !evidence.Available() || !candidate.Available() {
		return VectorView{}, false
	}
	index, ok := evidence.byCandidate[candidate]
	if !ok || index < 0 || index >= len(evidence.rows) {
		return VectorView{}, false
	}
	row := evidence.rows[index]
	return VectorView{candidate: row.candidate, keys: row.keys}, true
}

// CandidatesForKey returns the exact owner-ordered C postings for one
// tokenized R key. An authenticated key with no postings is distinct from an
// unavailable Evidence value.
func (evidence Evidence) CandidatesForKey(key binding.ValueToken) ([]model.RowID, bool) {
	if !evidence.Available() || !key.Available() || !key.ValidFor(evidence.fence) || key.Type() != evidence.keyType {
		return nil, false
	}
	indices, ok := evidence.byKey[evidenceKey{typeID: key.Type(), content: key.Opaque()}]
	if !ok {
		return []model.RowID{}, true
	}
	result := make([]model.RowID, len(indices))
	for position, index := range indices {
		if index < 0 || index >= len(evidence.rows) {
			return nil, false
		}
		result[position] = evidence.rows[index].candidate
	}
	return result, true
}

// VectorView is the runtime read-only projection of one frozen vector. Its
// tokens retain the exact fence and owner TypeID issued during mount.
type VectorView struct {
	candidate model.RowID
	keys      []binding.ValueToken
}

func (vector VectorView) Available() bool {
	if !vector.candidate.Available() || vector.keys == nil {
		return false
	}
	for _, key := range vector.keys {
		if !key.Available() {
			return false
		}
	}
	return true
}

func (vector VectorView) Candidate() model.RowID {
	if !vector.Available() {
		return model.RowID{}
	}
	return vector.candidate
}

func (vector VectorView) KeyCount() int {
	if !vector.Available() {
		return 0
	}
	return len(vector.keys)
}

func (vector VectorView) KeyAt(index int) (binding.ValueToken, bool) {
	if !vector.Available() || index < 0 || index >= len(vector.keys) {
		return binding.ValueToken{}, false
	}
	return vector.keys[index], true
}

func vectorDigest(candidate, publisher model.RowID, keys []binding.ValueToken) []byte {
	parts := make([][]byte, 0, 2+len(keys))
	parts = append(parts, rowBytes(candidate), rowBytes(publisher))
	for _, key := range keys {
		parts = append(parts, nominalBytes(key.Type().Owner().Content(), key.Type().Content()), contentBytes(key.Opaque()))
	}
	value, _ := identity.DeriveContentID(evidenceDigestDomain+"/vector", parts...)
	return contentBytes(value)
}

func rowBytes(value model.RowID) []byte {
	result := make([]byte, 0, 96)
	result = append(result, contentBytes(value.Relation().Owner().Content())...)
	result = append(result, contentBytes(value.Relation().Content())...)
	result = append(result, contentBytes(value.Content())...)
	return result
}

func nominalBytes(owner, content identity.ContentID) []byte {
	result := make([]byte, 0, len(owner)+len(content))
	result = append(result, owner[:]...)
	return append(result, content[:]...)
}

func contentBytes(value identity.ContentID) []byte {
	result := make([]byte, len(value))
	copy(result, value[:])
	return result
}

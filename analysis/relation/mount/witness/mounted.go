package witness

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

const mountedDigestDomain = "analysis/relation/mount/witness/mounted/v1"

// Mounted is the immutable, generation-fenced capability produced by
// Specialize. It owns no mutable runtime state and exposes only defensive
// read-only projections. The address book and arrangement plan remain opaque
// owner values; callers cannot manufacture or replace one coordinate.
type Mounted struct{ data *mountedData }

type mountedData struct {
	fence           address.Fence
	runtime         binding.Fence
	issuer          binding.Issuer
	lineage         lineage.Authority
	lineageOwner    model.OwnerID
	lineageIdentity identity.ContentID
	book            address.Book
	arrangement     arrangement.Plan
	digest          identity.ContentID
	columns         []model.ColumnSchema

	bindings   map[signature.Identity]binding.Binding
	identities []signature.Identity

	types    []model.TypeID
	algebras map[model.TypeID]binding.ValueAlgebra

	denominators []model.DenominatorRef
	witnesses    map[model.DenominatorRef]binding.DenominatorWitness

	scopes     []model.ScopeID
	scopeByID  map[model.ScopeID]binding.ScopeToken
	scopeArena *scopeArena

	wideningPermits []WideningPermit
}

// Available reports whether the mounted artifact is complete.
func (mounted Mounted) Available() bool {
	return mounted.data != nil && mounted.data.fence.Available() && mounted.data.runtime.Available() && mounted.data.issuer.Available() && mounted.data.lineage != nil && mounted.data.lineageOwner.Available() && mounted.data.lineageIdentity.Available() && mounted.data.book.Available() && mounted.data.arrangement.Available() && mounted.data.scopeArena != nil && mounted.data.scopeArena.available() && mounted.data.digest.Available()
}

// Fence returns the exact address/runtime certificate fence captured at
// admission.
func (mounted Mounted) Fence() address.Fence {
	if mounted.data == nil {
		return address.Fence{}
	}
	return mounted.data.fence
}

// RuntimeFence returns the solve-local semantic token fence. It is derived
// once from Fence and is never reissued by a lookup.
func (mounted Mounted) RuntimeFence() binding.Fence {
	if mounted.data == nil {
		return binding.Fence{}
	}
	return mounted.data.runtime
}

// Lineage returns the exact proof-sidecar authority admitted for this
// mounted runtime. The authority is bound to RuntimeFence and exposes only
// validation/join operations; no inventory fallback or second interface is
// introduced at the mount boundary.
func (mounted Mounted) Lineage() (lineage.Authority, bool) {
	if !mounted.Available() {
		return nil, false
	}
	return mounted.data.lineage, true
}

// Book returns the immutable logical-to-address snapshot. Book itself has no
// mutation methods and all enumeration accessors are defensive.
func (mounted Mounted) Book() address.Book {
	if mounted.data == nil {
		return address.Book{}
	}
	return mounted.data.book
}

// Arrangement returns the opaque physical arrangement plan. Its coordinate
// accessors remain private to arrangement; this value is only checked against
// the exact mount fence by downstream state.
func (mounted Mounted) Arrangement() arrangement.Plan {
	if mounted.data == nil {
		return arrangement.Plan{}
	}
	return mounted.data.arrangement
}

// Columns returns the complete canonical catalogue of column declarations
// admitted by this mount. The stored snapshot is returned defensively;
// callers cannot alter the geometry catalogue or the mounted digest.
func (mounted Mounted) Columns() []model.ColumnSchema {
	if !mounted.Available() {
		return nil
	}
	return append([]model.ColumnSchema(nil), mounted.data.columns...)
}

// ColumnIDs returns the complete canonical catalogue of column identities
// admitted by this mount. IDs are derived from the schema catalogue rather
// than stored as a second authority.
func (mounted Mounted) ColumnIDs() []model.ColumnID {
	if !mounted.Available() {
		return nil
	}
	result := make([]model.ColumnID, len(mounted.data.columns))
	for index, column := range mounted.data.columns {
		result[index] = column.ID()
	}
	return append([]model.ColumnID(nil), result...)
}

// Digest returns the deterministic mounted capability identity. It includes
// the certificate, exact fence, address snapshot, complete column catalogue,
// arrangement plan, and every admitted semantic proof projection.
func (mounted Mounted) Digest() identity.ContentID {
	if mounted.data == nil {
		return identity.ContentID{}
	}
	return mounted.data.digest
}

// Same compares complete mounted capabilities by exact fence and digest.
func (mounted Mounted) Same(other Mounted) bool {
	return mounted.Available() && other.Available() && mounted.Fence().Same(other.Fence()) && mounted.Digest() == other.Digest()
}

// SignatureIdentities returns the canonical operation identities admitted by
// the mount.
func (mounted Mounted) SignatureIdentities() []signature.Identity {
	if !mounted.Available() {
		return nil
	}
	return append([]signature.Identity(nil), mounted.data.identities...)
}

// Binding resolves the typed operation adapter admitted against one exact
// signature. The adapter is opaque and cannot be replaced through Mounted.
func (mounted Mounted) Binding(id signature.Identity) (binding.Binding, bool) {
	if !mounted.Available() || !id.Available() {
		return nil, false
	}
	value, ok := mounted.data.bindings[id]
	return value, ok && value != nil
}

// AlgebraRequirements returns all certificate TypeIDs whose value algebra
// was admitted, defensively copied.
func (mounted Mounted) AlgebraRequirements() []model.TypeID {
	if !mounted.Available() {
		return nil
	}
	return append([]model.TypeID(nil), mounted.data.types...)
}

// Algebra resolves the typed value authority for one exact TypeID.
func (mounted Mounted) Algebra(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	if !mounted.Available() || !typeID.Available() {
		return nil, false
	}
	value, ok := mounted.data.algebras[typeID]
	return value, ok && value != nil && value.Type() == typeID
}

// Denominators returns all exact denominator references admitted by the
// certificate, in canonical logical order.
func (mounted Mounted) Denominators() []model.DenominatorRef {
	if !mounted.Available() {
		return nil
	}
	return append([]model.DenominatorRef(nil), mounted.data.denominators...)
}

// Denominator resolves an authenticated denominator witness.
func (mounted Mounted) Denominator(ref model.DenominatorRef) (binding.DenominatorWitness, bool) {
	if !mounted.Available() || !ref.Available() {
		return binding.DenominatorWitness{}, false
	}
	value, ok := mounted.data.witnesses[ref]
	return value, ok && value.ValidFor(mounted.data.runtime) && value.Matches(ref)
}

// IssueCell issues one authenticated cell only when ref is an admitted
// denominator and scope is an authenticated scope owned by this runtime.
// The issuer remains private to Mounted; callers receive no minting authority.
func (mounted Mounted) IssueCell(ref model.DenominatorRef, scope Scope, column model.ColumnID, row model.RowID) (binding.CellToken, bool) {
	if !mounted.Available() || !ref.Available() || !scope.validFor(mounted.data.runtime) {
		return binding.CellToken{}, false
	}
	columnAddress, columnOK := mounted.data.book.Column(column)
	if !columnOK || !columnAddress.ValidFor(mounted.data.fence) {
		return binding.CellToken{}, false
	}
	witness, witnessOK := mounted.Denominator(ref)
	if !witnessOK {
		return binding.CellToken{}, false
	}
	if !mounted.data.scopeArena.contains(scope.token) {
		return binding.CellToken{}, false
	}
	return mounted.data.issuer.IssueCell(witness, scope.token, column, row)
}

// IssueValue issues one authenticated value only for an admitted algebra
// TypeID. Opaque value bytes remain the caller's domain payload.
func (mounted Mounted) IssueValue(typeID model.TypeID, opaque identity.ContentID) (binding.ValueToken, bool) {
	if !mounted.Available() || !typeID.Available() {
		return binding.ValueToken{}, false
	}
	if _, admitted := mounted.data.algebras[typeID]; !admitted {
		return binding.ValueToken{}, false
	}
	return mounted.data.issuer.IssueValue(typeID, opaque)
}

// RowIndex resolves an authenticated logical row to the dense index captured
// by the denominator witness. No hash or physical registry is consulted; the
// index is a private snapshot of inventory's logical row order.
func (mounted Mounted) RowIndex(ref model.DenominatorRef, row model.RowID) (int, bool) {
	if !mounted.Available() || !ref.Available() || !row.Available() || row.Relation() != ref.Relation() {
		return 0, false
	}
	witness, ok := mounted.data.witnesses[ref]
	if !ok || !witness.ValidFor(mounted.data.runtime) || !witness.Matches(ref) {
		return 0, false
	}
	return witness.Index(row)
}

// Scopes returns all exact scope identities admitted by the certificate.
func (mounted Mounted) Scopes() []model.ScopeID {
	if !mounted.Available() {
		return nil
	}
	return append([]model.ScopeID(nil), mounted.data.scopes...)
}

// Scope resolves an authenticated token by nominal scope identity. The
// token-to-Region association remains private inside the mounted arena.
func (mounted Mounted) Scope(id model.ScopeID) (Scope, bool) {
	if !mounted.Available() || !id.Available() {
		return Scope{}, false
	}
	value, ok := mounted.data.scopeByID[id]
	if !ok || !value.ValidFor(mounted.data.runtime) || !mounted.data.scopeArena.contains(value) {
		return Scope{}, false
	}
	return newScope(value)
}

// ScopeToken projects one already-authenticated scope to the narrow token
// needed by state Geometry. It cannot mint a token or accept a raw formula.
func (mounted Mounted) ScopeToken(scope Scope) (binding.ScopeToken, bool) {
	if !mounted.Available() || !scope.validFor(mounted.data.runtime) || !mounted.data.scopeArena.contains(scope.token) {
		return binding.ScopeToken{}, false
	}
	return scope.token, true
}

// RegionForToken resolves the exact neutral formula owned by one
// authenticated token. No declared-scope scan or token digest reconstruction
// is performed.
func (mounted Mounted) RegionForToken(token binding.ScopeToken) (Region, bool) {
	if !mounted.Available() || !token.ValidFor(mounted.data.runtime) {
		return nil, false
	}
	return mounted.data.scopeArena.resolve(token)
}

// RegionForScope resolves the token carried by an authenticated scope through
// the same mounted arena used by CellToken and Geometry.
func (mounted Mounted) RegionForScope(scope Scope) (Region, bool) {
	if !mounted.Available() || !scope.validFor(mounted.data.runtime) {
		return nil, false
	}
	return mounted.RegionForToken(scope.token)
}

// ConjoinScopes canonically combines two authenticated formulas and issues a
// runtime-fenced token for the result. The token-to-Region pair is interned in
// the append-only mounted arena, so repeated and concurrent conjunctions
// recover one canonical entry without a mutable scope replacement.
func (mounted Mounted) ConjoinScopes(left, right Scope) (Scope, bool) {
	if !mounted.Available() || !left.validFor(mounted.data.runtime) || !right.validFor(mounted.data.runtime) {
		return Scope{}, false
	}
	leftRegion, leftOK := mounted.RegionForToken(left.token)
	rightRegion, rightOK := mounted.RegionForToken(right.token)
	if !leftOK || !rightOK {
		return Scope{}, false
	}
	if left.Same(right) {
		return left, true
	}
	first, second := leftRegion, rightRegion
	firstID, firstIDOK := scopeRegionIdentity(first)
	secondID, secondIDOK := scopeRegionIdentity(second)
	if !firstIDOK || !secondIDOK {
		return Scope{}, false
	}
	if bytes.Compare(secondID[:], firstID[:]) < 0 {
		first, second = second, first
	}
	combined, combineOK := first.Conjoin(second)
	if !combineOK || !regionAvailable(combined) {
		return Scope{}, false
	}
	combinedIdentity, identityOK := scopeRegionIdentity(combined)
	if !identityOK {
		return Scope{}, false
	}
	token, tokenOK := mounted.data.issuer.IssueScope(combinedIdentity)
	if !tokenOK {
		return Scope{}, false
	}
	if _, internOK := mounted.data.scopeArena.intern(token, combined); !internOK {
		return Scope{}, false
	}
	return newScope(token)
}

// EntailsScopes evaluates the neutral formula entailment law for two
// authenticated scopes. The first scope is the entailing premise.
func (mounted Mounted) EntailsScopes(left, right Scope) bool {
	if !mounted.Available() || !left.validFor(mounted.data.runtime) || !right.validFor(mounted.data.runtime) {
		return false
	}
	leftRegion, leftOK := mounted.RegionForToken(left.token)
	rightRegion, rightOK := mounted.RegionForToken(right.token)
	return leftOK && rightOK && leftRegion.Entails(rightRegion)
}

// WideningPermits returns the complete defensive set of opaque recurrence
// permits admitted by this mount.
func (mounted Mounted) WideningPermits() []WideningPermit {
	if !mounted.Available() {
		return nil
	}
	return append([]WideningPermit(nil), mounted.data.wideningPermits...)
}

// Widening returns exactly one opaque admitted recurrence permit for the
// requested logical dependency/relation pair.
func (mounted Mounted) Widening(dependency model.DependencyID, relation model.RelationID) (WideningPermit, bool) {
	if !mounted.Available() || !dependency.Available() || !relation.Available() {
		return WideningPermit{}, false
	}
	for _, permit := range mounted.data.wideningPermits {
		if permit.Available() && permit.Dependency() == dependency && permit.Relation() == relation {
			return permit, true
		}
	}
	return WideningPermit{}, false
}

func canonicalizeIdentities(values []signature.Identity) []signature.Identity {
	result := append([]signature.Identity(nil), values...)
	sort.Slice(result, func(left, right int) bool { return signatureIdentityLess(result[left], result[right]) })
	return result
}

func signatureIdentityLess(left, right signature.Identity) bool {
	if compared := compareNominal(left.Operation.Owner().Content(), left.Operation.Content(), right.Operation.Owner().Content(), right.Operation.Content()); compared != 0 {
		return compared < 0
	}
	return left.Version < right.Version
}

func canonicalizeTypes(values []model.TypeID) []model.TypeID {
	result := append([]model.TypeID(nil), values...)
	sort.Slice(result, func(left, right int) bool {
		return compareNominal(result[left].Owner().Content(), result[left].Content(), result[right].Owner().Content(), result[right].Content()) < 0
	})
	return result
}

func canonicalizeDenominators(values []model.DenominatorRef) []model.DenominatorRef {
	result := append([]model.DenominatorRef(nil), values...)
	sort.Slice(result, func(left, right int) bool { return denominatorLess(result[left], result[right]) })
	return result
}

func canonicalizeScopes(values []model.ScopeID) []model.ScopeID {
	result := append([]model.ScopeID(nil), values...)
	sort.Slice(result, func(left, right int) bool {
		return compareNominal(result[left].Owner().Content(), result[left].Content(), result[right].Owner().Content(), result[right].Content()) < 0
	})
	return result
}

func denominatorLess(left, right model.DenominatorRef) bool {
	if compared := compareNominal(left.Relation().Owner().Content(), left.Relation().Content(), right.Relation().Owner().Content(), right.Relation().Content()); compared != 0 {
		return compared < 0
	}
	return compareNominal(left.Key().Relation().Owner().Content(), left.Key().Content(), right.Key().Relation().Owner().Content(), right.Key().Content()) < 0
}

func compareNominal(leftOwner, leftContent, rightOwner, rightContent identity.ContentID) int {
	if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
		return compared
	}
	return bytes.Compare(leftContent[:], rightContent[:])
}

func contentBytes(value identity.ContentID) []byte {
	result := make([]byte, len(value))
	copy(result, value[:])
	return result
}

func appendUint64(parts *[][]byte, value uint64) {
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, value)
	*parts = append(*parts, encoded)
}

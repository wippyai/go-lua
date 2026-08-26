package witness

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
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
	observations    []algebra.ObservationContract

	bindings   map[signature.Identity]binding.Binding
	identities []signature.Identity
	initials   []plan.Initial

	// codecTypes is the complete immutable typed-codec catalogue derived from
	// checked columns and signatures. It authorizes opaque token issuance;
	// types/algebras are the narrower subset that can semantically ascend.
	codecTypes       []model.TypeID
	capabilities     []model.TypeCapability
	capabilityByType map[model.TypeID]model.TypeCapability
	types            []model.TypeID
	algebras         map[model.TypeID]binding.ValueAlgebra
	equalityTypes    []model.TypeID
	equalities       map[model.TypeID]binding.ValueEquality

	denominators []model.DenominatorRef
	witnesses    map[model.DenominatorRef]binding.DenominatorWitness
	// denominatorLineage is the exact closed-world provenance atom for each
	// admitted denominator. It is materialized once so an empty complete span
	// has authenticated evidence without minting a zero/default lineage later.
	denominatorLineage map[model.DenominatorRef]model.LineageRef
	// rows is the sole relation-local row directory for this mounted
	// runtime.  Denominators prove membership; they never assign the row's
	// physical address.  The directory is the union of admitted memberships,
	// sorted by owner-issued RowID identity.
	rows       map[model.RelationID][]model.RowID
	rowLineage map[model.RowID]model.LineageRef

	scopes     []model.ScopeID
	scopeByID  map[model.ScopeID]binding.ScopeToken
	scopeArena *scopeArena

	wideningPermits []WideningPermit
}

// Available reports whether the mounted artifact is complete.
func (mounted Mounted) Available() bool {
	return mounted.data != nil && mounted.data.fence.Available() && mounted.data.runtime.Available() && mounted.data.issuer.Available() && mounted.data.lineage != nil && mounted.data.lineageOwner.Available() && mounted.data.lineageIdentity.Available() && mounted.data.book.Available() && mounted.data.arrangement.Available() && mounted.data.capabilityByType != nil && mounted.data.equalities != nil && mounted.data.rows != nil && mounted.data.rowLineage != nil && mounted.data.denominatorLineage != nil && mounted.data.scopeArena != nil && mounted.data.scopeArena.available() && mounted.data.digest.Available()
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

// Initials returns the exact schema-sealed zero-input invocation catalogue.
// Runtime admission may execute these rows, but cannot add, remove, reorder,
// or replace them through a side channel.
func (mounted Mounted) Initials() []plan.Initial {
	if !mounted.Available() {
		return nil
	}
	return append([]plan.Initial(nil), mounted.data.initials...)
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

// Observation resolves one exact schema-sealed terminal descriptor from the
// mounted catalogue. The runtime accepts only this projection by digest; it
// cannot inject a newly constructed descriptor after certificate admission.
func (mounted Mounted) Observation(id identity.ContentID) (algebra.ObservationContract, bool) {
	if !mounted.Available() || !id.Available() {
		return algebra.ObservationContract{}, false
	}
	for _, value := range mounted.data.observations {
		if value.Digest() == id {
			return value, true
		}
	}
	return algebra.ObservationContract{}, false
}

// Observations returns the defensive mounted descriptor catalogue.
func (mounted Mounted) Observations() []algebra.ObservationContract {
	if !mounted.Available() {
		return nil
	}
	return append([]algebra.ObservationContract(nil), mounted.data.observations...)
}

// AlgebraRequirements returns all certificate TypeIDs whose value algebra
// was admitted, defensively copied.
func (mounted Mounted) AlgebraRequirements() []model.TypeID {
	if !mounted.Available() {
		return nil
	}
	return append([]model.TypeID(nil), mounted.data.types...)
}

// CodecTypes returns every checked TypeID for which this mount can issue an
// authenticated opaque semantic token. Unlike AlgebraRequirements, this set
// includes read-only and AuthenticatedOpaque types.
func (mounted Mounted) CodecTypes() []model.TypeID {
	if !mounted.Available() {
		return nil
	}
	return append([]model.TypeID(nil), mounted.data.codecTypes...)
}

// TypeCapabilities returns the exact schema-sealed policies admitted by this
// runtime. DecodeOnly values remain codec-only; only an explicit Ascending
// policy can accompany a resolved ValueAlgebra.
func (mounted Mounted) TypeCapabilities() []model.TypeCapability {
	if !mounted.Available() {
		return nil
	}
	return append([]model.TypeCapability(nil), mounted.data.capabilities...)
}

// TypeCapability resolves one exact schema-sealed policy without reopening
// the unchecked schema. Physical Merge uses this distinction to select exact
// token equality for DecodeOnly values and algebraic ascent for Ascending
// values.
func (mounted Mounted) TypeCapability(typeID model.TypeID) (model.TypeCapability, bool) {
	if !mounted.Available() || !typeID.Available() {
		return model.TypeCapability{}, false
	}
	value, ok := mounted.data.capabilityByType[typeID]
	return value, ok && value.Available() && value.Type() == typeID
}

// Algebra resolves the typed value authority for one exact TypeID.
func (mounted Mounted) Algebra(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	if !mounted.Available() || !typeID.Available() {
		return nil, false
	}
	value, ok := mounted.data.algebras[typeID]
	return value, ok && value != nil && value.Type() == typeID
}

// Equality resolves the owner semantic equality authority admitted for one
// certified key TypeID. It is absent for DecodeOnly types; their physical
// Merge branch compares exact authenticated tokens instead.
func (mounted Mounted) Equality(typeID model.TypeID) (binding.ValueEquality, bool) {
	if !mounted.Available() || !typeID.Available() {
		return nil, false
	}
	value, ok := mounted.data.equalities[typeID]
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

// DenominatorLineage resolves the exact provenance atom materialized for one
// admitted denominator. This is the closed-world evidence lane used when a
// complete range is empty and therefore has no member RowID from which to
// recover lineage.
func (mounted Mounted) DenominatorLineage(ref model.DenominatorRef) (model.LineageRef, bool) {
	if !mounted.Available() || !ref.Available() {
		return model.LineageRef{}, false
	}
	value, ok := mounted.data.denominatorLineage[ref]
	return value, ok && value.Available()
}

// IssueCell issues one authenticated cell from an already redeemed
// denominator witness and an authenticated scope owned by this runtime. The
// caller must resolve the ordinary global witness once at its own boundary;
// correlated replay passes the q-specific posting directly.
func (mounted Mounted) IssueCell(denominator binding.DenominatorWitness, scope Scope, column model.ColumnID, row model.RowID) (binding.CellToken, bool) {
	if !mounted.Available() || !denominator.ValidFor(mounted.data.runtime) || !scope.validFor(mounted.data.runtime) || !row.Available() || !denominator.Contains(row) {
		return binding.CellToken{}, false
	}
	columnAddress, columnOK := mounted.data.book.Column(column)
	if !columnOK || !columnAddress.ValidFor(mounted.data.fence) {
		return binding.CellToken{}, false
	}
	if !mounted.data.scopeArena.contains(scope.token) {
		return binding.CellToken{}, false
	}
	return mounted.data.issuer.IssueCell(denominator, scope.token, column, row)
}

// IssueValue issues one authenticated codec token for a TypeID in the
// immutable catalogue certified by admitted columns and signatures. Codec
// admission is deliberately independent of ValueAlgebra admission:
// AuthenticatedOpaque cells must be able to carry an exact owner token even
// when no Present value can ascend for that TypeID.
func (mounted Mounted) IssueValue(typeID model.TypeID, opaque identity.ContentID) (binding.ValueToken, bool) {
	if !mounted.Available() || !typeID.Available() {
		return binding.ValueToken{}, false
	}
	certified := false
	for _, candidate := range mounted.data.codecTypes {
		if candidate == typeID {
			certified = true
			break
		}
	}
	if !certified {
		return binding.ValueToken{}, false
	}
	return mounted.data.issuer.IssueValue(typeID, opaque)
}

// RowIndex resolves an authenticated relation-local row directory position.
// The directory is the union of admitted denominator memberships, sorted by
// owner-issued RowID identity once during admission. Denominators prove
// membership; they never assign scalar addresses.
func (mounted Mounted) RowIndex(relation model.RelationID, row model.RowID) (int, bool) {
	if !mounted.Available() || !relation.Available() || !row.Available() || row.Relation() != relation {
		return 0, false
	}
	rows, ok := mounted.data.rows[relation]
	if !ok {
		return 0, false
	}
	index := sort.Search(len(rows), func(index int) bool {
		return !rowLess(rows[index], row)
	})
	if index < len(rows) && rows[index] == row {
		return index, true
	}
	return 0, false
}

// RowAt resolves the inverse of RowIndex from the immutable mounted relation
// directory. The round-trip is relation-local and independent of which
// denominator admitted the row.
func (mounted Mounted) RowAt(relation model.RelationID, index int) (model.RowID, bool) {
	if !mounted.Available() || !relation.Available() || index < 0 {
		return model.RowID{}, false
	}
	rows, ok := mounted.data.rows[relation]
	if !ok || index >= len(rows) {
		return model.RowID{}, false
	}
	return rows[index], true
}

// RowLineage resolves the exact canonical lineage atom materialized for one
// admitted row-directory member.  The projection is sealed during
// specialization, so lookup is a direct map access and never derives a
// replacement identity or consults a downstream source.
func (mounted Mounted) RowLineage(row model.RowID) (model.LineageRef, bool) {
	if !mounted.Available() || !row.Available() {
		return model.LineageRef{}, false
	}
	value, ok := mounted.data.rowLineage[row]
	return value, ok && value.Available()
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

// ScopeForToken restores the narrow mounted Scope facade for one already
// authenticated runtime token.  It is deliberately not a minting operation:
// the token must be fenced to this exact mount and already have an immutable
// Region entry in the mounted arena.  Physical state uses this only before it
// normalizes the scope through its cofiber authority.
func (mounted Mounted) ScopeForToken(token binding.ScopeToken) (Scope, bool) {
	if !mounted.Available() || !token.ValidFor(mounted.data.runtime) || !mounted.data.scopeArena.contains(token) {
		return Scope{}, false
	}
	return newScope(token)
}

// AdmitRuntimeRegion issues one runtime Scope for an immutable region created
// by the physical cofiber boundary.  The mounted arena remains the sole
// token-to-Region registry: this method cannot replace an existing entry;
// repeated identity admission recovers the original arena Region rather than
// accepting a replacement.  It is therefore a trusted runtime capability:
// callers with Mounted may issue a cell from an admitted scope.  Production
// import laws restrict this method to the cofiber boundary; cofiber
// independently refuses regions it did not normalize.
//
// This is intentionally narrower than a general scope factory.  Declared
// scopes still enter only through Specialize; runtime regions are append-only
// consequences of those scopes after exact physical Boolean partitioning.
func (mounted Mounted) AdmitRuntimeRegion(value region.Region) (Scope, bool) {
	if !mounted.Available() || !value.Available() {
		return Scope{}, false
	}
	regionID, identityOK := scopeRegionIdentity(value)
	if !identityOK {
		return Scope{}, false
	}
	token, tokenOK := mounted.data.issuer.IssueScope(regionID)
	if !tokenOK || !token.ValidFor(mounted.data.runtime) {
		return Scope{}, false
	}
	if _, internOK := mounted.data.scopeArena.intern(token, value); !internOK {
		return Scope{}, false
	}
	return newScope(token)
}

// RegionForToken resolves the exact neutral formula owned by one
// authenticated token. No declared-scope scan or token digest reconstruction
// is performed.
func (mounted Mounted) RegionForToken(token binding.ScopeToken) (region.Region, bool) {
	if !mounted.Available() || !token.ValidFor(mounted.data.runtime) {
		return region.Region{}, false
	}
	return mounted.data.scopeArena.resolve(token)
}

// RegionForScope resolves the token carried by an authenticated scope through
// the same mounted arena used by CellToken and Geometry.
func (mounted Mounted) RegionForScope(scope Scope) (region.Region, bool) {
	if !mounted.Available() || !scope.validFor(mounted.data.runtime) {
		return region.Region{}, false
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
	combined, combineOK := region.Conjoin(leftRegion, rightRegion)
	if !combineOK || !combined.Available() {
		return Scope{}, false
	}
	return mounted.AdmitRuntimeRegion(combined)
}

// EntailsScopes evaluates the neutral formula entailment law for two
// authenticated scopes. The first scope is the entailing premise.
func (mounted Mounted) EntailsScopes(left, right Scope) bool {
	if !mounted.Available() || !left.validFor(mounted.data.runtime) || !right.validFor(mounted.data.runtime) {
		return false
	}
	leftRegion, leftOK := mounted.RegionForToken(left.token)
	rightRegion, rightOK := mounted.RegionForToken(right.token)
	return leftOK && rightOK && region.Entails(leftRegion, rightRegion)
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

func canonicalizeInitials(values []plan.Initial) []plan.Initial {
	result := append([]plan.Initial(nil), values...)
	sort.Slice(result, func(left, right int) bool { return initialLess(result[left], result[right]) })
	return result
}

func initialLess(left, right plan.Initial) bool {
	leftOperation, rightOperation := left.Operation(), right.Operation()
	if compared := compareNominal(leftOperation.Operation.Owner().Content(), leftOperation.Operation.Content(), rightOperation.Operation.Owner().Content(), rightOperation.Operation.Content()); compared != 0 {
		return compared < 0
	}
	if leftOperation.Version != rightOperation.Version {
		return leftOperation.Version < rightOperation.Version
	}
	return compareNominal(left.Scope().Owner().Content(), left.Scope().Content(), right.Scope().Owner().Content(), right.Scope().Content()) < 0
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

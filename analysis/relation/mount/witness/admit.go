package witness

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func regionAvailable(value Region) bool {
	if value == nil {
		return false
	}
	_, ok := value.Identity()
	return ok
}

// newMounted snapshots every admitted proof projection into private maps and
// slices. The caller has already validated all cross-references and fences;
// this function only performs immutable copying and computes the identity.
func newMounted(
	certificateValue certificate.Certificate,
	book address.Book,
	arrangementPlan arrangement.Plan,
	runtime binding.Fence,
	issuer binding.Issuer,
	lineageAuthority lineage.Authority,
	lineageOwner model.OwnerID,
	lineageIdentity identity.ContentID,
	signatures []signature.Signature,
	bindings map[signature.Identity]binding.Binding,
	algebras map[model.TypeID]binding.ValueAlgebra,
	denominatorRefs []model.DenominatorRef,
	witnesses map[model.DenominatorRef]binding.DenominatorWitness,
	scopeIDs []model.ScopeID,
	scopeTokens map[model.ScopeID]binding.ScopeToken,
	scopeArena *scopeArena,
	wideningPermits []WideningPermit,
) (Mounted, bool) {
	if !certificateValue.Available() || !book.Available() || !arrangementPlan.Available() || !runtime.Available() || !issuer.Available() || lineageAuthority == nil || !lineageOwner.Available() || !lineageIdentity.Available() || scopeArena == nil || !scopeArena.available() || book.Fence() != arrangementPlan.Fence() || book.Fence().SchemaID() != certificateValue.SchemaID() || book.Fence().CertificateDigest() != certificateValue.Digest() {
		return Mounted{}, false
	}
	if len(signatures) != len(bindings) {
		return Mounted{}, false
	}
	columns, columnsOK := mountedColumns(certificateValue, book)
	if !columnsOK {
		return Mounted{}, false
	}
	identities := make([]signature.Identity, len(signatures))
	for index, value := range signatures {
		if !value.Available() {
			return Mounted{}, false
		}
		identities[index] = value.Identity()
		if !identities[index].Available() || bindings[identities[index]] == nil {
			return Mounted{}, false
		}
		if bound := bindings[identities[index]].Signature(); !bound.Available() || bound.Digest() != value.Digest() {
			return Mounted{}, false
		}
	}
	identities = canonicalizeIdentities(identities)
	types := canonicalizeTypes(certificateValue.AlgebraRequirements())
	if len(types) != len(algebras) {
		return Mounted{}, false
	}
	for _, typeID := range types {
		value := algebras[typeID]
		if value == nil || value.Type() != typeID {
			return Mounted{}, false
		}
	}

	denominatorRefs = canonicalizeDenominators(denominatorRefs)
	if len(denominatorRefs) != len(witnesses) {
		return Mounted{}, false
	}
	for _, ref := range denominatorRefs {
		witness, witnessOK := witnesses[ref]
		if !witnessOK || !witness.Available() || !witness.ValidFor(runtime) || !witness.Matches(ref) {
			return Mounted{}, false
		}
		if _, evidenceOK := witness.Evidence(); !evidenceOK {
			return Mounted{}, false
		}
	}
	rows, rowsOK := mountedRows(denominatorRefs, witnesses, runtime)
	if !rowsOK {
		return Mounted{}, false
	}

	scopeIDs = canonicalizeScopes(scopeIDs)
	if len(scopeIDs) != len(scopeTokens) {
		return Mounted{}, false
	}
	for _, scopeID := range scopeIDs {
		token, scopeOK := scopeTokens[scopeID]
		if !scopeOK || !token.ValidFor(runtime) || !scopeArena.contains(token) {
			return Mounted{}, false
		}
	}

	permits := append([]WideningPermit(nil), wideningPermits...)
	for _, permit := range permits {
		if !permit.Available() {
			return Mounted{}, false
		}
	}

	data := &mountedData{
		fence:           book.Fence(),
		runtime:         runtime,
		issuer:          issuer,
		lineage:         lineageAuthority,
		lineageOwner:    lineageOwner,
		lineageIdentity: lineageIdentity,
		book:            book,
		arrangement:     arrangementPlan,
		columns:         columns,
		bindings:        cloneBindings(bindings),
		identities:      identities,
		types:           types,
		algebras:        cloneAlgebras(algebras),
		denominators:    denominatorRefs,
		witnesses:       cloneWitnesses(witnesses),
		rows:            rows,
		scopes:          scopeIDs,
		scopeByID:       cloneScopeTokens(scopeTokens),
		scopeArena:      scopeArena,
		wideningPermits: permits,
	}
	digest, ok := digestMounted(*data, certificateValue.Digest())
	if !ok {
		return Mounted{}, false
	}
	data.digest = digest
	return Mounted{data: data}, true
}

// mountedRows builds the one relation-local row directory from the union of
// all admitted denominator memberships. A denominator's local order is never
// reused as a logical or physical relation address; only owner-issued RowID
// identity determines the canonical order here.
func mountedRows(refs []model.DenominatorRef, witnesses map[model.DenominatorRef]binding.DenominatorWitness, runtime binding.Fence) (map[model.RelationID][]model.RowID, bool) {
	result := make(map[model.RelationID][]model.RowID)
	seen := make(map[model.RelationID]map[model.RowID]struct{})
	for _, ref := range refs {
		if !ref.Available() {
			return nil, false
		}
		value, ok := witnesses[ref]
		if !ok || !value.ValidFor(runtime) || !value.Matches(ref) {
			return nil, false
		}
		rows := result[ref.Relation()]
		members := seen[ref.Relation()]
		if members == nil {
			members = make(map[model.RowID]struct{})
			seen[ref.Relation()] = members
		}
		for index := 0; index < value.Len(); index++ {
			row, rowOK := value.At(index)
			if !rowOK || !row.Available() || row.Relation() != ref.Relation() {
				return nil, false
			}
			if _, duplicate := members[row]; duplicate {
				continue
			}
			members[row] = struct{}{}
			rows = append(rows, row)
		}
		result[ref.Relation()] = rows
	}
	for relation, rows := range result {
		sort.Slice(rows, func(left, right int) bool { return rowLess(rows[left], rows[right]) })
		for index := 1; index < len(rows); index++ {
			if rows[index-1] == rows[index] || rows[index].Relation() != relation {
				return nil, false
			}
		}
		result[relation] = rows
	}
	return result, validRowDirectory(result)
}

func validRowDirectory(values map[model.RelationID][]model.RowID) bool {
	if values == nil {
		return false
	}
	for relation, rows := range values {
		if !relation.Available() {
			return false
		}
		for index, row := range rows {
			if !row.Available() || row.Relation() != relation || (index > 0 && !rowLess(rows[index-1], row)) {
				return false
			}
		}
	}
	return true
}

func rowLess(left, right model.RowID) bool {
	leftOwner, rightOwner := left.Owner().Content(), right.Owner().Content()
	if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
		return compared < 0
	}
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:]) < 0
}

// mountedColumns freezes the certificate's canonical column catalogue and
// checks every declaration against the exact owner Book address/order once
// during admission. Mounted accessors only return this immutable snapshot.
func mountedColumns(certificateValue certificate.Certificate, book address.Book) ([]model.ColumnSchema, bool) {
	if !certificateValue.Available() || !book.Available() {
		return nil, false
	}
	certificateColumns := certificateValue.Columns()
	certificateRelations := certificateValue.Relations()
	relations := make(map[model.RelationID]model.RelationSchema, len(certificateRelations))
	for _, relation := range certificateRelations {
		id := relation.ID()
		if !relation.Available() || !id.Available() {
			return nil, false
		}
		if _, duplicate := relations[id]; duplicate {
			return nil, false
		}
		relations[id] = relation
	}
	bookColumns := book.ColumnIDs()
	if len(bookColumns) != len(certificateColumns) {
		return nil, false
	}
	result := make([]model.ColumnSchema, len(certificateColumns))
	seen := make(map[model.ColumnID]struct{}, len(certificateColumns))
	for index, column := range certificateColumns {
		id := column.ID()
		if !column.Available() || !id.Available() {
			return nil, false
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, false
		}
		seen[id] = struct{}{}
		relation, relationOK := relations[column.Relation()]
		if !relationOK || !relation.HasColumn(id) {
			return nil, false
		}
		if bookColumns[index] != id {
			return nil, false
		}
		columnAddress, addressOK := book.Column(id)
		if !addressOK || !columnAddress.ValidFor(book.Fence()) {
			return nil, false
		}
		result[index] = column
	}
	for _, id := range bookColumns {
		if !id.Available() {
			return nil, false
		}
		if _, exists := seen[id]; !exists {
			return nil, false
		}
	}
	if len(bookColumns) != len(seen) {
		return nil, false
	}
	return result, true
}

func cloneBindings(values map[signature.Identity]binding.Binding) map[signature.Identity]binding.Binding {
	result := make(map[signature.Identity]binding.Binding, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneAlgebras(values map[model.TypeID]binding.ValueAlgebra) map[model.TypeID]binding.ValueAlgebra {
	result := make(map[model.TypeID]binding.ValueAlgebra, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneWitnesses(values map[model.DenominatorRef]binding.DenominatorWitness) map[model.DenominatorRef]binding.DenominatorWitness {
	result := make(map[model.DenominatorRef]binding.DenominatorWitness, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneScopeTokens(values map[model.ScopeID]binding.ScopeToken) map[model.ScopeID]binding.ScopeToken {
	result := make(map[model.ScopeID]binding.ScopeToken, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func digestMounted(value mountedData, certificateDigest identity.ContentID) (identity.ContentID, bool) {
	parts := make([][]byte, 0, 14+len(value.columns)+len(value.identities)+len(value.types)+len(value.denominators)+len(value.scopes)+len(value.wideningPermits))
	parts = append(parts, contentBytes(certificateDigest), contentBytes(value.book.Digest()), contentBytes(value.arrangement.Digest()))
	fence := value.fence
	parts = append(parts, contentBytes(fence.SchemaID().Owner().Content()), contentBytes(fence.SchemaID().Content()), contentBytes(fence.CertificateDigest()), contentBytes(identity.ContentID(fence.MountID())))
	appendUint64(&parts, uint64(fence.StoreID()))
	appendUint64(&parts, uint64(fence.Generation()))
	if value.lineage == nil || !value.lineageOwner.Available() || !value.lineageIdentity.Available() || value.scopeArena == nil || !value.scopeArena.available() {
		return identity.ContentID{}, false
	}
	parts = append(parts, contentBytes(value.lineageOwner.Content()), contentBytes(value.lineageIdentity))
	for _, column := range value.columns {
		if !column.Available() {
			return identity.ContentID{}, false
		}
		columnID := column.ID()
		relation := column.Relation()
		typeID := column.Type()
		if !columnID.Available() || !relation.Available() || !typeID.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(relation.Owner().Content()), contentBytes(relation.Content()), contentBytes(columnID.Owner().Content()), contentBytes(columnID.Content()), contentBytes(typeID.Owner().Content()), contentBytes(typeID.Content()))
	}
	for _, id := range value.identities {
		parts = append(parts, contentBytes(id.Operation.Owner().Content()), contentBytes(id.Operation.Content()))
		appendUint64(&parts, id.Version)
		bound := value.bindings[id]
		parts = append(parts, contentBytes(bound.Signature().Digest()))
	}
	for _, typeID := range value.types {
		parts = append(parts, contentBytes(typeID.Owner().Content()), contentBytes(typeID.Content()))
	}
	for _, ref := range value.denominators {
		witness, witnessOK := value.witnesses[ref]
		if !witnessOK {
			return identity.ContentID{}, false
		}
		evidenceID, evidenceOK := witness.Evidence()
		if !evidenceOK {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(ref.Relation().Owner().Content()), contentBytes(ref.Relation().Content()), contentBytes(ref.Key().Relation().Owner().Content()), contentBytes(ref.Key().Content()), contentBytes(evidenceID))
	}
	for _, relation := range canonicalRowRelations(value.rows) {
		parts = append(parts, contentBytes(relation.Owner().Content()), contentBytes(relation.Content()))
		for _, row := range value.rows[relation] {
			parts = append(parts, contentBytes(row.Owner().Content()), contentBytes(row.Content()))
		}
	}
	for _, scopeID := range value.scopes {
		token, tokenOK := value.scopeByID[scopeID]
		if !tokenOK {
			return identity.ContentID{}, false
		}
		regionIdentity, regionOK := value.scopeArena.identity(token)
		if !regionOK {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(scopeID.Owner().Content()), contentBytes(scopeID.Content()), contentBytes(regionIdentity))
	}
	for _, permit := range value.wideningPermits {
		if !permit.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(permit.Dependency().Owner().Content()), contentBytes(permit.Dependency().Content()), contentBytes(permit.Relation().Owner().Content()), contentBytes(permit.Relation().Content()), contentBytes(permit.Evidence()))
	}
	return identity.DeriveContentID(mountedDigestDomain, parts...)
}

func canonicalRowRelations(values map[model.RelationID][]model.RowID) []model.RelationID {
	result := make([]model.RelationID, 0, len(values))
	for relation := range values {
		result = append(result, relation)
	}
	sort.Slice(result, func(left, right int) bool {
		return compareNominal(result[left].Owner().Content(), result[left].Content(), result[right].Owner().Content(), result[right].Content()) < 0
	})
	return result
}

package witness

import (
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
	scopeValues map[model.ScopeID]Scope,
	wideningPermits []WideningPermit,
) (Mounted, bool) {
	if !certificateValue.Available() || !book.Available() || !arrangementPlan.Available() || !runtime.Available() || !issuer.Available() || lineageAuthority == nil || !lineageOwner.Available() || !lineageIdentity.Available() || book.Fence() != arrangementPlan.Fence() || book.Fence().SchemaID() != certificateValue.SchemaID() || book.Fence().CertificateDigest() != certificateValue.Digest() {
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

	scopeIDs = canonicalizeScopes(scopeIDs)
	if len(scopeIDs) != len(scopeValues) {
		return Mounted{}, false
	}
	for _, scopeID := range scopeIDs {
		scope, scopeOK := scopeValues[scopeID]
		if !scopeOK || !scope.validFor(runtime) {
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
		scopes:          scopeIDs,
		scopeByID:       cloneScopes(scopeValues),
		wideningPermits: permits,
	}
	digest, ok := digestMounted(*data, certificateValue.Digest())
	if !ok {
		return Mounted{}, false
	}
	data.digest = digest
	return Mounted{data: data}, true
}

// mountedColumns takes the complete logical catalogue from the owner Book
// and checks it against the certificate's column declarations once during
// admission. Mounted accessors only return this immutable snapshot.
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
	byID := make(map[model.ColumnID]model.ColumnSchema, len(certificateColumns))
	for _, column := range certificateColumns {
		id := column.ID()
		if !column.Available() || !id.Available() {
			return nil, false
		}
		if _, duplicate := byID[id]; duplicate {
			return nil, false
		}
		relation, relationOK := relations[column.Relation()]
		if !relationOK || !relation.HasColumn(id) {
			return nil, false
		}
		byID[id] = column
	}
	fromBook := book.ColumnIDs()
	result := make([]model.ColumnSchema, len(fromBook))
	bookColumns := make(map[model.ColumnID]struct{}, len(fromBook))
	for index, id := range fromBook {
		if !id.Available() {
			return nil, false
		}
		if _, duplicate := bookColumns[id]; duplicate {
			return nil, false
		}
		bookColumns[id] = struct{}{}
		if _, ok := book.Column(id); !ok {
			return nil, false
		}
		column, columnOK := byID[id]
		if !columnOK {
			return nil, false
		}
		result[index] = column
	}
	if len(fromBook) != len(byID) {
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

func cloneScopes(values map[model.ScopeID]Scope) map[model.ScopeID]Scope {
	result := make(map[model.ScopeID]Scope, len(values))
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
	if value.lineage == nil || !value.lineageOwner.Available() || !value.lineageIdentity.Available() {
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
	for _, scopeID := range value.scopes {
		scope, scopeOK := value.scopeByID[scopeID]
		if !scopeOK {
			return identity.ContentID{}, false
		}
		regionIdentity, _ := scope.identity()
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

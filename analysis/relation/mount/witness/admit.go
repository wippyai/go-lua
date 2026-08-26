package witness

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

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
	equalities map[model.TypeID]binding.ValueEquality,
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
	observations := certificateValue.Observations()
	for index, observation := range observations {
		if !observation.Available() || !observation.Digest().Available() {
			return Mounted{}, false
		}
		// Observation descriptors redeem only the dependency and sealed Apply
		// node admitted by the certificate.  Require the dependency's mounted
		// address here so a descriptor cannot be carried into a mount whose
		// execution catalogue does not contain its issuing declaration.
		if dependencyAddress, dependencyOK := book.Dependency(observation.Dependency()); !dependencyOK || !dependencyAddress.ValidFor(book.Fence()) {
			return Mounted{}, false
		}
		for _, prior := range observations[:index] {
			if prior.Digest() == observation.Digest() {
				return Mounted{}, false
			}
		}
	}
	codecTypes, codecTypesOK := certifiedCodecTypes(columns, signatures)
	if !codecTypesOK {
		return Mounted{}, false
	}
	capabilities, capabilityByType, capabilitiesOK := certifiedTypeCapabilities(certificateValue.TypeCapabilities())
	if !capabilitiesOK {
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
	initials := canonicalizeInitials(certificateValue.Initials())
	for _, initial := range initials {
		if !initial.Available() || !initial.Operation().Available() || !initial.Scope().Available() {
			return Mounted{}, false
		}
		if _, operationOK := bindings[initial.Operation()]; !operationOK {
			return Mounted{}, false
		}
		if _, scopeOK := scopeTokens[initial.Scope()]; !scopeOK {
			return Mounted{}, false
		}
	}
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
	equalityTypes := canonicalizeTypes(certificateValue.EqualityRequirements())
	if len(equalityTypes) != len(equalities) {
		return Mounted{}, false
	}
	for _, typeID := range equalityTypes {
		value := equalities[typeID]
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
	denominatorLineage, denominatorLineageOK := mountedDenominatorLineages(denominatorRefs, witnesses, lineageAuthority, runtime)
	if !denominatorLineageOK {
		return Mounted{}, false
	}
	rows, rowsOK := mountedRows(denominatorRefs, witnesses, runtime)
	if !rowsOK {
		return Mounted{}, false
	}
	rowLineage, rowLineageOK := mountedRowLineages(rows, lineageAuthority)
	if !rowLineageOK {
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
		fence:              book.Fence(),
		runtime:            runtime,
		issuer:             issuer,
		lineage:            lineageAuthority,
		lineageOwner:       lineageOwner,
		lineageIdentity:    lineageIdentity,
		book:               book,
		arrangement:        arrangementPlan,
		columns:            columns,
		observations:       append([]algebra.ObservationContract(nil), observations...),
		bindings:           cloneBindings(bindings),
		identities:         identities,
		initials:           initials,
		codecTypes:         codecTypes,
		capabilities:       capabilities,
		capabilityByType:   capabilityByType,
		types:              types,
		algebras:           cloneAlgebras(algebras),
		equalityTypes:      equalityTypes,
		equalities:         cloneEqualities(equalities),
		denominators:       denominatorRefs,
		witnesses:          cloneWitnesses(witnesses),
		denominatorLineage: denominatorLineage,
		rows:               rows,
		rowLineage:         rowLineage,
		scopes:             scopeIDs,
		scopeByID:          cloneScopeTokens(scopeTokens),
		scopeArena:         scopeArena,
		wideningPermits:    permits,
	}
	digest, ok := digestMounted(*data, certificateValue.Digest())
	if !ok {
		return Mounted{}, false
	}
	data.digest = digest
	return Mounted{data: data}, true
}

// certifiedCodecTypes is the complete immutable TypeID vocabulary admitted
// by the checked schema. A value token is an owner codec product, not proof of
// an order: inputs and opaque-only outputs must therefore remain issuable even
// when their TypeID is absent from the narrower ascent-algebra catalogue.
func certifiedCodecTypes(columns []model.ColumnSchema, signatures []signature.Signature) ([]model.TypeID, bool) {
	seen := make(map[model.TypeID]struct{})
	add := func(typeID model.TypeID) bool {
		if !typeID.Available() {
			return false
		}
		seen[typeID] = struct{}{}
		return true
	}
	for _, column := range columns {
		if !column.Available() || !add(column.Type()) {
			return nil, false
		}
	}
	for _, operation := range signatures {
		if !operation.Available() {
			return nil, false
		}
		for _, input := range operation.Inputs() {
			if !input.Available() || !add(input.Type) {
				return nil, false
			}
		}
		for _, output := range operation.Outputs() {
			if !output.Available() || !add(output.Type) {
				return nil, false
			}
		}
	}
	result := make([]model.TypeID, 0, len(seen))
	for typeID := range seen {
		result = append(result, typeID)
	}
	return canonicalizeTypes(result), true
}

// certifiedTypeCapabilities snapshots the certificate's owner-declared
// policy catalogue. A missing entry remains intentionally absent: the
// checker, rather than mount, decides which actual semantic use requires one.
func certifiedTypeCapabilities(values []model.TypeCapability) ([]model.TypeCapability, map[model.TypeID]model.TypeCapability, bool) {
	byType := make(map[model.TypeID]model.TypeCapability, len(values))
	for _, value := range values {
		if !value.Available() || !value.Type().Available() {
			return nil, nil, false
		}
		if _, duplicate := byType[value.Type()]; duplicate {
			return nil, nil, false
		}
		byType[value.Type()] = value
	}
	return canonicalizeCapabilities(values), byType, true
}

// mountedDenominatorLineages materializes one closed-world provenance atom
// per admitted denominator. Denominator evidence has only a content
// identity at the binding boundary; the denominator's relation owner is the
// canonical owner namespace available for that atom. The mounted lineage
// authority validates the atom exactly once during specialization.
func mountedDenominatorLineages(refs []model.DenominatorRef, witnesses map[model.DenominatorRef]binding.DenominatorWitness, authority lineage.Authority, runtime binding.Fence) (map[model.DenominatorRef]model.LineageRef, bool) {
	if authority == nil || !runtime.Available() || (witnesses == nil && len(refs) != 0) || len(witnesses) != len(refs) {
		return nil, false
	}
	result := make(map[model.DenominatorRef]model.LineageRef, len(refs))
	for _, ref := range refs {
		if !ref.Available() {
			return nil, false
		}
		witness, witnessOK := witnesses[ref]
		if !witnessOK || !witness.ValidFor(runtime) || !witness.Matches(ref) {
			return nil, false
		}
		evidence, evidenceOK := witness.Evidence()
		if !evidenceOK {
			return nil, false
		}
		atom, atomOK := model.IssueLineageRef(ref.Relation().Owner(), evidence)
		if !atomOK || !authority.Validate(atom) {
			return nil, false
		}
		if prior, duplicate := result[ref]; duplicate && prior != atom {
			return nil, false
		}
		result[ref] = atom
	}
	return result, len(result) == len(refs)
}

// mountedRowLineages materializes the one canonical lineage atom for every
// admitted row-directory member.  A row's owner/content pair is already its
// issued logical identity; mount preserves that atom instead of deriving a
// second content token.  The mounted authority authenticates each atom once
// at specialization, after which RowLineage is an O(1) immutable lookup.
func mountedRowLineages(rows map[model.RelationID][]model.RowID, authority lineage.Authority) (map[model.RowID]model.LineageRef, bool) {
	if rows == nil || authority == nil {
		return nil, false
	}
	result := make(map[model.RowID]model.LineageRef)
	for _, relation := range canonicalRowRelations(rows) {
		for _, row := range rows[relation] {
			if !row.Available() || row.Relation() != relation {
				return nil, false
			}
			atom, atomOK := model.IssueLineageRef(row.Owner(), row.Content())
			if !atomOK || !authority.Validate(atom) {
				return nil, false
			}
			if prior, duplicate := result[row]; duplicate && prior != atom {
				return nil, false
			} else if duplicate {
				continue
			}
			result[row] = atom
		}
	}
	return result, true
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

func cloneEqualities(values map[model.TypeID]binding.ValueEquality) map[model.TypeID]binding.ValueEquality {
	result := make(map[model.TypeID]binding.ValueEquality, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func canonicalizeCapabilities(values []model.TypeCapability) []model.TypeCapability {
	result := append([]model.TypeCapability(nil), values...)
	sort.Slice(result, func(left, right int) bool {
		leftType, rightType := result[left].Type(), result[right].Type()
		if compared := compareNominal(leftType.Owner().Content(), leftType.Content(), rightType.Owner().Content(), rightType.Content()); compared != 0 {
			return compared < 0
		}
		return result[left].Kind() < result[right].Kind()
	})
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
	parts := make([][]byte, 0, 20+len(value.columns)+len(value.observations)+len(value.identities)+len(value.codecTypes)+len(value.capabilities)+len(value.types)+len(value.equalityTypes)+len(value.denominators)+len(value.scopes)+len(value.wideningPermits))
	parts = append(parts, contentBytes(certificateDigest), contentBytes(value.book.Digest()), contentBytes(value.arrangement.Digest()))
	fence := value.fence
	parts = append(parts, contentBytes(fence.SchemaID().Owner().Content()), contentBytes(fence.SchemaID().Content()), contentBytes(fence.CertificateDigest()), contentBytes(identity.ContentID(fence.MountID())))
	appendUint64(&parts, uint64(fence.StoreID()))
	appendUint64(&parts, uint64(fence.Generation()))
	if value.lineage == nil || !value.lineageOwner.Available() || !value.lineageIdentity.Available() || value.rowLineage == nil || value.denominatorLineage == nil || value.scopeArena == nil || !value.scopeArena.available() {
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
	appendUint64(&parts, uint64(len(value.observations)))
	for _, observation := range value.observations {
		if !observation.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(observation.Digest()))
	}
	for _, id := range value.identities {
		parts = append(parts, contentBytes(id.Operation.Owner().Content()), contentBytes(id.Operation.Content()))
		appendUint64(&parts, id.Version)
		bound := value.bindings[id]
		parts = append(parts, contentBytes(bound.Signature().Digest()))
	}
	appendUint64(&parts, uint64(len(value.initials)))
	for _, initial := range value.initials {
		if !initial.Available() {
			return identity.ContentID{}, false
		}
		operation := initial.Operation()
		scope := initial.Scope()
		parts = append(parts, contentBytes(operation.Operation.Owner().Content()), contentBytes(operation.Operation.Content()))
		appendUint64(&parts, operation.Version)
		parts = append(parts, contentBytes(scope.Owner().Content()), contentBytes(scope.Content()), contentBytes(initial.Digest()))
	}
	appendUint64(&parts, uint64(len(value.codecTypes)))
	for _, typeID := range value.codecTypes {
		if !typeID.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(typeID.Owner().Content()), contentBytes(typeID.Content()))
	}
	appendUint64(&parts, uint64(len(value.capabilities)))
	for _, capability := range value.capabilities {
		if !capability.Available() || !capability.Type().Available() {
			return identity.ContentID{}, false
		}
		typeID := capability.Type()
		parts = append(parts, contentBytes(typeID.Owner().Content()), contentBytes(typeID.Content()), []byte{byte(capability.Kind())})
	}
	appendUint64(&parts, uint64(len(value.types)))
	for _, typeID := range value.types {
		if !typeID.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(typeID.Owner().Content()), contentBytes(typeID.Content()))
	}
	appendUint64(&parts, uint64(len(value.equalityTypes)))
	for _, typeID := range value.equalityTypes {
		equality, equalityOK := value.equalities[typeID]
		if !typeID.Available() || !equalityOK || equality == nil || equality.Type() != typeID {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(typeID.Owner().Content()), contentBytes(typeID.Content()))
	}
	for _, ref := range value.denominators {
		witness, witnessOK := value.witnesses[ref]
		if !witnessOK {
			return identity.ContentID{}, false
		}
		evidenceID, evidenceOK := witness.Evidence()
		denominatorLineage, lineageOK := value.denominatorLineage[ref]
		if !evidenceOK || !lineageOK || !denominatorLineage.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(ref.Relation().Owner().Content()), contentBytes(ref.Relation().Content()), contentBytes(ref.Key().Relation().Owner().Content()), contentBytes(ref.Key().Content()), contentBytes(evidenceID), contentBytes(denominatorLineage.Owner().Content()), contentBytes(denominatorLineage.Content()))
	}
	for _, relation := range canonicalRowRelations(value.rows) {
		parts = append(parts, contentBytes(relation.Owner().Content()), contentBytes(relation.Content()))
		for _, row := range value.rows[relation] {
			rowLineage, lineageOK := value.rowLineage[row]
			if !row.Available() || !lineageOK || !rowLineage.Available() {
				return identity.ContentID{}, false
			}
			parts = append(parts, contentBytes(row.Owner().Content()), contentBytes(row.Content()), contentBytes(rowLineage.Owner().Content()), contentBytes(rowLineage.Content()))
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

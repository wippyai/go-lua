package witness

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness/expandcatalog"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Specialize admits one complete checked certificate into one exact mounted
// inventory. Every certificate-facing obligation is consumed at this single
// boundary: addresses and arrangements, one exact typed operation factory,
// one registry for optional algebra/equality authorities, one exact lineage
// authority, all certified semantic requirements, denominator row/evidence
// witnesses, scope formulas, and the validated recurrence-head projection.
//
// The returned Mounted owns only immutable snapshots. A false result always
// returns the zero capability.
func Specialize(cert certificate.Certificate, inventory Inventory, factory binding.Factory, algebraRegistry binding.AlgebraRegistry, lineageFactory lineage.Factory) (Mounted, bool) {
	if inventory == nil || !cert.Available() {
		return Mounted{}, false
	}
	book, ok := address.Bind(cert, inventory)
	if !ok || !book.Available() {
		return Mounted{}, false
	}
	fence := book.Fence()
	runtime, ok := binding.NewFence(fence.SchemaID(), fence.MountID(), fence.Generation())
	if !ok {
		return Mounted{}, false
	}
	issuer, ok := binding.NewIssuer(runtime)
	if !ok {
		return Mounted{}, false
	}
	expandCatalog, catalogOK := expandcatalog.Freeze(cert, inventory.ResolveExpand, issuer, runtime)
	if !catalogOK {
		return Mounted{}, false
	}

	lineageAuthority, lineageOwner, lineageIdentity, lineageOK := bindLineage(lineageFactory, runtime)
	if !lineageOK {
		return Mounted{}, false
	}

	signatures := cert.Signatures()
	bindings := make(map[signature.Identity]binding.Binding, len(signatures))
	for _, operation := range signatures {
		if !operation.Available() || !operation.Identity().Available() {
			return Mounted{}, false
		}
		bound, bindOK := binding.Admit(factory, operation)
		if !bindOK {
			return Mounted{}, false
		}
		if _, duplicate := bindings[operation.Identity()]; duplicate {
			return Mounted{}, false
		}
		bindings[operation.Identity()] = bound
	}

	algebras := make(map[model.TypeID]binding.ValueAlgebra, len(cert.AlgebraRequirements()))
	types := cert.AlgebraRequirements()
	for _, typeID := range types {
		if !typeID.Available() {
			return Mounted{}, false
		}
		algebra, algebraOK := binding.ResolveAlgebra(algebraRegistry, typeID)
		if !algebraOK || algebra == nil || algebra.Type() != typeID {
			return Mounted{}, false
		}
		if _, duplicate := algebras[typeID]; duplicate {
			return Mounted{}, false
		}
		algebras[typeID] = algebra
	}

	equalities, equalitiesOK := bindEqualities(cert, algebras, algebraRegistry)
	if !equalitiesOK {
		return Mounted{}, false
	}

	denominatorRefs, denominatorsOK := certificateDenominators(signatures, cert.ObservationDenominators(), cert.CorrelationDenominators(), cert.CorrelationPartitions(), cert.CompleteDenominators())
	if !denominatorsOK {
		return Mounted{}, false
	}
	witnesses := make(map[model.DenominatorRef]binding.DenominatorWitness, len(denominatorRefs))
	for _, ref := range denominatorRefs {
		evidence, evidenceOK := inventory.ResolveDenominator(ref)
		if !evidenceOK || !evidence.Available() {
			return Mounted{}, false
		}
		rows := evidence.Rows()
		if rows == nil {
			return Mounted{}, false
		}
		membership, membershipOK := binding.NewMembershipView(ref.Relation(), rows)
		if !membershipOK {
			return Mounted{}, false
		}
		witness, witnessOK := issuer.IssueDenominator(ref, membership, evidence.Evidence())
		if !witnessOK || !witness.Available() || !witness.ValidFor(runtime) || !witness.Matches(ref) {
			return Mounted{}, false
		}
		for index, row := range rows {
			if row.Relation() != ref.Relation() {
				return Mounted{}, false
			}
			for _, prior := range rows[:index] {
				if prior == row {
					return Mounted{}, false
				}
			}
		}
		witnesses[ref] = witness
	}

	// Correlated Apply partitions are admitted only after the global
	// denominator witnesses exist. Inventory contributes raw child evidence;
	// this mount fence issues the immutable runtime directories consumed by
	// arrangement replay. There is no runtime inference fallback.
	directories, directoriesOK := issuePartitionDirectories(cert, inventory, issuer, runtime, witnesses)
	if !directoriesOK {
		return Mounted{}, false
	}
	arrangementPlan, ok := arrangement.Derive(cert, book, inventory, expandCatalog, directories)
	if !ok || !arrangementPlan.Available() || !arrangementPlan.ValidFor(book) {
		return Mounted{}, false
	}

	scopeSchemas := cert.Scopes()
	scopeIDs := make([]model.ScopeID, 0, len(scopeSchemas))
	seenScopes := make(map[model.ScopeID]struct{}, len(scopeSchemas))
	scopeTokens := make(map[model.ScopeID]binding.ScopeToken, len(scopeSchemas))
	scopeArena := newScopeArena()
	for _, scopeSchema := range scopeSchemas {
		if !scopeSchema.Available() || !scopeSchema.ID().Available() {
			return Mounted{}, false
		}
		scopeID := scopeSchema.ID()
		if _, duplicate := seenScopes[scopeID]; duplicate {
			return Mounted{}, false
		}
		seenScopes[scopeID] = struct{}{}
		scopeIDs = append(scopeIDs, scopeID)
		formula := scopeSchema.Region()
		if !formula.Available() {
			return Mounted{}, false
		}
		formulaIdentity := formula.Identity()
		if !formulaIdentity.Available() {
			return Mounted{}, false
		}
		token, tokenOK := issuer.IssueScope(formulaIdentity)
		if !tokenOK || !token.Available() || !token.ValidFor(runtime) {
			return Mounted{}, false
		}
		if _, arenaOK := scopeArena.intern(token, formula); !arenaOK {
			return Mounted{}, false
		}
		scopeTokens[scopeID] = token
	}
	scopeIDs = canonicalizeScopes(scopeIDs)

	heads := cert.WideningHeads()
	permits := make([]WideningPermit, 0, len(heads))
	for _, head := range heads {
		if !head.Available() || !head.Dependency().Available() || !head.Relation().Available() {
			return Mounted{}, false
		}
		if dependencyAddress, dependencyOK := book.Dependency(head.Dependency().ID()); !dependencyOK || !dependencyAddress.ValidFor(fence) {
			return Mounted{}, false
		}
		if relationAddress, relationOK := book.Relation(head.Relation().ID()); !relationOK || !relationAddress.ValidFor(fence) {
			return Mounted{}, false
		}
		permit, permitOK := newWideningPermit(head, runtime)
		if !permitOK {
			return Mounted{}, false
		}
		permits = append(permits, permit)
	}

	mounted, mountedOK := newMounted(cert, book, arrangementPlan, runtime, issuer, lineageAuthority, lineageOwner, lineageIdentity, signatures, bindings, algebras, equalities, denominatorRefs, witnesses, scopeIDs, scopeTokens, scopeArena, permits)
	return mounted, mountedOK
}

// bindEqualities admits the sealed key-equality authorities certified by
// typing. An Ascending type may project equality from its owner algebra
// without admitting that algebra into mounted ascent state. An explicit
// EqualityRegistry remains preferred when present. Every other equality
// obligation is supplied directly by its owner.
func bindEqualities(cert certificate.Certificate, algebras map[model.TypeID]binding.ValueAlgebra, registry binding.AlgebraRegistry) (map[model.TypeID]binding.ValueEquality, bool) {
	types := cert.EqualityRequirements()
	if len(types) == 0 {
		return map[model.TypeID]binding.ValueEquality{}, true
	}
	capabilities := make(map[model.TypeID]model.TypeCapability, len(cert.TypeCapabilities()))
	for _, capability := range cert.TypeCapabilities() {
		if !capability.Available() || !capability.Type().Available() {
			return nil, false
		}
		if _, duplicate := capabilities[capability.Type()]; duplicate {
			return nil, false
		}
		capabilities[capability.Type()] = capability
	}
	equalityRegistry, _ := registry.(binding.EqualityRegistry)
	equalities := make(map[model.TypeID]binding.ValueEquality, len(types))
	for _, typeID := range types {
		if !typeID.Available() {
			return nil, false
		}
		capability, capabilityOK := capabilities[typeID]
		if !capabilityOK || !capability.Equatable() {
			return nil, false
		}
		var equality binding.ValueEquality
		// Equality use never grants ascent authority. Prefer an explicit owner
		// equality witness when one is registered. If the owner exposes only an
		// AlgebraRegistry, resolve that algebra solely to project equality; do
		// not add it to the mounted ascent map.
		if capability.Ascending() {
			if equalityRegistry != nil {
				equality, _ = binding.ResolveEquality(equalityRegistry, typeID)
			}
			if equality == nil {
				algebra, algebraOK := algebras[typeID]
				if !algebraOK {
					algebra, algebraOK = binding.ResolveAlgebra(registry, typeID)
				}
				if !algebraOK {
					return nil, false
				}
				var projected bool
				equality, projected = binding.EqualityFromAlgebra(algebra)
				if !projected {
					return nil, false
				}
			}
		}
		if equality == nil {
			var equalityOK bool
			equality, equalityOK = binding.ResolveEquality(equalityRegistry, typeID)
			if !equalityOK {
				return nil, false
			}
		}
		if _, duplicate := equalities[typeID]; duplicate || equality.Type() != typeID {
			return nil, false
		}
		equalities[typeID] = equality
	}
	return equalities, true
}

// bindLineage admits exactly one immutable proof-sidecar authority for this
// mounted runtime.  The authority ABI is deliberately narrower than the
// mount: it must report the exact runtime fence and carry non-zero owner and
// identity values, but it contributes no mutable inventory lookup.
func bindLineage(factory lineage.Factory, runtime binding.Fence) (lineage.Authority, model.OwnerID, identity.ContentID, bool) {
	if factory == nil || !runtime.Available() {
		return nil, model.OwnerID{}, identity.ContentID{}, false
	}
	authority, ok := factory.Bind(runtime)
	if !ok || authority == nil {
		return nil, model.OwnerID{}, identity.ContentID{}, false
	}
	if fence := authority.Fence(); !fence.Available() || !fence.Same(runtime) {
		return nil, model.OwnerID{}, identity.ContentID{}, false
	}
	owner := authority.Owner()
	identityValue := authority.Identity()
	if !owner.Available() || !identityValue.Available() {
		return nil, model.OwnerID{}, identity.ContentID{}, false
	}
	return authority, owner, identityValue, true
}

func certificateDenominators(signatures []signature.Signature, observationDenominators, correlationDenominators []model.DenominatorRef, partitions []certificate.CorrelationPartition, completeDenominators []model.DenominatorRef) ([]model.DenominatorRef, bool) {
	seen := make(map[model.DenominatorRef]struct{})
	result := make([]model.DenominatorRef, 0)
	for _, operation := range signatures {
		for _, input := range operation.Inputs() {
			if !input.Available() || !input.Denominator.Available() {
				return nil, false
			}
			source, sourceOK := input.SourceDenominator()
			if !sourceOK || !source.Available() {
				return nil, false
			}
			// Carrier remains the delivery range authority, while a joined
			// source authority authenticates the delivered cell. Both must be
			// mounted: runtime deliberately resolves neither from the other.
			for _, denominator := range []model.DenominatorRef{input.CarrierDenominator(), source} {
				if _, exists := seen[denominator]; !exists {
					seen[denominator] = struct{}{}
					result = append(result, denominator)
				}
			}
		}
		for _, output := range operation.Outputs() {
			denominator := output.Denominator
			if !denominator.Available() {
				return nil, false
			}
			if _, exists := seen[denominator]; !exists {
				seen[denominator] = struct{}{}
				result = append(result, denominator)
			}
		}
	}
	for _, denominator := range observationDenominators {
		if !denominator.Available() {
			return nil, false
		}
		if _, exists := seen[denominator]; !exists {
			seen[denominator] = struct{}{}
			result = append(result, denominator)
		}
	}
	for _, denominator := range correlationDenominators {
		if !denominator.Available() {
			return nil, false
		}
		if _, exists := seen[denominator]; !exists {
			seen[denominator] = struct{}{}
			result = append(result, denominator)
		}
	}
	for _, partition := range partitions {
		if !partition.Available() {
			return nil, false
		}
		for _, denominator := range []model.DenominatorRef{partition.Population(), partition.Child()} {
			if !denominator.Available() {
				return nil, false
			}
			if _, exists := seen[denominator]; !exists {
				seen[denominator] = struct{}{}
				result = append(result, denominator)
			}
		}
	}
	for _, denominator := range completeDenominators {
		if !denominator.Available() {
			return nil, false
		}
		if _, exists := seen[denominator]; !exists {
			seen[denominator] = struct{}{}
			result = append(result, denominator)
		}
	}
	return canonicalizeDenominators(result), true
}

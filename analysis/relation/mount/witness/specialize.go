package witness

import (
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Specialize admits one complete checked certificate into one exact mounted
// inventory. Every certificate-facing obligation is consumed at this single
// boundary: addresses and arrangements, one exact typed operation factory,
// one exact algebra registry, all algebra requirements, denominator
// row/evidence witnesses, scope formulas, and the validated recurrence-head
// projection.
//
// The returned Mounted owns only immutable snapshots. A false result always
// returns the zero capability.
func Specialize(cert certificate.Certificate, inventory Inventory, factory binding.Factory, algebraRegistry binding.AlgebraRegistry) (Mounted, bool) {
	if inventory == nil || !cert.Available() {
		return Mounted{}, false
	}
	book, ok := address.Bind(cert, inventory)
	if !ok || !book.Available() {
		return Mounted{}, false
	}
	arrangementPlan, ok := arrangement.Derive(cert, book, inventory)
	if !ok || !arrangementPlan.Available() || !arrangementPlan.ValidFor(book) {
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

	denominatorRefs, denominatorsOK := certificateDenominators(signatures)
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

	scopeIDs, scopesOK := certificateScopes(cert)
	if !scopesOK {
		return Mounted{}, false
	}
	scopeValues := make(map[model.ScopeID]Scope, len(scopeIDs))
	for _, scopeID := range scopeIDs {
		formula, formulaOK := inventory.ScopeRegion(scopeID)
		if !formulaOK || !regionAvailable(formula) {
			return Mounted{}, false
		}
		formulaIdentity, identityOK := formula.Identity()
		if !identityOK {
			return Mounted{}, false
		}
		token, tokenOK := issuer.IssueScope(formulaIdentity)
		if !tokenOK || !token.Available() || !token.ValidFor(runtime) {
			return Mounted{}, false
		}
		scopeValue, scopeOK := newScope(token, formula)
		if !scopeOK {
			return Mounted{}, false
		}
		scopeValues[scopeID] = scopeValue
	}

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
		permit, permitOK := newWideningPermit(head)
		if !permitOK {
			return Mounted{}, false
		}
		permits = append(permits, permit)
	}

	return newMounted(cert, book, arrangementPlan, runtime, issuer, signatures, bindings, algebras, denominatorRefs, witnesses, scopeIDs, scopeValues, permits)
}

func certificateDenominators(signatures []signature.Signature) ([]model.DenominatorRef, bool) {
	seen := make(map[model.DenominatorRef]struct{})
	result := make([]model.DenominatorRef, 0)
	for _, operation := range signatures {
		for _, input := range operation.Inputs() {
			if !input.Denominator.Available() {
				return nil, false
			}
			if _, exists := seen[input.Denominator]; !exists {
				seen[input.Denominator] = struct{}{}
				result = append(result, input.Denominator)
			}
		}
		denominator := operation.Authority().Denominator
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

func certificateScopes(cert certificate.Certificate) ([]model.ScopeID, bool) {
	values := cert.Scopes()
	result := make([]model.ScopeID, 0, len(values))
	seen := make(map[model.ScopeID]struct{}, len(values))
	for _, scope := range values {
		if !scope.Available() || !scope.ID().Available() {
			return nil, false
		}
		if _, exists := seen[scope.ID()]; exists {
			continue
		}
		seen[scope.ID()] = struct{}{}
		result = append(result, scope.ID())
	}
	return canonicalizeScopes(result), true
}

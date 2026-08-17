package static

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// compactContracts owns dense authored static sidecars for Flow Function and
// Call terms. It neither reconstructs Flow geometry nor evaluates calls.
func compactContracts(component *Component, counts [keyspace.FamilyCount]uint32, input ContractsInput) error {
	store := &component.contracts
	for _, row := range input.Function {
		if !row.ReturnsKnown && len(row.Returns) != 0 {
			return errors.New("program/static: omitted function returns have children")
		}
		for _, result := range row.Returns {
			if !staticrole.Node(counts, result) {
				return errors.New("program/static: invalid function return")
			}
		}
		params, ok := appendTerms(&store.terms, row.TypeParams)
		if !ok {
			return errors.New("program/static: oversized function type parameters")
		}
		returns, ok := appendTerms(&store.terms, row.Returns)
		if !ok {
			return errors.New("program/static: oversized function returns")
		}
		store.functions = append(store.functions, functionContractRow{
			typeParams: params, returnsKnown: row.ReturnsKnown, returns: returns,
		})
	}
	for _, row := range input.Call {
		for _, typeArgument := range row.TypeArguments {
			if !staticrole.Node(counts, typeArgument) {
				return errors.New("program/static: invalid call type argument")
			}
		}
		typeArguments, ok := appendTerms(&store.terms, row.TypeArguments)
		if !ok {
			return errors.New("program/static: oversized call type arguments")
		}
		store.calls = append(store.calls, typeArguments)
		id, idOK := callTypeArgumentID(row.TypeArguments)
		if !idOK {
			return errors.New("program/static: unavailable call type-argument identity")
		}
		store.callTypeArgumentIDs = append(store.callTypeArgumentIDs, id)
	}
	return nil
}

func callTypeArgumentID(terms []keyspace.Term) (id identity.ContentID, ok bool) {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/static/call-type-arguments", 1) != nil || writeTypeTermsContent(&writer, terms) != nil || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}, false
	}
	return id, id.Available()
}

// completeTypeParamOwnership is the single exact owner/order law for every
// TypeParam. Each row is claimed once by the one authored owner relation;
// aliases, static type functions, and Flow functions cannot each grow a
// slightly different validator.
func completeTypeParamOwnership(component *Component, counts [keyspace.FamilyCount]uint32) bool {
	seen := make([]bool, len(component.declarations.params))
	claim := func(owner keyspace.Term, params []keyspace.Term) bool {
		for _, param := range params {
			if !hasFamily(counts, param, keyspace.FamilyTypeParam) {
				return false
			}
			ordinal := keyspace.TermOrdinal(param) - 1
			if seen[ordinal] || component.declarations.params[ordinal].Owner != owner {
				return false
			}
			seen[ordinal] = true
		}
		return true
	}
	for index, row := range component.declarations.aliases {
		if !claim(keyspace.MakeTerm(keyspace.FamilyTypeAlias, uint32(index+1)), component.declarations.aliasParams[row.params.Start:row.params.End]) {
			return false
		}
	}
	for index, row := range component.signatures.functions {
		if !claim(keyspace.MakeTerm(keyspace.FamilyTypeFunction, uint32(index+1)), component.signatures.params[row.typeParams.Start:row.typeParams.End]) {
			return false
		}
	}
	for index, row := range component.contracts.functions {
		if !claim(keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1)), component.contracts.terms[row.typeParams.Start:row.typeParams.End]) {
			return false
		}
	}
	for index, claimed := range seen {
		if !claimed || !staticrole.TypeParameterOwner(counts, component.declarations.params[index].Owner) {
			return false
		}
	}
	return true
}

// emitContractsContainment owns Static's authored sidecars on opaque Flow
// Function and Call parents. It does not inspect their bodies, values, or
// control flow.
func emitContractsContainment(component *Component, check *containment) bool {
	store := &component.contracts
	for index, row := range store.functions {
		parent := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		for _, child := range store.terms[row.returns.Start:row.returns.End] {
			if !check.attach(parent, child) || !check.markIfAssertionReturn(parent, child) {
				return false
			}
		}
	}
	for index, row := range store.calls {
		parent := keyspace.MakeTerm(keyspace.FamilyCall, uint32(index+1))
		for _, child := range store.terms[row.Start:row.End] {
			if !check.attach(parent, child) {
				return false
			}
		}
	}
	return true
}

// writeContractsContent owns the dense static sidecars for opaque Flow
// Function and Call identities. It hashes semantic sequences, never their
// shared-pool offsets.
func writeContractsContent(writer *framing.Writer, store contractsStore) error {
	if err := writer.Count(uint64(len(store.functions))); err != nil {
		return err
	}
	for _, row := range store.functions {
		if err := writeTypeTermsContent(writer, store.terms[row.typeParams.Start:row.typeParams.End]); err != nil {
			return err
		}
		if err := writer.Bool(row.returnsKnown); err != nil {
			return err
		}
		if err := writeTypeTermsContent(writer, store.terms[row.returns.Start:row.returns.End]); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.calls))); err != nil {
		return err
	}
	for _, row := range store.calls {
		if err := writeTypeTermsContent(writer, store.terms[row.Start:row.End]); err != nil {
			return err
		}
	}
	return nil
}

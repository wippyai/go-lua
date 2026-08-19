package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// Build seals authored static syntax into an immutable Component. It is a
// pure constructor: nothing is mutated across the pipeline. Each vertical
// validates its own authored rows and returns a sealed table by value, and
// the laws that span two verticals then run over those sealed values through
// the columns their owners publish. It accepts no inferred or domain
// resolution; those have no place in this owner.
//
// The stages are ordered by what they read, not by convenience:
//
//	census      the one cardinality column, sealed from the authored input
//	independent every vertical that needs nothing but the census
//	dependent   the verticals that consume an already-sealed sibling table
//	joint       the laws no single vertical can close alone
//	identity    the content digest over the sealed sections
func Build(input Input) (*Component, staticquery.View, error) {
	if !validCounts(input.Counts) {
		return nil, staticquery.View{}, errors.New("program/static: inconsistent authored cardinality")
	}
	// Counts are the already-sealed family column supplied by the enclosing
	// owner. Each typed child validates its own native rows below; Static does
	// not maintain a second family inventory or re-census those rows here.
	census := input.Counts

	typeTable, err := statictypes.Build(input.Types, census)
	if err != nil {
		return nil, staticquery.View{}, err
	}
	if !typeTable.CountsMatch(census) {
		return nil, staticquery.View{}, errors.New("program/static: type cardinality disagrees with sealed column")
	}
	referenceTable, err := staticrefs.Build(input.References, census)
	if err != nil {
		return nil, staticquery.View{}, err
	}
	if !referenceTable.CountsMatch(census) {
		return nil, staticquery.View{}, errors.New("program/static: reference cardinality disagrees with sealed column")
	}
	declarationTable, err := staticdecl.Build(input.Declarations, census)
	if err != nil {
		return nil, staticquery.View{}, err
	}
	if !declarationTable.CountsMatch(census) {
		return nil, staticquery.View{}, errors.New("program/static: declaration cardinality disagrees with sealed column")
	}
	signatureTable, err := staticsig.Build(input.Signatures, census)
	if err != nil {
		return nil, staticquery.View{}, err
	}
	if !signatureTable.CountsMatch(census) {
		return nil, staticquery.View{}, errors.New("program/static: signature cardinality disagrees with sealed column")
	}
	contractTable, err := staticcontracts.Build(input.Contracts, census)
	if err != nil {
		return nil, staticquery.View{}, err
	}
	if !contractTable.CountsMatch(census) {
		return nil, staticquery.View{}, errors.New("program/static: contract cardinality disagrees with sealed column")
	}
	operatorTable, err := staticoperators.Build(input.Operators, census)
	if err != nil {
		return nil, staticquery.View{}, err
	}
	if !operatorTable.CountsMatch(census) {
		return nil, staticquery.View{}, errors.New("program/static: operator cardinality disagrees with sealed column")
	}

	// Publications admits only a reference disposition References published;
	// Operands admits a runtime type target only on what Types and References
	// published. Both take those tables as sealed values.
	publicationTable, err := staticpubs.Build(input.Publications, census, referenceTable)
	if err != nil {
		return nil, staticquery.View{}, err
	}
	if !publicationTable.CountsMatch(census) {
		return nil, staticquery.View{}, errors.New("program/static: publication cardinality disagrees with sealed column")
	}
	operandTable, err := staticoperands.Build(input.Operands, census, typeTable, referenceTable)
	if err != nil {
		return nil, staticquery.View{}, err
	}
	if !operandTable.CountsMatch(census) {
		return nil, staticquery.View{}, errors.New("program/static: operand cardinality disagrees with sealed column")
	}

	component := &Component{
		census:       census,
		types:        typeTable,
		references:   referenceTable,
		declarations: declarationTable,
		signatures:   signatureTable,
		contracts:    contractTable,
		operators:    operatorTable,
		operands:     operandTable,
		publications: publicationTable,
	}
	if !interfaceMethodScopes(component) {
		return nil, staticquery.View{}, errors.New("program/static: interface method signature scope mismatch")
	}
	if !completeTypeParamOwnership(component, census) {
		return nil, staticquery.View{}, errors.New("program/static: incomplete or multiply-owned type parameter")
	}
	localProof, ok := localForest(component)
	if !ok {
		return nil, staticquery.View{}, errors.New("program/static: cyclic, shared, or incomplete static declaration child")
	}

	component.contentID = contentID(component)
	if !component.contentID.Available() {
		return nil, staticquery.View{}, errors.New("program/static: unavailable content identity")
	}
	return component, staticquery.NewView(component.querySnapshotWithProof(localProof)), nil
}

func validCounts(counts [keyspace.FamilyCount]uint32) bool {
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return false
	}
	for _, count := range counts {
		if count > keyspace.MaxTermOrdinal {
			return false
		}
	}
	return true
}

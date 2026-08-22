package manifesttarget_test

import (
	"strings"
	"testing"

	targetcontract "github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// standardLibraryContract seals the compiled-in provider set through the one
// composition entry point.
func standardLibraryContract() (*targetcontract.Contract, error) {
	catalogue, err := manifest.Seal(stdlib.Providers()...)
	if err != nil {
		return nil, err
	}
	return manifesttarget.SealCatalogue(catalogue)
}

// An outcome result vector is a fixed prefix, an open tail of one element
// type, and an end-anchored suffix. This file is the law for the third part:
// the seal either carries it into the sealed result geometry or refuses the
// declaration by name. It is never kept in the content identity and dropped
// from every reader.

const suffixMember = "reads"

// closedSuffixManifest declares a result suffix on a closed outcome. A closed
// vector has no end-relative coordinate to keep, so the suffix is exactly the
// tail of the fixed prefix and the seal folds it there.
func closedSuffixManifest() *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	declaration.DefineFunctionSignature(suffixMember, signature.Function{
		Type: typ.Func().Param("source", typ.String).Returns(typ.String).Build(),
	})
	declaration.DefineFunctionOperation(suffixMember, manifestwire.Operation{
		Replace: true,
		Input:   manifestwire.Values{Fixed: []typ.Type{typ.String}, Tail: manifestwire.ValuesClosed},
		Outcomes: []manifestwire.Outcome{
			{Kind: manifestwire.OutcomeNormal, Values: manifestwire.Values{
				Fixed: []typ.Type{typ.String}, Tail: manifestwire.ValuesClosed, Suffix: []typ.Type{typ.Integer},
			}},
			{Kind: manifestwire.OutcomeThrow, Values: anyClosed()},
		},
		Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	})
	return declaration
}

// openSuffixManifest declares the shape a real native return has: the values
// the call produces, followed by one end-anchored result.
func openSuffixManifest() *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	declaration.DefineFunctionSignature(suffixMember, signature.Function{
		Type: typ.Func().Param("source", typ.String).Build(),
	})
	declaration.DefineFunctionOperation(suffixMember, manifestwire.Operation{
		Replace:    true,
		ValuesVars: 1,
		Input:      manifestwire.Values{Fixed: []typ.Type{typ.String}, Tail: manifestwire.ValuesClosed},
		Outcomes: []manifestwire.Outcome{
			{Kind: manifestwire.OutcomeNormal, Values: manifestwire.Values{
				Tail: manifestwire.ValuesVariable, Var: 0, TailType: typ.Any, Suffix: []typ.Type{typ.Integer},
			}},
			{Kind: manifestwire.OutcomeThrow, Values: anyClosed()},
		},
		Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
	})
	return declaration
}

// signatureSuffixManifest states the same end-anchored result through the
// declaration signature instead of the operation law. Both spellings reach the
// one outcome relation, so both owe the same answer.
func signatureSuffixManifest() *manifestwire.Manifest {
	declaration := manifestwire.New(relationHostModule)
	declaration.DefineFunctionSignature(suffixMember, signature.Function{
		Type:         typ.Func().Param("source", typ.String).Build(),
		ResultTail:   typ.Any,
		ResultSuffix: []typ.Type{typ.Integer},
	})
	return declaration
}

// A suffix on a closed result vector reaches the sealed result geometry: the
// seal folds it into the fixed prefix, where it owns a result slot.
func TestClosedOutcomeSuffixReachesTheSealedResultGeometry(t *testing.T) {
	contract, err := sealRelationCatalogue(closedSuffixManifest())
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := contract.Operations.Lookup(relationBinding(suffixMember))
	if !ok {
		t.Fatal("sealed Target holds no reads operation")
	}
	_, values, outcomeOK := contract.Operations.OutcomeAt(operation, 0)
	if !outcomeOK {
		t.Fatal("sealed Target holds no normal outcome for reads")
	}
	if count := contract.Operations.ValuesCount(values); count != 2 {
		t.Fatalf("fixed result width = %d, want the declared prefix and the folded suffix", count)
	}
	slots, slotsOK := contract.Operations.OutcomeValueSlots(operation, 0)
	if !slotsOK || slots != 2 {
		t.Fatalf("outcome value slots = %d/%t, want a slot for every declared result", slots, slotsOK)
	}
	if suffix := contract.Operations.ValuesSuffixCount(values); suffix != 0 {
		t.Fatalf("folded suffix width = %d, want the suffix carried as fixed results", suffix)
	}
}

// A suffix behind an open result tail is end-anchored, and the sealed result
// vocabulary addresses a result by its fixed ordinal only. The declaration is
// refused by name instead of sealing with the distinction dropped.
func TestOpenOutcomeSuffixIsRefusedByName(t *testing.T) {
	for name, declaration := range map[string]*manifestwire.Manifest{
		"operation law": openSuffixManifest(),
		"signature":     signatureSuffixManifest(),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := sealRelationCatalogue(declaration)
			if err == nil {
				t.Fatal("an end-anchored result suffix sealed, want a refusal")
			}
			if !strings.Contains(err.Error(), "result suffix behind an open result tail") {
				t.Fatalf("refusal = %v, want it to name the end-anchored result suffix", err)
			}
		})
	}
}

// The sealed standard library declares no end-anchored result suffix. This is
// the negative half of the law above: a suffix that reached the seal would be
// invisible to every reader, so none may survive one.
func TestSealedStandardLibraryDeclaresNoUnreachableResultSuffix(t *testing.T) {
	contract, err := standardLibraryContract()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < contract.Operations.OperationCount(); index++ {
		operation, ok := contract.Operations.OperationAt(index)
		if !ok {
			t.Fatalf("operation %d is unavailable", index)
		}
		for outcome := 0; outcome < contract.Operations.OutcomeCount(operation); outcome++ {
			_, values, outcomeOK := contract.Operations.OutcomeAt(operation, outcome)
			if !outcomeOK {
				t.Fatalf("operation %d outcome %d is unavailable", index, outcome)
			}
			if suffix := contract.Operations.ValuesSuffixCount(values); suffix != 0 {
				t.Fatalf("operation %d outcome %d seals a %d-value result suffix no reader can address", index, outcome, suffix)
			}
		}
	}
}

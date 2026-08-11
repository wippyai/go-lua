package relations

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestGeneratedRelationDispositionsCoverCanonicalSchemaExactly(t *testing.T) {
	schema, err := CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := GeneratedRelationDispositions()
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Count() != schema.Count() {
		t.Fatalf("relation disposition count = %d, want %d", ledger.Count(), schema.Count())
	}
	for index, source := range schema.Rows() {
		row, ok := ledger.At(index)
		if !ok {
			t.Fatalf("missing disposition %d", index)
		}
		kind, valid := dispositionForForm(source.Form)
		if !valid || row.Definition != source.Definition || row.Owner != source.Owner || row.Kind != kind {
			t.Fatalf("disposition %d does not match generated relation", index)
		}
		resolved, ok := ledger.Disposition(source.Definition.Token())
		if !ok || resolved != row {
			t.Fatalf("disposition lookup %d changed row", index)
		}
	}
	if _, ok := ledger.At(-1); ok {
		t.Fatal("negative disposition index accepted")
	}
}

func TestRelationDispositionSealRejectsMissingDuplicateDefaultAndOwnerDrift(t *testing.T) {
	schema, err := CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := GeneratedRelationDispositions()
	if err != nil {
		t.Fatal(err)
	}
	rows := relationDispositionRows(t, canonical)

	if _, err := SealRelationDispositions(schema, rows[:len(rows)-1]); !errors.Is(err, ErrMissingRelationDisposition) {
		t.Fatalf("missing disposition = %v", err)
	}

	duplicate := append([]RelationDisposition(nil), rows...)
	duplicate[len(duplicate)-1] = duplicate[0]
	if _, err := SealRelationDispositions(schema, duplicate); !errors.Is(err, ErrDuplicateRelationDisposition) {
		t.Fatalf("duplicate disposition = %v", err)
	}

	invalid := append([]RelationDisposition(nil), rows...)
	invalid[0].Kind = RelationDispositionInvalid
	if _, err := SealRelationDispositions(schema, invalid); !errors.Is(err, ErrInvalidRelationDisposition) {
		t.Fatalf("default disposition = %v", err)
	}

	drifted := append([]RelationDisposition(nil), rows...)
	drifted[0].Owner++
	if _, err := SealRelationDispositions(schema, drifted); !errors.Is(err, ErrInvalidRelationDisposition) {
		t.Fatalf("owner drift = %v", err)
	}
}

func TestDerivativeLedgerSealsCanonicalPolynomialLawsAndDetaches(t *testing.T) {
	relations, err := GeneratedRelationDispositions()
	if err != nil {
		t.Fatal(err)
	}
	first, _ := relations.At(0)
	second, _ := relations.At(1)
	input := []DerivativeDeclaration{
		{
			Derivative: 2,
			Owner:      first.Owner,
			Kind:       DerivativePostRootCold,
			Observed:   []semanticsource.Token{first.Definition.Token(), second.Definition.Token()},
			Query:      QueryRebind,
			Cardinality: CardinalityLaw{Terms: []CardinalityTerm{
				{Factors: []semanticsource.Token{first.Definition.Token()}, Coefficient: 1},
				{Factors: []semanticsource.Token{first.Definition.Token(), second.Definition.Token()}, Coefficient: 2},
			}},
			Asymptotic: AsymptoticLogarithmic,
			Allocation: AllocationColdOnly,
			Replay:     ReplayPersistObservedClosure,
		},
		{
			Derivative: 1,
			Owner:      first.Owner,
			Kind:       DerivativePrivateIndex,
			Observed:   []semanticsource.Token{first.Definition.Token()},
			Query:      QueryExactLookup,
			Cardinality: CardinalityLaw{Terms: []CardinalityTerm{
				{Factors: []semanticsource.Token{first.Definition.Token()}, Coefficient: 1},
			}},
			Asymptotic: AsymptoticConstant,
			Allocation: AllocationZero,
			Replay:     ReplayRebuild,
		},
	}
	ledger, err := SealDerivativeLedger(relations, input)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Count() != 2 {
		t.Fatalf("derivative count = %d", ledger.Count())
	}
	firstRow, ok := ledger.At(0)
	if !ok || firstRow.Derivative != 1 {
		t.Fatalf("canonical derivative order = %#v/%v", firstRow, ok)
	}
	firstRow.Observed[0] = second.Definition.Token()
	firstRow.Cardinality.Terms[0].Coefficient = 99
	firstRow.Cardinality.Terms[0].Factors[0] = second.Definition.Token()
	again, _ := ledger.At(0)
	if again.Observed[0] != first.Definition.Token() || again.Cardinality.Terms[0].Coefficient != 1 || again.Cardinality.Terms[0].Factors[0] != first.Definition.Token() {
		t.Fatal("derivative ledger exposed mutable declaration storage")
	}
}

func TestDerivativeLedgerRejectsUnknownDefaultsAndReplayDrift(t *testing.T) {
	relations, err := GeneratedRelationDispositions()
	if err != nil {
		t.Fatal(err)
	}
	first, _ := relations.At(0)
	second, _ := relations.At(1)
	valid := DerivativeDeclaration{
		Derivative:  1,
		Owner:       first.Owner,
		Kind:        DerivativePrivateIndex,
		Observed:    []semanticsource.Token{first.Definition.Token(), second.Definition.Token()},
		Query:       QueryMembership,
		Cardinality: CardinalityLaw{Terms: []CardinalityTerm{{Factors: []semanticsource.Token{first.Definition.Token()}, Coefficient: 1}}},
		Asymptotic:  AsymptoticConstant,
		Allocation:  AllocationZero,
		Replay:      ReplayRebuild,
	}

	tests := []struct {
		name string
		edit func(*DerivativeDeclaration)
	}{
		{"zero identity", func(row *DerivativeDeclaration) { row.Derivative = 0 }},
		{"unset owner", func(row *DerivativeDeclaration) { row.Owner = OwnerUnset }},
		{"unset query", func(row *DerivativeDeclaration) { row.Query = QueryInvalid }},
		{"unknown closure", func(row *DerivativeDeclaration) { row.Observed[0] = semanticsource.Token{} }},
		{"unordered closure", func(row *DerivativeDeclaration) {
			row.Observed[0], row.Observed[1] = row.Observed[1], row.Observed[0]
		}},
		{"cardinality source outside closure", func(row *DerivativeDeclaration) {
			row.Observed = row.Observed[:1]
			row.Cardinality.Terms[0].Factors[0] = second.Definition.Token()
		}},
		{"empty factors", func(row *DerivativeDeclaration) { row.Cardinality.Terms[0].Factors = nil }},
		{"duplicate monomial", func(row *DerivativeDeclaration) {
			duplicate := row.Cardinality.Terms[0]
			duplicate.Factors = append([]semanticsource.Token(nil), duplicate.Factors...)
			row.Cardinality.Terms = append(row.Cardinality.Terms, duplicate)
		}},
		{"zero coefficient", func(row *DerivativeDeclaration) { row.Cardinality.Terms[0].Coefficient = 0 }},
		{"private persisted", func(row *DerivativeDeclaration) { row.Replay = ReplayPersistObservedClosure }},
		{"cold hot allocation", func(row *DerivativeDeclaration) {
			row.Kind = DerivativePostRootCold
			row.Replay = ReplayPersistObservedClosure
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := cloneDerivative(valid)
			test.edit(&row)
			if _, err := SealDerivativeLedger(relations, []DerivativeDeclaration{row}); !errors.Is(err, ErrInvalidDerivative) {
				t.Fatalf("invalid derivative = %v", err)
			}
		})
	}

	duplicate := []DerivativeDeclaration{cloneDerivative(valid), cloneDerivative(valid)}
	if _, err := SealDerivativeLedger(relations, duplicate); !errors.Is(err, ErrDuplicateDerivative) {
		t.Fatalf("duplicate derivative = %v", err)
	}
}

func TestDerivativeLedgerRejectsRetainedApplicationOperationProducts(t *testing.T) {
	relations, err := GeneratedRelationDispositions()
	if err != nil {
		t.Fatal(err)
	}
	application, applicationOK := CatalogToken("LinkProjectBaseApplication@-")
	operation, operationOK := CatalogToken("TargetOperation@-")
	boundary, boundaryOK := CatalogToken("LinkBoundary@-")
	if !applicationOK || !operationOK || !boundaryOK {
		t.Fatal("missing generated product axes")
	}
	declaration := DerivativeDeclaration{
		Derivative:  1,
		Owner:       OwnerLinkProject,
		Kind:        DerivativePrivateIndex,
		Observed:    []semanticsource.Token{operation, application},
		Query:       QueryMembership,
		Cardinality: CardinalityLaw{Terms: []CardinalityTerm{{Factors: []semanticsource.Token{operation, application}, Coefficient: 1}}},
		Asymptotic:  AsymptoticConstant,
		Allocation:  AllocationZero,
		Replay:      ReplayRebuild,
	}
	if lessToken(application, operation) {
		declaration.Observed[0], declaration.Observed[1] = declaration.Observed[1], declaration.Observed[0]
		declaration.Cardinality.Terms[0].Factors[0], declaration.Cardinality.Terms[0].Factors[1] = declaration.Cardinality.Terms[0].Factors[1], declaration.Cardinality.Terms[0].Factors[0]
	}
	if _, err := SealDerivativeLedger(relations, []DerivativeDeclaration{declaration}); !errors.Is(err, ErrInvalidDerivative) {
		t.Fatalf("retained Application×Operation product = %v", err)
	}
	declaration.Observed = []semanticsource.Token{boundary}
	declaration.Cardinality.Terms = []CardinalityTerm{{Factors: []semanticsource.Token{boundary}, Coefficient: 1}}
	if _, err := SealDerivativeLedger(relations, []DerivativeDeclaration{declaration}); !errors.Is(err, ErrInvalidDerivative) {
		t.Fatalf("retained virtual boundary product = %v", err)
	}
}

func relationDispositionRows(t testing.TB, ledger RelationDispositionLedger) []RelationDisposition {
	t.Helper()
	rows := make([]RelationDisposition, ledger.Count())
	for index := range rows {
		row, ok := ledger.At(index)
		if !ok {
			t.Fatalf("missing relation disposition %d", index)
		}
		rows[index] = row
	}
	return rows
}

func cloneDerivative(input DerivativeDeclaration) DerivativeDeclaration {
	input.Observed = append([]semanticsource.Token(nil), input.Observed...)
	input.Cardinality.Terms = append([]CardinalityTerm(nil), input.Cardinality.Terms...)
	for index := range input.Cardinality.Terms {
		input.Cardinality.Terms[index].Factors = append([]semanticsource.Token(nil), input.Cardinality.Terms[index].Factors...)
	}
	return input
}

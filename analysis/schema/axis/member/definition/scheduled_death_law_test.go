package definition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// specimenDerivation is one authored relation derivation on the specimen base:
// a dependent relation whose rows a domain symbol builds.
func specimenDerivation(t testing.TB) Definition {
	t.Helper()
	source := specimenBase()
	source.Relations = append(source.Relations, Relation{
		Name:              "Derived",
		Key:               "specimen/derived",
		Subject:           "FactCarrier",
		Inputs:            []RelationInput{{Carrier: "SeedCarrier"}},
		CandidateProvider: member.RelationRef{Axis: specimenAxis(), Member: "specimen/candidates"},
		Derivation: RelationDerivation{
			State:      specimenType("Plan"),
			Build:      GoSymbol{PackagePath: specimenPackage, Name: "DeriveRows"},
			Count:      GoSymbol{PackagePath: specimenPackage, Name: "RowCount"},
			At:         GoSymbol{PackagePath: specimenPackage, Name: "RowAt"},
			StaticAxes: []schema.EntryReference{specimenAxis()},
		},
	})
	return source
}

// withScheduledDeath installs one ledger row for the duration of a law and
// restores the declared table afterwards. The table is a declaration, so a law
// borrows it rather than growing a registration path nothing else has.
func withScheduledDeath(t testing.TB, rows ...ScheduledDeath) {
	t.Helper()
	restore := scheduledDeaths
	scheduledDeaths = append(append([]ScheduledDeath(nil), restore...), rows...)
	t.Cleanup(func() { scheduledDeaths = restore })
}

// TestAnAuthoredDerivationIsAdmittedOnlyWhileItIsScheduledToDie is fence four
// of the authored-Build ruling, stated where the declaration is admitted.
//
// A Build is authored domain logic behind a derived contract, which the other
// three fences hold it to. This one holds the AUTHORING itself to a deadline:
// a derivation is relational, relational derivation is what a declaration
// emits once the reduction algebra lands, and so every authored Build is a
// migration unit. An unregistered one would outlive the migration silently,
// which is the one failure a comment cannot prevent.
func TestAnAuthoredDerivationIsAdmittedOnlyWhileItIsScheduledToDie(t *testing.T) {
	source := specimenDerivation(t)
	if source.Complete() {
		t.Fatal("an unregistered authored derivation was admitted")
	}
	withScheduledDeath(t, ScheduledDeath{
		Axis: "specimen", Relation: "specimen/derived",
		Build: GoSymbol{PackagePath: specimenPackage, Name: "DeriveRows"},
	})
	if !source.Complete() {
		t.Fatal("a registered authored derivation was refused")
	}
}

// TestAScheduledDeathRegistersOneDerivationAndNotAName states what a row is
// keyed on. The registration is of one axis's one relation building its rows
// with one symbol; a row that matched on any less than that would let a second
// axis inherit a registration it never made, and the migration set would
// undercount the work it exists to bound.
func TestAScheduledDeathRegistersOneDerivationAndNotAName(t *testing.T) {
	build := GoSymbol{PackagePath: specimenPackage, Name: "DeriveRows"}
	for _, test := range []struct {
		name string
		row  ScheduledDeath
	}{
		{name: "another-axis", row: ScheduledDeath{Axis: "other", Relation: "specimen/derived", Build: build}},
		{name: "another-relation", row: ScheduledDeath{Axis: "specimen", Relation: "specimen/other", Build: build}},
		{name: "another-package", row: ScheduledDeath{Axis: "specimen", Relation: "specimen/derived",
			Build: GoSymbol{PackagePath: "example/other", Name: "DeriveRows"}}},
		{name: "another-symbol", row: ScheduledDeath{Axis: "specimen", Relation: "specimen/derived",
			Build: GoSymbol{PackagePath: specimenPackage, Name: "DeriveOther"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			withScheduledDeath(t, test.row)
			if specimenDerivation(t).Complete() {
				t.Fatal("a derivation was admitted by a row that does not name it")
			}
		})
	}
}

// TestTheMigrationSetIsPublishedAsAWholeCopy states that the ledger is a
// declaration and not a registry. Nothing appends to it at run time, so the
// set of authored derivations is decided in one place and read everywhere.
func TestTheMigrationSetIsPublishedAsAWholeCopy(t *testing.T) {
	published := ScheduledDeaths()
	if len(published) != len(scheduledDeaths) {
		t.Fatalf("published %d rows, the table declares %d", len(published), len(scheduledDeaths))
	}
	if len(published) == 0 {
		t.Fatal("the migration set is empty while authored derivations are still declared")
	}
	published[0] = ScheduledDeath{}
	if ScheduledDeaths()[0] == (ScheduledDeath{}) {
		t.Fatal("a consumer reached the declared table through its published copy")
	}
	for _, row := range ScheduledDeaths() {
		if !row.Axis.Available() || !row.Relation.Available() || !row.Build.Available() {
			t.Fatalf("migration row %+v does not name a derivation", row)
		}
	}
}

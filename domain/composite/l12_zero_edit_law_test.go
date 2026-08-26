package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	schemacomposite "github.com/wippyai/go-lua/analysis/schema/composite"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	issuanceschema "github.com/wippyai/go-lua/analysis/schema/issuance"
	"github.com/wippyai/go-lua/analysis/schema/observation"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/probe"
)

// The zero-edit walk is the acceptance law of the identity-row revision: a
// domain outside the analyzer declares a complete row set - a coordinate space
// and the writer principal it is, the rule that writes it, the semantic roles
// both are identified by, the observation population its finding is measured
// over, and a published code under a publication family the analyzer has never
// published before - from its own package, and the table composed from the
// analyzer's rows and its own seals.
//
// Each step below names the exact line the same declaration used to be rejected
// at, so what the law records is a direction that moved rather than a property
// that happens to hold:
//
//   - W1 family: diagnostic.go's Family enum held four members and a row's family
//     had to be one of them, so a code under a family no enum member spelled had
//     no admissible declaration.
//   - W2 observation: diagnostic.go's observationAvailable was a range bound over
//     the artifact's own observation enum, so a population the artifact does not
//     number was rejected at admission.
//   - W3 axis: axis.go's admission rejected a spec whose Principal was not a
//     closed factor-lane enum, so a writer principal outside that enum could
//     not be declared.
//   - W4 rule: rule.go's admission derived the writer lane from a Role catalog
//     and LawRoleOrdinal pinned the declaration position to that catalog, so a
//     rule beyond the artifact's twenty had no slot.
//   - W5 roles: vocabulary.Bundle was a closed struct of named fields, so a role
//     no field held could not be named by any surface.
//   - W6 issuance: the artifact's emission program was a switch over its own role
//     catalog, so a rule outside that catalog had no arm and its subscription had
//     nowhere to be declared.
//
// # The boundary this law states, exactly
//
// The walk proves DECLARABILITY: the probe's rows are admitted, the composed
// catalog seals, and every reference the probe declares resolves against the
// sealed table. It does NOT prove that the probe EXECUTES. Occurrence emission
// from the declared subscription, the engine binding of a probe hot half, and a
// compiled artifact's adoption of a role directory are the program fork's A1-A3
// work and are not reached here; the probe's own hooks reject for that reason.
// A new domain that must run, rather than only be declared, needs that work
// completed.

// walkInventory is what one composition of the walk produced: the semantic role
// vocabulary the table was composed against, the probe's own admitted rows, and
// the sizes of the analyzer's own inventories, which are the positions the
// probe's rows land at.
type walkInventory struct {
	roles         vocabulary.Roles
	structures    []*structure.Entry
	probeAxis     *axisTemplate
	probeRule     *rule.Template
	analyzerAxes  int
	analyzerRules int
}

// composeWalk composes one declaration table from the analyzer's own
// contributions plus the probe's, and seals it. The probe's diagnostic row is a
// parameter so the family law can be stated over a row that disagrees with its
// own family as well as over the one that agrees.
//
// Nothing here edits an analyzer table: every contribution is the production
// function that composes the analyzer's own rows, and the probe's rows are
// appended to the aggregation the surfaces already number.
func composeWalk(t *testing.T, spec diagnostic.Spec) (*seal.Schema, schema.SealFailure, walkInventory) {
	t.Helper()
	structures, structuresOK := structure.Collect(append(structureContributions(), probe.StructureSpecs())...)
	if !structuresOK {
		t.Fatal("structural vocabulary rejected the probe's contribution")
	}
	roles, rolesOK := vocabulary.NewRoles(structures)
	if !rolesOK {
		t.Fatal("semantic role vocabulary did not resolve with the probe's roles declared")
	}
	axes, _, axesOK := axisTemplates()
	if !axesOK {
		t.Fatal("analyzer axis inventory rejected at construction")
	}
	probeAxis, probeAxisOK := axis.New(probe.AxisEntry[LinkInputs]())
	if !probeAxisOK {
		t.Fatal("W3: the probe's axis was not admitted")
	}
	rules, _, rulesOK := RuleTemplates[principals, authorities]()
	if !rulesOK {
		t.Fatal("analyzer rule inventory rejected at construction")
	}
	probeRule, probeRuleOK := rule.New(probe.RuleEntry[principals, authorities]())
	if !probeRuleOK {
		t.Fatal("W4: the probe's rule was not admitted")
	}
	diagnostics, diagnosticsOK := diagnosticEntries()
	if !diagnosticsOK {
		t.Fatal("analyzer diagnostic inventory rejected at construction")
	}
	probeDiagnostic, probeDiagnosticOK := diagnostic.New(spec)
	if !probeDiagnosticOK {
		t.Fatalf("W1/W2: the probe's diagnostic %q was not admitted", spec.Code)
	}
	inventory := walkInventory{
		roles: roles, structures: structures,
		probeAxis: probeAxis, probeRule: probeRule,
		analyzerAxes: len(axes), analyzerRules: len(rules),
	}
	axes = append(axes, probeAxis)
	rules = append(rules, probeRule)
	diagnostics = append(diagnostics, probeDiagnostic)

	issuances, issuancesOK := issuanceEntries()
	if !issuancesOK {
		t.Fatal("analyzer issuance inventory rejected at construction")
	}
	composites, compositesOK := compositeEntries()
	denominators, denominatorsOK := denominatorEntries(axes, roles)
	queries, _, queriesOK := queryRegistrations(roles)
	observations, observationsOK := observationEntries(queries)
	if !compositesOK || !denominatorsOK || !queriesOK || !observationsOK {
		t.Fatal("a derived analyzer inventory rejected the extended axis and role sets")
	}

	builder := seal.NewBuilder()
	builder.Register(structure.NewSurface(structures))
	builder.Register(axis.NewSurface(axes))
	// The issuance machine is the surface a rule's subscription is sealed
	// against, so it is registered between the axes and the rules exactly as
	// the production composition registers it. The walk composes the
	// analyzer's own inventories; a table missing one of them would state a
	// composition no analyzer performs.
	builder.Register(issuanceschema.NewSurface(issuances))
	builder.Register(rule.NewSurface(rules))
	builder.Register(diagnostic.NewSurface(diagnostics))
	builder.Register(schemacomposite.NewSurface(composites))
	builder.Register(denominator.NewSurface(denominators, denominator.GeneratedRelationEntries()))
	builder.Register(query.NewSurface(queries))
	builder.Register(observation.NewSurface(observations))
	sealed, failure := builder.Seal()
	return sealed, failure, inventory
}

// walkStructureMember resolves one declared vocabulary member out of the sealed
// table by the key it was declared under, and states the category the resolution
// expects.
func walkStructureMember(t *testing.T, sealed *seal.Schema, key schema.Key, category structure.Category) *structure.Entry {
	t.Helper()
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table carries no structural vocabulary")
	}
	row, resolved := view.ByID(schema.NewEntryID(schema.SurfaceKindStructure, key))
	if !resolved {
		t.Fatalf("structural vocabulary declares no member %q", key)
	}
	member, memberOK := row.(*structure.Entry)
	if !memberOK || member == nil {
		t.Fatalf("member %q is not a structural row", key)
	}
	if member.Category() != category {
		t.Fatalf("member %q is declared in category %d, not %d", key, member.Category(), category)
	}
	return member
}

// TestL12ZeroEditWalk is the acceptance law. One table is composed from the
// analyzer's own rows plus a whole domain declared outside them, and each step
// states the thing that used to have no admissible declaration.
func TestL12ZeroEditWalk(t *testing.T) {
	sealed, failure, inventory := composeWalk(t, probe.DiagnosticSpec())
	if failure.Available() || sealed == nil {
		t.Fatalf("the composed catalog did not seal: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}

	t.Run("W1_family", func(t *testing.T) {
		// The family is a declared row, and the whole of the family law is that
		// the row's declared spelling is the first segment of the published code.
		family := walkStructureMember(t, sealed, probe.FamilyKey, structure.CategoryDiagnosticFamily)
		spelled, spelledOK := probe.Code.Family()
		if !spelledOK || spelled != family.Spelling() {
			t.Fatalf("code %q publishes under %q, and the declared family spells %q", probe.Code, spelled, family.Spelling())
		}
		view, viewOK := sealed.Surface(schema.SurfaceKindDiagnostic)
		if !viewOK {
			t.Fatal("sealed table carries no diagnostic surface")
		}
		row, resolved := view.ByID(schema.NewEntryID(schema.SurfaceKindDiagnostic, schema.Key(probe.Code)))
		if !resolved {
			t.Fatalf("the probe's code %q is not in the sealed diagnostic surface", probe.Code)
		}
		entry, entryOK := row.(*diagnostic.Entry)
		if !entryOK || entry.Family().Key != probe.FamilyKey {
			t.Fatalf("the sealed row for %q does not name the probe's family", probe.Code)
		}
	})

	t.Run("W2_observation", func(t *testing.T) {
		// The population is a declared row and the only bound on it, so the
		// derived table reaches the probe's row through the probe's own key.
		walkStructureMember(t, sealed, probe.ObservationKey, structure.CategoryDiagnosticObservation)
		view, viewOK := sealed.Surface(schema.SurfaceKindDiagnostic)
		if !viewOK {
			t.Fatal("sealed table carries no diagnostic surface")
		}
		table, tableOK := diagnostic.NewTable(view)
		if !tableOK {
			t.Fatal("the diagnostic projection did not form over a table carrying the probe's row")
		}
		entry, known := table.ForStaticObservation(probe.ObservationKey)
		if !known || entry.Code() != probe.Code {
			t.Fatalf("the probe's population resolves to %v, not to %q", entry, probe.Code)
		}
	})

	t.Run("W3_axis", func(t *testing.T) {
		// The axis is the writer principal. Its slot is its declaration position,
		// which is one past the analyzer's own inventory, and no enum names it.
		view, viewOK := sealed.Surface(schema.SurfaceKindAxis)
		if !viewOK {
			t.Fatal("sealed table carries no axis surface")
		}
		row, atOK := view.At(inventory.analyzerAxes)
		if !atOK || row.Key() != probe.AxisKey {
			t.Fatalf("the probe's axis did not seal at slot %d", inventory.analyzerAxes+1)
		}
		if count := inventory.probeAxis.OutputCount(); count != 1 {
			t.Fatalf("the probe's axis published %d columns, not one", count)
		}
		output, outputOK := inventory.probeAxis.OutputAt(0)
		if !outputOK || output.Key != probe.OutputKey || output.Writer != probe.AxisKey {
			t.Fatalf("the probe's published column is %v, and its writer is not the axis that declares it", output)
		}
		// A denominator is derived from the axis table, so a coordinate space
		// declared outside the analyzer carries its own closed world without the
		// derived surface being told about it.
		denominators, denominatorsOK := sealed.Surface(schema.SurfaceKindDenominator)
		if !denominatorsOK {
			t.Fatal("sealed table carries no denominator surface")
		}
		if _, resolved := denominators.ByID(schema.NewEntryID(schema.SurfaceKindDenominator, "coordinates/"+probe.AxisKey)); !resolved {
			t.Fatal("the probe's axis sealed without the closed world derived from it")
		}
		// The mount hook is the one executable half a declaration surface can run
		// without an engine binding, so the walk runs it: the probe seals its own
		// Link authority from the neutral artifact view, and rejects with its own
		// evidence when that view is empty.
		authority, _, mounted := inventory.probeAxis.Mount(LinkInputs{Artifacts: make([]programmount.MountedArtifact, 2)})
		sealedAuthority, authorityOK := axis.Payload[probe.MountAuthority](authority)
		if !mounted || !authorityOK || sealedAuthority.Artifacts != 2 {
			t.Fatalf("the probe's mount sealed %v over two mounted artifacts", sealedAuthority)
		}
		_, rejection, refused := inventory.probeAxis.Mount(LinkInputs{})
		evidence, evidenceOK := axis.Payload[probe.MountRejection](rejection)
		if refused || !evidenceOK || evidence.Artifacts != 0 {
			t.Fatalf("the probe's mount admitted an empty artifact view, or lost its own rejection evidence: %v", evidence)
		}
	})

	t.Run("W4_rule", func(t *testing.T) {
		// The role is the declaration position. The probe's rule seals at the slot
		// one past the analyzer's own inventory, writes the axis its own package
		// declares, and carries the subscription it declared.
		view, viewOK := sealed.Surface(schema.SurfaceKindRule)
		if !viewOK {
			t.Fatal("sealed table carries no rule surface")
		}
		row, atOK := view.At(inventory.analyzerRules)
		if !atOK || row.Key() != probe.RuleKey {
			t.Fatalf("the probe's rule did not seal at slot %d", inventory.analyzerRules+1)
		}
		if inventory.probeRule.Writes() != probe.AxisKey {
			t.Fatalf("the probe's rule writes %q", inventory.probeRule.Writes())
		}
		axes, axesOK := sealed.Surface(schema.SurfaceKindAxis)
		if !axesOK {
			t.Fatal("sealed table carries no axis surface")
		}
		if _, resolved := axes.ByID(schema.NewEntryID(schema.SurfaceKindAxis, inventory.probeRule.Writes())); !resolved {
			t.Fatal("the axis the probe's rule writes did not resolve against the sealed table")
		}
		if count := inventory.probeRule.IssuanceCount(); count != 1 {
			t.Fatalf("the probe's rule declared %d subscriptions, not one", count)
		}
	})

	t.Run("W5_roles", func(t *testing.T) {
		// Every role is a declared row, so a domain adds one by declaring it. The
		// identity each derives is the unchanged derivation over its own declared
		// spelling, and the spelling law over the one category is what proves the
		// probe's three distinct from every role the analyzer declares.
		for _, role := range [...]string{probe.FactorRole, probe.RuleRole, probe.OperandRole} {
			key := vocabulary.RoleKey(role)
			walkStructureMember(t, sealed, key, structure.CategorySemanticRole)
			resolved, resolvedOK := inventory.roles.Key(key)
			derived, derivedOK := vocabulary.Key(role)
			if !resolvedOK || !derivedOK || resolved != derived {
				t.Fatalf("role %q resolved to an identity the unchanged derivation does not produce", role)
			}
		}
		declared := make(map[identity.SemanticKey]string, len(inventory.structures))
		for _, member := range inventory.structures {
			if member.Category() != structure.CategorySemanticRole {
				continue
			}
			key, ok := inventory.roles.Key(member.Key())
			if !ok {
				t.Fatalf("declared role %q resolves no identity", member.Key())
			}
			if prior, duplicate := declared[key]; duplicate {
				t.Fatalf("roles %q and %q derive one identity", prior, member.Spelling())
			}
			declared[key] = member.Spelling()
		}
	})

	t.Run("W6_issuance", func(t *testing.T) {
		// The mapping from a compiled occurrence family to an issued rule is
		// declared data on the rule's own row: the family, the placement form, the
		// operand polarity, and the execution cut each name a member of the
		// vocabulary they belong to, and all three resolve against the sealed table.
		//
		// What this does not state is emission. Placing the issued occurrence on
		// the program's geometry is the artifact compiler's half, and it still
		// keys on its own role catalog, so the probe's rows are declared here and
		// emitted nowhere.
		issuance, issuanceOK := inventory.probeRule.IssuanceAt(0)
		if !issuanceOK {
			t.Fatal("the probe's declared subscription is not readable from its sealed row")
		}
		view, viewOK := sealed.Surface(schema.SurfaceKindIssuance)
		table, tableOK := issuanceschema.NewTable(view)
		if !viewOK || !tableOK {
			t.Fatal("the sealed issuance machine is unavailable")
		}
		if _, ok := table.Entry(issuance.Occurrence, issuanceschema.KindFamily); !ok {
			t.Fatal("the probe occurrence family is not a sealed predicate")
		}
		if _, ok := table.Entry(issuance.Form, issuanceschema.KindForm); !ok {
			t.Fatal("the probe form is not a sealed construction program")
		}
		if _, ok := table.Entry(issuance.Requirement, issuanceschema.KindRequirement); !ok {
			t.Fatal("the probe requirement is not a sealed admission program")
		}
	})
}

// TestWalkFamilyLawHoldsTheCodeToItsDeclaredFamily is the negative half of W1.
// The family law moved from an enum bound to a resolution, and a resolution that
// admitted anything would make the walk's positive step vacuous: a row whose code
// does not spell the family it names, and a row naming a family no one declared,
// are both rejected at seal.
func TestWalkFamilyLawHoldsTheCodeToItsDeclaredFamily(t *testing.T) {
	cases := []struct {
		name        string
		mutate      func(diagnostic.Spec) diagnostic.Spec
		disposition schema.Disposition
	}{
		{
			name: "code_does_not_spell_its_family",
			mutate: func(spec diagnostic.Spec) diagnostic.Spec {
				spec.Code = "other.example"
				return spec
			},
			disposition: schema.DispositionMalformed,
		},
		{
			name: "family_is_not_declared",
			mutate: func(spec diagnostic.Spec) diagnostic.Spec {
				spec.Family = diagnostic.Reference{Surface: schema.SurfaceKindStructure, Key: "family/undeclared"}
				return spec
			},
			disposition: schema.DispositionIncomplete,
		},
	}
	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			sealed, failure, _ := composeWalk(t, testcase.mutate(probe.DiagnosticSpec()))
			if sealed != nil || !failure.Available() {
				t.Fatal("the table sealed with a row that does not name the family it publishes under")
			}
			if failure.Contributor != schema.SurfaceKindDiagnostic || failure.Law != diagnostic.LawFamilyDeclared || failure.Disposition != testcase.disposition {
				t.Fatalf("rejected as contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
			}
		})
	}
}

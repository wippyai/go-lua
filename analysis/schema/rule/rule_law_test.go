package rule

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

type lawPrincipals struct{ present bool }

type lawAuthorities struct{ present bool }

type lawFragment struct{ id int }

type lawHot struct{ id int }

// lawSpec is one admissible declaration. Every law below starts from it and
// breaks exactly one field, so a rejection names the law it violated.
func lawSpec(key schema.Key, role programartifact.RuleRole, lane Lane, semantic func(vocabulary.Bundle) identity.SemanticKey) Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot] {
	spec := Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]{
		Key:      key,
		Role:     role,
		Lane:     lane,
		Semantic: semantic,
		Declare: func(Declaration[lawPrincipals]) (*lawFragment, bool) {
			return &lawFragment{id: int(role)}, true
		},
		Register: func(Registration[*lawFragment]) (engine.RuleSlotCapability, bool) {
			return engine.RuleSlotCapability{}, true
		},
		Bind: func(Binding[lawAuthorities, *lawFragment]) (*lawHot, bool) {
			return &lawHot{id: int(role)}, true
		},
	}
	if lane.Mounted() {
		spec.Attach = func(Attach[*lawHot]) bool { return true }
		spec.Member = func(Member[*lawHot]) bool { return true }
		return spec
	}
	spec.LinkAttach = func(LinkAttach[*lawHot]) bool { return true }
	spec.LinkMember = func(LinkMember[*lawHot]) bool { return true }
	spec.LinkCatalog = func(*lawHot) (LinkCatalog, bool) { return lawCatalog{}, true }
	return spec
}

type lawCatalog struct{}

func (lawCatalog) Count() int { return 0 }

func (lawCatalog) IDAt(int) (identity.ContentID, bool) { return identity.ContentID{}, false }

// lawTable declares the whole artifact role catalog in its canonical order, so
// a law can break exactly one row and keep every other law satisfied.
func lawTable(t *testing.T, mutate func(int, *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot])) []*Template[lawPrincipals, lawAuthorities] {
	t.Helper()
	semantics := []func(vocabulary.Bundle) identity.SemanticKey{
		func(b vocabulary.Bundle) identity.SemanticKey { return b.ValueSourceRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.PackSourceRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.HeapIngressRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.ValueAllocationRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.HeapEmptyRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.HeapClosedRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.RawGetRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.RawSetRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.CallDispatchRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.EffectSelectedRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.EffectOpaqueRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.EffectBodyRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.CallActivation },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.ValueBootstrapRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.HeapBootstrapRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.ValueTransferRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.ValueBinaryArithmeticRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.ValueBinaryEqualityRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.ValueBinaryOrderRule.Rule },
		func(b vocabulary.Bundle) identity.SemanticKey { return b.ValuePresenceRefinementRule.Rule },
	}
	keys := []schema.Key{
		"law-value-source", "law-pack-source", "law-heap-ingress", "law-value-allocation", "law-heap-empty",
		"law-heap-closed", "law-raw-get", "law-raw-set", "law-call-dispatch", "law-effect-selected",
		"law-effect-opaque", "law-effect-body", "law-call-activation", "law-value-bootstrap", "law-heap-bootstrap",
		"law-value-transfer", "law-value-binary-arithmetic", "law-value-binary-equality", "law-value-binary-order",
		"law-value-presence-refinement",
	}
	var templates []*Template[lawPrincipals, lawAuthorities]
	for position := range keys {
		role := programartifact.RuleRole(position + 1)
		lane := LaneMounted
		switch role {
		case programartifact.RuleRoleCallActivation:
			lane = LaneActivation
		case programartifact.RuleRoleValueBootstrap, programartifact.RuleRoleHeapBootstrap:
			lane = LaneLink
		}
		spec := lawSpec(keys[position], role, lane, semantics[position])
		if mutate != nil {
			mutate(position, &spec)
		}
		template, ok := New(spec)
		if !ok {
			// A spec a law deliberately broke is reported by the caller's own
			// admission check, not by silently dropping the row.
			continue
		}
		templates = append(templates, template)
	}
	return templates
}

// lawSiblingSurface stands in for one sibling surface. The declaration root
// admits surfaces in catalog order and requires every member to be registered,
// so a rule law is stated against a complete table rather than a half
// registered one. Standing the siblings in keeps this law blind to every other
// surface's own record.
type lawSiblingSurface struct{ kind schema.SurfaceKind }

type lawSiblingEntry struct{ key schema.Key }

func (entry lawSiblingEntry) Key() schema.Key { return entry.key }

func (entry lawSiblingEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry lawSiblingEntry) EntryContent(*framing.Writer) error { return nil }

func (surface lawSiblingSurface) Kind() schema.SurfaceKind { return surface.kind }

func (surface lawSiblingSurface) Entries() []schema.Entry {
	return []schema.Entry{lawSiblingEntry{key: "law-sibling"}}
}

func (lawSiblingSurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// sealLawTable seals one rule inventory into a complete declaration table. The
// catalog is walked rather than listed, so the surfaces the declaration root
// settles on do not change what these laws assert.
func sealLawTable(templates []*Template[lawPrincipals, lawAuthorities]) schema.SealFailure {
	_, failure := sealLawTableFor(templates)
	return failure
}

// sealLawTableFor is the same seal, read for the table it produces rather than
// for the verdict alone.
func sealLawTableFor(templates []*Template[lawPrincipals, lawAuthorities]) (*schema.Schema, schema.SealFailure) {
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind == schema.SurfaceKindRule {
			builder.Register(NewSurface(templates))
			continue
		}
		builder.Register(lawSiblingSurface{kind: kind})
	}
	return builder.Seal()
}

func TestRuleSurfaceAdmitsTheCompleteRoleCatalog(t *testing.T) {
	templates := lawTable(t, nil)
	if len(templates) != programartifact.MountedRuleRoleCount()+2 {
		t.Fatalf("law table admitted %d rules, want the complete role catalog", len(templates))
	}
	if failure := sealLawTable(templates); failure.Available() {
		t.Fatalf("complete role catalog rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
}

func TestRuleSurfaceRejectsAnIncompleteDeclaration(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(int, *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot])
	}{
		{"missing-key", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 0 {
				spec.Key = ""
			}
		}},
		{"missing-declare", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 0 {
				spec.Declare = nil
			}
		}},
		{"missing-bind", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 3 {
				spec.Bind = nil
			}
		}},
		{"missing-register", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 5 {
				spec.Register = nil
			}
		}},
		{"missing-semantic", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 6 {
				spec.Semantic = nil
			}
		}},
		{"mounted-without-attach", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 0 {
				spec.Attach = nil
			}
		}},
		{"mounted-with-link-lane-hooks", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 0 {
				spec.LinkAttach = func(LinkAttach[*lawHot]) bool { return true }
			}
		}},
		{"link-without-catalog", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if programartifact.RuleRole(position+1) == programartifact.RuleRoleValueBootstrap {
				spec.LinkCatalog = nil
			}
		}},
		{"invalid-lane", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 2 {
				spec.Lane = LaneInvalid
			}
		}},
		{"unowned-role", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 4 {
				spec.Role = programartifact.RuleRoleInvalid
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			templates := lawTable(t, test.mutate)
			if len(templates) == programartifact.MountedRuleRoleCount()+2 {
				t.Fatal("a deliberately incomplete declaration was admitted as a template")
			}
			if failure := sealLawTable(templates); !failure.Available() {
				t.Fatal("a table missing a rejected declaration sealed")
			}
		})
	}
}

func TestRuleSurfaceRejectsADriftedTable(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(int, *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot])
		law    schema.LawID
	}{
		{"role-out-of-order", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 0 {
				spec.Role = programartifact.RuleRoleRawGet
			}
		}, LawRoleOrdinal},
		{"absent-semantic", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 0 {
				spec.Semantic = func(vocabulary.Bundle) identity.SemanticKey { return identity.SemanticKey{} }
			}
		}, LawSemanticIdentity},
		{"duplicate-semantic", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 1 {
				spec.Semantic = func(b vocabulary.Bundle) identity.SemanticKey { return b.ValueSourceRule.Rule }
			}
		}, LawSemanticUnique},
		{"mounted-role-declared-on-the-link-lane", func(position int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
			if position == 0 {
				spec.Lane = LaneLink
				spec.Attach, spec.Member = nil, nil
				spec.LinkAttach = func(LinkAttach[*lawHot]) bool { return true }
				spec.LinkMember = func(LinkMember[*lawHot]) bool { return true }
				spec.LinkCatalog = func(*lawHot) (LinkCatalog, bool) { return lawCatalog{}, true }
			}
		}, LawMountedRoleLane},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			failure := sealLawTable(lawTable(t, test.mutate))
			if !failure.Available() {
				t.Fatal("a drifted table sealed")
			}
			if failure.Law != test.law {
				t.Fatalf("law = %d, want %d", failure.Law, test.law)
			}
			if failure.Contributor != schema.SurfaceKindRule {
				t.Fatalf("contributor = %d, want the rule surface", failure.Contributor)
			}
		})
	}
}

// lawForeignSurface contributes rows of a foreign record under this surface's
// own kind and states this surface's laws over them. It is how a contribution
// that is not a rule declaration reaches the rule surface through the public
// seal path.
type lawForeignSurface struct{}

func (lawForeignSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindRule }

func (lawForeignSurface) Entries() []schema.Entry {
	return []schema.Entry{lawSiblingEntry{key: "law-foreign"}}
}

func (lawForeignSurface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	return surface[lawPrincipals, lawAuthorities]{}.Seal(view, sealed)
}

// TestRuleSurfaceRejectsAForeignRow states that the rule surface reads rule
// declarations and nothing else. A row of another record type carries none of
// the declared data every rule law is stated over, so it is rejected as the
// wrong shape rather than read as a partially declared rule.
func TestRuleSurfaceRejectsAForeignRow(t *testing.T) {
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind == schema.SurfaceKindRule {
			builder.Register(lawForeignSurface{})
			continue
		}
		builder.Register(lawSiblingSurface{kind: kind})
	}
	sealed, failure := builder.Seal()
	if sealed != nil {
		t.Fatal("a foreign row was admitted into the rule surface")
	}
	if failure.Law != LawEntryShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign row rejected under law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindRule {
		t.Fatalf("shape verdict named surface %d, not the rule surface", failure.Contributor)
	}
}

func TestRuleSurfaceRejectsAnUncoveredMountedRole(t *testing.T) {
	complete := lawTable(t, nil)
	if len(complete) == 0 {
		t.Fatal("law table")
	}
	failure := sealLawTable(complete[:len(complete)-1])
	if !failure.Available() {
		t.Fatal("a table missing its last rule sealed")
	}
	// Dropping the final row leaves the presence-refinement mounted role with
	// no declaration at all.
	if failure.Law != LawMountedRoleCovered {
		t.Fatalf("law = %d, want the mounted-role coverage law", failure.Law)
	}
}

func TestRuleTemplateHandsBackOnlyItsOwnCell(t *testing.T) {
	templates := lawTable(t, nil)
	if len(templates) < 2 {
		t.Fatal("law table")
	}
	first, second := templates[0], templates[1]
	fragment, fragmentOK := first.Declare(Declaration[lawPrincipals]{Builder: engine.NewSchema(), Principals: lawPrincipals{present: true}})
	if !fragmentOK {
		t.Fatal("declare")
	}
	recovered, recoveredOK := Payload[*lawFragment](fragment)
	if !recoveredOK || recovered.id != int(first.Role()) {
		t.Fatal("a rule did not recover its own declared fragment")
	}
	// A cell is only ever handed back to the rule that produced it; a foreign
	// hot recovery must not succeed by structural coincidence.
	if _, foreign := Payload[*lawHot](fragment); foreign {
		t.Fatal("a fragment cell was recovered as a hot rule")
	}
	if second.Role() == first.Role() {
		t.Fatal("law table roles collided")
	}
}

// TestTableDigestCoversDeclaredContent is the drift law of this surface: the
// digest is what a derived inventory is checked against, so two catalogs that
// name the same rules and admit them on different lanes are two tables. The
// lane decides which admission path an occurrence takes, so moving one moves
// the digest.
func TestTableDigestCoversDeclaredContent(t *testing.T) {
	declared, failure := sealLawTableFor(lawTable(t, nil))
	if failure.Available() {
		t.Fatalf("complete role catalog rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	templates := lawTable(t, func(_ int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
		if spec.Role == programartifact.RuleRoleCallActivation {
			spec.Lane = LaneMounted
		}
	})
	shifted, failure := sealLawTableFor(templates)
	if failure.Available() {
		t.Fatalf("catalog with a shifted lane rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("a rule's declared admission lane left the table digest unchanged")
	}
}

// TestTableDigestCoversSemanticIdentity is the identity half of the same drift
// law. A rule's canonical identity is the role it selects from the closed
// vocabulary, and the engine slot it binds is resolved under that identity, so
// two catalogs whose rules occupy the same artifact roles and select different
// roles from the vocabulary are two tables. The two inventories below differ in
// one selected role and in nothing else.
func TestTableDigestCoversSemanticIdentity(t *testing.T) {
	declared, failure := sealLawTableFor(lawTable(t, nil))
	if failure.Available() {
		t.Fatalf("complete role catalog rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	templates := lawTable(t, func(_ int, spec *Spec[lawPrincipals, lawAuthorities, *lawFragment, *lawHot]) {
		if spec.Role == programartifact.RuleRoleValueSource {
			spec.Semantic = func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.ValueSourceRule.Operand }
		}
	})
	shifted, failure := sealLawTableFor(templates)
	if failure.Available() {
		t.Fatalf("catalog with a shifted semantic role rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("a rule's selected semantic role left the table digest unchanged")
	}
}

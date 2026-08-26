package rule

import (
	"bytes"
	"testing"

	seal "github.com/wippyai/go-lua/analysis/schema/seal"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	issuanceschema "github.com/wippyai/go-lua/analysis/schema/issuance"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// The law table's own vocabulary. A rule names the axis it writes and the
// members of the structural vocabulary its subscriptions are declared in, so
// the stand-in surfaces below declare exactly those and the laws resolve
// against a real table rather than against this package's own constants.
const (
	lawAxisKey       schema.Key = "law-axis"
	lawOccurrenceKey schema.Key = "occurrence/law-source"
	lawFormKey       schema.Key = "issuance/law-form"
	lawInputKey      schema.Key = "input/law-input"
	lawStageKey      schema.Key = "stage/law-stage"
	// lawRequirementKey is the operand shape the law table's subscriptions
	// consume: a member of the requirement vocabulary like any other, so a
	// declaration that omits it is refused rather than defaulted.
	lawRequirementKey schema.Key = "requirement/law-requirement"
	// lawNarrowRequirementKey is a second declared operand shape. Two members
	// is what it takes to state that the shape a subscription names is content
	// rather than a constant.
	lawNarrowRequirementKey schema.Key = "requirement/law-narrow-requirement"
)

// lawRuleCount is the size of the law table: the eighteen rules materialized
// from a compiled artifact plus the two Link-owned bootstrap rules.
const lawRuleCount = 20

// lawIssuance is one admissible subscription. Every term names a member the
// stand-in structural surface declares.
func lawIssuance() Issuance {
	return Issuance{Occurrence: lawOccurrenceKey, Form: lawFormKey, Requirement: lawRequirementKey}
}

type lawPrincipals struct{ present bool }

type lawAuthorities struct{}

type lawFragment struct{ id int }

type lawHot struct{ id int }

// lawSpec is one admissible declaration. Every law below starts from it and
// breaks exactly one field, so a rejection names the law it violated.
func lawSpec(key schema.Key, slot int, lane Lane, semantic schema.Key) Spec {
	spec := Spec{
		Key:      key,
		Lane:     lane,
		Writes:   lawAxisKey,
		Owner:    lawAxisKey,
		Semantic: semantic,
	}
	if lane.Mounted() {
		spec.Issues = []Issuance{lawIssuance()}
		return spec
	}
	return spec
}

type lawCatalog struct{}

func (lawCatalog) Count() int { return 0 }

func (lawCatalog) IDAt(int) (identity.ContentID, bool) { return identity.ContentID{}, false }

// lawSemanticKeys is the law table's role catalog: one declared semantic role
// per rule, in the table's own order. The stand-in structural surface declares
// exactly these rows, so a rule resolves its identity against a real
// vocabulary rather than against a constant of this file.
func lawSemanticKeys() []schema.Key {
	return []schema.Key{

		"semantic/rule/law/value-source", "semantic/rule/law/pack-source", "semantic/rule/law/heap-ingress",
		"semantic/rule/law/value-allocation", "semantic/rule/law/heap-empty", "semantic/rule/law/heap-closed",
		"semantic/rule/law/raw-get", "semantic/rule/law/raw-set", "semantic/rule/law/call-dispatch",
		"semantic/rule/law/effect-selected", "semantic/rule/law/effect-opaque", "semantic/rule/law/effect-body",
		"semantic/rule/law/call-activation", "semantic/rule/law/value-bootstrap", "semantic/rule/law/heap-bootstrap",
		"semantic/rule/law/value-transfer", "semantic/rule/law/value-binary-arithmetic",
		"semantic/rule/law/value-binary-equality", "semantic/rule/law/value-binary-order",
		"semantic/rule/law/value-presence-refinement",
	}
}

// lawRoles is the resolved semantic role vocabulary the law table's rules are
// declared against. A hook receives exactly the roles its own rule declared, so
// a declaration pass needs the vocabulary those references resolve in.
func lawRoles(t *testing.T) vocabulary.Roles {
	t.Helper()
	spellings := make([]string, 0, lawRuleCount)
	for _, key := range lawSemanticKeys() {
		spellings = append(spellings, string(key[len("semantic/"):]))
	}
	entries, entriesOK := structure.Collect(vocabulary.RoleSpecs(spellings...))
	roles, rolesOK := vocabulary.NewRoles(entries)
	if !entriesOK || !rolesOK {
		t.Fatal("law semantic role vocabulary")
	}
	return roles
}

// lawTable declares the whole artifact role catalog in its canonical order, so
// a law can break exactly one row and keep every other law satisfied.
func lawTable(t *testing.T, mutate func(int, *Spec)) []*Template {
	t.Helper()
	semantics := lawSemanticKeys()
	keys := []schema.Key{
		"law-value-source", "law-pack-source", "law-heap-ingress", "law-value-allocation", "law-heap-empty",
		"law-heap-closed", "law-raw-get", "law-raw-set", "law-call-dispatch", "law-effect-selected",
		"law-effect-opaque", "law-effect-body", "law-call-activation", "law-value-bootstrap", "law-heap-bootstrap",
		"law-value-transfer", "law-value-binary-arithmetic", "law-value-binary-equality", "law-value-binary-order",
		"law-value-presence-refinement",
	}
	var templates []*Template
	for position := range keys {
		slot := position + 1
		lane := LaneMounted
		switch keys[position] {
		case "law-call-activation":
			lane = LaneActivation
		case "law-value-bootstrap", "law-heap-bootstrap":
			lane = LaneLink
		}
		spec := lawSpec(keys[position], slot, lane, semantics[position])
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
type lawSiblingSurface struct {
	kind schema.SurfaceKind
	keys []schema.Key
}

// lawStructureSurface stands in for the structural vocabulary. Its rows are
// real vocabulary members, because a rule's subscriptions resolve against that
// surface's own record and a stand-in row would be a member of no vocabulary.
type lawStructureSurface struct{}

func (lawStructureSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindStructure }

func (lawStructureSurface) Entries() []schema.Entry {
	specs := []structure.Spec{}
	// The law table's rules are identified by declared roles, so the stand-in
	// carries one row per rule plus the operand form one law shifts a rule onto.
	roles := make([]string, 0, lawRuleCount+1)
	for _, key := range lawSemanticKeys() {
		roles = append(roles, string(key[len("semantic/"):]))
	}
	roles = append(roles, "operand/law/value-source")
	numbered, numberedOK := structure.Collect(specs, vocabulary.RoleSpecs(roles...))
	if !numberedOK {
		return nil
	}
	entries := make([]schema.Entry, 0, len(numbered))
	for _, entry := range numbered {
		entries = append(entries, entry)
	}
	return entries
}

// The stand-in states no law of its own: what this file is stating laws about
// is the rule surface, and the structural surface's own totality is that
// package's law.
func (lawStructureSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// lawSurfaceFor stands one sibling surface in. The axis surface carries the one
// axis the law table's rules write, and the structural surface carries the
// vocabulary their subscriptions name.
func lawSurfaceFor(kind schema.SurfaceKind) seal.Surface {
	switch kind {
	case schema.SurfaceKindStructure:
		return lawStructureSurface{}
	case schema.SurfaceKindAxis:
		return lawSiblingSurface{kind: kind, keys: []schema.Key{lawAxisKey}}
	case schema.SurfaceKindIssuance:
		return issuanceschema.NewSurface(lawIssuanceEntries())
	default:
		return lawSiblingSurface{kind: kind, keys: []schema.Key{"law-sibling"}}
	}
}

func lawIssuanceEntries() []*issuanceschema.Entry {
	specs := []issuanceschema.Spec{
		{Key: issuanceschema.TypePoint, Kind: issuanceschema.KindType, Ordinal: 1},
		{Key: issuanceschema.TypeEmission, Kind: issuanceschema.KindType, Ordinal: 2},
		{Key: issuanceschema.TypePointIdentity, Kind: issuanceschema.KindType, Ordinal: 3},
		{Key: issuanceschema.TypeRelationIndex, Kind: issuanceschema.KindType, Ordinal: 4},
		{Key: "row/law-occurrence", Kind: issuanceschema.KindRowSpace, Ordinal: 1},
		{Key: "row/law-geometry", Kind: issuanceschema.KindRowSpace, Ordinal: 2},
		{Key: "field/law-occurrence-id", Kind: issuanceschema.KindField, Ordinal: 1, Space: "row/law-occurrence", Type: issuanceschema.IdentityType(issuanceschema.TypePointIdentity), Cardinality: issuanceschema.CardinalityOne},
		{Key: "field/law-geometry-occurrence-id", Kind: issuanceschema.KindField, Ordinal: 2, Space: "row/law-geometry", Type: issuanceschema.IdentityType(issuanceschema.TypePointIdentity), Cardinality: issuanceschema.CardinalityOne},
		{Key: "field/law-geometry-position", Kind: issuanceschema.KindField, Ordinal: 3, Space: "row/law-geometry", Type: issuanceschema.UintType(issuanceschema.TypeRelationIndex), Cardinality: issuanceschema.CardinalityOne},
		{Key: "field/law-geometry-point", Kind: issuanceschema.KindField, Ordinal: 4, Space: "row/law-geometry", Type: issuanceschema.IdentityType(issuanceschema.TypePointIdentity), Cardinality: issuanceschema.CardinalityOne},
		{Key: "relation/law-geometry", Kind: issuanceschema.KindRelation, Ordinal: 1, Space: "row/law-occurrence", Target: "row/law-geometry", Cardinality: issuanceschema.CardinalityMany,
			Joins:   []issuanceschema.JoinField{{Source: "field/law-occurrence-id", Target: "field/law-geometry-occurrence-id", Missing: issuanceschema.JoinMissingNoEdge}},
			Program: issuanceschema.Program{{Op: issuanceschema.OpLiteral, Out: 1, Type: issuanceschema.BoolType(), Literal: 1}}, Result: 1},
		{Key: "output/law-occurrence", Kind: issuanceschema.KindOutput, Ordinal: 1,
			Type: issuanceschema.DataType{Value: issuanceschema.ValueRow, Space: "row/law-occurrence", Cardinality: issuanceschema.CardinalityOne}},
		{Key: lawOccurrenceKey, Kind: issuanceschema.KindFamily, Ordinal: 1, Space: "row/law-occurrence",
			Program: issuanceschema.Program{{Op: issuanceschema.OpLiteral, Out: 1, Type: issuanceschema.BoolType(), Literal: 1}}, Result: 1},
		{Key: lawRequirementKey, Kind: issuanceschema.KindRequirement, Ordinal: 1, Space: "row/law-occurrence",
			Program: issuanceschema.Program{{Op: issuanceschema.OpCurrent, Out: 1}, {Op: issuanceschema.OpLiteral, Out: 2, Type: issuanceschema.BoolType(), Literal: 1}}, Result: 2,
			Outputs: []issuanceschema.OutputBinding{{Output: "output/law-occurrence", Register: 1, Proof: 2}}},
		{Key: lawNarrowRequirementKey, Kind: issuanceschema.KindRequirement, Ordinal: 2, Space: "row/law-occurrence",
			Program: issuanceschema.Program{{Op: issuanceschema.OpCurrent, Out: 1}, {Op: issuanceschema.OpLiteral, Out: 2, Type: issuanceschema.BoolType(), Literal: 1}}, Result: 2,
			Outputs: []issuanceschema.OutputBinding{{Output: "output/law-occurrence", Register: 1, Proof: 2}}},
		{Key: lawInputKey, Kind: issuanceschema.KindInput, Ordinal: 1, Input: issuanceschema.InputFinish, InputSource: issuanceschema.InputSourceRelation, Source: "relation/law-geometry"},
		{Key: lawStageKey, Kind: issuanceschema.KindStage, Ordinal: 1, Constructor: issuanceschema.StageConstructorPassthrough,
			Parameters: []issuanceschema.DataType{{Value: issuanceschema.ValuePointRange, Name: issuanceschema.TypePoint, Cardinality: issuanceschema.CardinalityMany}}, Order: 1},
		{Key: lawFormKey, Kind: issuanceschema.KindForm, Ordinal: 1, Empty: issuanceschema.EmptyRefuse, Subject: "output/law-occurrence", Requires: []schema.Key{"output/law-occurrence"},
			Program: issuanceschema.Program{
				{Op: issuanceschema.OpSelection, Out: 1, Ref: "output/law-occurrence"},
				{Op: issuanceschema.OpFollow, Out: 2, Args: [6]uint16{1}, Ref: "relation/law-geometry"},
				{Op: issuanceschema.OpProjectPoints, Out: 3, Args: [6]uint16{2}, Ref: "field/law-geometry-point", Aux: "field/law-geometry-position"},
				{Op: issuanceschema.OpInput, Out: 4, Args: [6]uint16{3}, Ref: lawInputKey},
				{Op: issuanceschema.OpRequestStage, Out: 5, Args: [6]uint16{3, 4}, Ref: lawStageKey},
				{Op: issuanceschema.OpEmit, Out: 6, Args: [6]uint16{5}},
			}, Emissions: []uint16{6}},
	}
	entries := make([]*issuanceschema.Entry, 0, len(specs))
	for _, spec := range specs {
		entry, ok := issuanceschema.New(spec)
		if !ok {
			return nil
		}
		entries = append(entries, entry)
	}
	return entries
}

type lawSiblingEntry struct{ key schema.Key }

func (entry lawSiblingEntry) Key() schema.Key { return entry.key }

func (entry lawSiblingEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry lawSiblingEntry) EntryContent(*framing.Writer) error { return nil }

func (surface lawSiblingSurface) Kind() schema.SurfaceKind { return surface.kind }

func (surface lawSiblingSurface) Entries() []schema.Entry {
	entries := make([]schema.Entry, 0, len(surface.keys))
	for _, key := range surface.keys {
		entries = append(entries, lawSiblingEntry{key: key})
	}
	return entries
}

func (lawSiblingSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// sealLawTable seals one rule inventory into a complete declaration table. The
// catalog is walked rather than listed, so the surfaces the declaration root
// settles on do not change what these laws assert.
func sealLawTable(templates []*Template) schema.SealFailure {
	_, failure := sealLawTableFor(templates)
	return failure
}

// sealLawTableFor is the same seal, read for the table it produces rather than
// for the verdict alone.
func sealLawTableFor(templates []*Template) (*seal.Schema, schema.SealFailure) {
	builder := seal.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind == schema.SurfaceKindRule {
			builder.Register(NewSurface(templates))
			continue
		}
		builder.Register(lawSurfaceFor(kind))
	}
	return builder.Seal()
}

func TestRuleSurfaceAdmitsTheCompleteRoleCatalog(t *testing.T) {
	templates := lawTable(t, nil)
	if len(templates) != lawRuleCount {
		t.Fatalf("law table admitted %d rules, want the complete role catalog", len(templates))
	}
	if failure := sealLawTable(templates); failure.Available() {
		t.Fatalf("complete role catalog rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
}

func TestRuleSurfaceRejectsAnIncompleteDeclaration(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(int, *Spec)
	}{
		{"missing-key", func(position int, spec *Spec) {
			if position == 0 {
				spec.Key = ""
			}
		}},
		{"missing-semantic", func(position int, spec *Spec) {
			if position == 6 {
				spec.Semantic = ""
			}
		}},
		{"invalid-lane", func(position int, spec *Spec) {
			if position == 2 {
				spec.Lane = LaneInvalid
			}
		}},
		{"unwritten-axis", func(position int, spec *Spec) {
			if position == 4 {
				spec.Writes = ""
			}
		}},
		{"missing-owner", func(position int, spec *Spec) {
			if position == 4 {
				spec.Owner = ""
			}
		}},
		{"incomplete-issuance", func(position int, spec *Spec) {
			if position == 7 {
				spec.Issues = []Issuance{{Occurrence: lawOccurrenceKey, Form: lawFormKey}}
			}
		}},
		// A subscription that states no operand shape is the silence the
		// declared-admissibility column exists to remove: it would place the
		// rule on every row of its family while its owner seals a subset.
		{"issuance-without-a-requirement", func(position int, spec *Spec) {
			if position == 7 {
				issuance := lawIssuance()
				issuance.Requirement = ""
				spec.Issues = []Issuance{issuance}
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// A broken declaration yields no template at all, so the row it
			// would have occupied is missing from the inventory. Which rows a
			// complete catalog must hold is the composition's own law, stated
			// where the catalog is composed.
			templates := lawTable(t, test.mutate)
			if len(templates) == lawRuleCount {
				t.Fatal("a deliberately incomplete declaration was admitted as a template")
			}
		})
	}
}

func TestRuleSurfaceRejectsADriftedTable(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(int, *Spec)
		law    schema.LawID
	}{
		{"writes-an-undeclared-axis", func(position int, spec *Spec) {
			if position == 0 {
				spec.Writes = "law-no-such-axis"
			}
		}, LawWritesResolves},
		{"owner-an-undeclared-axis", func(position int, spec *Spec) {
			if position == 0 {
				spec.Owner = "law-no-such-owner"
			}
		}, LawOwnerResolves},
		{"subscribes-to-an-undeclared-occurrence", func(position int, spec *Spec) {
			if position == 0 {
				issuance := lawIssuance()
				issuance.Occurrence = "occurrence/law-no-such-family"
				spec.Issues = []Issuance{issuance}
			}
		}, LawIssuanceResolves},
		{"subscribes-through-the-wrong-vocabulary", func(position int, spec *Spec) {
			if position == 0 {
				issuance := lawIssuance()
				issuance.Form = lawStageKey
				spec.Issues = []Issuance{issuance}
			}
		}, LawIssuanceResolves},
		{"requires-an-undeclared-operand-shape", func(position int, spec *Spec) {
			if position == 0 {
				issuance := lawIssuance()
				issuance.Requirement = "requirement/law-no-such-shape"
				spec.Issues = []Issuance{issuance}
			}
		}, LawIssuanceRequirementResolves},
		{"requires-a-shape-from-the-wrong-vocabulary", func(position int, spec *Spec) {
			if position == 0 {
				issuance := lawIssuance()
				issuance.Requirement = lawStageKey
				spec.Issues = []Issuance{issuance}
			}
		}, LawIssuanceRequirementResolves},
		{"mounted-rule-subscribing-to-nothing", func(position int, spec *Spec) {
			if position == 0 {
				spec.Issues = nil
			}
		}, LawIssuanceDeclared},
		{"absent-semantic", func(position int, spec *Spec) {
			if position == 0 {
				spec.Semantic = "semantic/rule/law/absent"
			}
		}, LawSemanticIdentity},
		{"duplicate-semantic", func(position int, spec *Spec) {
			if position == 1 {
				spec.Semantic = "semantic/rule/law/value-source"
			}
		}, LawSemanticUnique},
		{"mounted-role-declared-on-the-link-lane", func(position int, spec *Spec) {
			if position == 0 {
				spec.Lane = LaneLink
			}
		}, LawIssuanceLane},
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

func (lawForeignSurface) Seal(view seal.View, sealed seal.Sealed) schema.SealFailure {
	return surface{}.Seal(view, sealed)
}

// TestRuleSurfaceRejectsAForeignRow states that the rule surface reads rule
// declarations and nothing else. A row of another record type carries none of
// the declared data every rule law is stated over, so it is rejected as the
// wrong shape rather than read as a partially declared rule.
func TestRuleSurfaceRejectsAForeignRow(t *testing.T) {
	builder := seal.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind == schema.SurfaceKindRule {
			builder.Register(lawForeignSurface{})
			continue
		}
		builder.Register(lawSurfaceFor(kind))
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

// TestRuleSurfaceRejectsAMountedRuleThatSubscribesToNothing states the lane's
// reachability law from the other side: a rule admitted from a compiled
// artifact is reached by the occurrence families it subscribes to, so one that
// subscribes to none would sit on that lane unreachable.
func TestRuleSurfaceRejectsAMountedRuleThatSubscribesToNothing(t *testing.T) {
	failure := sealLawTable(lawTable(t, func(position int, spec *Spec) {
		if position == 12 {
			spec.Issues = nil
		}
	}))
	if !failure.Available() {
		t.Fatal("a mounted rule subscribing to nothing sealed")
	}
	if failure.Law != LawIssuanceDeclared {
		t.Fatalf("law = %d, want the issuance coverage law", failure.Law)
	}
}

func ruleEntryContent(t *testing.T, template *Template) string {
	t.Helper()
	var sink bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&sink, "analysis/rule-entry-content-law/v1", 1); err != nil {
		t.Fatal(err)
	}
	if err := template.EntryContent(&writer); err != nil {
		t.Fatal(err)
	}
	return sink.String()
}

func TestRuleOwnerIsDeclarationContent(t *testing.T) {
	left, leftOK := New(lawSpec("law-owner-left", 1, LaneMounted, "semantic/rule/law/value-source"))
	rightSpec := lawSpec("law-owner-right", 1, LaneMounted, "semantic/rule/law/value-source")
	rightSpec.Key = "law-owner-left"
	rightSpec.Owner = "law-other-axis"
	right, rightOK := New(rightSpec)
	if !leftOK || !rightOK {
		t.Fatal("owner content templates")
	}
	if ruleEntryContent(t, left) == ruleEntryContent(t, right) {
		t.Fatal("owner is not declaration content")
	}
	if left.Owner() != lawAxisKey || right.Owner() != "law-other-axis" {
		t.Fatalf("owner = %q / %q", left.Owner(), right.Owner())
	}
}

func TestRuleTemplateHandsBackOnlyItsOwnCell(t *testing.T) {
	templates := lawTable(t, nil)
	if len(templates) < 2 {
		t.Fatal("law table")
	}
	first, second := templates[0], templates[1]
	fragment := NewCell(&lawFragment{id: 1})
	recovered, recoveredOK := Payload[*lawFragment](fragment)
	if !recoveredOK || recovered.id != 1 {
		t.Fatal("a rule did not recover its own declared fragment")
	}
	// A cell is only ever handed back to the rule that produced it; a foreign
	// hot recovery must not succeed by structural coincidence.
	if _, foreign := Payload[*lawHot](fragment); foreign {
		t.Fatal("a fragment cell was recovered as a hot rule")
	}
	if second.Key() == first.Key() {
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
	templates := lawTable(t, func(_ int, spec *Spec) {
		if spec.Key == "law-call-activation" {
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

// TestTableDigestCoversDeclaredOperandShape states that the operand shape a
// subscription requires is declaration content. The shape decides which
// compiled rows issue the rule at all, so two catalogs that subscribe the same
// rules to the same families under different shapes place different programs
// and are two tables.
func TestTableDigestCoversDeclaredOperandShape(t *testing.T) {
	declared, failure := sealLawTableFor(lawTable(t, nil))
	if failure.Available() {
		t.Fatalf("complete role catalog rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	templates := lawTable(t, func(_ int, spec *Spec) {
		if spec.Key != "law-value-source" {
			return
		}
		issuance := lawIssuance()
		issuance.Requirement = lawNarrowRequirementKey
		spec.Issues = []Issuance{issuance}
	})
	shifted, failure := sealLawTableFor(templates)
	if failure.Available() {
		t.Fatalf("catalog with a narrowed operand shape rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("a subscription's declared operand shape left the table digest unchanged")
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
	templates := lawTable(t, func(_ int, spec *Spec) {
		if spec.Key == "law-value-source" {
			spec.Semantic = "semantic/operand/law/value-source"
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

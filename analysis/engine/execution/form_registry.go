// form_registry.go declares the append-only execution form table. One sealed
// form ordinal names one execution form; each form owns a child file that
// registers its classifier and its typed family builder. No caller switches on
// form shape: the descriptor is classified once here, and families are built in
// sealed ordinal order so a Program's family ladder never depends on the order
// its rows were discovered.

package execution

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// Form is the sealed execution form ordinal one plan row carries. The table is
// append-only: an ordinal is never reused, renumbered, or reordered, so a
// Program sealed against one revision keeps its ladder under the next.
type Form uint8

const (
	// FormExact is the exact-product form: one or more exact reads folded onto
	// one exact write. The built-in implementation owns the one-read identity
	// case; typed reducers claim wider products through RuleFamilies.
	FormExact Form = iota + 1
	// FormSource is the read-free materialized source column write.
	FormSource
	// FormSummary is the Summary and Complete read axis: one partition row's
	// whole declared cell vector, delivered under a mandatory read contract.
	FormSummary
	// FormCarry is the exact read and write whose carried prior fact passes
	// through an owner-issued transform.
	FormCarry
	// FormSelectedRoute is the ordered join with exactly one selected read and
	// the bounded routed write that publishes over it.
	FormSelectedRoute
	// FormActivation is the structural activation form: one exact trigger read
	// joined to one selected candidate-branch read, published through a
	// structural output rather than an ordinary factor write. It computes no
	// fact - the row is an authentication receipt (execution.ActivationRow),
	// never a reduced value - so it has no generic builder: every rule this
	// form classifies is authored through its own RuleFamilyInstaller.
	FormActivation
	// FormSelectedExact is the exact publication over a selection: one exact
	// read joined to one or more dependent selected reads, folded ONCE over
	// the whole delivery, published at the row's own exact coordinate.
	//
	// It is not FormSelectedRoute, which publishes one fact per observed
	// member; a rule of this form concludes one fact FROM every member, so the
	// selection reaches its fold as one argument rather than as a cadence. It
	// is not FormExact either, whose reads are every one of them exact. Like
	// FormActivation it has no generic builder: the members a selection is
	// observed at come from a relation only the rule's own package derives, so
	// every rule this form classifies is authored through its own
	// RuleFamilyInstaller.
	FormSelectedExact
	// formCount is the exclusive upper bound of the declared ordinals. It is
	// the last constant in the block; a new form is appended above it.
	formCount
)

// formNames is the name column of the form table. Every declared ordinal has a
// name, whether or not it has an implementation, so a plan whose form is not
// executable refuses by that name instead of by a bare ordinal.
var formNames = [formCount]string{
	FormExact:         "exact",
	FormSource:        "source",
	FormSummary:       "summary",
	FormCarry:         "carry",
	FormSelectedRoute: "selected-route",
	FormActivation:    "activation",
	FormSelectedExact: "selected-exact",
}

// Declared reports whether form names a sealed ordinal of the table.
func (form Form) Declared() bool { return form > 0 && form < formCount }

// Name is the sealed name of a declared form and the empty string otherwise.
func (form Form) Name() string {
	if !form.Declared() {
		return ""
	}
	return formNames[form]
}

// FormRow is one plan row handed to its form. Form, Input, and Relation are
// classified from the sealed descriptor; Member, Unit, and Target are the
// occurrence coordinates the bound Factor minted for that row; Rule is the
// descriptor the row was classified from, so a form whose geometry is wider
// than one read and one write mints its own coordinates from the plan it was
// already classified against instead of classifying a second time. A row
// carries no owner capability and no domain value.
type FormRow struct {
	Member int
	Form   Form
	Input  uint16
	// Candidate is the row's dense position in the candidate relation its rule
	// draws from. A candidate-indexed transform - the transition a transformed
	// carry applies - is a different map at every row, so the row that carries
	// it has to say which candidate it is. It is the same coordinate the
	// invocation is later authenticated against.
	Candidate uint32
	// Source is the row-local Program capability a rule whose candidates are
	// Program rows carries. It holds the mounted publication the Candidate
	// ordinal addresses, so a family spanning several mounted Programs never
	// reads one mount's ordinal against another's rows. Every other rule
	// carries the zero value and resolves its candidate through its axis owner.
	Source   ProgramSource
	Relation uint32
	Unit     carrier.Unit
	Target   carrier.Target
	Rule     generated.CompiledRule
	// RuleOrdinal is this row's typed foreign key into the sealed rule table:
	// which rule Rule is a descriptor of. It is set by the engine from the
	// member row that produced this plan row, because the descriptor itself
	// does not carry its own coordinate - a row's position in a table is the
	// table's answer, not the row's.
	RuleOrdinal uint32
	exact       []carrier.Unit
	members     [][]uint32
}

// BindExact attaches the owner-issued Unit of one exact join to this sealed
// row. It is called only by the engine while lowering the already-issued read
// surfaces; rule packages can consume the result but cannot mint one.
func (row FormRow) BindExact(join int, unit carrier.Unit) (FormRow, bool) {
	if !row.Rule.Available() || join < 0 || join >= row.Rule.ReadCount() || unit.Kind() != carrier.ExactUnit {
		return FormRow{}, false
	}
	plan, ok := row.Rule.ReadAt(join)
	if !ok || plan.Form != ruleprogram.Exact || plan.Input > uint32(^uint16(0)) {
		return FormRow{}, false
	}
	firstExact := -1
	for index := 0; index < row.Rule.ReadCount(); index++ {
		read, readOK := row.Rule.ReadAt(index)
		if !readOK {
			return FormRow{}, false
		}
		if read.Form == ruleprogram.Exact {
			firstExact = index
			break
		}
	}
	if join == firstExact && row.Unit != (carrier.Unit{}) && row.Unit != unit {
		return FormRow{}, false
	}
	if row.exact == nil {
		row.exact = make([]carrier.Unit, row.Rule.ReadCount())
	} else {
		row.exact = append([]carrier.Unit(nil), row.exact...)
	}
	if row.exact[join] != (carrier.Unit{}) {
		return FormRow{}, false
	}
	row.exact[join] = unit
	return row, true
}

// BindMembers attaches one join's ordered nested member set to this sealed
// row: the axis-local coordinate of every member, in the owner's own member
// order, as the engine enumerated it at the row the read is addressed by.
//
// It is called only by the engine while lowering an already-resolved read. A
// rule package consumes the result and cannot mint one, which is the whole
// point: the set is the owner's answer under one parent row, and a family that
// could supply its own would be enumerating a directory it may not own.
func (row FormRow) BindMembers(join int, coordinates []uint32) (FormRow, bool) {
	if !row.Rule.Available() || join < 0 || join >= row.Rule.ReadCount() || coordinates == nil {
		return FormRow{}, false
	}
	plan, ok := row.Rule.ReadAt(join)
	if !ok || !plan.ParentPresent || plan.Form != ruleprogram.Summary && plan.Form != ruleprogram.Complete {
		return FormRow{}, false
	}
	if row.members == nil {
		row.members = make([][]uint32, row.Rule.ReadCount())
	} else {
		row.members = append([][]uint32(nil), row.members...)
	}
	if row.members[join] != nil {
		return FormRow{}, false
	}
	// An EMPTY set is copied as an empty slice, never as nil: a parent row with
	// no members is a set of width zero, and a join that spans no set at all
	// has none. Collapsing the two would report "this rule declares no member
	// set here" for a call that simply has no actuals.
	set := make([]uint32, len(coordinates))
	copy(set, coordinates)
	row.members[join] = set
	return row, true
}

// MemberCount is the sealed width of one join's nested member set. A join that
// spans no member set has none, which is not a set of width zero: an empty set
// is a parent row with no members, and both are distinguishable here.
func (row FormRow) MemberCount(join int) (int, bool) {
	if !row.Rule.Available() || join < 0 || join >= len(row.members) || row.members[join] == nil {
		return 0, false
	}
	return len(row.members[join]), true
}

// MemberAt is one member's axis-local coordinate, at the ordinal its owner
// declared it at.
func (row FormRow) MemberAt(join, index int) (uint32, bool) {
	if !row.Rule.Available() || join < 0 || join >= len(row.members) || row.members[join] == nil {
		return 0, false
	}
	set := row.members[join]
	if index < 0 || index >= len(set) {
		return 0, false
	}
	return set[index], true
}

func (row FormRow) exactAt(join int) (generated.ReadPlan, carrier.Unit, bool) {
	if !row.Rule.Available() || join < 0 || join >= len(row.exact) {
		return generated.ReadPlan{}, carrier.Unit{}, false
	}
	plan, ok := row.Rule.ReadAt(join)
	unit := row.exact[join]
	if !ok || plan.Form != ruleprogram.Exact || plan.Input > uint32(^uint16(0)) || unit.Kind() != carrier.ExactUnit {
		return generated.ReadPlan{}, carrier.Unit{}, false
	}
	return plan, unit, true
}

// FormAddress is where one plan row's invocation landed: the family offset
// within this Factor's returned ladder and the row's local ordinal inside that
// family.
type FormAddress struct {
	Member       int
	FamilyOffset uint32
	Local        uint32
}

// RuleFamilyInstaller authors the execution family of ONE sealed rule ordinal
// with concrete types. It is how a rule the engine cannot type reaches
// execution: a rule whose joins or whose reducer are typed in more than one
// Factor has no generic builder, because the engine may not name those types
// and the rule may not see the solve loop.
//
// The implementor is the rule's own package. A rule's family is the rule's
// knowledge - the schemas, contracts and derived plans its fold needs are
// sealed into it at the rule's own bind, where they are all in scope - and not
// the knowledge of the axis it happens to write to. An axis owner constructed
// from its own schema alone could not supply them, and must not acquire
// foreign schemas in order to.
type RuleFamilyInstaller[K scalar.Key, V any] interface {
	// InstallRuleFamily seals one family covering every plan row of that rule,
	// in the order given, and answers the local ordinal each row was sealed at.
	// Every primitive it needs is sealed through the plane, so an installer
	// outside this package builds a family without naming - or reaching - one
	// carrier, binding, or guard type.
	InstallRuleFamily(plane FormPlane[K, V], rule uint32, rows []FormRow) (Family, []FormAddress, bool)
}

// ExactDestinationProjector is the construction-only half of an authored
// heterogeneous exact family.  Such a rule takes its candidate from one axis
// but writes a key of another, so neither axis owner can project the row by
// itself: the candidate owner cannot normalize the output key, and the output
// owner does not own the candidate directory.  The generated installer is the
// one sealed object that holds both typed schemas and the declared accessor.
//
// Program construction asks this surface once per candidate, before any solve
// exists.  The returned local is then authenticated by the output Factor when
// it mints the strong target.  It is deliberately separate from execution and
// carries no Factor binding, target, callback, or runtime state.
type ExactDestinationProjector interface {
	ProjectExactDestination(candidate uint32) (uint64, bool)
}

// RuleFamilies is the sealed table of which installer authors which sealed
// rule ordinal. Membership in the table IS authorship: there is no separate
// predicate an installer could answer inconsistently with the table, and no
// order in which two claimants are resolved, because a second claim on one
// ordinal is refused when it is made.
//
// The table partitions before anything is built, so a refusal from an
// installer is a real refusal rather than a silent fall back to a generic form
// builder.
// It is a DENSE slice over the sealed rule table, not a map keyed by an
// ordinal: the ordinal is a position in that table, so the table it indexes is
// the same length as the one that assigned it, and an ordinal past the end is
// not an unclaimed rule but a coordinate of no table at all. Absence stays
// sparse - a nil entry is a rule with no installer, which takes the generic
// builder for its form.
type RuleFamilies[K scalar.Key, V any] struct {
	installers []RuleFamilyInstaller[K, V]
}

// NewRuleFamilies opens the claim table for one bound Factor over the sealed
// rule table's own width. Every claim is made against a position of that
// table, so the width is taken from it once here rather than grown by whoever
// claims last.
func NewRuleFamilies[K scalar.Key, V any](rules int) (*RuleFamilies[K, V], bool) {
	if rules < 0 || uint64(rules) > uint64(^uint32(0)) {
		return nil, false
	}
	return &RuleFamilies[K, V]{installers: make([]RuleFamilyInstaller[K, V], rules)}, true
}

// Install claims one sealed rule ordinal for one installer. A second claim on
// one ordinal is two authorities for one family and is refused, and so is a
// claim against an ordinal the sealed rule table does not have.
func (families *RuleFamilies[K, V]) Install(rule uint32, installer RuleFamilyInstaller[K, V]) bool {
	if families == nil || installer == nil || uint64(rule) >= uint64(len(families.installers)) {
		return false
	}
	if families.installers[rule] != nil {
		return false
	}
	families.installers[rule] = installer
	return true
}

// Installer resolves the installer authoring one sealed rule ordinal.
func (families *RuleFamilies[K, V]) Installer(rule uint32) (RuleFamilyInstaller[K, V], bool) {
	if families == nil || uint64(rule) >= uint64(len(families.installers)) {
		return nil, false
	}
	installer := families.installers[rule]
	return installer, installer != nil
}

// Count is the number of sealed rule ordinals this table authors. It is the
// number of CLAIMS, not the width of the rule table the claims index.
func (families *RuleFamilies[K, V]) Count() int {
	if families == nil {
		return 0
	}
	claimed := 0
	for _, installer := range families.installers {
		if installer != nil {
			claimed++
		}
	}
	return claimed
}

// ForeignFactor is one input axis's read side, delivered to a rule's installer
// with its key and fact types erased. A rule that reads a Factor it does not
// write to is exactly the rule the engine cannot type generically: the read
// fact is not the written fact, so the plane cannot seal that read against its
// own binding. The rule's own package knows both types and recovers the typed
// read through ForeignExactRead.
//
// The handle is a value, not a capability: it carries a bound Factor's fact
// binding and nothing that could reopen a schema, an owner, or a solve.
type ForeignFactor interface{ sealedForeignFactor() }

type foreignFactor[K scalar.Key, V any] struct {
	binding *factbinding.Binding[K, V]
	routes  RouteTable
}

func (foreignFactor[K, V]) sealedForeignFactor() {}

// NewForeignFactor seals one bound Factor's read side for delivery to the
// installers of rules that declared it as an input axis. The selection
// geometry travels with it because a dependent join on a foreign axis observes
// members of THAT axis: a rule that could not name the foreign coordinates
// would have nothing to observe its own selection at.
func NewForeignFactor[K scalar.Key, V any](binding *factbinding.Binding[K, V], routes RouteTable) (ForeignFactor, bool) {
	if binding == nil {
		return nil, false
	}
	return foreignFactor[K, V]{binding: binding, routes: routes}, true
}

// ForeignSelectedMember resolves one dense coordinate of a foreign input axis
// into the observation half of a member, at that axis's own types.
//
// It is the selection sibling of ForeignExactRead and it carries the same
// fence: the caller states the types because it is the only party that knows
// them, and a handle typed otherwise is refused rather than reinterpreted. No
// destination is ever resolved here - a rule publishes into the Factor it
// writes, never into one it merely joins.
func ForeignSelectedMember[K scalar.Key, V any](foreign ForeignFactor, dense uint32, tag uint64) (RouteMember, bool) {
	typed, ok := foreign.(foreignFactor[K, V])
	if !ok || typed.binding == nil {
		return RouteMember{}, false
	}
	member, memberOK := typed.routes.selectedMember(dense, tag)
	if !memberOK || !typed.binding.ValidUnit(member.coordinate.Unit) {
		return RouteMember{}, false
	}
	return member, true
}

// ForeignSelectionWidth is the dense extent of a foreign axis's coordinate
// universe, at that axis's own types.
func ForeignSelectionWidth[K scalar.Key, V any](foreign ForeignFactor) int {
	typed, ok := foreign.(foreignFactor[K, V])
	if !ok {
		return 0
	}
	return typed.routes.Width()
}

// ForeignExactRead seals one exact read of a foreign input axis at the read
// fact's own types. It is the read half of the heterogeneous fold: the write
// side stays sealed against the writing Factor's plane, and the two halves
// never have to agree on a type.
//
// The caller states the read types because it is the only party that knows
// them. A handle typed otherwise is refused rather than reinterpreted.
func ForeignExactRead[K scalar.Key, V any](foreign ForeignFactor, unit carrier.Unit, input uint16) (ExactRead[K, V], bool) {
	typed, ok := foreign.(foreignFactor[K, V])
	if !ok {
		return ExactRead[K, V]{}, false
	}
	return NewExactRead(typed.binding, unit, input)
}

// ForeignMemberExactRead seals one exact read of a foreign input axis at a
// dense coordinate the caller already resolved through that axis's own
// nested-member directory - MemberAt/Project - rather than through a route
// table. It is the third read primitive beside ForeignExactRead (one Unit the
// caller already holds) and ForeignSelectedMember (a coordinate resolved
// through a foreign RouteTable's selection geometry): a self-provided nested
// member set names its own members through its owner's directory and has no
// route table to select through, because it publishes no route and observes
// no selection - it is an ordinary exact coordinate the caller has already
// authenticated by construction.
//
// The caller states the read types because it is the only party that knows
// them. A handle typed otherwise, or a dense coordinate outside this axis's
// declared exact universe, is refused rather than reinterpreted or widened.
func ForeignMemberExactRead[K scalar.Key, V any](foreign ForeignFactor, dense uint32, input uint16) (ExactRead[K, V], bool) {
	typed, ok := foreign.(foreignFactor[K, V])
	if !ok || typed.binding == nil || uint64(dense) >= uint64(len(typed.routes.units)) {
		return ExactRead[K, V]{}, false
	}
	unit := typed.routes.units[int(dense)]
	if unit == (carrier.Unit{}) || unit.Kind() != carrier.ExactUnit {
		return ExactRead[K, V]{}, false
	}
	return NewExactRead(typed.binding, unit, input)
}

// ForeignSelectedRead seals one selected read of a foreign input axis at the
// read fact's own types. It is the selection sibling of ForeignExactRead: a
// family whose route join is its own Factor still reaches a dependent
// SELECTION of another axis, and without this it would have to erase that
// axis's coordinate/fact pair to observe the join it declared.
//
// The members are supplied by the caller, as they are for a plane-local
// selected read; what this seals is the typed read boundary, not the
// selection. A handle typed otherwise is refused rather than reinterpreted.
//
// The sealed policy is declaredCellPolicy's derivation over contract and
// binding; the policy argument is not read.
func ForeignSelectedRead[K scalar.Key, V any](foreign ForeignFactor, port uint16, contract ruleplan.ReadContract, _ ReadCellPolicy[V]) (SelectedRead[K, V], bool) {
	typed, ok := foreign.(foreignFactor[K, V])
	if !ok {
		return SelectedRead[K, V]{}, false
	}
	return NewSelectedRead(typed.binding, port, contract, ReadCellPolicy[V]{})
}

// ForeignRowExactRead seals one declared exact join against the foreign
// Factor it names. The Unit and input port come from the sealed FormRow; a
// family chooses only the join ordinal authored by its Program.
func ForeignRowExactRead[K scalar.Key, V any](foreign ForeignFactor, row FormRow, join int) (ExactRead[K, V], bool) {
	plan, unit, ok := row.exactAt(join)
	if !ok {
		return ExactRead[K, V]{}, false
	}
	return ForeignExactRead[K, V](foreign, unit, uint16(plan.Input))
}

// FormPlane is the sealed typed plane a form builder may read: the Factor's
// fact binding, its materialized source columns, the input axes its rows
// declared reads against, and the owner that installs families for its own
// rules. It is a value handoff, not an owner capability - a builder can seal
// descriptors from it and nothing else.
type FormPlane[K scalar.Key, V any] struct {
	binding  *factbinding.Binding[K, V]
	columns  []memberrelation.SourceColumn[V]
	present  []bool
	routes   RouteTable
	routed   bool
	selects  bool
	foreign  []ForeignFactor
	selected []ForeignFactor
	families *RuleFamilies[K, V]
}

// NewFormPlane seals one bound Factor's typed plane for the form table. A
// Factor that installs no family of its own passes a nil provider. foreign is
// the whole Program's Factor read table, indexed by sealed Factor ordinal; a
// rule's installer sees only the entries that rule's own joins declared, and
// routes is that Factor's own dense route geometry, which only a rule that
// declared a routed publication may address.
func NewFormPlane[K scalar.Key, V any](binding *factbinding.Binding[K, V], columns []memberrelation.SourceColumn[V], present []bool, routes RouteTable, foreign []ForeignFactor, families *RuleFamilies[K, V]) (FormPlane[K, V], bool) {
	if binding == nil {
		return FormPlane[K, V]{}, false
	}
	return FormPlane[K, V]{binding: binding, columns: columns, present: present, routes: routes, foreign: foreign, families: families}, true
}

// Foreign resolves the read side of one input axis this plane's rule declared
// a join against. A Factor no join of this rule named has no entry: a fold may
// read exactly the axes its own sealed plan depends on, so a read the plan
// does not account for cannot be sealed at all.
func (plane FormPlane[K, V]) Foreign(factor uint32) (ForeignFactor, bool) {
	if !plane.Valid() || uint64(factor) >= uint64(len(plane.foreign)) || plane.foreign[factor] == nil {
		return nil, false
	}
	return plane.foreign[factor], true
}

// ForeignSelection resolves the same handle for an axis this rule declared a
// DEPENDENT join against. It is narrower than Foreign on purpose: resolving
// members of an axis is what a selection does, and a rule that reads one
// coordinate of an axis has no member set of it to enumerate.
func (plane FormPlane[K, V]) ForeignSelection(factor uint32) (ForeignFactor, bool) {
	if !plane.Valid() || uint64(factor) >= uint64(len(plane.selected)) || plane.selected[factor] == nil {
		return nil, false
	}
	return plane.selected[factor], true
}

// forRule narrows this plane's foreign table to the input axes one rule's own
// rows declared joins against. It is the foreign fence, stated once: every
// installer receives a plane that can seal a read against the Factors its plan
// depends on and against no other.
func (plane FormPlane[K, V]) forRule(rows []FormRow) (FormPlane[K, V], bool) {
	if !plane.Valid() || len(rows) == 0 {
		return FormPlane[K, V]{}, false
	}
	var declared, selected []ForeignFactor
	grow := func(table []ForeignFactor, factor uint32) []ForeignFactor {
		if uint64(factor) < uint64(len(table)) {
			return table
		}
		grown := make([]ForeignFactor, factor+1)
		copy(grown, table)
		return grown
	}
	for _, row := range rows {
		for index := 0; index < row.Rule.ReadCount(); index++ {
			read, readOK := row.Rule.ReadAt(index)
			if !readOK {
				return FormPlane[K, V]{}, false
			}
			if uint64(read.Factor) >= uint64(len(plane.foreign)) {
				return FormPlane[K, V]{}, false
			}
			declared = grow(declared, read.Factor)
			declared[read.Factor] = plane.foreign[read.Factor]
			if read.Form != ruleprogram.Selected {
				continue
			}
			selected = grow(selected, read.Factor)
			selected[read.Factor] = plane.foreign[read.Factor]
		}
	}
	narrowed := plane
	narrowed.foreign = declared
	narrowed.selected = selected
	narrowed.routed = declaresRoute(rows)
	narrowed.selects = declaresSelection(rows)
	return narrowed, true
}

// declaresSelection reports whether any of one rule's rows reads a dependent
// join. It fences the coordinate universe the same way declaresRoute fences
// the destinations: a rule whose plan selects nothing resolves no member.
func declaresSelection(rows []FormRow) bool {
	for _, row := range rows {
		for index := 0; index < row.Rule.ReadCount(); index++ {
			form, formOK := row.Rule.ReadFormAt(index)
			if formOK && form == ruleprogram.Selected {
				return true
			}
		}
	}
	return false
}

// declaresRoute reports whether any of one rule's rows publishes through a
// route. It is the route half of the same fence the foreign table is narrowed
// under: geometry a plan does not depend on is geometry its installer cannot
// reach, so a rule that states no route cannot resolve one.
func declaresRoute(rows []FormRow) bool {
	for _, row := range rows {
		mode, modeOK := row.Rule.OutputMode()
		if !modeOK || mode != ruleprogram.ModeRoute {
			continue
		}
		output, outputOK := row.Rule.OutputAt(0)
		if outputOK && output.RouteJoinPresent {
			return true
		}
	}
	return false
}

// Valid reports whether the plane still names a live typed binding.
func (plane FormPlane[K, V]) Valid() bool { return plane.binding != nil }

// ExactRow seals one exact read and exact write of this plane. It is the
// primitive an installed family is built from: an owner hands back the
// coordinates its plan row carries and never holds a binding of its own.
func (plane FormPlane[K, V]) ExactRow(unit carrier.Unit, input uint16, target carrier.Target, output uint16) (ExactRow[K, V], bool) {
	if !plane.Valid() {
		return ExactRow[K, V]{}, false
	}
	return NewExactRow(plane.binding, unit, input, target, output)
}

// ExactWrite seals one exact write of this plane.
func (plane FormPlane[K, V]) ExactWrite(target carrier.Target, output uint16) (ExactWrite[K, V], bool) {
	if !plane.Valid() {
		return ExactWrite[K, V]{}, false
	}
	return NewExactWrite(plane.binding, target, output)
}

// ExactRead seals one exact read of this plane.
func (plane FormPlane[K, V]) ExactRead(unit carrier.Unit, input uint16) (ExactRead[K, V], bool) {
	if !plane.Valid() {
		return ExactRead[K, V]{}, false
	}
	return NewExactRead(plane.binding, unit, input)
}

// ReadCellPolicy seals the substitutions one read's declared contract derives
// from this plane's own Factor. An exact read delivers the observed coordinate
// unchanged and leaves the substitution to its caller, so the caller seals the
// derivation here rather than naming a fallback of its own.
func (plane FormPlane[K, V]) ReadCellPolicy(contract ruleplan.ReadContract) (ReadCellPolicy[V], bool) {
	if !plane.Valid() {
		return ReadCellPolicy[V]{}, false
	}
	return declaredCellPolicy[K, V](plane.binding, contract)
}

// ForeignReadCellPolicy seals the same derivation over a foreign input axis, at
// that axis's own fact types.
func ForeignReadCellPolicy[K scalar.Key, V any](foreign ForeignFactor, contract ruleplan.ReadContract) (ReadCellPolicy[V], bool) {
	typed, ok := foreign.(foreignFactor[K, V])
	if !ok || typed.binding == nil {
		return ReadCellPolicy[V]{}, false
	}
	return declaredCellPolicy[K, V](typed.binding, contract)
}

// SelectedRead seals one selected read of this plane under the contract its
// plan row declared. The sealed policy is declaredCellPolicy's
// derivation over contract and binding; the policy argument is not read.
func (plane FormPlane[K, V]) SelectedRead(input uint16, contract ruleplan.ReadContract, _ ReadCellPolicy[V]) (SelectedRead[K, V], bool) {
	if !plane.Valid() {
		return SelectedRead[K, V]{}, false
	}
	return NewSelectedRead(plane.binding, input, contract, ReadCellPolicy[V]{})
}

// RouteWrite seals one bounded routed write of this plane.
func (plane FormPlane[K, V]) RouteWrite(output uint16) (RouteWrite[K, V], bool) {
	if !plane.Valid() {
		return RouteWrite[K, V]{}, false
	}
	return NewRouteWrite(plane.binding, output)
}

// CarryWrite seals one transformed-carry write of this plane: the row target,
// the carried target closure, and the owner-issued map their prior facts pass
// through.
func (plane FormPlane[K, V]) CarryWrite(target carrier.Target, output uint16, carried []carrier.Target, carry func(V) (V, bool)) (CarryWrite[K, V], bool) {
	if !plane.Valid() {
		return CarryWrite[K, V]{}, false
	}
	return NewCarryWrite(plane.binding, target, output, carried, carry)
}

// RowCarry seals one plan row's transformed-carry write. The carried closure
// is the row's own target, not a set the installer chooses: which coordinates
// a row carries is the Plan's statement and the solver schedules the row
// against exactly it. The map is the caller's, because a candidate-indexed
// transition is a different map at every row and only the rule knows it.
//
// It exists so a rule package outside the engine can seal its carry without
// naming a carrier coordinate type it has no business holding.
func (plane FormPlane[K, V]) RowCarry(row FormRow, carry func(V) (V, bool)) (CarryWrite[K, V], bool) {
	if !plane.Valid() {
		return CarryWrite[K, V]{}, false
	}
	return NewCarryWrite(plane.binding, row.Target, 0, []carrier.Target{row.Target}, carry)
}

// SourceColumn returns one present materialized source column of this Factor
// by the relation member ordinal a plan row carries.
func (plane FormPlane[K, V]) SourceColumn(relation uint32) (memberrelation.SourceColumn[V], bool) {
	return plane.column(relation)
}

// column returns one present materialized source column of this Factor.
func (plane FormPlane[K, V]) column(relation uint32) (memberrelation.SourceColumn[V], bool) {
	var absent memberrelation.SourceColumn[V]
	if uint64(relation) >= uint64(len(plane.columns)) || uint64(relation) >= uint64(len(plane.present)) || !plane.present[relation] {
		return absent, false
	}
	return plane.columns[relation], true
}

// formBuilder compiles every row of one form into exactly one typed family.
type formBuilder[K scalar.Key, V any] func(FormPlane[K, V], []FormRow) (Family, []FormAddress, bool)

// formBuilders is the implementation column of the form table. It is built per
// typed instantiation at Program seal, never on a solve path. A declared
// ordinal with no entry here is named but not executable, and every row of it
// is authored by an installed RuleFamilyInstaller.
func formBuilders[K scalar.Key, V any]() [formCount]formBuilder[K, V] {
	var table [formCount]formBuilder[K, V]
	table[FormExact] = buildExactForm[K, V]
	table[FormSource] = buildSourceForm[K, V]
	return table
}

// DeclaredForm answers the one execution form a sealed descriptor DECLARES.
//
// The form is derived from the publication mode, the carry disposition and the
// read vocabulary the rule declares. It is not probed. A column of independent
// classifiers could only answer by coincidence of what each one happened to
// accept, and three of the six forms - Summary, Carry and SelectedRoute - have
// no generic builder at all, so their probes were refusing geometry that
// nothing behind them implements. A rule those probes turned away could not
// reach the installer that would have executed it.
//
// The derivation is total and single-valued by construction, which is what the
// exclusivity of a probe column was trying to approximate. A row whose
// geometry a generic builder cannot build refuses at that builder, which is
// where a build refusal belongs.
func DeclaredForm(rule generated.CompiledRule) (FormRow, bool) {
	if !rule.Available() {
		return FormRow{}, false
	}
	mode, modeOK := rule.OutputMode()
	if !modeOK {
		return FormRow{}, false
	}
	row, derived := declaredFormRow(rule, mode)
	if !derived || row.Form != row.Form.claimed() {
		return FormRow{}, false
	}
	row.Rule = rule
	return row, true
}

// claimed answers the ordinal a derived row may carry: itself when declared,
// and the undeclared zero otherwise. It exists so the derivation cannot name a
// ladder slot the table does not have.
func (form Form) claimed() Form {
	if !form.Declared() {
		return 0
	}
	return form
}

func declaredFormRow(rule generated.CompiledRule, mode ruleprogram.OutputMode) (FormRow, bool) {
	switch {
	case mode == ruleprogram.ModeStructural:
		// A structural publication transports axes across a transition rather
		// than writing a fact, and the descriptor carries that transport
		// vector exactly when its mode is structural. The branch set its rows
		// are drawn from names the input port the row is opened at, and the
		// relation those branches are members of.
		input, relation, inputOK := declaredBranchInput(rule)
		if !inputOK || rule.TransportCount() == 0 {
			return FormRow{}, false
		}
		return FormRow{Form: FormActivation, Input: input, Relation: relation}, true
	case mode == ruleprogram.ModeRoute:
		// A routed publication publishes at the members of the join the output
		// names, so that join's port and relation are the row's coordinates.
		output, outputOK := rule.OutputAt(0)
		if !outputOK || !output.RouteJoinPresent || uint64(output.RouteJoin) >= uint64(rule.ReadCount()) {
			return FormRow{}, false
		}
		route, routeOK := rule.ReadAt(int(output.RouteJoin))
		if !routeOK || route.Form != ruleprogram.Selected || !declaredPort(rule, route.Input) {
			return FormRow{}, false
		}
		return FormRow{Form: FormSelectedRoute, Input: uint16(route.Input), Relation: route.Relation.Member}, true
	case declaredTransformedCarry(rule):
		// A transformed carry applies one owner-issued candidate-indexed
		// transition to every carried coordinate. No identity fold can do
		// that, so the carry disposition alone decides this form.
		input, inputOK := declaredFirstInput(rule)
		if !inputOK {
			return FormRow{}, false
		}
		return FormRow{Form: FormCarry, Input: input}, true
	case rule.ReadCount() == 0:
		// A read-free rule writes its own materialized source column, so the
		// candidate relation it is indexed by must be published by the Factor
		// it writes.
		candidate := rule.CandidateRelation()
		if rule.InputCount() != 0 || candidate.Axis != rule.OutputFactor() {
			return FormRow{}, false
		}
		return FormRow{Form: FormSource, Relation: candidate.Member}, true
	case declaredVectorRead(rule):
		// A whole-vector read is delivered under a mandatory contract, and a
		// declaration that did not seal one has not said what its cells mean.
		for index := 0; index < rule.ReadCount(); index++ {
			form, formOK := rule.ReadFormAt(index)
			if !formOK {
				return FormRow{}, false
			}
			if form != ruleprogram.Summary && form != ruleprogram.Complete {
				continue
			}
			if _, contractOK := summaryFormContract(rule, index); !contractOK {
				return FormRow{}, false
			}
		}
		input, inputOK := declaredFirstInput(rule)
		if !inputOK {
			return FormRow{}, false
		}
		return FormRow{Form: FormSummary, Input: input}, true
	case declaredSelectedProduct(rule):
		// One exact prerequisite and the selection it derives, concluded once.
		// The selection's own members are the relation's answer, so the port
		// this row opens at is the first declared read's, exactly as it is for
		// the all-exact product below.
		input, inputOK := declaredFirstInput(rule)
		if !inputOK {
			return FormRow{}, false
		}
		return FormRow{Form: FormSelectedExact, Input: input}, true
	case declaredExactProduct(rule):
		input, inputOK := declaredFirstInput(rule)
		if !inputOK {
			return FormRow{}, false
		}
		return FormRow{Form: FormExact, Input: input}, true
	default:
		return FormRow{}, false
	}
}

// declaredTransformedCarry reports whether the descriptor declares a carry
// that applies an owner-issued transform rather than handing the prior fact on.
func declaredTransformedCarry(rule generated.CompiledRule) bool {
	carry, present := rule.CarryMode()
	if !present || carry != ruleprogram.CarryTransform {
		return false
	}
	_, transformPresent := rule.CarryTransform()
	return transformPresent
}

// declaredVectorRead reports whether any declared read delivers a whole cell
// vector rather than one cell.
func declaredVectorRead(rule generated.CompiledRule) bool {
	for index := 0; index < rule.ReadCount(); index++ {
		form, formOK := rule.ReadFormAt(index)
		if !formOK {
			return false
		}
		if form == ruleprogram.Summary || form == ruleprogram.Complete {
			return true
		}
	}
	return false
}

// declaredExactProduct reports whether every declared read is exact.
// declaredSelectedProduct reports whether the descriptor declares the exact
// publication over a selection: at least one exact read, at least one selected
// read, and nothing else. A rule with no selected read is the all-exact
// product; one with any other read form is neither.
func declaredSelectedProduct(rule generated.CompiledRule) bool {
	if rule.ReadCount() < 2 {
		return false
	}
	exact, selected := 0, 0
	for index := 0; index < rule.ReadCount(); index++ {
		form, formOK := rule.ReadFormAt(index)
		if !formOK {
			return false
		}
		switch form {
		case ruleprogram.Exact:
			exact++
		case ruleprogram.Selected:
			selected++
		default:
			return false
		}
	}
	return exact > 0 && selected > 0
}

func declaredExactProduct(rule generated.CompiledRule) bool {
	if rule.ReadCount() == 0 {
		return false
	}
	for index := 0; index < rule.ReadCount(); index++ {
		form, formOK := rule.ReadFormAt(index)
		if !formOK || form != ruleprogram.Exact {
			return false
		}
	}
	return true
}

// declaredBranchInput answers the port and relation of the read whose members
// are one trigger's candidate branches.
//
// A branch is COLD. The construct topology mounts one activation member per
// branch before any solve, and execution settles the disposition of branches
// already mounted - it can publish no others. So the branch read is the one
// whose relation declares a PARENT: an ordinal-addressed member set the owner
// already published under the trigger's own candidate row, enumerable at
// issuance.
//
// A selection is never that read. Its coordinates are, in this package's own
// words, the members of a relation that exists only per invocation, resolved
// by the reading family - so a structural row drawn from one would publish
// branches nothing had mounted.
func declaredBranchInput(rule generated.CompiledRule) (uint16, uint32, bool) {
	for index := 0; index < rule.ReadCount(); index++ {
		read, readOK := rule.ReadAt(index)
		if !readOK {
			return 0, 0, false
		}
		if !read.ParentPresent || read.Form != ruleprogram.Summary {
			continue
		}
		if !declaredPort(rule, read.Input) {
			return 0, 0, false
		}
		return uint16(read.Input), read.Relation.Member, true
	}
	return 0, 0, false
}

// declaredFirstInput answers the port of the descriptor's first declared read.
func declaredFirstInput(rule generated.CompiledRule) (uint16, bool) {
	input := rule.ReadInput()
	if input < 0 || !declaredPort(rule, uint32(input)) {
		return 0, false
	}
	return uint16(input), true
}

// declaredPort reports whether one read names a port inside the descriptor's
// sealed contiguous input prefix.
func declaredPort(rule generated.CompiledRule, input uint32) bool {
	return rule.InputCount() > 0 && uint64(input) < uint64(rule.InputCount()) && input <= uint32(^uint16(0))
}

// BuildForms compiles one Factor's generated plan rows into one typed family
// per present form, in sealed ordinal order. A row whose form has no registered
// implementation refuses and returns that form, which names itself.
func BuildForms[K scalar.Key, V any](plane FormPlane[K, V], rows []FormRow) ([]Family, []FormAddress, Form, bool) {
	return buildForms(plane, rows, formBuilders[K, V]())
}

// authoredRuleOrdinal answers the sealed rule ordinal one plan row belongs to
// when an installer authors that rule's family. The ordinal is the row's own
// foreign key; whether it is authored is the family table's answer, and a row
// naming a rule no installer claimed takes the generic builder for its form.
func authoredRuleOrdinal[K scalar.Key, V any](plane FormPlane[K, V], row FormRow) (uint32, bool) {
	_, authored := plane.families.Installer(row.RuleOrdinal)
	return row.RuleOrdinal, authored
}

// buildForms is BuildForms over an explicit implementation column, so the
// totality law can state what a declared form without an implementation does.
func buildForms[K scalar.Key, V any](plane FormPlane[K, V], rows []FormRow, builders [formCount]formBuilder[K, V]) ([]Family, []FormAddress, Form, bool) {
	if !plane.Valid() || len(rows) == 0 {
		return nil, nil, 0, false
	}
	var grouped [formCount][]FormRow
	var installedForms [formCount][]uint32
	installed := map[uint32][]FormRow{}
	for _, row := range rows {
		if row.Member < 0 || !row.Form.Declared() {
			return nil, nil, row.Form, false
		}
		ordinal, authored := authoredRuleOrdinal(plane, row)
		if authored {
			if existing, present := installed[ordinal]; present {
				if existing[0].Form != row.Form {
					return nil, nil, row.Form, false
				}
			} else {
				installedForms[row.Form] = append(installedForms[row.Form], ordinal)
			}
			installed[ordinal] = append(installed[ordinal], row)
			continue
		}
		if builders[row.Form] == nil {
			return nil, nil, row.Form, false
		}
		grouped[row.Form] = append(grouped[row.Form], row)
	}
	families := make([]Family, 0, formCount-1)
	addresses := make([]FormAddress, 0, len(rows))
	appendFamily := func(family Family, formAddresses []FormAddress, expected int) bool {
		if family == nil || len(formAddresses) != expected {
			return false
		}
		offset := uint32(len(families))
		families = append(families, family)
		for _, address := range formAddresses {
			address.FamilyOffset = offset
			addresses = append(addresses, address)
		}
		return true
	}
	for form := Form(1); form < formCount; form++ {
		if formRows := grouped[form]; len(formRows) != 0 {
			family, formAddresses, built := builders[form](plane, formRows)
			if !built || !appendFamily(family, formAddresses, len(formRows)) {
				return nil, nil, form, false
			}
		}
		// An installed rule follows its own form's family, in ascending rule
		// order, so the ladder a Program produces stays a function of the sealed
		// ordinals rather than of the order rows were discovered.
		ordinals := installedForms[form]
		sort.Slice(ordinals, func(left, right int) bool { return ordinals[left] < ordinals[right] })
		for _, ordinal := range ordinals {
			ruleRows := installed[ordinal]
			installer, authored := plane.families.Installer(ordinal)
			if !authored {
				return nil, nil, form, false
			}
			rulePlane, fenced := plane.forRule(ruleRows)
			if !fenced {
				return nil, nil, form, false
			}
			family, ruleAddresses, built := installer.InstallRuleFamily(rulePlane, ordinal, ruleRows)
			if !built || !appendFamily(family, ruleAddresses, len(ruleRows)) {
				return nil, nil, form, false
			}
		}
	}
	if len(families) == 0 {
		return nil, nil, 0, false
	}
	return families, addresses, 0, true
}

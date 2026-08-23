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
)

// Form is the sealed execution form ordinal one plan row carries. The table is
// append-only: an ordinal is never reused, renumbered, or reordered, so a
// Program sealed against one revision keeps its ladder under the next.
type Form uint8

const (
	// FormExact is the one-read identity fold onto an exact write.
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
	Member   int
	Form     Form
	Input    uint16
	Relation uint32
	Unit     carrier.Unit
	Target   carrier.Target
	Rule     generated.CompiledRule
}

// FormAddress is where one plan row's invocation landed: the family offset
// within this Factor's returned ladder and the row's local ordinal inside that
// family.
type FormAddress struct {
	Member       int
	FamilyOffset uint32
	Local        uint32
}

// RuleFamilyProvider is implemented by a bound owner that authors the execution
// family of one of its own sealed rule ordinals with concrete types. It is the
// handoff a materialized source column already uses, handing over an executor
// instead of a column, and it is how a rule the engine cannot type reaches
// execution: a rule whose joins or whose reducer are typed in more than one
// Factor has no generic builder, because the engine may not name those types
// and the owner may not see the solve loop.
//
// AuthorsRule partitions before anything is built, so a refusal from
// InstallRuleFamily is a real refusal rather than a silent fall back to a
// generic form builder.
type RuleFamilyProvider[K scalar.Key, V any] interface {
	// AuthorsRule reports whether this owner installs the family of one sealed
	// rule ordinal.
	AuthorsRule(rule uint32) bool
	// InstallRuleFamily seals one family covering every plan row of that rule,
	// in the order given, and answers the local ordinal each row was sealed at.
	// Every primitive it needs is sealed through the plane, so an owner outside
	// this package builds a family without naming - or reaching - one carrier,
	// binding, or guard type.
	InstallRuleFamily(plane FormPlane[K, V], rule uint32, rows []FormRow) (Family, []FormAddress, bool)
}

// FormPlane is the sealed typed plane a form builder may read: the Factor's
// fact binding, its materialized source columns, and the owner that installs
// families for its own rules. It is a value handoff, not an owner capability -
// a builder can seal descriptors from it and nothing else.
type FormPlane[K scalar.Key, V any] struct {
	binding  *factbinding.Binding[K, V]
	columns  []memberrelation.SourceColumn[V]
	present  []bool
	families RuleFamilyProvider[K, V]
}

// NewFormPlane seals one bound Factor's typed plane for the form table. A
// Factor that installs no family of its own passes a nil provider.
func NewFormPlane[K scalar.Key, V any](binding *factbinding.Binding[K, V], columns []memberrelation.SourceColumn[V], present []bool, families RuleFamilyProvider[K, V]) (FormPlane[K, V], bool) {
	if binding == nil {
		return FormPlane[K, V]{}, false
	}
	return FormPlane[K, V]{binding: binding, columns: columns, present: present, families: families}, true
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

// SelectedRead seals one selected read of this plane under the contract its
// plan row declared and the materialization the read boundary derived.
func (plane FormPlane[K, V]) SelectedRead(input uint16, contract ruleplan.ReadContract, policy ReadCellPolicy[V]) (SelectedRead[K, V], bool) {
	if !plane.Valid() {
		return SelectedRead[K, V]{}, false
	}
	return NewSelectedRead(plane.binding, input, contract, policy)
}

// RouteWrite seals one bounded routed write of this plane.
func (plane FormPlane[K, V]) RouteWrite(output uint16) (RouteWrite[K, V], bool) {
	if !plane.Valid() {
		return RouteWrite[K, V]{}, false
	}
	return NewRouteWrite(plane.binding, output)
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

// formClassifier decides whether one sealed descriptor is its own form and
// extracts that form's coordinates. It is type-neutral: classification reads
// the plan, never a typed binding.
type formClassifier func(generated.CompiledRule) (FormRow, bool)

// formBuilder compiles every row of one form into exactly one typed family.
type formBuilder[K scalar.Key, V any] func(FormPlane[K, V], []FormRow) (Family, []FormAddress, bool)

// formClassifiers is the classifier column of the form table, indexed by
// ordinal. A form lane appends its own entry here and owns its child file.
var formClassifiers = [formCount]formClassifier{
	FormExact:         classifyExactForm,
	FormSource:        classifySourceForm,
	FormSummary:       classifySummaryForm,
	FormCarry:         classifyCarryForm,
	FormSelectedRoute: classifySelectedRouteForm,
}

// formBuilders is the implementation column of the form table. It is built per
// typed instantiation at Program seal, never on a solve path. A declared
// ordinal with no entry here is named but not executable.
func formBuilders[K scalar.Key, V any]() [formCount]formBuilder[K, V] {
	var table [formCount]formBuilder[K, V]
	table[FormExact] = buildExactForm[K, V]
	table[FormSource] = buildSourceForm[K, V]
	return table
}

// ClassifyForm resolves the one execution form of a sealed descriptor. Every
// declared form is probed: exactly one may claim a descriptor, so two forms can
// never silently overlap and the winner never depends on probe order.
func ClassifyForm(rule generated.CompiledRule) (FormRow, bool) {
	return classifyForm(rule, formClassifiers)
}

// classifyForm is ClassifyForm over an explicit classifier column, so the
// exclusivity law can be stated over a table the test owns.
func classifyForm(rule generated.CompiledRule, classifiers [formCount]formClassifier) (FormRow, bool) {
	if !rule.Available() {
		return FormRow{}, false
	}
	var claimed FormRow
	found := false
	for form := Form(1); form < formCount; form++ {
		classifier := classifiers[form]
		if classifier == nil {
			continue
		}
		row, claims := classifier(rule)
		if !claims {
			continue
		}
		if found || row.Form != form {
			return FormRow{}, false
		}
		row.Rule = rule
		claimed, found = row, true
	}
	if !found {
		return FormRow{}, false
	}
	return claimed, true
}

// BuildForms compiles one Factor's generated plan rows into one typed family
// per present form, in sealed ordinal order. A row whose form has no registered
// implementation refuses and returns that form, which names itself.
func BuildForms[K scalar.Key, V any](plane FormPlane[K, V], rows []FormRow) ([]Family, []FormAddress, Form, bool) {
	return buildForms(plane, rows, formBuilders[K, V]())
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
		ordinal, ordinalOK := row.Rule.Ordinal()
		if plane.families != nil && ordinalOK && plane.families.AuthorsRule(ordinal) {
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
			family, ruleAddresses, built := plane.families.InstallRuleFamily(plane, ordinal, ruleRows)
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

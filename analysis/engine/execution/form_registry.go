// form_registry.go declares the append-only execution form table. One sealed
// form ordinal names one execution form; each form owns a child file that
// registers its classifier and its typed family builder. No caller switches on
// form shape: the descriptor is classified once here, and families are built in
// sealed ordinal order so a Program's family ladder never depends on the order
// its rows were discovered.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
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
	// formCount is the exclusive upper bound of the declared ordinals. It is
	// the last constant in the block; a new form is appended above it.
	formCount
)

// formNames is the name column of the form table. Every declared ordinal has a
// name, whether or not it has an implementation, so a plan whose form is not
// executable refuses by that name instead of by a bare ordinal.
var formNames = [formCount]string{
	FormExact:   "exact",
	FormSource:  "source",
	FormSummary: "summary",
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

// FormPlane is the sealed typed plane a form builder may read: the Factor's
// fact binding and its materialized source columns. It is a value handoff, not
// an owner capability - a builder can seal descriptors from it and nothing
// else.
type FormPlane[K scalar.Key, V any] struct {
	binding *factbinding.Binding[K, V]
	columns []memberrelation.SourceColumn[V]
	present []bool
}

// NewFormPlane seals one bound Factor's typed plane for the form table.
func NewFormPlane[K scalar.Key, V any](binding *factbinding.Binding[K, V], columns []memberrelation.SourceColumn[V], present []bool) (FormPlane[K, V], bool) {
	if binding == nil {
		return FormPlane[K, V]{}, false
	}
	return FormPlane[K, V]{binding: binding, columns: columns, present: present}, true
}

// Valid reports whether the plane still names a live typed binding.
func (plane FormPlane[K, V]) Valid() bool { return plane.binding != nil }

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
	FormExact:   classifyExactForm,
	FormSource:  classifySourceForm,
	FormSummary: classifySummaryForm,
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
	for _, row := range rows {
		if row.Member < 0 || !row.Form.Declared() || builders[row.Form] == nil {
			return nil, nil, row.Form, false
		}
		grouped[row.Form] = append(grouped[row.Form], row)
	}
	families := make([]Family, 0, formCount-1)
	addresses := make([]FormAddress, 0, len(rows))
	for form := Form(1); form < formCount; form++ {
		formRows := grouped[form]
		if len(formRows) == 0 {
			continue
		}
		family, formAddresses, built := builders[form](plane, formRows)
		if !built || family == nil || len(formAddresses) != len(formRows) {
			return nil, nil, form, false
		}
		offset := uint32(len(families))
		families = append(families, family)
		for _, address := range formAddresses {
			address.FamilyOffset = offset
			addresses = append(addresses, address)
		}
	}
	if len(families) == 0 {
		return nil, nil, 0, false
	}
	return families, addresses, 0, true
}

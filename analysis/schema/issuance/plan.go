package issuance

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/schema"
)

// SubscriptionSpec binds one rule row to three already-sealed machine
// declarations. It carries no projected ordinals or copied form semantics.
type SubscriptionSpec struct {
	Family      schema.Key
	Requirement schema.Key
	Form        schema.Key
	Rule        schema.Key
	Writes      schema.Key
	// Source is the relation this rule reaches its candidate row through, and
	// is empty for a rule whose candidates are not Program rows. It is
	// transported from the rule's own Program declaration rather than authored
	// beside it, so the candidate authority stays in one place.
	Source schema.Key
}

// Subscription is the admitted execution binding. The declaration pointers
// are the exact rows owned by the sealed issuance surface.
type Subscription struct {
	family      *Entry
	requirement *Entry
	form        *Entry
	source      *Entry
	rule        schema.Key
	writes      schema.Key
}

func (row Subscription) Family() *Entry      { return row.family }
func (row Subscription) Requirement() *Entry { return row.requirement }
func (row Subscription) Form() *Entry        { return row.form }

// Source is the relation reaching this subscription's candidate row, and nil
// when the rule draws its candidates from somewhere other than a Program row
// space.
func (row Subscription) Source() *Entry     { return row.source }
func (row Subscription) Rule() schema.Key   { return row.rule }
func (row Subscription) Writes() schema.Key { return row.writes }
func (row Subscription) Available() bool {
	return row.family != nil && row.family.kind == KindFamily &&
		row.requirement != nil && row.requirement.kind == KindRequirement &&
		row.form != nil && row.form.kind == KindForm &&
		row.rule.Available() && row.writes.Available()
}

// Plan is the canonical sealed-machine execution input. It replaces the
// former enum/framing Directory projection.
type Plan struct {
	table         Table
	subscriptions []Subscription
	axes          []schema.Key
}

func NewPlan(table Table, specs []SubscriptionSpec) (Plan, bool) {
	if table.entries == nil {
		return Plan{}, false
	}
	rows := make([]Subscription, 0, len(specs))
	axisSet := make(map[schema.Key]struct{}, len(specs))
	seen := make(map[SubscriptionSpec]struct{}, len(specs))
	for _, spec := range specs {
		if _, duplicate := seen[spec]; duplicate {
			return Plan{}, false
		}
		seen[spec] = struct{}{}
		family, familyOK := table.Entry(spec.Family, KindFamily)
		requirement, requirementOK := table.Entry(spec.Requirement, KindRequirement)
		form, formOK := table.Entry(spec.Form, KindForm)
		row := Subscription{
			family: family, requirement: requirement, form: form,
			rule: spec.Rule, writes: spec.Writes,
		}
		if spec.Source.Available() {
			source, sourceOK := table.Entry(spec.Source, KindRelation)
			// A candidate source is read from the rows the rule is issued over.
			// A relation rooted in another space would hand the rule a row no
			// occurrence of its family can reach.
			if !sourceOK || !familyOK || source.space != family.space {
				return Plan{}, false
			}
			row.source = source
		}
		if !familyOK || !requirementOK || !formOK || !row.Available() ||
			!formOutputsAvailable(form, requirement) ||
			!subscriptionSpacesCompose(family, requirement, form, table) {
			return Plan{}, false
		}
		rows = append(rows, row)
		axisSet[row.writes] = struct{}{}
	}
	axes := sortedKeys(axisSet)
	return Plan{table: table, subscriptions: rows, axes: axes}, true
}

func subscriptionSpacesCompose(family, requirement, form *Entry, table Table) bool {
	subject, ok := table.Entry(form.subject, KindOutput)
	return ok && subject.typ.Value == ValueRow && subject.typ.Cardinality == CardinalityOne &&
		family.space == requirement.space && requirement.space == subject.typ.Space
}

func formOutputsAvailable(form, requirement *Entry) bool {
	published := make(map[schema.Key]struct{}, len(requirement.outputs))
	for _, binding := range requirement.outputs {
		published[binding.Output] = struct{}{}
	}
	for _, required := range form.requires {
		if _, ok := published[required]; !ok {
			return false
		}
	}
	return true
}

func (plan Plan) Count() int { return len(plan.subscriptions) }

func (plan Plan) At(index int) (Subscription, bool) {
	if index < 0 || index >= len(plan.subscriptions) {
		return Subscription{}, false
	}
	row := plan.subscriptions[index]
	return row, row.Available()
}

func (plan Plan) Table() Table { return plan.table }

func (plan Plan) Axes() []schema.Key { return append([]schema.Key(nil), plan.axes...) }

func sortedKeys(set map[schema.Key]struct{}) []schema.Key {
	keys := make([]schema.Key, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	return keys
}

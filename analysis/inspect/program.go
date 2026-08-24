package inspect

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/domain/composite"
)

// declaredProgram is the read-only reading of the sealed rule declaration
// surface this session compiled through. It holds the two sealed views and the
// compiled Plan catalog; it copies no row of either.
type declaredProgram struct {
	table   *seal.Schema
	axes    seal.View
	rules   seal.View
	queries seal.View
	ok      bool
}

func newDeclaredProgram(compilation composite.Compilation) declaredProgram {
	if !compilation.Available() {
		return declaredProgram{}
	}
	table, failure := composite.Table(compilation)
	if failure.Available() || table == nil || !table.Available() {
		return declaredProgram{}
	}
	axes, axesOK := table.Surface(schema.SurfaceKindAxis)
	rules, rulesOK := table.Surface(schema.SurfaceKindRule)
	queries, queriesOK := table.Surface(schema.SurfaceKindQuery)
	if !axesOK || !rulesOK || !queriesOK || !axes.Available() || !rules.Available() || !queries.Available() {
		return declaredProgram{}
	}
	return declaredProgram{table: table, axes: axes, rules: rules, queries: queries, ok: true}
}

// RuleCount is the sealed rule inventory's cardinality.
func (declared declaredProgram) RuleCount() int {
	if !declared.ok {
		return 0
	}
	return declared.rules.Count()
}

// RuleAt is one sealed rule template by its dense declaration ordinal.
func (declared declaredProgram) RuleAt(position int) (*rule.Template, bool) {
	if !declared.ok {
		return nil, false
	}
	entry, entryOK := declared.rules.At(position)
	if !entryOK {
		return nil, false
	}
	template, templateOK := entry.(*rule.Template)
	if !templateOK || template == nil || !template.EntryAvailable() {
		return nil, false
	}
	return template, true
}

// QueryRegistration is one publication family's sealed query registration.
// The registration names the axes the family's answer is read from, which is
// the declared link between a published cell and the rules that could have
// written it.
func (declared declaredProgram) QueryRegistration(family schema.Key) (*query.Registration, bool) {
	if !declared.ok || !family.Available() {
		return nil, false
	}
	entry, entryOK := declared.queries.ByID(schema.NewEntryID(schema.SurfaceKindQuery, family))
	if !entryOK || entry == nil {
		return nil, false
	}
	registration, registrationOK := entry.(*query.Registration)
	if !registrationOK || registration == nil || !registration.EntryAvailable() {
		return nil, false
	}
	return registration, true
}

// writesAxis reports whether one rule's declared fold publishes a column on
// the named axis. It is the declaration-side answer to "which rule could have
// produced this cell", read from Program.Fold.Outputs rather than inferred
// from a solved value.
func writesAxis(declaration ruleprogram.Program, axis schema.Key) bool {
	if !axis.Available() {
		return false
	}
	for _, output := range declaration.Fold.Outputs {
		if output.Column.Axis.Key == axis {
			return true
		}
	}
	return false
}

// sourceSpelling names one join source the way the declaration writes it: the
// candidate relation, or a prior join by its declaration position.
func sourceSpelling(source ruleprogram.SourceRef) string {
	if source.Candidate {
		return "Candidate"
	}
	return "Joins[" + decimal(source.Position) + "]"
}

func readFormSpelling(form ruleprogram.ReadForm) string {
	switch form {
	case ruleprogram.Exact:
		return "Exact"
	case ruleprogram.Selected:
		return "Selected"
	case ruleprogram.Summary:
		return "Summary"
	case ruleprogram.Complete:
		return "Complete"
	default:
		return "ReadFormInvalid"
	}
}

func orderSpelling(order ruleprogram.Order) string {
	switch order {
	case ruleprogram.OrderCanonical:
		return "OrderCanonical"
	case ruleprogram.OrderByTag:
		return "OrderByTag"
	case ruleprogram.OrderOwner:
		return "OrderOwner"
	default:
		return "OrderInvalid"
	}
}

func sparseSpelling(sparse ruleprogram.Sparse) string {
	switch sparse {
	case ruleprogram.SparseExplicit:
		return "SparseExplicit"
	case ruleprogram.SparseDefault:
		return "SparseDefault"
	case ruleprogram.SparseDense:
		return "SparseDense"
	default:
		return "SparseInvalid"
	}
}

func onOpaqueSpelling(onOpaque ruleprogram.OnOpaque) string {
	switch onOpaque {
	case ruleprogram.OnOpaqueRefuse:
		return "OnOpaqueRefuse"
	case ruleprogram.OnOpaquePropagateAuthenticated:
		return "OnOpaquePropagateAuthenticated"
	default:
		return "OnOpaqueInvalid"
	}
}

func multiplicitySpelling(multiplicity ruleprogram.Multiplicity) string {
	switch multiplicity {
	case ruleprogram.MultiplicityOptional:
		return "MultiplicityOptional"
	case ruleprogram.MultiplicityOne:
		return "MultiplicityOne"
	case ruleprogram.MultiplicityMany:
		return "MultiplicityMany"
	default:
		return "MultiplicityInvalid"
	}
}

func outputModeSpelling(mode ruleprogram.OutputMode) string {
	switch mode {
	case ruleprogram.ModeExact:
		return "ModeExact"
	case ruleprogram.ModeRoute:
		return "ModeRoute"
	case ruleprogram.ModeStructural:
		return "ModeStructural"
	default:
		return "ModeInvalid"
	}
}

func carryModeSpelling(mode ruleprogram.CarryMode) string {
	switch mode {
	case ruleprogram.CarryIdentity:
		return "CarryIdentity"
	case ruleprogram.CarryTransform:
		return "CarryTransform"
	default:
		return "CarryModeInvalid"
	}
}

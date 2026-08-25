package emitlaw

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// Canonical renders one Program declaration in the single textual form every
// emitted law suite states a declaration's geometry in.
//
// The form is total: every field the cold ABI carries appears on exactly one
// line, in declaration order, so a geometry law stated over it covers the
// whole declaration rather than the clauses an author happened to spell. It
// is also the reason the emitted geometry law is not a tautology against a
// baked copy of the same struct: the rendering is checked in beside the
// declaration and held to it by the freshness law, so a declaration that
// moves without regeneration is a build failure and a declaration that moves
// with it is a reviewable diff - which is exactly what the authored
// field-by-field restatement bought, over every field instead of a sample.
func Canonical(declaration program.Program) string {
	var out strings.Builder
	line(&out, "operand", key(declaration.OperandRole))
	line(&out, "candidate", candidateForm(declaration.Candidate))
	for index, join := range declaration.Joins {
		prefix := fmt.Sprintf("join[%d]", index)
		line(&out, prefix+".read", readForm(join.Read))
		line(&out, prefix+".relation", relationForm(join.Relation))
		line(&out, prefix+".key", projectionForm(join.Key))
		line(&out, prefix+".predicate", projectionForm(join.Predicate))
		line(&out, prefix+".selection", selectionForm(join.Selection))
		line(&out, prefix+".parent", relationForm(join.Parent))
		line(&out, prefix+".sources", sourcesForm(join.Sources))
		line(&out, prefix+".contract", contractForm(join.Read.Contract))
	}
	observation(&out, declaration)
	line(&out, "carry", carryForm(declaration.Carry))
	line(&out, "fold.reducer", reducerForm(declaration.Fold.Reducer))
	line(&out, "fold.inputs", inputsForm(declaration.Fold.Inputs))
	for index, output := range declaration.Fold.Outputs {
		line(&out, fmt.Sprintf("output[%d]", index), outputForm(output))
	}
	if len(declaration.Transport) == 0 {
		line(&out, "transport", "none")
	}
	for index, transport := range declaration.Transport {
		line(&out, fmt.Sprintf("transport[%d]", index),
			fmt.Sprintf("axis=%s exported=%t", reference(transport.Axis.EntryReference()), transport.Exported))
	}
	return out.String()
}

// observation renders the point each read is taken at, and how many distinct
// points the rule observes in all.
//
// A port IS an observation point - it selects the predecessor state a read
// resolves against - so this is the one clause of a read that cannot be
// checked by looking at the read alone. Two reads on one port observe one
// point, which is legal and sometimes intended; what makes it reviewable is
// seeing the census beside the reads, because a declaration copied from
// another rule keeps the ports it was copied with and every other clause of it
// can be right while the points are wrong.
func observation(out *strings.Builder, declaration program.Program) {
	points := map[uint64]struct{}{}
	for _, join := range declaration.Joins {
		points[join.Read.Input.Uint64()] = struct{}{}
	}
	if declaration.Carry != nil {
		points[declaration.Carry.Input.Uint64()] = struct{}{}
	}
	line(out, "observation", fmt.Sprintf("points=%d reads=%d", len(points), len(declaration.Joins)))
	for index, join := range declaration.Joins {
		line(out, fmt.Sprintf("observation[%d]", index), fmt.Sprintf("read=%d point=%d", index, join.Read.Input.Uint64()))
	}
	if declaration.Carry != nil {
		line(out, "observation.carry", fmt.Sprintf("point=%d", declaration.Carry.Input.Uint64()))
	}
}

// CanonicalEntry renders the rule identity the declaration is sealed under.
// It is a separate form because a rule's identity and its execution geometry
// are separate statements: a family can restate one without restating the
// other, and an emitted law that folded them would report the wrong drift.
func CanonicalEntry(spec rule.Spec) string {
	var out strings.Builder
	line(&out, "rule", key(spec.Key))
	line(&out, "lane", laneName(spec.Lane))
	line(&out, "writes", key(spec.Writes))
	line(&out, "owner", key(spec.Owner))
	line(&out, "semantic", key(spec.Semantic))
	if len(spec.Roles) == 0 {
		line(&out, "roles", "none")
	}
	for index, role := range spec.Roles {
		line(&out, fmt.Sprintf("role[%d]", index), key(role))
	}
	if len(spec.Issues) == 0 {
		line(&out, "issues", "none")
	}
	for index, issue := range spec.Issues {
		line(&out, fmt.Sprintf("issue[%d]", index),
			fmt.Sprintf("occurrence=%s form=%s requirement=%s", key(issue.Occurrence), key(issue.Form), key(issue.Requirement)))
	}
	return out.String()
}

func line(out *strings.Builder, name, value string) {
	fmt.Fprintf(out, "%-20s %s\n", name, value)
}

func key(value schema.Key) string {
	if !value.Available() {
		return "-"
	}
	return string(value)
}

func reference(value schema.EntryReference) string {
	if !value.Declared() {
		return "-"
	}
	return fmt.Sprintf("%s/%s", surfaceName(value.Surface), key(value.Key))
}

func candidateForm(candidate member.CandidateRef) string {
	return fmt.Sprintf("relation=%s issued-row=%s", relationForm(candidate.AxisRelation), key(candidate.IssuedRow))
}

func relationForm(relation member.RelationRef) string {
	if !relation.Declared() {
		return "-"
	}
	return fmt.Sprintf("%s:%s", reference(relation.Axis), key(relation.Member))
}

func projectionForm(projection member.ProjectionRef) string {
	if !projection.Declared() {
		return "-"
	}
	return fmt.Sprintf("%s:%s", reference(projection.Axis), key(projection.Member))
}

func selectionForm(selection member.SelectionRef) string {
	if !selection.Declared() {
		return "-"
	}
	return fmt.Sprintf("%s:%s", reference(selection.Axis), key(selection.Member))
}

func reducerForm(reducer member.ReducerRef) string {
	if !reducer.Declared() {
		return "-"
	}
	return fmt.Sprintf("%s:%s", reference(reducer.Axis), key(reducer.Member))
}

func transformForm(transform member.CarryTransformRef) string {
	if !transform.Declared() {
		return "-"
	}
	return fmt.Sprintf("%s:%s", reference(transform.Axis), key(transform.Member))
}

func outputColumnForm(column axis.OutputRef) string {
	if !column.Declared() {
		return "-"
	}
	return fmt.Sprintf("%s:%s", reference(column.Axis), key(column.Key))
}

func readForm(read program.ReadDecl) string {
	return fmt.Sprintf("form=%s input=%d axis=%s point-bound=%s",
		formName(read.Form), read.Input.Uint64(), reference(read.Axis.EntryReference()), pointBoundName(read.PointBound))
}

func contractForm(contract program.ReadContract) string {
	return fmt.Sprintf("order=%s sparse=%s on-opaque=%s multiplicity=%s denominator=%s",
		orderName(contract.Order), sparseName(contract.Sparse), onOpaqueName(contract.OnOpaque),
		multiplicityName(contract.Multiplicity), reference(contract.DenominatorRef.EntryReference()))
}

func sourcesForm(sources []program.SourceRef) string {
	if len(sources) == 0 {
		return "none"
	}
	spellings := make([]string, 0, len(sources))
	for _, source := range sources {
		if source.Candidate {
			spellings = append(spellings, "candidate")
			continue
		}
		spellings = append(spellings, fmt.Sprintf("join %d", source.Position))
	}
	return strings.Join(spellings, ", ")
}

func carryForm(carry *program.CarryDecl) string {
	if carry == nil {
		return "none"
	}
	return fmt.Sprintf("mode=%s input=%d transform=%s",
		carryModeName(carry.Mode), carry.Input.Uint64(), transformForm(carry.Transform))
}

func inputsForm(inputs []program.JoinRef) string {
	if len(inputs) == 0 {
		return "none"
	}
	spellings := make([]string, 0, len(inputs))
	for _, input := range inputs {
		spellings = append(spellings, fmt.Sprintf("join %d", uint64(input)))
	}
	return strings.Join(spellings, ", ")
}

func outputForm(output program.OutputDecl) string {
	route := "-"
	if output.RouteJoinPresent {
		route = fmt.Sprintf("join %d", uint64(output.RouteJoin))
	}
	return fmt.Sprintf("mode=%s value-slot=%d route=%s column=%s destination=%s",
		outputModeName(output.Mode), output.ValueSlot, route,
		outputColumnForm(output.Column), projectionForm(output.Destination))
}

// The name tables below are the emitter's own spelling of closed vocabularies
// that publish no String method. An unnamed member renders as its ordinal so
// a vocabulary that grows produces a visible, reviewable diff rather than a
// silently collapsed form.

func surfaceName(surface schema.SurfaceKind) string {
	switch surface {
	case schema.SurfaceKindAxis:
		return "axis"
	case schema.SurfaceKindDenominator:
		return "denominator"
	case schema.SurfaceKindRule:
		return "rule"
	default:
		return fmt.Sprintf("surface(%d)", uint8(surface))
	}
}

func formName(form program.ReadForm) string {
	switch form {
	case program.Exact:
		return "exact"
	case program.Selected:
		return "selected"
	case program.Summary:
		return "summary"
	case program.Complete:
		return "complete"
	default:
		return fmt.Sprintf("form(%d)", uint8(form))
	}
}

func orderName(order program.Order) string {
	switch order {
	case program.OrderCanonical:
		return "canonical"
	case program.OrderByTag:
		return "by-tag"
	case program.OrderOwner:
		return "owner"
	default:
		return fmt.Sprintf("order(%d)", uint8(order))
	}
}

func sparseName(sparse program.Sparse) string {
	switch sparse {
	case program.SparseExplicit:
		return "explicit"
	case program.SparseDefault:
		return "default"
	case program.SparseDense:
		return "dense"
	default:
		return fmt.Sprintf("sparse(%d)", uint8(sparse))
	}
}

func onOpaqueName(onOpaque program.OnOpaque) string {
	switch onOpaque {
	case program.OnOpaqueRefuse:
		return "refuse"
	case program.OnOpaquePropagateAuthenticated:
		return "propagate-authenticated"
	default:
		return fmt.Sprintf("on-opaque(%d)", uint8(onOpaque))
	}
}

func multiplicityName(multiplicity program.Multiplicity) string {
	switch multiplicity {
	case program.MultiplicityOptional:
		return "optional"
	case program.MultiplicityOne:
		return "one"
	case program.MultiplicityMany:
		return "many"
	default:
		return fmt.Sprintf("multiplicity(%d)", uint8(multiplicity))
	}
}

func pointBoundName(bound program.PointBoundDecl) string {
	switch bound {
	case program.PointBound:
		return "bound"
	case program.PointBoundSelf:
		return "self"
	default:
		return fmt.Sprintf("point-bound(%d)", uint8(bound))
	}
}

func carryModeName(mode program.CarryMode) string {
	switch mode {
	case program.CarryIdentity:
		return "identity"
	case program.CarryTransform:
		return "transform"
	default:
		return fmt.Sprintf("carry(%d)", uint8(mode))
	}
}

func outputModeName(mode program.OutputMode) string {
	switch mode {
	case program.ModeExact:
		return "exact"
	case program.ModeRoute:
		return "route"
	case program.ModeStructural:
		return "structural"
	default:
		return fmt.Sprintf("mode(%d)", uint8(mode))
	}
}

func laneName(lane rule.Lane) string {
	switch lane {
	case rule.LaneMounted:
		return "mounted"
	case rule.LaneActivation:
		return "activation"
	case rule.LaneLink:
		return "link"
	case rule.LaneMountedPoint:
		return "mounted-point"
	default:
		return fmt.Sprintf("lane(%d)", uint8(lane))
	}
}

func problemKindName(kind program.ProblemKind) string {
	switch kind {
	case program.ProblemOperand:
		return "ProblemOperand"
	case program.ProblemCandidate:
		return "ProblemCandidate"
	case program.ProblemJoin:
		return "ProblemJoin"
	case program.ProblemInput:
		return "ProblemInput"
	case program.ProblemOutput:
		return "ProblemOutput"
	case program.ProblemFold:
		return "ProblemFold"
	case program.ProblemCarry:
		return "ProblemCarry"
	case program.ProblemTransport:
		return "ProblemTransport"
	default:
		return ""
	}
}

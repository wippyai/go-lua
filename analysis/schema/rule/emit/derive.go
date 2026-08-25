package emit

import (
	"fmt"
	"go/token"
	"unicode"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// shape is the emitted execution shape of one declaration. It is derived from
// the declared publication mode and carry disposition, never from a probe over
// read counts or read forms: the geometry an emitted family implements is the
// geometry its declaration states, so nothing here may narrow it.
type shape uint8

const (
	shapeInvalid shape = iota
	// shapeCarry publishes one exact fact at the row's own coordinate and
	// moves every carried coordinate through the candidate's declared
	// transition.
	shapeCarry
	// shapeExactFold reads one exact foreign fact, applies the declared typed
	// reducer, and publishes at the exact destination projected from the
	// candidate. It covers both consumer normal forms. When the candidate
	// belongs to another axis the installer also owns the construction-time
	// projection, because it is the only sealed object holding both schemas,
	// and the written Factor reaches the row through its identity carry. When
	// the candidate belongs to the written axis that axis's own relation owner
	// projects the row, and the publication carries nothing.
	shapeExactFold
	// shapeSelectedRoute publishes one fact per selected route of a dependent
	// relation the declaration derives.
	shapeSelectedRoute
	// shapeSelectedExact publishes ONE fact at the candidate's own coordinate,
	// concluded over the whole of a dependent selection the declaration
	// derives. It is the routed form's read half joined to the exact form's
	// write half: the selection reaches the fold as one argument rather than
	// as a cadence, because this row concludes one fact FROM every member
	// instead of one fact AT each.
	shapeSelectedExact
	// shapeStructural settles one candidate branch of its trigger at a time
	// and publishes the ordinals of those that activated. It writes no fact at
	// all: its output is the activation row set its branches mount, so it
	// seals no write primitive and its fold declares no output carrier.
	shapeStructural
)

// axisPlan is one axis's emitted vocabulary: the Go types and owner symbols
// the emitted family names that axis by. Every field is the axis's own
// declaration, resolved through the member roster.
type axisPlan struct {
	key        schema.Key
	source     definition.Definition
	schemaType definition.GoType
	fact       definition.GoType
	dense      definition.GoType
	normalizer definition.GoSymbol
	param      string
}

// candidatePlan is the owner-issued directory one rule draws its candidates
// from, and the direct symbol that reads one dense position of it.
type candidatePlan struct {
	axis     *axisPlan
	relation definition.Relation
	subject  definition.GoType
	at       definition.GoSymbol
}

// derivationArg is one ordered argument of a dependent relation's authored
// Build, resolved from the join's own declared source list.
type derivationArg struct {
	candidate bool
	join      *joinPlan
	many      bool
	// form is the delivery the relation input declares for a many-valued
	// position: a selection hands over its tagged cells, a whole-vector read
	// hands over one vector.
	form member.ReadForm
}

// derivationPlan is the direct-call construction of one dependent relation.
type derivationPlan struct {
	state      definition.GoType
	build      definition.GoSymbol
	count      definition.GoSymbol
	at         definition.GoSymbol
	staticAxes []*axisPlan
	arguments  []derivationArg
}

// vectorSpanPlan is the delivery of one Summary read over a self-provided
// nested member set. The set's own MemberCount/MemberAt enumerate it off the
// parent candidate row, and the join's key projection addresses each member at
// its own coordinate, so the vector is sealed one exact read per ordinal
// rather than opened as a Factor cursor.
// vectorSpanPlan marks one Summary join as delivered cell by cell over an
// ordered span. Which addressing produced that span - a nested member set of
// the read's own axis, or a key vector the candidate publishes - is settled
// when the join is derived; by delivery time both are the same fact, the
// ordered coordinates this read is taken over, and the engine has already
// lowered them onto the plan row.
type vectorSpanPlan struct{}

// joinPlan is one declared read, resolved to the primitive that seals it and
// the owner rows that address its relation.
type joinPlan struct {
	position     int
	axis         *axisPlan
	foreign      bool
	read         program.ReadDecl
	relation     definition.Relation
	key          definition.Projection
	predicate    definition.Projection
	hasPredicate bool
	derivation   *derivationPlan
	vectorSpan   *vectorSpanPlan
	name         string
}

// branchPlan is the declared branch set of a structural publication: the join
// whose cold members are the candidate branches this row settles.
type branchPlan struct {
	join *joinPlan
}

// carryPlan is the declared whole-output carry: the input port it names and
// the candidate-indexed transition it applies.
type carryPlan struct {
	input     uint32
	transform definition.CarryTransform
}

// exactPlan is the consumer-owned destination of one heterogeneous exact
// fold.  Its accessor is applied to the candidate row and its result is
// normalized by the written axis; neither owner can perform both halves.
type exactPlan struct {
	destination definition.Projection
	// candidateOwned states that the candidate directory belongs to the axis
	// the rule writes, so the destination is projected by that axis's own
	// relation owner. The heterogeneous rule is the other case: it projects
	// its own destination at construction, because no single owner holds both
	// the candidate directory and the output key.
	candidateOwned bool
	// carried states whether the declaration names an identity carry. It is
	// restated by the emitted fence rather than required by it: a rule
	// publishing at its own candidate's coordinate has a predecessor world to
	// retain only if it says so.
	carried bool
}

// foldPlan is the declared reducer and the call shape its declaration derives.
type foldPlan struct {
	reducer   definition.Reducer
	arguments []definition.Argument
	results   []definition.GoType
	inputs    []*joinPlan
	state     *foldStatePlan
}

// foldStatePlan is the install-time construction of one reducer's sealed
// state: the type the installed family holds, the symbol that seals it, and
// the axes whose cold schemas it is sealed from.
//
// It is what lets a judgment that rests on those schemas stay a call over
// carriers. The state is the family's, so it is built once when the family is
// installed and read by every invocation; the schemas themselves never reach
// the fold's parameter vector.
type foldStatePlan struct {
	state      definition.GoType
	build      definition.GoSymbol
	staticAxes []*axisPlan
}

// routePlan is the selected join a routed output publishes through.
type routePlan struct {
	join            *joinPlan
	slot            uint16
	destination     definition.Projection
	destinationType definition.GoType
	// carry is the transition each published row moves the image at its own
	// destination through. It is nil for a declaration with no carry and for
	// an identity carry, which is the trivial closure and needs no vector.
	carry *definition.CarryTransform
	// carryMode is what the declaration said, retained separately because the
	// emitted fence restates the declared mode and the trivial closure is a
	// declared answer rather than the absence of one.
	carryMode program.CarryMode
}

// plan is one rule's complete emission plan.
type plan struct {
	target    Target
	shape     shape
	form      string
	write     *axisPlan
	candidate candidatePlan
	joins     []*joinPlan
	fold      foldPlan
	carry     *carryPlan
	exact     *exactPlan
	route     *routePlan
	// selection is the dependent selected join an exact conclusion is folded
	// over. It is nil for every other shape.
	selection  *joinPlan
	branch     *branchPlan
	outputSlot uint16
	inputPorts int
	axes       []*axisPlan
	// familyAxes is the subset of axes the installed family retains: the
	// static axes a declared relation derives against, and the axis whose key
	// normalizer resolves a derived route to a dense coordinate. Every other
	// axis is the installer's alone, so the family holds no schema it never
	// reads on an invocation path.
	familyAxes []*axisPlan
	// deliveredFact and deliveredTag are the fold inputs the execution
	// primitive itself hands over, keyed by fold input position. An input the
	// primitive delivers is never a field of the emitted reducer: sealing it
	// per row would be a second copy of the cell the fold already carries.
	deliveredFact map[int]string
	deliveredTag  map[int]string
	// deliveredRoute is the owner-issued destination carrier delivered beside
	// the selected cell. It is populated from the same relation member as the
	// RouteMember and never reconstructed from a dense coordinate or tag.
	deliveredRoute map[int]string
	imports        *importSet
}

// derive resolves one rule declaration against the axis member roster into the
// emission plan. Every refusal names the declared clause that has no emitted
// form.
func derive(target Target, roster definition.Roster) (*plan, error) {
	ruleKey := target.Spec.Key
	sources, err := composeRoster(ruleKey, roster)
	if err != nil {
		return nil, err
	}
	declaration := target.Spec.Program
	if !declaration.Available() {
		return nil, unexpressible(ruleKey, "an incomplete Program", "the declaration does not seal")
	}
	built := &plan{target: target, imports: newImportSet(target.PackagePath)}
	resolver := &axisResolver{rule: ruleKey, sources: sources, byKey: map[schema.Key]*axisPlan{}}

	write, err := resolver.axis(target.Spec.Writes)
	if err != nil {
		return nil, err
	}
	built.write = write

	if len(declaration.Fold.Outputs) != 1 {
		return nil, unexpressible(ruleKey, fmt.Sprintf("%d output columns", len(declaration.Fold.Outputs)),
			"an emitted family publishes exactly one column, because one row settles one disposition")
	}
	output := declaration.Fold.Outputs[0]
	if output.Column.Axis.Key != write.key {
		return nil, unexpressible(ruleKey, "an output column of an axis the rule does not write",
			fmt.Sprintf("column axis %q, written axis %q", string(output.Column.Axis.Key), string(write.key)))
	}
	built.outputSlot = output.ValueSlot

	if err := deriveCandidate(built, resolver, declaration); err != nil {
		return nil, err
	}
	if err := deriveJoins(built, resolver, declaration); err != nil {
		return nil, err
	}
	if err := deriveShape(built, resolver, declaration, output); err != nil {
		return nil, err
	}
	if err := deriveFold(built, resolver, declaration); err != nil {
		return nil, err
	}
	deriveDelivery(built)
	built.inputPorts = inputPorts(declaration)
	built.axes = resolver.ordered()
	built.familyAxes = familyAxes(built)
	return built, nil
}

// deriveDelivery states which fold inputs the execution primitive hands the
// reducer directly. A carry fold delivers its one exact cell; an exact product
// delivers one cell per declared read of the product it drains; a routed fold
// delivers the selected cell and the tag it was observed under.
func deriveDelivery(built *plan) {
	built.deliveredFact = map[int]string{}
	built.deliveredTag = map[int]string{}
	built.deliveredRoute = map[int]string{}
	switch built.shape {
	case shapeCarry:
		if len(built.fold.inputs) == 1 {
			built.deliveredFact[0] = "cell"
		}
	case shapeExactFold:
		// The product cursor materializes every read's cell for one common
		// refinement cell, so each declared read is delivered by the
		// invocation and none of them is sealed into the reducer.
		for position, join := range built.fold.inputs {
			built.deliveredFact[position] = exactCellName(join)
		}
	case shapeSelectedExact:
		// The selection is delivered whole; every other declared read is a
		// prerequisite the worker takes into a local and seals into the fold.
		for position, join := range built.fold.inputs {
			if join == built.selection {
				built.deliveredFact[position] = "cells"
			}
		}
	case shapeSelectedRoute:
		for position, join := range built.fold.inputs {
			if join == built.route.join {
				built.deliveredFact[position] = "cell.Value"
				built.deliveredTag[position] = "cell.Tag"
				built.deliveredRoute[position] = "routeCoordinate"
			}
		}
	case shapeStructural:
		// The branch is delivered one member at a time, tagged by the ordinal
		// its parent addresses it at - the only address a cold member set's
		// rows have.
		for position, join := range built.fold.inputs {
			if join == built.branch.join {
				built.deliveredFact[position] = "cell"
				built.deliveredTag[position] = "branch"
			}
		}
	}
}

// familyAxes is the schema set the installed family retains.
func familyAxes(built *plan) []*axisPlan {
	retained := make([]*axisPlan, 0, len(built.axes))
	seen := map[schema.Key]struct{}{}
	keep := func(axis *axisPlan) {
		if axis == nil {
			return
		}
		if _, present := seen[axis.key]; present {
			return
		}
		seen[axis.key] = struct{}{}
		retained = append(retained, axis)
	}
	for _, join := range built.joins {
		if join.derivation == nil {
			continue
		}
		for _, static := range join.derivation.staticAxes {
			keep(static)
		}
	}
	if built.route != nil {
		keep(built.route.join.axis)
	}
	ordered := make([]*axisPlan, 0, len(retained))
	for _, axis := range built.axes {
		if _, present := seen[axis.key]; present {
			ordered = append(ordered, axis)
		}
	}
	return ordered
}

// inputPorts is the sealed contiguous input-port count of a declaration: the
// widest port any read or carry names, plus one. It is what the emitted family
// answers as its input capacity, so the run's port vector is the declaration's
// own width rather than a count of reads.
func inputPorts(declaration program.Program) int {
	widest := -1
	for _, join := range declaration.Joins {
		if port := int(join.Read.Input); port > widest {
			widest = port
		}
	}
	if declaration.Carry != nil {
		if port := int(declaration.Carry.Input); port > widest {
			widest = port
		}
	}
	return widest + 1
}

func deriveCandidate(built *plan, resolver *axisResolver, declaration program.Program) error {
	ruleKey := built.target.Spec.Key
	// An issued-row candidate is reached from the issuance surface rather than
	// from an axis directory, and no axis owner declares a dense accessor for
	// one. It is refused by name rather than emitted against a directory the
	// declaration does not name.
	if declaration.Candidate.Issued() {
		return unexpressible(ruleKey, "an issued-row candidate",
			fmt.Sprintf("candidate rows come from issuance relation %q, for which no axis owner declares a dense accessor", string(declaration.Candidate.IssuedRow)))
	}
	axis, err := resolver.axis(declaration.Candidate.AxisRelation.Axis.Key)
	if err != nil {
		return err
	}
	relation, relationOK := findRelation(axis.source, declaration.Candidate.AxisRelation.Member)
	if !relationOK {
		return unexpressible(ruleKey, "a candidate relation its axis does not declare",
			fmt.Sprintf("relation %q is not a member row of axis %q", string(declaration.Candidate.AxisRelation.Member), string(axis.key)))
	}
	if !relation.CandidateAt.Available() {
		return unexpressible(ruleKey, "a candidate directory with no dense accessor",
			fmt.Sprintf("relation %q declares no CandidateAt symbol, so one dense candidate cannot be read", relation.Name))
	}
	subject, subjectOK := carrierType(axis.source, relation.Subject)
	if !subjectOK {
		return unexpressible(ruleKey, "a candidate relation whose subject carrier is undeclared", relation.Subject)
	}
	built.candidate = candidatePlan{axis: axis, relation: relation, subject: subject, at: relation.CandidateAt}
	return nil
}

func deriveJoins(built *plan, resolver *axisResolver, declaration program.Program) error {
	ruleKey := built.target.Spec.Key
	for position, join := range declaration.Joins {
		axis, err := resolver.axis(join.Read.Axis.EntryReference().Key)
		if err != nil {
			return err
		}
		relationAxis, err := resolver.axis(join.Relation.Axis.Key)
		if err != nil {
			return err
		}
		relation, relationOK := findRelation(relationAxis.source, join.Relation.Member)
		if !relationOK {
			return unexpressible(ruleKey, "a join relation its axis does not declare",
				fmt.Sprintf("relation %q is not a member row of axis %q", string(join.Relation.Member), string(relationAxis.key)))
		}
		key, keyOK := findProjection(relationAxis.source, join.Key.Member)
		if !keyOK {
			return unexpressible(ruleKey, "a join key its axis does not declare", string(join.Key.Member))
		}
		row := &joinPlan{
			position: position,
			axis:     axis,
			foreign:  axis.key != built.write.key,
			read:     join.Read,
			relation: relation,
			key:      key,
			name:     fmt.Sprintf("read%d", position),
		}
		if join.Predicate.Declared() {
			predicate, predicateOK := findProjection(relationAxis.source, join.Predicate.Member)
			if !predicateOK {
				return unexpressible(ruleKey, "a join predicate its axis does not declare", string(join.Predicate.Member))
			}
			row.predicate, row.hasPredicate = predicate, true
		}
		if relation.Derivation.State.Available() {
			derivation, err := deriveRelation(built, resolver, join, relation, position)
			if err != nil {
				return err
			}
			row.derivation = derivation
		} else if relation.MemberParent.Available() {
			vectorSpan, err := deriveMemberSet(built, declaration, join, relation, relationAxis, position)
			if err != nil {
				return err
			}
			row.vectorSpan = vectorSpan
		} else if join.KeyVector.Declared() {
			vector, err := deriveKeyVector(built, join, relation, relationAxis, position)
			if err != nil {
				return err
			}
			row.vectorSpan = vector
		}
		built.joins = append(built.joins, row)
	}
	return nil
}

// deriveKeyVector resolves one Summary read spanned by the key vector its
// candidate publishes. It states the same fact deriveMemberSet does - the
// ordered denominator this read is taken over - from the other addressing, so
// the emitted delivery below is identical: one exact read per coordinate, in
// the order the row published them, viewed as the vector the fold receives.
//
// What it refuses is the shape whose delivery would be a guess. The span comes
// from the candidate's own row, so a read whose relation is not joined from
// that directory borrows a denominator it has no claim to; and the coordinates
// are of the read axis, so a read of the axis this rule writes has no foreign
// handle to seal them through.
func deriveKeyVector(built *plan, join program.JoinDecl, relation definition.Relation, relationAxis *axisPlan, position int) (*vectorSpanPlan, error) {
	ruleKey := built.target.Spec.Key
	if join.Read.Form != program.Summary {
		return nil, unexpressible(ruleKey, fmt.Sprintf("a %s read spanned by a published key vector", readFormName(join.Read.Form)),
			fmt.Sprintf("join %d takes a whole denominator, which only a Summary read spans", position))
	}
	if relation.CandidateProvider.AxisRelation != join.KeyVector {
		return nil, unexpressible(ruleKey, fmt.Sprintf("join %d whose key vector is not published by the directory it is joined from", position),
			fmt.Sprintf("relation %q is joined from %q", relation.Name, string(relation.CandidateProvider.AxisRelation.Member)))
	}
	if !relationAxis.foreignTo(built.write) {
		return nil, unexpressible(ruleKey, fmt.Sprintf("join %d over a key vector of the written axis", position),
			"a published span is sealed one exact read per coordinate through the read axis's foreign handle, and a rule's own Factor publishes no such handle")
	}
	if _, subjectOK := carrierType(relationAxis.source, relation.Subject); !subjectOK {
		return nil, unexpressible(ruleKey, "a key-vector relation whose subject carrier is undeclared", relation.Subject)
	}
	return &vectorSpanPlan{}, nil
}

// deriveMemberSet resolves one Summary read over a self-provided nested member
// set. Every refusal names the declared clause: the emitter states what the
// declaration says about this relation, and never infers nestedness from the
// join's own shape.
func deriveMemberSet(built *plan, declaration program.Program, join program.JoinDecl, relation definition.Relation, relationAxis *axisPlan, position int) (*vectorSpanPlan, error) {
	ruleKey := built.target.Spec.Key
	if join.Read.Form != program.Summary {
		return nil, unexpressible(ruleKey, fmt.Sprintf("a %s read over a nested member set", readFormName(join.Read.Form)),
			fmt.Sprintf("relation %q is addressed by (parent, ordinal), which is the whole closed denominator a Summary read spans", relation.Name))
	}
	// The declaration restates the relation's own Parent. The restatement is
	// what admits the untagged summary form, so a join that omits it, or one
	// that names a relation the catalog does not agree is the parent, has not
	// stated the fact this delivery rests on.
	if join.Parent != relation.MemberParent {
		return nil, unexpressible(ruleKey, fmt.Sprintf("join %d whose parent restatement disagrees with its relation", position),
			fmt.Sprintf("relation %q declares parent %q", relation.Name, string(relation.MemberParent.Member)))
	}
	// The parent may be a FOREIGN candidate directory. The installer no longer
	// enumerates the set - the engine does, at the row this read is addressed
	// by, and lowers every member's coordinate onto the plan row - so a member
	// set that hangs off another axis's row is sealed exactly like one that
	// hangs off the rule's own. What still has to hold is that the two orders
	// are declared to enumerate the same subjects, which is the plan's
	// correspondence law and not this derivation's to restate.
	if !relation.MemberCount.Available() || !relation.MemberAt.Available() {
		return nil, unexpressible(ruleKey, fmt.Sprintf("join %d over a member set with no census", position),
			fmt.Sprintf("relation %q declares no MemberCount/MemberAt, so its owner publishes no member set for the engine to enumerate", relation.Name))
	}
	if _, parentOK := findRelation(relationAxis.source, join.Parent.Member); !parentOK {
		return nil, unexpressible(ruleKey, "a parent relation its axis does not declare", string(join.Parent.Member))
	}
	if !relationAxis.foreignTo(built.write) {
		return nil, unexpressible(ruleKey, fmt.Sprintf("join %d over a member set of the written axis", position),
			"a member set is sealed one exact read per ordinal through the read axis's foreign handle, and a rule's own Factor publishes no such handle")
	}
	if _, memberOK := carrierType(relationAxis.source, relation.Subject); !memberOK {
		return nil, unexpressible(ruleKey, "a member relation whose subject carrier is undeclared", relation.Subject)
	}
	return &vectorSpanPlan{}, nil
}

// foreignTo reports whether this axis is read through a foreign handle rather
// than through the plane the rule writes.
func (axis *axisPlan) foreignTo(write *axisPlan) bool {
	return axis != nil && write != nil && axis.key != write.key
}

func deriveRelation(built *plan, resolver *axisResolver, join program.JoinDecl, relation definition.Relation, position int) (*derivationPlan, error) {
	ruleKey := built.target.Spec.Key
	derivation := &derivationPlan{
		state: relation.Derivation.State,
		build: relation.Derivation.Build,
		count: relation.Derivation.Count,
		at:    relation.Derivation.At,
	}
	for _, static := range relation.Derivation.StaticAxes {
		axis, err := resolver.axis(static.Key)
		if err != nil {
			return nil, err
		}
		derivation.staticAxes = append(derivation.staticAxes, axis)
	}
	if len(join.Sources) != len(relation.Inputs) {
		return nil, unexpressible(ruleKey, fmt.Sprintf("join %d whose sources and relation inputs disagree", position),
			fmt.Sprintf("the join names %d sources and relation %q declares %d inputs", len(join.Sources), relation.Name, len(relation.Inputs)))
	}
	for index, source := range join.Sources {
		declared := relation.Inputs[index]
		argument := derivationArg{many: declared.Many, form: declared.Form}
		if source.Candidate {
			argument.candidate = true
			if !sameGoType(built.candidate.subject, mustCarrier(built.candidate.axis.source, declared.Carrier)) {
				return nil, unexpressible(ruleKey, fmt.Sprintf("join %d whose candidate source is not the candidate carrier", position),
					fmt.Sprintf("relation input %d is %s", index, declared.Carrier))
			}
		} else {
			if source.Position >= uint64(position) || int(source.Position) >= len(built.joins) {
				return nil, unexpressible(ruleKey, fmt.Sprintf("join %d consuming a later result", position),
					fmt.Sprintf("source %d names join %d", index, source.Position))
			}
			argument.join = built.joins[source.Position]
		}
		derivation.arguments = append(derivation.arguments, argument)
	}
	return derivation, nil
}

func deriveShape(built *plan, resolver *axisResolver, declaration program.Program, output program.OutputDecl) error {
	ruleKey := built.target.Spec.Key
	switch output.Mode {
	case program.ModeRoute:
		if !output.RouteJoinPresent {
			return unexpressible(ruleKey, "a routed output that names no route join",
				"a routed publication is addressed by the join that derives its routes")
		}
		if uint64(output.RouteJoin) >= uint64(len(built.joins)) {
			return unexpressible(ruleKey, "a routed output naming an undeclared join", fmt.Sprintf("join %d", output.RouteJoin))
		}
		route := built.joins[output.RouteJoin]
		if route.read.Form != program.Selected {
			return unexpressible(ruleKey, "a routed output over a non-selected join",
				fmt.Sprintf("join %d is read as %s; a route publishes at the members a selection observed", output.RouteJoin, readFormName(route.read.Form)))
		}
		if route.foreign {
			return unexpressible(ruleKey, "a routed output over a foreign selection",
				"a rule publishes into the Factor it writes, so its route join must name that axis")
		}
		if route.derivation == nil {
			return unexpressible(ruleKey, "a routed output over a join with no relation derivation",
				fmt.Sprintf("relation %q declares no Build/Count/At, so the emitted worker has no route set to observe", route.relation.Name))
		}
		if !route.hasPredicate {
			return unexpressible(ruleKey, "a routed output over an untagged selection",
				fmt.Sprintf("relation %q declares no predicate projection, so a member carries no tag to pair with its cell", route.relation.Name))
		}
		if output.Destination.Axis.Key != route.axis.key {
			return unexpressible(ruleKey, "a routed output whose destination belongs to another axis",
				fmt.Sprintf("destination axis %q, route axis %q", string(output.Destination.Axis.Key), string(route.axis.key)))
		}
		destination, destinationOK := findProjection(route.axis.source, output.Destination.Member)
		if !destinationOK {
			return unexpressible(ruleKey, "a routed output whose destination projection its axis does not declare",
				string(output.Destination.Member))
		}
		if destination.Relation != route.relation.Name || destination.Role != member.Destination {
			return unexpressible(ruleKey, "a routed output whose destination is not a projection of its route relation",
				fmt.Sprintf("projection %q belongs to relation %q, route relation %q", destination.Name, destination.Relation, route.relation.Name))
		}
		destinationType, destinationCarrierOK := carrierType(route.axis.source, destination.Result)
		if !destinationCarrierOK {
			return unexpressible(ruleKey, "a routed output whose destination carrier is undeclared",
				destination.Result)
		}
		keyType, keyCarrierOK := carrierType(route.axis.source, route.axis.source.Binding.Key.Carrier)
		if !keyCarrierOK || !sameGoType(destinationType, keyType) {
			return unexpressible(ruleKey, "a routed output whose destination carrier is not the routed axis key type",
				fmt.Sprintf("destination %s, routed key %s", destinationType.Name, keyType.Name))
		}
		rowCarry, err := deriveRoutedCarry(built, resolver, declaration, route)
		if err != nil {
			return err
		}
		built.shape, built.form = shapeSelectedRoute, "FormSelectedRoute"
		routeCarryMode := program.CarryMode(0)
		if declaration.Carry != nil {
			routeCarryMode = declaration.Carry.Mode
		}
		built.route = &routePlan{join: route, slot: output.ValueSlot, destination: destination, destinationType: destinationType, carry: rowCarry, carryMode: routeCarryMode}
		return nil
	case program.ModeExact:
		if declaration.Carry == nil || declaration.Carry.Mode != program.CarryTransform {
			destinationAxis, err := resolver.axis(output.Destination.Axis.Key)
			if err != nil {
				return err
			}
			if destinationAxis.key != built.write.key {
				return unexpressible(ruleKey, "an authored exact destination declared by an axis the rule does not write",
					fmt.Sprintf("destination axis %q, written axis %q", string(destinationAxis.key), string(built.write.key)))
			}
			// Which owner answers the destination is the declaration's own
			// statement of where its candidate lives. A candidate on ANOTHER
			// axis has no owner holding both the directory and the output key,
			// so the emitted installer projects the row itself, and the
			// written Factor reaches a coordinate that belongs to no directory
			// of its own through the declared identity carry. A candidate on
			// the WRITTEN axis has such an owner - its own - so the row
			// publishes at the coordinate that owner already projects, and
			// what it carries is whatever the declaration says it carries,
			// including nothing at all.
			candidateOwned := destinationAxis.key == built.candidate.axis.key
			if !candidateOwned && (declaration.Carry == nil || declaration.Carry.Mode != program.CarryIdentity) {
				return unexpressible(ruleKey, "an authored exact output with no identity carry",
					"a heterogeneous exact fold retains the written Factor through its declared identity carry")
			}
			if len(built.joins) == 0 {
				return unexpressible(ruleKey, "an authored exact fold over no read",
					"an exact fold reduces the cells its declared reads observe, and a rule that reads nothing writes its own source column instead")
			}
			exact, selected := 0, 0
			var selection *joinPlan
			for _, join := range built.joins {
				switch join.read.Form {
				case program.Exact:
					exact++
				case program.Selected:
					selected++
					selection = join
				case program.Summary:
					// A whole vector over a closed span is a settled partition
					// like an exact cell: its width and order were fixed by
					// the span itself before this rule ran, so folding it
					// needs no relation derived per invocation. A vector with
					// no declared span is the one this form cannot fold - it
					// has no denominator at all.
					if join.vectorSpan == nil {
						return unexpressible(ruleKey, "an exact fold over a vector with no span",
							fmt.Sprintf("join %d declares neither a member set nor a published key vector, so its width is stated nowhere", join.position))
					}
					exact++
				default:
					return unexpressible(ruleKey, fmt.Sprintf("an exact fold beside a %s read", readFormName(join.read.Form)),
						fmt.Sprintf("join %d is neither exact nor selected, and an exact conclusion is folded over cells its reads observed", join.position))
				}
			}
			destination, destinationOK := findProjection(destinationAxis.source, output.Destination.Member)
			if !destinationOK {
				return unexpressible(ruleKey, "an exact destination its output axis does not declare", string(output.Destination.Member))
			}
			if destination.CandidateProvider != declaration.Candidate || !sameGoType(destination.Accessor.Receiver, built.candidate.subject) {
				return unexpressible(ruleKey, "an exact destination not projected from its declared candidate",
					fmt.Sprintf("projection %q is not a typed projection of %q", destination.Name, built.candidate.relation.Name))
			}
			if destination.Result != built.write.source.Binding.Key.Carrier {
				return unexpressible(ruleKey, "an exact destination that does not publish the output axis key",
					fmt.Sprintf("projection %q publishes %s, output key is %s", destination.Name, destination.Result, built.write.source.Binding.Key.Carrier))
			}
			built.exact = &exactPlan{destination: destination, candidateOwned: candidateOwned, carried: declaration.Carry != nil}
			if selected == 0 {
				built.shape, built.form = shapeExactFold, "FormExact"
				return nil
			}
			// A conclusion over a selection is one exact prerequisite and the
			// selection it derives. Two selections would be two member sets
			// with no declared correlation between them, and no prerequisite
			// at all would leave the derivation nothing to derive FROM.
			if exact != 1 || selected != 1 {
				return unexpressible(ruleKey, fmt.Sprintf("an exact conclusion over %d exact and %d selected reads", exact, selected),
					"a conclusion over a selection derives its members from exactly one exact prerequisite")
			}
			if selection.foreign {
				return unexpressible(ruleKey, "an exact conclusion over a foreign selection",
					"a rule observes members of the Factor it writes, so its selected join must name that axis")
			}
			if selection.derivation == nil {
				return unexpressible(ruleKey, "an exact conclusion over a selection with no relation derivation",
					fmt.Sprintf("relation %q declares no Build/Count/At, so the emitted worker has no member set to observe", selection.relation.Name))
			}
			if !selection.hasPredicate {
				return unexpressible(ruleKey, "an exact conclusion over an untagged selection",
					fmt.Sprintf("relation %q declares no predicate projection, so a member carries no tag to pair with its cell", selection.relation.Name))
			}
			built.shape, built.form = shapeSelectedExact, "FormSelectedExact"
			built.selection = selection
			return nil
		}
		transformAxis, err := resolver.axis(declaration.Carry.Transform.Axis.Key)
		if err != nil {
			return err
		}
		transform, transformOK := findCarryTransform(transformAxis.source, declaration.Carry.Transform.Member)
		if !transformOK {
			return unexpressible(ruleKey, "a carry transform its axis does not declare", string(declaration.Carry.Transform.Member))
		}
		if !transform.Implementation.Available() || transform.Implementation.Receiver.Name == "" {
			return unexpressible(ruleKey, "a carry transform with no candidate-bound implementation",
				fmt.Sprintf("transform %q declares no method on the candidate carrier, so the row has no closure to seal", transform.Name))
		}
		if !sameGoType(transform.Implementation.Receiver, built.candidate.subject) {
			return unexpressible(ruleKey, "a carry transform issued by a type that is not the candidate carrier",
				fmt.Sprintf("transform %q is a method on %s, the candidate carrier is %s", transform.Name, transform.Implementation.Receiver.Name, built.candidate.subject.Name))
		}
		for _, join := range built.joins {
			if join.read.Form != program.Exact {
				return unexpressible(ruleKey, fmt.Sprintf("a transformed carry beside a %s read", readFormName(join.read.Form)),
					fmt.Sprintf("join %d is not exact, and the carry fold reduces one exact cell", join.position))
			}
		}
		if len(built.joins) != 1 {
			return unexpressible(ruleKey, fmt.Sprintf("a transformed carry over %d reads", len(built.joins)),
				"FoldCarry reduces exactly one exact cell; a wider carry has no fold in the execution vocabulary")
		}
		built.shape, built.form = shapeCarry, "FormCarry"
		built.carry = &carryPlan{input: uint64ToUint32(uint64(declaration.Carry.Input)), transform: transform}
		return nil
	case program.ModeStructural:
		// A structural publication writes no fact, so there is nothing for a
		// carry to preserve and no coordinate for it to be taken over.
		if declaration.Carry != nil {
			return unexpressible(ruleKey, "a structural output beside a carry",
				"a structural row publishes no Factor, so it has no coordinate a carry could preserve")
		}
		if declaration.Activation == nil {
			return unexpressible(ruleKey, "a structural output with no branch vocabulary",
				"a structural row mounts its branches as owner-issued identities, and a row that names none has not said what it mounts")
		}
		if uint64(declaration.Activation.Branch) >= uint64(len(built.joins)) {
			return unexpressible(ruleKey, "a branch reference naming an undeclared join",
				fmt.Sprintf("join %d", uint64(declaration.Activation.Branch)))
		}
		branch := built.joins[uint64(declaration.Activation.Branch)]
		if branch.vectorSpan == nil || branch.read.Form != program.Summary {
			return unexpressible(ruleKey, "a structural output over a branch set that is not a cold member set",
				fmt.Sprintf("join %d is read as %s over a relation that declares no parent; the construct plane mounts one member per branch before any solve, so a per-invocation selection has no branches to mount",
					branch.position, readFormName(branch.read.Form)))
		}
		if branch.foreign {
			return unexpressible(ruleKey, "a structural output over a foreign branch set",
				"a row's branches hang off its own candidate row, so the branch join names the axis the rule is indexed by")
		}
		built.shape, built.form = shapeStructural, "FormActivation"
		built.branch = &branchPlan{join: branch}
		return nil
	default:
		return unexpressible(ruleKey, fmt.Sprintf("output mode %s", outputModeName(output.Mode)),
			"only exact, routed and structural publications have an emitted family")
	}
}

// deriveRoutedCarry admits the carry of a routed publication. The carry of a
// routed row is indexed by the ROW, because a candidate that publishes at N
// derived destinations has N images to carry and N transitions to carry them
// through: asking which of them is "the" carry has no answer. The exact form
// is the same statement at one row, and an identity carry is the trivial
// closure at every row, so it needs no vector and emits the plain routed fold.
//
// A transform is therefore a method on the row of the derived relation that
// produced the route. A transform issued by the CANDIDATE carrier is refused by
// name: it is the shape that has no answer, not a shape this arm has not got to
// yet.
func deriveRoutedCarry(built *plan, resolver *axisResolver, declaration program.Program, route *joinPlan) (*definition.CarryTransform, error) {
	ruleKey := built.target.Spec.Key
	if declaration.Carry == nil {
		return nil, nil
	}
	if uint64(declaration.Carry.Input) != uint64(route.read.Input) {
		return nil, unexpressible(ruleKey, "a routed carry at an input its route is not read at",
			fmt.Sprintf("carry input %d, route read input %d", uint64(declaration.Carry.Input), uint64(route.read.Input)))
	}
	if declaration.Carry.Mode == program.CarryIdentity {
		return nil, nil
	}
	transformAxis, err := resolver.axis(declaration.Carry.Transform.Axis.Key)
	if err != nil {
		return nil, err
	}
	transform, transformOK := findCarryTransform(transformAxis.source, declaration.Carry.Transform.Member)
	if !transformOK {
		return nil, unexpressible(ruleKey, "a carry transform its axis does not declare", string(declaration.Carry.Transform.Member))
	}
	if !transform.Implementation.Available() || transform.Implementation.Receiver.Name == "" {
		return nil, unexpressible(ruleKey, "a carry transform with no row-bound implementation",
			fmt.Sprintf("transform %q declares no method on the route relation's subject, so a published row has no closure to seal", transform.Name))
	}
	subject, subjectOK := carrierType(route.axis.source, route.relation.Subject)
	if !subjectOK {
		return nil, unexpressible(ruleKey, "a route relation whose subject carrier is undeclared", route.relation.Subject)
	}
	if sameGoType(transform.Implementation.Receiver, built.candidate.subject) && !sameGoType(built.candidate.subject, subject) {
		return nil, unexpressible(ruleKey, "a routed carry indexed by the candidate",
			fmt.Sprintf("transform %q is a method on the candidate carrier %s, and a candidate that publishes at derived destinations has one image and one transition per row rather than one of each",
				transform.Name, built.candidate.subject.Name))
	}
	if !sameGoType(transform.Implementation.Receiver, subject) {
		return nil, unexpressible(ruleKey, "a routed carry issued by a type that is not the route relation's subject",
			fmt.Sprintf("transform %q is a method on %s, the route rows are %s", transform.Name, transform.Implementation.Receiver.Name, subject.Name))
	}
	fact := built.write.source.Signature.Fact
	if transform.Input != fact || transform.Output != fact {
		return nil, unexpressible(ruleKey, "a routed carry that does not map the written fact",
			fmt.Sprintf("transform %q maps %s to %s, the written fact is %s", transform.Name, transform.Input, transform.Output, fact))
	}
	return &transform, nil
}

func deriveFold(built *plan, resolver *axisResolver, declaration program.Program) error {
	ruleKey := built.target.Spec.Key
	axis, err := resolver.axis(declaration.Fold.Reducer.Axis.Key)
	if err != nil {
		return err
	}
	reducer, reducerOK := findReducer(axis.source, declaration.Fold.Reducer.Member)
	if !reducerOK {
		return unexpressible(ruleKey, "a reducer its axis does not declare", string(declaration.Fold.Reducer.Member))
	}
	arguments, results, derived := axis.source.ReducerSignature(reducer, outcomeType(), cellType(), vectorType())
	if !derived {
		return unexpressible(ruleKey, "a reducer whose declared rows name an undeclared carrier", reducer.Name)
	}
	inputs := make([]*joinPlan, 0, len(declaration.Fold.Inputs))
	for _, input := range declaration.Fold.Inputs {
		if uint64(input) >= uint64(len(built.joins)) {
			return unexpressible(ruleKey, "a fold input naming an undeclared join", fmt.Sprintf("join %d", uint64(input)))
		}
		inputs = append(inputs, built.joins[uint64(input)])
	}
	if len(inputs) != len(reducer.Inputs) {
		return unexpressible(ruleKey, "a fold whose inputs and reducer rows disagree",
			fmt.Sprintf("the fold names %d inputs and reducer %q declares %d", len(inputs), reducer.Name, len(reducer.Inputs)))
	}
	state, err := deriveFoldState(built, resolver, reducer)
	if err != nil {
		return err
	}
	built.fold = foldPlan{reducer: reducer, arguments: arguments, results: results, inputs: inputs, state: state}
	return nil
}

// deriveFoldState resolves the sealed state a reducer's judgment is issued by.
// A reducer that declares none is a free function over its carriers and this
// answers nothing; one that declares a state has its axes resolved here, so
// the schemas it is sealed from are the installer's own parameters.
func deriveFoldState(built *plan, resolver *axisResolver, reducer definition.Reducer) (*foldStatePlan, error) {
	ruleKey := built.target.Spec.Key
	derivation := reducer.Derivation
	receiver := reducer.Implementation.Receiver.Name != ""
	if derivation.Declared() != receiver {
		return nil, unexpressible(ruleKey, "a fold whose sealed state and implementation disagree",
			fmt.Sprintf("reducer %q declares state %t and an implementation issued by a receiver %t; a state is read by the method it issues",
				reducer.Name, derivation.Declared(), receiver))
	}
	if !derivation.Declared() {
		return nil, nil
	}
	if !sameGoType(derivation.State, reducer.Implementation.Receiver) {
		return nil, unexpressible(ruleKey, "a fold whose implementation is issued by a type that is not its declared state",
			fmt.Sprintf("reducer %q declares state %s and a method on %s", reducer.Name, derivation.State.Name, reducer.Implementation.Receiver.Name))
	}
	if len(derivation.StaticAxes) == 0 || !derivation.Build.Available() {
		return nil, unexpressible(ruleKey, "a fold state with no construction",
			fmt.Sprintf("reducer %q declares no Build over static axes, so the installer has no state to seal", reducer.Name))
	}
	axes := make([]*axisPlan, 0, len(derivation.StaticAxes))
	for _, static := range derivation.StaticAxes {
		if static.Surface != schema.SurfaceKindAxis {
			return nil, unexpressible(ruleKey, "a fold state sealed against a surface that is not an axis",
				fmt.Sprintf("reducer %q names %q", reducer.Name, string(static.Key)))
		}
		resolved, err := resolver.axis(static.Key)
		if err != nil {
			return nil, err
		}
		axes = append(axes, resolved)
	}
	return &foldStatePlan{state: derivation.State, build: derivation.Build, staticAxes: axes}, nil
}

// axisResolver resolves each axis the declaration names exactly once and keeps
// the resolution order stable, so the emitted constructor's parameter vector is
// a function of the declaration rather than of map iteration.
type axisResolver struct {
	rule    schema.Key
	sources map[schema.Key]definition.Definition
	byKey   map[schema.Key]*axisPlan
	order   []*axisPlan
}

func (resolver *axisResolver) axis(key schema.Key) (*axisPlan, error) {
	if resolved, present := resolver.byKey[key]; present {
		return resolved, nil
	}
	source, sourceOK := resolver.sources[key]
	if !sourceOK {
		return nil, unexpressible(resolver.rule, "an axis with no member definition", string(key))
	}
	schemaType, schemaOK := definition.AxisSchemaType(source)
	if !schemaOK {
		return nil, unexpressible(resolver.rule, "an axis that declares no schema type",
			fmt.Sprintf("axis %q declares no key normalizer receiver", string(key)))
	}
	fact, factOK := carrierType(source, source.Signature.Fact)
	if !factOK {
		return nil, unexpressible(resolver.rule, "an axis whose fact carrier is undeclared", string(key))
	}
	if fact.PackagePath != schemaType.PackagePath {
		return nil, unexpressible(resolver.rule, "an axis whose schema and fact are issued by different packages",
			fmt.Sprintf("axis %q declares schema %s and fact %s", string(key), schemaType.PackagePath, fact.PackagePath))
	}
	param, paramOK := axisIdentifier(key)
	if !paramOK {
		return nil, unexpressible(resolver.rule, "an axis key that is not a Go identifier", string(key))
	}
	dense, denseOK := definition.DenseCoordinateType(source, denseCoordinateType)
	if !denseOK {
		return nil, unexpressible(resolver.rule, "an axis that publishes no dense coordinate", string(key))
	}
	resolved := &axisPlan{
		key:        key,
		source:     source,
		schemaType: schemaType,
		fact:       fact,
		dense:      dense,
		normalizer: source.Binding.Key.Normalizer,
		param:      param + "Schema",
	}
	resolver.byKey[key] = resolved
	resolver.order = append(resolver.order, resolved)
	return resolved, nil
}

// ordered returns the resolved axes in declaration-resolution order, with the
// written axis first. The written axis leads because it is the one every
// emitted family holds; the rest follow in the order the declaration reached
// them, which is stable across regenerations of one declaration.
func (resolver *axisResolver) ordered() []*axisPlan {
	return append([]*axisPlan(nil), resolver.order...)
}

func composeRoster(ruleKey schema.Key, roster definition.Roster) (map[schema.Key]definition.Definition, error) {
	sources := make(map[schema.Key]definition.Definition, roster.Count())
	for index := 0; index < roster.Count(); index++ {
		source, sourceOK := roster.At(index)
		if !sourceOK {
			return nil, unexpressible(ruleKey, "a member roster that does not enumerate", fmt.Sprintf("source %d", index))
		}
		composed, composedOK := source.Compose()
		if !composedOK {
			return nil, unexpressible(ruleKey, "a member source that does not compose", source.Name)
		}
		sources[composed.Axis] = composed
	}
	if len(sources) == 0 {
		return nil, unexpressible(ruleKey, "an empty member roster", "no axis declares a member vocabulary")
	}
	return sources, nil
}

func findRelation(source definition.Definition, key schema.Key) (definition.Relation, bool) {
	for _, relation := range source.Relations {
		if relation.Key == key {
			return relation, true
		}
	}
	return definition.Relation{}, false
}

func findProjection(source definition.Definition, key schema.Key) (definition.Projection, bool) {
	for _, projection := range source.Projections {
		if projection.Key == key {
			return projection, true
		}
	}
	return definition.Projection{}, false
}

func findReducer(source definition.Definition, key schema.Key) (definition.Reducer, bool) {
	for _, reducer := range source.Reducers {
		if reducer.Key == key {
			return reducer, true
		}
	}
	return definition.Reducer{}, false
}

func findCarryTransform(source definition.Definition, key schema.Key) (definition.CarryTransform, bool) {
	for _, transform := range source.CarryTransforms {
		if transform.Key == key {
			return transform, true
		}
	}
	return definition.CarryTransform{}, false
}

func carrierType(source definition.Definition, name string) (definition.GoType, bool) {
	for _, carrier := range source.Carriers {
		if carrier.Name == name {
			return carrier.Type, true
		}
	}
	return definition.GoType{}, false
}

func mustCarrier(source definition.Definition, name string) definition.GoType {
	carrier, _ := carrierType(source, name)
	return carrier
}

func sameGoType(left, right definition.GoType) bool {
	return left.PackagePath == right.PackagePath && left.Name == right.Name
}

// axisIdentifier lowers one axis key to the Go identifier the emitted family
// names that axis's schema by. An axis key that is not a bare identifier has
// no derived spelling and is refused rather than mangled.
func axisIdentifier(key schema.Key) (string, bool) {
	text := string(key)
	if text == "" || !token.IsIdentifier(text) || token.Lookup(text).IsKeyword() {
		return "", false
	}
	runes := []rune(text)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes), true
}

func uint64ToUint32(value uint64) uint32 { return uint32(value) }

// readFormExpression is the declared read form spelled as its program-package
// constant, used by the emitted installer's shape fence.
func readFormExpression(form member.ReadForm) (string, bool) {
	switch form {
	case member.ReadFormExact:
		return "Exact", true
	case member.ReadFormSelected:
		return "Selected", true
	case member.ReadFormSummary:
		return "Summary", true
	case member.ReadFormComplete:
		return "Complete", true
	default:
		return "", false
	}
}

// readFormName spells one declared read form in a refusal. A refusal that
// printed the ordinal would name the clause in a vocabulary the declaration
// author never wrote.
func readFormName(form member.ReadForm) string {
	if name, named := readFormExpression(form); named {
		return name
	}
	return "undeclared"
}

// outputModeName spells one declared publication mode in a refusal.
func outputModeName(mode program.OutputMode) string {
	switch mode {
	case program.ModeExact:
		return "Exact"
	case program.ModeRoute:
		return "Route"
	case program.ModeStructural:
		return "Structural"
	default:
		return "undeclared"
	}
}

// sparseName spells one declared sparsity in a refusal.
func sparseName(sparse program.Sparse) string {
	switch sparse {
	case program.SparseExplicit:
		return "explicit"
	case program.SparseDefault:
		return "default"
	case program.SparseDense:
		return "dense"
	default:
		return "undeclared"
	}
}

// exactCellName is the invocation-local name one exact read's product cell is
// delivered under. It is keyed by the join's own declared position so a fold
// that consumes a subset of the declared reads still names each cell by the
// read that observed it.
func exactCellName(join *joinPlan) string {
	return fmt.Sprintf("cell%d", join.position)
}

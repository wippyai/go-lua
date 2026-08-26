package relcompile

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/rule/relinput"
)

// Placement is the composition-supplied decision scope of one rule: the scope
// its candidate rows are decided at, and the scope each declared input port
// observes. Scope is composition data rather than rule-declaration data, so it
// is named here instead of being inferred from a read's port ordinal.
type Placement struct {
	Candidate Name
	Ports     []Name
}

// Resolution is one lowered rule declaration: the relational rules the
// compiler consumes, and the placement those rules were lowered at.
//
// The pass that resolves a rule's reads is the pass that knows which decision
// scope its candidate rows are decided at and which one each declared input
// port observes, so the placement is published here rather than rediscovered
// by a later pass.
type Resolution struct {
	Rules []Rule
	// Placed is the rule's placement resolved to issued identities: the
	// candidate scope, and one scope per declared input port in the rule's
	// own port order.
	Placed relinput.Placement
}

// Available reports whether the resolution carries a placed candidate scope.
func (resolution Resolution) Available() bool { return resolution.Placed.Candidate.Available() }

// Resolve lowers one authored rule declaration into the resolved relational
// rules the compiler consumes. Every authored reference resolves through the
// one canonical identity registry; a name no owner installed refuses with the
// rule and the declaration site named.
//
// One publication is one dependency, so a fold that publishes several output
// columns resolves to one rule per column, each named by the column key its
// owner authored.
func Resolve(registry *Registry, spec rule.Spec, placement Placement) (Resolution, error) {
	if registry == nil {
		return Resolution{}, refuse(Site{Path: "registry"}, Name{}, KindOwner, ReasonUnavailable)
	}
	entry := schema.EntryReference{Surface: schema.SurfaceKindRule, Key: spec.Key}
	if !spec.Key.Available() {
		return Resolution{}, refuse(Site{Path: "key"}, Name{Entry: entry}, KindDependency, ReasonUnavailable)
	}
	program := spec.Program
	if !program.Available() {
		return Resolution{}, refuse(Site{Rule: spec.Key, Path: "program"}, Name{Entry: entry}, KindExpression, ReasonUndeclared)
	}
	if program.InputCount() != len(placement.Ports) {
		return Resolution{}, refuse(Site{Rule: spec.Key, Path: "program.inputs"}, Name{Entry: entry}, KindScope, ReasonUndeclared)
	}

	if len(program.Fold.Outputs) == 0 {
		return Resolution{}, refuse(Site{Rule: spec.Key, Path: "program.fold.outputs"}, Name{Entry: entry}, KindPublicationKey, ReasonUndeclared)
	}
	producers := make([]Rule, 0, program.JoinCount())
	resolver := ruleResolver{registry: registry, rule: spec.Key, entry: entry, placement: placement, producers: &producers}
	publications := make([]Publication, len(program.Fold.Outputs))
	for outputIndex, output := range program.Fold.Outputs {
		published, err := resolver.publication(outputIndex, output)
		if err != nil {
			return Resolution{}, err
		}
		publications[outputIndex] = published
		if output.Mode != ruleprogram.ModeRoute {
			continue
		}
		if !output.RouteJoinPresent || uint64(output.RouteJoin) >= uint64(program.JoinCount()) {
			return Resolution{}, refuse(resolver.site(fmt.Sprintf("program.fold.outputs[%d].routeJoin", outputIndex)), Name{Entry: entry}, KindRelation, ReasonUndeclared)
		}
		if resolver.routes == nil {
			resolver.routes = make(map[int]routedOutput)
		}
		routeIndex := int(output.RouteJoin)
		if _, duplicate := resolver.routes[routeIndex]; duplicate {
			return Resolution{}, refuse(resolver.site(fmt.Sprintf("program.fold.outputs[%d].routeJoin", outputIndex)), Name{Entry: entry}, KindRelation, ReasonDuplicateName)
		}
		resolver.routes[routeIndex] = routedOutput{output: output, publication: published, index: outputIndex}
	}
	candidate, err := resolver.candidateRelation(program.Candidate)
	if err != nil {
		return Resolution{}, err
	}
	scope, err := registry.Scope(resolver.site("scope"), placement.Candidate)
	if err != nil {
		return Resolution{}, err
	}

	// Lowering and placing are one act. Every declared input port is resolved
	// here, in the rule's own port order, so the published placement is total
	// over the ports the rule declared and a port naming a scope no owner
	// installed refuses with that port named rather than being left unplaced.
	placed := relinput.Placement{Candidate: scope, Ports: make([]model.ScopeID, 0, len(placement.Ports))}
	for port := range placement.Ports {
		observed, err := registry.Scope(resolver.site(fmt.Sprintf("program.inputs[%d]", port)), placement.Ports[port])
		if err != nil {
			return Resolution{}, err
		}
		placed.Ports = append(placed.Ports, observed)
	}

	joins := make([]JoinSpec, 0, program.JoinCount())
	// joinOccurrences retains every physical read occurrence one authored
	// Program.Join lowered into. A Selected route has both its route and final
	// Factor read; foldSlots resolves the sealed semantic relation/column to the
	// exact final tuple occurrence instead of assuming the first raw join.
	joinOccurrences := make([][]ReadOccurrence, program.JoinCount())
	relations := []Name{candidate}
	for index := 0; index < program.JoinCount(); index++ {
		declaration, ok := program.JoinAt(index)
		if !ok {
			return Resolution{}, refuse(resolver.site(fmt.Sprintf("program.joins[%d]", index)), Name{Entry: entry}, KindRelation, ReasonUndeclared)
		}
		lowered, joined, err := resolver.join(index, declaration, relations)
		if err != nil {
			return Resolution{}, err
		}
		if len(lowered) == 0 {
			return Resolution{}, refuse(resolver.site(fmt.Sprintf("program.joins[%d]", index)), Name{Entry: entry}, KindRelation, ReasonUnlowered)
		}
		occurrences := make([]ReadOccurrence, len(lowered))
		for loweredIndex := range lowered {
			occurrences[loweredIndex] = JoinOccurrence(uint32(len(joins) + loweredIndex))
		}
		joinOccurrences[index] = occurrences
		joins = append(joins, lowered...)
		relations = append(relations, joined)
	}

	if err := resolver.structural(program); err != nil {
		return Resolution{}, err
	}

	operation, err := resolver.operation(program.Fold)
	if err != nil {
		return Resolution{}, err
	}
	operationName := NewName(program.Fold.Reducer.Axis, program.Fold.Reducer.Member)
	semantic, err := registry.SealedSignature(resolver.site("program.fold.reducer"), operationName)
	if err != nil {
		return Resolution{}, err
	}
	candidateID, err := registry.Relation(resolver.site("program.candidate"), candidate)
	if err != nil {
		return Resolution{}, err
	}
	applySlots, err := resolver.foldSlots(program.Fold.Inputs, joinOccurrences, joins, semantic, candidateID)
	if err != nil {
		return Resolution{}, err
	}
	addresses := make([]algebra.OutputAddress, len(program.Fold.Outputs))
	for index, output := range program.Fold.Outputs {
		address, addressErr := resolver.outputAddress(
			fmt.Sprintf("program.fold.outputs[%d].address", index),
			output, publications[index], semantic, applySlots, joinOccurrences, program.Candidate, candidateID, joins,
		)
		if addressErr != nil {
			return Resolution{}, addressErr
		}
		addresses[index] = address
	}

	rules := make([]Rule, 0, len(program.Fold.Outputs)+len(producers))
	rules = append(rules, producers...)
	for index, output := range program.Fold.Outputs {
		published := publications[index]
		carry, err := resolver.carry(program.Carry, published)
		if err != nil {
			return Resolution{}, err
		}
		name := NewName(entry, output.Column.Key)
		dependency, err := registry.Dependency(resolver.site(fmt.Sprintf("program.fold.outputs[%d].column", index)), name)
		if err != nil {
			return Resolution{}, err
		}
		expression, err := registry.Expression(resolver.site(fmt.Sprintf("program.fold.outputs[%d].column", index)), name)
		if err != nil {
			return Resolution{}, err
		}
		rules = append(rules, Rule{
			ID:         dependency,
			Expression: expression,
			Candidate:  candidateID,
			Joins:      joins,
			ApplySlots: applySlots,
			Scope:      scope,
			Apply:      operation,
			Output:     addresses[index],
			Carry:      carry,
			Publish:    &published,
		})
	}
	return Resolution{Rules: rules, Placed: placed}, nil
}

// ruleResolver carries the rule identity every refusal names.
type ruleResolver struct {
	registry  *Registry
	rule      schema.Key
	entry     schema.EntryReference
	placement Placement
	// produced collects the dependencies that publish the rows this rule's
	// selected reads consume. They are rules like any other and are returned
	// beside the reading rule.
	producers *[]Rule
	routes    map[int]routedOutput
}

type routedOutput struct {
	output      ruleprogram.OutputDecl
	publication Publication
	index       int
}

func (resolver ruleResolver) produce(rule Rule) {
	*resolver.producers = append(*resolver.producers, rule)
}

func (resolver ruleResolver) site(path string) Site {
	return Site{Rule: resolver.rule, Path: path}
}

// candidateRelation resolves the authored candidate authority. Both arms name
// a declared relation: an axis-owned relation directly, an issuance relation
// through the issuance surface that publishes its rows.
func (resolver ruleResolver) candidateRelation(candidate member.CandidateRef) (Name, error) {
	site := resolver.site("program.candidate")
	if !candidate.Available() {
		return Name{}, refuse(site, Name{Entry: resolver.entry}, KindRelation, ReasonUndeclared)
	}
	if candidate.AxisRelation.Available() {
		name := NewName(candidate.AxisRelation.Axis, candidate.AxisRelation.Member)
		if _, err := resolver.registry.Relation(site, name); err != nil {
			return Name{}, err
		}
		return name, nil
	}
	name := EntryName(schema.SurfaceKindIssuance, candidate.IssuedRow)
	if _, err := resolver.registry.Relation(site, name); err != nil {
		return Name{}, err
	}
	return name, nil
}

// join lowers one authored relation declaration into one oriented equijoin.
//
// The pairing is the only one both sides of which the declaration names: the
// source relation's declared address column against the joined relation's
// authored key projection. Every further authored addressing role names one
// side alone, so it refuses here rather than being paired by inference.
func (resolver ruleResolver) join(index int, declaration ruleprogram.JoinDecl, relations []Name) ([]JoinSpec, Name, error) {
	path := fmt.Sprintf("program.joins[%d]", index)
	joined := NewName(declaration.Relation.Axis, declaration.Relation.Member)
	joinedID, err := resolver.registry.Relation(resolver.site(path+".relation"), joined)
	if err != nil {
		return nil, Name{}, err
	}
	portScope, err := resolver.portScope(path, declaration.Read.Input)
	if err != nil {
		return nil, Name{}, err
	}
	completion, err := resolver.completion(path, declaration.Read.Contract)
	if err != nil {
		return nil, Name{}, err
	}
	// A routed Selected read is two explicit correlations in one left-deep
	// tuple: candidate -> route, then route.destination -> the read axis's
	// Factor denominator. The first relation is the route owner R; the final
	// relation is the Factor A that supplies the selected fact and must also be
	// the output writer W. This is deliberately not a second Apply child.
	if declaration.Read.Form == ruleprogram.Selected {
		if route, routed := resolver.routes[index]; routed {
			return resolver.selectedRoute(index, declaration, joined, joinedID, relations, portScope, completion, route)
		}
	}

	// A selection produces the rows it returns: which rows those are depends
	// on the values the reads before it delivered, so it is an operation that
	// publishes them and not a column vector anything could pair against. A
	// read that names its operation lowers the way the raw indexed access
	// plans do - one Apply publishing into the read relation, then an ordinary
	// equijoin onto the tag it stamps. A read that names none refuses here
	// rather than being paired by a guess.
	if declaration.Produced() {
		if !declaration.Selection.Available() {
			site := path + ".predicate"
			if !declaration.Predicate.Available() {
				site = path + ".sources"
			}
			return nil, Name{}, refuse(resolver.site(site), joined, KindOperation, ReasonUndeclared)
		}
		return resolver.produced(index, declaration, joined, joinedID, relations, portScope, completion)
	}
	// A publisher-owned key vector expands through two facts that an ordinary
	// equijoin cannot state: candidate-to-publisher correspondence, then each
	// emitted semantic key to the reader's Key column. The model-owned Expand
	// contract carries those identities; no coordinate parent/ordinal or
	// publisher address is substituted for the reader key.
	if declaration.KeyVector.Available() {
		// Expand has one implemented logical delivery form: the publisher's
		// authored sparse vector is redeemed in that same order.  A different
		// read ordering or a denominator-backed completion is not an alias for
		// this operation; reject it before constructing the contract rather than
		// dropping the declaration at mount.
		if declaration.Read.Contract.Order != ruleprogram.OrderCanonical {
			return nil, Name{}, refuse(resolver.site(path+".keyVector.order"), joined, KindExpand, ReasonUnlowered)
		}
		if completion != nil {
			return nil, Name{}, refuse(resolver.site(path+".keyVector.completion"), joined, KindExpand, ReasonUnlowered)
		}
		publisher := NewName(declaration.KeyVector.Axis, declaration.KeyVector.Member)
		key := NewName(declaration.Key.Axis, declaration.Key.Member)
		candidate := relations[0]
		contract, err := resolver.registry.ExpandContract(
			resolver.site(path+".keyVector"),
			candidate,
			publisher,
			joined,
			key,
			portScope,
		)
		if err != nil {
			return nil, Name{}, err
		}
		return []JoinSpec{{Relation: joinedID, Scope: portScope, Expand: &contract}}, joined, nil
	}
	right, err := resolver.registry.Column(resolver.site(path+".key"), NewName(declaration.Key.Axis, declaration.Key.Member))
	if err != nil {
		return nil, Name{}, err
	}
	if right.Relation() != joinedID {
		return nil, Name{}, refuse(resolver.site(path+".key"), NewName(declaration.Key.Axis, declaration.Key.Member), KindColumn, ReasonForeign)
	}

	// A Selection-absent join consumes every declared source without creating
	// a producer dependency. Each source is an independently authenticated
	// correlation to the owner materialized relation; reducing this to source
	// zero would silently drop a predecessor from Exact/shared-port families.
	// Completion belongs to the final physical component of the one logical
	// read, so a bounded span is closed once rather than once per source.
	specs := make([]JoinSpec, 0, len(declaration.Sources))
	for sourceIndex, source := range declaration.Sources {
		sourceName, err := resolver.sourceRelation(path, source, relations)
		if err != nil {
			return nil, Name{}, err
		}
		left, err := resolver.registry.Addressed(resolver.site(fmt.Sprintf("%s.sources[%d]", path, sourceIndex)), sourceName, CoordinateAddress)
		if err != nil {
			return nil, Name{}, err
		}
		spec := JoinSpec{
			Relation:     joinedID,
			LeftColumns:  []model.ColumnID{left},
			RightColumns: []model.ColumnID{right},
			Scope:        portScope,
		}
		if sourceIndex == len(declaration.Sources)-1 {
			spec.Complete = completion
		}
		specs = append(specs, spec)
	}

	// A nested member set hangs off one parent row. Its child Parent
	// coordinate and its Ordinal are the nested-set address, so this is the
	// only lowering path allowed to use CoordinateParent.
	if declaration.Parent.Available() {
		parentName := NewName(declaration.Parent.Axis, declaration.Parent.Member)
		parentID, err := resolver.registry.Relation(resolver.site(path+".parent"), parentName)
		if err != nil {
			return nil, Name{}, err
		}
		child, err := resolver.registry.Addressed(resolver.site(path+".parent"), joined, CoordinateParent)
		if err != nil {
			return nil, Name{}, err
		}
		parent, err := resolver.registry.Addressed(resolver.site(path+".parent"), parentName, CoordinateAddress)
		if err != nil {
			return nil, Name{}, err
		}
		specs = append(specs, JoinSpec{
			Relation:     parentID,
			LeftColumns:  []model.ColumnID{child},
			RightColumns: []model.ColumnID{parent},
			Scope:        portScope,
		})
	}

	// A corresponded foreign directory is reached under the occurrence both
	// directories are addressed by. The rule projects that identity from its
	// own candidate row and the read relation publishes the occurrence column
	// its rows are enumerated under, so the correspondence is an equijoin.
	if declaration.AddressIdentity.Declared() {
		projection := declaration.AddressIdentity
		if !projection.Available() {
			return nil, Name{}, refuse(resolver.site(path+".addressIdentity"), joined, KindColumn, ReasonUndeclared)
		}
		identity, err := resolver.registry.Column(resolver.site(path+".addressIdentity"), NewName(projection.Axis, projection.Member))
		if err != nil {
			return nil, Name{}, err
		}
		occurrence, err := resolver.registry.Addressed(resolver.site(path+".addressIdentity"), joined, CoordinateOccurrence)
		if err != nil {
			return nil, Name{}, err
		}
		specs = append(specs, JoinSpec{
			Relation:     joinedID,
			LeftColumns:  []model.ColumnID{identity},
			RightColumns: []model.ColumnID{occurrence},
			Scope:        portScope,
		})
	}
	return specs, joined, nil
}

// selectedRoute lowers the one real routed-read geometry. A route row is
// correlated to the candidate by R.address, then its declared destination is
// correlated to the key of Factor(Read.Axis, denominator). The Factor is the
// final joined tuple component, which makes its exact semantic columns
// available to the single Apply child without a Cartesian product.
func (resolver ruleResolver) selectedRoute(index int, declaration ruleprogram.JoinDecl, routeName Name, routeID model.RelationID, relations []Name, portScope model.ScopeID, completion *model.DenominatorRef, routed routedOutput) ([]JoinSpec, Name, error) {
	path := fmt.Sprintf("program.joins[%d]", index)
	if len(relations) == 0 || len(declaration.Sources) == 0 || !declaration.Sources[0].Candidate {
		return nil, Name{}, refuse(resolver.site(path+".sources[0]"), routeName, KindRelation, ReasonUndeclared)
	}
	// A Selection declaration is the only authority to run a producer. A
	// selection-absent routed read is an already-materialized consumer and
	// therefore deliberately emits no second producer dependency.
	if declaration.Produced() {
		operationName := NewName(declaration.Selection.Axis, declaration.Selection.Member)
		operation, err := resolver.registry.Signature(resolver.site(path+".selection"), operationName)
		if err != nil {
			return nil, Name{}, err
		}
		semantic, err := resolver.registry.SealedSignature(resolver.site(path+".selection"), operationName)
		if err != nil {
			return nil, Name{}, err
		}
		producer, err := resolver.selection(index, declaration, routeName, routeID, relations, portScope, operation, semantic.InputLen())
		if err != nil {
			return nil, Name{}, err
		}
		resolver.produce(producer)
	}

	candidateName := relations[0]
	candidateAddress, err := resolver.registry.Addressed(resolver.site(path+".candidate"), candidateName, CoordinateAddress)
	if err != nil {
		return nil, Name{}, err
	}
	routeAddress, err := resolver.registry.Addressed(resolver.site(path+".route"), routeName, CoordinateAddress)
	if err != nil {
		return nil, Name{}, err
	}

	denominatorName := Name{Entry: schema.EntryReference(declaration.Read.Contract.DenominatorRef)}
	denominator, err := resolver.registry.Denominator(resolver.site(path+".read.contract.denominator"), denominatorName)
	if err != nil {
		return nil, Name{}, err
	}
	factor, err := resolver.registry.Factor(resolver.site(path+".read.factor"), declaration.Read.Axis.EntryReference(), denominatorName)
	if err != nil {
		return nil, Name{}, err
	}
	if factor.Relation != denominator.Relation() || factor.Key != denominator.Key() {
		return nil, Name{}, refuse(resolver.site(path+".read.factor"), denominatorName, KindDenominator, ReasonForeign)
	}
	// The output's fact/key are W. The selected input is A. Absent an explicit
	// correspondence contract, a routed fold may not publish a fact from one
	// Factor under another factor's relation or key.
	if factor.Relation != routed.publication.Relation || factor.Key != routed.publication.Key {
		return nil, Name{}, refuse(resolver.site(fmt.Sprintf("program.fold.outputs[%d].column", routed.index)), NewName(routed.output.Column.Axis, routed.output.Column.Key), KindRelation, ReasonForeign)
	}
	if len(factor.Columns) != 1 {
		return nil, Name{}, refuse(resolver.site(path+".read.factor"), denominatorName, KindKey, ReasonUnlowered)
	}

	destinationName := NewName(routed.output.Destination.Axis, routed.output.Destination.Member)
	destination, err := resolver.registry.Column(resolver.site(fmt.Sprintf("program.fold.outputs[%d].destination", routed.index)), destinationName)
	if err != nil {
		return nil, Name{}, err
	}
	if destination.Relation() != routeID {
		return nil, Name{}, refuse(resolver.site(fmt.Sprintf("program.fold.outputs[%d].destination", routed.index)), destinationName, KindColumn, ReasonForeign)
	}
	declaredDestination, err := resolver.registry.Addressed(resolver.site(path+".route.destination"), routeName, CoordinateDestination)
	if err != nil {
		return nil, Name{}, err
	}
	if declaredDestination != destination {
		return nil, Name{}, refuse(resolver.site(path+".route.destination"), destinationName, KindColumn, ReasonForeign)
	}

	return []JoinSpec{
		{
			Relation:     routeID,
			LeftColumns:  []model.ColumnID{candidateAddress},
			RightColumns: []model.ColumnID{routeAddress},
			Scope:        portScope,
		},
		{
			Relation:     factor.Relation,
			LeftColumns:  []model.ColumnID{destination},
			RightColumns: append([]model.ColumnID(nil), factor.Columns...),
			Scope:        portScope,
			Complete:     completion,
		},
	}, routeName, nil
}

// produced lowers one read whose rows an operation publishes. The operation
// becomes its own dependency over the results the read consumes, and the read
// itself is an equijoin onto the tag those rows carry, so nothing about it is
// a form: it is one Apply and one join over declared columns.
func (resolver ruleResolver) produced(index int, declaration ruleprogram.JoinDecl, joined Name, joinedID model.RelationID, relations []Name, portScope model.ScopeID, completion *model.DenominatorRef) ([]JoinSpec, Name, error) {
	path := fmt.Sprintf("program.joins[%d]", index)
	operationName := NewName(declaration.Selection.Axis, declaration.Selection.Member)
	operation, err := resolver.registry.Signature(resolver.site(path+".selection"), operationName)
	if err != nil {
		return nil, Name{}, err
	}
	semantic, err := resolver.registry.SealedSignature(resolver.site(path+".selection"), operationName)
	if err != nil {
		return nil, Name{}, err
	}
	tag, err := resolver.registry.Column(resolver.site(path+".predicate"),
		NewName(declaration.Predicate.Axis, declaration.Predicate.Member))
	if err != nil {
		tag, err = resolver.registry.Addressed(resolver.site(path+".selection"), joined, CoordinateTag)
		if err != nil {
			return nil, Name{}, err
		}
	}
	if tag.Relation() != joinedID {
		return nil, Name{}, refuse(resolver.site(path+".predicate"), joined, KindColumn, ReasonForeign)
	}
	producer, err := resolver.selection(index, declaration, joined, joinedID, relations, portScope, operation, semantic.InputLen())
	if err != nil {
		return nil, Name{}, err
	}
	resolver.produce(producer)

	source := declaration.Sources[0]
	sourceName, err := resolver.sourceRelation(path, source, relations)
	if err != nil {
		return nil, Name{}, err
	}
	left, err := resolver.registry.Addressed(resolver.site(path+".sources[0]"), sourceName, CoordinateAddress)
	if err != nil {
		return nil, Name{}, err
	}
	return []JoinSpec{{
		Relation:     joinedID,
		LeftColumns:  []model.ColumnID{left},
		RightColumns: []model.ColumnID{tag},
		Scope:        portScope,
		Complete:     completion,
	}}, joined, nil
}

// selection builds the dependency that publishes the produced rows: the
// operation applied over the candidate and every earlier result the read
// consumes, publishing into the relation those rows land in.
func (resolver ruleResolver) selection(index int, declaration ruleprogram.JoinDecl, joined Name, joinedID model.RelationID, relations []Name, portScope model.ScopeID, operation signature.Identity, semanticInputs int) (Rule, error) {
	path := fmt.Sprintf("program.joins[%d].selection", index)
	name := NewName(declaration.Selection.Axis, declaration.Selection.Member)
	dependency, err := resolver.registry.Dependency(resolver.site(path), name)
	if err != nil {
		return Rule{}, err
	}
	expression, err := resolver.registry.Expression(resolver.site(path), name)
	if err != nil {
		return Rule{}, err
	}
	candidate, err := resolver.registry.Relation(resolver.site(path), relations[0])
	if err != nil {
		return Rule{}, err
	}
	joins := make([]JoinSpec, 0, len(declaration.Sources))
	applySlots := make([]ReadOccurrence, 0, len(declaration.Sources))
	for position, source := range declaration.Sources {
		if source.Candidate {
			applySlots = append(applySlots, CandidateOccurrence())
			continue
		}
		consumed, err := resolver.sourceRelation(path, source, relations)
		if err != nil {
			return Rule{}, err
		}
		left, err := resolver.registry.Addressed(resolver.site(path), relations[0], CoordinateAddress)
		if err != nil {
			return Rule{}, err
		}
		right, err := resolver.registry.Addressed(resolver.site(path), consumed, CoordinateAddress)
		if err != nil {
			return Rule{}, err
		}
		consumedID, err := resolver.registry.Relation(resolver.site(path), consumed)
		if err != nil {
			return Rule{}, err
		}
		_ = position
		applySlots = append(applySlots, JoinOccurrence(uint32(len(joins))))
		joins = append(joins, JoinSpec{
			Relation:     consumedID,
			LeftColumns:  []model.ColumnID{left},
			RightColumns: []model.ColumnID{right},
			Scope:        portScope,
		})
	}
	if len(applySlots) != semanticInputs {
		return Rule{}, refuse(resolver.site(path+".selection.inputs"), Name{Entry: resolver.entry}, KindSignature, ReasonUndeclared)
	}
	// The operation publishes into the relation the read names, so the key its
	// rows are published under is that relation's own. Reading it off the
	// authored predicate would hold only for a read that declares one, and a
	// read whose rows carry the relation's declared tag coordinate declares
	// none.
	key, err := resolver.registry.RelationPublicationKey(resolver.site(path), joined)
	if err != nil {
		return Rule{}, err
	}
	return Rule{
		ID:         dependency,
		Expression: expression,
		Candidate:  candidate,
		Joins:      joins,
		ApplySlots: applySlots,
		Scope:      portScope,
		Apply:      operation,
		// A generated selection is the owner of the route rows it issues;
		// unlike a terminal fold it has no authored destination projection.
		// Its output geometry is therefore explicitly owner-named, not inferred
		// from the first source row.
		Output:  algebra.OwnerNamed(),
		Publish: &Publication{Relation: joinedID, Key: key},
	}, nil
}

func (resolver ruleResolver) sourceRelation(path string, source ruleprogram.SourceRef, relations []Name) (Name, error) {
	if source.Candidate {
		return relations[0], nil
	}
	position := int(source.Position)
	if position < 0 || position+1 >= len(relations) {
		return Name{}, refuse(resolver.site(path+".sources[0]"), Name{Entry: resolver.entry}, KindRelation, ReasonUndeclared)
	}
	return relations[position+1], nil
}

func (resolver ruleResolver) portScope(path string, port ruleprogram.InputRef) (model.ScopeID, error) {
	index := int(port.Uint64())
	if index < 0 || index >= len(resolver.placement.Ports) {
		return model.ScopeID{}, refuse(resolver.site(path+".input"), Name{Entry: resolver.entry}, KindScope, ReasonUndeclared)
	}
	return resolver.registry.Scope(resolver.site(path+".input"), resolver.placement.Ports[index])
}

// completion maps the authored sparse disposition onto explicit completion.
// An explicitly sparse read closes over nothing and therefore completes
// nothing; a default or dense read closes over its authored denominator.
func (resolver ruleResolver) completion(path string, contract ruleprogram.ReadContract) (*model.DenominatorRef, error) {
	switch contract.Sparse {
	case ruleprogram.SparseExplicit:
		return nil, nil
	case ruleprogram.SparseDefault, ruleprogram.SparseDense:
		name := Name{Entry: schema.EntryReference(contract.DenominatorRef)}
		reference, err := resolver.registry.Denominator(resolver.site(path+".read.contract.denominator"), name)
		if err != nil {
			return nil, err
		}
		return &reference, nil
	default:
		return nil, refuse(resolver.site(path+".read.contract.sparse"), Name{Entry: resolver.entry}, KindDenominator, ReasonUnavailable)
	}
}

func (resolver ruleResolver) operation(fold ruleprogram.FoldDecl) (signature.Identity, error) {
	site := resolver.site("program.fold.reducer")
	name := NewName(fold.Reducer.Axis, fold.Reducer.Member)
	return resolver.registry.Signature(site, name)
}

// foldSlots turns the Program's ordered Fold.Inputs into the exact source
// occurrence for every semantic slot. The reducer signature and Fold.Inputs
// are both positional authority; a mismatch is a declaration error, never an
// excuse to match the same relation or column by name.

func (resolver ruleResolver) foldSlots(inputs []ruleprogram.JoinRef, occurrences [][]ReadOccurrence, joins []JoinSpec, semantic signature.Signature, candidate model.RelationID) ([]ReadOccurrence, error) {
	semanticInputs := semantic.InputLen()
	// A reducer's candidate carrier is an authored direct-call argument, not a
	// Fold.Inputs join: allocationbirth and the other candidate-bearing
	// families name only their explicit read ports in the Program while the
	// generated binding's sealed ABI places Candidate before those reads. The
	// candidate relation identity is checked against slot zero here; no slot is
	// inferred from a same-named relation or a tuple position later.
	candidateImplicit := semanticInputs == len(inputs)+1
	if candidateImplicit {
		input, ok := semantic.InputAt(0)
		if !ok || input.Relation != candidate {
			candidateImplicit = false
		}
	}
	if !candidateImplicit && len(inputs) != semanticInputs {
		return nil, refuse(resolver.site("program.fold.inputs"), Name{Entry: resolver.entry}, KindSignature, ReasonUndeclared)
	}
	result := make([]ReadOccurrence, 0, semanticInputs)
	if candidateImplicit {
		result = append(result, CandidateOccurrence())
	}
	semanticOffset := 0
	if candidateImplicit {
		semanticOffset = 1
	}
	for index, input := range inputs {
		if uint64(input) >= uint64(len(occurrences)) {
			return nil, refuse(resolver.site(fmt.Sprintf("program.fold.inputs[%d]", index)), Name{Entry: resolver.entry}, KindRelation, ReasonUndeclared)
		}
		semanticInput, ok := semantic.InputAt(semanticOffset + index)
		if !ok || semanticInput.Column.Relation() != semanticInput.Relation {
			return nil, refuse(resolver.site(fmt.Sprintf("program.fold.inputs[%d]", index)), Name{Entry: resolver.entry}, KindSignature, ReasonUndeclared)
		}
		var selected ReadOccurrence
		matches := 0
		for _, occurrence := range occurrences[input] {
			joinIndex, joined := occurrence.Join()
			if !joined || int(joinIndex) >= len(joins) || joins[joinIndex].Relation != semanticInput.Relation {
				continue
			}
			selected = occurrence
			matches++
		}
		if matches == 0 {
			return nil, refuse(resolver.site(fmt.Sprintf("program.fold.inputs[%d]", index)), Name{Entry: resolver.entry}, KindRelation, ReasonForeign)
		}
		// Supporting joins may repeat the same relation (for example a keyed
		// read followed by its occurrence correspondence). The final matching
		// component is the one present at the end of that logical read's
		// composite tuple; retaining it avoids the old first-lowered-join
		// alias while still resolving a sealed relation/column, never a name.
		result = append(result, selected)
	}
	return result, nil
}

// structural resolves the branch and transport vocabulary of an activation
// publication. The branch set is the one nested relation the candidate owns;
// the transport vector is not a relation at all, but an ordered list of axis
// owners. Each axis is resolved through the canonical owner directory. No
// relation is looked up or invented for an axis, because that would create a
// second authority for the same activation edge.
func (resolver ruleResolver) structural(program ruleprogram.Program) error {
	if program.Activation != nil {
		branch := NewName(program.Activation.Branch.Axis, program.Activation.Branch.Member)
		if _, err := resolver.registry.Relation(resolver.site("program.activation.branch"), branch); err != nil {
			return err
		}
		if _, err := resolver.registry.Addressed(resolver.site("program.activation.branch"), branch, CoordinateParent); err != nil {
			return err
		}
		if _, err := resolver.registry.Addressed(resolver.site("program.activation.branch"), branch, CoordinateOrdinal); err != nil {
			return err
		}
		for index, declaration := range program.Activation.Transport {
			site := resolver.site(fmt.Sprintf("program.activation.transport[%d].axis", index))
			if _, err := resolver.registry.Owner(site, schema.EntryReference(declaration.Axis)); err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

// carry resolves the authored whole-output carry into the alternative
// derivation the destination key's algebra merges with the operation's rows.
// A carry is not a form: it names the destination relation observed at one
// input port, and an owner-issued transform when the carried fact is
// transformed rather than preserved.
func (resolver ruleResolver) carry(declaration *ruleprogram.CarryDecl, destination Publication) (*CarrySpec, error) {
	if declaration == nil {
		return nil, nil
	}
	if !declaration.Available() {
		return nil, refuse(resolver.site("program.carry"), Name{Entry: resolver.entry}, KindRelation, ReasonUndeclared)
	}
	scope, err := resolver.portScope("program.carry", declaration.Input)
	if err != nil {
		return nil, err
	}
	spec := CarrySpec{Relation: destination.Relation, Scope: scope, Columns: append([]model.ColumnID(nil), destination.Columns...)}
	switch declaration.Mode {
	case ruleprogram.CarryIdentity:
		return &spec, nil
	case ruleprogram.CarryTransform:
		name := NewName(declaration.Transform.Axis, declaration.Transform.Member)
		transform, err := resolver.registry.Signature(resolver.site("program.carry.transform"), name)
		if err != nil {
			return nil, err
		}
		spec.Transform = &transform
		// The carry declaration owns the transform's destination geometry. Keep
		// it opaque and transport it unchanged; selecting a relation/column or
		// dense ordinal here would create a second address vocabulary and would
		// choose the wrong occurrence when a carried relation repeats.
		if !declaration.Output.Available() {
			return nil, refuse(resolver.site("program.carry.output"), Name{Entry: resolver.entry}, KindAddress, ReasonUndeclared)
		}
		spec.Output = declaration.Output
		return &spec, nil
	default:
		return nil, refuse(resolver.site("program.carry.mode"), Name{Entry: resolver.entry}, KindOperation, ReasonUnavailable)
	}
}

func (resolver ruleResolver) publication(index int, output ruleprogram.OutputDecl) (Publication, error) {
	path := fmt.Sprintf("program.fold.outputs[%d]", index)
	if !output.Available() {
		return Publication{}, refuse(resolver.site(path), Name{Entry: resolver.entry}, KindColumn, ReasonUndeclared)
	}
	writer := NewName(output.Column.Axis, output.Column.Key)
	binding, err := resolver.registry.Output(resolver.site(path+".column"), writer)
	if err != nil {
		return Publication{}, err
	}
	return Publication{Relation: binding.Relation, Key: binding.Key, Result: binding.Result, Columns: []model.ColumnID{binding.Column}}, nil
}

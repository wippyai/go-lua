package relcompile

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// Placement is the composition-supplied decision scope of one rule: the scope
// its candidate rows are decided at, and the scope each declared input port
// observes. Scope is composition data rather than rule-declaration data, so it
// is named here instead of being inferred from a read's port ordinal.
type Placement struct {
	Candidate Name
	Ports     []Name
}

// Resolve lowers one authored rule declaration into the resolved relational
// rules the compiler consumes. Every authored reference resolves through the
// one canonical identity registry; a name no owner installed refuses with the
// rule and the declaration site named.
//
// One publication is one dependency, so a fold that publishes several output
// columns resolves to one rule per column, each named by the column key its
// owner authored.
func Resolve(registry *Registry, spec rule.Spec, placement Placement) ([]Rule, error) {
	if registry == nil {
		return nil, refuse(Site{Path: "registry"}, Name{}, KindOwner, ReasonUnavailable)
	}
	entry := schema.EntryReference{Surface: schema.SurfaceKindRule, Key: spec.Key}
	if !spec.Key.Available() {
		return nil, refuse(Site{Path: "key"}, Name{Entry: entry}, KindDependency, ReasonUnavailable)
	}
	program := spec.Program
	if !program.Available() {
		return nil, refuse(Site{Rule: spec.Key, Path: "program"}, Name{Entry: entry}, KindExpression, ReasonUndeclared)
	}
	if program.InputCount() != len(placement.Ports) {
		return nil, refuse(Site{Rule: spec.Key, Path: "program.inputs"}, Name{Entry: entry}, KindScope, ReasonUndeclared)
	}

	producers := make([]Rule, 0, program.JoinCount())
	resolver := ruleResolver{registry: registry, rule: spec.Key, entry: entry, placement: placement, producers: &producers}
	candidate, err := resolver.candidateRelation(program.Candidate)
	if err != nil {
		return nil, err
	}
	scope, err := registry.Scope(resolver.site("scope"), placement.Candidate)
	if err != nil {
		return nil, err
	}

	joins := make([]JoinSpec, 0, program.JoinCount())
	relations := []Name{candidate}
	for index := 0; index < program.JoinCount(); index++ {
		declaration, ok := program.JoinAt(index)
		if !ok {
			return nil, refuse(resolver.site(fmt.Sprintf("program.joins[%d]", index)), Name{Entry: entry}, KindRelation, ReasonUndeclared)
		}
		lowered, joined, err := resolver.join(index, declaration, relations)
		if err != nil {
			return nil, err
		}
		joins = append(joins, lowered...)
		relations = append(relations, joined)
	}

	if err := resolver.structural(program); err != nil {
		return nil, err
	}

	operation, err := resolver.operation(program.Fold)
	if err != nil {
		return nil, err
	}

	if len(program.Fold.Outputs) == 0 {
		return nil, refuse(resolver.site("program.fold.outputs"), Name{Entry: entry}, KindPublicationKey, ReasonUndeclared)
	}
	rules := make([]Rule, 0, len(program.Fold.Outputs)+len(producers))
	rules = append(rules, producers...)
	for index, output := range program.Fold.Outputs {
		published, err := resolver.publication(index, output)
		if err != nil {
			return nil, err
		}
		carry, err := resolver.carry(program.Carry, published.Relation)
		if err != nil {
			return nil, err
		}
		name := NewName(entry, output.Column.Key)
		dependency, err := registry.Dependency(resolver.site(fmt.Sprintf("program.fold.outputs[%d].column", index)), name)
		if err != nil {
			return nil, err
		}
		expression, err := registry.Expression(resolver.site(fmt.Sprintf("program.fold.outputs[%d].column", index)), name)
		if err != nil {
			return nil, err
		}
		candidateID, err := registry.Relation(resolver.site("program.candidate"), candidate)
		if err != nil {
			return nil, err
		}
		rules = append(rules, Rule{
			ID:         dependency,
			Expression: expression,
			Candidate:  candidateID,
			Joins:      joins,
			Scope:      scope,
			Apply:      operation,
			Carry:      carry,
			Publish:    &published,
		})
	}
	return rules, nil
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

	source := declaration.Sources[0]
	sourceName, err := resolver.sourceRelation(path, source, relations)
	if err != nil {
		return nil, Name{}, err
	}
	left, err := resolver.registry.Addressed(resolver.site(path+".sources[0]"), sourceName, CoordinateAddress)
	if err != nil {
		return nil, Name{}, err
	}
	right, err := resolver.registry.Column(resolver.site(path+".key"), NewName(declaration.Key.Axis, declaration.Key.Member))
	if err != nil {
		return nil, Name{}, err
	}
	if right.Relation() != joinedID {
		return nil, Name{}, refuse(resolver.site(path+".key"), NewName(declaration.Key.Axis, declaration.Key.Member), KindColumn, ReasonForeign)
	}

	specs := []JoinSpec{{
		Relation:     joinedID,
		LeftColumns:  []model.ColumnID{left},
		RightColumns: []model.ColumnID{right},
		Scope:        portScope,
		Complete:     completion,
	}}

	// A nested member set hangs off a parent row, and a candidate-published
	// span hangs off the directory that publishes it. Both are ordinary
	// equijoins once the read relation publishes the coordinate that carries
	// its parent's address.
	for _, nesting := range [...]struct {
		field    string
		relation member.RelationRef
	}{
		{field: ".parent", relation: declaration.Parent},
		{field: ".keyVector", relation: declaration.KeyVector},
	} {
		if !nesting.relation.Available() {
			continue
		}
		base := NewName(nesting.relation.Axis, nesting.relation.Member)
		baseID, err := resolver.registry.Relation(resolver.site(path+nesting.field), base)
		if err != nil {
			return nil, Name{}, err
		}
		child, err := resolver.registry.Addressed(resolver.site(path+nesting.field), joined, CoordinateParent)
		if err != nil {
			return nil, Name{}, err
		}
		parent, err := resolver.registry.Addressed(resolver.site(path+nesting.field), base, CoordinateAddress)
		if err != nil {
			return nil, Name{}, err
		}
		specs = append(specs, JoinSpec{
			Relation:     baseID,
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

// produced lowers one read whose rows an operation publishes. The operation
// becomes its own dependency over the results the read consumes, and the read
// itself is an equijoin onto the tag those rows carry, so nothing about it is
// a form: it is one Apply and one join over declared columns.
func (resolver ruleResolver) produced(index int, declaration ruleprogram.JoinDecl, joined Name, joinedID model.RelationID, relations []Name, portScope model.ScopeID, completion *model.DenominatorRef) ([]JoinSpec, Name, error) {
	path := fmt.Sprintf("program.joins[%d]", index)
	operation, err := resolver.registry.Signature(resolver.site(path+".selection"),
		NewName(declaration.Selection.Axis, declaration.Selection.Member))
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
	producer, err := resolver.selection(index, declaration, joined, joinedID, relations, portScope, operation)
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
func (resolver ruleResolver) selection(index int, declaration ruleprogram.JoinDecl, joined Name, joinedID model.RelationID, relations []Name, portScope model.ScopeID, operation signature.Identity) (Rule, error) {
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
	for position, source := range declaration.Sources {
		if source.Candidate {
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
		joins = append(joins, JoinSpec{
			Relation:     consumedID,
			LeftColumns:  []model.ColumnID{left},
			RightColumns: []model.ColumnID{right},
			Scope:        portScope,
		})
	}
	key, err := resolver.registry.PublicationKey(resolver.site(path), NewName(declaration.Predicate.Axis, declaration.Predicate.Member))
	if err != nil {
		return Rule{}, err
	}
	return Rule{
		ID:         dependency,
		Expression: expression,
		Candidate:  candidate,
		Joins:      joins,
		Scope:      portScope,
		Apply:      operation,
		Publish:    &Publication{Relation: joinedID, Key: key},
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

// structural resolves the branch and transport vocabulary of an activation
// publication. A branch set is a nested member set of the rule's own candidate
// row and a transported axis is the relation carried across the activation
// edge, so both are ordinary relations joined by ordinary column vectors. The
// declaration names the branch relation and the transported axes but not the
// column a branch row is addressed by its parent through, nor the relation one
// transported axis crosses the edge as, so each refuses at its own site.
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
	}
	for index := 0; index < program.TransportCount(); index++ {
		declaration, ok := program.TransportAt(index)
		if !ok {
			continue
		}
		site := resolver.site(fmt.Sprintf("program.transport[%d].axis", index))
		return refuse(site, Name{Entry: schema.EntryReference(declaration.Axis)}, KindRelation, ReasonUndeclared)
	}
	return nil
}

// carry resolves the authored whole-output carry into the alternative
// derivation the destination key's algebra merges with the operation's rows.
// A carry is not a form: it names the destination relation observed at one
// input port, and an owner-issued transform when the carried fact is
// transformed rather than preserved.
func (resolver ruleResolver) carry(declaration *ruleprogram.CarryDecl, destination model.RelationID) (*CarrySpec, error) {
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
	spec := CarrySpec{Relation: destination, Scope: scope}
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
	destination := NewName(output.Destination.Axis, output.Destination.Member)
	column, err := resolver.registry.Column(resolver.site(path+".destination"), destination)
	if err != nil {
		return Publication{}, err
	}
	key, err := resolver.registry.PublicationKey(resolver.site(path+".destination"), destination)
	if err != nil {
		return Publication{}, err
	}
	return Publication{Relation: column.Relation(), Key: key}, nil
}

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

	resolver := ruleResolver{registry: registry, rule: spec.Key, entry: entry, placement: placement}
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
	rules := make([]Rule, 0, len(program.Fold.Outputs))
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
	// on values the reads before it delivered, so it is an operation that
	// publishes them and not a column vector anything could pair against. The
	// declaration names the tag such an operation stamps its rows with, and
	// names no operation, so it refuses here rather than being paired by a
	// guess. Its lowering is the one the raw indexed access plans use.
	if declaration.Predicate.Available() {
		return nil, Name{}, refuse(resolver.site(path+".predicate"), joined, KindOperation, ReasonUndeclared)
	}
	if len(declaration.Sources) != 1 {
		return nil, Name{}, refuse(resolver.site(path+".sources"), joined, KindOperation, ReasonUndeclared)
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

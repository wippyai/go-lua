package e2e

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// surface is the declaration side of the demo: the owner statements the value
// chain's rules name, issued under one execution schema identity so the
// certificate checker can admit them.
//
// It differs from the compiler census's surface in the three statements the
// checker constrains and the compiler does not: an axis publishes one value
// lattice rather than one type per column, every sealed signature carries the
// schema identity of the plan it is compiled into, and a signature's inputs
// name the relation the lowered expression delivers.
type surface struct {
	t        *testing.T
	registry *relcompile.Registry
	seen     map[string]bool
	schemaID model.SchemaID
	// lattice is the single semantic type of the axis under demonstration.
	// Two columns joined by an equijoin carry one algebra, so they carry one
	// TypeID.
	lattice relcompile.Name
}

func newSurface(t *testing.T, axis schema.EntryReference) *surface {
	t.Helper()
	value := &surface{t: t, registry: relcompile.NewRegistry(), seen: map[string]bool{}}
	value.owner(axis)
	owner, err := value.registry.Owner(relcompile.Site{Path: "demo"}, axis)
	if err != nil {
		t.Fatalf("resolve demo owner: %v", err)
	}
	schemaID, ok := model.IssueSchemaID(owner, value.token("schema", relcompile.Name{Entry: axis}))
	if !ok {
		t.Fatalf("issue demo schema identity")
	}
	value.schemaID = schemaID
	value.lattice = value.valueType(relcompile.NewName(axis, "demo/lattice"))
	return value
}

func (value *surface) token(kind string, name relcompile.Name) identity.ContentID {
	value.t.Helper()
	token, ok := identity.DeriveContentID("relcompile-e2e/v1", []byte(kind), []byte(name.String()))
	if !ok {
		value.t.Fatalf("derive %s token for %v", kind, name)
	}
	return token
}

func (value *surface) once(kind string, name relcompile.Name) bool {
	marker := kind + "\x00" + name.String()
	if value.seen[marker] {
		return false
	}
	value.seen[marker] = true
	return true
}

func (value *surface) owner(entry schema.EntryReference) {
	value.t.Helper()
	name := relcompile.Name{Entry: entry}
	if !value.once("owner", name) {
		return
	}
	if err := value.registry.InstallOwner(entry, value.token("owner", name)); err != nil {
		value.t.Fatalf("install owner %v: %v", entry, err)
	}
}

func (value *surface) scope(name relcompile.Name) relcompile.Name {
	value.t.Helper()
	value.owner(name.Entry)
	if value.once("scope", name) {
		if err := value.registry.InstallScope(name, value.token("scope", name)); err != nil {
			value.t.Fatalf("install scope %v: %v", name, err)
		}
	}
	return name
}

func (value *surface) valueType(name relcompile.Name) relcompile.Name {
	value.t.Helper()
	value.owner(name.Entry)
	if value.once("type", name) {
		if err := value.registry.InstallType(name, value.token("type", name)); err != nil {
			value.t.Fatalf("install type %v: %v", name, err)
		}
	}
	return name
}

// relation installs one relation, the address column its rows are joined by,
// the key they are published under, and the denominator they are enumerated
// against. All four are statements about the relation itself.
func (value *surface) relation(name relcompile.Name, scope relcompile.Name) {
	value.t.Helper()
	value.owner(name.Entry)
	if !value.once("relation", name) {
		return
	}
	if err := value.registry.InstallRelation(name, value.token("relation", name), scope); err != nil {
		value.t.Fatalf("install relation %v: %v", name, err)
	}
	address := relcompile.NewName(name.Entry, name.Member+"#address")
	value.column(address, name)
	if err := value.registry.DeclareCoordinate(name, relcompile.CoordinateAddress, address); err != nil {
		value.t.Fatalf("declare address of %v: %v", name, err)
	}
	key := relcompile.NewName(name.Entry, name.Member+"#key")
	if err := value.registry.InstallKey(key, value.token("key", key), name, address); err != nil {
		value.t.Fatalf("install key %v: %v", key, err)
	}
	if err := value.registry.DeclarePublicationKey(name, key); err != nil {
		value.t.Fatalf("declare publication key of %v: %v", name, err)
	}
	if err := value.registry.InstallDenominator(name, name, key); err != nil {
		value.t.Fatalf("install denominator %v: %v", name, err)
	}
}

func (value *surface) column(name relcompile.Name, relation relcompile.Name) {
	value.t.Helper()
	if !value.once("column", name) {
		return
	}
	if err := value.registry.InstallColumn(name, value.token("column", name), relation, value.lattice); err != nil {
		value.t.Fatalf("install column %v of %v: %v", name, relation, err)
	}
}

func (value *surface) expression(entry schema.EntryReference, member schema.Key) {
	value.t.Helper()
	value.owner(entry)
	name := relcompile.NewName(entry, member)
	if !value.once("expression", name) {
		return
	}
	if err := value.registry.InstallExpression(name, value.token("expression", name)); err != nil {
		value.t.Fatalf("install expression %v: %v", name, err)
	}
	if err := value.registry.InstallDependency(name, value.token("dependency", name)); err != nil {
		value.t.Fatalf("install dependency %v: %v", name, err)
	}
}

// operation seals one typed semantic signature. The inputs name the relation
// and columns the lowered expression delivers to Apply; the outputs are every
// column of the destination relation, because Publish admits only a child that
// carries the destination row whole.
func (value *surface) operation(name relcompile.Name, inputs []relcompile.Name, inputRelation relcompile.Name, destination relcompile.Name) {
	value.t.Helper()
	value.owner(name.Entry)
	if !value.once("operation", name) {
		return
	}
	if err := value.registry.InstallOperation(name, value.token("operation", name)); err != nil {
		value.t.Fatalf("install operation %v: %v", name, err)
	}
	site := relcompile.Site{Path: "demo.operation"}
	operation, err := value.registry.Operation(site, name)
	if err != nil {
		value.t.Fatalf("resolve operation %v: %v", name, err)
	}
	owner, err := value.registry.Owner(site, name.Entry)
	if err != nil {
		value.t.Fatalf("resolve operation owner %v: %v", name, err)
	}
	typeID, err := value.registry.Type(site, value.lattice)
	if err != nil {
		value.t.Fatalf("resolve demo lattice: %v", err)
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		value.t.Fatal("construct scalar delivery")
	}

	inputRelationID, err := value.registry.Relation(site, inputRelation)
	if err != nil {
		value.t.Fatalf("resolve input relation %v: %v", inputRelation, err)
	}
	inputDenominator, err := value.registry.Denominator(site, inputRelation)
	if err != nil {
		value.t.Fatalf("resolve input denominator %v: %v", inputRelation, err)
	}
	sealed := make([]signature.Input, 0, len(inputs))
	for _, column := range inputs {
		columnID, err := value.registry.Column(site, column)
		if err != nil {
			value.t.Fatalf("resolve input column %v: %v", column, err)
		}
		sealed = append(sealed, signature.Input{
			Relation: inputRelationID, Column: columnID, Type: typeID,
			Presence: signature.AllowMissing, Delivery: delivery, Denominator: inputDenominator,
		})
	}

	destinationID, err := value.registry.Relation(site, destination)
	if err != nil {
		value.t.Fatalf("resolve destination relation %v: %v", destination, err)
	}
	destinationDenominator, err := value.registry.Denominator(site, destination)
	if err != nil {
		value.t.Fatalf("resolve destination denominator %v: %v", destination, err)
	}
	produced := make([]signature.Output, 0, 2)
	for _, column := range value.columnsOf(destinationID) {
		produced = append(produced, signature.Output{
			Relation: destinationID, Column: column, Type: typeID,
			Presence: signature.ProduceOptional,
		})
	}

	cardinality, ok := model.NewCardinality(model.Optional, 0)
	if !ok {
		value.t.Fatal("construct cardinality")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		value.t.Fatal("construct outcome set")
	}
	contract, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: owner, Schema: value.schemaID},
		Inputs:      sealed,
		Outputs:     produced,
		Authority:   signature.OutputAuthority{Denominator: destinationDenominator},
		Cardinality: cardinality,
		Outcomes:    accepted,
	})
	if !ok {
		value.t.Fatalf("seal signature for %v", name)
	}
	if err := value.registry.InstallSignature(name, contract); err != nil {
		value.t.Fatalf("install signature %v: %v", name, err)
	}
}

// columnsOf reads back the columns one relation carries, in the order the
// registry holds them.
func (value *surface) columnsOf(relation model.RelationID) []model.ColumnID {
	value.t.Helper()
	declaration := value.registry.Declaration(value.schemaID)
	for _, entry := range declaration.Relations {
		if entry.ID() == relation {
			return entry.Columns()
		}
	}
	value.t.Fatalf("relation %v carries no columns", relation)
	return nil
}

// declare performs the relation pass for one authored rule: every relation,
// column, key, denominator and expression the declaration names. It answers
// the placement the rule's candidate and input ports are decided at.
//
// Relations are declared for every rule of the chain before any signature is
// sealed, because a signature states the whole destination row and a later
// rule may publish one more column of it.
func (value *surface) declare(spec rule.Spec) relcompile.Placement {
	value.t.Helper()
	program := spec.Program
	ruleEntry := schema.EntryReference{Surface: schema.SurfaceKindRule, Key: spec.Key}

	candidateScope := value.scope(relcompile.EntryName(schema.SurfaceKindStructure, "demo/candidate"))
	placement := relcompile.Placement{Candidate: candidateScope}
	for port := 0; port < program.InputCount(); port++ {
		placement.Ports = append(placement.Ports,
			value.scope(relcompile.EntryName(schema.SurfaceKindStructure, schema.Key(fmt.Sprintf("demo/port/%d", port)))))
	}

	value.relation(value.candidateOf(spec), candidateScope)
	for _, read := range value.readsOf(spec) {
		value.relation(read.relation, candidateScope)
		value.column(read.key, read.relation)
	}
	for _, output := range program.Fold.Outputs {
		destination := relcompile.NewName(output.Column.Axis, output.Column.Key)
		value.relation(destination, candidateScope)
		value.column(relcompile.NewName(output.Destination.Axis, output.Destination.Member), destination)
		value.expression(ruleEntry, output.Column.Key)
	}
	return placement
}

// seal performs the operation pass for one authored rule: the reducer, and the
// transform of a carried output, sealed over the frame the lowered expression
// delivers to Apply.
func (value *surface) seal(spec rule.Spec) {
	value.t.Helper()
	program := spec.Program
	candidate := value.candidateOf(spec)

	// The frame the reducer receives is the candidate row and the rows its
	// declared reads delivered: one input slot each, in declaration order.
	frame := []relcompile.Name{relcompile.NewName(candidate.Entry, candidate.Member+"#address")}
	for _, read := range value.readsOf(spec) {
		frame = append(frame, read.key)
	}
	for _, output := range program.Fold.Outputs {
		destination := relcompile.NewName(output.Column.Axis, output.Column.Key)
		value.operation(relcompile.NewName(program.Fold.Reducer.Axis, program.Fold.Reducer.Member), frame, candidate, destination)
		if program.Carry != nil && program.Carry.Mode == ruleprogram.CarryTransform {
			value.operation(relcompile.NewName(program.Carry.Transform.Axis, program.Carry.Transform.Member), frame, candidate, destination)
		}
	}
}

// candidateOf names the relation one rule's candidate rows belong to.
func (value *surface) candidateOf(spec rule.Spec) relcompile.Name {
	candidate := relcompile.NewName(spec.Program.Candidate.AxisRelation.Axis, spec.Program.Candidate.AxisRelation.Member)
	if candidate.Available() {
		return candidate
	}
	return relcompile.EntryName(schema.SurfaceKindIssuance, spec.Program.Candidate.IssuedRow)
}

// read is one declared equijoin of a rule: the relation it reads and the key
// column its rows are paired by.
type read struct {
	relation relcompile.Name
	key      relcompile.Name
}

// readsOf names every read of one rule in declaration order.
func (value *surface) readsOf(spec rule.Spec) []read {
	program := spec.Program
	reads := make([]read, 0, program.JoinCount())
	for index := 0; index < program.JoinCount(); index++ {
		declaration, ok := program.JoinAt(index)
		if !ok {
			continue
		}
		reads = append(reads, read{
			relation: relcompile.NewName(declaration.Relation.Axis, declaration.Relation.Member),
			key:      relcompile.NewName(declaration.Key.Axis, declaration.Key.Member),
		})
	}
	return reads
}

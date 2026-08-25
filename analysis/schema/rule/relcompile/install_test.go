package relcompile_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// owners stands in for the declaration surfaces that issue the entries a rule
// names. Each surface derives its tokens under its own domain separation and
// hands them to the registry; the registry itself derives nothing, so the
// no-local-minting law holds through this harness exactly as it holds in
// composition.
type owners struct {
	t        *testing.T
	registry *relcompile.Registry
	seen     map[relcompile.Name]bool
}

func newOwners(t *testing.T) *owners {
	t.Helper()
	return &owners{t: t, registry: relcompile.NewRegistry(), seen: map[relcompile.Name]bool{}}
}

func (surfaces *owners) token(kind string, name relcompile.Name) identity.ContentID {
	surfaces.t.Helper()
	value, ok := identity.DeriveContentID("relcompile-census/v1", []byte(kind), []byte(name.String()))
	if !ok {
		surfaces.t.Fatalf("derive %s token for %v", kind, name)
	}
	return value
}

func (surfaces *owners) once(kind string, name relcompile.Name) bool {
	marker := relcompile.NewName(name.Entry, schema.Key(kind)+"\x00"+name.Member)
	if surfaces.seen[marker] {
		return false
	}
	surfaces.seen[marker] = true
	return true
}

func (surfaces *owners) owner(entry schema.EntryReference) {
	surfaces.t.Helper()
	name := relcompile.Name{Entry: entry}
	if !surfaces.once("owner", name) {
		return
	}
	if err := surfaces.registry.InstallOwner(entry, surfaces.token("owner", name)); err != nil {
		surfaces.t.Fatalf("install owner %v: %v", entry, err)
	}
}

func (surfaces *owners) scope(name relcompile.Name) relcompile.Name {
	surfaces.t.Helper()
	surfaces.owner(name.Entry)
	if surfaces.once("scope", name) {
		if err := surfaces.registry.InstallScope(name, surfaces.token("scope", name)); err != nil {
			surfaces.t.Fatalf("install scope %v: %v", name, err)
		}
	}
	return name
}

func (surfaces *owners) valueType(name relcompile.Name) relcompile.Name {
	surfaces.t.Helper()
	surfaces.owner(name.Entry)
	if surfaces.once("type", name) {
		if err := surfaces.registry.InstallType(name, surfaces.token("type", name)); err != nil {
			surfaces.t.Fatalf("install type %v: %v", name, err)
		}
	}
	return name
}

// relation installs one relation together with the address column its rows are
// joined by and the key they are published under. Both are owner statements
// about the relation itself, not roles the compiler invents.
func (surfaces *owners) relation(name relcompile.Name, scope relcompile.Name) {
	surfaces.t.Helper()
	surfaces.owner(name.Entry)
	if !surfaces.once("relation", name) {
		return
	}
	if err := surfaces.registry.InstallRelation(name, surfaces.token("relation", name), scope); err != nil {
		surfaces.t.Fatalf("install relation %v: %v", name, err)
	}
	surfaces.coordinate(name, relcompile.CoordinateAddress)
	key := relcompile.NewName(name.Entry, name.Member+"#key")
	if surfaces.once("key", key) {
		address := relcompile.NewName(name.Entry, name.Member+"#address")
		if err := surfaces.registry.InstallKey(key, surfaces.token("key", key), name, address); err != nil {
			surfaces.t.Fatalf("install key %v: %v", key, err)
		}
	}
	if err := surfaces.registry.DeclarePublicationKey(name, key); err != nil {
		surfaces.t.Fatalf("declare publication key of %v: %v", name, err)
	}
}

// coordinate stands in for the relation owner declaring one column its own
// rows are addressed by. The census declares exactly the coordinates the
// authored rule declaration names a use for, so a row that lowers proves the
// lowering works once the owner publishes them and never that the compiler
// invented one.
func (surfaces *owners) coordinate(relation relcompile.Name, coordinate relcompile.Coordinate) {
	surfaces.t.Helper()
	column := relcompile.NewName(relation.Entry, relation.Member+"#"+schema.Key(coordinate.String()))
	if !surfaces.once("coordinate", column) {
		return
	}
	surfaces.column(column, relation)
	if err := surfaces.registry.DeclareCoordinate(relation, coordinate, column); err != nil {
		surfaces.t.Fatalf("declare %s of %v: %v", coordinate, relation, err)
	}
}

func (surfaces *owners) column(name relcompile.Name, relation relcompile.Name) {
	surfaces.t.Helper()
	if !surfaces.once("column", name) {
		return
	}
	valueType := surfaces.valueType(relcompile.NewName(name.Entry, name.Member+"#type"))
	if err := surfaces.registry.InstallColumn(name, surfaces.token("column", name), relation, valueType); err != nil {
		surfaces.t.Fatalf("install column %v of %v: %v", name, relation, err)
	}
}

func (surfaces *owners) denominator(reference ruleprogram.DenominatorRef, scope relcompile.Name) {
	surfaces.t.Helper()
	name := relcompile.Name{Entry: schema.EntryReference(reference)}
	if !name.Available() || !surfaces.once("denominator", name) {
		return
	}
	surfaces.relation(name, scope)
	key := relcompile.NewName(name.Entry, name.Member+"#key")
	if err := surfaces.registry.InstallDenominator(name, name, key); err != nil {
		surfaces.t.Fatalf("install denominator %v: %v", name, err)
	}
}

// operation installs one sealed semantic signature under the reducer or
// transform name the axis owner declared. Version, delivery, denominator and
// outcome vocabulary come from the signature, so the compiler mints none of
// them.
func (surfaces *owners) operation(name relcompile.Name, destination relcompile.Name) {
	surfaces.t.Helper()
	surfaces.owner(name.Entry)
	if !surfaces.once("operation", name) {
		return
	}
	token := surfaces.token("operation", name)
	if err := surfaces.registry.InstallOperation(name, token); err != nil {
		surfaces.t.Fatalf("install operation %v: %v", name, err)
	}
	operation, err := surfaces.registry.Operation(relcompile.Site{Path: "census.operation"}, name)
	if err != nil {
		surfaces.t.Fatalf("resolve operation %v: %v", name, err)
	}
	owner, err := surfaces.registry.Owner(relcompile.Site{Path: "census.operation"}, name.Entry)
	if err != nil {
		surfaces.t.Fatalf("resolve operation owner %v: %v", name, err)
	}
	schemaID, ok := model.IssueSchemaID(owner, surfaces.token("schema", relcompile.Name{Entry: name.Entry}))
	if !ok {
		surfaces.t.Fatalf("issue schema identity for %v", name)
	}
	column, err := surfaces.registry.Column(relcompile.Site{Path: "census.operation"}, destination)
	if err != nil {
		surfaces.t.Fatalf("resolve destination %v: %v", destination, err)
	}
	key, err := surfaces.registry.PublicationKey(relcompile.Site{Path: "census.operation"}, destination)
	if err != nil {
		surfaces.t.Fatalf("resolve publication key of %v: %v", destination, err)
	}
	reference, ok := model.NewDenominatorRef(column.Relation(), key)
	if !ok {
		surfaces.t.Fatalf("construct denominator for %v", destination)
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		surfaces.t.Fatal("construct scalar delivery")
	}
	valueType, err := surfaces.registry.Type(relcompile.Site{Path: "census.operation"}, relcompile.NewName(destination.Entry, destination.Member+"#type"))
	if err != nil {
		surfaces.t.Fatalf("resolve destination type %v: %v", destination, err)
	}
	cardinality, ok := model.NewCardinality(model.Optional, 0)
	if !ok {
		surfaces.t.Fatal("construct cardinality")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		surfaces.t.Fatal("construct outcome set")
	}
	sealed, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{{
			Relation: column.Relation(), Column: column, Type: valueType,
			Presence: signature.AllowMissing, Delivery: delivery, Denominator: reference,
		}},
		Outputs: []signature.Output{{
			Relation: column.Relation(), Column: column, Type: valueType,
			Presence: signature.ProduceOptional,
		}},
		Authority:   signature.OutputAuthority{Denominator: reference},
		Cardinality: cardinality,
		Outcomes:    accepted,
	})
	if !ok {
		surfaces.t.Fatalf("seal signature for %v", name)
	}
	if err := surfaces.registry.InstallSignature(name, sealed); err != nil {
		surfaces.t.Fatalf("install signature %v: %v", name, err)
	}
}

func (surfaces *owners) expression(entry schema.EntryReference, member schema.Key) {
	surfaces.t.Helper()
	surfaces.owner(entry)
	name := relcompile.NewName(entry, member)
	if !surfaces.once("expression", name) {
		return
	}
	if err := surfaces.registry.InstallExpression(name, surfaces.token("expression", name)); err != nil {
		surfaces.t.Fatalf("install expression %v: %v", name, err)
	}
	if err := surfaces.registry.InstallDependency(name, surfaces.token("dependency", name)); err != nil {
		surfaces.t.Fatalf("install dependency %v: %v", name, err)
	}
}

// install performs the composition-side declaration pass for one rule: every
// surface installs the entries its own declaration names, and the placement
// names the decision scope of the candidate and of each declared input port.
func (surfaces *owners) install(spec rule.Spec) relcompile.Placement {
	surfaces.t.Helper()
	program := spec.Program
	ruleEntry := schema.EntryReference{Surface: schema.SurfaceKindRule, Key: spec.Key}

	candidateScope := surfaces.scope(relcompile.EntryName(schema.SurfaceKindStructure, "census/candidate"))
	placement := relcompile.Placement{Candidate: candidateScope}
	for port := 0; port < program.InputCount(); port++ {
		placement.Ports = append(placement.Ports,
			surfaces.scope(relcompile.EntryName(schema.SurfaceKindStructure, schema.Key(fmt.Sprintf("census/port/%d", port)))))
	}

	var candidate relcompile.Name
	if reference := program.Candidate.AxisRelation; reference.Available() {
		candidate = relcompile.NewName(reference.Axis, reference.Member)
	} else if program.Candidate.IssuedRow.Available() {
		candidate = relcompile.EntryName(schema.SurfaceKindIssuance, program.Candidate.IssuedRow)
	}
	if candidate.Available() {
		surfaces.relation(candidate, candidateScope)
	}

	for index := 0; index < program.JoinCount(); index++ {
		declaration, ok := program.JoinAt(index)
		if !ok {
			continue
		}
		surfaces.installJoin(declaration, candidateScope, candidate)
	}

	if program.Activation != nil {
		branch := relcompile.NewName(program.Activation.Branch.Axis, program.Activation.Branch.Member)
		surfaces.relation(branch, candidateScope)
		surfaces.coordinate(branch, relcompile.CoordinateParent)
		surfaces.coordinate(branch, relcompile.CoordinateOrdinal)
	}

	for _, output := range program.Fold.Outputs {
		destination := surfaces.installOutput(output, candidateScope)
		surfaces.operation(relcompile.NewName(program.Fold.Reducer.Axis, program.Fold.Reducer.Member), destination)
		if program.Carry != nil && program.Carry.Mode == ruleprogram.CarryTransform {
			surfaces.operation(relcompile.NewName(program.Carry.Transform.Axis, program.Carry.Transform.Member), destination)
		}
		surfaces.expression(ruleEntry, output.Column.Key)
	}
	return placement
}

func (surfaces *owners) installJoin(declaration ruleprogram.JoinDecl, scope relcompile.Name, candidate relcompile.Name) {
	surfaces.t.Helper()
	joined := relcompile.NewName(declaration.Relation.Axis, declaration.Relation.Member)
	if !joined.Available() {
		return
	}
	surfaces.relation(joined, scope)
	surfaces.installProjection(declaration.Key, joined)
	surfaces.installProjection(declaration.Predicate, joined)
	for _, nested := range []member.RelationRef{declaration.Parent, declaration.KeyVector} {
		if !nested.Available() {
			continue
		}
		surfaces.relation(relcompile.NewName(nested.Axis, nested.Member), scope)
		surfaces.coordinate(joined, relcompile.CoordinateParent)
		surfaces.coordinate(joined, relcompile.CoordinateOrdinal)
	}
	// The occurrence identity is projected from the rule's own candidate row,
	// so the column belongs to the candidate relation and never to the foreign
	// directory the read reaches through it.
	if projection := declaration.AddressIdentity; projection.Available() {
		surfaces.column(relcompile.NewName(projection.Axis, projection.Member), candidate)
		surfaces.coordinate(joined, relcompile.CoordinateOccurrence)
	}
	surfaces.denominator(declaration.Read.Contract.DenominatorRef, scope)
}

func (surfaces *owners) installProjection(projection member.ProjectionRef, relation relcompile.Name) {
	surfaces.t.Helper()
	if !projection.Available() {
		return
	}
	surfaces.column(relcompile.NewName(projection.Axis, projection.Member), relation)
}

// installOutput installs the published Factor a rule writes: the axis output
// column names the relation, and the authored destination projection is one
// column of it.
func (surfaces *owners) installOutput(output ruleprogram.OutputDecl, scope relcompile.Name) relcompile.Name {
	surfaces.t.Helper()
	relation := relcompile.NewName(output.Column.Axis, output.Column.Key)
	surfaces.relation(relation, scope)
	destination := relcompile.NewName(output.Destination.Axis, output.Destination.Member)
	surfaces.column(destination, relation)
	return destination
}

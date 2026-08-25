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
	schemaID model.SchemaID
	// columnOwner and relationColumns are the census's own record of what it
	// installed, so a signature can be shaped from the rows it will read and
	// publish rather than from a name it rebuilds.
	columnOwner     map[relcompile.Name]relcompile.Name
	relationColumns map[relcompile.Name][]relcompile.Name
}

// schema is the one execution-schema identity this census builds. A signature
// is fenced to the exact schema artifact it belongs to, so the plan and every
// signature sealed into it name the same one.
func (surfaces *owners) schema() model.SchemaID {
	surfaces.t.Helper()
	if surfaces.schemaID.Available() {
		return surfaces.schemaID
	}
	entry := schema.EntryReference{Surface: schema.SurfaceKindStructure, Key: "census/schema"}
	surfaces.owner(entry)
	owner, err := surfaces.registry.Owner(relcompile.Site{Path: "census.schema"}, entry)
	if err != nil {
		surfaces.t.Fatalf("resolve census schema owner: %v", err)
	}
	issued, ok := model.IssueSchemaID(owner, surfaces.token("schema", relcompile.Name{Entry: entry}))
	if !ok {
		surfaces.t.Fatal("issue census schema identity")
	}
	surfaces.schemaID = issued
	return issued
}

func newOwners(t *testing.T) *owners {
	t.Helper()
	return &owners{
		t: t, registry: relcompile.NewRegistry(), seen: map[relcompile.Name]bool{},
		columnOwner:     map[relcompile.Name]relcompile.Name{},
		relationColumns: map[relcompile.Name][]relcompile.Name{},
	}
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
	surfaces.coordinateColumn(column, relation)
	if err := surfaces.registry.DeclareCoordinate(relation, coordinate, column); err != nil {
		surfaces.t.Fatalf("declare %s of %v: %v", coordinate, relation, err)
	}
}

func (surfaces *owners) column(name relcompile.Name, relation relcompile.Name) {
	surfaces.t.Helper()
	surfaces.typedColumn(name, relation, relcompile.NewName(name.Entry, name.Member+"#type"))
}

// coordinateColumn installs a column that names an address rather than a
// value. An oriented equijoin pairs two columns that name the same coordinate
// space - a source's address against the key a read is taken at - so every
// addressing column this census installs carries the one coordinate type, and
// a join over them agrees by construction instead of by accident.
func (surfaces *owners) coordinateColumn(name relcompile.Name, relation relcompile.Name) {
	surfaces.t.Helper()
	surfaces.typedColumn(name, relation, relcompile.EntryName(schema.SurfaceKindStructure, "census/coordinate"))
}

func (surfaces *owners) typedColumn(name relcompile.Name, relation relcompile.Name, columnType relcompile.Name) {
	surfaces.t.Helper()
	if !surfaces.once("column", name) {
		return
	}
	valueType := surfaces.valueType(columnType)
	if err := surfaces.registry.InstallColumn(name, surfaces.token("column", name), relation, valueType); err != nil {
		surfaces.t.Fatalf("install column %v of %v: %v", name, relation, err)
	}
	surfaces.columnOwner[name] = relation
	surfaces.relationColumns[relation] = append(surfaces.relationColumns[relation], name)
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
// operation installs one sealed semantic signature and, with it, the operand
// relation the operation reads.
//
// A join yields a row of two relations' columns and no relation of its own, so
// an operation is applied to a row projected onto a relation the signature
// names. The census declares that relation with one column per read the rule
// makes, and declares the operation's outputs as the whole destination row, so
// what the operation yields is exactly what the publication owes.
func (surfaces *owners) operation(name relcompile.Name, destination relcompile.Name, reads int, candidate relcompile.Name) {
	surfaces.t.Helper()
	surfaces.owner(name.Entry)
	if !surfaces.once("operation", name) {
		return
	}
	site := relcompile.Site{Path: "census.operation"}
	if err := surfaces.registry.InstallOperation(name, surfaces.token("operation", name)); err != nil {
		surfaces.t.Fatalf("install operation %v: %v", name, err)
	}
	operation, err := surfaces.registry.Operation(site, name)
	if err != nil {
		surfaces.t.Fatalf("resolve operation %v: %v", name, err)
	}
	owner, err := surfaces.registry.Owner(site, name.Entry)
	if err != nil {
		surfaces.t.Fatalf("resolve operation owner %v: %v", name, err)
	}

	// A rule that reads nothing applies its operation to the candidate row
	// itself, which is already a relation: there is no joined row to project.
	operand := candidate
	first := surfaces.coordinateName(candidate, relcompile.CoordinateAddress)
	if reads != 0 {
		operand = relcompile.NewName(name.Entry, name.Member+"#operand")
		first = surfaces.operandRelation(operand, reads)
	}
	operandDenominator := surfaces.denominatorOf(operand)
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		surfaces.t.Fatal("construct scalar delivery")
	}
	firstColumn, err := surfaces.registry.Column(site, first)
	if err != nil {
		surfaces.t.Fatalf("resolve operand column %v: %v", first, err)
	}
	firstType, err := surfaces.registry.ColumnType(site, first)
	if err != nil {
		surfaces.t.Fatalf("resolve operand column type %v: %v", first, err)
	}
	// One child, one input: the operation is applied to the operand row, and
	// what it reads of that row is the column its signature names.
	inputs := []signature.Input{{
		Relation: operandDenominator.Relation(), Column: firstColumn, Type: firstType,
		Presence: signature.AllowMissing, Delivery: delivery, Denominator: operandDenominator,
	}}

	published := surfaces.relationOf(destination)
	outputs := surfaces.destinationOutputs(published)
	if len(outputs) == 0 {
		surfaces.t.Fatalf("destination %v publishes no column", destination)
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
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: owner, Schema: surfaces.schema()},
		Inputs:      inputs,
		Outputs:     outputs,
		Authority:   signature.OutputAuthority{Denominator: surfaces.denominatorOf(published)},
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

// transform seals one carry transform: it reads the published row and yields
// that same row, so both sides of the merge it feeds have one typed shape.
func (surfaces *owners) transform(name relcompile.Name, destination relcompile.Name) {
	surfaces.t.Helper()
	surfaces.owner(name.Entry)
	if !surfaces.once("operation", name) {
		return
	}
	site := relcompile.Site{Path: "census.transform"}
	if err := surfaces.registry.InstallOperation(name, surfaces.token("operation", name)); err != nil {
		surfaces.t.Fatalf("install transform %v: %v", name, err)
	}
	operation, err := surfaces.registry.Operation(site, name)
	if err != nil {
		surfaces.t.Fatalf("resolve transform %v: %v", name, err)
	}
	owner, err := surfaces.registry.Owner(site, name.Entry)
	if err != nil {
		surfaces.t.Fatalf("resolve transform owner %v: %v", name, err)
	}
	published := surfaces.relationOf(destination)
	reference := surfaces.denominatorOf(published)
	column, err := surfaces.registry.Column(site, destination)
	if err != nil {
		surfaces.t.Fatalf("resolve destination %v: %v", destination, err)
	}
	columnType, err := surfaces.registry.ColumnType(site, destination)
	if err != nil {
		surfaces.t.Fatalf("resolve destination type %v: %v", destination, err)
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		surfaces.t.Fatal("construct scalar delivery")
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
		Fence:    signature.Fence{Owner: owner, Schema: surfaces.schema()},
		Inputs: []signature.Input{{
			Relation: reference.Relation(), Column: column, Type: columnType,
			Presence: signature.AllowMissing, Delivery: delivery, Denominator: reference,
		}},
		Outputs:     surfaces.destinationOutputs(published),
		Authority:   signature.OutputAuthority{Denominator: reference},
		Cardinality: cardinality,
		Outcomes:    accepted,
	})
	if !ok {
		surfaces.t.Fatalf("seal transform signature for %v", name)
	}
	if err := surfaces.registry.InstallSignature(name, sealed); err != nil {
		surfaces.t.Fatalf("install transform signature %v: %v", name, err)
	}
}

// coordinateName is the column a relation publishes one of its coordinates as.
func (surfaces *owners) coordinateName(relation relcompile.Name, coordinate relcompile.Coordinate) relcompile.Name {
	return relcompile.NewName(relation.Entry, relation.Member+"#"+schema.Key(coordinate.String()))
}

// operandRelation installs the row an operation is applied to: one column per
// read the rule makes and nothing else, so the projection that builds it
// defines every column exactly once. Its first column addresses it.
func (surfaces *owners) operandRelation(name relcompile.Name, reads int) relcompile.Name {
	surfaces.t.Helper()
	scope := surfaces.scope(relcompile.EntryName(schema.SurfaceKindStructure, "census/candidate"))
	surfaces.owner(name.Entry)
	if surfaces.once("relation", name) {
		if err := surfaces.registry.InstallRelation(name, surfaces.token("relation", name), scope); err != nil {
			surfaces.t.Fatalf("install operand relation %v: %v", name, err)
		}
	}
	var first relcompile.Name
	for index := 0; index < reads; index++ {
		column := relcompile.NewName(name.Entry, name.Member+"#read"+schema.Key(rune('0'+index)))
		surfaces.coordinateColumn(column, name)
		if index == 0 {
			first = column
		}
	}
	if !first.Available() {
		surfaces.t.Fatalf("operand relation %v reads nothing", name)
	}
	if err := surfaces.registry.DeclareCoordinate(name, relcompile.CoordinateAddress, first); err != nil {
		surfaces.t.Fatalf("declare operand address of %v: %v", name, err)
	}
	key := relcompile.NewName(name.Entry, name.Member+"#key")
	if surfaces.once("key", key) {
		if err := surfaces.registry.InstallKey(key, surfaces.token("key", key), name, first); err != nil {
			surfaces.t.Fatalf("install operand key %v: %v", key, err)
		}
	}
	if err := surfaces.registry.DeclarePublicationKey(name, key); err != nil {
		surfaces.t.Fatalf("declare operand publication key of %v: %v", name, err)
	}
	return first
}

// relationOf names the relation one installed column belongs to.
func (surfaces *owners) relationOf(column relcompile.Name) relcompile.Name {
	surfaces.t.Helper()
	owner, ok := surfaces.columnOwner[column]
	if !ok {
		surfaces.t.Fatalf("column %v belongs to no installed relation", column)
	}
	return owner
}

// denominatorOf is the relation's own row universe: its rows under the key
// they are published at.
func (surfaces *owners) denominatorOf(relation relcompile.Name) model.DenominatorRef {
	surfaces.t.Helper()
	site := relcompile.Site{Path: "census.denominator"}
	id, err := surfaces.registry.Relation(site, relation)
	if err != nil {
		surfaces.t.Fatalf("resolve relation %v: %v", relation, err)
	}
	key, err := surfaces.registry.RelationPublicationKey(site, relation)
	if err != nil {
		surfaces.t.Fatalf("resolve publication key of %v: %v", relation, err)
	}
	reference, ok := model.NewDenominatorRef(id, key)
	if !ok {
		surfaces.t.Fatalf("construct denominator for %v", relation)
	}
	return reference
}

// destinationOutputs is the whole published row: an operation yields every
// column the destination declares, so a publication of its result owes nothing
// the operation did not produce.
func (surfaces *owners) destinationOutputs(relation relcompile.Name) []signature.Output {
	surfaces.t.Helper()
	site := relcompile.Site{Path: "census.operation"}
	id, err := surfaces.registry.Relation(site, relation)
	if err != nil {
		surfaces.t.Fatalf("resolve relation %v: %v", relation, err)
	}
	outputs := make([]signature.Output, 0, len(surfaces.relationColumns[relation]))
	for _, column := range surfaces.relationColumns[relation] {
		resolved, err := surfaces.registry.Column(site, column)
		if err != nil {
			surfaces.t.Fatalf("resolve destination column %v: %v", column, err)
		}
		columnType, err := surfaces.registry.ColumnType(site, column)
		if err != nil {
			surfaces.t.Fatalf("resolve destination column type %v: %v", column, err)
		}
		outputs = append(outputs, signature.Output{
			Relation: id, Column: resolved, Type: columnType, Presence: signature.ProduceOptional,
		})
	}
	return outputs
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
		for _, row := range program.Activation.Transport {
			surfaces.owner(schema.EntryReference(row.Axis))
		}
	}

	// Every published row is installed before any operation is sealed over
	// one: an operation yields the whole destination row, so the row has to be
	// whole before its signature says what it produces.
	destinations := make([]relcompile.Name, 0, len(program.Fold.Outputs))
	for _, output := range program.Fold.Outputs {
		destinations = append(destinations, surfaces.installOutput(output, candidateScope))
	}
	// The operand row has one column per read the rule makes, so the operation
	// is shaped from the reads it is applied over.
	reads := program.JoinCount()
	for index, output := range program.Fold.Outputs {
		destination := destinations[index]
		surfaces.operation(relcompile.NewName(program.Fold.Reducer.Axis, program.Fold.Reducer.Member), destination, reads, candidate)
		if program.Carry != nil && program.Carry.Mode == ruleprogram.CarryTransform {
			// A carried fact is transformed where it already is: the transform
			// reads the published row, not the operand row the fold was
			// applied to.
			surfaces.transform(relcompile.NewName(program.Carry.Transform.Axis, program.Carry.Transform.Member), destination)
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
	// An operation that publishes produced rows is a member of the axis those
	// rows belong to, so the owner installs its signature the way it installs
	// a reducer's, and the relation publishes the tag it stamps.
	if declaration.Selection.Available() {
		surfaces.coordinate(joined, relcompile.CoordinateTag)
		destination := relcompile.NewName(declaration.Predicate.Axis, declaration.Predicate.Member)
		if !declaration.Predicate.Available() {
			destination = relcompile.NewName(joined.Entry, joined.Member+"#tag")
		}
		// A selection reads the results it is computed from: one per prior
		// source the declaration names.
		priors := 0
		for _, source := range declaration.Sources {
			if !source.Candidate {
				priors++
			}
		}
		surfaces.expansion(relcompile.NewName(declaration.Selection.Axis, declaration.Selection.Member), destination, 64, priors, candidate)
		surfaces.expression(schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: declaration.Selection.Axis.Key}, declaration.Selection.Member)
	}
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
		surfaces.coordinateColumn(relcompile.NewName(projection.Axis, projection.Member), candidate)
		surfaces.coordinate(joined, relcompile.CoordinateOccurrence)
	}
	surfaces.denominator(declaration.Read.Contract.DenominatorRef, scope)
}

// installProjection installs the column a read is addressed at. A key or tag
// projection is one half of an equijoin, so it is an addressing column like
// the coordinate it pairs with.
func (surfaces *owners) installProjection(projection member.ProjectionRef, relation relcompile.Name) {
	surfaces.t.Helper()
	if !projection.Available() {
		return
	}
	surfaces.coordinateColumn(relcompile.NewName(projection.Axis, projection.Member), relation)
}

// installOutput installs the published Factor a rule writes: the axis output
// column names the relation, and the authored destination projection is one
// column of it.
func (surfaces *owners) installOutput(output ruleprogram.OutputDecl, scope relcompile.Name) relcompile.Name {
	surfaces.t.Helper()
	relation := relcompile.NewName(output.Column.Axis, output.Column.Key)
	surfaces.relation(relation, scope)
	// The published row carries the fact the fold writes. The destination
	// projection names a coordinate of whichever relation produced it, so it
	// is installed where it belongs and never as a column of this one.
	published := relcompile.NewName(relation.Entry, relation.Member+"#fact")
	surfaces.column(published, relation)
	destination := relcompile.NewName(output.Destination.Axis, output.Destination.Member)
	if _, installed := surfaces.columnOwner[destination]; !installed {
		surfaces.coordinateColumn(destination, relation)
	}
	return published
}

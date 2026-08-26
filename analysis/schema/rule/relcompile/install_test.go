package relcompile_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	calldomain "github.com/wippyai/go-lua/domain/call"
	effectdomain "github.com/wippyai/go-lua/domain/effect"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typestateprogram "github.com/wippyai/go-lua/domain/typestate/program"
	valuedomain "github.com/wippyai/go-lua/domain/value"
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
	catalogs map[schema.EntryReference]bool
	results  map[schema.EntryReference]carrier.Key
}

// outputInstallation is the test declaration surface's explicit writer
// contract for one OutputRef. The writer fact column is intentionally distinct
// from a routed Destination projection, which belongs to a route relation.
type outputInstallation struct {
	relation relcompile.Name
	column   relcompile.Name
	key      relcompile.Name
}

type routedOutputInstallation struct {
	output  ruleprogram.OutputDecl
	binding outputInstallation
}

func newOwners(t *testing.T) *owners {
	t.Helper()
	return &owners{t: t, registry: relcompile.NewRegistry(), seen: map[relcompile.Name]bool{}, catalogs: map[schema.EntryReference]bool{}, results: map[schema.EntryReference]carrier.Key{}}
}

// installCatalogOverride installs an explicit test owner vocabulary for a
// synthetic relation family. These catalogs are still sealed member rows --
// the harness never invents a projection while resolving a rule -- but they
// are kept separate from production domain catalogs so a law can use names
// that intentionally do not exist in a real axis.
func (surfaces *owners) installCatalogOverride(axis schema.EntryReference, catalog member.Catalog, result carrier.Key) {
	surfaces.t.Helper()
	surfaces.owner(axis)
	if err := surfaces.registry.InstallMemberCatalog(axis, catalog); err != nil {
		surfaces.t.Fatalf("install synthetic member catalog %v: %v", axis, err)
	}
	surfaces.catalogs[axis] = true
	if result.Available() {
		surfaces.results[axis] = result
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
		if err := surfaces.registry.InstallScope(name, surfaces.token("scope", name), region.True()); err != nil {
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

// factor installs the exact owner statement a Selected read needs: its
// denominator is the writer Factor relation/key, and the read axis consumes
// that Factor. No address is synthesized from the route or output names.
func (surfaces *owners) factor(reference ruleprogram.DenominatorRef, axis schema.EntryReference, binding outputInstallation) {
	surfaces.t.Helper()
	name := relcompile.Name{Entry: schema.EntryReference(reference)}
	if !name.Available() || !surfaces.once("denominator", name) {
		return
	}
	surfaces.owner(axis)
	if err := surfaces.registry.InstallDenominator(name, binding.relation, binding.key); err != nil {
		surfaces.t.Fatalf("install factor denominator %v: %v", name, err)
	}
	if err := surfaces.registry.InstallFactor(axis, name, binding.relation, binding.key); err != nil {
		surfaces.t.Fatalf("install factor %v: %v", name, err)
	}
}

// operation installs one sealed semantic signature under the reducer or
// transform name the axis owner declared. Version, delivery, denominator and
// outcome vocabulary come from the signature, so the compiler mints none of
// them.
func (surfaces *owners) operation(name relcompile.Name, destination relcompile.Name, inputs []relcompile.Name) {
	surfaces.t.Helper()
	surfaces.installOperation(name, destination, surfaces.operationInputs(inputs))
}

// installOperation seals a test-only operation from explicit semantic inputs.
// Most census rows derive those inputs from Fold.Inputs above. Hostile routed
// laws use this seam to state distinct route scalar columns and the final
// selected Factor fact column, rather than hiding that geometry behind the
// synthetic address-only convenience.
func (surfaces *owners) installOperation(name relcompile.Name, destination relcompile.Name, semanticInputs []signature.Input) {
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
		Inputs:   semanticInputs,
		Outputs: []signature.Output{{
			Relation: column.Relation(), Column: column, Type: valueType,
			Presence: signature.ProduceOptional, Denominator: reference,
		}},
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

// operationInputs seals semantic inputs from the read relations an authored
// Program.Fold names. It deliberately does not use the output destination:
// that was a census-only placeholder which hid the missing Apply occurrence
// mapping. Every source relation already owns an address column and a
// publication denominator in this declaration fixture.
func (surfaces *owners) operationInputs(inputs []relcompile.Name) []signature.Input {
	surfaces.t.Helper()
	result := make([]signature.Input, 0, len(inputs))
	for index, input := range inputs {
		site := relcompile.Site{Path: fmt.Sprintf("census.operation.inputs[%d]", index)}
		relation, err := surfaces.registry.Relation(site, input)
		if err != nil {
			surfaces.t.Fatalf("resolve semantic input relation %v: %v", input, err)
		}
		columnName := relcompile.NewName(input.Entry, input.Member+"#address")
		column, err := surfaces.registry.Column(site, columnName)
		if err != nil {
			surfaces.t.Fatalf("resolve semantic input column %v: %v", input, err)
		}
		key, err := surfaces.registry.RelationPublicationKey(site, input)
		if err != nil {
			surfaces.t.Fatalf("resolve semantic input denominator %v: %v", input, err)
		}
		denominator, ok := model.NewDenominatorRef(relation, key)
		if !ok {
			surfaces.t.Fatalf("construct semantic input denominator %v", input)
		}
		typeID, err := surfaces.registry.Type(site, relcompile.NewName(input.Entry, input.Member+"#address#type"))
		if err != nil {
			surfaces.t.Fatalf("resolve semantic input type %v: %v", input, err)
		}
		delivery, ok := signature.NewScalarDelivery()
		if !ok {
			surfaces.t.Fatal("construct semantic input delivery")
		}
		result = append(result, signature.Input{Relation: relation, Column: column, Type: typeID, Presence: signature.AllowMissing, Delivery: delivery, Denominator: denominator})
	}
	return result
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
	// Every owner whose sealed projection or reducer row is named by this
	// Program must be present before the declaration rows are installed.  The
	// writer alone is insufficient: cross-axis projections are owner data too,
	// and resolving them later would make the test fixture a second vocabulary.
	axis := map[schema.EntryReference]struct{}{}
	addAxis := func(reference schema.EntryReference) {
		if reference.Surface == schema.SurfaceKindAxis && reference.Key.Available() {
			axis[reference] = struct{}{}
		}
	}
	addAxis(schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: spec.Writes})
	addAxis(program.Candidate.AxisRelation.Axis)
	addAxis(program.Fold.Reducer.Axis)
	for _, input := range program.Fold.Inputs {
		if join, ok := program.JoinAt(int(input)); ok {
			addAxis(join.Relation.Axis)
			addAxis(join.Key.Axis)
			addAxis(join.Predicate.Axis)
			addAxis(join.Parent.Axis)
			addAxis(join.KeyVector.Axis)
			addAxis(join.AddressIdentity.Axis)
			addAxis(join.Selection.Axis)
		}
	}
	for _, join := range program.Joins {
		addAxis(join.Relation.Axis)
		addAxis(join.Key.Axis)
		addAxis(join.Predicate.Axis)
		addAxis(join.Parent.Axis)
		addAxis(join.KeyVector.Axis)
		addAxis(join.AddressIdentity.Axis)
		addAxis(join.Selection.Axis)
	}
	for _, output := range program.Fold.Outputs {
		addAxis(output.Column.Axis)
		addAxis(output.Destination.Axis)
	}
	for axis := range axis {
		surfaces.installCatalog(axis.Key)
	}

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
	outputs := make([]outputInstallation, len(program.Fold.Outputs))
	routed := make(map[int]routedOutputInstallation)
	for index, output := range program.Fold.Outputs {
		outputs[index] = surfaces.installOutput(output, candidateScope)
		if output.Mode == ruleprogram.ModeRoute && output.RouteJoinPresent {
			routed[int(output.RouteJoin)] = routedOutputInstallation{output: output, binding: outputs[index]}
		}
	}
	// A route's Selected Factor may share its denominator with an earlier
	// Complete read. Install the owner-issued factor binding before either read
	// asks for that denominator so the earlier read cannot reserve a synthetic
	// placeholder relation and erase the real selected-factor contract.
	for joinIndex, installed := range routed {
		declaration, ok := program.JoinAt(joinIndex)
		if !ok || declaration.Read.Form != ruleprogram.Selected {
			continue
		}
		surfaces.factor(declaration.Read.Contract.DenominatorRef, declaration.Read.Axis.EntryReference(), installed.binding)
	}

	relations := []relcompile.Name{candidate}
	for index := 0; index < program.JoinCount(); index++ {
		declaration, ok := program.JoinAt(index)
		if !ok {
			continue
		}
		var route *routedOutputInstallation
		if installed, ok := routed[index]; ok {
			route = &installed
		}
		surfaces.installJoin(declaration, candidateScope, candidate, relations, route)
		relations = append(relations, relcompile.NewName(declaration.Relation.Axis, declaration.Relation.Member))
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

	for index, output := range program.Fold.Outputs {
		destination := outputs[index].column
		surfaces.operation(relcompile.NewName(program.Fold.Reducer.Axis, program.Fold.Reducer.Member), destination, foldInputRelations(program))
		if program.Carry != nil && program.Carry.Mode == ruleprogram.CarryTransform {
			carried := outputs[index].relation
			surfaces.operation(relcompile.NewName(program.Carry.Transform.Axis, program.Carry.Transform.Member), destination, []relcompile.Name{carried})
		}
		if output.Mode == ruleprogram.ModeExact && candidate.Available() {
			surfaces.installProjection(output.Destination, candidateScope, candidate, member.Destination, program.Candidate)
		}
		surfaces.expression(ruleEntry, output.Column.Key)
	}
	return placement
}

// installCatalog hands the exact generated member catalog to the relational
// registry before any relation rows are installed. Projection rows are then
// copied from that sealed owner vocabulary; this harness never mints a result
// carrier, role, or candidate provider of its own.
func (surfaces *owners) installCatalog(axisKey schema.Key) {
	surfaces.t.Helper()
	axis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: axisKey}
	if surfaces.catalogs[axis] {
		return
	}
	surfaces.owner(axis)
	var catalog member.Catalog
	switch axisKey {
	case "call":
		catalog = calldomain.AxisMemberCatalog()
	case "effect":
		catalog = effectdomain.AxisMemberCatalog()
	case "heap":
		catalog = heapdomain.AxisMemberCatalog()
	case "pack":
		catalog = packdomain.AxisMemberCatalog()
	case "placement":
		catalog = placementdomain.AxisMemberCatalog()
	case "placement-suspension-evidence":
		catalog = placementsuspension.AxisMemberCatalog()
	case "static-type":
		catalog = staticdomain.AxisMemberCatalog()
	case "typestate":
		catalog = typestateprogram.AxisMemberCatalog()
	case "value":
		catalog = valuedomain.AxisMemberCatalog()
	default:
		surfaces.t.Fatalf("no sealed member catalog for axis %q", axisKey)
	}
	if err := surfaces.registry.InstallMemberCatalog(axis, catalog); err != nil {
		surfaces.t.Fatalf("install member catalog %v: %v", axis, err)
	}
	surfaces.catalogs[axis] = true
}

func (surfaces *owners) installJoin(declaration ruleprogram.JoinDecl, scope relcompile.Name, candidate relcompile.Name, relations []relcompile.Name, routed *routedOutputInstallation) {
	surfaces.t.Helper()
	joined := relcompile.NewName(declaration.Relation.Axis, declaration.Relation.Member)
	if !joined.Available() {
		return
	}
	surfaces.relation(joined, scope)
	surfaces.installProjection(declaration.Key, scope, joined, member.Key, member.AxisRelationCandidate(member.RelationRef{Axis: joined.Entry, Member: joined.Member}))
	surfaces.installProjection(declaration.Predicate, scope, joined, member.Predicate, member.AxisRelationCandidate(member.RelationRef{Axis: joined.Entry, Member: joined.Member}))
	// An operation that publishes produced rows is a member of the axis those
	// rows belong to, so the owner installs its signature the way it installs
	// a reducer's, and the relation publishes the tag it stamps.
	if declaration.Selection.Available() {
		surfaces.coordinate(joined, relcompile.CoordinateTag)
		destination := relcompile.NewName(declaration.Predicate.Axis, declaration.Predicate.Member)
		if !declaration.Predicate.Available() {
			destination = relcompile.NewName(joined.Entry, joined.Member+"#tag")
		}
		surfaces.expansion(relcompile.NewName(declaration.Selection.Axis, declaration.Selection.Member), destination, selectionInputRelations(declaration, relations), 64)
		surfaces.expression(schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: declaration.Selection.Axis.Key}, declaration.Selection.Member)
	}
	// Parent/Ordinal are the physical address of a nested member set only.
	// A KeyVector publisher is installed as an ordinary relation here, but its
	// span binding is deliberately NOT synthesized by this fixture: Resolve
	// derives it from an installed sealed member catalog.
	if parent := declaration.Parent; parent.Available() {
		surfaces.relation(relcompile.NewName(parent.Axis, parent.Member), scope)
		surfaces.coordinate(joined, relcompile.CoordinateParent)
		surfaces.coordinate(joined, relcompile.CoordinateOrdinal)
	}
	if publisher := declaration.KeyVector; publisher.Available() {
		surfaces.relation(relcompile.NewName(publisher.Axis, publisher.Member), scope)
	}
	// The occurrence identity is projected from the rule's own candidate row,
	// so the column belongs to the candidate relation and never to the foreign
	// directory the read reaches through it.
	if projection := declaration.AddressIdentity; projection.Available() {
		surfaces.column(relcompile.NewName(projection.Axis, projection.Member), candidate)
		surfaces.coordinate(joined, relcompile.CoordinateOccurrence)
	}
	if declaration.Read.Form == ruleprogram.Selected && routed != nil {
		destination := relcompile.NewName(routed.output.Destination.Axis, routed.output.Destination.Member)
		surfaces.installProjection(routed.output.Destination, scope, joined, member.Destination, member.AxisRelationCandidate(member.RelationRef{Axis: joined.Entry, Member: joined.Member}))
		if err := surfaces.registry.DeclareCoordinate(joined, relcompile.CoordinateDestination, destination); err != nil {
			// The route destination is an owner projection. If its column is
			// absent, retain the incomplete declaration so Resolve reports the
			// missing owner row instead of installing a coordinate by convention.
			return
		}
	} else {
		surfaces.denominator(declaration.Read.Contract.DenominatorRef, scope)
	}
}

func (surfaces *owners) installProjection(projection member.ProjectionRef, scope relcompile.Name, relation relcompile.Name, role member.Role, provider member.CandidateRef) {
	surfaces.t.Helper()
	if !projection.Available() {
		return
	}
	name := relcompile.NewName(projection.Axis, projection.Member)
	declared, err := surfaces.registry.DeclaredProjection(relcompile.Site{Path: "test.projection"}, name)
	if err != nil {
		// An absent owner row is a real census finding. Do not mint a local
		// projection just to make the fixture lower: Resolve must receive the
		// incomplete registry and report the missing owner statement at the
		// authored Program site.
		return
	}
	declaredRelation := relcompile.NewName(projection.Axis, declared.Relation)
	if _, relationErr := surfaces.registry.Relation(relcompile.Site{Path: "test.projection.relation"}, declaredRelation); relationErr != nil {
		// The owner projection is authoritative about its relation. The test
		// declaration must install that exact relation before its column; it is
		// not legal to retain a same-named synthetic relation under a different
		// owner row.
		surfaces.relation(declaredRelation, scope)
	}
	column := relcompile.NewName(projection.Axis, projection.Member)
	surfaces.column(column, declaredRelation)
	if err := surfaces.registry.InstallProjection(name, declaredRelation, column, declared.Role, declared.Result, declared.CandidateProvider); err != nil {
		surfaces.t.Fatalf("install projection %v: %v", projection, err)
	}
}

// installOutput installs the published Factor a rule writes. Output.Column is
// the writer fact column; a routed output's Destination remains installed on
// its route relation by installJoin.
func (surfaces *owners) installOutput(output ruleprogram.OutputDecl, scope relcompile.Name) outputInstallation {
	surfaces.t.Helper()
	relation := relcompile.NewName(output.Column.Axis, output.Column.Key)
	surfaces.relation(relation, scope)
	column := relcompile.NewName(output.Column.Axis, output.Column.Key)
	surfaces.column(column, relation)
	key := relcompile.NewName(relation.Entry, relation.Member+"#key")
	// The writer result is the actual owner-axis key carrier supplied by the
	// candidate relation in the sealed member catalog, never a fixture-local
	// carrier spelling.
	if err := surfaces.registry.InstallOutput(column, relation, column, key, surfaces.axisResult(output.Column.Axis)); err != nil {
		surfaces.t.Fatalf("install output %v: %v", column, err)
	}
	return outputInstallation{relation: relation, column: column, key: key}
}

func (surfaces *owners) axisResult(axis schema.EntryReference) carrier.Key {
	surfaces.t.Helper()
	if result := surfaces.results[axis]; result.Available() {
		return result
	}
	switch axis.Key {
	case "call":
		return calldomain.CallKeyCarrier
	case "effect":
		return effectdomain.EffectKeyCarrier
	case "heap":
		return heapdomain.HeapKeyCarrier
	case "pack":
		return packdomain.RootCarrier
	case "placement":
		return placementdomain.PlacementKeyCarrier
	case "placement-suspension-evidence":
		// OutputBinding.Result is the axis signature's key carrier. The
		// evidence axis writes EvidenceFactCarrier, but its Factor rows are
		// still addressed by the shared PlacementKeyCarrier coordinate.
		return placementsuspension.PlacementKeyCarrier
	case "static-type":
		return staticdomain.CoordinateCarrier
	case "typestate":
		return typestateprogram.CellCarrier
	case "value":
		return valuedomain.ValueCoordinateCarrier
	default:
		surfaces.t.Fatalf("no sealed axis signature key for %v", axis)
		return ""
	}
}

// installSyntheticHeapCatalog is the owner vocabulary for the two focused
// relational laws that deliberately use heap-shaped names without depending
// on the production heap catalog. It makes every relation/projection/
// selection explicit and gives the writer an owner-issued result carrier.
func installSyntheticHeapCatalog(t *testing.T, surfaces *owners) {
	t.Helper()
	axis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}
	candidate := member.AxisRelationCandidate(member.RelationRef{Axis: axis, Member: "heap/candidates"})
	const (
		keyCarrier   carrier.Key = "test/heap/key"
		factCarrier  carrier.Key = "test/heap/fact"
		tagCarrier   carrier.Key = "test/heap/tag"
		routeCarrier carrier.Key = "test/heap/route"
	)
	catalog, ok := member.NewCatalog(
		[]carrier.Authority{
			{Carrier: keyCarrier, Capability: carrier.Equatable},
			{Carrier: factCarrier, Capability: carrier.Ascending},
			{Carrier: tagCarrier, Capability: carrier.DecodeOnly},
			{Carrier: routeCarrier, Capability: carrier.DecodeOnly},
		}, nil,
		[]member.Relation{
			{Key: "heap/candidates", Subject: keyCarrier, CandidateProvider: candidate},
			{Key: "heap/published", Subject: factCarrier, CandidateProvider: candidate},
			{Key: "heap/facts", Subject: factCarrier, CandidateProvider: candidate},
			{Key: "heap/routes", Subject: routeCarrier, Inputs: []carrier.Key{tagCarrier}, CandidateProvider: candidate},
		},
		[]member.Projection{
			{Key: "heap/published-key", Relation: "heap/published", Role: member.Key, Result: keyCarrier, CandidateProvider: candidate},
			{Key: "heap/publication", Relation: "heap/published", Role: member.Destination, Result: keyCarrier, CandidateProvider: candidate},
			{Key: "heap/fact-key", Relation: "heap/facts", Role: member.Key, Result: keyCarrier, CandidateProvider: candidate},
			{Key: "heap/route-key", Relation: "heap/routes", Role: member.Key, Result: keyCarrier, CandidateProvider: candidate},
			{Key: "heap/route-tag", Relation: "heap/routes", Role: member.Predicate, Result: tagCarrier, CandidateProvider: candidate},
		},
		[]member.Reducer{
			{Key: "heap/self-reading-reducer", Inputs: []member.ReducerInput{{Axis: axis, Carrier: factCarrier, Form: member.Exact, Multiplicity: member.MultiplicityOne}}, Outputs: []member.ReducerOutput{{Axis: axis, Carrier: factCarrier}}},
			{Key: "heap/specimen-reducer", Inputs: []member.ReducerInput{{Axis: axis, Carrier: factCarrier, Form: member.Exact, Multiplicity: member.MultiplicityOne}}, Outputs: []member.ReducerOutput{{Axis: axis, Carrier: factCarrier}}},
		}, nil,
	)
	if !ok {
		t.Fatal("construct synthetic heap member catalog")
	}
	withSelection, ok := catalog.WithSelections([]member.Selection{{Key: "heap/route-selection", Relation: "heap/routes", Tag: "heap/route-tag"}})
	if !ok {
		t.Fatal("add synthetic heap selection catalog")
	}
	surfaces.installCatalogOverride(axis, withSelection, keyCarrier)
}

// installSyntheticRoutedCatalogs supplies the three explicit owner catalogs
// used by the hostile routed-selected law. The route destination imports the
// writer's result carrier; that cross-axis binding is the point of the law.
func installSyntheticRoutedCatalogs(t *testing.T, surfaces *owners, candidateAxis, routeAxis, writerAxis schema.EntryReference) (carrier.Key, carrier.Key) {
	t.Helper()
	candidateKey := carrier.Key("test/routed/candidate-key")
	routeKey := carrier.Key("test/routed/route-key")
	routeTag := carrier.Key("test/routed/route-tag")
	routeRow := carrier.Key("test/routed/route-row")
	writerKey := carrier.Key("test/routed/writer-key")
	writerFact := carrier.Key("test/routed/writer-fact")
	candidateRef := member.AxisRelationCandidate(member.RelationRef{Axis: candidateAxis, Member: "routed-law/candidates"})

	candidateCatalog, ok := member.NewCatalog(
		[]carrier.Authority{{Carrier: candidateKey, Capability: carrier.Equatable}}, nil,
		[]member.Relation{{Key: "routed-law/candidates", Subject: candidateKey, CandidateProvider: candidateRef}}, nil, nil, nil)
	if !ok {
		t.Fatal("construct routed candidate catalog")
	}
	surfaces.installCatalogOverride(candidateAxis, candidateCatalog, candidateKey)

	writerCatalog, ok := member.NewCatalog(
		[]carrier.Authority{
			{Carrier: writerKey, Capability: carrier.Equatable},
			{Carrier: writerFact, Capability: carrier.Ascending},
		}, nil,
		[]member.Relation{{Key: "routed-law/fact", Subject: writerFact, CandidateProvider: candidateRef}}, nil, nil, nil)
	if !ok {
		t.Fatal("construct routed writer catalog")
	}
	surfaces.installCatalogOverride(writerAxis, writerCatalog, writerKey)

	routeCandidate := member.AxisRelationCandidate(member.RelationRef{Axis: candidateAxis, Member: "routed-law/candidates"})
	routeCatalog, ok := member.NewCatalog(
		[]carrier.Authority{
			{Carrier: routeKey, Capability: carrier.Equatable},
			{Carrier: routeTag, Capability: carrier.DecodeOnly},
			{Carrier: routeRow, Capability: carrier.DecodeOnly},
		}, []carrier.Binding{{Use: writerKey, Ref: carrier.Ref{Owner: writerAxis, Carrier: writerKey}}},
		[]member.Relation{{Key: "routed-law/routes", Subject: routeRow, Inputs: []carrier.Key{routeTag}, CandidateProvider: routeCandidate}},
		[]member.Projection{
			{Key: "routed-law/route-key", Relation: "routed-law/routes", Role: member.Key, Result: routeKey, CandidateProvider: routeCandidate},
			{Key: "routed-law/route-tag", Relation: "routed-law/routes", Role: member.Predicate, Result: routeTag, CandidateProvider: routeCandidate},
			{Key: "routed-law/route-destination", Relation: "routed-law/routes", Role: member.Destination, Result: writerKey, CandidateProvider: routeCandidate},
		}, nil, nil)
	if !ok {
		t.Fatal("construct routed route catalog")
	}
	withSelection, ok := routeCatalog.WithSelections([]member.Selection{{Key: "routed-law/route-selection", Relation: "routed-law/routes", Tag: "routed-law/route-tag"}})
	if !ok {
		t.Fatal("add routed route selection catalog")
	}
	surfaces.installCatalogOverride(routeAxis, withSelection, routeKey)
	return writerKey, writerFact
}

func foldInputRelations(program ruleprogram.Program) []relcompile.Name {
	result := make([]relcompile.Name, 0, len(program.Fold.Inputs))
	for _, input := range program.Fold.Inputs {
		declaration, ok := program.JoinAt(int(input))
		if !ok {
			continue
		}
		result = append(result, relcompile.NewName(declaration.Relation.Axis, declaration.Relation.Member))
	}
	return result
}

func selectionInputRelations(declaration ruleprogram.JoinDecl, relations []relcompile.Name) []relcompile.Name {
	result := make([]relcompile.Name, 0, len(declaration.Sources))
	for _, source := range declaration.Sources {
		if source.Candidate {
			if len(relations) != 0 {
				result = append(result, relations[0])
			}
			continue
		}
		index := int(source.Position) + 1
		if index >= 0 && index < len(relations) {
			result = append(result, relations[index])
		}
	}
	return result
}

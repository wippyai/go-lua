// Package generator projects one owner-authored member definition into its
// declaration-only cold catalog and its bind-time relation owner.  The latter
// is still generated source: it contains direct calls and dense switches, not
// callbacks, reflection, or a runtime-discovered table.
package generator

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"os"
	"strings"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

// The sealed reduction outcome vocabulary the generated bind-time owner reads
// a materializer's conclusion through. There is one outcome vocabulary in the
// analyzer, so the generator names it rather than letting an owner declare a
// boolean of its own meaning.
const (
	OutcomePackagePath = "github.com/wippyai/go-lua/analysis/schema/structure"
	OutcomeType        = "ReductionOutcome"
	OutcomeConcrete    = "Concrete"
	OutcomeRefuse      = "Refuse"
)

// The generator-published dense Factor coordinate of an axis. One name is
// used for every axis because the type is not an owner's choice: it is the
// position a key of that axis occupies in the Factor its owner binds, and an
// axis that hand-exported its own spelling would be a second authority over
// the same coordinate.
const (
	CoordinateType = "DenseCoordinate"

	// The execution package the generated read handle is typed against. It is
	// named here rather than by an axis so no owner can hand a consumer a read
	// sealed against something else.
	ExecutionPackagePath = "github.com/wippyai/go-lua/analysis/engine/execution"
)

// Artifact is the pair of generated owner artifacts. Cold is the declaration
// catalog; Relations is the immutable bind-time relation owner. Typed
// metadata is returned only through Resolve and is never retained by runtime
// code.
type Artifact struct {
	Cold      []byte
	Relations []byte
	ExactFold []byte
}

// KeyBinding is the resolved axis-level key normalization needed by a future
// composition generator.
type KeyBinding struct {
	Carrier member.Carrier
	Input   definition.GoType
	Dense   definition.GoType
	// Coordinate is the generated dense Factor coordinate type of this axis,
	// resolved in the axis's own package. It is derived, never authored: the
	// declaration states a width and the generator publishes the type.
	Coordinate definition.GoType
	Normalizer definition.GoSymbol
}

// RelationBinding is typed metadata for exactly one relation declaration.
type RelationBinding struct {
	Key               schema.Key
	Subject           definition.GoType
	Inputs            []definition.GoType
	CandidateProvider member.CandidateRef
	CandidateResolver definition.GoSymbol
	CandidateOrdinal  definition.GoSymbol
	CandidateAt       definition.GoSymbol
	CandidateCount    definition.GoSymbol
	// CandidateIdentityAt is present exactly on a globally addressed relation:
	// the axis publishes the occurrence identity of every dense candidate, so
	// the relation is resolved from an occurrence alone and carries its own
	// occurrence inventory.
	CandidateIdentityAt definition.GoSymbol
	// CandidateRelation is the local relation ordinal for the explicitly named
	// CandidateProvider. It is populated only when that provider belongs to
	// this axis; foreign providers retain CandidateProvider for composition to
	// resolve and deliberately have no local ordinal.
	CandidateRelation    uint32
	HasCandidateRelation bool
	// CandidateProviderLocal distinguishes a same-axis dense ordinal from a
	// foreign owner-qualified provider. Foreign providers intentionally have
	// no local ordinal: composition resolves their axis/member pair once.
	CandidateProviderLocal bool
	// Derivation is the optional direct composition call that materializes the
	// dependent relation subject from its explicit inputs. It is metadata only;
	// the generated RelationOwner deliberately does not retain or execute it.
	Derivation RelationDerivationBinding
	// MemberSet is the resolved nested ordered member set this relation
	// declares, present exactly when it declares one. It survives resolution
	// because a CHILD Program consumes it: the parent and the ordinal carrier
	// address an owner's members from outside, and the accessor pair is what
	// this axis's own bind-time owner answers them with.
	MemberSet MemberSetBinding
}

// MemberSetBinding is the resolved nested ordered member set of one relation:
// the parent whose rows carry the members, the carrier that keys a member
// under its parent, and the owner accessor pair that answers the census and
// the row.
//
// Present distinguishes a relation with no member set from one whose set
// happens to resolve to zero-value rows, which is the distinction a consumer
// has to make before it can address anything.
type MemberSetBinding struct {
	Present bool
	Parent  member.RelationRef
	Ordinal definition.GoType
	Count   definition.GoSymbol
	At      definition.GoSymbol
}

// RelationDerivationBinding is the resolved, callback-free source form for a
// dependent relation's direct Build/Count/At calls. StaticAxes remain in
// authored order for rule codegen to resolve against the sealed roster.
type RelationDerivationBinding struct {
	State      definition.GoType
	Build      definition.GoSymbol
	Count      definition.GoSymbol
	At         definition.GoSymbol
	StaticAxes []schema.EntryReference
}

// ProjectionBinding is typed metadata for exactly one projection declaration.
type ProjectionBinding struct {
	Key                    schema.Key
	Relation               schema.Key
	Role                   member.Role
	Result                 definition.GoType
	Accessor               definition.GoSymbol
	CandidateProvider      member.CandidateRef
	CandidateRelation      uint32
	CandidateProviderLocal bool
}

// ReducerInputBinding is typed metadata for one reducer input row. Tag and
// Route are the conditional carriers: a tag names which member of a selection
// the invocation folds, a route names the coordinate it publishes at. Whether
// either is present is the reading rule's plan, so both are optional here and
// the rule model settles them against that plan.
type ReducerInputBinding struct {
	Axis         schema.EntryReference
	Type         definition.GoType
	Form         member.ReadForm
	Multiplicity member.Multiplicity
	Tag          definition.GoType
	Route        definition.GoType
}

// ReducerOutputBinding is typed metadata for one reducer output row.
type ReducerOutputBinding struct {
	Axis schema.EntryReference
	Type definition.GoType
}

// ReducerBinding is typed metadata for exactly one reducer declaration.
type ReducerBinding struct {
	Key schema.Key
	// Rule is the rule whose contribution declared this reducer. It is the
	// provenance a generated dispatch row is attributable by.
	Rule schema.Key
	// Candidate is the optional owner-issued candidate/subject carrier. When
	// CandidatePresent is false the reducer signature starts with its joined
	// inputs; generated call sites use a true constant for the absent
	// candidate guard and never infer a carrier from a plan relation.
	Candidate         definition.GoType
	CandidatePresent  bool
	CandidateConstant bool
	Inputs            []ReducerInputBinding
	Outputs           []ReducerOutputBinding
	Implementation    definition.GoSymbol
}

// CarryTransformBinding is typed metadata for exactly one owner-issued carry
// transform. It is generator input for a future direct-call composition
// emitter; no function value is retained here. A receiver-bearing symbol is
// called as candidate.Method(input), while a free symbol is called as
// Symbol(candidate, input); both forms retain the direct (Output, bool) call
// contract derived from the carrier rows.
type CarryTransformBinding struct {
	Key            schema.Key
	Candidate      definition.GoType
	Input          definition.GoType
	Output         definition.GoType
	Implementation definition.GoSymbol
}

// Metadata is the generator-only, member-aligned hot input that a later
// composition generator can consume to emit direct calls. It is intentionally
// not serializable into or imported by the runtime value package.
type Metadata struct {
	// Axis is the owner axis whose definition produced this metadata. It is a
	// declaration key, not a second axis identity or digest; code generation
	// resolves it against the sealed rule-plan directory.
	Axis            schema.Key
	FactCarrier     member.Carrier
	FactType        definition.GoType
	Key             KeyBinding
	Relations       []RelationBinding
	Projections     []ProjectionBinding
	Reducers        []ReducerBinding
	CarryTransforms []CarryTransformBinding
}

// Resolve consumes the same owner source as Render and resolves carrier names
// into typed rows. The returned metadata remains in generator tooling; no
// runtime binding table is generated.
func Resolve(source definition.Definition) (Metadata, error) {
	if !source.Complete() {
		return Metadata{}, errors.New("member generator: incomplete member definition")
	}
	carriers := make(map[string]definition.Carrier, len(source.Carriers))
	for _, carrier := range source.Carriers {
		carriers[carrier.Name] = carrier
	}
	keyCarrier, keyOK := carriers[source.Binding.Key.Carrier]
	if !keyOK {
		return Metadata{}, errors.New("member generator: key carrier is not declared")
	}
	relationsByName := make(map[string]definition.Relation, len(source.Relations))
	relationsByKey := make(map[schema.Key]uint32, len(source.Relations))
	relations := make([]RelationBinding, len(source.Relations))
	for index, relation := range source.Relations {
		relationsByName[relation.Name] = relation
		relationsByKey[relation.Key] = uint32(index)
		inputTypes := make([]definition.GoType, len(relation.Inputs))
		for inputIndex, declaredInput := range relation.Inputs {
			inputName := declaredInput.Carrier
			inputTypes[inputIndex] = carriers[inputName].Type
		}
		if !relation.CandidateProvider.Available() {
			return Metadata{}, fmt.Errorf("member generator: relation %s has no explicit candidate provider", relation.Name)
		}
		relations[index] = RelationBinding{
			Key: relation.Key, Subject: carriers[relation.Subject].Type, Inputs: inputTypes,
			CandidateProvider: relation.CandidateProvider,
			CandidateResolver: relation.CandidateResolver, CandidateOrdinal: relation.CandidateOrdinal,
			CandidateAt: relation.CandidateAt, CandidateCount: relation.CandidateCount,
			CandidateIdentityAt: relation.CandidateIdentityAt,
			Derivation: RelationDerivationBinding{
				State: relation.Derivation.State, Build: relation.Derivation.Build,
				Count: relation.Derivation.Count, At: relation.Derivation.At,
				StaticAxes: append([]schema.EntryReference(nil), relation.Derivation.StaticAxes...),
			},
		}
		if relation.MemberParent.Available() {
			ordinal, ordinalOK := carriers[relation.MemberOrdinal]
			if !ordinalOK {
				return Metadata{}, fmt.Errorf("member generator: relation %s keys its member set by an undeclared carrier", relation.Name)
			}
			relations[index].MemberSet = MemberSetBinding{
				Present: true, Parent: relation.MemberParent, Ordinal: ordinal.Type,
				Count: relation.MemberCount, At: relation.MemberAt,
			}
		}
	}
	owner := source.Binding.Key.Normalizer.Receiver
	for index, relation := range source.Relations {
		// An issued provider has no local directory to bind: there is no axis
		// relation to take an ordinal from, so the binding stays foreign and
		// the emitted accessors reach for nothing.
		if relation.CandidateProvider.Issued() {
			continue
		}
		providerOrdinal, providerLocal := relationsByKey[relation.CandidateProvider.AxisRelation.Member]
		if relation.CandidateProvider.AxisRelation.Axis.Key == source.Axis {
			if !providerLocal {
				return Metadata{}, fmt.Errorf("member generator: relation %s candidate provider is not declared", relation.Name)
			}
			relations[index].CandidateRelation = providerOrdinal
			relations[index].HasCandidateRelation = true
			relations[index].CandidateProviderLocal = true
			provider := source.Relations[providerOrdinal]
			if !sameOwnerSymbol(provider.CandidateResolver, owner) || !sameOwnerSymbol(provider.CandidateOrdinal, owner) || !sameOwnerSymbol(provider.CandidateAt, owner) {
				return Metadata{}, fmt.Errorf("member generator: relation %s candidate provider is not owned by the axis owner", relation.Name)
			}
			if relation.Key != provider.Key {
				providerCarrier := carriers[provider.Subject]
				inputCarrier := false
				for _, declaredInput := range relation.Inputs {
					inputName := declaredInput.Carrier
					if input, ok := carriers[inputName]; ok && sameGoType(input.Type, providerCarrier.Type) {
						inputCarrier = true
						break
					}
				}
				if !inputCarrier {
					return Metadata{}, fmt.Errorf("member generator: relation %s provider carrier is not an input", relation.Name)
				}
			}
			continue
		}
		// A foreign provider is resolved by composition. Its directory must not
		// be copied into this owner definition.
		if !optionalSymbol(relation.CandidateResolver) || !optionalSymbol(relation.CandidateOrdinal) || !optionalSymbol(relation.CandidateAt) {
			return Metadata{}, fmt.Errorf("member generator: relation %s foreign provider has local directory symbols", relation.Name)
		}
	}
	projections := make([]ProjectionBinding, len(source.Projections))
	for index, projection := range source.Projections {
		relation, relationOK := relationsByName[projection.Relation]
		if !relationOK {
			return Metadata{}, errors.New("member generator: projection relation is not declared")
		}
		relationOrdinal, relationOrdinalOK := relationOrdinalByName(source.Relations, projection.Relation)
		if !relationOrdinalOK {
			return Metadata{}, fmt.Errorf("member generator: projection %s relation ordinal is unavailable", projection.Name)
		}
		projections[index] = ProjectionBinding{
			Key: projection.Key, Relation: relation.Key, Role: projection.Role, Result: carriers[projection.Result].Type,
			Accessor: projection.Accessor, CandidateProvider: projection.CandidateProvider, CandidateRelation: relations[relationOrdinal].CandidateRelation,
			CandidateProviderLocal: relations[relationOrdinal].CandidateProviderLocal,
		}
		if !projection.CandidateProvider.Available() || projection.CandidateProvider != relation.CandidateProvider {
			return Metadata{}, fmt.Errorf("member generator: projection %s candidate provider mismatches relation", projection.Name)
		}
		// -1 is the sole-result accessor: the owner publishes exactly this
		// projection and no fact beside it. 0 and 1 select one result of a
		// pair. Any other index names a result the emitted direct call has no
		// binding for.
		if projection.Accessor.ResultIndex < -1 || projection.Accessor.ResultIndex > 1 {
			return Metadata{}, fmt.Errorf("member generator: projection %s must select accessor result -1, 0 or 1", projection.Name)
		}
	}
	reducers := make([]ReducerBinding, len(source.Reducers))
	for index, reducer := range source.Reducers {
		var candidateType definition.GoType
		candidatePresent := reducer.Candidate != ""
		if candidatePresent {
			candidate, candidateOK := carriers[reducer.Candidate]
			if !candidateOK {
				return Metadata{}, fmt.Errorf("member generator: reducer %s candidate carrier is not declared", reducer.Name)
			}
			candidateType = candidate.Type
		}
		inputs := make([]ReducerInputBinding, len(reducer.Inputs))
		for inputIndex, input := range reducer.Inputs {
			row := ReducerInputBinding{Axis: input.Axis, Type: carriers[input.Carrier].Type, Form: input.Form, Multiplicity: input.Multiplicity}
			if input.Tag != "" {
				row.Tag = carriers[input.Tag].Type
			}
			if input.Route != "" {
				row.Route = carriers[input.Route].Type
			}
			inputs[inputIndex] = row
		}
		outputs := make([]ReducerOutputBinding, len(reducer.Outputs))
		for outputIndex, output := range reducer.Outputs {
			outputs[outputIndex] = ReducerOutputBinding{Axis: output.Axis, Type: carriers[output.Carrier].Type}
		}
		reducers[index] = ReducerBinding{
			Key: reducer.Key, Rule: reducer.Rule, Candidate: candidateType, CandidatePresent: candidatePresent, CandidateConstant: !candidatePresent,
			Inputs: inputs, Outputs: outputs, Implementation: reducer.Implementation,
		}
	}
	transforms := make([]CarryTransformBinding, len(source.CarryTransforms))
	for index, transform := range source.CarryTransforms {
		candidate, candidateOK := carriers[transform.Candidate]
		input, inputOK := carriers[transform.Input]
		output, outputOK := carriers[transform.Output]
		if !candidateOK || !inputOK || !outputOK {
			return Metadata{}, errors.New("member generator: carry transform carrier is not declared")
		}
		transforms[index] = CarryTransformBinding{
			Key: transform.Key, Candidate: candidate.Type, Input: input.Type, Output: output.Type,
			Implementation: transform.Implementation,
		}
	}
	// The dense coordinate is published in the axis's own package, which is the
	// package its fact carrier is declared in: a Factor is the pair of that
	// fact and the coordinate it is indexed by, and they cannot be owned by two
	// packages. A key carrier borrowed from another axis therefore does not
	// decide where the coordinate lands.
	factCarrier := carriers[source.Signature.Fact]
	if factCarrier.Type.PackagePath == "" {
		return Metadata{}, errors.New("member generator: fact carrier has no declaring package")
	}
	coordinateType := definition.GoType{PackagePath: factCarrier.Type.PackagePath, Name: CoordinateType}
	return Metadata{
		Axis:            source.Axis,
		FactCarrier:     member.Carrier(carriers[source.Signature.Fact].Key),
		FactType:        carriers[source.Signature.Fact].Type,
		Key:             KeyBinding{Carrier: keyCarrier.Key, Input: keyCarrier.Type, Dense: source.Binding.Key.Dense, Coordinate: coordinateType, Normalizer: source.Binding.Key.Normalizer},
		Relations:       relations,
		Projections:     projections,
		Reducers:        reducers,
		CarryTransforms: transforms,
	}, nil
}

func optionalSymbol(symbol definition.GoSymbol) bool {
	return symbol.PackagePath == "" && symbol.Name == "" && symbol.Receiver == (definition.GoType{}) &&
		!symbol.ReceiverPointer && symbol.ResultIndex == 0
}

func sameOwnerSymbol(symbol definition.GoSymbol, owner definition.GoType) bool {
	return symbol.Receiver == owner && symbol.PackagePath == owner.PackagePath
}

// relationByKey resolves one declared relation by its owner-issued key.
func relationByKey(source definition.Definition, key schema.Key) (definition.Relation, bool) {
	for _, relation := range source.Relations {
		if relation.Key == key {
			return relation, true
		}
	}
	return definition.Relation{}, false
}

func relationOrdinalByName(relations []definition.Relation, name string) (uint32, bool) {
	for index, relation := range relations {
		if relation.Name == name {
			return uint32(index), true
		}
	}
	return 0, false
}

// Render validates and renders the cold output without touching the
// filesystem. Resolve is called first so the same source is proven consumable
// by the future hot composition path.
func Render(packageName string, source definition.Definition) (Artifact, error) {
	if packageName == "" || !token.IsIdentifier(packageName) {
		return Artifact{}, errors.New("member generator: invalid package name")
	}
	if _, err := Resolve(source); err != nil {
		return Artifact{}, err
	}
	cold, err := renderCold(packageName, source)
	if err != nil {
		return Artifact{}, err
	}
	relations, err := renderRelations(packageName, source)
	if err != nil {
		return Artifact{}, err
	}
	exactFold, err := renderExactFold(packageName, source)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{Cold: cold, Relations: relations, ExactFold: exactFold}, nil
}

// Generate writes the generated cold output, or checks that the path is
// already fresh when check is true.
func Generate(packageName string, source definition.Definition, coldPath string, check bool) error {
	if coldPath == "" {
		return errors.New("member generator: cold path is required")
	}
	artifact, err := Render(packageName, source)
	if err != nil {
		return err
	}
	if check {
		return fresh(coldPath, artifact.Cold)
	}
	if err := os.WriteFile(coldPath, artifact.Cold, 0o644); err != nil {
		return fmt.Errorf("member generator: write cold output: %w", err)
	}
	return nil
}

// GenerateRelations writes or checks the generated bind-time relation owner.
// It is separate from Generate so existing cold-only callers remain stable
// while a package can opt into the second generated artifact explicitly.
func GenerateRelations(packageName string, source definition.Definition, relationPath string, check bool) error {
	if relationPath == "" {
		return errors.New("member generator: relation path is required")
	}
	artifact, err := Render(packageName, source)
	if err != nil {
		return err
	}
	if check {
		return fresh(relationPath, artifact.Relations)
	}
	if err := os.WriteFile(relationPath, artifact.Relations, 0o644); err != nil {
		return fmt.Errorf("member generator: write relation output: %w", err)
	}
	return nil
}

// GenerateExactFold writes or checks the generated same-axis exact-fold
// reducer dispatch. It is a separate artifact because the relation owner is a
// construction directory, while reducer dispatch belongs to execution.
func GenerateExactFold(packageName string, source definition.Definition, path string, check bool) error {
	if path == "" {
		return errors.New("member generator: exact-fold path is required")
	}
	artifact, err := Render(packageName, source)
	if err != nil {
		return err
	}
	if check {
		return fresh(path, artifact.ExactFold)
	}
	if err := os.WriteFile(path, artifact.ExactFold, 0o644); err != nil {
		return fmt.Errorf("member generator: write exact-fold output: %w", err)
	}
	return nil
}

// GenerateAll writes or checks both generated artifacts from one owner
// definition. The command-line generator uses this operation for checked-in
// package output and its freshness law.
func GenerateAll(packageName string, source definition.Definition, coldPath, relationPath string, check bool) error {
	if err := Generate(packageName, source, coldPath, check); err != nil {
		return err
	}
	return GenerateRelations(packageName, source, relationPath, check)
}

func fresh(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("member generator: read %s: %w", path, err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("member generator: stale generated output: %s", path)
	}
	return nil
}

func renderCold(packageName string, source definition.Definition) ([]byte, error) {
	carriers := make(map[string]string, len(source.Carriers))
	for _, carrier := range source.Carriers {
		carriers[carrier.Name] = carrier.Name
	}
	relations := make(map[string]string, len(source.Relations))
	for _, relation := range source.Relations {
		relations[relation.Name] = relation.Name
	}
	projectionNames := make(map[string]string, len(source.Projections))
	for _, projection := range source.Projections {
		projectionNames[projection.Name] = projection.Name
	}
	var out strings.Builder
	fmt.Fprintf(&out, "// Code generated by axis member definition generator; DO NOT EDIT.\n\npackage %s\n\n", packageName)
	relationsPath := "generated_relation_owner.go"
	if source.RelationsPath != "" {
		relationsPath = source.RelationsPath
	}
	relationsPackageFlag := ""
	if source.RelationsPackage != "" {
		relationsPackageFlag = fmt.Sprintf(" -relations-package %s", source.RelationsPackage)
	}
	fmt.Fprintf(&out, "//go:generate go run %sanalysis/schema/axis/member/generator/cmd -source %s -cold rule_members.go -relations %s%s\n\n", generatorPrefix(source), source.Axis, relationsPath, relationsPackageFlag)
	out.WriteString("import (\n\tschemaapi \"github.com/wippyai/go-lua/analysis/schema\"\n\t\"github.com/wippyai/go-lua/analysis/schema/axis/member\"\n)\n\n")
	out.WriteString("const (\n")
	for _, relation := range source.Relations {
		fmt.Fprintf(&out, "\t%s schemaapi.Key = %q\n", relation.Name, relation.Key)
	}
	for _, projection := range source.Projections {
		fmt.Fprintf(&out, "\t%s schemaapi.Key = %q\n", projection.Name, projection.Key)
	}
	for _, reducer := range source.Reducers {
		fmt.Fprintf(&out, "\t%s schemaapi.Key = %q\n", reducer.Name, reducer.Key)
	}
	for _, selection := range source.Selections {
		fmt.Fprintf(&out, "\t%s schemaapi.Key = %q\n", selection.Name, selection.Key)
	}
	for _, transform := range source.CarryTransforms {
		fmt.Fprintf(&out, "\t%s schemaapi.Key = %q\n", transform.Name, transform.Key)
	}
	for _, carrier := range source.Carriers {
		fmt.Fprintf(&out, "\t%s member.Carrier = %q\n", carrier.Name, carrier.Key)
	}
	out.WriteString(")\n\n")
	fmt.Fprintf(&out, "// AxisMemberCatalog is %s's declaration-only member vocabulary.\n", source.Axis)
	out.WriteString("func AxisMemberCatalog() member.Catalog {\n")
	if len(source.Reducers) != 0 {
		fmt.Fprintf(&out, "\tvalueAxis := schemaapi.EntryReference{Surface: schemaapi.SurfaceKindAxis, Key: %q}\n", source.Axis)
	}
	out.WriteString("\tcatalog, ok := member.NewCatalog(\n")
	out.WriteString("\t\t[]member.Relation{\n")
	for _, relation := range source.Relations {
		fmt.Fprintf(&out, "\t\t\t{Key: %s, Subject: %s, CandidateProvider: %s", relations[relation.Name], carriers[relation.Subject], candidateProviderExpression(relation.CandidateProvider))
		if len(relation.Inputs) != 0 {
			out.WriteString(", Inputs: []member.Carrier{")
			for index, input := range relation.Inputs {
				if index != 0 {
					out.WriteString(", ")
				}
				out.WriteString(carriers[input.Carrier])
			}
			out.WriteString("}")
		}
		// A correspondence is cold catalog data for the same reason: it is what
		// tells a child Program that a foreign candidate addresses these rows,
		// and it resolves through no owner symbol of this axis at all.
		if len(relation.Correspondences) != 0 {
			out.WriteString(", Correspondences: []member.RelationRef{")
			for index, correspondence := range relation.Correspondences {
				if index != 0 {
					out.WriteString(", ")
				}
				out.WriteString(relationProviderExpression(correspondence))
			}
			out.WriteString("}")
		}
		// A nested member set is cold catalog data, not just owner symbols. The
		// parent and the ordinal carrier are what a child Program addresses an
		// owner's members by, so they are emitted beside the row rather than
		// left to the bind-time accessors this axis alone can call.
		if relation.MemberParent.Available() {
			fmt.Fprintf(&out, ", Parent: %s, Ordinal: %s", relationProviderExpression(relation.MemberParent), carriers[relation.MemberOrdinal])
		}
		// A published key vector is cold catalog data for the same reason a
		// nested set is: it is the other addressing a whole-vector read can
		// have, and the Program that restates it is authenticated against this
		// row. The accessors stay with the owner; what a child reads here is
		// that the span exists and comes from this directory.
		if relation.KeyVectorCount.Available() && relation.KeyVectorAt.Available() {
			out.WriteString(", PublishesKeyVector: true")
		}
		out.WriteString("},\n")
	}
	out.WriteString("\t\t},\n\t\t[]member.Projection{\n")
	for _, projection := range source.Projections {
		role, ok := roleExpression(projection.Role)
		if !ok {
			return nil, fmt.Errorf("member generator: unsupported projection role %d", projection.Role)
		}
		fmt.Fprintf(&out, "\t\t\t{Key: %s, Relation: %s, Role: %s, Result: %s, CandidateProvider: %s},\n", projection.Name, relations[projection.Relation], role, carriers[projection.Result], candidateProviderExpression(projection.CandidateProvider))
	}
	out.WriteString("\t\t},\n\t\t[]member.Reducer{\n")
	for _, reducer := range source.Reducers {
		fmt.Fprintf(&out, "\t\t\t{Key: %s, Inputs: []member.ReducerInput{\n", reducer.Name)
		for _, input := range reducer.Inputs {
			form, formOK := formExpression(input.Form)
			multiplicity, multiplicityOK := multiplicityExpression(input.Multiplicity)
			if !formOK || !multiplicityOK {
				return nil, errors.New("member generator: unsupported reducer input vocabulary")
			}
			fmt.Fprintf(&out, "\t\t\t\t{Axis: %s, Carrier: %s, Form: %s, Multiplicity: %s", coldEntryReferenceExpression(input.Axis, source.Axis), carriers[input.Carrier], form, multiplicity)
			if input.Tag != "" {
				fmt.Fprintf(&out, ", Tag: %s", carriers[input.Tag])
			}
			if input.Route != "" {
				fmt.Fprintf(&out, ", Route: %s", carriers[input.Route])
			}
			out.WriteString("},\n")
		}
		out.WriteString("\t\t\t}, Outputs: []member.ReducerOutput{\n")
		for _, output := range reducer.Outputs {
			fmt.Fprintf(&out, "\t\t\t\t{Axis: %s, Carrier: %s},\n", coldEntryReferenceExpression(output.Axis, source.Axis), carriers[output.Carrier])
		}
		out.WriteString("\t\t\t}")
		if reducer.Structural {
			out.WriteString(", Structural: true")
		}
		out.WriteString("},\n")
	}
	out.WriteString("\t\t},\n\t\t[]member.CarryTransform{\n")
	for _, transform := range source.CarryTransforms {
		fmt.Fprintf(&out, "\t\t\t{Key: %s, Candidate: %s, Input: %s, Output: %s},\n", transform.Name, carriers[transform.Candidate], carriers[transform.Input], carriers[transform.Output])
	}
	fmt.Fprintf(&out, "\t\t},\n\t)\n\tif !ok {\n\t\tpanic(%q)\n\t}\n", packageName+": invalid axis member catalog")
	// An axis that publishes produced rows extends its catalog with the
	// operations that publish them. An axis that publishes none emits nothing,
	// so its generated catalog is exactly the one it had.
	if len(source.Selections) != 0 {
		out.WriteString("\tcatalog, ok = catalog.WithSelections([]member.Selection{\n")
		for _, selection := range source.Selections {
			fmt.Fprintf(&out, "\t\t{Key: %s, Relation: %s, Tag: %s},\n",
				selection.Name, relations[selection.Relation], projectionNames[selection.Tag])
		}
		fmt.Fprintf(&out, "\t})\n\tif !ok {\n\t\tpanic(%q)\n\t}\n", packageName+": invalid axis selection catalog")
	}
	out.WriteString("\treturn catalog\n}\n")
	return format.Source([]byte(out.String()))
}

// generatorPrefix is the path back to the repository root from the package a
// catalog is generated into. It is derived from the axis's own ImportPath
// because the directive is written INTO that package: a fixed prefix would
// hold only for the depth the first axis happened to sit at, and a package one
// level deeper would carry a directive that cannot run.
func generatorPrefix(source definition.Definition) string {
	const module = "github.com/wippyai/go-lua/"
	path := strings.TrimPrefix(source.ImportPath, module)
	if path == "" || path == source.ImportPath {
		return "../../"
	}
	depth := strings.Count(path, "/") + 1
	return strings.Repeat("../", depth)
}

// relationKeyName and projectionKeyName resolve a definition-local member name
// to the exported constant the generated catalog names it by, so a selection
// refers to its relation and tag the same way every other row does.
func relationKeyName(source definition.Definition, name string) string {
	for _, relation := range source.Relations {
		if relation.Name == name {
			return relation.Name
		}
	}
	return name
}

func projectionKeyName(source definition.Definition, name string) string {
	for _, projection := range source.Projections {
		if projection.Name == name {
			return projection.Name
		}
	}
	return name
}

// exactFoldArity is the greatest number of exact reads one generated fold
// consumes. It is the execution layer's typed product depth: the shared
// family chains one product extender per read, so a reducer that declares
// more reads is outside this generated shape and stays explicit.
const exactFoldArity = 3

type exactFoldReducer struct {
	ordinal               uint32
	reducer               ReducerBinding
	candidate             RelationBinding
	readRelation          uint32
	readKeys              []uint32
	destinationProjection uint32
}

// exactFoldReducers selects every reducer shape the shared axis executor
// understands: one canonical candidate directory, between one and
// exactFoldArity same-axis exact multiplicity-one reads of the fact carrier,
// and one fact output. Selection is generation-time only. A reducer outside
// this shape remains explicit and must be claimed by another sealed family;
// it is never coerced into this one at runtime.
func exactFoldReducers(source definition.Definition, metadata Metadata) ([]exactFoldReducer, error) {
	fact, factOK := carrierByName(source, source.Signature.Fact)
	if !factOK {
		return nil, errors.New("member generator: fact carrier is missing")
	}
	axis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: source.Axis}
	selected := make([]exactFoldReducer, 0)
	for ordinal, reducer := range metadata.Reducers {
		if !reducer.CandidatePresent || reducer.CandidateConstant || reducer.Implementation.Name == "" || reducer.Implementation.Receiver.Name != "" ||
			len(reducer.Inputs) < 1 || len(reducer.Inputs) > exactFoldArity || len(reducer.Outputs) != 1 {
			continue
		}
		shape := true
		for _, input := range reducer.Inputs {
			shape = shape && input.Axis == axis && sameGoType(input.Type, fact.Type) && input.Form == member.ReadFormExact && input.Multiplicity == member.MultiplicityOne && input.Tag.Name == "" && input.Route.Name == ""
		}
		shape = shape && reducer.Outputs[0].Axis == axis && sameGoType(reducer.Outputs[0].Type, fact.Type)
		if !shape {
			continue
		}
		var provider RelationBinding
		providers := 0
		for _, relation := range metadata.Relations {
			if relation.CandidateProviderLocal && sameGoType(relation.Subject, reducer.Candidate) && relation.CandidateAt.Name != "" {
				provider = relation
				providers++
			}
		}
		if providers != 1 {
			// The candidate carrier alone underdetermines the directory: an axis
			// whose key carrier subjects several candidate directories has no
			// single canonical provider for this reducer, because Relation and
			// Projection carry no rule provenance to attribute one by. Such a
			// reducer is outside this generated shape; SupportsExactFoldReducer
			// answers false for it and its install fails closed.
			continue
		}
		readRelation, readKeys, destination, geometryOK := exactFoldGeometry(metadata, provider, fact.Type, len(reducer.Inputs))
		if !geometryOK {
			return nil, fmt.Errorf("member generator: exact-fold reducer %s has no canonical read/write member geometry", reducer.Key)
		}
		selected = append(selected, exactFoldReducer{
			ordinal:               uint32(ordinal),
			reducer:               reducer,
			candidate:             provider,
			readRelation:          readRelation,
			readKeys:              readKeys,
			destinationProjection: destination,
		})
	}
	return selected, nil
}

// exactFoldGeometry resolves the owner-issued member coordinates consumed by
// one folding reducer. The relation/projection ordinals are part of the axis
// member catalog, not a rule-family convention: the generator derives them
// from the provider reference and the typed source/destination rows, then
// emits the result into the owner dispatch below. The declared key
// projections are taken in catalog order, which is the join order the rule's
// Program states.
func exactFoldGeometry(metadata Metadata, provider RelationBinding, fact definition.GoType, reads int) (uint32, []uint32, uint32, bool) {
	if !provider.HasCandidateRelation || !provider.CandidateProviderLocal || reads < 1 || reads > exactFoldArity {
		return 0, nil, 0, false
	}
	readRelation := ^uint32(0)
	for index, relation := range metadata.Relations {
		if relation.Key == provider.Key || relation.CandidateProvider != provider.CandidateProvider ||
			!relation.CandidateProviderLocal || !sameGoType(relation.Subject, fact) {
			continue
		}
		if readRelation != ^uint32(0) {
			return 0, nil, 0, false
		}
		readRelation = uint32(index)
	}
	if readRelation == ^uint32(0) {
		return 0, nil, 0, false
	}

	keys := make([]uint32, 0, reads)
	destination := ^uint32(0)
	for index, projection := range metadata.Projections {
		if projection.CandidateProvider != provider.CandidateProvider || !projection.CandidateProviderLocal {
			continue
		}
		if projection.Relation == metadata.Relations[readRelation].Key && projection.Role == member.Key && sameGoType(projection.Result, metadata.Key.Input) {
			if len(keys) >= reads {
				return 0, nil, 0, false
			}
			keys = append(keys, uint32(index))
			continue
		}
		if projection.Relation == provider.Key && projection.Role == member.Destination && sameGoType(projection.Result, metadata.Key.Input) {
			if destination != ^uint32(0) {
				return 0, nil, 0, false
			}
			destination = uint32(index)
		}
	}
	if len(keys) != reads || destination == ^uint32(0) {
		return 0, nil, 0, false
	}
	return readRelation, keys, destination, true
}

// renderExactFold emits the axis-local concrete reducer switch. It contains
// no function table, reflection, callback, or fallback. The boolean result is
// a construction fence: a sealed executor proves the reducer is supported
// before solve, so false is never a semantic Refuse outcome.
func renderExactFold(packageName string, source definition.Definition) ([]byte, error) {
	metadata, err := Resolve(source)
	if err != nil {
		return nil, err
	}
	reducers, err := exactFoldReducers(source, metadata)
	if err != nil {
		return nil, err
	}
	owner := source.Binding.Key.Normalizer.Receiver
	if owner.Name == "" {
		return nil, errors.New("member generator: exact-fold schema receiver is missing")
	}
	fact, factOK := carrierByName(source, source.Signature.Fact)
	if !factOK {
		return nil, errors.New("member generator: exact-fold fact carrier is missing")
	}
	aliases := make(packageAliases)
	add := func(path string) {
		if path == "" || packagePathPackage(path) == packageName {
			return
		}
		if _, exists := aliases[path]; exists {
			return
		}
		alias := packagePathPackage(path)
		for suffix := 2; ; suffix++ {
			used := false
			for _, existing := range aliases {
				if existing == alias {
					used = true
					break
				}
			}
			if !used {
				break
			}
			alias = fmt.Sprintf("%s%d", packagePathPackage(path), suffix)
		}
		aliases[path] = alias
	}
	add(OutcomePackagePath)
	add(owner.PackagePath)
	add(fact.Type.PackagePath)
	for _, selected := range reducers {
		add(selected.candidate.CandidateAt.PackagePath)
		add(selected.reducer.Implementation.PackagePath)
		add(selected.reducer.Candidate.PackagePath)
	}

	var out strings.Builder
	out.WriteString("// Code generated by axis member definition generator; DO NOT EDIT.\n")
	out.WriteString("// This file is the concrete same-axis exact-fold reducer dispatch.\n\n")
	fmt.Fprintf(&out, "package %s\n\n", packageName)
	out.WriteString("//go:generate go run " + generatorPrefix(source) + "analysis/schema/axis/member/generator/cmd -source ")
	out.WriteString(string(source.Axis))
	out.WriteString(" -exact-fold generated_exact_fold.go\n\n")
	out.WriteString("import (\n")
	for _, path := range aliases.paths() {
		fmt.Fprintf(&out, "\t%s %q\n", aliases[path], path)
	}
	out.WriteString(")\n\n")
	ownerType := qualifiedType(owner, packageName, aliases)
	factType := qualifiedType(fact.Type, packageName, aliases)
	outcome := aliases[OutcomePackagePath] + "." + OutcomeType
	refuse := aliases[OutcomePackagePath] + "." + OutcomeRefuse

	// ExactFoldArity is the read width every sealed payload and mapping is
	// dimensioned by. It is the execution layer's typed product depth rather
	// than an owner's choice, so it is published beside the dispatch that
	// consumes it.
	out.WriteString("// ExactFoldArity is the greatest number of exact reads one generated fold\n")
	out.WriteString("// consumes. The shared execution family chains one product extender per\n")
	out.WriteString("// read, so a reducer declaring more reads is outside this generated shape.\n")
	fmt.Fprintf(&out, "const ExactFoldArity = %d\n\n", exactFoldArity)

	out.WriteString("// SupportsExactFoldReducer reports whether the sealed axis member ordinal\n")
	out.WriteString("// names this generated execution shape. Unknown ordinals fail construction.\n")
	fmt.Fprintf(&out, "func SupportsExactFoldReducer(reducerOrdinal uint32) bool {\n\tswitch reducerOrdinal {\n")
	for _, selected := range reducers {
		fmt.Fprintf(&out, "\tcase %d:\n\t\treturn true\n", selected.ordinal)
	}
	out.WriteString("\tdefault:\n\t\treturn false\n\t}\n}\n\n")

	// ExactFoldMapping is the owner-issued correspondence between one reducer
	// member and the relation/projection members that its exact program
	// consumes. Runtime installation authenticates this mapping before
	// redeeming a candidate payload; no candidate ordinal can stand in for a
	// different relation member, and no read position can stand in for
	// another.
	out.WriteString("// ExactFoldMapping is one generated reducer/member correspondence.\n")
	out.WriteString("type ExactFoldMapping struct {\n")
	out.WriteString("\tReducerOrdinal uint32\n")
	out.WriteString("\tCandidateRelationMember uint32\n")
	out.WriteString("\tReadCount uint32\n")
	out.WriteString("\tReadRelationMember [ExactFoldArity]uint32\n")
	out.WriteString("\tReadKeyMember [ExactFoldArity]uint32\n")
	out.WriteString("\tDestinationProjectionMember uint32\n")
	out.WriteString("}\n\n")
	out.WriteString("// ExactFoldMappingAt issues the canonical geometry for one reducer.\n")
	fmt.Fprintf(&out, "func (schema *%s) ExactFoldMappingAt(reducerOrdinal uint32) (ExactFoldMapping, bool) {\n", ownerType)
	out.WriteString("\tif schema == nil {\n\t\treturn ExactFoldMapping{}, false\n\t}\n\tswitch reducerOrdinal {\n")
	for _, selected := range reducers {
		relations := make([]string, 0, len(selected.readKeys))
		keys := make([]string, 0, len(selected.readKeys))
		for _, key := range selected.readKeys {
			relations = append(relations, fmt.Sprintf("%d", selected.readRelation))
			keys = append(keys, fmt.Sprintf("%d", key))
		}
		fmt.Fprintf(&out, "\tcase %d:\n", selected.ordinal)
		fmt.Fprintf(&out, "\t\treturn ExactFoldMapping{ReducerOrdinal: %d, CandidateRelationMember: %d, ReadCount: %d, ReadRelationMember: [ExactFoldArity]uint32{%s}, ReadKeyMember: [ExactFoldArity]uint32{%s}, DestinationProjectionMember: %d}, true\n",
			selected.ordinal, selected.candidate.CandidateRelation, len(selected.readKeys), strings.Join(relations, ", "), strings.Join(keys, ", "), selected.destinationProjection)
	}
	out.WriteString("\tdefault:\n\t\treturn ExactFoldMapping{}, false\n\t}\n}\n\n")

	// ExactFoldPayload is the immutable, concrete candidate payload a
	// generated family seals at bind time. It is deliberately a closed sum of
	// candidate types rather than an erased interface value: execution carries
	// this payload directly and never redeems a candidate ordinal through the
	// owner directory again.
	out.WriteString("// ExactFoldPayload is one sealed concrete exact-fold reducer payload.\n")
	out.WriteString("// Its unexported cells can only be populated by the owner below.\n")
	out.WriteString("type ExactFoldPayload struct {\n")
	fmt.Fprintf(&out, "\towner *%s\n", ownerType)
	out.WriteString("\treducerOrdinal uint32\n")
	out.WriteString("\tcandidateRelationMember uint32\n")
	out.WriteString("\tcandidateOrdinal uint32\n")
	out.WriteString("\treadCount uint32\n")
	out.WriteString("\tavailable bool\n")
	for _, selected := range reducers {
		field := exactFoldPayloadField(selected.ordinal)
		candidateType := qualifiedType(selected.reducer.Candidate, packageName, aliases)
		fmt.Fprintf(&out, "\t%s %s\n", field, candidateType)
	}
	out.WriteString("}\n\n")
	out.WriteString("// ReducerOrdinal returns the sealed reducer identity of this payload.\n")
	out.WriteString("func (candidate ExactFoldPayload) ReducerOrdinal() (uint32, bool) {\n")
	out.WriteString("\tif !candidate.available {\n\t\treturn 0, false\n\t}\n\treturn candidate.reducerOrdinal, true\n}\n\n")
	out.WriteString("// CandidateOrdinal returns the sealed candidate identity of this payload.\n")
	out.WriteString("func (candidate ExactFoldPayload) CandidateOrdinal() (uint32, bool) {\n")
	out.WriteString("\tif !candidate.available {\n\t\treturn 0, false\n\t}\n\treturn candidate.candidateOrdinal, true\n}\n\n")
	out.WriteString("// CandidateRelationMember returns the owner-issued relation member\n")
	out.WriteString("// whose directory redeemed this payload.\n")
	out.WriteString("func (candidate ExactFoldPayload) CandidateRelationMember() (uint32, bool) {\n")
	out.WriteString("\tif !candidate.available {\n\t\treturn 0, false\n\t}\n\treturn candidate.candidateRelationMember, true\n}\n\n")
	out.WriteString("// ReadCount is the number of exact reads this payload's fold consumes.\n")
	out.WriteString("func (candidate ExactFoldPayload) ReadCount() (int, bool) {\n")
	out.WriteString("\tif !candidate.available {\n\t\treturn 0, false\n\t}\n\treturn int(candidate.readCount), true\n}\n\n")

	// ExactFoldPayloadAt is the one cold directory redemption. The installer
	// calls it once per sealed row and retains the returned concrete payload;
	// warm execution calls only ReduceExactFoldPayload below.
	out.WriteString("// ExactFoldPayloadAt redeems one candidate into an immutable concrete payload.\n")
	fmt.Fprintf(&out, "func (schema *%s) ExactFoldPayloadAt(reducerOrdinal, candidateRelationMember, candidateOrdinal uint32) (ExactFoldPayload, bool) {\n", ownerType)
	out.WriteString("\tif schema == nil {\n\t\treturn ExactFoldPayload{}, false\n\t}\n\tswitch reducerOrdinal {\n")
	for _, selected := range reducers {
		field := exactFoldPayloadField(selected.ordinal)
		candidateAt := directCall(selected.candidate.CandidateAt, owner, "schema", "candidate", []string{"int(candidateOrdinal)"}, packageName, aliases)
		fmt.Fprintf(&out, "\tcase %d:\n", selected.ordinal)
		fmt.Fprintf(&out, "\t\tif candidateRelationMember != %d {\n\t\t\treturn ExactFoldPayload{}, false\n\t\t}\n", selected.candidate.CandidateRelation)
		fmt.Fprintf(&out, "\t\tcandidate, candidateOK := %s\n", candidateAt)
		fmt.Fprintf(&out, "\t\tif !candidateOK {\n\t\t\treturn ExactFoldPayload{}, false\n\t\t}\n")
		fmt.Fprintf(&out, "\t\treturn ExactFoldPayload{owner: schema, reducerOrdinal: %d, candidateRelationMember: candidateRelationMember, candidateOrdinal: candidateOrdinal, readCount: %d, available: true, %s: candidate}, true\n", selected.ordinal, len(selected.readKeys), field)
	}
	out.WriteString("\tdefault:\n\t\treturn ExactFoldPayload{}, false\n\t}\n}\n\n")

	// ReduceExactFoldPayload is the warm direct dispatch. It receives the
	// concrete payload sealed above and the dense read vector the family
	// drained, so no candidate table, callback, reflection, or repeated cold
	// authority derivation is reachable from Execute.
	out.WriteString("// ReduceExactFoldPayload invokes the owner fold on a sealed payload.\n")
	out.WriteString("// The final boolean is structural dispatch validity, not a semantic outcome.\n")
	fmt.Fprintf(&out, "func (schema *%s) ReduceExactFoldPayload(candidate ExactFoldPayload, reads [ExactFoldArity]%s) (%s, %s, bool) {\n", ownerType, factType, factType, outcome)
	fmt.Fprintf(&out, "\tvar zero %s\n\tif schema == nil || !candidate.available || candidate.owner != schema {\n\t\treturn zero, %s, false\n\t}\n", factType, refuse)
	out.WriteString("\tswitch candidate.reducerOrdinal {\n")
	for _, selected := range reducers {
		field := exactFoldPayloadField(selected.ordinal)
		arguments := make([]string, 0, len(selected.readKeys)+1)
		arguments = append(arguments, "candidate."+field)
		for position := range selected.readKeys {
			arguments = append(arguments, fmt.Sprintf("reads[%d]", position))
		}
		fold := directCall(selected.reducer.Implementation, owner, "schema", "candidate", arguments, packageName, aliases)
		fmt.Fprintf(&out, "\tcase %d:\n", selected.ordinal)
		fmt.Fprintf(&out, "\t\tif candidate.readCount != %d {\n\t\t\treturn zero, %s, false\n\t\t}\n", len(selected.readKeys), refuse)
		fmt.Fprintf(&out, "\t\tresult, reduction := %s\n", fold)
		fmt.Fprintf(&out, "\t\tif !reduction.Available() || reduction == %s {\n\t\t\treturn zero, reduction, false\n\t\t}\n", refuse)
		out.WriteString("\t\treturn result, reduction, true\n")
	}
	fmt.Fprintf(&out, "\tdefault:\n\t\treturn zero, %s, false\n\t}\n}\n\n", refuse)
	return format.Source([]byte(out.String()))
}

// renderRelations emits the owner-local, bind-time relation directory. The
// generated methods deliberately use no member, candidate, or coordinate
// value in their public signature: all domain values below are short-lived
// temporaries between direct owner calls and are reduced to uint32 before the
// method returns.
func renderRelations(packageName string, source definition.Definition) ([]byte, error) {
	metadata, err := Resolve(source)
	if err != nil {
		return nil, err
	}
	owner := source.Binding.Key.Normalizer.Receiver
	if owner.Name == "" {
		return nil, errors.New("member generator: relation owner receiver is missing")
	}
	aliases := relationImportAliases(packageName, source, metadata)
	ownerType := qualifiedType(owner, packageName, aliases)
	ownerFieldType := ownerType
	if source.Binding.Key.Normalizer.ReceiverPointer {
		ownerFieldType = "*" + ownerFieldType
	}
	var factType definition.GoType
	factOK := false
	for _, carrier := range source.Carriers {
		if carrier.Name == source.Signature.Fact {
			factType = carrier.Type
			factOK = true
			break
		}
	}
	if !factOK {
		return nil, errors.New("member generator: fact carrier is missing")
	}
	materializable := make([]int, 0, len(source.Relations))
	global := make([]int, 0, len(source.Relations))
	for index, relation := range source.Relations {
		if !optionalSymbol(relation.Materialize) {
			materializable = append(materializable, index)
		}
		if !optionalSymbol(relation.CandidateIdentityAt) {
			global = append(global, index)
		}
	}

	var out strings.Builder
	out.WriteString("// Code generated by axis member definition generator; DO NOT EDIT.\n")
	out.WriteString("// This file is the immutable bind-time relation owner for the axis.\n\n")
	fmt.Fprintf(&out, "package %s\n\n", packageName)
	out.WriteString("import (\n")
	fmt.Fprintf(&out, "\t%q\n", ExecutionPackagePath)
	out.WriteString("\t\"github.com/wippyai/go-lua/analysis/identity\"\n")
	out.WriteString("\tmemberrelation \"github.com/wippyai/go-lua/analysis/schema/axis/member/relation\"\n")
	for _, path := range aliases.paths() {
		fmt.Fprintf(&out, "\t%s %q\n", aliases[path], path)
	}
	out.WriteString(")\n\n")
	qualifiedFact := qualifiedType(factType, packageName, aliases)
	fmt.Fprintf(&out, "// %s is %s's dense Factor coordinate: the position a key of this\n", CoordinateType, source.Axis)
	out.WriteString("// axis occupies in the Factor its owner binds. It is published here rather\n")
	out.WriteString("// than hand-exported by an owner, so one axis has exactly one coordinate\n")
	out.WriteString("// type and a family of another axis names this one instead of erasing it to\n")
	out.WriteString("// a builtin width. It carries no capability: it is an index, and every value\n")
	out.WriteString("// of it an owner hands out is one that owner minted.\n")
	fmt.Fprintf(&out, "type %s %s\n\n", CoordinateType, metadata.Key.Dense.Name)
	fmt.Fprintf(&out, "// ForeignRead seals one exact read of a bound %s Factor at this axis's own\n", source.Axis)
	out.WriteString("// coordinate and fact types. It is the read handle a rule family of another\n")
	out.WriteString("// axis holds: that family may not name this pair, and this handle is why it\n")
	out.WriteString("// never has to erase one to reach the read. The coordinate is the one the\n")
	out.WriteString("// reading rule's own selection derived, and a handle bound at any other\n")
	out.WriteString("// pair of types is refused rather than reinterpreted.\n")
	fmt.Fprintf(&out, "func ForeignRead(foreign execution.ForeignFactor, coordinate execution.SelectedCoordinate, input uint16) (execution.ExactRead[%s, %s], bool) {\n", CoordinateType, qualifiedFact)
	fmt.Fprintf(&out, "\treturn execution.ForeignExactRead[%s, %s](foreign, coordinate.Unit, input)\n", CoordinateType, qualifiedFact)
	out.WriteString("}\n\n")
	fmt.Fprintf(&out, "// ForeignSelectedMember resolves one dense coordinate of a bound %s Factor\n", source.Axis)
	out.WriteString("// into one member of a dependent join, at this axis's own coordinate and fact\n")
	out.WriteString("// types. It is the selection sibling of the read handle: a family of another\n")
	out.WriteString("// axis enumerates the members it joins without naming this axis's pair, and\n")
	out.WriteString("// resolves no destination, because a rule publishes into the Factor it writes\n")
	out.WriteString("// and never into one it merely joins.\n")
	fmt.Fprintf(&out, "func ForeignSelectedMember(foreign execution.ForeignFactor, dense uint32, tag uint64) (execution.RouteMember, bool) {\n")
	fmt.Fprintf(&out, "\treturn execution.ForeignSelectedMember[%s, %s](foreign, dense, tag)\n", CoordinateType, qualifiedFact)
	out.WriteString("}\n\n")
	fmt.Fprintf(&out, "// RelationOwner is the generated bind-time owner for %s's member relations.\n", source.Axis)
	out.WriteString("type RelationOwner struct {\n")
	fmt.Fprintf(&out, "\tschema %s\n", ownerFieldType)
	for _, relationIndex := range materializable {
		fmt.Fprintf(&out, "\tsourceColumn%d memberrelation.SourceColumn[%s]\n", relationIndex, qualifiedType(factType, packageName, aliases))
	}
	out.WriteString("}\n\n")
	out.WriteString("var _ memberrelation.Owner = (*RelationOwner)(nil)\n\n")
	if len(materializable) != 0 {
		fmt.Fprintf(&out, "var _ memberrelation.SourceColumns[%s] = (*RelationOwner)(nil)\n\n", qualifiedType(factType, packageName, aliases))
	}
	if len(global) != 0 {
		out.WriteString("var _ memberrelation.OccurrenceDirectory = (*RelationOwner)(nil)\n\n")
	}
	// The identity surface is claimed exactly where it is declared. An axis
	// that publishes only locals is a complete Owner and grows no method for a
	// capability none of its relations names.
	identityProjections := make([]int, 0, len(source.Projections))
	for index, projection := range source.Projections {
		if projection.Role == member.Identity {
			identityProjections = append(identityProjections, index)
		}
	}
	if len(identityProjections) != 0 {
		out.WriteString("var _ memberrelation.IdentityProjection = (*RelationOwner)(nil)\n\n")
	}
	out.WriteString("// NewRelationOwner binds the generated relation owner to one immutable axis schema.\n")
	out.WriteString("func NewRelationOwner(schema ")
	out.WriteString(ownerFieldType)
	out.WriteString(") *RelationOwner {\n")
	if source.Binding.Key.Normalizer.ReceiverPointer {
		out.WriteString("\tif schema == nil {\n\t\treturn nil\n\t}\n")
	}
	out.WriteString("\towner := &RelationOwner{schema: schema}\n")
	if len(materializable) != 0 {
		out.WriteString("\tif !owner.materializeSourceColumns() {\n\t\treturn nil\n\t}\n")
	}
	out.WriteString("\treturn owner\n")
	out.WriteString("}\n\n")
	out.WriteString("// candidate resolves one occurrence to the owner-issued dense candidate ordinal.\n")
	out.WriteString("// A mounted relation requires the mount that qualifies the occurrence; a global\n")
	out.WriteString("// relation owns the occurrence directory itself and refuses one.\n")
	out.WriteString("func (owner *RelationOwner) candidate(relationOrdinal uint32, mount, occurrence identity.ContentID) (uint32, bool) {\n")
	fmt.Fprintf(&out, "\tif %s || !occurrence.Available() {\n\t\treturn 0, false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer))
	out.WriteString("\tswitch relationOrdinal {\n")
	for index, relation := range source.Relations {
		if optionalSymbol(relation.CandidateResolver) {
			continue
		}
		fmt.Fprintf(&out, "\tcase %d:\n", index)
		arguments := []string{"mount", "occurrence"}
		if !optionalSymbol(relation.CandidateIdentityAt) {
			out.WriteString("\t\tif mount.Available() {\n\t\t\treturn 0, false\n\t\t}\n")
			arguments = []string{"occurrence"}
		} else {
			out.WriteString("\t\tif !mount.Available() {\n\t\t\treturn 0, false\n\t\t}\n")
		}
		resolver := directCall(relation.CandidateResolver, owner, "owner.schema", "candidate", arguments, packageName, aliases)
		fmt.Fprintf(&out, "\t\tcandidate, candidateOK := %s\n", resolver)
		out.WriteString("\t\tif !candidateOK {\n\t\t\treturn 0, false\n\t\t}\n")
		ordinal := directCall(relation.CandidateOrdinal, owner, "owner.schema", "candidate", []string{"candidate"}, packageName, aliases)
		fmt.Fprintf(&out, "\t\treturn %s\n", ordinal)
	}
	out.WriteString("\tdefault:\n\t\treturn 0, false\n\t}\n")
	out.WriteString("}\n\n")

	out.WriteString("// CandidateCount is the census of the candidate set one occurrence carries.\n")
	out.WriteString("// Every relation this axis declares is keyed: one occurrence names one row, so\n")
	out.WriteString("// the census is one wherever the keyed resolution succeeds and no row at all\n")
	out.WriteString("// where it does not.\n")
	out.WriteString("func (owner *RelationOwner) CandidateCount(relationOrdinal uint32, mount, occurrence identity.ContentID) (int, bool) {\n")
	out.WriteString("\tif _, ok := owner.candidate(relationOrdinal, mount, occurrence); !ok {\n\t\treturn 0, false\n\t}\n")
	out.WriteString("\treturn 1, true\n}\n\n")

	out.WriteString("// CandidateAt indexes that set. A keyed relation admits index zero only.\n")
	out.WriteString("func (owner *RelationOwner) CandidateAt(relationOrdinal uint32, mount, occurrence identity.ContentID, index int) (uint32, bool) {\n")
	out.WriteString("\tif index != 0 {\n\t\treturn 0, false\n\t}\n")
	out.WriteString("\treturn owner.candidate(relationOrdinal, mount, occurrence)\n}\n\n")

	memberSets := make([]int, 0, len(source.Relations))
	for index, relation := range source.Relations {
		if relation.MemberParent.Available() {
			memberSets = append(memberSets, index)
		}
	}
	out.WriteString("// MemberCount is the census of one nested ordered member set under one parent\n")
	out.WriteString("// row. It is the width of the denominator a vector read over this relation\n")
	out.WriteString("// spans; a relation that declares no member set holds none.\n")
	out.WriteString("func (owner *RelationOwner) MemberCount(relationOrdinal, parentCandidateOrdinal uint32) (int, bool) {\n")
	if len(memberSets) == 0 {
		out.WriteString("\treturn 0, false\n}\n\n")
	} else {
		fmt.Fprintf(&out, "\tif %s {\n\t\treturn 0, false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer))
		out.WriteString("\tswitch relationOrdinal {\n")
		for _, index := range memberSets {
			relation := source.Relations[index]
			parent, parentOK := relationByKey(source, relation.MemberParent.Member)
			if !parentOK {
				return nil, fmt.Errorf("member generator: relation %s names an undeclared member parent", relation.Name)
			}
			fmt.Fprintf(&out, "\tcase %d:\n", index)
			parentAt := directCall(parent.CandidateAt, owner, "owner.schema", "parent", []string{"int(parentCandidateOrdinal)"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\tparent, parentOK := %s\n", parentAt)
			out.WriteString("\t\tif !parentOK {\n\t\t\treturn 0, false\n\t\t}\n")
			count := directCall(relation.MemberCount, owner, "owner.schema", "parent", nil, packageName, aliases)
			fmt.Fprintf(&out, "\t\tcount := %s\n", count)
			out.WriteString("\t\tif count < 0 {\n\t\t\treturn 0, false\n\t\t}\n\t\treturn count, true\n")
		}
		out.WriteString("\tdefault:\n\t\treturn 0, false\n\t}\n}\n\n")
	}

	out.WriteString("// MemberAt addresses one row of a nested ordered member set by its ordinal.\n")
	out.WriteString("// The row it answers is a row of THIS relation, densified through this\n")
	out.WriteString("// relation's own directory, so a member is projected the way every other row\n")
	out.WriteString("// of it is and members need no projection language of their own.\n")
	out.WriteString("func (owner *RelationOwner) MemberAt(relationOrdinal, parentCandidateOrdinal uint32, ordinal int) (uint32, bool) {\n")
	if len(memberSets) == 0 {
		out.WriteString("\treturn 0, false\n}\n\n")
	} else {
		fmt.Fprintf(&out, "\tif %s || ordinal < 0 {\n\t\treturn 0, false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer))
		out.WriteString("\tswitch relationOrdinal {\n")
		for _, index := range memberSets {
			relation := source.Relations[index]
			parent, _ := relationByKey(source, relation.MemberParent.Member)
			fmt.Fprintf(&out, "\tcase %d:\n", index)
			parentAt := directCall(parent.CandidateAt, owner, "owner.schema", "parent", []string{"int(parentCandidateOrdinal)"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\tparent, parentOK := %s\n", parentAt)
			out.WriteString("\t\tif !parentOK {\n\t\t\treturn 0, false\n\t\t}\n")
			memberAt := directCall(relation.MemberAt, owner, "owner.schema", "parent", []string{"ordinal"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\tmember, memberOK := %s\n", memberAt)
			out.WriteString("\t\tif !memberOK {\n\t\t\treturn 0, false\n\t\t}\n")
			memberOrdinal := directCall(relation.CandidateOrdinal, owner, "owner.schema", "member", []string{"member"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\treturn %s\n", memberOrdinal)
		}
		out.WriteString("\tdefault:\n\t\treturn 0, false\n\t}\n}\n\n")
	}

	keyVectors := make([]int, 0, len(source.Relations))
	for index, relation := range source.Relations {
		if relation.KeyVectorCount.Available() && relation.KeyVectorAt.Available() {
			keyVectors = append(keyVectors, index)
		}
	}
	out.WriteString("// KeyVectorCount is the span one row of this directory publishes: the number\n")
	out.WriteString("// of coordinates of another axis that row was constructed from. It is the\n")
	out.WriteString("// width of the denominator a vector read over those coordinates spans, and a\n")
	out.WriteString("// relation whose rows publish no such vector holds none.\n")
	out.WriteString("func (owner *RelationOwner) KeyVectorCount(relationOrdinal, candidateOrdinal uint32) (int, bool) {\n")
	if len(keyVectors) == 0 {
		out.WriteString("\treturn 0, false\n}\n\n")
	} else {
		fmt.Fprintf(&out, "\tif %s {\n\t\treturn 0, false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer))
		out.WriteString("\tswitch relationOrdinal {\n")
		for _, index := range keyVectors {
			relation := source.Relations[index]
			fmt.Fprintf(&out, "\tcase %d:\n", index)
			rowAt := directCall(relation.CandidateAt, owner, "owner.schema", "row", []string{"int(candidateOrdinal)"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\trow, rowOK := %s\n", rowAt)
			out.WriteString("\t\tif !rowOK {\n\t\t\treturn 0, false\n\t\t}\n")
			count := directCall(relation.KeyVectorCount, owner, "owner.schema", "row", nil, packageName, aliases)
			fmt.Fprintf(&out, "\t\tcount := %s\n", count)
			out.WriteString("\t\tif count < 0 {\n\t\t\treturn 0, false\n\t\t}\n\t\treturn count, true\n")
		}
		out.WriteString("\tdefault:\n\t\treturn 0, false\n\t}\n}\n\n")
	}

	out.WriteString("// KeyVectorAt is one coordinate of that vector, at the ordinal the row holds\n")
	out.WriteString("// it at. The coordinate is dense in the axis the vector spans, which is the\n")
	out.WriteString("// axis that issued it; this owner passes it through and normalizes nothing.\n")
	out.WriteString("func (owner *RelationOwner) KeyVectorAt(relationOrdinal, candidateOrdinal uint32, ordinal int) (uint32, bool) {\n")
	if len(keyVectors) == 0 {
		out.WriteString("\treturn 0, false\n}\n\n")
	} else {
		fmt.Fprintf(&out, "\tif %s || ordinal < 0 {\n\t\treturn 0, false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer))
		out.WriteString("\tswitch relationOrdinal {\n")
		for _, index := range keyVectors {
			relation := source.Relations[index]
			fmt.Fprintf(&out, "\tcase %d:\n", index)
			rowAt := directCall(relation.CandidateAt, owner, "owner.schema", "row", []string{"int(candidateOrdinal)"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\trow, rowOK := %s\n", rowAt)
			out.WriteString("\t\tif !rowOK {\n\t\t\treturn 0, false\n\t\t}\n")
			keyAt := directCall(relation.KeyVectorAt, owner, "owner.schema", "row", []string{"ordinal"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\treturn %s\n", keyAt)
		}
		out.WriteString("\tdefault:\n\t\treturn 0, false\n\t}\n}\n\n")
	}

	if len(global) != 0 {
		out.WriteString("// OccurrenceCount is the sealed census of one global relation's occurrence\n")
		out.WriteString("// directory. A mounted relation has no directory of its own and is refused.\n")
		out.WriteString("func (owner *RelationOwner) OccurrenceCount(relationOrdinal uint32) (int, bool) {\n")
		fmt.Fprintf(&out, "\tif %s {\n\t\treturn 0, false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer))
		out.WriteString("\tswitch relationOrdinal {\n")
		for _, relationIndex := range global {
			relation := source.Relations[relationIndex]
			count := directCall(relation.CandidateCount, owner, "owner.schema", "candidate", nil, packageName, aliases)
			fmt.Fprintf(&out, "\tcase %d:\n\t\tcount := %s\n", relationIndex, count)
			out.WriteString("\t\tif count < 0 {\n\t\t\treturn 0, false\n\t\t}\n\t\treturn count, true\n")
		}
		out.WriteString("\tdefault:\n\t\treturn 0, false\n\t}\n}\n\n")

		out.WriteString("// OccurrenceIDAt is the occurrence identity of one dense candidate of a\n")
		out.WriteString("// global relation, in the owner's canonical directory order.\n")
		out.WriteString("func (owner *RelationOwner) OccurrenceIDAt(relationOrdinal uint32, index int) (identity.ContentID, bool) {\n")
		fmt.Fprintf(&out, "\tif %s || index < 0 {\n\t\treturn identity.ContentID{}, false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer))
		out.WriteString("\tswitch relationOrdinal {\n")
		for _, relationIndex := range global {
			relation := source.Relations[relationIndex]
			identityAt := directCall(relation.CandidateIdentityAt, owner, "owner.schema", "candidate", []string{"index"}, packageName, aliases)
			fmt.Fprintf(&out, "\tcase %d:\n\t\treturn %s\n", relationIndex, identityAt)
		}
		out.WriteString("\tdefault:\n\t\treturn identity.ContentID{}, false\n\t}\n}\n\n")
	}

	out.WriteString("// Project projects one dense candidate through one relation/projection pair to a local coordinate ordinal.\n")
	out.WriteString("func (owner *RelationOwner) Project(relationOrdinal, projectionOrdinal, candidateOrdinal uint32) (uint32, bool) {\n")
	fmt.Fprintf(&out, "\tif %s {\n\t\treturn 0, false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer))
	out.WriteString("\tswitch relationOrdinal {\n")
	for relationIndex, relation := range source.Relations {
		fmt.Fprintf(&out, "\tcase %d:\n", relationIndex)
		out.WriteString("\t\tswitch projectionOrdinal {\n")
		for projectionIndex, projection := range source.Projections {
			if projection.Relation != relation.Name {
				continue
			}
			// An identity column has no local. Project answers the address of a
			// row this analyzer minted, and reducing a digest to one would be a
			// truncation rather than a projection, so the identity surface
			// below is the only arm an identity row gets.
			if projection.Role == member.Identity {
				continue
			}
			projectionBinding := metadata.Projections[projectionIndex]
			// A foreign provider is consumed by composition-generated code,
			// which already holds its typed candidate row. This owner-local
			// bind artifact must not grow a second owner field or a CandidateAt
			// mirror. Likewise, a dependent relation whose projection receiver
			// is its derived row cannot be projected from a provider ordinal;
			// leave that direct call to the composition emitter.
			if !projectionBinding.CandidateProviderLocal {
				continue
			}
			providerRelation := source.Relations[projectionBinding.CandidateRelation]
			providerSubject, providerSubjectOK := carrierByName(source, providerRelation.Subject)
			if !providerSubjectOK || !sameGoType(projection.Accessor.Receiver, providerSubject.Type) {
				continue
			}
			candidateRelation := source.Relations[projectionBinding.CandidateRelation]
			fmt.Fprintf(&out, "\t\tcase %d:\n", projectionIndex)
			candidateAt := directCall(candidateRelation.CandidateAt, owner, "owner.schema", "candidate", []string{"int(candidateOrdinal)"}, packageName, aliases)
			accessor := directCall(projection.Accessor, owner, "owner.schema", "candidate", nil, packageName, aliases)
			renderProjectedRow(&out, projection.Accessor, candidateAt, accessor, "return 0, false")
			// A projection whose result is this axis's KEY is a coordinate and
			// becomes a local through the axis's own key normalizer. A
			// projection whose result is anything else - a selection tag, a
			// member ordinal - is already the local it publishes: normalizing
			// it would reinterpret an owner-issued scalar as a coordinate of a
			// directory it was never an index into.
			if projection.Result == source.Binding.Key.Carrier {
				normalizer := directCall(source.Binding.Key.Normalizer, owner, "owner.schema", "projected", []string{"projected"}, packageName, aliases)
				fmt.Fprintf(&out, "\t\t\treturn %s\n", normalizer)
				continue
			}
			scalar, scalarOK := carrierByName(source, projection.Result)
			if !scalarOK || !unsignedLocalType(scalar.Type) {
				return nil, fmt.Errorf("member generator: projection %s publishes neither this axis's key nor an unsigned local", projection.Name)
			}
			out.WriteString("\t\t\tlocal := uint64(projected)\n")
			out.WriteString("\t\t\tif local > uint64(^uint32(0)) {\n\t\t\t\treturn 0, false\n\t\t\t}\n")
			out.WriteString("\t\t\treturn uint32(local), true\n")
		}
		out.WriteString("\t\tdefault:\n\t\t\treturn 0, false\n\t\t}\n")
	}
	out.WriteString("\tdefault:\n\t\treturn 0, false\n\t}\n")
	out.WriteString("}\n\n")

	if len(identityProjections) != 0 {
		if err := renderIdentityProjection(&out, source, metadata, identityProjections, owner, packageName, aliases); err != nil {
			return nil, err
		}
	}

	if len(materializable) != 0 {
		out.WriteString("// materializeSourceColumns seals each owner-issued source fact column once.\n")
		out.WriteString("func (owner *RelationOwner) materializeSourceColumns() bool {\n")
		fmt.Fprintf(&out, "\tif %s {\n\t\treturn false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer))
		for _, relationIndex := range materializable {
			relation := source.Relations[relationIndex]
			count := directCall(relation.CandidateCount, owner, "owner.schema", "candidate", nil, packageName, aliases)
			fmt.Fprintf(&out, "\tcount%d := %s\n", relationIndex, count)
			fmt.Fprintf(&out, "\tif count%d < 0 {\n\t\treturn false\n\t}\n", relationIndex)
			fmt.Fprintf(&out, "\tfacts%d := make([]%s, count%d)\n", relationIndex, qualifiedType(factType, packageName, aliases), relationIndex)
			fmt.Fprintf(&out, "\toutcomes%d := make([]%s.%s, count%d)\n", relationIndex, aliases[OutcomePackagePath], OutcomeType, relationIndex)
			fmt.Fprintf(&out, "\tfor index := 0; index < count%d; index++ {\n", relationIndex)
			candidateAt := directCall(relation.CandidateAt, owner, "owner.schema", "candidate", []string{"index"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\tcandidate, candidateOK := %s\n", candidateAt)
			out.WriteString("\t\tif !candidateOK {\n\t\t\treturn false\n\t\t}\n")
			materialize := directCall(relation.Materialize, owner, "owner.schema", "candidate", []string{"candidate"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\tfact, outcome := %s\n", materialize)
			fmt.Fprintf(&out, "\t\tif outcome == %s.%s {\n\t\t\treturn false\n\t\t}\n", aliases[OutcomePackagePath], OutcomeRefuse)
			fmt.Fprintf(&out, "\t\tfacts%d[index] = fact\n", relationIndex)
			fmt.Fprintf(&out, "\t\toutcomes%d[index] = outcome\n", relationIndex)
			out.WriteString("\t}\n")
			fmt.Fprintf(&out, "\tcolumn%d, column%dOK := memberrelation.NewSourceColumn(facts%d, outcomes%d)\n", relationIndex, relationIndex, relationIndex, relationIndex)
			fmt.Fprintf(&out, "\tif !column%dOK {\n\t\treturn false\n\t}\n", relationIndex)
			fmt.Fprintf(&out, "\towner.sourceColumn%d = column%d\n", relationIndex, relationIndex)
		}
		out.WriteString("\treturn true\n}\n\n")
		out.WriteString("// SourceFactColumn returns the immutable typed source fact column for one relation.\n")
		out.WriteString("// RelationCount is the sealed relation-ordinal extent. It preserves absent\n")
		out.WriteString("// materializations separately from a valid empty source column.\n")
		fmt.Fprintf(&out, "func (*RelationOwner) RelationCount() int { return %d }\n\n", len(source.Relations))
		fmt.Fprintf(&out, "func (owner *RelationOwner) SourceFactColumn(relationOrdinal uint32) (memberrelation.SourceColumn[%s], bool) {\n", qualifiedType(factType, packageName, aliases))
		fmt.Fprintf(&out, "\tif %s {\n\t\treturn memberrelation.SourceColumn[%s]{}, false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer), qualifiedType(factType, packageName, aliases))
		out.WriteString("\tswitch relationOrdinal {\n")
		for _, relationIndex := range materializable {
			fmt.Fprintf(&out, "\tcase %d:\n\t\treturn owner.sourceColumn%d, true\n", relationIndex, relationIndex)
		}
		out.WriteString("\tdefault:\n\t\treturn memberrelation.SourceColumn[" + qualifiedType(factType, packageName, aliases) + "]{}, false\n\t}\n}\n")
	}

	return format.Source([]byte(out.String()))
}

// packageAliases is intentionally a tiny generation-only helper. Generated
// production code receives concrete imports and never retains this map.
type packageAliases map[string]string

func (aliases packageAliases) paths() []string {
	paths := make([]string, 0, len(aliases))
	for path := range aliases {
		paths = append(paths, path)
	}
	slicesSortStrings(paths)
	return paths
}

func relationImportAliases(packageName string, source definition.Definition, metadata Metadata) packageAliases {
	aliases := make(packageAliases)
	add := func(path string) {
		if path == "" || packagePathPackage(path) == packageName || path == "github.com/wippyai/go-lua/analysis/identity" || path == "github.com/wippyai/go-lua/analysis/schema/axis/member" {
			return
		}
		if _, exists := aliases[path]; exists {
			return
		}
		base := packagePathPackage(path)
		alias := base
		for ordinal := 2; ; ordinal++ {
			used := false
			for _, existing := range aliases {
				if existing == alias {
					used = true
					break
				}
			}
			if !used {
				break
			}
			alias = fmt.Sprintf("%s%d", base, ordinal)
		}
		aliases[path] = alias
	}
	add(source.Binding.Key.Normalizer.PackagePath)
	for _, relation := range source.Relations {
		add(relation.CandidateResolver.PackagePath)
		add(relation.CandidateOrdinal.PackagePath)
		add(relation.CandidateAt.PackagePath)
		add(relation.CandidateCount.PackagePath)
		add(relation.Materialize.PackagePath)
		add(relation.CandidateIdentityAt.PackagePath)
		if !optionalSymbol(relation.Materialize) {
			add(OutcomePackagePath)
		}
	}
	for index, projection := range source.Projections {
		if projectionOwnerLocal(source, metadata, index) {
			add(projection.Accessor.PackagePath)
		}
	}
	_ = metadata
	return aliases
}

func projectionOwnerLocal(source definition.Definition, metadata Metadata, index int) bool {
	if index < 0 || index >= len(source.Projections) || index >= len(metadata.Projections) {
		return false
	}
	binding := metadata.Projections[index]
	if !binding.CandidateProviderLocal || int(binding.CandidateRelation) >= len(source.Relations) {
		return false
	}
	provider := source.Relations[binding.CandidateRelation]
	providerSubject, ok := carrierByName(source, provider.Subject)
	return ok && sameGoType(source.Projections[index].Accessor.Receiver, providerSubject.Type)
}

func packagePathPackage(path string) string {
	for index := len(path) - 1; index >= 0; index-- {
		if path[index] == '/' {
			return path[index+1:]
		}
	}
	return path
}

func qualifiedType(typ definition.GoType, packageName string, aliases packageAliases) string {
	if typ.PackagePath == "" || packagePathPackage(typ.PackagePath) == packageName {
		return typ.Name
	}
	if alias, ok := aliases[typ.PackagePath]; ok {
		return alias + "." + typ.Name
	}
	return typ.Name
}

func exactFoldPayloadField(ordinal uint32) string {
	return fmt.Sprintf("candidate%d", ordinal)
}

func directCall(symbol definition.GoSymbol, owner definition.GoType, ownerExpr, candidateExpr string, args []string, packageName string, aliases packageAliases) string {
	receiver := ""
	if symbol.Receiver.Name != "" {
		if sameGoType(symbol.Receiver, owner) {
			receiver = ownerExpr
		} else {
			receiver = candidateExpr
		}
	}
	if receiver == "" {
		prefix := ""
		if symbol.PackagePath != "" && packagePathPackage(symbol.PackagePath) != packageName {
			prefix = aliases[symbol.PackagePath] + "."
		}
		return prefix + symbol.Name + "(" + strings.Join(args, ", ") + ")"
	}
	return receiver + "." + symbol.Name + "(" + strings.Join(args, ", ") + ")"
}

func sameGoType(left, right definition.GoType) bool {
	return left.PackagePath == right.PackagePath && left.Name == right.Name
}

func slicesSortStrings(values []string) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && values[cursor] < values[cursor-1]; cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}

func entryReferenceExpression(reference schema.EntryReference) string {
	return fmt.Sprintf("schemaapi.EntryReference{Surface: schemaapi.SurfaceKindAxis, Key: %q}", reference.Key)
}

func coldEntryReferenceExpression(reference schema.EntryReference, axis schema.Key) string {
	if reference.Surface == schema.SurfaceKindAxis && reference.Key == axis {
		return "valueAxis"
	}
	return entryReferenceExpression(reference)
}

func ownerSchemaMissing(pointer bool) string {
	if pointer {
		return "owner == nil || owner.schema == nil"
	}
	return "owner == nil"
}

// candidateProviderExpression emits whichever arm the provider states. The
// axis arm emits the exact relation literal it emitted before the choice
// existed, wrapped in the constructor that names the arm.
func candidateProviderExpression(provider member.CandidateRef) string {
	if provider.Issued() {
		return fmt.Sprintf("member.IssuedRowCandidate(%q)", provider.IssuedRow)
	}
	return fmt.Sprintf("member.AxisRelationCandidate(%s)", relationProviderExpression(provider.AxisRelation))
}

func relationProviderExpression(reference member.RelationRef) string {
	return fmt.Sprintf("member.RelationRef{Axis: schemaapi.EntryReference{Surface: schemaapi.SurfaceKind(%d), Key: %q}, Member: %q}", reference.Axis.Surface, reference.Axis.Key, reference.Member)
}

func carrierByName(source definition.Definition, name string) (definition.Carrier, bool) {
	for _, carrier := range source.Carriers {
		if carrier.Name == name {
			return carrier, true
		}
	}
	return definition.Carrier{}, false
}

// renderProjectedRow emits the head every projection arm shares: fetch the
// candidate row this ordinal addresses, apply the declared accessor, and bind
// the answer to `projected`.
//
// The refusal is the caller's. The two projection surfaces answer different
// shapes - a local and an identity - and everything up to the answer is the
// same read, so the read is written once and each surface spells its own
// absence.
func renderProjectedRow(out *strings.Builder, accessor definition.GoSymbol, candidateAt, accessorCall, refusal string) {
	fmt.Fprintf(out, "\t\t\tcandidate, candidateOK := %s\n", candidateAt)
	fmt.Fprintf(out, "\t\t\tif !candidateOK {\n\t\t\t\t%s\n\t\t\t}\n", refusal)
	// A sole-result accessor publishes exactly this projection and no value
	// beside it, so the call binds one name. Binding a second would force every
	// owner to publish a fact it may not have, which is how a paired accessor
	// gets written around an unpaired one.
	projected, discarded := "first", ""
	results := "first, second, projectionOK := "
	switch accessor.ResultIndex {
	case -1:
		results = "first, projectionOK := "
	case 1:
		projected, discarded = "second", "first"
	default:
		discarded = "second"
	}
	fmt.Fprintf(out, "\t\t\t%s%s\n", results, accessorCall)
	fmt.Fprintf(out, "\t\t\tif !projectionOK {\n\t\t\t\t%s\n\t\t\t}\n", refusal)
	if discarded != "" {
		fmt.Fprintf(out, "\t\t\t_ = %s\n", discarded)
	}
	fmt.Fprintf(out, "\t\t\tprojected := %s\n", projected)
}

func roleExpression(role member.Role) (string, bool) {
	switch role {
	case member.Key:
		return "member.Key", true
	case member.Predicate:
		return "member.Predicate", true
	case member.Destination:
		return "member.Destination", true
	case member.Identity:
		return "member.Identity", true
	default:
		return "", false
	}
}

func formExpression(form member.ReadForm) (string, bool) {
	switch form {
	case member.ReadFormExact:
		return "member.ReadFormExact", true
	case member.ReadFormSelected:
		return "member.ReadFormSelected", true
	case member.ReadFormSummary:
		return "member.ReadFormSummary", true
	case member.ReadFormComplete:
		return "member.ReadFormComplete", true
	default:
		return "", false
	}
}

func multiplicityExpression(multiplicity member.Multiplicity) (string, bool) {
	switch multiplicity {
	case member.MultiplicityOptional:
		return "member.MultiplicityOptional", true
	case member.MultiplicityOne:
		return "member.MultiplicityOne", true
	case member.MultiplicityMany:
		return "member.MultiplicityMany", true
	default:
		return "", false
	}
}

// unsignedLocalType reports whether a carrier is spelled as an unsigned
// builtin, which is what a local that is not a coordinate has to be: the
// Owner surface publishes locals as uint32, and a value that cannot widen to
// uint64 and back is not one.
func unsignedLocalType(typ definition.GoType) bool {
	if typ.PackagePath != "" || typ.Pointer {
		return false
	}
	switch typ.Name {
	case "uint", "uint8", "uint16", "uint32", "uint64", "byte":
		return true
	default:
		return false
	}
}

// renderIdentityProjection emits the owner's identity surface: one arm per
// column declared in the Identity role, answering the digest and the frame it
// was issued under.
//
// The frame is the owner's own and is never a constant this emitter invents. A
// content identity is issued under no frame and answers zero; a semantic key
// already carries the frame its owner minted it at, so the arm reads it off
// the value. Which of the two a column is is the declared carrier's statement,
// not a shape probed from the accessor.
func renderIdentityProjection(out *strings.Builder, source definition.Definition, metadata Metadata, identityProjections []int, owner definition.GoType, packageName string, aliases packageAliases) error {
	byRelation := make(map[int][]int, len(identityProjections))
	order := make([]int, 0, len(source.Relations))
	for _, projectionIndex := range identityProjections {
		projection := source.Projections[projectionIndex]
		binding := metadata.Projections[projectionIndex]
		// A foreign provider is consumed by composition-generated code, which
		// already holds its typed candidate row, on the same terms the local
		// projection states above.
		if !binding.CandidateProviderLocal {
			continue
		}
		relationIndex := -1
		for index, relation := range source.Relations {
			if relation.Name == projection.Relation {
				relationIndex = index
				break
			}
		}
		if relationIndex < 0 {
			return fmt.Errorf("member generator: identity projection %s names an undeclared relation", projection.Name)
		}
		providerRelation := source.Relations[binding.CandidateRelation]
		providerSubject, providerSubjectOK := carrierByName(source, providerRelation.Subject)
		if !providerSubjectOK || !sameGoType(projection.Accessor.Receiver, providerSubject.Type) {
			continue
		}
		if _, held := byRelation[relationIndex]; !held {
			order = append(order, relationIndex)
		}
		byRelation[relationIndex] = append(byRelation[relationIndex], projectionIndex)
	}

	const refusal = "return identity.ContentID{}, 0, false"
	out.WriteString("// ProjectIdentity answers one candidate row's owner-issued identity: the\n")
	out.WriteString("// canonical digest, and the frame it was issued under. A content identity is\n")
	out.WriteString("// issued under no frame and answers zero; a semantic axis answers the frame\n")
	out.WriteString("// its own owner minted it at, which is what reconstitutes the key.\n")
	out.WriteString("func (owner *RelationOwner) ProjectIdentity(relationOrdinal, projectionOrdinal, candidateOrdinal uint32) (identity.ContentID, uint64, bool) {\n")
	fmt.Fprintf(out, "\tif %s {\n\t\t%s\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer), refusal)
	out.WriteString("\tswitch relationOrdinal {\n")
	for _, relationIndex := range order {
		fmt.Fprintf(out, "\tcase %d:\n", relationIndex)
		out.WriteString("\t\tswitch projectionOrdinal {\n")
		for _, projectionIndex := range byRelation[relationIndex] {
			projection := source.Projections[projectionIndex]
			binding := metadata.Projections[projectionIndex]
			candidateRelation := source.Relations[binding.CandidateRelation]
			result, resultOK := carrierByName(source, projection.Result)
			framed, issued := definition.IdentityCarrier(result.Type)
			if !resultOK || !issued {
				return fmt.Errorf("member generator: identity projection %s publishes no owner-issued identity", projection.Name)
			}
			fmt.Fprintf(out, "\t\tcase %d:\n", projectionIndex)
			candidateAt := directCall(candidateRelation.CandidateAt, owner, "owner.schema", "candidate", []string{"int(candidateOrdinal)"}, packageName, aliases)
			accessor := directCall(projection.Accessor, owner, "owner.schema", "candidate", nil, packageName, aliases)
			renderProjectedRow(out, projection.Accessor, candidateAt, accessor, refusal)
			if framed {
				out.WriteString("\t\t\treturn identity.ContentID(projected.Digest()), projected.Version(), true\n")
				continue
			}
			out.WriteString("\t\t\treturn projected, 0, true\n")
		}
		fmt.Fprintf(out, "\t\tdefault:\n\t\t\t%s\n\t\t}\n", refusal)
	}
	fmt.Fprintf(out, "\tdefault:\n\t\t%s\n\t}\n", refusal)
	out.WriteString("}\n\n")
	return nil
}

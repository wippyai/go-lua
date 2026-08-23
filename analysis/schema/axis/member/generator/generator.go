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
)

// Artifact is the pair of generated owner artifacts. Cold is the declaration
// catalog; Relations is the immutable bind-time relation owner. Typed
// metadata is returned only through Resolve and is never retained by runtime
// code.
type Artifact struct {
	Cold      []byte
	Relations []byte
}

// KeyBinding is the resolved axis-level key normalization needed by a future
// composition generator.
type KeyBinding struct {
	Carrier    member.Carrier
	Input      definition.GoType
	Dense      definition.GoType
	Normalizer definition.GoSymbol
}

// RelationBinding is typed metadata for exactly one relation declaration.
type RelationBinding struct {
	Key               schema.Key
	Subject           definition.GoType
	Inputs            []definition.GoType
	CandidateProvider member.RelationRef
	CandidateResolver definition.GoSymbol
	CandidateOrdinal  definition.GoSymbol
	CandidateAt       definition.GoSymbol
	CandidateCount    definition.GoSymbol
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
	CandidateProvider      member.RelationRef
	CandidateRelation      uint32
	CandidateProviderLocal bool
}

// ReducerInputBinding is typed metadata for one reducer input row.
type ReducerInputBinding struct {
	Axis         schema.EntryReference
	Type         definition.GoType
	Form         member.ReadForm
	Multiplicity member.Multiplicity
	Tag          definition.GoType
}

// ReducerOutputBinding is typed metadata for one reducer output row.
type ReducerOutputBinding struct {
	Axis schema.EntryReference
	Type definition.GoType
}

// ReducerBinding is typed metadata for exactly one reducer declaration.
type ReducerBinding struct {
	Key schema.Key
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
		for inputIndex, inputName := range relation.Inputs {
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
			Derivation: RelationDerivationBinding{
				State: relation.Derivation.State, Build: relation.Derivation.Build,
				Count: relation.Derivation.Count, At: relation.Derivation.At,
				StaticAxes: append([]schema.EntryReference(nil), relation.Derivation.StaticAxes...),
			},
		}
	}
	owner := source.Binding.Key.Normalizer.Receiver
	for index, relation := range source.Relations {
		providerOrdinal, providerLocal := relationsByKey[relation.CandidateProvider.Member]
		if relation.CandidateProvider.Axis.Key == source.Axis {
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
				for _, inputName := range relation.Inputs {
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
		if projection.Accessor.ResultIndex != 0 && projection.Accessor.ResultIndex != 1 {
			return Metadata{}, fmt.Errorf("member generator: projection %s must select accessor result 0 or 1", projection.Name)
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
			inputs[inputIndex] = row
		}
		outputs := make([]ReducerOutputBinding, len(reducer.Outputs))
		for outputIndex, output := range reducer.Outputs {
			outputs[outputIndex] = ReducerOutputBinding{Axis: output.Axis, Type: carriers[output.Carrier].Type}
		}
		reducers[index] = ReducerBinding{
			Key: reducer.Key, Candidate: candidateType, CandidatePresent: candidatePresent, CandidateConstant: !candidatePresent,
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
	return Metadata{
		Axis:            source.Axis,
		FactCarrier:     member.Carrier(carriers[source.Signature.Fact].Key),
		FactType:        carriers[source.Signature.Fact].Type,
		Key:             KeyBinding{Carrier: keyCarrier.Key, Input: keyCarrier.Type, Dense: source.Binding.Key.Dense, Normalizer: source.Binding.Key.Normalizer},
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
	return Artifact{Cold: cold, Relations: relations}, nil
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
	var out strings.Builder
	fmt.Fprintf(&out, "// Code generated by axis member definition generator; DO NOT EDIT.\n\npackage %s\n\n", packageName)
	fmt.Fprintf(&out, "//go:generate go run ../../analysis/schema/axis/member/generator/cmd -source %s -cold rule_members.go -relations generated_relation_owner.go\n\n", source.Axis)
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
		fmt.Fprintf(&out, "\t\t\t{Key: %s, Subject: %s, CandidateProvider: %s", relations[relation.Name], carriers[relation.Subject], relationProviderExpression(relation.CandidateProvider))
		if len(relation.Inputs) != 0 {
			out.WriteString(", Inputs: []member.Carrier{")
			for index, input := range relation.Inputs {
				if index != 0 {
					out.WriteString(", ")
				}
				out.WriteString(carriers[input])
			}
			out.WriteString("}")
		}
		out.WriteString("},\n")
	}
	out.WriteString("\t\t},\n\t\t[]member.Projection{\n")
	for _, projection := range source.Projections {
		role, ok := roleExpression(projection.Role)
		if !ok {
			return nil, fmt.Errorf("member generator: unsupported projection role %d", projection.Role)
		}
		fmt.Fprintf(&out, "\t\t\t{Key: %s, Relation: %s, Role: %s, Result: %s, CandidateProvider: %s},\n", projection.Name, relations[projection.Relation], role, carriers[projection.Result], relationProviderExpression(projection.CandidateProvider))
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
			out.WriteString("},\n")
		}
		out.WriteString("\t\t\t}, Outputs: []member.ReducerOutput{\n")
		for _, output := range reducer.Outputs {
			fmt.Fprintf(&out, "\t\t\t\t{Axis: %s, Carrier: %s},\n", coldEntryReferenceExpression(output.Axis, source.Axis), carriers[output.Carrier])
		}
		fmt.Fprintf(&out, "\t\t\t}},\n")
	}
	out.WriteString("\t\t},\n\t\t[]member.CarryTransform{\n")
	for _, transform := range source.CarryTransforms {
		fmt.Fprintf(&out, "\t\t\t{Key: %s, Candidate: %s, Input: %s, Output: %s},\n", transform.Name, carriers[transform.Candidate], carriers[transform.Input], carriers[transform.Output])
	}
	fmt.Fprintf(&out, "\t\t},\n\t)\n\tif !ok {\n\t\tpanic(%q)\n\t}\n\treturn catalog\n}\n", packageName+": invalid axis member catalog")
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
	for index, relation := range source.Relations {
		if !optionalSymbol(relation.Materialize) {
			materializable = append(materializable, index)
		}
	}

	var out strings.Builder
	out.WriteString("// Code generated by axis member definition generator; DO NOT EDIT.\n")
	out.WriteString("// This file is the immutable bind-time relation owner for the axis.\n\n")
	fmt.Fprintf(&out, "package %s\n\n", packageName)
	out.WriteString("import (\n")
	out.WriteString("\t\"github.com/wippyai/go-lua/analysis/identity\"\n")
	out.WriteString("\tmemberrelation \"github.com/wippyai/go-lua/analysis/schema/axis/member/relation\"\n")
	for _, path := range aliases.paths() {
		fmt.Fprintf(&out, "\t%s %q\n", aliases[path], path)
	}
	out.WriteString(")\n\n")
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
	out.WriteString("// Candidate resolves one mounted occurrence to the owner-issued dense candidate ordinal.\n")
	out.WriteString("func (owner *RelationOwner) Candidate(relationOrdinal uint32, mount, occurrence identity.ContentID) (uint32, bool) {\n")
	fmt.Fprintf(&out, "\tif %s || !mount.Available() || !occurrence.Available() {\n\t\treturn 0, false\n\t}\n", ownerSchemaMissing(source.Binding.Key.Normalizer.ReceiverPointer))
	out.WriteString("\tswitch relationOrdinal {\n")
	for index, relation := range source.Relations {
		if optionalSymbol(relation.CandidateResolver) {
			continue
		}
		fmt.Fprintf(&out, "\tcase %d:\n", index)
		resolver := directCall(relation.CandidateResolver, owner, "owner.schema", "candidate", []string{"mount", "occurrence"}, packageName, aliases)
		fmt.Fprintf(&out, "\t\tcandidate, candidateOK := %s\n", resolver)
		out.WriteString("\t\tif !candidateOK {\n\t\t\treturn 0, false\n\t\t}\n")
		ordinal := directCall(relation.CandidateOrdinal, owner, "owner.schema", "candidate", []string{"candidate"}, packageName, aliases)
		fmt.Fprintf(&out, "\t\treturn %s\n", ordinal)
	}
	out.WriteString("\tdefault:\n\t\treturn 0, false\n\t}\n")
	out.WriteString("}\n\n")

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
			fmt.Fprintf(&out, "\t\t\tcandidate, candidateOK := %s\n", candidateAt)
			out.WriteString("\t\t\tif !candidateOK {\n\t\t\t\treturn 0, false\n\t\t\t}\n")
			accessor := directCall(projection.Accessor, owner, "owner.schema", "candidate", nil, packageName, aliases)
			out.WriteString("\t\t\tfirst, second, projectionOK := ")
			out.WriteString(accessor)
			out.WriteString("\n\t\t\tif !projectionOK {\n\t\t\t\treturn 0, false\n\t\t\t}\n")
			projected := "first"
			if projection.Accessor.ResultIndex == 1 {
				out.WriteString("\t\t\t_ = first\n")
				projected = "second"
			} else {
				out.WriteString("\t\t\t_ = second\n")
			}
			out.WriteString("\t\t\tprojected := ")
			out.WriteString(projected)
			out.WriteString("\n")
			normalizer := directCall(source.Binding.Key.Normalizer, owner, "owner.schema", "projected", []string{"projected"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\t\treturn %s\n", normalizer)
		}
		out.WriteString("\t\tdefault:\n\t\t\treturn 0, false\n\t\t}\n")
	}
	out.WriteString("\tdefault:\n\t\treturn 0, false\n\t}\n")
	out.WriteString("}\n\n")

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
			fmt.Fprintf(&out, "\tfor index := 0; index < count%d; index++ {\n", relationIndex)
			candidateAt := directCall(relation.CandidateAt, owner, "owner.schema", "candidate", []string{"index"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\tcandidate, candidateOK := %s\n", candidateAt)
			out.WriteString("\t\tif !candidateOK {\n\t\t\treturn false\n\t\t}\n")
			materialize := directCall(relation.Materialize, owner, "owner.schema", "candidate", []string{"candidate"}, packageName, aliases)
			fmt.Fprintf(&out, "\t\tfact, outcome := %s\n", materialize)
			fmt.Fprintf(&out, "\t\tif outcome != %s.%s {\n\t\t\treturn false\n\t\t}\n", aliases[OutcomePackagePath], OutcomeConcrete)
			fmt.Fprintf(&out, "\t\tfacts%d[index] = fact\n", relationIndex)
			out.WriteString("\t}\n")
			fmt.Fprintf(&out, "\towner.sourceColumn%d = memberrelation.NewSourceColumn(facts%d)\n", relationIndex, relationIndex)
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

func roleExpression(role member.Role) (string, bool) {
	switch role {
	case member.Key:
		return "member.Key", true
	case member.Predicate:
		return "member.Predicate", true
	case member.Destination:
		return "member.Destination", true
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

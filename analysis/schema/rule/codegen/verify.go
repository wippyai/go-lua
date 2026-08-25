package codegen

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	membergenerator "github.com/wippyai/go-lua/analysis/schema/axis/member/generator"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// Build verifies a composition-wide owner metadata roster against one sealed
// rule-plan catalog and lowers every present plan to a direct reducer call.
// Each referenced dense axis must have exactly one metadata row in roster;
// relation, projection, read, reducer, and output owners may all differ.
// Build consumes no authored definition and derives no schema or digest.
func Build(roster []membergenerator.Metadata, catalog ruleplan.Catalog) (Model, error) {
	if !catalog.Digest().Available() {
		return Model{}, failure(ProblemDigest, 0, 0, 0, 0, "rule codegen: sealed rule-plan digest is unavailable")
	}
	if !catalog.Available() {
		return Model{}, failure(ProblemCatalog, 0, 0, 0, 0, "rule codegen: sealed rule-plan catalog is unavailable")
	}
	metadataByAxis, axes, rosterOK := resolveRoster(roster, catalog)
	if !rosterOK {
		return Model{}, failure(ProblemAxis, 0, 0, 0, 0, "rule codegen: owner metadata roster is incomplete or duplicated")
	}

	rules := make([]Rule, 0, catalog.Count())
	for position := 0; position < catalog.Count(); position++ {
		compiled, present := catalog.At(position)
		if !present {
			return Model{}, failure(ProblemRule, uint32(position), 0, 0, 0, "rule codegen: rule-plan ordinal is unavailable")
		}
		if !compiled.Present() {
			continue
		}
		ordinal := uint32(position)
		if compiled.Rule() != ordinal {
			return Model{}, failure(ProblemRule, ordinal, 0, 0, 0, "rule codegen: rule-plan ordinal drift")
		}
		row, kind, rowOK := buildRule(metadataByAxis, catalog, ordinal, compiled)
		if !rowOK {
			if kind == ProblemNone {
				kind = ProblemRule
			}
			return Model{}, failure(kind, ordinal, 0, 0, 0, "rule codegen: plan does not match owner metadata")
		}
		rules = append(rules, row)
	}
	return Model{digest: catalog.Digest(), axes: axes, rules: rules}, nil
}

// Verify runs the same bounded checks as Build and discards the model.
func Verify(roster []membergenerator.Metadata, catalog ruleplan.Catalog) error {
	_, err := Build(roster, catalog)
	return err
}

func resolveRoster(roster []membergenerator.Metadata, catalog ruleplan.Catalog) (map[uint32]membergenerator.Metadata, []Axis, bool) {
	if len(roster) == 0 {
		return nil, nil, false
	}
	byAxis := make(map[uint32]membergenerator.Metadata, len(roster))
	for _, metadata := range roster {
		if !metadataComplete(metadata) {
			return nil, nil, false
		}
		axisOrdinal, axisOK := findAxis(catalog, metadata.Axis)
		if !axisOK {
			return nil, nil, false
		}
		if _, duplicate := byAxis[axisOrdinal]; duplicate {
			return nil, nil, false
		}
		byAxis[axisOrdinal] = metadata
	}
	needed := referencedAxes(catalog)
	for axisOrdinal := range needed {
		if _, present := byAxis[axisOrdinal]; !present {
			return nil, nil, false
		}
	}
	axes := make([]Axis, 0, len(byAxis))
	for axisOrdinal, metadata := range byAxis {
		axes = append(axes, Axis{ordinal: axisOrdinal, key: metadata.Axis})
	}
	for index := 1; index < len(axes); index++ {
		for cursor := index; cursor > 0 && axes[cursor].ordinal < axes[cursor-1].ordinal; cursor-- {
			axes[cursor], axes[cursor-1] = axes[cursor-1], axes[cursor]
		}
	}
	return byAxis, axes, true
}

func referencedAxes(catalog ruleplan.Catalog) map[uint32]struct{} {
	needed := make(map[uint32]struct{})
	for position := 0; position < catalog.Count(); position++ {
		compiled, present := catalog.At(position)
		if !present || !compiled.Present() {
			continue
		}
		candidate := compiled.Candidate()
		reducer := compiled.Reducer()
		needed[candidate.Axis] = struct{}{}
		needed[reducer.Axis] = struct{}{}
		for joinPosition := 0; joinPosition < compiled.JoinCount(); joinPosition++ {
			join, joinOK := compiled.JoinAt(joinPosition)
			if !joinOK {
				continue
			}
			needed[join.Relation.Axis] = struct{}{}
			needed[join.Key.Axis] = struct{}{}
			needed[join.ReadAxis] = struct{}{}
			if join.PredicatePresent {
				needed[join.Predicate.Axis] = struct{}{}
			}
		}
		for outputPosition := 0; outputPosition < compiled.OutputCount(); outputPosition++ {
			output, outputOK := compiled.OutputAt(outputPosition)
			if !outputOK {
				continue
			}
			needed[output.Address.Axis] = struct{}{}
			needed[output.Destination.Axis] = struct{}{}
		}
		if carry, carryPresent := compiled.Carry(); carryPresent && carry.TransformPresent {
			needed[carry.Transform.Axis] = struct{}{}
		}
	}
	return needed
}

func metadataComplete(metadata membergenerator.Metadata) bool {
	if !metadata.Axis.Available() || !metadata.FactCarrier.Available() || !metadata.FactType.Available() {
		return false
	}
	if !metadata.Key.Carrier.Available() || !metadata.Key.Input.Available() || !metadata.Key.Dense.Available() || !metadata.Key.Normalizer.Available() {
		return false
	}
	relationKeys := make(map[schema.Key]struct{}, len(metadata.Relations))
	relationsByKey := make(map[schema.Key]membergenerator.RelationBinding, len(metadata.Relations))
	for _, relation := range metadata.Relations {
		if !relation.Key.Available() || !relation.Subject.Available() || !relation.CandidateProvider.Available() {
			return false
		}
		if _, duplicate := relationKeys[relation.Key]; duplicate {
			return false
		}
		relationKeys[relation.Key] = struct{}{}
		relationsByKey[relation.Key] = relation
		for _, input := range relation.Inputs {
			if !input.Available() {
				return false
			}
		}
		derived := relationHasDerivation(relation)
		if derived {
			if !relationDerivationComplete(relation.Derivation) || len(relation.Inputs) == 0 ||
				relation.CandidateProvider.AxisRelation.Axis.Key == metadata.Axis && relation.CandidateProvider.AxisRelation.Member == relation.Key ||
				!symbolAbsent(relation.CandidateResolver) || !symbolAbsent(relation.CandidateOrdinal) || !symbolAbsent(relation.CandidateAt) || !symbolAbsent(relation.CandidateCount) {
				return false
			}
		}
		// An issued provider is neither local nor foreign to an axis: there is
		// no axis directory to be local to. It carries none of the dense
		// symbols and takes no relation ordinal.
		if relation.CandidateProvider.Issued() {
			if relation.CandidateProviderLocal || relation.HasCandidateRelation || relation.CandidateRelation != 0 ||
				!symbolAbsent(relation.CandidateResolver) || !symbolAbsent(relation.CandidateOrdinal) ||
				!symbolAbsent(relation.CandidateAt) || !symbolAbsent(relation.CandidateCount) ||
				!symbolAbsent(relation.CandidateIdentityAt) {
				return false
			}
			continue
		}
		localProvider := relation.CandidateProvider.AxisRelation.Axis.Key == metadata.Axis
		if relation.CandidateProviderLocal != localProvider || relation.HasCandidateRelation != localProvider {
			return false
		}
		if localProvider {
			if uint64(relation.CandidateRelation) >= uint64(len(metadata.Relations)) {
				return false
			}
			provider := metadata.Relations[relation.CandidateRelation]
			if provider.Key != relation.CandidateProvider.AxisRelation.Member || provider.CandidateProvider.AxisRelation.Axis.Key != metadata.Axis || provider.CandidateProvider.AxisRelation.Member != provider.Key ||
				!provider.CandidateProviderLocal || !provider.HasCandidateRelation || provider.CandidateRelation != relation.CandidateRelation || !symbolDirectoryComplete(provider) {
				return false
			}
			if symbolAbsent(relation.CandidateResolver) != symbolAbsent(relation.CandidateOrdinal) || symbolAbsent(relation.CandidateResolver) != symbolAbsent(relation.CandidateAt) {
				return false
			}
			if !symbolAbsent(relation.CandidateResolver) && !symbolDirectoryComplete(relation) {
				return false
			}
		} else if !symbolAbsent(relation.CandidateResolver) || !symbolAbsent(relation.CandidateOrdinal) || !symbolAbsent(relation.CandidateAt) || relation.CandidateRelation != 0 {
			// A consumer never copies a foreign provider's directory or
			// launders its ordinal into a local field.
			return false
		}
	}
	projectionKeys := make(map[schema.Key]struct{}, len(metadata.Projections))
	for _, projection := range metadata.Projections {
		if !projection.Key.Available() || !projection.Relation.Available() || !projection.Result.Available() || !projection.Accessor.Available() || !projection.Role.Available() || !projection.CandidateProvider.Available() {
			return false
		}
		if _, duplicate := projectionKeys[projection.Key]; duplicate {
			return false
		}
		projectionKeys[projection.Key] = struct{}{}
		relation, relationOK := relationsByKey[projection.Relation]
		if !relationOK || relation.CandidateProvider != projection.CandidateProvider || projection.CandidateProviderLocal != relation.CandidateProviderLocal || projection.CandidateRelation != relation.CandidateRelation {
			return false
		}
		if !projectionAccessorCarrier(projection, relation) {
			return false
		}
	}
	reducerKeys := make(map[schema.Key]struct{}, len(metadata.Reducers))
	for _, reducer := range metadata.Reducers {
		if !reducer.Key.Available() || !reducer.Implementation.Available() || reducer.CandidatePresent != reducer.Candidate.Available() || reducer.CandidateConstant == reducer.CandidatePresent {
			return false
		}
		if _, duplicate := reducerKeys[reducer.Key]; duplicate {
			return false
		}
		reducerKeys[reducer.Key] = struct{}{}
		for _, input := range reducer.Inputs {
			if !input.Axis.Available() || !input.Type.Available() || !input.Form.Available() || !input.Multiplicity.Available() {
				return false
			}
			if input.Tag.Available() && input.Form != member.Selected && input.Form != member.Summary {
				return false
			}
		}
		for _, output := range reducer.Outputs {
			if !output.Axis.Available() || !output.Type.Available() {
				return false
			}
		}
	}
	allMemberKeys := make(map[schema.Key]struct{}, len(metadata.Relations)+len(metadata.Projections)+len(metadata.Reducers)+len(metadata.CarryTransforms))
	for _, relation := range metadata.Relations {
		allMemberKeys[relation.Key] = struct{}{}
	}
	for _, projection := range metadata.Projections {
		allMemberKeys[projection.Key] = struct{}{}
	}
	for _, reducer := range metadata.Reducers {
		allMemberKeys[reducer.Key] = struct{}{}
	}
	for _, transform := range metadata.CarryTransforms {
		if !transform.Key.Available() || !transform.Candidate.Available() || !transform.Input.Available() || !transform.Output.Available() || !transform.Implementation.Available() {
			return false
		}
		// A receiver-bearing transform is a candidate method. Reject an owner
		// Schema method (or any other unrelated receiver) here instead of
		// allowing a key-taking helper to masquerade as a candidate/prior-fact
		// direct call. Free functions remain valid direct symbols: their
		// candidate argument is supplied by the later emitter.
		if transform.Implementation.Receiver.Name != "" && transform.Implementation.Receiver != transform.Candidate {
			return false
		}
		if _, duplicate := allMemberKeys[transform.Key]; duplicate {
			return false
		}
		allMemberKeys[transform.Key] = struct{}{}
	}
	return true
}

func relationHasDerivation(relation membergenerator.RelationBinding) bool {
	derivation := relation.Derivation
	return derivation.State != (memberdefinition.GoType{}) || !symbolAbsent(derivation.Build) ||
		!symbolAbsent(derivation.Count) || !symbolAbsent(derivation.At) || len(derivation.StaticAxes) != 0
}

func relationDerivationComplete(derivation membergenerator.RelationDerivationBinding) bool {
	if !derivation.State.Available() || !derivation.Build.Available() || !derivation.Count.Available() || !derivation.At.Available() || len(derivation.StaticAxes) == 0 {
		return false
	}
	seen := make(map[schema.Key]struct{}, len(derivation.StaticAxes))
	for _, axis := range derivation.StaticAxes {
		if axis.Surface != schema.SurfaceKindAxis || !axis.Key.Available() {
			return false
		}
		if _, duplicate := seen[axis.Key]; duplicate {
			return false
		}
		seen[axis.Key] = struct{}{}
	}
	return true
}

func findAxis(catalog ruleplan.Catalog, key schema.Key) (uint32, bool) {
	for index := 0; index < catalog.AxisCount(); index++ {
		axis, axisOK := catalog.AxisAt(index)
		if axisOK && axis.Key == key {
			return uint32(index), true
		}
	}
	return 0, false
}

func axisKeyAt(catalog ruleplan.Catalog, ordinal uint32) (schema.Key, bool) {
	if uint64(ordinal) >= uint64(catalog.AxisCount()) {
		return "", false
	}
	axis, axisOK := catalog.AxisAt(int(ordinal))
	if !axisOK || !axis.Key.Available() {
		return "", false
	}
	return axis.Key, true
}

func ownerMetadata(byAxis map[uint32]membergenerator.Metadata, catalog ruleplan.Catalog, ordinal uint32) (membergenerator.Metadata, schema.Key, bool) {
	metadata, metadataOK := byAxis[ordinal]
	if !metadataOK {
		return membergenerator.Metadata{}, "", false
	}
	key, keyOK := axisKeyAt(catalog, ordinal)
	return metadata, key, keyOK && metadata.Axis == key
}

func relationOrdinal(relations []membergenerator.RelationBinding, key schema.Key) (uint32, bool) {
	for index, relation := range relations {
		if relation.Key == key {
			return uint32(index), true
		}
	}
	return 0, false
}

func symbolAbsent(symbol memberdefinition.GoSymbol) bool {
	return symbol == (memberdefinition.GoSymbol{})
}

func symbolDirectoryComplete(relation membergenerator.RelationBinding) bool {
	return relation.CandidateResolver.Available() && relation.CandidateOrdinal.Available() && relation.CandidateAt.Available()
}

func sameGoType(left, right memberdefinition.GoType) bool {
	return left.PackagePath == right.PackagePath && left.Name == right.Name
}

func projectionAccessorCarrier(projection membergenerator.ProjectionBinding, relation membergenerator.RelationBinding) bool {
	if !projection.Accessor.Receiver.Available() {
		return false
	}
	if sameGoType(projection.Accessor.Receiver, relation.Subject) {
		return true
	}
	for _, input := range relation.Inputs {
		if sameGoType(projection.Accessor.Receiver, input) {
			return true
		}
	}
	return false
}

// resolveCandidateProvider resolves the explicit provider relation through
// the composition roster. The returned address is always the provider's
// sealed axis/member address, even when the consumer relation belongs to a
// different axis. The provider itself must be a local self-owned directory;
// a consumer may never replace that proof with a local CandidateAt mirror.
func resolveCandidateProvider(byAxis map[uint32]membergenerator.Metadata, catalog ruleplan.Catalog, relation membergenerator.RelationBinding) (membergenerator.RelationBinding, ruleplan.RelationAddr, schema.Key, bool) {
	// An issued provider has no axis address to resolve. Its rows are Program
	// rows, reached by the ordinal the mounted placement carries, so this
	// resolver refuses it rather than inventing a directory for it.
	if relation.CandidateProvider.Issued() {
		return membergenerator.RelationBinding{}, ruleplan.RelationAddr{}, "", false
	}
	providerRef := relation.CandidateProvider.AxisRelation
	if !providerRef.Available() {
		return membergenerator.RelationBinding{}, ruleplan.RelationAddr{}, "", false
	}
	providerAxis, providerAxisOK := findAxis(catalog, providerRef.Axis.Key)
	if !providerAxisOK {
		return membergenerator.RelationBinding{}, ruleplan.RelationAddr{}, "", false
	}
	providerMetadata, providerAxisKey, providerMetadataOK := ownerMetadata(byAxis, catalog, providerAxis)
	if !providerMetadataOK || providerAxisKey != providerRef.Axis.Key {
		return membergenerator.RelationBinding{}, ruleplan.RelationAddr{}, "", false
	}
	providerOrdinal, providerOrdinalOK := relationOrdinal(providerMetadata.Relations, providerRef.Member)
	if !providerOrdinalOK {
		return membergenerator.RelationBinding{}, ruleplan.RelationAddr{}, "", false
	}
	provider := providerMetadata.Relations[providerOrdinal]
	providerAddress := ruleplan.RelationAddr{Axis: providerAxis, Member: providerOrdinal}
	if provider.CandidateProvider.AxisRelation.Axis != providerRef.Axis || provider.CandidateProvider.AxisRelation.Member != provider.Key ||
		!provider.CandidateProviderLocal || !provider.HasCandidateRelation || provider.CandidateRelation != providerOrdinal ||
		!symbolDirectoryComplete(provider) {
		return membergenerator.RelationBinding{}, ruleplan.RelationAddr{}, "", false
	}
	return provider, providerAddress, providerAxisKey, true
}

func resolveRelationDerivation(byAxis map[uint32]membergenerator.Metadata, catalog ruleplan.Catalog, relation membergenerator.RelationBinding) (RelationDerivation, bool) {
	if !relationHasDerivation(relation) {
		return RelationDerivation{}, true
	}
	if !relationDerivationComplete(relation.Derivation) {
		return RelationDerivation{}, false
	}
	staticAxes := make([]Axis, len(relation.Derivation.StaticAxes))
	for index, reference := range relation.Derivation.StaticAxes {
		axisOrdinal, axisOK := findAxis(catalog, reference.Key)
		if !axisOK {
			return RelationDerivation{}, false
		}
		metadata, axisKey, metadataOK := ownerMetadata(byAxis, catalog, axisOrdinal)
		if !metadataOK || metadata.Axis != reference.Key || axisKey != reference.Key {
			return RelationDerivation{}, false
		}
		staticAxes[index] = Axis{ordinal: axisOrdinal, key: axisKey}
	}
	return RelationDerivation{
		state: relation.Derivation.State, build: relation.Derivation.Build,
		count: relation.Derivation.Count, at: relation.Derivation.At,
		staticAxes: staticAxes,
	}, true
}

func buildRule(byAxis map[uint32]membergenerator.Metadata, catalog ruleplan.Catalog, ordinal uint32, compiled ruleplan.Plan) (Rule, ProblemKind, bool) {
	candidateAddress := compiled.Candidate()
	candidateMetadata, candidateAxis, candidateMetadataOK := ownerMetadata(byAxis, catalog, candidateAddress.Axis)
	if !candidateMetadataOK || uint64(candidateAddress.Member) >= uint64(len(candidateMetadata.Relations)) {
		return Rule{}, ProblemCandidate, false
	}
	candidateRelation := candidateMetadata.Relations[candidateAddress.Member]
	candidateProvider, candidateProviderAddress, _, candidateProviderOK := resolveCandidateProvider(byAxis, catalog, candidateRelation)
	if !candidateRelation.Key.Available() || !candidateRelation.Subject.Available() || !candidateProviderOK || candidateProviderAddress != candidateAddress || candidateProvider.Key != candidateRelation.Key || !symbolDirectoryComplete(candidateRelation) {
		return Rule{}, ProblemCandidate, false
	}

	reducerAddress := compiled.Reducer()
	reducerMetadata, reducerAxis, reducerMetadataOK := ownerMetadata(byAxis, catalog, reducerAddress.Axis)
	if !reducerMetadataOK || uint64(reducerAddress.Member) >= uint64(len(reducerMetadata.Reducers)) {
		return Rule{}, ProblemReducer, false
	}
	reducer := reducerMetadata.Reducers[reducerAddress.Member]
	if !reducer.Key.Available() || !reducer.Implementation.Available() {
		return Rule{}, ProblemSymbol, false
	}
	if reducer.CandidatePresent != reducer.Candidate.Available() || reducer.CandidateConstant == reducer.CandidatePresent || (reducer.CandidatePresent && reducer.Candidate != candidateRelation.Subject) {
		return Rule{}, ProblemCandidate, false
	}

	joins, joinKind, joinOK := verifyJoins(byAxis, catalog, compiled)
	if !joinOK {
		return Rule{}, joinKind, false
	}
	// Outputs are verified before inputs because the route correspondence is
	// theirs: a join is a route join because an output writes through it. The
	// destination coordinate a routed fold receives is resolved once there and
	// handed to the input verification, so the two never disagree about which
	// joins are routed.
	outputs, routes, outputKind, outputOK := verifyOutputs(byAxis, catalog, compiled, reducer)
	if !outputOK {
		return Rule{}, outputKind, false
	}
	inputs, inputKind, inputOK := verifyReducerInputs(byAxis, catalog, compiled, reducer, joins, routes)
	if !inputOK {
		return Rule{}, inputKind, false
	}
	carry, carryKind, carryOK := verifyCarry(byAxis, catalog, compiled, candidateRelation, outputs)
	if !carryOK {
		return Rule{}, carryKind, false
	}
	_, carryPresent := compiled.Carry()

	call := ReducerCall{
		Address: reducerAddress, Axis: reducerAxis, Key: reducer.Key, Rule: reducer.Rule,
		Implementation: reducer.Implementation, Candidate: reducer.Candidate,
		CandidatePresent: reducer.CandidatePresent, CandidateConstant: reducer.CandidateConstant,
		Inputs: inputs, Outputs: outputs, Outcome: ReducerOutcomeType,
	}
	return Rule{
		ordinal: ordinal,
		candidate: Candidate{
			address: candidateAddress, axis: candidateAxis, key: candidateRelation.Key, subject: candidateRelation.Subject,
			resolver: candidateRelation.CandidateResolver, ordinal: candidateRelation.CandidateOrdinal, at: candidateRelation.CandidateAt,
		},
		joins: joins, reducer: call, outputs: outputs, carry: carry, carryPresent: carryPresent,
	}, ProblemNone, true
}

// verifyCarry resolves the optional carry against the exact owner metadata
// row selected by the sealed Plan. It deliberately consumes the retained
// transform address/key from Plan: no member key, candidate, axis, or input
// port is inferred from an output row or from a zero address.
func verifyCarry(byAxis map[uint32]membergenerator.Metadata, catalog ruleplan.Catalog, compiled ruleplan.Plan, candidate membergenerator.RelationBinding, outputs []Output) (Carry, ProblemKind, bool) {
	compiledCarry, present := compiled.Carry()
	if !present {
		return Carry{}, ProblemNone, true
	}
	if !compiledCarry.Mode.Available() || uint64(compiledCarry.Input) >= uint64(compiled.InputCount()) {
		return Carry{}, ProblemCarry, false
	}
	carry := Carry{input: compiledCarry.Input, mode: compiledCarry.Mode}
	if compiledCarry.Mode == program.CarryIdentity {
		if compiledCarry.TransformPresent || compiledCarry.TransformKey.Available() || compiledCarry.TransformAxis.Available() {
			return Carry{}, ProblemCarry, false
		}
		return carry, ProblemNone, true
	}
	if compiledCarry.Mode != program.CarryTransform || !compiledCarry.TransformPresent || !compiledCarry.TransformAxis.Available() || !compiledCarry.TransformKey.Available() {
		return Carry{}, ProblemCarry, false
	}
	axisMetadata, axisKey, axisOK := ownerMetadata(byAxis, catalog, compiledCarry.Transform.Axis)
	if !axisOK || axisKey != compiledCarry.TransformAxis || uint64(compiledCarry.Transform.Member) >= uint64(len(axisMetadata.CarryTransforms)) {
		return Carry{}, ProblemCarry, false
	}
	transform := axisMetadata.CarryTransforms[compiledCarry.Transform.Member]
	if transform.Key != compiledCarry.TransformKey || transform.Candidate != candidate.Subject || !transform.Input.Available() || !transform.Output.Available() || !transform.Implementation.Available() {
		return Carry{}, ProblemCarry, false
	}
	if len(outputs) == 0 {
		return Carry{}, ProblemCarry, false
	}
	// Every output in a valid Plan is written by the same owner axis and has
	// one fact type. Compare the transform signature with each retained output
	// type so a multi-column plan cannot launder a transform through the last
	// output row.
	for _, output := range outputs {
		outputMetadata, outputAxis, outputOK := ownerMetadata(byAxis, catalog, output.address.Axis)
		if !outputOK || outputAxis != compiledCarry.TransformAxis || transform.Input != outputMetadata.FactType || transform.Output != outputMetadata.FactType {
			return Carry{}, ProblemCarry, false
		}
	}
	carry.transform = compiledCarry.Transform
	carry.transformPresent = true
	carry.transformAxis = compiledCarry.TransformAxis
	carry.transformKey = compiledCarry.TransformKey
	carry.candidate = transform.Candidate
	carry.inputType = transform.Input
	carry.outputType = transform.Output
	carry.implementation = transform.Implementation
	return carry, ProblemNone, true
}

func verifyJoins(byAxis map[uint32]membergenerator.Metadata, catalog ruleplan.Catalog, compiled ruleplan.Plan) ([]Join, ProblemKind, bool) {
	joins := make([]Join, compiled.JoinCount())
	for index := 0; index < compiled.JoinCount(); index++ {
		join, joinOK := compiled.JoinAt(index)
		if !joinOK || uint64(join.Sources.Start)+uint64(join.Sources.Count) > uint64(compiled.SourceCount()) {
			return nil, ProblemMember, false
		}
		relationMetadata, relationAxis, relationMetadataOK := ownerMetadata(byAxis, catalog, join.Relation.Axis)
		keyMetadata, keyAxis, keyMetadataOK := ownerMetadata(byAxis, catalog, join.Key.Axis)
		readMetadata, readAxisKey, readMetadataOK := ownerMetadata(byAxis, catalog, join.ReadAxis)
		if !relationMetadataOK || !keyMetadataOK || !readMetadataOK || uint64(join.Relation.Member) >= uint64(len(relationMetadata.Relations)) || uint64(join.Key.Member) >= uint64(len(keyMetadata.Projections)) {
			return nil, ProblemMember, false
		}
		relation := relationMetadata.Relations[join.Relation.Member]
		key := keyMetadata.Projections[join.Key.Member]
		if relation.Key != key.Relation || key.Role != member.Key || key.Result != keyMetadata.Key.Input || !key.Accessor.Available() || !projectionAccessorCarrier(key, relation) || key.CandidateProvider != relation.CandidateProvider || uint64(join.Sources.Count) != uint64(len(relation.Inputs)) || !relation.CandidateProvider.Available() {
			return nil, ProblemMember, false
		}
		providerRelation, providerAddress, _, providerOK := resolveCandidateProvider(byAxis, catalog, relation)
		if !providerOK {
			return nil, ProblemMember, false
		}
		derivation, derivationOK := resolveRelationDerivation(byAxis, catalog, relation)
		if !derivationOK {
			return nil, ProblemMember, false
		}
		providerCarrierPresent := false
		for _, input := range relation.Inputs {
			if sameGoType(input, providerRelation.Subject) {
				providerCarrierPresent = true
				break
			}
		}
		if join.Relation != providerAddress && !providerCarrierPresent {
			return nil, ProblemCandidate, false
		}
		for sourceIndex := uint32(0); sourceIndex < join.Sources.Count; sourceIndex++ {
			flatIndex := join.Sources.Start + sourceIndex
			source, sourceOK := compiled.SourceAt(int(flatIndex))
			if !sourceOK {
				return nil, ProblemMember, false
			}
			var sourceType memberdefinition.GoType
			if source.Candidate {
				candidateAddress := compiled.Candidate()
				candidateMetadata, _, candidateOK := ownerMetadata(byAxis, catalog, candidateAddress.Axis)
				if !candidateOK || uint64(candidateAddress.Member) >= uint64(len(candidateMetadata.Relations)) {
					return nil, ProblemCandidate, false
				}
				sourceType = candidateMetadata.Relations[candidateAddress.Member].Subject
			} else {
				if source.Position >= uint32(index) || source.Position >= uint32(len(joins)) {
					return nil, ProblemMember, false
				}
				sourceType = joins[source.Position].ResultType
			}
			if relation.Inputs[sourceIndex] != sourceType {
				return nil, ProblemCandidate, false
			}
		}
		var predicate membergenerator.ProjectionBinding
		predicatePresent := join.PredicatePresent
		predicateAxisKey := schema.Key("")
		if predicatePresent {
			predicateMetadata, predicateAxis, predicateOK := ownerMetadata(byAxis, catalog, join.Predicate.Axis)
			if !predicateOK || uint64(join.Predicate.Member) >= uint64(len(predicateMetadata.Projections)) {
				return nil, ProblemMember, false
			}
			predicate = predicateMetadata.Projections[join.Predicate.Member]
			if predicate.Relation != relation.Key || predicate.Role != member.Predicate || !predicate.Result.Available() || predicate.CandidateProvider != relation.CandidateProvider || !projectionAccessorCarrier(predicate, relation) {
				return nil, ProblemMember, false
			}
			predicateAxisKey = predicateAxis
		}
		if !join.ReadForm.Available() || !join.ReadContract.Multiplicity.Available() {
			return nil, ProblemForm, false
		}
		joins[index] = Join{
			Position: uint32(index), Relation: join.Relation, RelationAxis: relationAxis,
			Key: join.Key, KeyAxis: keyAxis, Predicate: join.Predicate,
			PredicateAxis: predicateAxisKey, PredicatePresent: predicatePresent,
			ReadAxis: join.ReadAxis, ReadAxisKey: readAxisKey, ReadForm: join.ReadForm,
			Multiplicity: join.ReadContract.Multiplicity, Cardinality: join.Cardinality,
			Denominator: join.Denominator, ResultType: readMetadata.FactType,
			RelationResolver: providerRelation.CandidateResolver, RelationOrdinal: providerRelation.CandidateOrdinal, RelationAt: providerRelation.CandidateAt,
			RelationCandidate: providerAddress,
			KeyAccessor:       key.Accessor,
			derivation:        derivation,
			derivationPresent: relationHasDerivation(relation),
		}
		if predicatePresent {
			joins[index].PredicateAccessor = predicate.Accessor
		}
	}
	return joins, ProblemNone, true
}

// verifyReducerInputs holds each declared input to the join it reads. Two of
// its three carriers are conditional and neither condition is the input row's
// to state: a tag is required exactly when the join declares a Predicate, and a
// route coordinate exactly when an output routes through that join. Both
// conditions are the rule's plan, which is why the declaration leaves them
// optional and this is where they are settled.
func verifyReducerInputs(byAxis map[uint32]membergenerator.Metadata, catalog ruleplan.Catalog, compiled ruleplan.Plan, reducer membergenerator.ReducerBinding, joins []Join, routes map[uint32]memberdefinition.GoType) ([]ReducerInput, ProblemKind, bool) {
	if compiled.FoldInputCount() != len(reducer.Inputs) {
		return nil, ProblemReducer, false
	}
	inputs := make([]ReducerInput, len(reducer.Inputs))
	for index, binding := range reducer.Inputs {
		joinOrdinal, joinOK := compiled.FoldInputAt(index)
		if !joinOK || uint64(joinOrdinal) >= uint64(len(joins)) {
			return nil, ProblemReducer, false
		}
		join := joins[joinOrdinal]
		if binding.Axis.Surface != schema.SurfaceKindAxis || binding.Axis.Key != join.ReadAxisKey || binding.Form != join.ReadForm || binding.Multiplicity != join.Multiplicity || binding.Type != join.ResultType {
			return nil, ProblemForm, false
		}
		tag := binding.Tag
		tagged := tag.Available()
		if join.PredicatePresent {
			predicateMetadata, _, predicateOK := ownerMetadata(byAxis, catalog, join.Predicate.Axis)
			if !predicateOK || uint64(join.Predicate.Member) >= uint64(len(predicateMetadata.Projections)) {
				return nil, ProblemMember, false
			}
			predicate := predicateMetadata.Projections[join.Predicate.Member]
			if !tagged || tag != predicate.Result {
				return nil, ProblemForm, false
			}
		} else if tagged {
			return nil, ProblemForm, false
		}
		// A route coordinate is available on every join an output routes
		// through, so taking it is the declaration's choice, exactly as the
		// candidate carrier is: a fold whose answer does not depend on which
		// coordinate it publishes at does not grow a parameter for one.
		// Declaring it where no output routes through the join is refused -
		// there is no coordinate to deliver.
		route := binding.Route
		routed := route.Available()
		if routed {
			destination, routeJoin := routes[joinOrdinal]
			if !routeJoin || route != destination {
				return nil, ProblemForm, false
			}
		}
		inputs[index] = ReducerInput{Join: joinOrdinal, Type: binding.Type, Form: binding.Form, Multiplicity: binding.Multiplicity, Tag: tag, Tagged: tagged, Route: route, Routed: routed}
	}
	return inputs, ProblemNone, true
}

// verifyOutputs resolves every published column and, for a routed one, the
// destination coordinate its route join delivers to the fold. That second
// result is the route correspondence stated once: the input verification reads
// it rather than rediscovering which joins a plan routes through.
func verifyOutputs(byAxis map[uint32]membergenerator.Metadata, catalog ruleplan.Catalog, compiled ruleplan.Plan, reducer membergenerator.ReducerBinding) ([]Output, map[uint32]memberdefinition.GoType, ProblemKind, bool) {
	if compiled.OutputCount() != len(reducer.Outputs) {
		return nil, nil, ProblemOutputType, false
	}
	outputs := make([]Output, compiled.OutputCount())
	routes := make(map[uint32]memberdefinition.GoType, compiled.OutputCount())
	seenSlots := make(map[uint32]struct{}, len(outputs))
	seenRouteDestinations := make(map[ruleplan.ProjectionAddr]struct{}, len(outputs))
	candidateAddress := compiled.Candidate()
	candidateMetadata, _, candidateMetadataOK := ownerMetadata(byAxis, catalog, candidateAddress.Axis)
	if !candidateMetadataOK || uint64(candidateAddress.Member) >= uint64(len(candidateMetadata.Relations)) {
		return nil, nil, ProblemCandidate, false
	}
	candidateRelation := candidateMetadata.Relations[candidateAddress.Member]
	if !candidateRelation.Key.Available() {
		return nil, nil, ProblemCandidate, false
	}
	for index := 0; index < compiled.OutputCount(); index++ {
		compiledOutput, outputOK := compiled.OutputAt(index)
		if !outputOK || uint64(compiledOutput.Slot) >= uint64(len(reducer.Outputs)) {
			return nil, nil, ProblemOutputType, false
		}
		if _, duplicate := seenSlots[compiledOutput.Slot]; duplicate {
			return nil, nil, ProblemOutputType, false
		}
		seenSlots[compiledOutput.Slot] = struct{}{}
		outputMetadata, outputAxis, outputMetadataOK := ownerMetadata(byAxis, catalog, compiledOutput.Address.Axis)
		destinationMetadata, destinationAxis, destinationMetadataOK := ownerMetadata(byAxis, catalog, compiledOutput.Destination.Axis)
		if !outputMetadataOK || !destinationMetadataOK || uint64(compiledOutput.Destination.Member) >= uint64(len(destinationMetadata.Projections)) || uint64(compiledOutput.Slot) >= uint64(len(reducer.Outputs)) {
			return nil, nil, ProblemMember, false
		}
		binding := reducer.Outputs[compiledOutput.Slot]
		axisKey, axisKeyOK := axisKeyAt(catalog, compiledOutput.Address.Axis)
		if !axisKeyOK || binding.Axis.Surface != schema.SurfaceKindAxis || binding.Axis.Key != axisKey || binding.Type != outputMetadata.FactType || !compiledOutput.Mode.Available() {
			return nil, nil, ProblemOutputType, false
		}
		if compiledOutput.Mode != program.ModeRoute && compiledOutput.RouteJoinPresent {
			return nil, nil, ProblemOutputType, false
		}
		if compiledOutput.Mode == program.ModeRoute {
			if !compiledOutput.RouteJoinPresent || uint64(compiledOutput.RouteJoin) >= uint64(compiled.JoinCount()) {
				return nil, nil, ProblemOutputType, false
			}
			if _, duplicate := seenRouteDestinations[compiledOutput.Destination]; duplicate {
				return nil, nil, ProblemMember, false
			}
			seenRouteDestinations[compiledOutput.Destination] = struct{}{}
			routeJoin, routeJoinOK := compiled.JoinAt(int(compiledOutput.RouteJoin))
			if !routeJoinOK || routeJoin.ReadForm != member.Selected || !routeJoin.Cardinality.Available() || routeJoin.ReadContract.Multiplicity == member.MultiplicityMany || routeJoin.Cardinality == member.MultiplicityMany || !routeJoin.Denominator.Present {
				return nil, nil, ProblemForm, false
			}
			foldInput := false
			for inputIndex := 0; inputIndex < compiled.FoldInputCount(); inputIndex++ {
				input, inputOK := compiled.FoldInputAt(inputIndex)
				if inputOK && input == compiledOutput.RouteJoin {
					foldInput = true
					break
				}
			}
			if !foldInput {
				return nil, nil, ProblemReducer, false
			}
		}
		destination := destinationMetadata.Projections[compiledOutput.Destination.Member]
		if destination.Role != member.Destination || !destination.Result.Available() || destination.Result != outputMetadata.Key.Input {
			return nil, nil, ProblemMember, false
		}
		if compiledOutput.Mode == program.ModeRoute {
			routeJoin, routeJoinOK := compiled.JoinAt(int(compiledOutput.RouteJoin))
			if !routeJoinOK {
				return nil, nil, ProblemMember, false
			}
			routeRelationMetadata, _, routeRelationOK := ownerMetadata(byAxis, catalog, routeJoin.Relation.Axis)
			if !routeRelationOK || uint64(routeJoin.Relation.Member) >= uint64(len(routeRelationMetadata.Relations)) || destination.CandidateProvider != routeRelationMetadata.Relations[routeJoin.Relation.Member].CandidateProvider {
				return nil, nil, ProblemMember, false
			}
			routeRelation := routeRelationMetadata.Relations[routeJoin.Relation.Member]
			routeAxisKey, routeAxisOK := axisKeyAt(catalog, routeJoin.Relation.Axis)
			if !routeAxisOK || destinationAxis != routeAxisKey || destination.Relation != routeRelation.Key {
				return nil, nil, ProblemMember, false
			}
		} else {
			candidateAxisKey, candidateAxisOK := axisKeyAt(catalog, candidateAddress.Axis)
			if !candidateAxisOK {
				return nil, nil, ProblemMember, false
			}
			switch {
			case destinationAxis == candidateAxisKey:
				if destination.Relation != candidateRelation.Key || destination.CandidateProvider != candidateRelation.CandidateProvider {
					return nil, nil, ProblemMember, false
				}
			case compiledOutput.Mode == program.ModeExact && destinationAxis == outputAxis && destinationAxis != candidateAxisKey:
				// Consumer-owned exact projection: the output axis declares a
				// relation addressed by this exact foreign candidate.  The
				// emitted installer applies its typed accessor and normalizes the
				// result with the output schema; no candidate ordinal is copied
				// into the consumer owner.
				var destinationProvider member.CandidateRef
				relationFound := false
				for _, relation := range destinationMetadata.Relations {
					if relation.Key == destination.Relation {
						destinationProvider, relationFound = relation.CandidateProvider, true
						break
					}
				}
				if !relationFound || destinationProvider != candidateRelation.CandidateProvider || destination.CandidateProvider != candidateRelation.CandidateProvider {
					return nil, nil, ProblemMember, false
				}
			default:
				return nil, nil, ProblemMember, false
			}
		}
		if compiledOutput.Mode == program.ModeRoute {
			if existing, duplicate := routes[compiledOutput.RouteJoin]; duplicate && existing != destination.Result {
				return nil, nil, ProblemMember, false
			}
			routes[compiledOutput.RouteJoin] = destination.Result
		}
		outputs[index] = newOutput(compiledOutput, outputAxis, destinationAxis, binding.Type, destination.Accessor)
	}
	return outputs, routes, ProblemNone, true
}

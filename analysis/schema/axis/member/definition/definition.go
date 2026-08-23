// Package definition owns the neutral, owner-authored source form for one
// axis member vocabulary. It contains schema declarations and callback-free
// Go symbol descriptors only; execution choreography remains in rule.Program
// and its sealed plan.
package definition

import (
	"go/token"
	"strings"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// GoType is a source-level reference to one Go type. PackagePath is empty only
// for a built-in type. The reference is metadata for a later composition
// generator; it is never resolved or retained as a runtime value.
type GoType struct {
	PackagePath string
	Name        string
}

func (typ GoType) Available() bool {
	if typ.Name == "" || typ.Name == "_" || !token.IsIdentifier(typ.Name) {
		return false
	}
	if typ.PackagePath == "" {
		switch typ.Name {
		case "bool", "byte", "error", "int", "int8", "int16", "int32", "int64",
			"rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
			return true
		default:
			return false
		}
	}
	return strings.TrimSpace(typ.PackagePath) == typ.PackagePath
}

func sameType(left, right GoType) bool {
	return left.PackagePath == right.PackagePath && left.Name == right.Name
}

// sameOwnerSymbol states the owner fence for a direct member symbol. A
// receiver-bearing declaration must be issued by the same owner type and
// package as the axis binding's key normalizer; composition cannot substitute
// a foreign method or infer an adapter around it.
func sameOwnerSymbol(symbol GoSymbol, owner GoType) bool {
	return symbol.Available() && symbol.Receiver.Name != "" && sameType(symbol.Receiver, owner) && symbol.PackagePath == owner.PackagePath
}

// GoSymbol is a callback-free qualified source reference. Receiver records
// the method's owner type, ReceiverPointer records its receiver shape, and
// ResultIndex selects one result when a tuple-returning symbol feeds a single
// projection. Parameter and result lists intentionally do not live here:
// those signatures are derived from the carrier rows by the composition
// generator, where a direct call is emitted and compile-checked.
type GoSymbol struct {
	PackagePath     string
	Name            string
	Receiver        GoType
	ReceiverPointer bool
	ResultIndex     int8
}

func (symbol GoSymbol) Available() bool {
	if strings.TrimSpace(symbol.PackagePath) != symbol.PackagePath || symbol.PackagePath == "" ||
		symbol.Name == "" || symbol.Name == "_" || !token.IsIdentifier(symbol.Name) || symbol.ResultIndex < -1 {
		return false
	}
	if symbol.Receiver.Name == "" {
		return !symbol.ReceiverPointer
	}
	return symbol.Receiver.Available()
}

func symbolOptional(symbol GoSymbol) bool {
	return symbol.PackagePath == "" && symbol.Name == "" && symbol.Receiver == (GoType{}) &&
		!symbol.ReceiverPointer && symbol.ResultIndex == 0
}

func derivationOptional(derivation RelationDerivation) bool {
	return derivation.State == (GoType{}) && symbolOptional(derivation.Build) &&
		symbolOptional(derivation.Count) && symbolOptional(derivation.At) && len(derivation.StaticAxes) == 0
}

func cloneSymbol(symbol GoSymbol) GoSymbol { return symbol }

// Carrier names one exported Go constant, its owner-issued carrier key, and
// the Go type carried by that key. Type is the single source of identity for
// member-level signature derivation; relation/projection/reducer rows never
// repeat it.
type Carrier struct {
	Name string
	Key  member.Carrier
	Type GoType
}

// Relation is a named owner-issued relation declaration. Subject and Inputs
// refer to Carrier.Name values in this same definition. CandidateResolver is
// optional for relations whose rows are derived by composition; when present
// it is a direct typed symbol descriptor, never a callback.
type Relation struct {
	Name    string
	Key     schema.Key
	Subject string
	Inputs  []string
	// CandidateProvider explicitly names the owner-qualified dense directory
	// used by this relation. It is required even when the provider is a
	// same-axis relation; no carrier-type inference is permitted.
	CandidateProvider member.RelationRef
	CandidateResolver GoSymbol
	// CandidateOrdinal and CandidateAt are the two dense-directory symbols
	// paired with CandidateResolver. They are optional together: a relation
	// without a resolver is composition-derived and carries no owner directory
	// metadata. When present, all three symbols are direct methods on the same
	// owner receiver; the generator derives their argument/result types from
	// this relation's subject and the axis binding's dense type.
	CandidateOrdinal GoSymbol
	CandidateAt      GoSymbol
	// CandidateCount seals the exact width of a materializable candidate
	// directory. Materializers size their typed source column from this
	// owner-issued census and then prove every ordinal is occupied; they do not
	// probe CandidateAt until it happens to fail.
	CandidateCount GoSymbol
	// Materialize is the optional zero-input reducer applied to one dense
	// candidate. It is the source/ingress fact producer: (subject) (Fact, bool).
	Materialize GoSymbol
	// CandidateIdentityAt declares that this relation is addressed globally
	// rather than by mount: it publishes the occurrence identity of each dense
	// candidate, (index) (identity.ContentID, bool). Its presence is the whole
	// statement - a relation that names its own occurrence directory resolves
	// candidates from an occurrence alone, and a Link rule reading this
	// relation derives its occurrence inventory from this directory instead of
	// from an artifact's rows.
	CandidateIdentityAt GoSymbol
	// Derivation is the optional typed construction of a dependent relation
	// row. It is invoked by generated composition code, never retained as a
	// runtime callback or owner handle.
	Derivation RelationDerivation
}

// RelationDerivation is the direct-call shape for one dependent relation's
// short-lived row. Build returns State from ordered StaticAxes followed by
// the relation Inputs; Count and At consume State to expose relation Subject
// rows in canonical order. The generator emits these calls directly, letting
// Go compile-check their concrete signatures.
type RelationDerivation struct {
	State      GoType
	Build      GoSymbol
	Count      GoSymbol
	At         GoSymbol
	StaticAxes []schema.EntryReference
}

func (derivation RelationDerivation) complete() bool {
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

// Projection is a named owner-issued projection declaration. Relation and
// Result refer to names in this same definition. Accessor is the typed direct
// accessor for this projection; its receiver and result type are checked from
// the related carrier rows by the composition generator.
type Projection struct {
	Name              string
	Key               schema.Key
	Relation          string
	Role              member.Role
	Result            string
	Accessor          GoSymbol
	CandidateProvider member.RelationRef
}

// ReducerInput is one named reducer input. Carrier and Tag refer to carrier
// names; an empty Tag is the untagged spelling for Exact and Complete reads.
type ReducerInput struct {
	Axis         schema.EntryReference
	Carrier      string
	Form         member.ReadForm
	Multiplicity member.Multiplicity
	Tag          string
}

// ReducerOutput is one named reducer output. Carrier refers to a carrier name.
type ReducerOutput struct {
	Axis    schema.EntryReference
	Carrier string
}

// Reducer is a named owner-issued reducer declaration. Implementation is a
// typed direct reducer symbol whose parameter/result signature is derived from
// the declared carrier rows by the composition generator.
type Reducer struct {
	Name string
	Key  schema.Key
	// Rule is the rule whose contribution declared this reducer. It is set by
	// Source.Compose from the contribution's own identity and is never authored
	// on a row: a reducer with no rule behind it is a fold nothing folds with.
	Rule schema.Key
	// Candidate is the optional owner-issued candidate/subject carrier passed
	// as the first argument to Implementation. An empty name is intentional:
	// reducers that fold only joined inputs have no candidate argument. The
	// generated call model represents that absence as the constant true guard,
	// rather than inventing a carrier or looking one up from a relation.
	Candidate      string
	Inputs         []ReducerInput
	Outputs        []ReducerOutput
	Implementation GoSymbol
}

// CarryTransform is a named owner-issued typed transform. Input and Output
// refer to carrier names in the same definition; the implementation is kept
// as a source-level Go symbol descriptor and never crosses into the runtime
// schema as a callback. A receiver-bearing implementation is invoked on the
// Candidate and receives the Input fact as its sole argument; a free function
// receives Candidate followed by Input. In either spelling the direct call
// returns Output and its boolean validity result.
type CarryTransform struct {
	Name           string
	Key            schema.Key
	Candidate      string
	Input          string
	Output         string
	Implementation GoSymbol
}

// KeyNormalization is the one axis-level conversion from an owner key carrier
// to the dense key consumed by the engine. Carrier supplies the input Go type;
// Dense names the normalized output type; Normalizer is the direct owner
// symbol. The generator derives the call signature as (Carrier.Type) -> Dense,
// with the owner's boolean validity result retained by the emitted call.
type KeyNormalization struct {
	Carrier    string
	Dense      GoType
	Normalizer GoSymbol
}

// Binding is the member-definition axis binding used in this migration. It
// carries only key normalization; the already-sealed fact algebra remains an
// axis-owner concern until the later axis-owner cut. It does not describe
// relation traversal, joins, reads, or output choreography.
type Binding struct {
	Key KeyNormalization
}

// Signature names the axis's two nominal carriers. They are references to
// Carrier.Name rather than repeated keys.
type Signature struct {
	Key  string
	Fact string
}

// Definition is the one authored source for an axis member vocabulary. Its
// named cold declarations and callback-free Go symbol descriptors are
// projected separately by member/generator, so generated outputs cannot
// become a second schema.
type Definition struct {
	Name            string
	Axis            schema.Key
	Binding         Binding
	Signature       Signature
	Carriers        []Carrier
	Relations       []Relation
	Projections     []Projection
	Reducers        []Reducer
	CarryTransforms []CarryTransform
}

func identifierAvailable(name string) bool {
	return name != "" && name != "_" && token.IsIdentifier(name)
}

func (definition Definition) carrierIndex() (map[string]Carrier, map[member.Carrier]struct{}, bool) {
	byName := make(map[string]Carrier, len(definition.Carriers))
	byKey := make(map[member.Carrier]struct{}, len(definition.Carriers))
	for _, carrier := range definition.Carriers {
		if !identifierAvailable(carrier.Name) || !carrier.Key.Available() || !carrier.Type.Available() {
			return nil, nil, false
		}
		if _, duplicate := byName[carrier.Name]; duplicate {
			return nil, nil, false
		}
		if _, duplicate := byKey[carrier.Key]; duplicate {
			return nil, nil, false
		}
		byName[carrier.Name] = carrier
		byKey[carrier.Key] = struct{}{}
	}
	return byName, byKey, true
}

// Catalog projects the named cold declarations into the declaration-only
// member catalog. It is the semantic bridge used by the generator and is also
// useful to owner-side admission tests.
func (definition Definition) Catalog() (member.Catalog, bool) {
	if !definition.Axis.Available() || !identifierAvailable(definition.Name) {
		return member.Catalog{}, false
	}
	carriers, _, carriersOK := definition.carrierIndex()
	if !carriersOK {
		return member.Catalog{}, false
	}
	if signature := definition.Signature; !identifierAvailable(signature.Key) || !identifierAvailable(signature.Fact) {
		return member.Catalog{}, false
	} else if _, keyOK := carriers[signature.Key]; !keyOK {
		return member.Catalog{}, false
	} else if _, factOK := carriers[signature.Fact]; !factOK {
		return member.Catalog{}, false
	}
	relations := make([]member.Relation, len(definition.Relations))
	relationNames := make(map[string]schema.Key, len(definition.Relations))
	relationKeys := make(map[schema.Key]struct{}, len(definition.Relations))
	for index, relation := range definition.Relations {
		if !identifierAvailable(relation.Name) || !relation.Key.Available() || relation.Subject == "" {
			return member.Catalog{}, false
		}
		if _, duplicate := relationNames[relation.Name]; duplicate {
			return member.Catalog{}, false
		}
		if _, duplicate := relationKeys[relation.Key]; duplicate {
			return member.Catalog{}, false
		}
		subject, subjectOK := carriers[relation.Subject]
		if !subjectOK {
			return member.Catalog{}, false
		}
		inputs := make([]member.Carrier, len(relation.Inputs))
		for inputIndex, inputName := range relation.Inputs {
			input, inputOK := carriers[inputName]
			if !inputOK {
				return member.Catalog{}, false
			}
			inputs[inputIndex] = input.Key
		}
		relations[index] = member.Relation{Key: relation.Key, Subject: subject.Key, Inputs: inputs, CandidateProvider: relation.CandidateProvider}
		relationNames[relation.Name] = relation.Key
		relationKeys[relation.Key] = struct{}{}
	}
	projections := make([]member.Projection, len(definition.Projections))
	projectionKeys := make(map[schema.Key]struct{}, len(definition.Projections))
	for index, projection := range definition.Projections {
		if !identifierAvailable(projection.Name) || !projection.Key.Available() || !projection.Role.Available() {
			return member.Catalog{}, false
		}
		if _, duplicate := projectionKeys[projection.Key]; duplicate {
			return member.Catalog{}, false
		}
		relation, relationOK := relationNames[projection.Relation]
		if !relationOK {
			return member.Catalog{}, false
		}
		result, resultOK := carriers[projection.Result]
		if !resultOK {
			return member.Catalog{}, false
		}
		projections[index] = member.Projection{Key: projection.Key, Relation: relation, Role: projection.Role, Result: result.Key, CandidateProvider: projection.CandidateProvider}
		projectionKeys[projection.Key] = struct{}{}
	}
	reducers := make([]member.Reducer, len(definition.Reducers))
	reducerKeys := make(map[schema.Key]struct{}, len(definition.Reducers))
	for index, reducer := range definition.Reducers {
		if !identifierAvailable(reducer.Name) || !reducer.Key.Available() || len(reducer.Outputs) == 0 {
			return member.Catalog{}, false
		}
		if _, duplicate := reducerKeys[reducer.Key]; duplicate {
			return member.Catalog{}, false
		}
		inputs := make([]member.ReducerInput, len(reducer.Inputs))
		for inputIndex, input := range reducer.Inputs {
			carrier, carrierOK := carriers[input.Carrier]
			if !carrierOK {
				return member.Catalog{}, false
			}
			var tag member.Carrier
			if input.Tag != "" {
				tagged, tagOK := carriers[input.Tag]
				if !tagOK {
					return member.Catalog{}, false
				}
				tag = tagged.Key
			}
			inputs[inputIndex] = member.ReducerInput{Axis: input.Axis, Carrier: carrier.Key, Form: input.Form, Multiplicity: input.Multiplicity, Tag: tag}
		}
		outputs := make([]member.ReducerOutput, len(reducer.Outputs))
		for outputIndex, output := range reducer.Outputs {
			carrier, carrierOK := carriers[output.Carrier]
			if !carrierOK {
				return member.Catalog{}, false
			}
			outputs[outputIndex] = member.ReducerOutput{Axis: output.Axis, Carrier: carrier.Key}
		}
		reducers[index] = member.Reducer{Key: reducer.Key, Inputs: inputs, Outputs: outputs}
		reducerKeys[reducer.Key] = struct{}{}
	}
	transforms := make([]member.CarryTransform, len(definition.CarryTransforms))
	transformKeys := make(map[schema.Key]struct{}, len(definition.CarryTransforms))
	for index, transform := range definition.CarryTransforms {
		if !identifierAvailable(transform.Name) || !transform.Key.Available() || !transform.Implementation.Available() {
			return member.Catalog{}, false
		}
		if _, duplicate := transformKeys[transform.Key]; duplicate {
			return member.Catalog{}, false
		}
		candidate, candidateOK := carriers[transform.Candidate]
		input, inputOK := carriers[transform.Input]
		output, outputOK := carriers[transform.Output]
		if !candidateOK || !inputOK || !outputOK {
			return member.Catalog{}, false
		}
		transforms[index] = member.CarryTransform{Key: transform.Key, Candidate: candidate.Key, Input: input.Key, Output: output.Key}
		transformKeys[transform.Key] = struct{}{}
	}
	return member.NewCatalog(relations, projections, reducers, transforms)
}

func (binding Binding) complete(carriers map[string]Carrier) bool {
	keyCarrier, keyOK := carriers[binding.Key.Carrier]
	return keyOK && binding.Key.Dense.Available() && binding.Key.Normalizer.Available() && keyCarrier.Type.Available()
}

// Complete validates both the cold declaration graph and every member-level
// typed implementation reference. A relation, projection, or reducer row is
// therefore one named source row for both generated outputs.
func (definition Definition) Complete() bool {
	catalog, catalogOK := definition.Catalog()
	if !catalogOK || !catalog.Available() {
		return false
	}
	carriers, _, carriersOK := definition.carrierIndex()
	if !carriersOK || definition.Binding.Key.Carrier != definition.Signature.Key || !definition.Binding.complete(carriers) {
		return false
	}
	relations := make(map[string]Relation, len(definition.Relations))
	relationsByKey := make(map[schema.Key]Relation, len(definition.Relations))
	owner := definition.Binding.Key.Normalizer.Receiver
	for _, relation := range definition.Relations {
		if !relation.CandidateProvider.Available() {
			return false
		}
		resolverOptional := symbolOptional(relation.CandidateResolver)
		ordinalOptional := symbolOptional(relation.CandidateOrdinal)
		atOptional := symbolOptional(relation.CandidateAt)
		materializeOptional := symbolOptional(relation.Materialize)
		countOptional := symbolOptional(relation.CandidateCount)
		derivationAbsent := derivationOptional(relation.Derivation)
		if !derivationAbsent {
			// A derivation belongs only to a dependent relation. Its state is
			// built from declared relation inputs and static sealed axes; it can
			// neither replace a provider directory nor coexist with ingress
			// materialization.
			if !relation.Derivation.complete() || relation.CandidateProvider.Axis.Key == definition.Axis && relation.CandidateProvider.Member == relation.Key ||
				len(relation.Inputs) == 0 || !resolverOptional || !ordinalOptional || !atOptional || !countOptional || !materializeOptional ||
				!symbolOptional(relation.CandidateIdentityAt) {
				return false
			}
		}
		if !materializeOptional && !relation.Materialize.Available() {
			return false
		}
		if !symbolOptional(relation.CandidateIdentityAt) {
			// A global relation is a closed owner directory of occurrences: it
			// owns the resolver triple, the census its inventory is bounded by,
			// and the identity of every dense row. None of the three can be
			// supplied by composition or inferred from the others.
			if !relation.CandidateIdentityAt.Available() || !sameOwnerSymbol(relation.CandidateIdentityAt, owner) ||
				resolverOptional || countOptional {
				return false
			}
		}
		if materializeOptional {
			if !countOptional {
				return false
			}
		} else if !relation.CandidateCount.Available() || !sameOwnerSymbol(relation.CandidateCount, owner) {
			// A source/ingress materializer owns an exact dense column width.
			// The count symbol is part of that same owner directory and cannot
			// be inferred from CandidateAt or supplied by composition.
			return false
		}
		if resolverOptional {
			// A directory is one closed owner-authored relation. A partial
			// directory cannot be repaired by composition or inferred from a
			// cold catalog.
			if !ordinalOptional || !atOptional {
				return false
			}
		} else {
			if !relation.CandidateResolver.Available() || !relation.CandidateOrdinal.Available() || !relation.CandidateAt.Available() {
				return false
			}
			if !sameType(relation.CandidateResolver.Receiver, relation.CandidateOrdinal.Receiver) ||
				!sameType(relation.CandidateResolver.Receiver, relation.CandidateAt.Receiver) ||
				relation.CandidateResolver.PackagePath != relation.CandidateOrdinal.PackagePath ||
				relation.CandidateResolver.PackagePath != relation.CandidateAt.PackagePath {
				return false
			}
			// A local candidate directory is authored by the axis owner. A
			// consumer may reference a foreign directory, but it may not copy
			// its resolver/ordinal/At symbols into this definition.
			if !sameOwnerSymbol(relation.CandidateResolver, owner) {
				return false
			}
		}
		relations[relation.Name] = relation
		relationsByKey[relation.Key] = relation
	}
	for _, relation := range definition.Relations {
		if relation.CandidateProvider.Axis.Key != definition.Axis {
			// Foreign ownership is resolved against the composition roster.
			// The consumer definition must not retain a second owner directory.
			if !symbolOptional(relation.CandidateResolver) || !symbolOptional(relation.CandidateOrdinal) || !symbolOptional(relation.CandidateAt) || !symbolOptional(relation.CandidateCount) || !symbolOptional(relation.Materialize) || !symbolOptional(relation.CandidateIdentityAt) {
				return false
			}
			continue
		}
		provider, providerOK := relationsByKey[relation.CandidateProvider.Member]
		if !providerOK {
			return false
		}
		providerHasDirectory := !symbolOptional(provider.CandidateResolver) &&
			!symbolOptional(provider.CandidateOrdinal) && !symbolOptional(provider.CandidateAt)
		if !providerHasDirectory {
			return false
		}
		if relation.Key == provider.Key {
			// Only the provider relation itself may carry the directory
			// symbols. A self-reference is explicit, not inferred.
			if symbolOptional(relation.CandidateResolver) || symbolOptional(relation.CandidateOrdinal) || symbolOptional(relation.CandidateAt) {
				return false
			}
			continue
		}
		// A dependent relation consumes the provider's typed candidate row
		// through its declared inputs; it does not own CandidateAt or mint a
		// local mirror. The subject carrier of the provider must occur as one
		// input by type, which is the only type check available before the
		// composition seal resolves the foreign axis.
		if !symbolOptional(relation.CandidateResolver) || !symbolOptional(relation.CandidateOrdinal) || !symbolOptional(relation.CandidateAt) {
			return false
		}
		if !symbolOptional(relation.CandidateCount) {
			return false
		}
		if !symbolOptional(relation.Materialize) {
			return false
		}
		if !symbolOptional(relation.CandidateIdentityAt) {
			return false
		}
		providerCarrier, providerCarrierOK := carriers[provider.Subject]
		if !providerCarrierOK {
			return false
		}
		inputCarrier := false
		for _, inputName := range relation.Inputs {
			input, inputOK := carriers[inputName]
			if inputOK && sameType(input.Type, providerCarrier.Type) {
				inputCarrier = true
				break
			}
		}
		if !inputCarrier {
			return false
		}
	}
	for _, projection := range definition.Projections {
		if !projection.CandidateProvider.Available() {
			return false
		}
		if !projection.Accessor.Available() {
			return false
		}
		relation, relationOK := relations[projection.Relation]
		if !relationOK || relation.CandidateProvider != projection.CandidateProvider || !projectionReceiverMatches(projection.Accessor, relation, carriers) {
			return false
		}
	}
	for _, reducer := range definition.Reducers {
		if !reducer.Implementation.Available() {
			return false
		}
		if reducer.Candidate != "" {
			candidate, candidateOK := carriers[reducer.Candidate]
			if !candidateOK || !candidate.Key.Available() {
				return false
			}
		}
	}
	for _, transform := range definition.CarryTransforms {
		if !transform.Implementation.Available() {
			return false
		}
	}
	return true
}

func projectionReceiverMatches(accessor GoSymbol, relation Relation, carriers map[string]Carrier) bool {
	if accessor.Receiver.Name == "" {
		return false
	}
	if subject, ok := carriers[relation.Subject]; ok && sameType(subject.Type, accessor.Receiver) {
		return true
	}
	for _, inputName := range relation.Inputs {
		if input, ok := carriers[inputName]; ok && sameType(input.Type, accessor.Receiver) {
			return true
		}
	}
	return false
}

// Clone returns an independent source definition. The generator uses this to
// ensure rendering never mutates the owner-authored source.
func (definition Definition) Clone() Definition {
	clone := definition
	clone.Carriers = append([]Carrier(nil), definition.Carriers...)
	clone.Relations = make([]Relation, len(definition.Relations))
	for index, relation := range definition.Relations {
		clone.Relations[index] = relation
		clone.Relations[index].Inputs = append([]string(nil), relation.Inputs...)
		clone.Relations[index].CandidateProvider = relation.CandidateProvider
		clone.Relations[index].CandidateResolver = cloneSymbol(relation.CandidateResolver)
		clone.Relations[index].CandidateOrdinal = cloneSymbol(relation.CandidateOrdinal)
		clone.Relations[index].CandidateAt = cloneSymbol(relation.CandidateAt)
		clone.Relations[index].CandidateCount = cloneSymbol(relation.CandidateCount)
		clone.Relations[index].Materialize = cloneSymbol(relation.Materialize)
		clone.Relations[index].CandidateIdentityAt = cloneSymbol(relation.CandidateIdentityAt)
		clone.Relations[index].Derivation = relation.Derivation
		clone.Relations[index].Derivation.Build = cloneSymbol(relation.Derivation.Build)
		clone.Relations[index].Derivation.Count = cloneSymbol(relation.Derivation.Count)
		clone.Relations[index].Derivation.At = cloneSymbol(relation.Derivation.At)
		clone.Relations[index].Derivation.StaticAxes = append([]schema.EntryReference(nil), relation.Derivation.StaticAxes...)
	}
	clone.Projections = make([]Projection, len(definition.Projections))
	for index, projection := range definition.Projections {
		clone.Projections[index] = projection
		clone.Projections[index].CandidateProvider = projection.CandidateProvider
		clone.Projections[index].Accessor = cloneSymbol(projection.Accessor)
	}
	clone.Reducers = make([]Reducer, len(definition.Reducers))
	for index, reducer := range definition.Reducers {
		clone.Reducers[index] = reducer
		clone.Reducers[index].Inputs = append([]ReducerInput(nil), reducer.Inputs...)
		clone.Reducers[index].Outputs = append([]ReducerOutput(nil), reducer.Outputs...)
		clone.Reducers[index].Implementation = cloneSymbol(reducer.Implementation)
	}
	clone.CarryTransforms = make([]CarryTransform, len(definition.CarryTransforms))
	for index, transform := range definition.CarryTransforms {
		clone.CarryTransforms[index] = transform
		clone.CarryTransforms[index].Implementation = cloneSymbol(transform.Implementation)
	}
	return clone
}

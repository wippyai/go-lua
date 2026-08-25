// Package definition owns the neutral, owner-authored source form for one
// axis member vocabulary. It contains schema declarations and callback-free
// Go symbol descriptors only; execution choreography remains in rule.Program
// and its sealed plan.
package definition

import (
	"go/token"
	"path"
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
	// Pointer records that the reference is to the pointer form of the named
	// type. An axis whose schema is passed by pointer is a different parameter
	// from one passed by value, and a derived signature that could not tell
	// them apart would admit a call the compiler refuses.
	Pointer bool
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

// derivationOptional reports that a relation derives nothing at all - neither
// form is stated, so its rows come from somewhere else entirely.
func derivationOptional(derivation RelationDerivation) bool {
	return !derivation.AuthoredDerivation() && !derivation.DeclaredDerivation() && len(derivation.StaticAxes) == 0
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
	Inputs  []RelationInput
	// Axis is the axis whose rows these are. A relation over call coordinates
	// is call-axis data whichever rule declares it, so a contribution states
	// the axis per row rather than per contribution and the roster folds the
	// row into that axis's source. Left empty it is the contribution's own
	// axis, which is what a rule declaring rows of the axis it writes means.
	Axis schema.Key
	// CandidateProvider explicitly names the candidate authority this relation
	// is addressed through. It is required even when the provider is a
	// same-axis relation; no carrier-type inference is permitted. Its issued
	// arm names an issuance relation whose target rows are Program rows: there
	// is no owner directory to name, so the three dense-directory symbols
	// below are absent on that arm rather than optional.
	CandidateProvider member.CandidateRef
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
	// MemberParent, MemberCount and MemberAt declare this relation's nested
	// ordered member set: the bounded row list one PARENT row carries. They are
	// optional together, and a relation that declares them is one a vector read
	// spans as a whole denominator rather than one coordinate at a time.
	//
	// MemberParent names the relation whose rows are the parents. MemberCount
	// and MemberAt are direct methods on that parent's subject; MemberAt
	// answers one row of THIS relation, which the generator densifies through
	// this relation's own directory. A member-set relation is therefore
	// self-provided - its rows are addressed by its own directory - so the
	// coordinate a member projects to is reached the same way any other row's
	// is, and there is no second projection language for members.
	MemberParent member.RelationRef
	// MemberOrdinal is the carrier that keys the nested member set: the address
	// a member is reached by under its parent. It is declared with the rest of
	// the set, and it is what a CHILD Program consumes - the cold catalog row
	// carries the parent and the ordinal, so a consumer that never sees this
	// owner's Go symbols can still address its members.
	MemberOrdinal string
	MemberCount   GoSymbol
	MemberAt      GoSymbol
	// KeyVectorCount and KeyVectorAt declare that rows of THIS directory
	// publish an ordered dense key vector of another axis: the coordinates a
	// row was constructed from, in the order it holds them.
	//
	// It is the second way a whole-vector read gets its span. A nested member
	// set hangs off a parent row of the read's own axis and is enumerated
	// there; a constructor's operand vector has no such directory - the row
	// that knows which coordinates it consumes belongs to another axis, and
	// the read axis groups them nowhere. Publishing the vector here keeps both
	// halves with their owners: the row answers coordinates it already holds,
	// and the read axis resolves cells at coordinates it issued.
	//
	// Both are direct methods on this relation's own subject, declared
	// together or not at all - a span with no accessor addresses nothing, and
	// an accessor with no span is unbounded. At answers one dense coordinate
	// of the read axis at an ordinal.
	KeyVectorCount GoSymbol
	KeyVectorAt    GoSymbol
	// Correspondences name the foreign axis relations whose candidate orders
	// enumerate the same subjects this relation's own order does. They carry
	// no Go symbol: the correlation is determined by two directories that
	// already exist and are addressed by the same occurrence, and an owner
	// asked to answer it would be a third authority over it.
	Correspondences []member.RelationRef
	// Derivation is the optional typed construction of a dependent relation
	// row. It is invoked by generated composition code, never retained as a
	// runtime callback or owner handle.
	Derivation RelationDerivation
}

// memberSetDeclared reports whether this relation declares a nested member
// set. The three rows are one declaration: a parent with no accessor pair, or
// an accessor pair with no parent, states half of a set nothing can read.
func (relation Relation) memberSetDeclared() bool {
	return relation.MemberParent.Available() || relation.MemberOrdinal != "" ||
		!symbolOptional(relation.MemberCount) || !symbolOptional(relation.MemberAt)
}

// keyVectorDeclared reports whether rows of this relation publish an ordered
// dense key vector of another axis. The two accessors are one declaration for
// the same reason the member-set triple is: a span with no accessor addresses
// nothing.
func (relation Relation) keyVectorDeclared() bool {
	return !symbolOptional(relation.KeyVectorCount) || !symbolOptional(relation.KeyVectorAt)
}

// keyVectorComplete validates a declared key vector. Both accessors must be
// present and be methods on this relation's own subject - the row that holds
// the coordinates - because a directory publishes the vector its own rows
// carry and no other.
func (relation Relation) keyVectorComplete(carriers map[string]Carrier) bool {
	if !relation.keyVectorDeclared() {
		return true
	}
	if !relation.KeyVectorCount.Available() || !relation.KeyVectorAt.Available() {
		return false
	}
	subject, subjectOK := carriers[relation.Subject]
	if !subjectOK {
		return false
	}
	return sameType(relation.KeyVectorCount.Receiver, subject.Type) && sameType(relation.KeyVectorAt.Receiver, subject.Type)
}

// memberSetComplete validates a declared member set against the relations it
// names. The parent must be a declared relation of this axis, this relation
// must be self-provided so its members are addressed by its own directory, and
// both accessors must be methods on the parent's subject.
func (relation Relation) memberSetComplete(relations map[string]Relation, byKey map[schema.Key]Relation, carriers map[string]Carrier) bool {
	if !relation.memberSetDeclared() {
		return true
	}
	if !relation.MemberParent.Available() || !relation.MemberCount.Available() || !relation.MemberAt.Available() ||
		relation.MemberOrdinal == "" {
		return false
	}
	if _, ordinalOK := carriers[relation.MemberOrdinal]; !ordinalOK {
		return false
	}
	if relation.CandidateProvider.AxisRelation.Member != relation.Key {
		return false
	}
	if symbolOptional(relation.CandidateOrdinal) || symbolOptional(relation.CandidateAt) {
		return false
	}
	parent, parentOK := byKey[relation.MemberParent.Member]
	if !parentOK || parent.Key == relation.Key {
		return false
	}
	parentSubject, parentSubjectOK := carriers[parent.Subject]
	if _, subjectOK := carriers[relation.Subject]; !parentSubjectOK || !subjectOK {
		return false
	}
	_ = relations
	return sameType(relation.MemberCount.Receiver, parentSubject.Type) && sameType(relation.MemberAt.Receiver, parentSubject.Type)
}

// RelationInput is one carrier a dependent relation's derivation consumes.
//
// Many says the input arrives as the ordered CELLS of a selected join rather
// than as one carrier value. A derivation over a whole selection - which is
// what a route set computed from every mounted actual is - cannot be handed
// one member at a time without asking it to rebuild the correlation the read
// already established, so the delivery is declared here and the derived Build
// signature carries it.
//
// The delivery is owner-local on purpose. It says how THIS axis's Build is
// called, not how another axis addresses these rows, so it is not a cold
// catalog row: a child consuming the relation still addresses it by candidate
// and projection exactly as before.
type RelationInput struct {
	Carrier string
	Many    bool
	// Form is the read form the delivery arrives under, and it is declared
	// exactly when Many is. It is what decides the view a many-valued position
	// takes: a selection hands over tagged cells, and a whole-vector read over
	// a closed denominator hands over one vector whose positions are its
	// denominator. The two establish different facts, so the delivery is
	// stated here rather than inferred from the carrier.
	Form member.ReadForm
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

	// Source, Resolve and Widen are the DECLARED form of the same derivation.
	// A relation states them instead of State/Build/Count/At, and the emitter
	// writes the construction from them.
	//
	// What they replace is not the judgment - that stays authored, as Resolve
	// - but everything every hand-written Build did around it: enumerating a
	// source, unioning the rows, widening to a directory at a lattice
	// endpoint, ordering by the coordinate the rows are read at, and refusing
	// a repeat. All six authored Builds wrote those five by hand, identically,
	// and a mistake in any of them is a soundness mistake.
	Source  []EnumerationRef
	Resolve GoSymbol
	Widen   DerivationWiden
	// InlineWidth is how many rows the generated set holds BY VALUE before it
	// reaches its explicit spill. The generated construction is the shape every
	// authored one converged on independently - a bounded inline prefix, a
	// spill suffix, a count - so the ordinary answer never allocates a slice
	// just to be returned, and the width past which it does is the relation's
	// own statement of how many members it ordinarily answers.
	InlineWidth int
}

// EnumerationRef names one axis's declared enumeration.
type EnumerationRef struct {
	Axis schema.EntryReference
	Name string
}

// Available reports whether this reference names an enumeration at all.
func (reference EnumerationRef) Available() bool {
	return reference.Axis.Surface == schema.SurfaceKindAxis && reference.Axis.Key.Available() && identifierAvailable(reference.Name)
}

// DerivationWiden is the lattice endpoint at which a derived set stops being
// enumerable and becomes a whole declared directory.
//
// Every authored derivation had one: a Top or opaque fact denotes alternatives
// the read did not observe, and the sound answer is the whole directory rather
// than the alternatives that happen to be written down. Declaring it puts that
// answer where it can be read, instead of inside a Build where each one spelled
// it differently.
type DerivationWiden struct {
	// Predicate answers, of the source fact, whether the set is beyond
	// enumeration. It is the owner's own statement - IsTop, HasOpaque - and
	// never a shape this package guesses from the carrier.
	Predicate GoSymbol
	// Source is what the widened answer is read out of. It is an enumeration
	// like any other, and it is read out of an axis's SCHEMA rather than out
	// of the fact: what "everything" means at a lattice endpoint is the
	// owner's whole directory, which the fact by definition failed to name.
	Source []EnumerationRef
}

// Declared reports whether a widen endpoint is stated.
func (widen DerivationWiden) Declared() bool {
	return widen.Predicate.Available() || len(widen.Source) != 0
}

func (widen DerivationWiden) complete() bool {
	if !widen.Predicate.Available() || len(widen.Source) == 0 {
		return false
	}
	for _, source := range widen.Source {
		if !source.Available() {
			return false
		}
	}
	return true
}

// Enumeration is one axis's statement of how a sequence is read out of one of
// its own carriers: the atoms of a fact, the allocations a fact projects to,
// the targets a call value dispatches to.
//
// It is declared on the AXIS rather than on a rule, because how an owner's
// value decomposes is the owner's answer and every rule that decomposes it the
// same way is asking the same question. Two rules sourcing from one
// enumeration name it once each and share the owner's two symbols.
type Enumeration struct {
	Name string
	// Over is the carrier a sequence is read out of. An EMPTY Over is a
	// statement, not an omission: the sequence is read out of the axis's own
	// SCHEMA. That is what a directory is - every row the owner has, answered
	// by the owner rather than by a value - and it is what a derivation widens
	// to when its source fact reaches a lattice endpoint.
	Over string
	// Item is the carrier of one element of the sequence.
	Item  string
	Count GoSymbol
	At    GoSymbol
}

// OverSchema reports that this enumeration reads its sequence out of the
// axis's own schema rather than out of one of its carriers.
func (enumeration Enumeration) OverSchema() bool { return enumeration.Over == "" }

func (enumeration Enumeration) complete() bool {
	return identifierAvailable(enumeration.Name) && identifierAvailable(enumeration.Item) &&
		enumeration.Count.Available() && enumeration.At.Available()
}

// DeclaredDerivation reports whether a relation states the declared form.
func (derivation RelationDerivation) DeclaredDerivation() bool {
	return len(derivation.Source) != 0 || derivation.Resolve.Available() || derivation.Widen.Declared()
}

// AuthoredDerivation reports whether a relation states the authored form: the
// State/Build/Count/At quartet that the declared form replaces and that the
// scheduled-death ledger admits one row at a time.
func (derivation RelationDerivation) AuthoredDerivation() bool {
	return derivation.State.Available() || derivation.Build.Available() ||
		derivation.Count.Available() || derivation.At.Available()
}

// declaredComplete states the row-local law of the declared form: at least one
// source enumeration, the one authored judgment that resolves an item, the
// width the generated set holds by value, a widen endpoint stated whole or not
// at all, and at least one axis to resolve against.
func (derivation RelationDerivation) declaredComplete() bool {
	if len(derivation.Source) == 0 || !derivation.Resolve.Available() || len(derivation.StaticAxes) == 0 {
		return false
	}
	if derivation.InlineWidth <= 0 {
		return false
	}
	for _, source := range derivation.Source {
		if !source.Available() {
			return false
		}
	}
	if derivation.Widen.Declared() && !derivation.Widen.complete() {
		return false
	}
	return staticAxesDistinct(derivation.StaticAxes)
}

func staticAxesDistinct(axes []schema.EntryReference) bool {
	seen := make(map[schema.Key]struct{}, len(axes))
	for _, axis := range axes {
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

// complete states the row-local law of a derivation in whichever form it is
// declared. The two forms are exclusive: a relation that states both is two
// answers to how its rows are built, and which one a consumer reads would
// decide the rows.
func (derivation RelationDerivation) complete() bool {
	authored, declared := derivation.AuthoredDerivation(), derivation.DeclaredDerivation()
	if authored == declared {
		return false
	}
	if declared {
		return derivation.declaredComplete()
	}
	if !derivation.State.Available() || !derivation.Build.Available() || !derivation.Count.Available() || !derivation.At.Available() || len(derivation.StaticAxes) == 0 {
		return false
	}
	return staticAxesDistinct(derivation.StaticAxes)
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
	CandidateProvider member.CandidateRef
	// Axis is the axis whose rows this projection reads, declared under the
	// same law as a relation's. A projection names a relation, so it folds into
	// the source its relation does.
	Axis schema.Key
}

// ReducerInput is one named reducer input. Carrier, Tag and Route refer to
// carrier names; an empty Tag is the untagged spelling, and an empty Route is
// the unrouted one. Whether a Selected read is tagged or routed is stated by
// the Program that reads it, so both are optional here.
type ReducerInput struct {
	Axis         schema.EntryReference
	Carrier      string
	Form         member.ReadForm
	Multiplicity member.Multiplicity
	Tag          string
	Route        string
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
	Candidate string
	Inputs    []ReducerInput
	Outputs   []ReducerOutput
	// Structural marks a fold that publishes no fact: its whole result is the
	// disposition of the branch it was invoked for. It declares no output
	// carrier, and the marker is what tells that apart from an ordinary
	// reducer whose output was simply left out.
	Structural bool
	// Derivation is the optional sealed state this reducer's judgment is
	// issued by. A fold whose answer rests on its axes' cold schemas cannot
	// take them as parameters - that is what keeps a call shape from growing
	// plumbing - so it names the state those schemas are sealed into once, and
	// Implementation is a method on that state. A reducer that needs no cold
	// schema declares nothing here and stays a free function over carriers.
	Derivation     ReducerDerivation
	Implementation GoSymbol
}

// ReducerDerivation is the install-time construction of one reducer's sealed
// state. It is the same construction a dependent relation declares, narrowed
// to the one thing a fold needs: Build answers State from the schemas of its
// ordered static axes, the installed family holds that State, and every
// invocation calls Implementation on it.
//
// It is built once per family, never per row and never per invocation: the
// schemas it seals are immutable for the life of the binding, so a state
// rebuilt on an invocation path would be the same answer allocated again.
type ReducerDerivation struct {
	State      GoType
	Build      GoSymbol
	StaticAxes []schema.EntryReference
}

// Declared reports whether a reducer names a sealed state at all.
func (derivation ReducerDerivation) Declared() bool {
	return derivation.State.Available() || derivation.Build.Available() || len(derivation.StaticAxes) != 0
}

// complete states the row-local law of a declared reducer state: a state type,
// the symbol that seals it, and at least one axis whose schema it is sealed
// from, each axis named once.
func (derivation ReducerDerivation) complete() bool {
	if !derivation.State.Available() || !derivation.Build.Available() || len(derivation.StaticAxes) == 0 {
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
	Carrier string
	// Dense is the builtin width the axis's dense Factor coordinate occupies.
	// The coordinate type itself is not authored here: the generator publishes
	// one named type per axis over this width, so an owner never hand-exports
	// a Factor key type and a consumer of another axis never erases one to
	// uint32 in order to name it.
	Dense      GoType
	Normalizer GoSymbol
}

// denseWidthAvailable reports whether a declared dense width is one of the two
// builtin widths a Factor coordinate is defined over. A qualified type here
// would be a hand-exported coordinate, which is the thing generation replaces.
func denseWidthAvailable(width GoType) bool {
	return width.PackagePath == "" && (width.Name == "uint32" || width.Name == "uint64")
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
	// Enumerations are this axis's statements of how a sequence is read out of
	// one of its own carriers. A rule's derivation names one rather than
	// carrying its own pair of symbols, so two rules decomposing one carrier
	// the same way ask the owner once.
	Enumerations []Enumeration

	// ImportPath is the Go package this axis's cold catalog is generated into.
	// It is the one place an axis says where it lives: every other package the
	// generator writes into is a subdirectory of it, named by the path below,
	// and every consumer that has to SPELL a generated symbol - a law suite
	// naming the cold catalog, an emitted family typing a primitive at the
	// dense coordinate - reads it from here rather than guessing it off a
	// carrier's package.
	ImportPath string

	// RelationsPackage and RelationsPath, when set, say that this axis's
	// generated bind-time relation owner lives apart from its cold catalog:
	// at a different package (the fact type's own algebra reaches a
	// dependency the cold catalog's importers must not also reach) and a
	// path relative to the cold catalog's directory. Both empty is the
	// default: the relation owner is generated beside the cold catalog, in
	// its package.
	RelationsPackage string
	RelationsPath    string
}

// ColdImportPath is the import path of the package this axis's cold catalog is
// generated into. It is the ONE path an axis authors: the package clause, the
// relation owner's package, and the dense coordinate's package are all read
// off it, so a consumer never learns a second spelling of where an axis lives.
//
// An axis that names none is refused HERE, by the consumer that needs to spell
// one of its generated symbols, rather than by the vocabulary law: a
// definition is internally consistent without knowing where it will be
// written, and only a generator's consumer has to know.
func ColdImportPath(source Definition) (string, bool) {
	if source.ImportPath == "" {
		return "", false
	}
	return source.ImportPath, true
}

// RelationsImportPath is the import path of the package the bind-time relation
// owner is generated into: the axis's own package, or the subdirectory
// RelationsPath names when the owner lives apart from the cold catalog.
func RelationsImportPath(source Definition) (string, bool) {
	cold, coldOK := ColdImportPath(source)
	if !coldOK {
		return "", false
	}
	if source.RelationsPath == "" {
		return cold, true
	}
	directory := path.Dir(source.RelationsPath)
	if directory == "." {
		return cold, true
	}
	return cold + "/" + directory, true
}

// DenseCoordinateType is the Go type one axis's dense Factor coordinate is
// spelled as. The member generator publishes it beside the relation owner, so
// it is named in whichever package that owner is generated into. One
// statement, read by everything that types a primitive at this axis.
func DenseCoordinateType(source Definition, name string) (GoType, bool) {
	relations, relationsOK := RelationsImportPath(source)
	if !relationsOK {
		return GoType{}, false
	}
	return GoType{PackagePath: relations, Name: name}, true
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
		for inputIndex, declared := range relation.Inputs {
			inputName := declared.Carrier
			input, inputOK := carriers[inputName]
			if !inputOK {
				return member.Catalog{}, false
			}
			inputs[inputIndex] = input.Key
		}
		row := member.Relation{Key: relation.Key, Subject: subject.Key, Inputs: inputs, CandidateProvider: relation.CandidateProvider}
		// A correspondence survives into the cold catalog for the same reason a
		// nested set does: it is what a CHILD Program consumes to know that a
		// foreign candidate addresses these rows, and a consumer that never
		// sees this owner's source names must still be able to read it.
		row.Correspondences = cloneCorrespondences(relation.Correspondences)
		// A nested member set survives into the cold catalog. The parent and the
		// ordinal carrier are what a CHILD Program consumes to address an
		// owner's members, and dropping them here would leave the set visible
		// only through Go symbols the child cannot reach.
		if relation.memberSetDeclared() {
			ordinal, ordinalOK := carriers[relation.MemberOrdinal]
			if !ordinalOK {
				return member.Catalog{}, false
			}
			row.Parent, row.Ordinal = relation.MemberParent, ordinal.Key
		}
		// A published key vector survives for the same reason and in the same
		// currency: a child Program needs to know a row of this directory
		// carries the span a vector read over another axis is taken over, and
		// the accessors that answer it are this owner's.
		row.PublishesKeyVector = relation.keyVectorDeclared()
		relations[index] = row
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
		// A structural fold declares no output carrier: its whole result is the
		// disposition of the branch it was invoked for. Every other fold owes
		// one, so the exception is the marker's rather than an empty list's.
		if !identifierAvailable(reducer.Name) || !reducer.Key.Available() ||
			(len(reducer.Outputs) == 0) != reducer.Structural {
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
			var routed member.Carrier
			if input.Route != "" {
				route, routeOK := carriers[input.Route]
				if !routeOK {
					return member.Catalog{}, false
				}
				routed = route.Key
			}
			inputs[inputIndex] = member.ReducerInput{Axis: input.Axis, Carrier: carrier.Key, Form: input.Form, Multiplicity: input.Multiplicity, Tag: tag, Route: routed}
		}
		outputs := make([]member.ReducerOutput, len(reducer.Outputs))
		for outputIndex, output := range reducer.Outputs {
			carrier, carrierOK := carriers[output.Carrier]
			if !carrierOK {
				return member.Catalog{}, false
			}
			outputs[outputIndex] = member.ReducerOutput{Axis: output.Axis, Carrier: carrier.Key}
		}
		reducers[index] = member.Reducer{Key: reducer.Key, Inputs: inputs, Outputs: outputs, Structural: reducer.Structural}
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
	return keyOK && denseWidthAvailable(binding.Key.Dense) && binding.Key.Normalizer.Available() && keyCarrier.Type.Available()
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
	// Where the relation owner is generated is one statement in two parts: the
	// package clause and the file. Whether the axis names its own package at
	// all is not a vocabulary law - a definition is internally consistent
	// without it - so it is answered by the consumers that have to SPELL a
	// generated symbol, and by the roster law that every composed axis does.
	if (definition.RelationsPackage != "") != (definition.RelationsPath != "") {
		return false
	}
	relations := make(map[string]Relation, len(definition.Relations))
	relationsByKey := make(map[schema.Key]Relation, len(definition.Relations))
	owner := definition.Binding.Key.Normalizer.Receiver
	for _, relation := range definition.Relations {
		// A row states the axis whose rows it is. A definition holds exactly
		// the rows of its own axis, so a row that names another one has been
		// folded into the wrong source and is refused where the vocabulary
		// seals rather than discovered when a plan cannot resolve it.
		if relation.Axis.Available() && relation.Axis != definition.Axis {
			return false
		}
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
			if !relation.Derivation.complete() || relation.CandidateProvider.AxisRelation.Axis.Key == definition.Axis && relation.CandidateProvider.AxisRelation.Member == relation.Key ||
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
			// Two independent obligations need a census, and only one of them
			// is materialization. A global directory owes the count that bounds
			// the occurrence inventory a Link rule enumerates from it, whether
			// or not any fact is materialized from its rows; every other
			// relation with no materializer owes no width at all.
			if !countOptional && (symbolOptional(relation.CandidateIdentityAt) || !sameOwnerSymbol(relation.CandidateCount, owner)) {
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
		// One name is one relation and one key is one relation. Without this
		// the later row silently wins both lookups, and every provider that
		// names the shared key resolves to whichever declaration happened to be
		// composed last.
		if _, duplicate := relations[relation.Name]; duplicate {
			return false
		}
		if _, duplicate := relationsByKey[relation.Key]; duplicate {
			return false
		}
		relations[relation.Name] = relation
		relationsByKey[relation.Key] = relation
	}
	for _, relation := range definition.Relations {
		if !relation.memberSetComplete(relations, relationsByKey, carriers) || !relation.keyVectorComplete(carriers) {
			return false
		}
		// An authored derivation is admitted only while the migration set knows
		// about it. Registration is not a formality: the ledger is what says the
		// authored form is scheduled to be emitted, and a derivation nothing
		// scheduled would outlive the migration silently.
		// The ledger admits the AUTHORED form, which is the one scheduled to be
		// replaced. A declared derivation has no Build to schedule: its
		// construction is generated, so there is nothing for a migration set to
		// know about and nothing to outlive it.
		if relation.Derivation.AuthoredDerivation() && !scheduledForDeath(definition.Axis, relation.Key, relation.Derivation.Build) {
			return false
		}
		// An issued provider names no axis directory at all. The three dense
		// symbols are absent on that arm rather than optional, and the relation
		// is addressed through the Program row the rule was issued for.
		if relation.CandidateProvider.Issued() {
			if !symbolOptional(relation.CandidateResolver) || !symbolOptional(relation.CandidateOrdinal) ||
				!symbolOptional(relation.CandidateAt) || !symbolOptional(relation.CandidateCount) ||
				!symbolOptional(relation.Materialize) || !symbolOptional(relation.CandidateIdentityAt) {
				return false
			}
			continue
		}
		if relation.CandidateProvider.AxisRelation.Axis.Key != definition.Axis {
			// Foreign ownership is resolved against the composition roster.
			// The consumer definition must not retain a second owner directory.
			if !symbolOptional(relation.CandidateResolver) || !symbolOptional(relation.CandidateOrdinal) || !symbolOptional(relation.CandidateAt) || !symbolOptional(relation.CandidateCount) || !symbolOptional(relation.Materialize) || !symbolOptional(relation.CandidateIdentityAt) {
				return false
			}
			continue
		}
		provider, providerOK := relationsByKey[relation.CandidateProvider.AxisRelation.Member]
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
		for _, declaredInput := range relation.Inputs {
			inputName := declaredInput.Carrier
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
	projectionNames := make(map[string]struct{}, len(definition.Projections))
	projectionKeys := make(map[schema.Key]struct{}, len(definition.Projections))
	for _, projection := range definition.Projections {
		if projection.Axis.Available() && projection.Axis != definition.Axis {
			return false
		}
		if !projection.CandidateProvider.Available() {
			return false
		}
		if !projection.Accessor.Available() {
			return false
		}
		// A projection is addressed by name and sealed by key on the same terms
		// a relation is.
		if _, duplicate := projectionNames[projection.Name]; duplicate {
			return false
		}
		if _, duplicate := projectionKeys[projection.Key]; duplicate {
			return false
		}
		projectionNames[projection.Name] = struct{}{}
		projectionKeys[projection.Key] = struct{}{}
		relation, relationOK := relations[projection.Relation]
		if !relationOK || relation.CandidateProvider != projection.CandidateProvider || !projectionReceiverMatches(projection.Accessor, relation, carriers) {
			return false
		}
		if !identityRoleAgrees(projection, carriers) {
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
		// A declared state and the method that reads it are one statement: the
		// implementation is issued by the state, so a state with a free
		// function beside it, or a receiver with no state behind it, names
		// half a fold.
		if reducer.Derivation.Declared() != (reducer.Implementation.Receiver.Name != "") {
			return false
		}
		if reducer.Derivation.Declared() &&
			(!reducer.Derivation.complete() || !sameType(reducer.Derivation.State, reducer.Implementation.Receiver)) {
			return false
		}
	}
	for _, transform := range definition.CarryTransforms {
		if !transform.Implementation.Available() {
			return false
		}
	}
	return true
}

// identityPackagePath is the analyzer's one identity tree. A carrier from any
// other package is a value this analyzer minted and addresses by a local.
const identityPackagePath = "github.com/wippyai/go-lua/analysis/identity"

// IdentityCarrier answers whether one carrier is an owner-issued identity, and
// whether it carries the frame it was issued under.
//
// The analyzer mints exactly two things shaped like an identity: a content
// identity, which is a digest issued under no frame, and a semantic key, which
// is that digest and the frame its owner minted it at. Those are the two the
// owner's ProjectIdentity surface can answer, so they are the two a projection
// in the Identity role may publish.
func IdentityCarrier(typ GoType) (framed bool, issued bool) {
	if typ.PackagePath != identityPackagePath || typ.Pointer {
		return false, false
	}
	switch typ.Name {
	case "ContentID":
		return false, true
	case "SemanticKey":
		return true, true
	default:
		return false, false
	}
}

// identityRoleAgrees holds a projection's role and its result carrier to one
// statement. The Identity role publishes an owner-issued identity and every
// other role publishes a local, so the two questions have one answer and a
// declaration that disagrees with itself is refused where it is written.
//
// The converse half is what keeps Attribute's own statement true: an identity
// spelled in a local role would be read through Project and truncated to a
// coordinate of a directory it was never an index into.
func identityRoleAgrees(projection Projection, carriers map[string]Carrier) bool {
	result, resultOK := carriers[projection.Result]
	if !resultOK {
		return false
	}
	_, issued := IdentityCarrier(result.Type)
	return issued == (projection.Role == member.Identity)
}

func projectionReceiverMatches(accessor GoSymbol, relation Relation, carriers map[string]Carrier) bool {
	if accessor.Receiver.Name == "" {
		return false
	}
	if subject, ok := carriers[relation.Subject]; ok && sameType(subject.Type, accessor.Receiver) {
		return true
	}
	for _, declaredInput := range relation.Inputs {
		if input, ok := carriers[declaredInput.Carrier]; ok && sameType(input.Type, accessor.Receiver) {
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
		clone.Relations[index].Inputs = append([]RelationInput(nil), relation.Inputs...)
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
		clone.Reducers[index].Derivation.Build = cloneSymbol(reducer.Derivation.Build)
		clone.Reducers[index].Derivation.StaticAxes = append([]schema.EntryReference(nil), reducer.Derivation.StaticAxes...)
	}
	clone.CarryTransforms = make([]CarryTransform, len(definition.CarryTransforms))
	for index, transform := range definition.CarryTransforms {
		clone.CarryTransforms[index] = transform
		clone.CarryTransforms[index].Implementation = cloneSymbol(transform.Implementation)
	}
	return clone
}

// ArgumentRole names what one position of a reducer's direct call carries.
// The roles below are the whole vocabulary: every one of them is a carrier
// value the declaration named, and there is deliberately no role for an owner
// schema, a derived route plan, a projection, or an ordinal.
type ArgumentRole uint8

const (
	// ArgumentCandidate is the optional owner-issued candidate carrier, which
	// always precedes the inputs when present.
	ArgumentCandidate ArgumentRole = iota + 1
	// ArgumentRoute is the destination coordinate of a routed input: the route
	// join's Destination projection result. It is how a routed fold learns
	// which coordinate it is publishing at, rather than resolving a plan of its
	// own. It precedes that input's tag.
	ArgumentRoute
	// ArgumentTag is the tag carrier of a tagged input. It is how a fold learns
	// which member of a selection it was handed - the route member's own
	// carrier value - rather than an engine-supplied projection or index.
	ArgumentTag
	// ArgumentFact is one declared input's fact carrier, delivered under that
	// input's sealed read contract.
	ArgumentFact
	// ArgumentVector is the whole delivered cell vector of a many-valued
	// input. A Summary or Complete read answers one row with every cell of its
	// sealed denominator, and a fold over such a read is a fold over the
	// vector: decomposing it into one invocation per cell would ask the fold
	// to reassemble a correlation the read already established, and there is
	// no per-cell invocation for it to be handed. It is a derived role, not an
	// authored one - which inputs are many-valued is the read's multiplicity,
	// not an owner's choice of parameter.
	//
	// WHICH view it is delivered through is the read's Form, not the role: a
	// whole-vector read establishes each cell's position and nothing else, and
	// a selection establishes the tag it correlated each cell by.
	// ManyValuedView is the one place that choice is made.
	ArgumentVector
	// ArgumentBranch is the ordinal of the candidate branch one STRUCTURAL
	// invocation is settling. It is how such a fold learns which branch it was
	// asked about - a structural row is invoked once per branch of the set its
	// declaration names, and that set is enumerated rather than read, so there
	// is no cell and no tag carrier to identify a branch by.
	//
	// It is a derived role, not an authored one: whether a fold is structural
	// is its publication's statement, not an owner's choice of parameter. It
	// follows the candidate and precedes every declared input.
	ArgumentBranch
)

// Argument is one position of a reducer's derived direct-call signature.
type Argument struct {
	Role ArgumentRole
	Type GoType
	// Element is the cell type a vector position delivers. It is the one place
	// the call shape names a type argument, because the vector view is the one
	// instantiated type in a signature; every other role leaves it zero.
	Element GoType
	// Slice says the delivery is a slice of that view rather than one value of
	// it. A selection hands over the cells it observed, which is a slice; a
	// whole-vector read hands over one vector.
	Slice bool
	Input int
}

// ArgumentInput is one declared input reduced to the carrier types its
// positions carry. It exists so the ordering rule below has exactly one
// statement, shared by the name-resolving derivation here and by the
// address-resolving one in the rule codegen model.
type ArgumentInput struct {
	Route  GoType
	Routed bool
	Tag    GoType
	Tagged bool
	Fact   GoType
	// Vector is the view type a many-valued read delivers its cells through,
	// and Many says this input is one. The view is named by the caller because
	// it belongs to the execution layer's vocabulary, the same way the sealed
	// disposition is.
	Vector GoType
	Slice  bool
	Many   bool
}

// ManyValuedView answers the view one many-valued delivery is handed through,
// and whether it arrives as a slice of that view.
//
// It is the ONE statement of that choice. A reducer folding a many-valued
// input and a relation derived over the same one are handed the same delivery,
// so both read this rather than each deciding for itself - which is how the
// two came to disagree, a reducer seeing a bare vector where its derivation
// saw tagged cells.
//
// The choice is the read's Form and nothing else. A SELECTION established a
// tag per cell, and dropping it would ask the fold to recover a correlation
// the read already proved; a WHOLE-VECTOR read over a closed denominator
// established each cell's position and no tag at all, so there is none to
// carry. Both views are named by the caller for the same reason the sealed
// disposition is: they belong to the execution layer's vocabulary.
func ManyValuedView(form member.ReadForm, cell, vector GoType) (view GoType, slice bool, ok bool) {
	switch form {
	case member.ReadFormSelected:
		if !cell.Available() {
			return GoType{}, false, false
		}
		return cell, true, true
	case member.ReadFormSummary:
		if !vector.Available() {
			return GoType{}, false, false
		}
		return vector, false, true
	default:
		return GoType{}, false, false
	}
}

// ComposeArguments is the one statement of a reducer's parameter order: the
// optional candidate carrier first, then for each input its route coordinate
// when routed, its tag carrier when tagged, and the input itself - one fact
// when the read delivers one member, one vector when it delivers a whole
// denominator. A route precedes the tag and the tag precedes the input,
// outermost address first: the route says where the invocation publishes, the
// tag says which member of the selection it folds, and the input is that
// member.
//
// A many-valued input takes exactly one position and it carries no tag. Its
// members are identified by their sealed position in the delivered vector -
// that order IS the denominator the read declared - so a tag naming one member
// has nothing to name in a delivery that carries them all.
func ComposeArguments(candidate GoType, candidatePresent bool, inputs []ArgumentInput) []Argument {
	return composeArguments(candidate, candidatePresent, false, inputs)
}

// ComposeStructuralArguments is ComposeArguments for a fold that settles one
// candidate branch per invocation. The branch ordinal is the one thing such a
// call carries that an ordinary fold does not, because a structural row's
// branch set is enumerated and its members reach the fold as an address rather
// than as a cell.
func ComposeStructuralArguments(candidate GoType, candidatePresent bool, inputs []ArgumentInput) []Argument {
	return composeArguments(candidate, candidatePresent, true, inputs)
}

func composeArguments(candidate GoType, candidatePresent, structural bool, inputs []ArgumentInput) []Argument {
	arguments := make([]Argument, 0, len(inputs)*3+2)
	if candidatePresent {
		arguments = append(arguments, Argument{Role: ArgumentCandidate, Type: candidate, Input: -1})
	}
	if structural {
		arguments = append(arguments, Argument{Role: ArgumentBranch, Type: GoType{Name: "uint64"}, Input: -1})
	}
	for index, input := range inputs {
		if input.Many {
			arguments = append(arguments, Argument{Role: ArgumentVector, Type: input.Vector, Element: input.Fact, Slice: input.Slice, Input: index})
			continue
		}
		if input.Routed {
			arguments = append(arguments, Argument{Role: ArgumentRoute, Type: input.Route, Input: index})
		}
		if input.Tagged {
			arguments = append(arguments, Argument{Role: ArgumentTag, Type: input.Tag, Input: index})
		}
		arguments = append(arguments, Argument{Role: ArgumentFact, Type: input.Fact, Input: index})
	}
	return arguments
}

// ReducerSignature derives the complete direct-call signature one declared
// reducer must have: its parameter vector and its result vector. It is the one
// statement of the call shape, so the emitter, the generator and the laws that
// fence a fold all read the same derivation.
//
// The parameters are carrier values only - the optional candidate carrier, then
// for each declared input its route coordinate when routed, its tag carrier
// when tagged, and the input itself: one fact carrier, or one vector view of
// that carrier when the read is many-valued. Nothing else is ever a parameter.
// The owner schema, the derived route plan and the projections a fold consults
// are the sealed state of the installed Family that calls the reducer, bound
// once when the owner installs it. That is what keeps a signature from growing
// plumbing: its width is a function of the declared rows alone.
//
// cell and vector are the two views a many-valued read delivers through, and
// which of them one input takes is ManyValuedView's answer from that input's
// declared Form - the same answer a relation derived over the input gets. They
// are supplied by the caller for the same reason outcome is: all three belong
// to the execution vocabulary, and this package states the shape without
// naming that package.
//
// The results are the declared output carriers in row order followed by the one
// sealed disposition, which a caller supplies as outcome so this package states
// the shape without naming the vocabulary's package.
func (definition Definition) ReducerSignature(reducer Reducer, outcome, cell, vector GoType) ([]Argument, []GoType, bool) {
	carriers, _, carriersOK := definition.carrierIndex()
	if !carriersOK {
		return nil, nil, false
	}
	var candidate Carrier
	if reducer.Candidate != "" {
		var candidateOK bool
		candidate, candidateOK = carriers[reducer.Candidate]
		if !candidateOK {
			return nil, nil, false
		}
	}
	inputs := make([]ArgumentInput, len(reducer.Inputs))
	for index, input := range reducer.Inputs {
		fact, factOK := carriers[input.Carrier]
		if !factOK {
			return nil, nil, false
		}
		inputs[index] = ArgumentInput{Fact: fact.Type}
		if input.Multiplicity == member.MultiplicityMany {
			view, slice, viewOK := ManyValuedView(input.Form, cell, vector)
			if !viewOK {
				return nil, nil, false
			}
			inputs[index].Vector, inputs[index].Slice, inputs[index].Many = view, slice, true
			continue
		}
		if input.Route != "" {
			route, routeOK := carriers[input.Route]
			if !routeOK {
				return nil, nil, false
			}
			inputs[index].Route, inputs[index].Routed = route.Type, true
		}
		if input.Tag != "" {
			tag, tagOK := carriers[input.Tag]
			if !tagOK {
				return nil, nil, false
			}
			inputs[index].Tag, inputs[index].Tagged = tag.Type, true
		}
	}
	arguments := composeArguments(candidate.Type, reducer.Candidate != "", reducer.Structural, inputs)
	results := make([]GoType, 0, len(reducer.Outputs)+1)
	for _, output := range reducer.Outputs {
		carrier, carrierOK := carriers[output.Carrier]
		if !carrierOK {
			return nil, nil, false
		}
		results = append(results, carrier.Type)
	}
	return arguments, append(results, outcome), true
}

// Package library owns the library contract surface of the analyzer
// declaration table: the KINDS of serialized, mount-bound contract artifact a
// library or an environment may be published as, and the surface laws the
// declaration root seals those kinds under.
//
// A library contract is a serialized artifact whose signatures, intrinsic
// markers, effect labels and metatable edges attach to EXPORTED VALUES, never
// to names. The distinction is the whole point of the surface. Today the
// analyzer addresses library members by dotted global name three times over:
// target.BindingSpec carries an owner/member name path, the stdlib registry is
// keyed by "string.sub", and stdlib.MethodProvider recovers a contract by
// rebuilding the name "string."+method from a receiver type. Under name
// addressing, `local f = string.len` loses the contract that made f callable,
// while `string.len = print` inherits one it was never given. Under value
// addressing both are correct by construction: the contract rides the exported
// value, aliasing keeps it, and rebinding the slot does not confer it.
//
// The environment contract is a specialization on the same algebra, not a
// second algebra. It declares every form a library kind declares and adds the
// four forms only the environment may own: the boot roots, the
// environment-slot bindings, the primitive metatable attachments, and the host
// capabilities. An individual library therefore cannot declare a form that
// would let it mutate the global environment, and the surface states that as a
// law rather than as a convention.
//
// What this surface does NOT own: contract instances. A contract instance is
// mount-time data, and its mount identity is Link-local; no mount identity
// appears here or anywhere else in the process-global declaration table. The
// schema owns the codec identity and the validation law-set identity a kind is
// checked under; the instances, the member types, and the export graphs live
// with their owners.
//
// The consumer cutover is not performed here. The four shapes this surface
// will absorb - the frozen target-operation catalogue, the boot ledger, the
// stdlib signature registry, and the string-library method table - live in
// packages this surface may not edit, so replacing them with projections of a
// declared contract kind is a following lane. Declaring the kinds without
// cutting the consumers over leaves the name addressing visible rather than
// hidden, which is the honest intermediate state.
//
// Nothing registers itself: declarations are values, handed to the table at
// composition.
package library

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Content record markers. They separate the parts one kind writes, so a member
// row can never be read as the validation reference.
const (
	contentRecordValidation uint64 = iota + 1
	contentRecordMember
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = schema.SurfaceLawFloor + iota
	LawContractIdentity
	LawClassDeclared
	LawCodecDeclared
	LawCodecVersioned
	LawCodecUnique
	LawValidationDeclared
	LawValidationPhase
	LawValidationResolves
	LawValidationDeferred
	LawMemberFormDeclared
	LawMemberFormUnique
	LawMemberFormComplete
	LawEnvironmentExclusive
	LawAddressingDeclared
	LawAddressingProvenance
	LawClassPopulated
)

// Class is the closed catalog of contract kinds this surface declares. The
// environment class is a specialization of the library class over the same
// member-form algebra: it declares every base form and adds the environment's
// own.
type Class uint8

const (
	ClassInvalid Class = iota
	// ClassLibrary is a contract over one library's exported values. It owns
	// its own export graph and nothing outside it.
	ClassLibrary
	// ClassEnvironment is the specialization that owns the boot roots, the
	// environment slots, the primitive metatable attachments, and the host
	// capabilities. There is one initial environment, so there is one
	// environment contract kind.
	ClassEnvironment
	classLimit
)

func (class Class) Available() bool { return class > ClassInvalid && class < classLimit }

// Addressing is how a contract kind names the member a signature or marker
// attaches to. Both spellings are declarable so that the rejected one is a
// stated verdict rather than an unspellable shape: a kind that addresses by
// name is the exact defect this surface exists to forbid, and a law can only
// reject what can be written down.
type Addressing uint8

const (
	AddressingInvalid Addressing = iota
	// AddressingExportPath addresses a member by the path of exported values
	// from the contract root. The contract rides the value, so an alias keeps
	// it and a rebound slot does not acquire it.
	AddressingExportPath
	// AddressingGlobalName addresses a member by a dotted global name. A name
	// is not a value: it is rebindable, aliasable, and shadowable, so a
	// contract attached to one is attached to whatever the name later holds.
	// This surface rejects it.
	AddressingGlobalName
	addressingLimit
)

func (addressing Addressing) Available() bool {
	return addressing > AddressingInvalid && addressing < addressingLimit
}

// ValueProvenance reports whether an addressing form attaches a contract to a
// value rather than to a name. It is the seal-checkable property the
// value-provenance law states over a kind declaration.
func (addressing Addressing) ValueProvenance() bool { return addressing == AddressingExportPath }

// Form is the closed catalog of member shapes a contract kind may declare. A
// form is a declared shape identity, not the payload's Go type: the payload
// format belongs to its owner, and this surface names it, versions it through
// the kind's codec, and states which kinds must declare it.
//
// The catalog is split at the environment boundary. Forms below
// formEnvironmentFloor are the base algebra every kind declares; forms at or
// above it are the environment's own, and a library kind that declared one
// would be declaring a shape that mutates the global environment.
type Form uint8

const (
	FormInvalid Form = iota
	// FormCallableSignature is the typed application envelope of an exported
	// callable: its parameter row, its variadic tail, its type parameters, its
	// alternative result rows, and the nested envelopes of the callables it
	// applies. One member, one envelope.
	FormCallableSignature
	// FormIntrinsicMarker is the sealed semantic identity of a native
	// operation whose result depends on caller values. It is a marker, not a
	// type: a consumer that reconstructed it from a callee name would be
	// addressing by name again.
	FormIntrinsicMarker
	// FormEffectLabel is a reference to a declared effect label, so a
	// contract states which audited capability an exported callable exercises
	// without restating the label vocabulary.
	FormEffectLabel
	// FormMetatableEdge is an edge published through a metatable key inside
	// the contract's own export graph. It is how a library exposes members
	// that are reached through __index rather than through a direct export.
	// Attaching that metatable to a primitive is the environment's business,
	// not the library's.
	FormMetatableEdge
	// FormExportValue is a non-callable exported value: a constant, an
	// aggregate export, and the mutability the export is published with.
	FormExportValue
	// FormResultProvenance is where a result value comes from: it aliases a
	// declared input, it is a freshly allocated value of a declared kind, or
	// it is another declared callable together with the inputs that callable
	// captures. It is the value-identity fact a signature cannot carry,
	// because a type says what a value is and never which value it is.
	FormResultProvenance
	// FormResultRefinement is a result refinement predicated on declared
	// caller data: a literal argument, or a proved subject length. The
	// predicate and the refined result are both enumerable, so the refinement
	// is contract data.
	FormResultRefinement
	// FormSuspension is the yield-and-reentry relation of a suspending
	// callable. A suspension is not a result: it is a point at which control
	// leaves and may return, with its own multiplicity.
	FormSuspension
	// FormRuleDelegation is a reference to the rule that owns a member's
	// caller-dependent result selection. Selection driven by an arbitrary
	// literal cannot be enumerated as contract data, so the contract names
	// the rule instead of pretending to carry the computation.
	FormRuleDelegation
	// FormDeniedEntry is a member its owner declares and refuses to publish. A
	// denial is owner-declared member data, so each class states its own: a
	// library declares a member it models and will not hand out, and the
	// environment declares an entry it boots refused or absent. Denial is
	// contract data rather than omission, so coverage of the unsupported
	// boundary can be checked instead of assumed, and the refusal rides the
	// contract that owns the member rather than whoever observes it missing.
	FormDeniedEntry
	// FormExportType is a named runtime TYPE one contract publishes: the type
	// itself, addressed by the export key an annotation spells it under. It is
	// the one member form whose payload IS a type, so the payload format belongs
	// to the layer that owns the type wire and nothing about the type is
	// restated here. A contract that exports a name a type annotation resolves -
	// the runtime channel marker, the top of the table lattice - states it as
	// contract data rather than leaving a resolver to carry a table of built-in
	// names it invented. It is a base form: publishing a type under one's own
	// export key reaches nothing outside the contract's own export graph, and a
	// host module publishes named types exactly as a library would.
	FormExportType

	// formEnvironmentFloor is the first form only the environment class may
	// declare. It is not itself a form.
	formEnvironmentFloor

	// FormBootRoot is one root of the initial environment: its identity, the
	// aggregate it is, and whether it is published frozen.
	FormBootRoot
	// FormEnvironmentSlot binds one environment slot to the exported value
	// that initially occupies it. The binding is the only place a name meets a
	// value, and it is owned by the environment, which is why a library kind
	// may not declare it.
	FormEnvironmentSlot
	// FormPrimitiveMetatable attaches a metatable to a base primitive. It is
	// what makes a primitive's methods reachable, and it is an environment
	// fact: the library declares the edge, the environment declares that the
	// edge applies to a primitive.
	FormPrimitiveMetatable
	// FormHostCapability is a capability the host grants the environment. It
	// is the outermost authority a contract can reference, so only the
	// environment class declares it.
	FormHostCapability
	formLimit
)

func (form Form) Available() bool {
	return form > FormInvalid && form < formLimit && form != formEnvironmentFloor
}

// Environment reports whether a form belongs to the environment extension of
// the algebra.
func (form Form) Environment() bool { return form > formEnvironmentFloor && form < formLimit }

// Required is the set of forms one class must declare. The environment class
// extends the base set rather than replacing it, so an environment contract
// carries every library-contract shape and its own four besides.
func (class Class) Required() []Form {
	if !class.Available() {
		return nil
	}
	forms := make([]Form, 0, formLimit)
	for form := FormInvalid + 1; form < formLimit; form++ {
		if form == formEnvironmentFloor {
			continue
		}
		if form.Environment() && class != ClassEnvironment {
			continue
		}
		forms = append(forms, form)
	}
	return forms
}

// Resolution is how a kind's validation law-set reference is answered. A
// reference is either resolved against a surface sealed below this one, or it
// is deferred to a surface that does not exist yet. Form-validating an
// identity is not resolving it, and a deferred reference says so.
type Resolution uint8

const (
	ResolutionInvalid Resolution = iota
	// ResolutionSealed resolves the reference against an already-sealed
	// sibling surface, in the same table this surface is being sealed into.
	ResolutionSealed
	// ResolutionDeferred carries a form-valid law-set identity whose
	// describing surface has not landed. Nothing pretends it was resolved.
	ResolutionDeferred
)

func (resolution Resolution) Available() bool {
	return resolution == ResolutionSealed || resolution == ResolutionDeferred
}

// LawSet is the reference to the validation law set a contract kind's
// instances are checked under.
type LawSet struct {
	// Resolution states how this reference is answered.
	Resolution Resolution
	// Surface and Entry name the already-sealed row that owns the law set.
	// They are declared only by a sealed reference.
	Surface schema.SurfaceKind
	Entry   schema.Key
	// Deferred is the declared identity of a law set whose surface has not
	// landed. It is declared only by a deferred reference.
	Deferred identity.ContentID
}

func (set LawSet) Available() bool {
	switch set.Resolution {
	case ResolutionSealed:
		return set.Surface.Available() && set.Entry.Available() && !set.Deferred.Available()
	case ResolutionDeferred:
		return set.Deferred.Available() && !set.Surface.Available() && !set.Entry.Available()
	default:
		return false
	}
}

// Member is one declared member shape of a contract kind: which form it is,
// and the declared identity of the payload format that form is serialized in.
// The payload format itself belongs to its owner; this surface names it so a
// verdict can be issued by identity.
type Member struct {
	Form    Form
	Payload identity.ContentID
}

func (member Member) Available() bool { return member.Form.Available() && member.Payload.Available() }

// Codec is the serialized contract format one kind's instances are published
// in: the format's declared identity and the version of that format. Two kinds
// over one format identity are one kind under two names, so the identity is
// unique across the surface.
type Codec struct {
	Format  identity.ContentID
	Version uint32
}

func (codec Codec) Available() bool { return codec.Format.Available() && codec.Version != 0 }

// Spec is the authored declaration of one contract kind.
type Spec struct {
	// Key is the kind's authored identity and its diagnostic name, so a
	// contract kind has exactly one spelling in the analyzer. It derives the
	// entry identity a verdict carries.
	Key schema.Key
	// Class is which contract algebra this kind is: the library base, or the
	// environment specialization over it.
	Class Class
	// Codec is the serialized format this kind's instances are published in.
	Codec Codec
	// Validation is the law set this kind's instances are checked under.
	Validation LawSet
	// Addressing is how this kind names the member a contract attaches to. A
	// name-based addressing form is rejected: it is the defect this surface
	// exists to forbid.
	Addressing Addressing
	// Members is the member-shape vocabulary this kind declares. A class's
	// required forms must all appear, each exactly once.
	Members []Member
}

// Entry is one admitted contract kind declaration. It is immutable once built.
type Entry struct {
	key        schema.Key
	id         schema.EntryID
	class      Class
	codec      Codec
	validation LawSet
	addressing Addressing
	members    []Member
}

// New admits one authored declaration. A rejected spec returns false rather
// than a partially usable entry.
func New(spec Spec) (*Entry, bool) {
	if !spec.Key.Available() || !spec.Class.Available() || !spec.Codec.Available() || !spec.Validation.Available() {
		return nil, false
	}
	// The value-provenance law is stated at admission as well as at seal: a
	// name-addressed kind never becomes an entry that a later reader could
	// mistake for a declared one.
	if !spec.Addressing.Available() || !spec.Addressing.ValueProvenance() {
		return nil, false
	}
	// A kind resolves a sealed reference against a surface below it, so a
	// reference at or above this surface names a table that does not exist yet.
	if spec.Validation.Resolution == ResolutionSealed && spec.Validation.Surface >= schema.SurfaceKindLibrary {
		return nil, false
	}
	entry := &Entry{
		key:        spec.Key,
		id:         schema.NewEntryID(schema.SurfaceKindLibrary, spec.Key),
		class:      spec.Class,
		codec:      spec.Codec,
		validation: spec.Validation,
		addressing: spec.Addressing,
		members:    append([]Member(nil), spec.Members...),
	}
	if !entry.EntryAvailable() || !entry.declarationComplete() {
		return nil, false
	}
	return entry, true
}

func (entry *Entry) Key() schema.Key { return entry.key }

func (entry *Entry) ID() schema.EntryID { return entry.id }

func (entry *Entry) Class() Class { return entry.class }

func (entry *Entry) Codec() Codec { return entry.codec }

func (entry *Entry) Validation() LawSet { return entry.validation }

func (entry *Entry) Addressing() Addressing { return entry.addressing }

// Members returns a copy of the declared member-shape vocabulary, so a reader
// cannot rewrite a sealed declaration through the slice it was handed.
func (entry *Entry) Members() []Member {
	if entry == nil {
		return nil
	}
	return append([]Member(nil), entry.members...)
}

// Declares reports whether this kind declares one member form.
func (entry *Entry) Declares(form Form) bool {
	if entry == nil || !form.Available() {
		return false
	}
	for _, member := range entry.members {
		if member.Form == form {
			return true
		}
	}
	return false
}

// Payload resolves the declared payload format identity of one member form.
func (entry *Entry) Payload(form Form) (identity.ContentID, bool) {
	if entry == nil || !form.Available() {
		return identity.ContentID{}, false
	}
	for _, member := range entry.members {
		if member.Form == form {
			return member.Payload, true
		}
	}
	return identity.ContentID{}, false
}

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the contract kind it identifies is completely declared is
// the surface's own law, stated by Seal.
func (entry *Entry) EntryAvailable() bool {
	return entry != nil && entry.key.Available() && entry.id.Available()
}

// EntryContent writes this contract kind's declared data: which algebra it is,
// the serialized format and version its instances are published in, the law set
// its instances are checked under, how it addresses a member, and the member
// shapes it declares, in declaration order. A mount resolves a kind from
// exactly these, so a kind that gains or loses a member form is a different
// kind and the table digest says so.
func (entry *Entry) EntryContent(content *framing.Writer) error {
	if err := content.Uint(uint64(entry.class)); err != nil {
		return err
	}
	if err := content.Bytes(entry.codec.Format[:]); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.codec.Version)); err != nil {
		return err
	}
	if err := entry.validationContent(content); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.addressing)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(entry.members))); err != nil {
		return err
	}
	for _, member := range entry.members {
		if err := content.Record(contentRecordMember); err != nil {
			return err
		}
		if err := content.Uint(uint64(member.Form)); err != nil {
			return err
		}
		if err := content.Bytes(member.Payload[:]); err != nil {
			return err
		}
	}
	return nil
}

// validationContent writes the law-set reference. The resolution is written
// first and only the half that resolution declares follows it, so a sealed
// reference and a deferred one are written as the different references they are.
func (entry *Entry) validationContent(content *framing.Writer) error {
	if err := content.Record(contentRecordValidation); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.validation.Resolution)); err != nil {
		return err
	}
	switch entry.validation.Resolution {
	case ResolutionSealed:
		if err := content.Uint(uint64(entry.validation.Surface)); err != nil {
			return err
		}
		return content.String(string(entry.validation.Entry))
	case ResolutionDeferred:
		return content.Bytes(entry.validation.Deferred[:])
	}
	return nil
}

func (entry *Entry) declarationComplete() bool {
	return entry.class.Available() && entry.codec.Available() && entry.validation.Available() &&
		entry.addressing.ValueProvenance() && entry.membersComplete()
}

func (entry *Entry) membersComplete() bool {
	for _, form := range entry.class.Required() {
		if !entry.Declares(form) {
			return false
		}
	}
	return true
}

// Table is the immutable projection a consumer reads the sealed contract kinds
// through. It is the shape a mount-time contract loader resolves a kind
// against, so a loader never restates the catalog.
type Table struct {
	entries     []*Entry
	byClass     [classLimit][]*Entry
	environment *Entry
}

// NewTable projects one sealed library view. The class and completeness laws
// have already run at seal, so the projection is total by construction rather
// than by check.
func NewTable(view schema.View) (Table, bool) {
	if view.Kind() != schema.SurfaceKindLibrary || !view.Available() {
		return Table{}, false
	}
	var table Table
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || entry == nil || !entry.class.Available() {
			return Table{}, false
		}
		table.entries = append(table.entries, entry)
		table.byClass[entry.class] = append(table.byClass[entry.class], entry)
		if entry.class == ClassEnvironment {
			if table.environment != nil {
				return Table{}, false
			}
			table.environment = entry
		}
	}
	if len(table.byClass[ClassLibrary]) == 0 || table.environment == nil {
		return Table{}, false
	}
	return table, true
}

// Count is the number of declared contract kinds.
func (table Table) Count() int { return len(table.entries) }

// At resolves one declared kind by its declaration position.
func (table Table) At(position int) (*Entry, bool) {
	if position < 0 || position >= len(table.entries) {
		return nil, false
	}
	return table.entries[position], true
}

// Class returns every declared kind of one class, in declaration order.
func (table Table) Class(class Class) []*Entry {
	if !class.Available() {
		return nil
	}
	return append([]*Entry(nil), table.byClass[class]...)
}

// Environment returns the one declared environment contract kind.
func (table Table) Environment() (*Entry, bool) {
	return table.environment, table.environment != nil
}

// surface is the library contract contribution to the analyzer declaration
// root.
type surface struct{ entries []*Entry }

// NewSurface hands one ordered set of contract kind declarations to the table.
func NewSurface(entries []*Entry) schema.Surface { return surface{entries: entries} }

func (contribution surface) Kind() schema.SurfaceKind { return schema.SurfaceKindLibrary }

func (contribution surface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.entries))
	for index, entry := range contribution.entries {
		entries[index] = entry
	}
	return entries
}

// Seal states the library contract surface's own laws over the indexed view.
// Every validation reference is resolved against the surface it names, in the
// same table this surface is being sealed into, or is declared deferred.
func (contribution surface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	formats := make(map[identity.ContentID]schema.EntryID, view.Count())
	var libraries, environments int
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || entry == nil {
			return failure(schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		// Entry uniqueness is the root's law. What the surface states here is
		// that the identity a verdict carries is this surface's own derivation
		// of this entry's key, so an entry cannot travel under another
		// surface's identity.
		if !entry.key.Available() || entry.id != schema.NewEntryID(schema.SurfaceKindLibrary, entry.key) {
			return failure(entry.id, LawContractIdentity, schema.DispositionMalformed)
		}
		if !entry.class.Available() {
			return failure(entry.id, LawClassDeclared, schema.DispositionIncomplete)
		}
		if failure := contribution.sealAddressing(entry); failure.Available() {
			return failure
		}
		if failure := contribution.sealCodec(entry, formats); failure.Available() {
			return failure
		}
		if failure := contribution.sealValidation(entry, sealed); failure.Available() {
			return failure
		}
		if failure := contribution.sealMembers(entry); failure.Available() {
			return failure
		}
		switch entry.class {
		case ClassLibrary:
			libraries++
		case ClassEnvironment:
			environments++
		}
	}
	// There is one initial environment, so two environment kinds are two
	// competing formats for it and a mount would have no ground to choose
	// between them.
	if environments > 1 {
		return failure(schema.EntryID{}, LawClassPopulated, schema.DispositionDuplicate)
	}
	// A surface with no library kind declares nothing, and one with no
	// environment kind leaves the boot roots, the environment slots and the
	// primitive metatable attachments unowned.
	if libraries == 0 || environments == 0 {
		return failure(schema.EntryID{}, LawClassPopulated, schema.DispositionIncomplete)
	}
	return schema.SealFailure{}
}

// sealAddressing states the value-provenance law. A contract attaches to an
// exported value reached from the contract root, so an alias of that value
// keeps the contract and a slot rebound to another value does not acquire it.
// A name-addressed kind inverts both, and is rejected as the kind it is rather
// than repaired into the kind it should have been.
func (contribution surface) sealAddressing(entry *Entry) schema.SealFailure {
	if !entry.addressing.Available() {
		return failure(entry.id, LawAddressingDeclared, schema.DispositionIncomplete)
	}
	if !entry.addressing.ValueProvenance() {
		return failure(entry.id, LawAddressingProvenance, schema.DispositionMalformed)
	}
	return schema.SealFailure{}
}

func (contribution surface) sealCodec(entry *Entry, formats map[identity.ContentID]schema.EntryID) schema.SealFailure {
	if !entry.codec.Format.Available() {
		return failure(entry.id, LawCodecDeclared, schema.DispositionIncomplete)
	}
	// An unversioned format cannot be evolved: a reader has no ground to
	// distinguish a contract it can decode from one it cannot.
	if entry.codec.Version == 0 {
		return failure(entry.id, LawCodecVersioned, schema.DispositionIncomplete)
	}
	if prior, duplicate := formats[entry.codec.Format]; duplicate {
		return failure(prior, LawCodecUnique, schema.DispositionDuplicate)
	}
	formats[entry.codec.Format] = entry.id
	return schema.SealFailure{}
}

func (contribution surface) sealValidation(entry *Entry, sealed schema.Sealed) schema.SealFailure {
	if !entry.validation.Resolution.Available() {
		return failure(entry.id, LawValidationDeclared, schema.DispositionIncomplete)
	}
	if entry.validation.Resolution == ResolutionDeferred {
		// A deferred reference carries a form-valid identity and nothing that
		// looks resolved. Half of a resolution is not a resolution.
		if !entry.validation.Available() {
			return failure(entry.id, LawValidationDeferred, schema.DispositionMalformed)
		}
		return schema.SealFailure{}
	}
	if !entry.validation.Available() {
		return failure(entry.id, LawValidationDeclared, schema.DispositionIncomplete)
	}
	// The catalog order is the reference order. A law set at or above this
	// surface has not been sealed yet, so the reference cannot be resolved and
	// is not admitted as though it had been.
	if entry.validation.Surface >= schema.SurfaceKindLibrary {
		return failure(entry.id, LawValidationPhase, schema.DispositionMalformed)
	}
	owning, owningOK := sealed.Surface(entry.validation.Surface)
	if !owningOK {
		return failure(entry.id, LawValidationPhase, schema.DispositionIncomplete)
	}
	if _, declared := owning.ByID(schema.NewEntryID(entry.validation.Surface, entry.validation.Entry)); !declared {
		return failure(entry.id, LawValidationResolves, schema.DispositionIncomplete)
	}
	return schema.SealFailure{}
}

func (contribution surface) sealMembers(entry *Entry) schema.SealFailure {
	claimed := make(map[Form]struct{}, len(entry.members))
	payloads := make(map[identity.ContentID]struct{}, len(entry.members))
	for _, member := range entry.members {
		if !member.Form.Available() || !member.Payload.Available() {
			return failure(entry.id, LawMemberFormDeclared, schema.DispositionIncomplete)
		}
		// A library kind that declared an environment form would be declaring
		// a shape whose only effect is on the global environment, which no
		// individual library owns.
		if member.Form.Environment() && entry.class != ClassEnvironment {
			return failure(entry.id, LawEnvironmentExclusive, schema.DispositionMalformed)
		}
		if _, duplicate := claimed[member.Form]; duplicate {
			return failure(entry.id, LawMemberFormUnique, schema.DispositionDuplicate)
		}
		// Two forms over one payload format are one form under two names, and
		// a member would then decode differently depending on which name it
		// was declared under.
		if _, duplicate := payloads[member.Payload]; duplicate {
			return failure(entry.id, LawMemberFormUnique, schema.DispositionDuplicate)
		}
		claimed[member.Form] = struct{}{}
		payloads[member.Payload] = struct{}{}
	}
	// Completeness is what makes a contract kind able to carry the shapes the
	// analyzer already expresses. A missing form is a shape the absorbed
	// catalogue could state and the contract could not.
	for _, form := range entry.class.Required() {
		if _, declared := claimed[form]; !declared {
			return failure(entry.id, LawMemberFormComplete, schema.DispositionIncomplete)
		}
	}
	return schema.SealFailure{}
}

func failure(entry schema.EntryID, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	return schema.SurfaceLawFailure(schema.SurfaceKindLibrary, entry, law, disposition)
}

// Package factkey declares the key shapes the fixpoint's published families
// use.
//
// A fact key is a path: the family that states the fact, the thing the fact is
// about, any discriminators that narrow it, and the occurrence that published
// it. Consumers that need to know what a fact is about — which term it names,
// which allocation it belongs to — read that structure. Before this package
// each such consumer re-derived it by position or by matching a list of
// families it happened to know, so a family added later was silently invisible
// to walks that should have carried it, and two consumers could disagree about
// the same key.
//
// The declaration is the authority. A family named here is parsed by its
// declared shape; a family not named here is left to whatever positional rule
// its consumer already applied, so adding a declaration can only make a
// consumer see more, never change what it already saw for families it never
// declared.
package factkey

import (
	"encoding/base64"
	"sort"
	"strings"
)

// Kind classifies what one position of a key names.
type Kind uint8

const (
	// Opaque names nothing this schema resolves: a slot ordinal, an encoded
	// literal discriminator.
	Opaque Kind = iota
	// EncodedOpaque is an opaque byte string encoded as one URL-safe base64
	// segment. It is distinct from Opaque because builders, unlike the original
	// parse-only schema, must know which positions require encoding.
	EncodedOpaque
	// Identity names an allocation, encoded.
	Identity
	// EncodedTerm names a term, encoded.
	EncodedTerm
	// Term names a term written literally. A term is itself two segments.
	Term
	// Coordinate names one equation application coordinate. The body owning the
	// coordinate is carried by the closed equation artifact; the key records
	// its body-local name.
	Coordinate
	// Tagged names either an allocation or a term and says which, so the same
	// family can state a fact about a container it has an identity for and about
	// one it can only name by path.
	Tagged
)

// segments is how many key segments one position of this kind occupies.
func (k Kind) segments() int {
	if k == Term || k == Tagged {
		return 2
	}
	return 1
}

// PayloadKind names the codec responsible for interpreting a family's Value.
// The family record owns this choice even when the engine continues to invoke
// the codec itself during the staged migration.
type PayloadKind uint8

const (
	PayloadBytes PayloadKind = iota
	PayloadMarker
	PayloadIdentity
	PayloadTerm
	PayloadValue
	PayloadInteger
	PayloadType
	PayloadRelation
	PayloadTypestate
)

// FamilyID is the stable identity used by revocation sets. Prefix strings are
// a wire representation, not the identity by which declarations refer to one
// another.
type FamilyID uint8

const (
	FamilyHeapTableIdentity FamilyID = iota + 1
	FamilyHeapTableClosed
	FamilyHeapMember
	FamilyHeapMemberIdentity
	FamilyHeapMemberCell
	FamilyHeapMemberOrigin
	FamilyHeapStaticReplace
	FamilyHeapMetaAttached
	FamilyHeapMetaIdentity
	FamilyHeapMetaNewIndex
	FamilyHeapExternalCallback
	FamilyHeapOpaqueMemberWrite
	FamilyHeapKeysOf
	FamilyHeapKeyedRead
	FamilyHeapKeyedElement
	FamilyHeapIndexPresence
	FamilyHeapKeyPresence
	FamilyHeapIndexRevoke
	FamilyHeapLengthFloor
	FamilyHeapTableEscape
	FamilyHeapIndexLower
	FamilyHeapIndexUpper
	FamilyHeapIndexRelation
	FamilyValue
	FamilyCallResult
	FamilyCallArgument
	FamilyLocalCallResult
	FamilyType
	FamilyDeclaredType
	FamilySummaryType
	FamilyMethodReturnSummary
	FamilyBranchProof
	FamilyIteratorElement
	FamilyIteratorKey
	FamilyIteratorKeySource
	FamilyNativeConstantValue
	FamilyNativePublicationIdentity
	FamilyNativeBranchPartition
	FamilyNativeTruthinessClass
	FamilyBranchResidueClass
	FamilyNativeConcatSite
	FamilyNativeBuiltinCall
	FamilyHeapAllocationDisplay
	FamilyNativeAliasDisjoint
	FamilyNativeCaptureEpochRoot
	FamilyNativeCaptureTransport
	FamilyNativeTypedProducer
	FamilyNativeTableConstructionBound
	FamilyNativeProjection
	FamilyLifecycleChannelState
	FamilyLifecycleChannelDisplay
	FamilyLifecycleResourceState
)

var (
	nextFamilyID = FamilyLifecycleResourceState
	families     []Family
)

// revocationSet is the declaration-owned closed set of families whose
// publications can invalidate another family. Its representation is private:
// consumers receive only a cursor minted from the family declaration.
type revocationSet struct{ ids []FamilyID }

func revokers(ids ...FamilyID) revocationSet {
	return revocationSet{ids: append([]FamilyID(nil), ids...)}
}

// RevokerCursor is an opaque traversal of one family's declared revokers. A
// consumer cannot populate or subset it; only Family.Revokers can mint one.
type RevokerCursor struct {
	set   revocationSet
	index int
}

func (cursor *RevokerCursor) Next() (Family, bool) {
	if cursor == nil || cursor.index >= len(cursor.set.ids) {
		return Family{}, false
	}
	id := cursor.set.ids[cursor.index]
	cursor.index++
	return FamilyByID(id)
}

// Family declares one fact family's key shape. A key of this family is the
// prefix, then the subject, then each qualifier in order, then the occurrence
// that published it.
type Family struct {
	ID          FamilyID
	Prefix      string
	Subject     Kind
	Qualifiers  []Kind
	PayloadKind PayloadKind
	revokers    revocationSet
}

// Revokers returns the declaration-owned opaque revoker traversal.
func (f Family) Revokers() RevokerCursor {
	return RevokerCursor{set: f.revokers}
}

// Position is one resolved position of a parsed key.
type Position struct {
	Kind Kind
	// Term is the term this position names, empty when it names none.
	Term string
	// Identity is the allocation this position names, nil when it names none.
	Identity []byte
}

// Projection is the generic publication-key coordinate recovered for a
// serializer. Family is the first path segment; Term and Occurrence are
// selected only from vocabularies the caller supplies from its artifact.
// Keeping this segment walk here prevents output adapters from growing a
// second, implicitly different key-kind system.
type Projection struct {
	Family     string
	Term       string
	Occurrence string
}

// tagged subject spellings. A subject that states its own kind uses them.
const (
	taggedIdentity = "identity"
	taggedTerm     = "term"
)

var (
	HeapTableIdentity     = revokingFamilyRecord(FamilyHeapTableIdentity, "heap/table-identity/", Term, PayloadIdentity, revokers(FamilyHeapTableIdentity))
	HeapTableClosed       = revokingFamilyRecord(FamilyHeapTableClosed, "heap/table-closed/", Identity, PayloadMarker, revokers(FamilyHeapMetaAttached, FamilyHeapExternalCallback, FamilyHeapOpaqueMemberWrite, FamilyHeapTableEscape))
	HeapMember            = revokingFamilyRecord(FamilyHeapMember, "heap/member/", Identity, PayloadValue, revokers(FamilyHeapMember, FamilyHeapStaticReplace, FamilyHeapOpaqueMemberWrite, FamilyHeapExternalCallback, FamilyHeapTableEscape), EncodedOpaque)
	HeapMemberIdentity    = revokingFamilyRecord(FamilyHeapMemberIdentity, "heap/member-identity/", Identity, PayloadIdentity, revokers(FamilyHeapMemberIdentity, FamilyHeapStaticReplace, FamilyHeapOpaqueMemberWrite, FamilyHeapExternalCallback, FamilyHeapTableEscape), EncodedOpaque)
	HeapMemberCell        = revokingFamilyRecord(FamilyHeapMemberCell, "heap/member-cell/", Identity, PayloadBytes, revokers(FamilyHeapMemberCell, FamilyHeapOpaqueMemberWrite, FamilyHeapExternalCallback, FamilyHeapTableEscape), EncodedOpaque)
	HeapMemberOrigin      = revokingFamilyRecord(FamilyHeapMemberOrigin, "heap/member-origin/", Term, PayloadTerm, revokers(FamilyHeapMemberOrigin), EncodedOpaque)
	HeapStaticReplace     = revokingFamilyRecord(FamilyHeapStaticReplace, "heap/static-replace/", Identity, PayloadMarker, revokers(FamilyHeapStaticReplace, FamilyHeapTableEscape))
	HeapMetaAttached      = revokingFamilyRecord(FamilyHeapMetaAttached, "heap/meta-attached/", Identity, PayloadMarker, revokers(FamilyHeapMetaAttached))
	HeapMetaIdentity      = revokingFamilyRecord(FamilyHeapMetaIdentity, "heap/meta-identity/", Identity, PayloadIdentity, revokers(FamilyHeapMetaIdentity, FamilyHeapExternalCallback))
	HeapMetaNewIndex      = revokingFamilyRecord(FamilyHeapMetaNewIndex, "heap/meta-newindex/", Identity, PayloadIdentity, revokers(FamilyHeapMetaNewIndex, FamilyHeapExternalCallback))
	HeapExternalCallback  = revokingFamilyRecord(FamilyHeapExternalCallback, "heap/external-callback/", Identity, PayloadMarker, revokers(FamilyHeapExternalCallback))
	HeapOpaqueMemberWrite = revokingFamilyRecord(
		FamilyHeapOpaqueMemberWrite, "heap/opaque-member-write/", Identity, PayloadBytes, revokers(FamilyHeapTableEscape),
	)
	HeapKeysOf = revokingFamilyRecord(
		FamilyHeapKeysOf, "heap/keys-of/", Identity, PayloadTerm,
		revokers(FamilyHeapMember, FamilyHeapMemberCell, FamilyHeapStaticReplace, FamilyHeapMetaAttached, FamilyHeapOpaqueMemberWrite, FamilyHeapExternalCallback, FamilyHeapIndexRevoke),
	)
	HeapKeyedRead       = familyRecord(FamilyHeapKeyedRead, "heap/keyed-read/", EncodedTerm, PayloadIdentity)
	HeapKeyedElement    = revokingFamilyRecord(FamilyHeapKeyedElement, "heap/keyed-element/", Identity, PayloadType, revokers(FamilyHeapMetaAttached, FamilyHeapExternalCallback, FamilyHeapTableEscape))
	HeapIndexPresence   = revokingFamilyRecord(FamilyHeapIndexPresence, "heap/index-presence/", Tagged, PayloadMarker, revokers(FamilyHeapIndexRevoke), EncodedTerm)
	HeapKeyPresence     = revokingFamilyRecord(FamilyHeapKeyPresence, "heap/key-presence/", Tagged, PayloadMarker, revokers(FamilyHeapMember, FamilyHeapMemberCell, FamilyHeapStaticReplace, FamilyHeapMetaAttached, FamilyHeapOpaqueMemberWrite, FamilyHeapExternalCallback, FamilyHeapIndexRevoke), EncodedTerm)
	HeapIndexRevoke     = revokingFamilyRecord(FamilyHeapIndexRevoke, "heap/index-revoke/", Tagged, PayloadMarker, revokers(FamilyHeapIndexRevoke))
	HeapLengthFloor     = revokingFamilyRecord(FamilyHeapLengthFloor, "heap/length-floor/", Tagged, PayloadInteger, revokers(FamilyHeapIndexRevoke))
	HeapTableEscape     = revokingFamilyRecord(FamilyHeapTableEscape, "heap/table-escape/", Tagged, PayloadMarker, revokers(FamilyHeapTableEscape))
	HeapIndexLower      = revokingFamilyRecord(FamilyHeapIndexLower, "heap/index-lower/", EncodedTerm, PayloadMarker, revokers(FamilyHeapIndexRevoke))
	HeapIndexUpper      = revokingFamilyRecord(FamilyHeapIndexUpper, "heap/index-upper/", EncodedTerm, PayloadMarker, revokers(FamilyHeapIndexRevoke), EncodedTerm)
	HeapIndexRelation   = revokingFamilyRecord(FamilyHeapIndexRelation, "heap/index-relation/", Opaque, PayloadRelation, revokers(FamilyHeapIndexRelation))
	Value               = revokingFamilyRecord(FamilyValue, "value/", Term, PayloadValue, revokers(FamilyValue))
	CallResult          = revokingFamilyRecord(FamilyCallResult, "call-result/", Coordinate, PayloadValue, revokers(FamilyCallResult))
	CallArgument        = revokingFamilyRecord(FamilyCallArgument, "call-argument/", Coordinate, PayloadTerm, revokers(FamilyCallArgument))
	LocalCallResult     = revokingFamilyRecord(FamilyLocalCallResult, "local-call-result/", Term, PayloadMarker, revokers(FamilyLocalCallResult))
	Type                = revokingFamilyRecord(FamilyType, "type/", Term, PayloadType, revokers(FamilyType))
	DeclaredType        = revokingFamilyRecord(FamilyDeclaredType, "declared-type/", Term, PayloadType, revokers(FamilyDeclaredType))
	SummaryType         = revokingFamilyRecord(FamilySummaryType, "summary-type/", Term, PayloadType, revokers(FamilySummaryType))
	MethodReturnSummary = revokingFamilyRecord(
		FamilyMethodReturnSummary, "method-return-summary/", Term, PayloadType, revokers(FamilyMethodReturnSummary),
	)
	BranchProofFamily            = revokingFamilyRecord(FamilyBranchProof, "branch-proof/", Opaque, PayloadMarker, revokers(FamilyBranchProof), Coordinate)
	IteratorElement              = revokingFamilyRecord(FamilyIteratorElement, "iterator-element/", Term, PayloadType, revokers(FamilyIteratorElement))
	IteratorKey                  = revokingFamilyRecord(FamilyIteratorKey, "iterator-key/", Term, PayloadType, revokers(FamilyIteratorKey))
	IteratorKeySource            = revokingFamilyRecord(FamilyIteratorKeySource, "iterator-key-source/", Term, PayloadTerm, revokers(FamilyIteratorKeySource))
	NativeConstantValue          = familyRecord(FamilyNativeConstantValue, "constant_value/", Opaque, PayloadBytes)
	NativePublicationIdentity    = familyRecord(FamilyNativePublicationIdentity, "publication_identity/", Opaque, PayloadBytes)
	NativeBranchPartition        = familyRecord(FamilyNativeBranchPartition, "branch_partition/", Opaque, PayloadBytes)
	NativeTruthinessClass        = familyRecord(FamilyNativeTruthinessClass, "truthiness_class/", Opaque, PayloadBytes)
	BranchResidueClass           = familyRecord(FamilyBranchResidueClass, "branch-residue-class/", EncodedTerm, PayloadRelation)
	NativeConcatSite             = familyRecord(FamilyNativeConcatSite, "concat_site/", Opaque, PayloadBytes)
	NativeBuiltinCall            = familyRecord(FamilyNativeBuiltinCall, "builtin_call/", Opaque, PayloadBytes, Opaque, Opaque)
	HeapAllocationDisplay        = familyRecord(FamilyHeapAllocationDisplay, "heap/allocation-display/", Identity, PayloadRelation)
	NativeAliasDisjoint          = familyRecord(FamilyNativeAliasDisjoint, "alias_disjoint/", Term, PayloadRelation, Identity)
	NativeCaptureEpochRoot       = familyRecord(FamilyNativeCaptureEpochRoot, "capture_epoch_root/", Opaque, PayloadRelation, Opaque)
	NativeCaptureTransport       = familyRecord(FamilyNativeCaptureTransport, "capture_transport/", Opaque, PayloadRelation, Opaque)
	NativeTypedProducer          = familyRecord(FamilyNativeTypedProducer, "typed_producer/", Opaque, PayloadBytes)
	NativeTableConstructionBound = familyRecord(FamilyNativeTableConstructionBound, "table_construction_bound/", Opaque, PayloadBytes)
	// NativeProjection is the typed transport for a native row whose public
	// key does not itself carry enough information to recover its subject,
	// occurrence, and validity interval. The payload is the authority; this
	// key only gives the guarded equation publication a stable coordinate.
	NativeProjection = familyRecord(FamilyNativeProjection, "native-projection/", Opaque, PayloadRelation, Opaque)
	// Lifecycle state families retain their established wire prefixes, while
	// their payload declaration makes typestate's publication codec the sole
	// interpreter. ChannelDisplay is term metadata, not lifecycle state.
	LifecycleChannelState   = revokingFamilyRecord(FamilyLifecycleChannelState, "effect.lifecycle.channel/", Identity, PayloadTypestate, revokers(FamilyLifecycleChannelState))
	LifecycleChannelDisplay = revokingFamilyRecord(FamilyLifecycleChannelDisplay, "effect.lifecycle.channel.display/", EncodedTerm, PayloadBytes, revokers(FamilyLifecycleChannelDisplay))
	LifecycleResourceState  = revokingFamilyRecord(FamilyLifecycleResourceState, "effect.lifecycle.resource/", Identity, PayloadTypestate, revokers(FamilyLifecycleResourceState))

	ResidueWindow              = newFamilyRecord("residue-window/", Term, PayloadRelation)
	LengthTerm                 = newFamilyRecord("length-term/", Term, PayloadTerm)
	PathEquality               = newFamilyRecord("path-equality/", EncodedTerm, PayloadMarker, EncodedTerm)
	PlacementAllocation        = newFamilyRecord("placement/allocation/", Opaque, PayloadRelation)
	PlacementBinding           = newFamilyRecord("placement/binding/", EncodedTerm, PayloadIdentity)
	PlacementEvent             = newFamilyRecord("placement/event/", Identity, PayloadMarker, Opaque)
	PlacementBlocker           = newFamilyRecord("placement/blocker/", Identity, PayloadMarker, Opaque)
	PlacementContainment       = newFamilyRecord("placement/contains/", Identity, PayloadMarker, Identity)
	PlacementContract          = newFamilyRecord("placement/contract/", Identity, PayloadMarker, Opaque)
	PlacementLocalReturnRoot   = newFamilyRecord("placement/local-return-root/", Coordinate, PayloadIdentity)
	LiteralDiagnostic          = newFamilyRecord("diagnostic/literal-source/", Term, PayloadValue)
	Epoch                      = newFamilyRecord("epoch/", Term, PayloadTerm)
	CallReturnArity            = newFamilyRecord("call-return-arity/", Term, PayloadInteger)
	InferredCallableReturn     = newFamilyRecord("inferred-callable-return/", Term, PayloadType)
	ContextualCallable         = newFamilyRecord("contextual-callable/", Term, PayloadType)
	ReturnMemberSummary        = newFamilyRecord("return-member-summary/", Term, PayloadRelation)
	ImportedReturnRelation     = newFamilyRecord("imported-return-relation/", Term, PayloadRelation)
	ChannelPayload             = newFamilyRecord("channel-payload/", Term, PayloadRelation)
	NumericForInduction        = newFamilyRecord("numeric-for-induction/", Term, PayloadValue)
	CorrelationConeValue       = newFamilyRecord("correlation-cone/value/", EncodedTerm, PayloadValue)
	ReturnTupleTrue            = newFamilyRecord("return-tuple-true/", EncodedOpaque, PayloadMarker, EncodedTerm)
	IterationKeyOf             = newFamilyRecord("iteration/key-of/", EncodedTerm, PayloadTerm)
	CallKeysOf                 = newFamilyRecord("call-keys-of/", Coordinate, PayloadTerm, Opaque)
	AffineIndex                = newFamilyRecord("affine-index/", Term, PayloadRelation)
	IndexReadDisplay           = newFamilyRecord("index-read-display/", Term, PayloadBytes)
	IndexReadScalar            = newFamilyRecord("index-read-scalar/", Term, PayloadMarker)
	TypedOptionalRead          = newFamilyRecord("typed-optional-read/", Term, PayloadMarker)
	TypePredicateTarget        = newFamilyRecord("type-predicate-target/", EncodedTerm, PayloadTerm)
	TypePredicatePair          = newFamilyRecord("type-predicate-pair/", EncodedTerm, PayloadTerm, EncodedTerm)
	TypePredicateValue         = newFamilyRecord("type-predicate-value/", EncodedTerm, PayloadTerm, EncodedTerm)
	CallTypePredicate          = newFamilyRecord("call-type-predicate/", Coordinate, PayloadTerm, Opaque)
	TypePredicateRelation      = newFamilyRecord("type-predicate-relation/", Term, PayloadRelation)
	OptionalResultOrigin       = newFamilyRecord("optional-provider-origin/", Term, PayloadTerm)
	ConcatOperandOrigin        = newFamilyRecord("concat-operand-origin/", Coordinate, PayloadRelation, Opaque)
	BooleanResult              = newFamilyRecord("boolean-result/", Term, PayloadMarker)
	OptionalWriteContainer     = newFamilyRecord("optional-write-container/", Coordinate, PayloadTerm)
	CallInferredReturn         = newFamilyRecord("call-inferred-return/", Coordinate, PayloadType, Opaque)
	IsolationFrozen            = newFamilyRecord("send.isolation.state/frozen/", Term, PayloadMarker)
	IsolationEscaped           = newFamilyRecord("send.isolation.state/escaped/", Term, PayloadMarker)
	ClaimBound                 = newFamilyRecord("claim-bound/", Term, PayloadValue)
	ReturnCandidate            = newFamilyRecord("return-candidate/", Opaque, PayloadValue)
	ReturnMemberClosure        = newFamilyRecord("return-member-closure/", Term, PayloadRelation)
	Closure                    = newFamilyRecord("closure/", Term, PayloadRelation)
	SelectConstraint           = newFamilyRecord("select/constraint/", Opaque, PayloadRelation)
	Return                     = newFamilyRecord("return/", Opaque, PayloadValue)
	EffectFreeze               = newFamilyRecord("effect.freeze/", Term, PayloadMarker)
	ImportedRelationResult     = newFamilyRecord("imported-relation-result/", EncodedTerm, PayloadMarker)
	MemberClosure              = newFamilyRecord("member-closure/", Term, PayloadRelation)
	ReturnMemberOrigin         = newFamilyRecord("return-member-origin/", Term, PayloadTerm, Opaque)
	GradualAny                 = newFamilyRecord("gradual-any/", Term, PayloadTerm)
	GradualLogical             = newFamilyRecord("gradual-logical/", Term, PayloadTerm)
	RuntimeTypeProof           = newFamilyRecord("runtime-type-proof/", EncodedTerm, PayloadMarker)
	CallMemberClosure          = newFamilyRecord("call-member-closure/", Coordinate, PayloadRelation, Opaque)
	IdentityFact               = newFamilyRecord("identity/", Term, PayloadIdentity)
	CallClosure                = newFamilyRecord("call-closure/", Coordinate, PayloadRelation, Opaque)
	CallMemberIdentity         = newFamilyRecord("call-member-identity/", Coordinate, PayloadIdentity, Opaque)
	ProviderAnyResult          = newFamilyRecord("provider-any-result/", Term, PayloadTerm)
	OptionalProviderResult     = newFamilyRecord("optional-provider-result/", Term, PayloadValue)
	AssignmentFunctionContract = newFamilyRecord("assignment-function-contract/", Coordinate, PayloadMarker)
	CastTarget                 = newFamilyRecord("cast-target/", Term, PayloadType)
	AssignmentMemberSurface    = newFamilyRecord("assignment-member-surface/", Coordinate, PayloadRelation)
	ThrowTemplate              = newFamilyRecord("throw_template/", Coordinate, PayloadRelation)
	EvalNode                   = newFamilyRecord("eval_node/", Coordinate, PayloadRelation)
	SelectArm                  = newFamilyRecord("select/arm/", Opaque, PayloadRelation, Opaque)
	SelectOrigin               = newFamilyRecord("select/origin/", Term, PayloadRelation)
	SelectMeta                 = newFamilyRecord("select/meta/", Opaque, PayloadRelation)
	Branch                     = newFamilyRecord("branch/", Coordinate, PayloadValue)
	Narrowing                  = newFamilyRecord("narrowing/", Coordinate, PayloadRelation)
	FrozenBranch               = newFamilyRecord("frozen-branch/", Coordinate, PayloadMarker)
	EffectCallBool             = newFamilyRecord("effect.call-bool/", Coordinate, PayloadValue)
	ExplicitAny                = newFamilyRecord("explicit-any/", Term, PayloadMarker)
	AssertionValue             = newFamilyRecord("assertion-value/", Term, PayloadValue)
	ReturnRelationSurface      = newFamilyRecord("return-relation-surface/", Coordinate, PayloadRelation, Opaque)
	DeclaredEntryBoundary      = newFamilyRecord("declared-entry-boundary/", Opaque, PayloadMarker)
	CallHeapIdentity           = newFamilyRecord("call-heap-identity/", Coordinate, PayloadIdentity, Opaque)
	CalleeSet                  = newFamilyRecord("callee_set/", Opaque, PayloadRelation)
	NativeTableElement         = newFamilyRecord("table_element/", Opaque, PayloadRelation)

	FunctionEntry         = newFamilyRecord("function_entry/", Opaque, PayloadRelation)
	EffectRow             = newFamilyRecord("effect_row/", Opaque, PayloadRelation)
	CallSCC               = newFamilyRecord("call_scc/", Opaque, PayloadRelation)
	ShapeIdentity         = newFamilyRecord("shape_identity/", Opaque, PayloadRelation)
	RecordConstruction    = newFamilyRecord("record_construction/", Opaque, PayloadRelation)
	RecordEntryOwnership  = newFamilyRecord("record_entry_ownership/", Opaque, PayloadRelation)
	ShapeTransition       = newFamilyRecord("shape_transition/", Opaque, PayloadRelation)
	DiscriminantSelect    = newFamilyRecord("discriminant_select/", Opaque, PayloadRelation)
	RecursiveTypeIdentity = newFamilyRecord("recursive_type_identity/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("metatable_seal/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("nilability/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("representation/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("divisor_property/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("scalar_operator/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("numeric_branch/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("numeric_loop_carrier/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("host_global_binding/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("list_construction/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("table_length/", Opaque, PayloadRelation)
	_                     = newFamilyRecord("table_growth/", Opaque, PayloadRelation)
	InterprocSummary      = newFamilyRecord("interproc_summary/", Opaque, PayloadRelation)
)

func familyRecord(id FamilyID, prefix string, subject Kind, payload PayloadKind, qualifiers ...Kind) Family {
	return registerFamily(Family{
		ID: id, Prefix: prefix, Subject: subject, Qualifiers: qualifiers,
		PayloadKind: payload, revokers: revocationSet{},
	})
}

func newFamilyRecord(prefix string, subject Kind, payload PayloadKind, qualifiers ...Kind) Family {
	nextFamilyID++
	return familyRecord(nextFamilyID, prefix, subject, payload, qualifiers...)
}

func revokingFamilyRecord(id FamilyID, prefix string, subject Kind, payload PayloadKind, revocations revocationSet, qualifiers ...Kind) Family {
	return registerFamily(Family{
		ID: id, Prefix: prefix, Subject: subject, Qualifiers: qualifiers,
		PayloadKind: payload, revokers: revocations,
	})
}

// Construction and registration are one operation, so an exported family
// cannot exist outside the lookup domain derived below.
func registerFamily(record Family) Family {
	families = append(families, record)
	return record
}

// byPrefix indexes the declarations so a key is matched without scanning them.
// widths are the distinct segment counts the declared prefixes use, longest
// first, so a family whose name extends another's is found before the shorter
// one. Both are derived from the declarations themselves rather than assumed.
var byPrefix, byID, widths = index()

func index() (map[string]Family, map[FamilyID]Family, []int) {
	table := make(map[string]Family, len(families))
	identities := make(map[FamilyID]Family, len(families))
	seen := make(map[int]bool)
	var counts []int
	for _, family := range families {
		table[family.Prefix] = family
		identities[family.ID] = family
		width := strings.Count(family.Prefix, "/")
		if !seen[width] {
			seen[width] = true
			counts = append(counts, width)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(counts)))
	return table, identities, counts
}

// Lookup returns the family that declares this key's shape. The longest
// declared prefix wins, so a family whose name extends another's is still read
// by its own declaration.
func Lookup(key string) (Family, bool) {
	for _, width := range widths {
		if prefix, ok := segmentPrefix(key, width); ok {
			if family, found := byPrefix[prefix]; found {
				return family, true
			}
		}
	}
	return Family{}, false
}

// FamilyByID resolves a stable family identity. Consumers never translate
// through prefix strings.
func FamilyByID(id FamilyID) (Family, bool) {
	family, ok := byID[id]
	return family, ok
}

// Families returns the declared domain used by source fences and audits.
func Families() []Family { return append([]Family(nil), families...) }

// Key is the family-wide typed prefix used by indexed readers.
func (f Family) Key() Key { return BuildKey(f, nil, "") }

// Owns reports whether key belongs to this exact declared namespace. Lookup's
// longest-prefix rule keeps a parent spelling from claiming a nested family.
func (f Family) Owns(key string) bool {
	declared, ok := Lookup(key)
	return ok && declared.ID == f.ID
}

// Tail returns the record body below the declared namespace. It fails closed
// for malformed or foreign keys.
func (f Family) Tail(key string) (string, bool) {
	if !f.Owns(key) {
		return "", false
	}
	return strings.TrimPrefix(key, f.Prefix), true
}

// Body is the fail-closed single-result form for callers that already selected
// the family. A foreign key yields no body.
func (f Family) Body(key string) string {
	tail, _ := f.Tail(key)
	return tail
}

// BodySegments is the fail-closed single-result form for an already-selected
// family.
func (f Family) BodySegments(key string) []string {
	body, ok := f.Tail(key)
	if !ok {
		return nil
	}
	return strings.Split(body, "/")
}

// OwnsPrefix reports whether prefix and key select the same declared family and
// key lies below prefix. It is the migration surface for subject-qualified
// indexed reads: callers may compute the subject, but cannot parse or cross a
// namespace the registry does not own.
func OwnsPrefix(prefix, key string) bool {
	prefixFamily, prefixOK := Lookup(prefix)
	keyFamily, keyOK := Lookup(key)
	return prefixOK && keyOK && prefixFamily.ID == keyFamily.ID && strings.HasPrefix(key, prefix)
}

// TailPrefix returns the suffix below a declared subject-qualified prefix.
func TailPrefix(prefix, key string) (string, bool) {
	if !OwnsPrefix(prefix, key) {
		return "", false
	}
	return strings.TrimPrefix(key, prefix), true
}

// BodyPrefix is the fail-closed single-result form of TailPrefix.
func BodyPrefix(prefix, key string) string {
	tail, _ := TailPrefix(prefix, key)
	return tail
}

// PrefixSegments is the fail-closed segmented form of a declared
// subject-qualified prefix.
func PrefixSegments(prefix, key string) []string {
	tail, ok := TailPrefix(prefix, key)
	if !ok {
		return nil
	}
	return strings.Split(tail, "/")
}

// Segments returns the complete segments of a key in a declared namespace.
func Segments(key string) []string {
	if _, ok := Lookup(key); !ok {
		return nil
	}
	return strings.Split(key, "/")
}

// Head returns the first namespace segment of a declared key. Native adapters
// retain this historical projection while the full Family remains authoritative.
func Head(key string) (string, bool) {
	family, ok := Lookup(key)
	if !ok {
		return "", false
	}
	head, _, ok := strings.Cut(family.Prefix, "/")
	return head, ok && head != ""
}

// segmentPrefix returns the key's first width segments, separator included.
func segmentPrefix(key string, width int) (string, bool) {
	at := 0
	for ; width > 0; width-- {
		next := strings.IndexByte(key[at:], '/')
		if next < 0 {
			return "", false
		}
		at += next + 1
	}
	return key[:at], true
}

// Parse resolves every declared position of one key of this family. It fails
// when the key does not have the declared shape, which keeps a malformed or
// foreign key from being read as though it named something.
func (f Family) Parse(key string) ([]Position, bool) {
	if f.Subject == Term && len(f.Qualifiers) == 0 {
		parsed, ok := f.ParseKey(key)
		if !ok {
			return nil, false
		}
		return []Position{{Kind: Term, Term: parsed.Subject.Spelling()}}, true
	}
	rest, ok := strings.CutPrefix(key, f.Prefix)
	if !ok {
		return nil, false
	}
	segments := strings.Split(rest, "/")
	kinds := append([]Kind{f.Subject}, f.Qualifiers...)
	width := 0
	for _, kind := range kinds {
		width += kind.segments()
	}
	// The occurrence is the one segment every family ends with.
	if len(segments) != width+1 || segments[len(segments)-1] == "" {
		return nil, false
	}
	positions := make([]Position, 0, len(kinds))
	at := 0
	for _, kind := range kinds {
		position, ok := resolve(kind, segments[at:at+kind.segments()])
		if !ok {
			return nil, false
		}
		positions = append(positions, position)
		at += kind.segments()
	}
	return positions, true
}

func resolve(kind Kind, segments []string) (Position, bool) {
	switch kind {
	case Term:
		term := strings.Join(segments, "/")
		if segments[0] == "" || segments[1] == "" {
			return Position{}, false
		}
		return Position{Kind: kind, Term: term}, true
	case Tagged:
		decoded, ok := decode(segments[1])
		if !ok {
			return Position{}, false
		}
		switch segments[0] {
		case taggedIdentity:
			return Position{Kind: kind, Identity: decoded}, true
		case taggedTerm:
			return Position{Kind: kind, Term: string(decoded)}, true
		}
		return Position{}, false
	case Identity:
		decoded, ok := decode(segments[0])
		if !ok {
			return Position{}, false
		}
		return Position{Kind: kind, Identity: decoded}, true
	case EncodedOpaque:
		if _, ok := decode(segments[0]); !ok {
			return Position{}, false
		}
		return Position{Kind: kind}, true
	case EncodedTerm:
		decoded, ok := decode(segments[0])
		if !ok {
			return Position{}, false
		}
		return Position{Kind: kind, Term: string(decoded)}, true
	}
	if segments[0] == "" {
		return Position{}, false
	}
	return Position{Kind: kind}, true
}

func decode(segment string) ([]byte, bool) {
	if segment == "" {
		return nil, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(segment)
	return decoded, err == nil && len(decoded) != 0
}

// AnchoredAt reports whether a key names this term in a position its family
// declares to name one, and whether the family is declared at all. An undeclared
// family reports nothing, leaving its consumer's own rule in force.
func AnchoredAt(key, term string) (anchored bool, declared bool) {
	positions, ok := positionsOf(key)
	if !ok {
		return false, false
	}
	for _, position := range positions {
		if position.Term == term {
			return true, true
		}
	}
	return false, true
}

// Allocations returns every allocation identity a key names, and whether its
// family is declared. A fact about an allocation belongs to that allocation
// whichever position names it: a member states its container as the subject,
// while an index bound states its container as a discriminator.
func Allocations(key string) ([][]byte, bool) {
	positions, ok := positionsOf(key)
	if !ok {
		return nil, false
	}
	var out [][]byte
	for _, position := range positions {
		if len(position.Identity) != 0 {
			out = append(out, position.Identity)
		}
	}
	return out, true
}

func positionsOf(key string) ([]Position, bool) {
	family, found := Lookup(key)
	if !found {
		return nil, false
	}
	positions, ok := family.Parse(key)
	if !ok {
		// A key that does not have its family's declared shape is not read here
		// at all. Its consumer keeps whatever rule it applied before, so a
		// declaration can only widen what a consumer sees.
		return nil, false
	}
	return positions, true
}

// Project recovers the family, longest known term, and last known occurrence
// from a published key. Known terms and occurrences come from the equation
// artifact, so this function only parses key structure; it does not infer a
// subject or coordinate the artifact did not publish.
func Project(key string, terms map[string]string, occurrences map[string]string, longest int) Projection {
	var out Projection
	if key == "" {
		return out
	}
	starts := make([]int, 1, 8)
	for index := 0; index < len(key); index++ {
		if key[index] == '/' {
			starts = append(starts, index+1)
		}
	}
	end := func(segment int) int {
		if segment+1 < len(starts) {
			return starts[segment+1] - 1
		}
		return len(key)
	}
	out.Family = key[:end(0)]
	for segment := len(starts) - 1; segment >= 0; segment-- {
		if _, coordinate := occurrences[key[starts[segment]:end(segment)]]; coordinate {
			out.Occurrence = key[starts[segment]:end(segment)]
			break
		}
	}
	best := 0
	for first := 0; first < len(starts); first++ {
		last := first + longest
		if last > len(starts) {
			last = len(starts)
		}
		for count := last - first; count > best; count-- {
			candidate := key[starts[first]:end(first+count-1)]
			if _, known := terms[candidate]; known {
				best, out.Term = count, candidate
				break
			}
		}
	}
	return out
}

// BranchGuardPrefix roots the encoding every certified CFG branch guard carries.
// The two edges of one decision are mutually exclusive and jointly exhaustive,
// which is what lets a consumer treat them as alternatives of each other.
const BranchGuardPrefix = "front/branch/"

// RecurrenceExitPrefix roots the fact family that names, for one decision, the
// edge through which control leaves a loop that decision continues. The loop's
// back edge re-evaluates that decision, so a publication on the opposite edge
// belongs to an earlier trip and reaches every point past the loop instead of
// describing a region those points exclude. The family states the relation
// only; which decision a read joins, and when, stays with the guard algebra.
// Its key carries the decision and its value carries the edge.
const RecurrenceExitPrefix = "front/recurrence-exit/"

// The two edges a branch decision has.
const (
	TrueEdge  = "true"
	FalseEdge = "false"
)

// BranchGuard names one edge of one certified branch decision.
type BranchGuard struct {
	Name string
	Edge string
}

// TrueEdged reports whether this is the decision's true edge.
func (g BranchGuard) TrueEdged() bool { return g.Edge == TrueEdge }

// Encoding writes the guard encoding for this edge.
func (g BranchGuard) Encoding() string { return BranchGuardPrefix + g.Name + "/" + g.Edge }

// AppendEncoding appends the guard encoding to dst. The equation bridge uses
// this form so the sealed []byte representation is built in one allocation.
func (g BranchGuard) AppendEncoding(dst []byte) []byte {
	dst = append(dst, BranchGuardPrefix...)
	dst = append(dst, g.Name...)
	dst = append(dst, '/')
	return append(dst, g.Edge...)
}

// BranchProof is the body-qualified statement of one branch decision.
type BranchProof struct {
	// Body is the deciding body, hex-encoded as the key spells it.
	Body string
	BranchGuard
}

// Key builds the fact key that publishes this body-qualified branch decision.
func (p BranchProof) Key() Key {
	return BuildKey(BranchProofFamily, []Part{OpaquePart(p.Body), CoordinatePart(p.Name)}, p.Edge)
}

// ParseBranchGuard reads one guard encoding. The name is whatever the encoding
// states between the prefix and the edge it ends with.
func ParseBranchGuard(encoding string) (BranchGuard, bool) {
	rest, ok := strings.CutPrefix(encoding, BranchGuardPrefix)
	if !ok {
		return BranchGuard{}, false
	}
	return cutEdge(rest)
}

// ParseBranchProof reads one branch-proof key: the deciding body, the decision,
// and the edge, each one segment.
func ParseBranchProof(key string) (BranchProof, bool) {
	parsed, ok := BranchProofFamily.ParseKey(key)
	if !ok {
		return BranchProof{}, false
	}
	decision, present := parsed.Qualifier(0)
	if !present {
		return BranchProof{}, false
	}
	guard := BranchGuard{Name: decision.Spelling(), Edge: parsed.Occurrence}
	if guard.Name == "" || (guard.Edge != TrueEdge && guard.Edge != FalseEdge) {
		return BranchProof{}, false
	}
	return BranchProof{Body: parsed.Subject.Spelling(), BranchGuard: guard}, true
}

// cutEdge splits a decision's name from the edge it ends with.
func cutEdge(rest string) (BranchGuard, bool) {
	cut := strings.LastIndexByte(rest, '/')
	if cut <= 0 {
		return BranchGuard{}, false
	}
	name, edge := rest[:cut], rest[cut+1:]
	if edge != TrueEdge && edge != FalseEdge {
		return BranchGuard{}, false
	}
	return BranchGuard{Name: name, Edge: edge}, true
}

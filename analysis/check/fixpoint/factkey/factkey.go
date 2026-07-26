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

// RevocationSet names the fact families whose publications can invalidate a
// fact in this family. The engine's ordering predicates still decide whether a
// particular publication is later; this record states the closed vocabulary.
type RevocationSet []FamilyID

// Family declares one fact family's key shape. A key of this family is the
// prefix, then the subject, then each qualifier in order, then the occurrence
// that published it.
type Family struct {
	ID            FamilyID
	Prefix        string
	Subject       Kind
	Qualifiers    []Kind
	PayloadKind   PayloadKind
	RevocationSet RevocationSet
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
	HeapTableIdentity = Family{
		ID: FamilyHeapTableIdentity, Prefix: "heap/table-identity/", Subject: Term,
		PayloadKind: PayloadIdentity, RevocationSet: RevocationSet{FamilyHeapTableIdentity},
	}
	HeapTableClosed = Family{
		ID: FamilyHeapTableClosed, Prefix: "heap/table-closed/", Subject: Identity,
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{
			FamilyHeapMetaAttached, FamilyHeapExternalCallback, FamilyHeapOpaqueMemberWrite, FamilyHeapTableEscape,
		},
	}
	HeapMember = Family{
		ID: FamilyHeapMember, Prefix: "heap/member/", Subject: Identity, Qualifiers: []Kind{EncodedOpaque},
		PayloadKind: PayloadValue, RevocationSet: RevocationSet{
			FamilyHeapMember, FamilyHeapStaticReplace, FamilyHeapOpaqueMemberWrite, FamilyHeapExternalCallback, FamilyHeapTableEscape,
		},
	}
	HeapMemberIdentity = Family{
		ID: FamilyHeapMemberIdentity, Prefix: "heap/member-identity/", Subject: Identity, Qualifiers: []Kind{EncodedOpaque},
		PayloadKind: PayloadIdentity, RevocationSet: RevocationSet{
			FamilyHeapMemberIdentity, FamilyHeapStaticReplace, FamilyHeapOpaqueMemberWrite, FamilyHeapExternalCallback, FamilyHeapTableEscape,
		},
	}
	HeapMemberCell = Family{
		ID: FamilyHeapMemberCell, Prefix: "heap/member-cell/", Subject: Identity, Qualifiers: []Kind{EncodedOpaque},
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{
			FamilyHeapMemberCell, FamilyHeapOpaqueMemberWrite, FamilyHeapExternalCallback, FamilyHeapTableEscape,
		},
	}
	HeapMemberOrigin = Family{
		ID: FamilyHeapMemberOrigin, Prefix: "heap/member-origin/", Subject: Term, Qualifiers: []Kind{EncodedOpaque},
		PayloadKind: PayloadTerm, RevocationSet: RevocationSet{FamilyHeapMemberOrigin},
	}
	HeapStaticReplace = Family{
		ID: FamilyHeapStaticReplace, Prefix: "heap/static-replace/", Subject: Identity,
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{FamilyHeapStaticReplace, FamilyHeapTableEscape},
	}
	HeapMetaAttached = Family{
		ID: FamilyHeapMetaAttached, Prefix: "heap/meta-attached/", Subject: Identity,
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{FamilyHeapMetaAttached},
	}
	HeapMetaIdentity = Family{
		ID: FamilyHeapMetaIdentity, Prefix: "heap/meta-identity/", Subject: Identity,
		PayloadKind: PayloadIdentity, RevocationSet: RevocationSet{FamilyHeapMetaIdentity, FamilyHeapExternalCallback},
	}
	HeapMetaNewIndex = Family{
		ID: FamilyHeapMetaNewIndex, Prefix: "heap/meta-newindex/", Subject: Identity,
		PayloadKind: PayloadIdentity, RevocationSet: RevocationSet{FamilyHeapMetaNewIndex, FamilyHeapExternalCallback},
	}
	HeapExternalCallback = Family{
		ID: FamilyHeapExternalCallback, Prefix: "heap/external-callback/", Subject: Identity,
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{FamilyHeapExternalCallback},
	}
	HeapOpaqueMemberWrite = Family{
		ID: FamilyHeapOpaqueMemberWrite, Prefix: "heap/opaque-member-write/", Subject: Identity,
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{FamilyHeapTableEscape},
	}
	HeapKeysOf = Family{
		ID: FamilyHeapKeysOf, Prefix: "heap/keys-of/", Subject: Identity,
		PayloadKind: PayloadTerm, RevocationSet: RevocationSet{
			FamilyHeapMember, FamilyHeapMemberCell, FamilyHeapStaticReplace, FamilyHeapMetaAttached,
			FamilyHeapOpaqueMemberWrite, FamilyHeapExternalCallback, FamilyHeapIndexRevoke,
		},
	}
	HeapKeyedRead = Family{
		ID: FamilyHeapKeyedRead, Prefix: "heap/keyed-read/", Subject: EncodedTerm,
		PayloadKind: PayloadIdentity, RevocationSet: RevocationSet{},
	}
	HeapKeyedElement = Family{
		ID: FamilyHeapKeyedElement, Prefix: "heap/keyed-element/", Subject: Identity,
		PayloadKind: PayloadType, RevocationSet: RevocationSet{
			FamilyHeapMetaAttached, FamilyHeapExternalCallback, FamilyHeapTableEscape,
		},
	}
	HeapIndexPresence = Family{
		ID: FamilyHeapIndexPresence, Prefix: "heap/index-presence/", Subject: Tagged, Qualifiers: []Kind{EncodedTerm},
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{FamilyHeapIndexRevoke},
	}
	HeapKeyPresence = Family{
		ID: FamilyHeapKeyPresence, Prefix: "heap/key-presence/", Subject: Tagged, Qualifiers: []Kind{EncodedTerm},
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{
			FamilyHeapMember, FamilyHeapMemberCell, FamilyHeapStaticReplace, FamilyHeapMetaAttached,
			FamilyHeapOpaqueMemberWrite, FamilyHeapExternalCallback, FamilyHeapIndexRevoke,
		},
	}
	HeapIndexRevoke = Family{
		ID: FamilyHeapIndexRevoke, Prefix: "heap/index-revoke/", Subject: Tagged,
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{FamilyHeapIndexRevoke},
	}
	HeapLengthFloor = Family{
		ID: FamilyHeapLengthFloor, Prefix: "heap/length-floor/", Subject: Tagged,
		PayloadKind: PayloadInteger, RevocationSet: RevocationSet{FamilyHeapIndexRevoke},
	}
	HeapTableEscape = Family{
		ID: FamilyHeapTableEscape, Prefix: "heap/table-escape/", Subject: Tagged,
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{FamilyHeapTableEscape},
	}
	HeapIndexLower = Family{
		ID: FamilyHeapIndexLower, Prefix: "heap/index-lower/", Subject: EncodedTerm,
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{FamilyHeapIndexRevoke},
	}
	HeapIndexUpper = Family{
		ID: FamilyHeapIndexUpper, Prefix: "heap/index-upper/", Subject: EncodedTerm, Qualifiers: []Kind{EncodedTerm},
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{FamilyHeapIndexRevoke},
	}
	HeapIndexRelation = Family{
		ID: FamilyHeapIndexRelation, Prefix: "heap/index-relation/", Subject: Opaque,
		PayloadKind: PayloadRelation, RevocationSet: RevocationSet{FamilyHeapIndexRelation},
	}
	Value = Family{
		ID: FamilyValue, Prefix: "value/", Subject: Term,
		PayloadKind: PayloadValue, RevocationSet: RevocationSet{FamilyValue},
	}
	CallResult = Family{
		ID: FamilyCallResult, Prefix: "call-result/", Subject: Coordinate,
		PayloadKind: PayloadValue, RevocationSet: RevocationSet{FamilyCallResult},
	}
	CallArgument = Family{
		ID: FamilyCallArgument, Prefix: "call-argument/", Subject: Coordinate,
		PayloadKind: PayloadTerm, RevocationSet: RevocationSet{FamilyCallArgument},
	}
	LocalCallResult = Family{
		ID: FamilyLocalCallResult, Prefix: "local-call-result/", Subject: Term,
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{FamilyLocalCallResult},
	}
	Type = Family{
		ID: FamilyType, Prefix: "type/", Subject: Term,
		PayloadKind: PayloadType, RevocationSet: RevocationSet{FamilyType},
	}
	DeclaredType = Family{
		ID: FamilyDeclaredType, Prefix: "declared-type/", Subject: Term,
		PayloadKind: PayloadType, RevocationSet: RevocationSet{FamilyDeclaredType},
	}
	SummaryType = Family{
		ID: FamilySummaryType, Prefix: "summary-type/", Subject: Term,
		PayloadKind: PayloadType, RevocationSet: RevocationSet{FamilySummaryType},
	}
	MethodReturnSummary = Family{
		ID: FamilyMethodReturnSummary, Prefix: "method-return-summary/", Subject: Term,
		PayloadKind: PayloadType, RevocationSet: RevocationSet{FamilyMethodReturnSummary},
	}
	BranchProofFamily = Family{
		ID: FamilyBranchProof, Prefix: "branch-proof/", Subject: Opaque, Qualifiers: []Kind{Coordinate},
		PayloadKind: PayloadMarker, RevocationSet: RevocationSet{FamilyBranchProof},
	}
	IteratorElement = Family{
		ID: FamilyIteratorElement, Prefix: "iterator-element/", Subject: Term,
		PayloadKind: PayloadType, RevocationSet: RevocationSet{FamilyIteratorElement},
	}
	IteratorKey = Family{
		ID: FamilyIteratorKey, Prefix: "iterator-key/", Subject: Term,
		PayloadKind: PayloadType, RevocationSet: RevocationSet{FamilyIteratorKey},
	}
	IteratorKeySource = Family{
		ID: FamilyIteratorKeySource, Prefix: "iterator-key-source/", Subject: Term,
		PayloadKind: PayloadTerm, RevocationSet: RevocationSet{FamilyIteratorKeySource},
	}
	NativeConstantValue = Family{
		ID: FamilyNativeConstantValue, Prefix: "constant_value/", Subject: Opaque,
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{},
	}
	NativePublicationIdentity = Family{
		ID: FamilyNativePublicationIdentity, Prefix: "publication_identity/", Subject: Opaque,
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{},
	}
	NativeBranchPartition = Family{
		ID: FamilyNativeBranchPartition, Prefix: "branch_partition/", Subject: Opaque,
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{},
	}
	NativeTruthinessClass = Family{
		ID: FamilyNativeTruthinessClass, Prefix: "truthiness_class/", Subject: Opaque,
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{},
	}
	BranchResidueClass = Family{
		ID: FamilyBranchResidueClass, Prefix: "branch-residue-class/", Subject: EncodedTerm,
		PayloadKind: PayloadRelation, RevocationSet: RevocationSet{},
	}
	NativeConcatSite = Family{
		ID: FamilyNativeConcatSite, Prefix: "concat_site/", Subject: Opaque,
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{},
	}
	NativeBuiltinCall = Family{
		ID: FamilyNativeBuiltinCall, Prefix: "builtin_call/", Subject: Opaque,
		Qualifiers:  []Kind{Opaque, Opaque},
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{},
	}
	HeapAllocationDisplay = Family{
		ID: FamilyHeapAllocationDisplay, Prefix: "heap/allocation-display/", Subject: Identity,
		PayloadKind: PayloadRelation, RevocationSet: RevocationSet{},
	}
	NativeAliasDisjoint = Family{
		ID: FamilyNativeAliasDisjoint, Prefix: "alias_disjoint/", Subject: Term,
		Qualifiers:  []Kind{Identity},
		PayloadKind: PayloadRelation, RevocationSet: RevocationSet{},
	}
	NativeCaptureEpochRoot = Family{
		ID: FamilyNativeCaptureEpochRoot, Prefix: "capture_epoch_root/", Subject: Opaque,
		Qualifiers:  []Kind{Opaque},
		PayloadKind: PayloadRelation, RevocationSet: RevocationSet{},
	}
	NativeCaptureTransport = Family{
		ID: FamilyNativeCaptureTransport, Prefix: "capture_transport/", Subject: Opaque,
		Qualifiers:  []Kind{Opaque},
		PayloadKind: PayloadRelation, RevocationSet: RevocationSet{},
	}
	NativeTypedProducer = Family{
		ID: FamilyNativeTypedProducer, Prefix: "typed_producer/", Subject: Opaque,
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{},
	}
	NativeTableConstructionBound = Family{
		ID: FamilyNativeTableConstructionBound, Prefix: "table_construction_bound/", Subject: Opaque,
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{},
	}
	// NativeProjection is the typed transport for a native row whose public
	// key does not itself carry enough information to recover its subject,
	// occurrence, and validity interval. The payload is the authority; this
	// key only gives the guarded equation publication a stable coordinate.
	NativeProjection = Family{
		ID: FamilyNativeProjection, Prefix: "native-projection/", Subject: Opaque,
		Qualifiers:  []Kind{Opaque},
		PayloadKind: PayloadRelation, RevocationSet: RevocationSet{},
	}
	// Lifecycle state families retain their established wire prefixes, while
	// their payload declaration makes typestate's publication codec the sole
	// interpreter. ChannelDisplay is term metadata, not lifecycle state.
	LifecycleChannelState = Family{
		ID: FamilyLifecycleChannelState, Prefix: "effect.lifecycle.channel/", Subject: Identity,
		PayloadKind: PayloadTypestate, RevocationSet: RevocationSet{FamilyLifecycleChannelState},
	}
	LifecycleChannelDisplay = Family{
		ID: FamilyLifecycleChannelDisplay, Prefix: "effect.lifecycle.channel.display/", Subject: EncodedTerm,
		PayloadKind: PayloadBytes, RevocationSet: RevocationSet{FamilyLifecycleChannelDisplay},
	}
	LifecycleResourceState = Family{
		ID: FamilyLifecycleResourceState, Prefix: "effect.lifecycle.resource/", Subject: Identity,
		PayloadKind: PayloadTypestate, RevocationSet: RevocationSet{FamilyLifecycleResourceState},
	}
)

// families declares every family whose keys are built or read structurally.
// Heap facts about unresolved keyed reads/writes and the native coordinate
// publications are first-class records rather than producer-local spellings.
var families = []Family{
	HeapTableIdentity,
	HeapTableClosed,
	HeapMember,
	HeapMemberIdentity,
	HeapMemberCell,
	HeapMemberOrigin,
	HeapStaticReplace,
	HeapMetaAttached,
	HeapMetaIdentity,
	HeapMetaNewIndex,
	HeapExternalCallback,
	HeapOpaqueMemberWrite,
	HeapKeysOf,
	HeapKeyedRead,
	HeapKeyedElement,
	HeapIndexPresence,
	HeapKeyPresence,
	HeapIndexRevoke,
	HeapLengthFloor,
	HeapTableEscape,
	HeapIndexLower,
	HeapIndexUpper,
	HeapIndexRelation,
	Value,
	CallResult,
	CallArgument,
	LocalCallResult,
	Type,
	DeclaredType,
	SummaryType,
	MethodReturnSummary,
	BranchProofFamily,
	IteratorElement,
	IteratorKey,
	IteratorKeySource,
	NativeConstantValue,
	NativePublicationIdentity,
	NativeBranchPartition,
	NativeTruthinessClass,
	BranchResidueClass,
	NativeConcatSite,
	NativeBuiltinCall,
	HeapAllocationDisplay,
	NativeAliasDisjoint,
	NativeCaptureEpochRoot,
	NativeCaptureTransport,
	NativeTypedProducer,
	NativeTableConstructionBound,
	NativeProjection,
	LifecycleChannelState,
	LifecycleChannelDisplay,
	LifecycleResourceState,
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

// FamilyByID resolves the stable identity used by a declaration's
// RevocationSet. Consumers never translate through prefix strings.
func FamilyByID(id FamilyID) (Family, bool) {
	family, ok := byID[id]
	return family, ok
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

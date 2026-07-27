package equation

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

// FamilyValue is one family-scoped publication. Subject and qualifiers are
// parsed zero-copy views of the key; Payload aliases the immutable fact value.
type FamilyValue struct {
	Subject     factkey.SubjectRef
	Occurrence  string
	Payload     []byte
	familyID    factkey.FamilyID
	payloadKind factkey.PayloadKind
	parsed      factkey.ParsedKey
	guarded     bool
}

func (v FamilyValue) Qualifier(index int) (factkey.SubjectRef, bool) {
	return v.parsed.Qualifier(index)
}

func (v FamilyValue) QualifierCount() int { return v.parsed.QualifierCount() }

func (v FamilyValue) OccurrenceUint(base, bitSize int) (uint64, bool) {
	value, err := strconv.ParseUint(v.Occurrence, base, bitSize)
	return value, err == nil
}

// Guarded reports whether the publication belongs to a branch edge. It lets a
// family consumer preserve the publication's control-flow provenance without
// exposing or re-parsing the guard encoding.
func (v FamilyValue) Guarded() bool { return v.guarded }

// Truth decodes a marker family through the family declaration. A family-kind
// mismatch or malformed marker fails closed to TruthUnknown.
func (v FamilyValue) Truth() factkey.Truth {
	if v.payloadKind != factkey.PayloadMarker {
		return factkey.TruthUnknown
	}
	return factkey.DecodeTruth(v.Payload)
}

// Freeze decodes the closed effect.freeze payload domain.
func (v FamilyValue) Freeze() (factkey.FreezePayload, bool) {
	if v.payloadKind != factkey.PayloadFreeze {
		return factkey.FreezePayload{}, false
	}
	return factkey.DecodeFreezePayload(v.Payload)
}

// DecodePair appends a two-position family key to dst and returns views into
// the resulting buffer. It is valid only for families with exactly one
// qualifier. Callers can reuse dst across rows and never split or base64-decode
// key storage themselves.
func (v FamilyValue) DecodePair(dst []byte) (left, right, storage []byte, ok bool) {
	if v.parsed.QualifierCount() != 1 {
		return nil, nil, dst, false
	}
	start := len(dst)
	dst, ok = v.Subject.Decode(dst)
	if !ok {
		return nil, nil, dst[:start], false
	}
	middle := len(dst)
	qualifier, present := v.parsed.Qualifier(0)
	if !present {
		return nil, nil, dst[:start], false
	}
	dst, ok = qualifier.Decode(dst)
	if !ok {
		return nil, nil, dst[:start], false
	}
	return dst[start:middle], dst[middle:], dst, true
}

type ReturnCandidateField uint8

const (
	ReturnCandidateInvalid ReturnCandidateField = iota
	ReturnCandidateArity
	ReturnCandidateSlot
)

// ReturnCandidateRow is the typed view of one return-candidate record.
type ReturnCandidateRow struct {
	Candidate string
	Field     ReturnCandidateField
	Index     int
	Arity     int
	Value     []byte
}

// ReturnMemberClosureRow is the typed view of the historical terminal-term
// subject used by return-member-closure. Candidate may itself contain path
// separators; the final subject segment is the numeric return slot.
type ReturnMemberClosureRow struct {
	Candidate  string
	ReturnSlot uint64
	Member     string
	Payload    []byte
}

func (v FamilyValue) ReturnMemberClosure() (ReturnMemberClosureRow, bool) {
	if v.familyID != factkey.ReturnMemberClosure.ID {
		return ReturnMemberClosureRow{}, false
	}
	subject := v.Subject.Spelling()
	cut := strings.LastIndexByte(subject, '/')
	if cut <= 0 || cut == len(subject)-1 {
		return ReturnMemberClosureRow{}, false
	}
	slot, err := strconv.ParseUint(subject[cut+1:], 10, 32)
	if err != nil || v.Occurrence == "" {
		return ReturnMemberClosureRow{}, false
	}
	return ReturnMemberClosureRow{
		Candidate: subject[:cut], ReturnSlot: slot,
		Member: v.Occurrence, Payload: v.Payload,
	}, true
}

// Return arities in source are overwhelmingly small and fact payloads are
// immutable after publication. Keep the common closed spellings codec-owned
// so repeated fixpoint execution does not reallocate the same sentinel bytes.
var returnCandidateArityWire = [...][]byte{
	[]byte("0"), []byte("1"), []byte("2"), []byte("3"), []byte("4"),
	[]byte("5"), []byte("6"), []byte("7"), []byte("8"),
}

// ReturnCandidateArityFact and ReturnCandidateSlotFact are the sole
// constructors for the return-candidate occurrence union.
func ReturnCandidateArityFact(candidate string, arity int) (Fact, bool) {
	if candidate == "" || arity < 0 {
		return Fact{}, false
	}
	key := factkey.ReturnCandidate.Key().String() + candidate + "/arity"
	if arity < len(returnCandidateArityWire) {
		return Fact{Key: key, Value: returnCandidateArityWire[arity]}, true
	}
	return Fact{Key: key, Value: []byte(strconv.Itoa(arity))}, true
}

func ReturnCandidateSlotFact(candidate string, index int, value []byte) (Fact, bool) {
	if candidate == "" || index < 0 || len(value) == 0 {
		return Fact{}, false
	}
	key := factkey.ReturnCandidate.Key().String() + candidate + "/" + strconv.Itoa(index)
	return Fact{Key: key, Value: value}, true
}

// ReturnCandidate decodes the arity/slot occurrence union owned by the
// return-candidate family. The legacy wire spelling remains storage only.
func (v FamilyValue) ReturnCandidate() (ReturnCandidateRow, bool) {
	if v.familyID != factkey.ReturnCandidate.ID {
		return ReturnCandidateRow{}, false
	}
	row := ReturnCandidateRow{Candidate: v.Subject.Spelling()}
	if v.Occurrence == "arity" {
		arity, err := strconv.Atoi(string(v.Payload))
		if err != nil || arity < 0 {
			return ReturnCandidateRow{}, false
		}
		row.Field, row.Arity = ReturnCandidateArity, arity
		return row, true
	}
	index, err := strconv.Atoi(v.Occurrence)
	if err != nil || index < 0 {
		return ReturnCandidateRow{}, false
	}
	row.Field, row.Index, row.Value = ReturnCandidateSlot, index, v.Payload
	return row, true
}

// DecodedPayload opts a value-payload family into the shared codec. Families
// whose declarations name another payload kind fail closed; their bytes must
// be interpreted by the codec named in factkey.Family.PayloadKind.
func (v FamilyValue) DecodedPayload() (shapefact.Payload, bool) {
	if v.payloadKind != factkey.PayloadValue {
		return shapefact.Payload{}, false
	}
	return shapefact.Decode(v.Payload)
}

// DecodedTypestatePublication opts a typestate-payload family into the domain
// codec and verifies that the typed resource repeats the key's exact identity.
// A family-kind mismatch, malformed payload, or key/payload identity mismatch
// fails closed.
func (v FamilyValue) DecodedTypestatePublication() (typestate.Publication, bool) {
	if v.payloadKind != factkey.PayloadTypestate {
		return typestate.Publication{}, false
	}
	publication, ok := typestate.DecodePublication(v.Payload)
	if !ok {
		return typestate.Publication{}, false
	}
	identity, ok := v.Subject.Decode(nil)
	if !ok || publication.Resource.ID != typestate.ResourceID(string(identity)) {
		return typestate.Publication{}, false
	}
	return publication, true
}

// FamilyValueIterator walks one binary-searched prefix range without
// materializing a fact slice or splitting keys. It is a value iterator so
// constructing and advancing it does not allocate.
type FamilyValueIterator struct {
	family factkey.Family
	prefix string
	lane   []Fact
	active []Guard
	index  int
	all    bool
}

// DecodeFamilyValue applies one family record to an already-selected fact.
// It is the slice-reader counterpart of FamilyValues and keeps callers from
// reconstructing record fields with Segments, Split, or suffix tests.
func DecodeFamilyValue(family factkey.Family, fact Fact) (FamilyValue, bool) {
	parsed, ok := family.ParseKey(fact.Key)
	if !ok {
		return FamilyValue{}, false
	}
	return FamilyValue{
		Subject:     parsed.Subject,
		Occurrence:  parsed.Occurrence,
		Payload:     fact.Value,
		familyID:    family.ID,
		payloadKind: family.PayloadKind,
		parsed:      parsed,
		guarded:     len(fact.Guards) != 0,
	}, true
}

// FamilyValues returns the visible publications selected by a typed family
// prefix built with factkey.BuildKey.
func (p Partition) FamilyValues(prefix factkey.Key) FamilyValueIterator {
	return p.familyValues(prefix, false)
}

// AllFamilyValues is the unfiltered-history counterpart used by may-fact
// families whose guarded publication remains possible after reconvergence.
func (p Partition) AllFamilyValues(prefix factkey.Key) FamilyValueIterator {
	return p.familyValues(prefix, true)
}

func (p Partition) familyValues(prefix factkey.Key, all bool) FamilyValueIterator {
	family, ok := prefix.Family()
	if !ok {
		return FamilyValueIterator{}
	}
	text := prefix.String()
	view := p.view()
	lane := p.closure.Values
	if view.orderedValues() {
		start, end := prefixRange(lane, text)
		lane = lane[start:end:end]
	}
	var active []Guard
	if !all {
		active = view.activeGuards()
	}
	return FamilyValueIterator{
		family: family,
		prefix: text,
		lane:   lane,
		active: active,
		all:    all,
	}
}

// Next returns the next structurally valid row. Malformed keys fail closed and
// are skipped rather than being exposed as a different subject.
func (it *FamilyValueIterator) Next() (FamilyValue, bool) {
	for it.index < len(it.lane) {
		fact := it.lane[it.index]
		it.index++
		if !strings.HasPrefix(fact.Key, it.prefix) || (!it.all && !guardsIncluded(fact.Guards, it.active)) {
			continue
		}
		parsed, ok := it.family.ParseKey(fact.Key)
		if !ok {
			continue
		}
		return FamilyValue{
			Subject:     parsed.Subject,
			Occurrence:  parsed.Occurrence,
			Payload:     fact.Value,
			familyID:    it.family.ID,
			payloadKind: it.family.PayloadKind,
			parsed:      parsed,
			guarded:     len(fact.Guards) != 0,
		}, true
	}
	return FamilyValue{}, false
}

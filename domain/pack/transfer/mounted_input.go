package transfer

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const mountedInputDomain = "wippy.analysis.pack.mounted-input-source.v2\x00"

// MountedInput is the minimal neutral row joining one Target InputSource to a
// mounted Pack call. It retains only scalar provenance and the exact ordered
// mounted semantic-source members selected from that call. Value owns the
// subsequent coordinate lookup; Heap/Placement own allocation-root lookup.
//
// In particular, this row does not retain InputSelector, Pack Schema, Target,
// Link, runtime context, or a placement conclusion. The open bit is an
// independent fact about an actual tail producer. It never causes a tail ID to
// be fabricated as a Value member.
type MountedInput struct {
	module    identity.ContentID
	call      identity.ContentID
	operation vocabulary.Operation
	source    vocabulary.InputSource
	members   []identity.ContentID
	open      bool
	owner     identity.ContentID
	id        identity.ContentID
	sealed    bool
}

func mountedInputID(owner, module, call identity.ContentID, members []identity.ContentID, open bool, operation vocabulary.Operation, source vocabulary.InputSource) identity.ContentID {
	if !owner.Available() || !module.Available() || !call.Available() || !mountedInputSourceValid(source) {
		return identity.ContentID{}
	}
	for _, member := range members {
		if !member.Available() {
			return identity.ContentID{}
		}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(mountedInputDomain))
	for _, value := range [...]identity.ContentID{owner, module, call} {
		_, _ = hash.Write(value[:])
	}
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], uint32(len(members)))
	_, _ = hash.Write(count[:])
	for _, member := range members {
		_, _ = hash.Write(member[:])
	}
	if open {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
	var scalar [12]byte
	binary.BigEndian.PutUint32(scalar[0:4], uint32(operation))
	binary.BigEndian.PutUint32(scalar[4:8], uint32(source.Kind))
	binary.BigEndian.PutUint32(scalar[8:12], source.Ordinal)
	_, _ = hash.Write(scalar[:])
	return identity.ContentID(sha256.Sum256(hash.Sum(nil)))
}

func mountedInputSourceValid(source vocabulary.InputSource) bool {
	switch source.Kind {
	case vocabulary.InputSourceValueFormal, vocabulary.InputSourceValuesVar:
		return true
	case vocabulary.InputSourceAllInputs:
		return source.Ordinal == 0
	default:
		return false
	}
}

// NewMountedInput validates one Target source against Pack's exact mounted
// call. It is the source-plane constructor used when a caller needs to map a
// transfer payload/alias independently of a scalar runtime binding.
//
// ValueFormal selects exactly one closed member when the call authors a fixed
// actual at that position. When it does not, the projection has no member and
// its open bit carries the exact Lua reading: open when an actual tail may
// populate the position, closed when under-application proves the formal nil.
// ValuesVar selects the exact ordered fixed actual suffix beginning at
// selector.start and sets open only when the call has an authenticated actual
// tail. AllInputs follows the same rule from selector.start (zero). The actual
// tail identity is deliberately used only as the open witness; it is never
// published as a Value member.
func NewMountedInput(schema *packdomain.Schema, module, call identity.ContentID, operation vocabulary.Operation, source vocabulary.InputSource) (MountedInput, bool) {
	if schema == nil || !module.Available() || !call.Available() || operation == 0 {
		return MountedInput{}, false
	}
	if _, callOK := schema.CallRootForMountedSemantic(module, call); !callOK {
		return MountedInput{}, false
	}
	selector, selectorOK := schema.InputSelector(operation, source)
	if !mountedInputSourceValid(source) || !selectorOK || !schema.OwnsInputSelector(selector) {
		return MountedInput{}, false
	}
	actual, actualOK := schema.MountedActualProjection(module, call)
	if !actualOK || !actual.Valid() {
		return MountedInput{}, false
	}
	members := make([]identity.ContentID, 0, 1)
	open := false
	if source.Kind == vocabulary.InputSourceValueFormal {
		start, startOK := selector.Start()
		if !startOK || start < 0 {
			return MountedInput{}, false
		}
		if start < actual.ActualCount() {
			member, memberOK := actual.ActualAt(start)
			if !memberOK || !member.Available() || member.Module() != module {
				return MountedInput{}, false
			}
			members = append(members, member.ID())
		} else {
			// The call authors no fixed actual at this formal position. An
			// actual tail reaches every position past the fixed row, so its
			// presence makes the formal statically unknown; without one, Lua
			// under-application proves the formal nil.
			_, open = actual.TailID()
		}
	} else {
		start, startOK := selector.Start()
		if !startOK {
			return MountedInput{}, false
		}
		for index := start; index < actual.ActualCount(); index++ {
			member, memberOK := actual.ActualAt(index)
			if !memberOK || !member.Available() || member.Module() != module {
				return MountedInput{}, false
			}
			members = append(members, member.ID())
		}
		_, open = actual.TailID()
	}
	owner := schema.LinkOwner().ContentID()
	row := MountedInput{module: module, call: call, operation: operation, source: source, members: members, open: open, owner: owner}
	row.id = mountedInputID(owner, module, call, members, open, operation, source)
	row.sealed = row.id.Available()
	return row, row.Valid()
}

func (input MountedInput) valid() bool {
	if !input.sealed || !input.owner.Available() || !input.module.Available() || !input.call.Available() || input.operation == 0 || !mountedInputSourceValid(input.source) || !input.id.Available() {
		return false
	}
	if input.source.Kind == vocabulary.InputSourceValueFormal {
		switch len(input.members) {
		case 1:
			// A position filled by a fixed actual cannot also be reached by
			// the tail, so exactly-one-member is always closed.
			if input.open {
				return false
			}
		case 0:
			// Zero members closed is the proven-nil formal; zero members open
			// is the tail-fed formal whose value is statically unknown.
		default:
			return false
		}
	}
	for _, member := range input.members {
		if !member.Available() {
			return false
		}
	}
	return input.id == mountedInputID(input.owner, input.module, input.call, input.members, input.open, input.operation, input.source)
}

// Valid reports that the row was issued by NewMountedInput and its scalar
// seal is intact.  Value/Heap owner joins must still authenticate the row's
// module and semantic ID against their own exact schemas.
func (input MountedInput) Valid() bool { return input.valid() }

func (input MountedInput) ContentID() (identity.ContentID, bool) {
	return input.id, input.valid()
}

// Equal compares two sealed mounted projections by their complete canonical
// identity. The identity includes ordered members and the independent open
// bit, so equal scalar provenance with a different heterogeneous shape does
// not compare equal.
func (input MountedInput) Equal(other MountedInput) bool {
	left, leftOK := input.ContentID()
	right, rightOK := other.ContentID()
	return leftOK && rightOK && left == right
}

func (input MountedInput) Module() (identity.ContentID, bool) {
	return input.module, input.valid()
}

// OwnerID is the Link owner identity admitted by Pack when this row was
// issued. Consumers use it as the scalar cross-schema fence before asking
// Value for a mounted coordinate.
func (input MountedInput) OwnerID() (identity.ContentID, bool) {
	return input.owner, input.valid()
}

func (input MountedInput) Source() (vocabulary.InputSource, bool) {
	return input.source, input.valid()
}

func (input MountedInput) IsOpen() bool {
	return input.valid() && input.open
}

// IsProvenNil reports the Lua under-application shape: the mounted call
// authors no fixed actual at this ValueFormal position and carries no actual
// tail that could reach it, so the formal holds nil. This is a positive fact
// about the call, not a missing join; a consumer reads it as the nil value and
// never as an unknown. A ValuesVar/AllInputs projection with no member selects
// an empty value list, which is a different fact and never reported here.
func (input MountedInput) IsProvenNil() bool {
	return input.valid() && input.source.Kind == vocabulary.InputSourceValueFormal && len(input.members) == 0 && !input.open
}

// MemberCount reports the exact number of fixed semantic-source members in
// this call-specific projection. Open does not add a fabricated member.
func (input MountedInput) MemberCount() int {
	if !input.valid() {
		return 0
	}
	return len(input.members)
}

// MemberAt returns one ordered fixed semantic-source member ID.
func (input MountedInput) MemberAt(index int) (identity.ContentID, bool) {
	if !input.valid() || index < 0 || index >= len(input.members) {
		return identity.ContentID{}, false
	}
	return input.members[index], true
}

// CoordinateForInput joins one closed neutral Pack input row with exactly one
// member to Value's exact mounted coordinate. Heterogeneous rows must use
// CoordinateForInputMember for each member; this function never guesses a
// coordinate for an empty or multi-member row.
func CoordinateForInput(values *valuedomain.Schema, input MountedInput) (valuedomain.Coordinate, bool) {
	if input.MemberCount() != 1 || input.IsOpen() {
		return valuedomain.Coordinate{}, false
	}
	return CoordinateForInputMember(values, input, 0)
}

// CoordinateForInputMember joins one exact fixed member of a mounted input to
// Value. The Link owner is the cross-schema fence: equal module and semantic
// IDs from a separately sealed schema cannot acquire a local coordinate.
func CoordinateForInputMember(values *valuedomain.Schema, input MountedInput, memberIndex int) (valuedomain.Coordinate, bool) {
	if values == nil || !values.Valid() || !input.Valid() {
		return valuedomain.Coordinate{}, false
	}
	owner, ownerOK := input.OwnerID()
	module, moduleOK := input.Module()
	semantic, semanticOK := input.MemberAt(memberIndex)
	if !ownerOK || owner != values.LinkID() || !moduleOK || !semanticOK || !semantic.Available() {
		return valuedomain.Coordinate{}, false
	}
	coordinate, coordinateOK := values.CoordinateForMountedSemantic(module, semantic)
	return coordinate, coordinateOK && coordinate.Valid()
}

// CoordinatesForInput returns every closed fixed-member coordinate in source
// order. An open tail contributes no fabricated coordinate; callers decide
// whether the independent open fact widens their result.
func CoordinatesForInput(values *valuedomain.Schema, input MountedInput) ([]valuedomain.Coordinate, bool) {
	if values == nil || !values.Valid() || !input.Valid() {
		return nil, false
	}
	coordinates := make([]valuedomain.Coordinate, input.MemberCount())
	for index := range coordinates {
		coordinate, ok := CoordinateForInputMember(values, input, index)
		if !ok {
			return nil, false
		}
		coordinates[index] = coordinate
	}
	return coordinates, true
}

// SummaryValueAtInput reads one fixed mounted source from Value's detached
// summary vector. It is the one-member convenience form; heterogeneous rows
// use SummaryValuesAtInput.
func SummaryValueAtInput(values *valuedomain.Schema, summary valuedomain.ValueSummaryObservation, input MountedInput) (valuedomain.Value, bool, bool) {
	if input.MemberCount() != 1 || input.IsOpen() {
		return valuedomain.Value{}, false, false
	}
	return SummaryValueAtInputMember(values, summary, input, 0)
}

// SummaryValueAtInputMember reads one exact fixed member from Value's
// detached summary vector. Open inputs intentionally have no readable tail
// fact; known fixed members remain readable when an input is open. A missing
// fixed summary cell is an unavailable join, not a Bottom value: callers may
// widen only from input.IsOpen(), which is an independently authenticated
// Pack tail fact.
func SummaryValueAtInputMember(values *valuedomain.Schema, summary valuedomain.ValueSummaryObservation, input MountedInput, memberIndex int) (valuedomain.Value, bool, bool) {
	if values == nil || !values.Valid() || !values.OwnsSummaryObservation(summary) || !input.Valid() {
		return valuedomain.Value{}, false, false
	}
	coordinate, coordinateOK := CoordinateForInputMember(values, input, memberIndex)
	index, indexOK := values.CoordinateIndex(coordinate)
	if !coordinateOK || !indexOK || int(index) >= len(summary.Values) {
		return valuedomain.Value{}, false, false
	}
	if !summary.Present[index] {
		return valuedomain.Value{}, false, false
	}
	fact := summary.Values[index]
	return fact, true, values.AdmitsCoordinate(coordinate, fact)
}

// SummaryValuesAtInput folds every exact fixed member into one Value fact.
// Presence is true only when every fixed member has a present summary cell;
// an incomplete fixed-member join is unavailable, never replaced by Bottom or
// a guessed tail value. If the input is also open, the returned finite-member
// fact remains useful evidence, while callers must separately inspect IsOpen
// before treating the aggregate as exhaustive.
func SummaryValuesAtInput(values *valuedomain.Schema, summary valuedomain.ValueSummaryObservation, input MountedInput) (valuedomain.Value, bool, bool) {
	if values == nil || !values.Valid() || !values.OwnsSummaryObservation(summary) || !input.Valid() {
		return valuedomain.Value{}, false, false
	}
	if input.MemberCount() == 0 {
		return values.Bottom(), false, true
	}
	var result valuedomain.Value
	haveResult := false
	for index := 0; index < input.MemberCount(); index++ {
		fact, memberPresent, readable := SummaryValueAtInputMember(values, summary, input, index)
		if !readable {
			return valuedomain.Value{}, false, false
		}
		if !memberPresent {
			// SummaryValueAtInputMember currently reports this as unavailable;
			// retain the explicit guard so a future member reader cannot turn an
			// incomplete heterogeneous row into an aggregate Bottom.
			return valuedomain.Value{}, false, false
		}
		if !haveResult {
			result = fact
			haveResult = true
			continue
		}
		joined, joinOK := values.Join(result, fact)
		if !joinOK {
			return valuedomain.Value{}, false, false
		}
		result = joined
	}
	if !haveResult {
		return valuedomain.Value{}, false, false
	}
	return result, true, true
}

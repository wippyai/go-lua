package contract

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
)

// The suspension payload format. It is owned here for the reason the rule
// delegation and the export path are: everything it carries is a reference or a
// policy, and none of it is a type.
//
// A suspension relates two OUTCOME cases of the member it attaches to - the case
// control leaves at, and the case it re-enters at - together with the authority
// that restores it and the multiplicity of that restoration. The two outcome
// cases are named rather than numbered: the structural vocabulary surface owns
// the closed outcome catalog, so this payload writes the sealed member's own key
// and a reader resolves it against that surface. Minting an ordinal here would
// have been a second outcome vocabulary with nothing keeping it in step with the
// declared one.
//
// The policy half is this format's own. Which authority may restore a suspended
// member - a matching call, or the member's own provider - and whether a live
// suspension is discharged by its first restoration are statements about the
// published member, not propositions about which values inhabit a set, so they
// are contract data and are written here as closed vocabularies.
const (
	suspensionDomain  = "analysis/library/contract/suspension"
	suspensionVersion = 1
)

// ReentrySource is the closed authority that may restore one suspended member.
type ReentrySource uint8

const (
	ReentrySourceInvalid ReentrySource = iota
	// ReentryByCall is restored by a call that matches the suspended member
	// dynamically: the caller supplies the re-entry.
	ReentryByCall
	// ReentryByProvider is restored by the member's own provider, which is the
	// authority the contract publishes the member under rather than anything a
	// caller reaches.
	ReentryByProvider
	reentrySourceLimit
)

func (source ReentrySource) Available() bool {
	return source > ReentrySourceInvalid && source < reentrySourceLimit
}

// ReentryMultiplicity is whether one live suspension survives its restoration.
type ReentryMultiplicity uint8

const (
	ReentryMultiplicityInvalid ReentryMultiplicity = iota
	// ReentryOnce is discharged by its first restoration.
	ReentryOnce
	// ReentryMany remains available for later restorations.
	ReentryMany
	reentryMultiplicityLimit
)

func (multiplicity ReentryMultiplicity) Available() bool {
	return multiplicity > ReentryMultiplicityInvalid && multiplicity < reentryMultiplicityLimit
}

// Suspension is one suspension payload: the outcome control leaves at, the
// outcome it re-enters at, and the policy under which the re-entry happens.
type Suspension struct {
	// Yield names the sealed outcome member control leaves the callable at.
	Yield schema.Key
	// Reentry names the sealed outcome member control re-enters at.
	Reentry schema.Key
	// Source is the authority that may restore the suspension.
	Source ReentrySource
	// Multiplicity is whether the restored suspension survives.
	Multiplicity ReentryMultiplicity
}

// Available reports whether this row relates two distinct outcome cases under a
// declared policy. One outcome named twice is not a relation: a member that left
// and re-entered at the same case never suspended.
func (suspension Suspension) Available() bool {
	return suspension.Yield.Available() && suspension.Reentry.Available() &&
		suspension.Yield != suspension.Reentry &&
		suspension.Source.Available() && suspension.Multiplicity.Available()
}

// EncodeSuspension writes one suspension as a member payload body. The result is
// a complete framed stream: a payload is decodable on its own, so a reader that
// holds a member and its declared format never needs the enclosing instance to
// interpret it.
func EncodeSuspension(suspension Suspension) ([]byte, error) {
	if !suspension.Available() {
		return nil, ErrMalformed
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, suspensionDomain, suspensionVersion); err != nil {
		return nil, err
	}
	if err := writer.String(string(suspension.Yield)); err != nil {
		return nil, err
	}
	if err := writer.String(string(suspension.Reentry)); err != nil {
		return nil, err
	}
	if err := writer.Uint(uint64(suspension.Source)); err != nil {
		return nil, err
	}
	if err := writer.Uint(uint64(suspension.Multiplicity)); err != nil {
		return nil, err
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// DecodeSuspension reads one suspension payload body.
func DecodeSuspension(data []byte) (Suspension, error) {
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		return Suspension{}, ErrMalformed
	}
	if err := reader.Header(suspensionDomain, suspensionVersion); err != nil {
		return Suspension{}, ErrMalformed
	}
	yield, err := reader.String(maxKey)
	if err != nil {
		return Suspension{}, ErrMalformed
	}
	reentry, err := reader.String(maxKey)
	if err != nil {
		return Suspension{}, ErrMalformed
	}
	source, err := reader.Uint()
	if err != nil || source > uint64(^uint8(0)) {
		return Suspension{}, ErrMalformed
	}
	multiplicity, err := reader.Uint()
	if err != nil || multiplicity > uint64(^uint8(0)) {
		return Suspension{}, ErrMalformed
	}
	if err := reader.Finish(); err != nil {
		return Suspension{}, ErrMalformed
	}
	row := Suspension{
		Yield:        schema.Key(yield),
		Reentry:      schema.Key(reentry),
		Source:       ReentrySource(source),
		Multiplicity: ReentryMultiplicity(multiplicity),
	}
	if !row.Available() {
		return Suspension{}, ErrMalformed
	}
	return row, nil
}

// SuspensionOutcomeEntryID derives the declaration-table identity one suspension
// outcome reference names. It states once which surface an outcome reference
// resolves against, so a reader resolving a member never restates the surface
// the reference belongs to.
func SuspensionOutcomeEntryID(outcome schema.Key) schema.EntryID {
	return schema.NewEntryID(schema.SurfaceKindStructure, outcome)
}

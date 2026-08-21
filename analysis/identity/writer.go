package identity

// IdentityWriter is the neutral primitive sink used when an owning schema
// replays an identity preimage. The schema owns field order and validation;
// the caller owns the domain framing and digest implementation.
//
// Each method commits exactly one canonical primitive field and reports
// whether the sink still accepts the preimage. A writer must fail closed after
// the first rejected field.
type IdentityWriter interface {
	WriteContentID(ContentID) bool
	WriteUint(uint64) bool
	WriteBool(bool) bool
}

// StringIdentityWriter extends IdentityWriter for canonical row families
// whose identity preimages contain source names or other owned text. Keeping
// this separate preserves the smaller primitive contract used by row families
// that never replay text.
type StringIdentityWriter interface {
	IdentityWriter
	WriteString(string) bool
}

package factor

import "github.com/wippyai/go-lua/analysis/identity"

const (
	effectResultHeaderSize = 8 + 1 + 1 + 8
	effectResultAtomSize   = 32
)

// Result is an immutable view over one encoded Effect result. It retains the
// caller's string as its backing storage and never materializes an atom slice.
// A Result's zero value is unavailable.
type Result struct {
	payload string
	count   uint64
	present bool
	top     bool
	valid   bool
}

// DecodeResult validates one EncodeResult payload and returns a zero-copy
// view of its public metadata and atom projection. The payload string must be
// kept alive by the caller for as long as the view is used.
func DecodeResult(present bool, rows uint64, payload string) (Result, bool) {
	if rows > 1 || len(payload) < effectResultHeaderSize {
		return Result{}, false
	}
	if effectResultUint64(payload, 0) != effectResultFormat {
		return Result{}, false
	}
	presentByte, topByte := payload[8], payload[9]
	if presentByte > 1 || topByte > 1 || presentByte != effectResultBoolByte(present) {
		return Result{}, false
	}
	count := effectResultUint64(payload, 10)
	bodySize := len(payload) - effectResultHeaderSize
	if bodySize%effectResultAtomSize != 0 || count != uint64(bodySize/effectResultAtomSize) {
		return Result{}, false
	}
	top := topByte == 1
	if !present && (top || count != 0) || top && count != 0 {
		return Result{}, false
	}
	for index := uint64(0); index < count; index++ {
		start := effectResultHeaderSize + int(index)*effectResultAtomSize
		var id identity.ContentID
		copy(id[:], payload[start:start+effectResultAtomSize])
		if !id.Available() {
			return Result{}, false
		}
	}
	return Result{payload: payload, count: count, present: present, top: top, valid: true}, true
}

func effectResultUint64(payload string, offset int) uint64 {
	return uint64(payload[offset])<<56 |
		uint64(payload[offset+1])<<48 |
		uint64(payload[offset+2])<<40 |
		uint64(payload[offset+3])<<32 |
		uint64(payload[offset+4])<<24 |
		uint64(payload[offset+5])<<16 |
		uint64(payload[offset+6])<<8 |
		uint64(payload[offset+7])
}

func effectResultBoolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

// Available reports whether the view passed DecodeResult validation.
func (result Result) Available() bool { return result.valid }

// Present reports whether the encoded result carries a present value.
func (result Result) Present() bool { return result.valid && result.present }

// Top reports whether the present value is the algebra's top value.
func (result Result) Top() bool { return result.valid && result.top }

// AtomCount reports the number of portable atom IDs in the view.
func (result Result) AtomCount() int {
	if !result.valid {
		return 0
	}
	return int(result.count)
}

// AtomAt returns one portable atom ID without allocating or exposing the
// encoded payload. The returned identity is a value copy of its fixed-width
// wire slot.
func (result Result) AtomAt(index int) (identity.ContentID, bool) {
	if !result.valid || index < 0 || uint64(index) >= result.count {
		return identity.ContentID{}, false
	}
	start := effectResultHeaderSize + index*effectResultAtomSize
	var id identity.ContentID
	copy(id[:], result.payload[start:start+effectResultAtomSize])
	return id, true
}

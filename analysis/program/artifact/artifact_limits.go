package artifact

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/internal/framing"
)

const (
	artifactDomain = "program/artifact"
	// v21 adds the Source-owned authored debug-spelling record to the fixed
	// quartet payload grammar. Older streams are intentionally rejected; the
	// artifact codec has no compatibility representation.
	artifactVersion = 21

	// These limits are deliberately owned by this package. They bound both
	// the encoded byte stream and the amount of reconstruction work admitted
	// from an untrusted byte stream.
	artifactMaxBytes   = 256 << 20
	artifactMaxEvents  = 32 << 20
	artifactMaxStrings = 64 << 20
	artifactMaxInt     = int(^uint(0) >> 1)
	// Record + String + Bytes(32-byte ContentID), using canonical's smallest
	// one-byte length varint for each frame. This is a pre-allocation floor;
	// the child decoders perform their own exact row preflights.
	artifactDependencyMin = 3 + 3 + 34
)

var (
	ErrUnavailableTarget  = errors.New("program artifact: unavailable target contract")
	ErrUnavailableProgram = errors.New("program artifact: unavailable Program")
	ErrTargetMismatch     = errors.New("program artifact: target identity mismatch")
	ErrDependencyMismatch = errors.New("program artifact: dependency manifest mismatch")
	ErrNoncanonical       = errors.New("program artifact: noncanonical encoding")
	ErrLimit              = errors.New("program artifact: resource limit")
)

func artifactMeasureAllowed(measure framing.StreamMeasure) bool {
	return measure.Events <= artifactMaxEvents && measure.StringBytes <= artifactMaxStrings
}

// artifactBuffer is the local all-or-nothing persistence sink. A Writer error
// never exposes its partially filled bytes because Encode returns nil on all
// failures.
type artifactBuffer struct {
	data  []byte
	limit int
}

func newArtifactBuffer(limit int) *artifactBuffer { return &artifactBuffer{limit: limit} }

func (buffer *artifactBuffer) Write(value []byte) (int, error) {
	if buffer == nil || buffer.limit < 0 || len(value) > buffer.limit-len(buffer.data) {
		return 0, ErrLimit
	}
	if !buffer.reserve(len(value)) {
		return 0, ErrLimit
	}
	buffer.data = append(buffer.data, value...)
	return len(value), nil
}

func (buffer *artifactBuffer) WriteString(value string) (int, error) {
	if buffer == nil || buffer.limit < 0 || len(value) > buffer.limit-len(buffer.data) {
		return 0, ErrLimit
	}
	if !buffer.reserve(len(value)) {
		return 0, ErrLimit
	}
	buffer.data = append(buffer.data, value...)
	return len(value), nil
}

func (buffer *artifactBuffer) reserve(extra int) bool {
	if buffer == nil || buffer.limit < 0 || extra < 0 || len(buffer.data) > buffer.limit-extra {
		return false
	}
	need := len(buffer.data) + extra
	if cap(buffer.data) >= need {
		return true
	}
	// Grow geometrically for normal streams, but clamp capacity to the hard
	// persistence ceiling instead of letting append overshoot it.
	capacity := cap(buffer.data) * 2
	if capacity < need {
		capacity = need
	}
	if capacity > buffer.limit {
		capacity = buffer.limit
	}
	next := make([]byte, len(buffer.data), capacity)
	copy(next, buffer.data)
	buffer.data = next
	return true
}

func (buffer *artifactBuffer) Bytes() []byte {
	if buffer == nil {
		return nil
	}
	return buffer.data
}

func encodeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrLimit) || errors.Is(err, framing.ErrLimit) {
		return ErrLimit
	}
	return ErrUnavailableProgram
}

func decodeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrTargetMismatch) {
		return ErrTargetMismatch
	}
	if errors.Is(err, ErrDependencyMismatch) {
		return ErrDependencyMismatch
	}
	if errors.Is(err, ErrLimit) || errors.Is(err, framing.ErrLimit) {
		return ErrLimit
	}
	if errors.Is(err, ErrNoncanonical) {
		return ErrNoncanonical
	}
	return fmt.Errorf("%w: %v", ErrNoncanonical, err)
}

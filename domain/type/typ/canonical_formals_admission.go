package typ

import "context"

// canonicalFormalsAdmission carries cancellation through the scoped artifact
// boundary.  It deliberately has no semantic size, definition, edge, or work
// budget: a canonical artifact is admitted whenever its caller-provided input
// and every derived allocation are representable by this Go process.
type canonicalFormalsAdmission struct {
	ctx   context.Context
	steps uint64
}

const (
	// Retention is not admission: pooled encoder scratch above this size is
	// dropped after use so one valid large artifact cannot pin memory forever.
	canonicalFormalsRetainBytes = 1 << 20

	canonicalFormalsIntBytes       = 8
	canonicalFormalsTypeBytes      = 16 // interface header on supported Go targets
	canonicalFormalsMapEntryBytes  = 48 // deliberately conservative map bucket share
	canonicalFormalsNodeBytes      = 64
	canonicalFormalsShapeBytes     = 32
	canonicalFormalsFrameBytes     = 24
	canonicalFormalsBoolBytes      = 1
	canonicalFormalsPointerBytes   = 8
	canonicalFormalsMethodBytes    = 32
	canonicalFormalsFieldBytes     = 48
	canonicalFormalsStaticMemBytes = 48
	canonicalFormalsParamBytes     = 40
)

func newCanonicalFormalsAdmission(ctx context.Context, raw int) (*canonicalFormalsAdmission, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if raw < 0 {
		return nil, invalidCanonicalFormals("negative raw artifact length")
	}
	return &canonicalFormalsAdmission{ctx: ctx}, nil
}

func (a *canonicalFormalsAdmission) checkpoint() error {
	if a == nil {
		return nil
	}
	return canonicalDecodeCheckpoint(a.ctx, &a.steps)
}

// reserve is the scoped boundary's pre-allocation representability check.
// Count and element size are kept separate so every caller must state the
// representation it is about to allocate rather than hiding a multiplication
// beside make/append.
func (a *canonicalFormalsAdmission) reserve(count, elementBytes int) error {
	if _, ok := canonicalFormalsAllocationBytes(count, elementBytes); !ok {
		return invalidCanonicalFormals("allocation admission")
	}
	return nil
}


func canonicalFormalsAllocationBytes(count, elementBytes int) (int, bool) {
	if count < 0 || elementBytes < 0 || count == 0 || elementBytes == 0 {
		return 0, count >= 0 && elementBytes >= 0
	}
	if count > maxInt()/elementBytes {
		return 0, false
	}
	return count * elementBytes, true
}

func canonicalFormalsCapacityExceeds(capacity, elementBytes, threshold int) bool {
	return capacity < 0 || elementBytes < 0 || threshold < 0 || (elementBytes != 0 && capacity > threshold/elementBytes)
}

// canonicalFormalsPreflight makes cancellation and representability happen
// immediately before a private representation is allocated.  It intentionally
// does not silently accept a nil admission: every scoped production path has
// one object shared from entry through validation, materialization, and the
// mandatory re-encode.  Nil remains useful only for narrow package tests of
// context behavior.
func canonicalFormalsPreflight(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, count, elementBytes int) error {
	if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
		return err
	}
	if admission == nil {
		return nil
	}
	return admission.reserve(count, elementBytes)
}

// canonicalFormalsAppend is the scoped boundary's checked grow path for
// private slices whose final cardinality is discovered incrementally.  It
// never lets append choose an attacker-sized reallocation behind an unchecked
// call site: the next backing capacity is admitted and copied in small,
// cancellable chunks first.
func canonicalFormalsAppend[T any](ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, values []T, value T, elementBytes int) ([]T, error) {
	if len(values) == cap(values) {
		next := 16
		if cap(values) != 0 {
			if cap(values) > maxInt()/2 {
				if cap(values) == maxInt() {
					return nil, invalidCanonicalFormals("slice admission")
				}
				next = cap(values) + 1
			} else {
				next = cap(values) * 2
			}
		}
		if err := canonicalFormalsPreflight(ctx, admission, steps, next, elementBytes); err != nil {
			return nil, err
		}
		grown := make([]T, len(values), next)
		for start := 0; start < len(values); start += 64 {
			if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
				return nil, err
			}
			end := start + 64
			if end > len(values) {
				end = len(values)
			}
			copy(grown[start:end], values[start:end])
		}
		values = grown
	}
	return append(values, value), nil
}

// canonicalFormalsAppendBytes is the byte-slice counterpart of
// canonicalFormalsAppend. Output is discovered incrementally, so it must grow
// through the same checked path rather than letting append allocate an
// unvalidated backing array.
func canonicalFormalsAppendBytes(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, values []byte, sources ...[]byte) ([]byte, error) {
	need := len(values)
	for _, source := range sources {
		if len(source) > maxInt()-need {
			return nil, invalidCanonicalFormals("byte slice admission")
		}
		need += len(source)
	}
	if cap(values) < need {
		next := 16
		if cap(values) != 0 {
			if cap(values) > maxInt()/2 {
				next = need
			} else {
				next = cap(values) * 2
			}
		}
		if next < need {
			next = need
		}
		if err := canonicalFormalsPreflight(ctx, admission, steps, next, 1); err != nil {
			return nil, err
		}
		grown := make([]byte, len(values), next)
		for start := 0; start < len(values); start += 64 {
			if err := canonicalFormalsCheckpoint(ctx, admission, steps); err != nil {
				return nil, err
			}
			end := start + 64
			if end > len(values) {
				end = len(values)
			}
			copy(grown[start:end], values[start:end])
		}
		values = grown
	}
	for _, source := range sources {
		values = append(values, source...)
	}
	return values, nil
}

func canonicalFormalsCheckpoint(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64) error {
	if admission != nil {
		return admission.checkpoint()
	}
	if ctx == nil {
		return nil
	}
	return canonicalDecodeCheckpoint(ctx, steps)
}

func canonicalFormalsClone(ctx context.Context, admission *canonicalFormalsAdmission, source []byte) ([]byte, error) {
	var steps uint64
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(source), 1); err != nil {
		return nil, err
	}
	destination := make([]byte, len(source))
	for start := 0; start < len(source); start += 64 {
		if err := canonicalFormalsCheckpoint(ctx, admission, &steps); err != nil {
			return nil, err
		}
		end := start + 64
		if end > len(source) {
			end = len(source)
		}
		copy(destination[start:end], source[start:end])
	}
	return destination, nil
}

func canonicalFormalsEqual(ctx context.Context, admission *canonicalFormalsAdmission, left, right []byte) (bool, error) {
	if len(left) != len(right) {
		return false, nil
	}
	var steps uint64
	for index := range left {
		if err := canonicalFormalsCheckpoint(ctx, admission, &steps); err != nil {
			return false, err
		}
		if left[index] != right[index] {
			return false, nil
		}
	}
	return true, nil
}

func canonicalFormalsUvarintSize(value uint64) int {
	size := 1
	for value >= 0x80 {
		value >>= 7
		size++
	}
	return size
}

func (e *canonicalEncoder) appendCanonicalTypes(destination, source []Type) ([]Type, error) {
	need := len(destination) + len(source)
	if need < len(destination) || need < len(source) {
		return nil, invalidCanonicalFormals("child admission")
	}
	if cap(destination) < need {
		if err := canonicalFormalsPreflight(e.ctx, e.admission, &e.steps, need, canonicalFormalsTypeBytes); err != nil {
			return nil, err
		}
		grown := make([]Type, len(destination), need)
		for start := 0; start < len(destination); start += 64 {
			if err := e.checkpoint(); err != nil {
				return nil, err
			}
			end := start + 64
			if end > len(destination) {
				end = len(destination)
			}
			copy(grown[start:end], destination[start:end])
		}
		destination = grown
	}
	for _, value := range source {
		if err := e.checkpoint(); err != nil {
			return nil, err
		}
		destination = append(destination, value)
	}
	return destination, nil
}

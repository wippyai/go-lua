package hash

import "strconv"

// FNV-1a constants.
const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// MixHash combines two hashes using FNV-1a style mixing.
func MixHash(h, v uint64) uint64 {
	h ^= v
	h *= fnvPrime64

	return h
}

// FnvString hashes a string using FNV-1a.
func FnvString(s string) uint64 {
	var h uint64 = fnvOffset64
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}

	return h
}

// Writer streams bytes into an FNV-1a hash without forcing callers to allocate
// temporary byte slices for strings or formatted scalar values.
type Writer struct {
	h uint64
}

// NewWriter returns an FNV-1a writer seeded like fnv.New64a.
func NewWriter() Writer {
	return Writer{h: fnvOffset64}
}

// Sum64 returns the current FNV-1a digest.
func (w *Writer) Sum64() uint64 {
	if w == nil {
		return 0
	}
	return w.h
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	for _, b := range p {
		w.mixByte(b)
	}
	return len(p), nil
}

// WriteString writes s directly into the hash.
func (w *Writer) WriteString(s string) (int, error) {
	for i := range len(s) {
		w.mixByte(s[i])
	}
	return len(s), nil
}

// WriteByte writes one byte into the hash.
func (w *Writer) WriteByte(b byte) error {
	w.mixByte(b)
	return nil
}

func (w *Writer) mixByte(b byte) {
	w.h ^= uint64(b)
	w.h *= fnvPrime64
}

// WriteUintDecimal writes v in base-10 ASCII form.
func (w *Writer) WriteUintDecimal(v uint64) {
	var buf [20]byte
	out := strconv.AppendUint(buf[:0], v, 10)
	_, _ = w.Write(out)
}

// WriteIntDecimal writes v in base-10 ASCII form.
func (w *Writer) WriteIntDecimal(v int64) {
	var buf [20]byte
	out := strconv.AppendInt(buf[:0], v, 10)
	_, _ = w.Write(out)
}

// WriteUintHex writes v in lower-case hexadecimal ASCII form.
func (w *Writer) WriteUintHex(v uint64) {
	var buf [16]byte
	out := strconv.AppendUint(buf[:0], v, 16)
	_, _ = w.Write(out)
}

// WriteBool writes b using fmt's %t spelling.
func (w *Writer) WriteBool(b bool) {
	if b {
		_, _ = w.WriteString("true")
		return
	}
	_, _ = w.WriteString("false")
}

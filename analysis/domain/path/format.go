package path

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DisplayRoot returns the display name for the path root.
// For symbol-rooted paths, uses the provided nameResolver to get the name.
// For placeholder paths (Symbol=0), returns the Root field directly.
func (p Path) DisplayRoot(nameResolver func(symbol.ID) string) string {
	if p.Symbol != 0 && nameResolver != nil {
		if name := nameResolver(p.Symbol); name != "" {
			return name
		}
	}
	return p.Root
}

// String returns a human-readable representation of the path.
// Format: root.field[index]. Symbol-rooted paths use Root when present and
// fall back to $symN only for symbol-only paths without a display root.
func (p Path) String() string {
	if p.Root == "" && p.Symbol == 0 {
		return ""
	}

	var b strings.Builder

	if p.Root != "" {
		b.WriteString(p.Root)
	} else {
		// Symbol-only path: use symbolic display
		b.WriteString("$sym")
		b.WriteString(strconv.FormatUint(uint64(p.Symbol), 10))
	}

	for _, seg := range p.Segments {
		switch seg.Kind {
		case segment.SegmentField:
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case segment.SegmentIndexString:
			b.WriteByte('[')
			b.WriteString(seg.Name)
			b.WriteByte(']')
		case segment.SegmentIndexInt:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(seg.Index))
			b.WriteByte(']')
		}
	}

	return b.String()
}

// Key returns the structural, version-sensitive representation of the path.
// This is path syntax identity, not stable address identity. Callers that need
// version-insensitive finite-address identity should use domain/path/address;
// callers that need point-visible state keys should use the state-key resolver
// at the engine boundary.
// Format: sym<symbol>@<version><segments> for versioned symbol paths,
// sym<symbol><segments> for unversioned symbol paths, <Root><segments> for placeholders.
func (p Path) Key() PathKey {
	if p.IsEmpty() {
		return ""
	}
	var b strings.Builder
	b.Grow(p.keyLen())
	if p.Symbol != 0 {
		b.WriteString("sym")
		writeUint(&b, uint64(p.Symbol))
		if p.Version != 0 {
			b.WriteByte('@')
			writeInt(&b, p.Version)
		}
	} else {
		b.WriteString(p.Root)
	}
	segment.WriteFormattedSegments(&b, p.Segments)
	return PathKey(b.String())
}

func (p Path) keyLen() int {
	n := segment.FormattedLen(p.Segments)
	if p.Symbol == 0 {
		return n + len(p.Root)
	}
	n += 3 + unsignedDecimalLen(uint64(p.Symbol))
	if p.Version != 0 {
		n += 1 + signedDecimalLen(p.Version)
	}
	return n
}

func writeUint(b *strings.Builder, n uint64) {
	var buf [20]byte
	out := buf[:0]
	out = appendUint(out, n)
	b.Write(out)
}

func writeInt(b *strings.Builder, n int) {
	if n < 0 {
		b.WriteByte('-')
		writeUint(b, uint64(-(n+1))+1)
		return
	}
	writeUint(b, uint64(n))
}

func signedDecimalLen(n int) int {
	if n < 0 {
		return 1 + unsignedDecimalLen(uint64(-(n+1))+1)
	}
	return unsignedDecimalLen(uint64(n))
}

func unsignedDecimalLen(n uint64) int {
	digits := 1
	for n >= 10 {
		n /= 10
		digits++
	}
	return digits
}

func appendUint(out []byte, n uint64) []byte {
	var rev [20]byte
	i := 0
	for {
		rev[i] = byte('0' + n%10)
		i++
		n /= 10
		if n == 0 {
			break
		}
	}
	for i > 0 {
		i--
		out = append(out, rev[i])
	}
	return out
}

// Hash returns a 64-bit hash of the path for use in hash-based collections.
// Symbol-based identity is used when available, otherwise Root is hashed.
func (p Path) Hash() uint64 {
	if p.Root == "" && p.Symbol == 0 {
		return 0
	}

	var h uint64
	if p.Symbol != 0 {
		// Use Symbol as primary identity for hashing
		h = hash.MixHash(0, uint64(p.Symbol))
		h = hash.MixHash(h, uint64(p.Version))
	} else {
		h = hash.FnvString(p.Root)
	}

	for _, seg := range p.Segments {
		h = hash.MixHash(h, uint64(seg.Kind))

		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			h = hash.MixHash(h, hash.FnvString(seg.Name))
		case segment.SegmentIndexInt:
			h = hash.MixHash(h, uint64(seg.Index))
		}
	}

	return h
}

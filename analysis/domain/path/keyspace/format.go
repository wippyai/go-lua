package keyspace

import (
	"strconv"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/internal/keycodec"
)

// Format reproduces the exact old PathKey string for a key. It is the shadow
// oracle: Format(FromPath(p)) == p.Key() for every path, and the analogous
// identities hold for the resolver, stable-symbol, rootless, and canonical
// stable-named spellings.
func (ks *KeySpace) Format(k Key) pathdom.PathKey {
	if ks == nil || k.Kind == KindInvalid || !ks.validKey(k) {
		return ""
	}
	return pathdom.PathKey(ks.formatByKey[k])
}

// FormatReadOnly reads the same owner-sealed spelling as Format. Neither method
// mutates the KeySpace; key minting is the only operation that seals spellings.
func (ks *KeySpace) FormatReadOnly(k Key) pathdom.PathKey {
	if ks == nil || k.Kind == KindInvalid || !ks.validKey(k) {
		return ""
	}
	return pathdom.PathKey(ks.formatByKey[k])
}

// PathKey returns this key's canonical spelling from the KeySpace that minted
// it. It is the read-only bridge used by cross-lane semantic invariants whose
// structural keys already carry their unforgeable owner but not a separate
// KeySpace parameter.
func (k Key) PathKey() (pathdom.PathKey, bool) {
	if k.Kind == KindInvalid || k.owner == nil || k.owner.space == nil || !k.owner.space.Valid() {
		return "", false
	}
	return k.owner.space.FormatReadOnly(k), true
}

// String returns the owner-sealed spelling of k. It lets leaf wire-codec
// packages consume a structural path through fmt.Stringer without importing
// the domain layer. Invalid or detached keys return the empty string.
func (k Key) String() string {
	path, ok := k.PathKey()
	if !ok {
		return ""
	}
	return string(path)
}

func (ks *KeySpace) writeRoot(b *strings.Builder, k Key) {
	switch k.Kind {
	case KindResolverSym:
		b.WriteString("sym")
		b.WriteString(strconv.FormatUint(uint64(k.Sym), 10))
		b.WriteByte('@')
		b.WriteString(strconv.FormatUint(uint64(k.Ver), 10))
	case KindUnversionedSym:
		b.WriteString("sym")
		b.WriteString(strconv.FormatUint(uint64(k.Sym), 10))
	case KindStableSym:
		b.WriteByte('s')
		b.WriteString(strconv.FormatUint(uint64(k.Sym), 10))
	case KindPlaceholder, KindRetSlot, KindNamed:
		root := ks.namedRootString(k)
		if k.Canon && ks.namedRootNeedsEncoding(root, k.Segs) {
			b.WriteString(encodeNamedRoot(root))
		} else {
			b.WriteString(root)
		}
	case kindFormalRoot:
		b.WriteString(encodeFormalRootDescriptor(ks.formalRootEntries[k.Root]))
	case kindBoundaryExistential:
		b.WriteString(encodeBoundaryExistentialDescriptor(ks.existentialEntries[k.Root]))
	case KindRootlessSuffix:
		// no root
	}
}

// namedRootString returns the verbatim root spelling for the stable-named kinds.
func (ks *KeySpace) namedRootString(k Key) string {
	switch k.Kind {
	case KindPlaceholder:
		return "$" + strconv.FormatUint(uint64(k.Root), 10)
	case KindRetSlot:
		return "ret[" + strconv.FormatUint(uint64(k.Root), 10) + "]"
	case KindNamed:
		return ks.rootName(rootID(k.Root))
	case kindBoundaryExistential:
		return encodeBoundaryExistentialDescriptor(ks.existentialEntries[k.Root])
	default:
		return ""
	}
}

func encodeNamedRoot(root string) string {
	return "n" + strconv.Itoa(len(root)) + ":" + root
}

// namedRootNeedsEncoding mirrors address.namedRootNeedsEncoding: it reports
// whether the verbatim spelling root+suffix is ambiguous against the symbol,
// resolver, or encoded-named spelling spaces, or fails to parse back to exactly
// (root, segments) in the plain-named space.
func (ks *KeySpace) namedRootNeedsEncoding(root string, segs SegmentsID) bool {
	if strings.HasPrefix(root, boundaryExistentialPrefix) || strings.HasPrefix(root, formalRootPrefix) {
		return true
	}
	raw := root + ks.suffix(segs)
	if keycodec.LooksEncodedNamedRootKey(raw) || keycodec.LooksStableSymbolRootSuffix(raw) || keycodec.LooksResolverRootSuffix(raw) {
		return true
	}
	if symbolPathKeyParses(raw) {
		return true
	}
	if resolverPathKeyParses(raw) {
		return true
	}
	parsedRoot, parsedSegs, ok := parsePlainNamedRoot(raw)
	if !ok || parsedRoot != root {
		return true
	}
	return !sameSegments(parsedSegs, ks.segments(segs))
}

func sameSegments(a, b []segment.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

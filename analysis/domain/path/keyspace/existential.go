package keyspace

import (
	"encoding/base64"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/symbol"
)

const boundaryExistentialPrefix = "__wippy_boundary_existential_v1_"

// ExistentialNamespace is the finite lexical provenance of one boundary
// application. It deliberately excludes dynamic stack/world/tuple identities:
// recursive applications at the same μ equation reuse the same namespace.
type ExistentialNamespace struct {
	OwnerHi   uint64
	OwnerLo   uint64
	Point     uint32
	Partition uint32
}

func (n ExistentialNamespace) Valid() bool {
	return n.OwnerHi != 0 || n.OwnerLo != 0
}

// ImportExistential creates a structural, activation-independent root for one
// otherwise-unbound source root. The root has its own private KeyKind and
// descriptor interner; it can never collide with a user named root.
func (ks *KeySpace) ImportExistential(from *KeySpace, source Key, namespace ExistentialNamespace) (Key, bool) {
	if ks == nil || from == nil || !ks.Valid() || !from.Valid() || !namespace.Valid() ||
		!from.validKey(source) || source.Segs != 0 || source.Kind == KindRootlessSuffix || source.Kind == KindInvalid {
		return Key{}, false
	}
	if source.Kind == kindBoundaryExistential {
		descriptor := from.existentialEntries[source.Root]
		// A recursive application of the same lexical equation is a fixed
		// point, not a fresh name.
		if descriptor.namespace == namespace {
			return ks.existentialKey(descriptor), true
		}
	}
	descriptor, ok := from.boundaryExistentialDescriptor(source, namespace)
	if !ok {
		return Key{}, false
	}
	return ks.existentialKey(descriptor), true
}

func (ks *KeySpace) boundaryExistentialDescriptor(source Key, namespace ExistentialNamespace) (boundaryExistentialDescriptor, bool) {
	d := boundaryExistentialDescriptor{namespace: namespace, sourceKind: source.Kind, sym: source.Sym, version: source.Ver, canon: source.Canon}
	switch source.Kind {
	case KindResolverSym, KindUnversionedSym, KindStableSym:
	case KindPlaceholder, KindRetSlot:
		d.slot = source.Root
	case KindNamed:
		d.namedRoot = ks.rootName(rootID(source.Root))
		if d.namedRoot == "" {
			return boundaryExistentialDescriptor{}, false
		}
	case kindFormalRoot:
		if source.Root == 0 || int(source.Root) >= len(ks.formalRootEntries) {
			return boundaryExistentialDescriptor{}, false
		}
		d.namedRoot = encodeFormalRootDescriptor(ks.formalRootEntries[source.Root])
	case kindBoundaryExistential:
		d.namedRoot = encodeBoundaryExistentialDescriptor(ks.existentialEntries[source.Root])
	default:
		return boundaryExistentialDescriptor{}, false
	}
	return d, true
}

func (ks *KeySpace) existentialKey(descriptor boundaryExistentialDescriptor) Key {
	if id, ok := ks.existentialByDescriptor[descriptor]; ok {
		return ks.bindKey(Key{Kind: kindBoundaryExistential, Root: id})
	}
	id := uint32(len(ks.existentialEntries))
	ks.existentialEntries = append(ks.existentialEntries, descriptor)
	ks.existentialByDescriptor[descriptor] = id
	return ks.bindKey(Key{Kind: kindBoundaryExistential, Root: id})
}

func encodeBoundaryExistentialDescriptor(d boundaryExistentialDescriptor) string {
	buf := make([]byte, 0, 48+len(d.namedRoot))
	buf = append(buf, 1)
	buf = binary.BigEndian.AppendUint64(buf, d.namespace.OwnerHi)
	buf = binary.BigEndian.AppendUint64(buf, d.namespace.OwnerLo)
	buf = binary.BigEndian.AppendUint32(buf, d.namespace.Point)
	buf = binary.BigEndian.AppendUint32(buf, d.namespace.Partition)
	buf = append(buf, byte(d.sourceKind))
	buf = binary.BigEndian.AppendUint64(buf, uint64(d.sym))
	buf = binary.BigEndian.AppendUint32(buf, d.version)
	buf = binary.BigEndian.AppendUint32(buf, d.slot)
	if d.canon {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(d.namedRoot)))
	buf = append(buf, d.namedRoot...)
	return boundaryExistentialPrefix + base64.RawURLEncoding.EncodeToString(buf)
}

func decodeBoundaryExistentialDescriptor(root string) (boundaryExistentialDescriptor, bool) {
	if len(root) <= len(boundaryExistentialPrefix) || root[:len(boundaryExistentialPrefix)] != boundaryExistentialPrefix {
		return boundaryExistentialDescriptor{}, false
	}
	buf, err := base64.RawURLEncoding.DecodeString(root[len(boundaryExistentialPrefix):])
	if err != nil || len(buf) < 47 || buf[0] != 1 {
		return boundaryExistentialDescriptor{}, false
	}
	offset := 1
	d := boundaryExistentialDescriptor{}
	d.namespace.OwnerHi = binary.BigEndian.Uint64(buf[offset:])
	offset += 8
	d.namespace.OwnerLo = binary.BigEndian.Uint64(buf[offset:])
	offset += 8
	d.namespace.Point = binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	d.namespace.Partition = binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	d.sourceKind = KeyKind(buf[offset])
	offset++
	d.sym = symbol.ID(binary.BigEndian.Uint64(buf[offset:]))
	offset += 8
	d.version = binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	d.slot = binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	if buf[offset] > 1 {
		return boundaryExistentialDescriptor{}, false
	}
	d.canon = buf[offset] == 1
	offset++
	nameLen := int(binary.BigEndian.Uint32(buf[offset:]))
	offset += 4
	if nameLen < 0 || len(buf)-offset != nameLen || !d.namespace.Valid() {
		return boundaryExistentialDescriptor{}, false
	}
	d.namedRoot = string(buf[offset:])
	if !validBoundaryExistentialDescriptor(d) {
		return boundaryExistentialDescriptor{}, false
	}
	return d, true
}

func validBoundaryExistentialDescriptor(d boundaryExistentialDescriptor) bool {
	switch d.sourceKind {
	case KindResolverSym:
		return d.sym != 0 && d.version != 0 && d.slot == 0 && d.namedRoot == "" && !d.canon
	case KindUnversionedSym, KindStableSym:
		return d.sym != 0 && d.version == 0 && d.slot == 0 && d.namedRoot == "" && !d.canon
	case KindPlaceholder, KindRetSlot:
		return d.sym == 0 && d.version == 0 && d.namedRoot == ""
	case KindNamed:
		return d.sym == 0 && d.version == 0 && d.slot == 0 && d.namedRoot != ""
	case kindFormalRoot:
		_, ok := decodeFormalRootDescriptor(d.namedRoot)
		return d.sym == 0 && d.version == 0 && d.slot == 0 && ok
	case kindBoundaryExistential:
		_, ok := decodeBoundaryExistentialDescriptor(d.namedRoot)
		return d.sym == 0 && d.version == 0 && d.slot == 0 && ok
	default:
		return false
	}
}

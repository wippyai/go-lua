package keyspace

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

const formalRootPrefix = "__wippy_formal_root_v1_"

// InternFormalRoot mints the typed root of one frozen relational coordinate. The
// complete 32-byte lexical owner, uint64 ordinal, and Vocabulary are the
// structural identity; Key.Root is only this KeySpace's dense local index.
func (ks *KeySpace) InternFormalRoot(root formal.Root) (Key, bool) {
	if !ks.validSpace() || !root.Valid() {
		return Key{}, false
	}
	return ks.formalRootKey(root)
}

// DescribeFormalRoot returns the exact typed descriptor of a formal-rooted
// key, including keys with a non-empty structural suffix.
func (ks *KeySpace) DescribeFormalRoot(
	key Key,
) (formal.Root, bool) {
	if !ks.validKey(key) || key.Kind != kindFormalRoot {
		return formal.Root{}, false
	}
	return ks.formalRootEntries[key.Root], true
}

func (ks *KeySpace) formalRootKey(root formal.Root) (Key, bool) {
	if id, ok := ks.formalRootByValue[root]; ok {
		return ks.bindKey(Key{Kind: kindFormalRoot, Root: id}), true
	}
	id, ok := nextFormalRootID(uint64(len(ks.formalRootEntries)))
	if !ok {
		return Key{}, false
	}
	ks.formalRootEntries = append(ks.formalRootEntries, root)
	ks.formalRootByValue[root] = id
	return ks.bindKey(Key{Kind: kindFormalRoot, Root: id}), true
}

func nextFormalRootID(tableLength uint64) (uint32, bool) {
	if tableLength == 0 || tableLength > math.MaxUint32 {
		return 0, false
	}
	return uint32(tableLength), true
}

func encodeFormalRootDescriptor(root formal.Root) string {
	ownerValue := root.Owner()
	var owner [64]byte
	hex.Encode(owner[:], ownerValue[:])
	var ordinalRaw [8]byte
	binary.BigEndian.PutUint64(ordinalRaw[:], root.Ordinal())
	var ordinal [16]byte
	hex.Encode(ordinal[:], ordinalRaw[:])

	var out strings.Builder
	out.Grow(len(formalRootPrefix) + len(owner) + 1 + len(root.Vocabulary().String()) + 1 + len(ordinal))
	out.WriteString(formalRootPrefix)
	out.Write(owner[:])
	out.WriteByte(':')
	out.WriteString(root.Vocabulary().String())
	out.WriteByte(':')
	out.Write(ordinal[:])
	return out.String()
}

// decodeFormalRootDescriptor is the inverse of the one reserved formal-root
// spelling.  It accepts no presentation aliases: owner and ordinal widths are
// exact, vocabulary is closed, and zero coordinates remain invalid.  Callers
// must separately test formalRootPrefix so a malformed reserved spelling is
// rejected rather than silently admitted as a user-named root.
func decodeFormalRootDescriptor(encoded string) (formal.Root, bool) {
	if !strings.HasPrefix(encoded, formalRootPrefix) {
		return formal.Root{}, false
	}
	rest := strings.TrimPrefix(encoded, formalRootPrefix)
	parts := strings.Split(rest, ":")
	if len(parts) != 3 || len(parts[0]) != hex.EncodedLen(len(lexicalidentity.StableLexicalBodyID{})) || len(parts[2]) != 16 {
		return formal.Root{}, false
	}
	var owner lexicalidentity.StableLexicalBodyID
	if decoded, err := hex.Decode(owner[:], []byte(parts[0])); err != nil || decoded != len(owner) {
		return formal.Root{}, false
	}
	var vocabulary formal.Vocabulary
	switch parts[1] {
	case "in":
		vocabulary = formal.Input
	case "mid":
		vocabulary = formal.Middle
	case "out":
		vocabulary = formal.Output
	default:
		return formal.Root{}, false
	}
	ordinal, err := strconv.ParseUint(parts[2], 16, 64)
	if err != nil {
		return formal.Root{}, false
	}
	root := formal.NewRoot(owner, ordinal, vocabulary)
	if !root.Valid() || encodeFormalRootDescriptor(root) != encoded {
		return formal.Root{}, false
	}
	return root, true
}

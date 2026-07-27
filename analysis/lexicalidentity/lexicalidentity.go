// Package lexicalidentity defines deterministic, full-width identities for
// lexical bodies. These identities are semantic content, never process-local
// CFG instance handles.
package lexicalidentity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// StableLexicalBodyID is the full-width identity used for compilation
// namespaces and the lexical bodies derived within them.
type StableLexicalBodyID [sha256.Size]byte

func (id StableLexicalBodyID) String() string { return hex.EncodeToString(id[:]) }

func (id StableLexicalBodyID) MarshalText() ([]byte, error) {
	out := make([]byte, hex.EncodedLen(len(id)))
	hex.Encode(out, id[:])
	return out, nil
}

// UnitNamespaceFromDigest imports an already verified full-width unit digest.
func UnitNamespaceFromDigest(digest [sha256.Size]byte) StableLexicalBodyID {
	return StableLexicalBodyID(digest)
}

// UnitNamespaceFromContent derives the deterministic standalone namespace
// from a canonical recursive program encoding.
func UnitNamespaceFromContent(content []byte) StableLexicalBodyID {
	h := sha256.New()
	writeBytes(h, []byte("wippy.lexical-unit.v1"))
	writeBytes(h, content)
	var out StableLexicalBodyID
	copy(out[:], h.Sum(nil))
	return out
}

// UnitNamespaceFromLogicalUnit derives a namespace from stable logical
// ownership only. Revision/configuration inputs belong in artifact cache keys,
// not lexical allocation identity.
func UnitNamespaceFromLogicalUnit(unitID, modulePath, entryDocumentID string) StableLexicalBodyID {
	h := sha256.New()
	writeBytes(h, []byte("wippy.lexical-unit.logical.v1"))
	for _, part := range []string{unitID, modulePath, entryDocumentID} {
		writeBytes(h, []byte(part))
	}
	var out StableLexicalBodyID
	copy(out[:], h.Sum(nil))
	return out
}

// RootBody identifies the chunk body in namespace.
func RootBody(namespace StableLexicalBodyID) StableLexicalBodyID {
	return deriveBody(namespace, 1, 0)
}

// FunctionBody identifies a binder-owned lexical function in namespace.
func FunctionBody(namespace StableLexicalBodyID, function uint64) StableLexicalBodyID {
	if function == 0 {
		return StableLexicalBodyID{}
	}
	return deriveBody(namespace, 2, function)
}

func deriveBody(namespace StableLexicalBodyID, ownerKind byte, owner uint64) StableLexicalBodyID {
	if namespace == (StableLexicalBodyID{}) {
		return StableLexicalBodyID{}
	}
	h := sha256.New()
	writeBytes(h, []byte("wippy.lexical-body.v1"))
	writeBytes(h, namespace[:])
	_, _ = h.Write([]byte{ownerKind})
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], owner)
	_, _ = h.Write(raw[:])
	var out StableLexicalBodyID
	copy(out[:], h.Sum(nil))
	return out
}

type byteWriter interface{ Write([]byte) (int, error) }

func writeBytes(w byteWriter, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = w.Write(size[:])
	_, _ = w.Write(value)
}

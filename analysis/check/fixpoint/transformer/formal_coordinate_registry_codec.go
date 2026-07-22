package transformer

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

const formalCoordinateRegistryCodecVersion = "formal-coordinate-registry/content-v1"

// CanonicalBytes is the complete content spelling of a frozen coordinate
// registry. It is intentionally independent of Go map order, selector layout,
// arena address, and registry construction order.
func (r *formalCoordinateRegistry) CanonicalBytes() []byte {
	if r == nil {
		return nil
	}
	encoded := make([]byte, 0, 256)
	encoded = appendCanonicalText(encoded, formalCoordinateRegistryCodecVersion)
	encoded = appendCanonicalOwner(encoded, r.owner)
	roots := r.sortedRoots()
	encoded = appendCanonicalUint64(encoded, uint64(len(roots)))
	for _, root := range roots {
		encoded = appendCanonicalRoot(encoded, root)
		encoded = appendCanonicalClass(encoded, r.classes[root])
	}
	encoded = appendCanonicalUint64(encoded, uint64(len(r.aliases)))
	for _, alias := range r.aliases {
		encoded = appendCanonicalRoot(encoded, alias.left)
		encoded = appendCanonicalRoot(encoded, alias.right)
		encoded = appendCanonicalOccurrence(encoded, alias.guard.occurrence)
		encoded = appendCanonicalUint32(encoded, alias.guard.branch)
		encoded = appendCanonicalUint64(encoded, uint64(len(alias.support)))
		for _, support := range alias.support {
			encoded = appendCanonicalClass(encoded, support)
		}
	}
	alphabetOccurrences := r.sortedAlphabetOccurrences()
	encoded = appendCanonicalUint64(encoded, uint64(len(alphabetOccurrences)))
	for _, occurrence := range alphabetOccurrences {
		alphabet := r.alphabets[occurrence]
		encoded = appendCanonicalOccurrence(encoded, occurrence)
		encoded = appendCanonicalUint64(encoded, uint64(len(alphabet.roots)))
		for _, root := range alphabet.roots {
			encoded = appendCanonicalRoot(encoded, root)
		}
	}
	advanceOccurrences := r.sortedAdvanceOccurrences()
	encoded = appendCanonicalUint64(encoded, uint64(len(advanceOccurrences)))
	for _, occurrence := range advanceOccurrences {
		advance := r.advances[occurrence]
		encoded = appendCanonicalOccurrence(encoded, occurrence)
		encoded = appendCanonicalUint64(encoded, uint64(len(advance.classes)))
		for _, class := range advance.classes {
			encoded = appendCanonicalClass(encoded, class)
		}
	}
	return encoded
}

func (r *formalCoordinateRegistry) ContentID() ContentID { return contentID(r.CanonicalBytes()) }

func (r *formalCoordinateRegistry) sortedRoots() []formal.Root {
	roots := make([]formal.Root, 0, len(r.classes))
	for root := range r.classes {
		roots = append(roots, root)
	}
	sortFormalRoots(roots)
	return roots
}

func (r *formalCoordinateRegistry) sortedAlphabetOccurrences() []formal.OccurrenceID {
	out := make([]formal.OccurrenceID, 0, len(r.alphabets))
	for occurrence := range r.alphabets {
		out = append(out, occurrence)
	}
	sortOccurrences(out)
	return out
}

func (r *formalCoordinateRegistry) sortedAdvanceOccurrences() []formal.OccurrenceID {
	out := make([]formal.OccurrenceID, 0, len(r.advances))
	for occurrence := range r.advances {
		out = append(out, occurrence)
	}
	sortOccurrences(out)
	return out
}

func appendCanonicalText(out []byte, value string) []byte {
	out = appendCanonicalUint64(out, uint64(len(value)))
	return append(out, value...)
}

func appendCanonicalOwner(out []byte, owner lexicalidentity.StableLexicalBodyID) []byte {
	return append(out, owner[:]...)
}

func appendCanonicalUint64(out []byte, value uint64) []byte {
	var scratch [8]byte
	binary.BigEndian.PutUint64(scratch[:], value)
	return append(out, scratch[:]...)
}

func appendCanonicalUint32(out []byte, value uint32) []byte {
	var scratch [4]byte
	binary.BigEndian.PutUint32(scratch[:], value)
	return append(out, scratch[:]...)
}

func appendCanonicalRoot(out []byte, root formal.Root) []byte {
	out = appendCanonicalOwner(out, root.Owner())
	out = appendCanonicalUint64(out, root.Ordinal())
	return append(out, byte(root.Vocabulary()))
}

func appendCanonicalClass(out []byte, class formal.LexicalClassID) []byte {
	out = appendCanonicalOwner(out, class.Owner())
	return appendCanonicalUint64(out, class.Ordinal())
}

func appendCanonicalOccurrence(out []byte, occurrence formal.OccurrenceID) []byte {
	out = appendCanonicalOwner(out, occurrence.Owner())
	return appendCanonicalUint64(out, occurrence.Ordinal())
}

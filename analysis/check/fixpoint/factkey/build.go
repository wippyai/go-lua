package factkey

import (
	"encoding/base64"
	"strings"
)

// Part is one typed subject or qualifier accepted by BuildKey. Constructors
// below are the only way to populate one, keeping literal, encoded, tagged, and
// path spellings distinct at call sites.
type Part struct {
	kind Kind
	tag  string
	text string
	data []byte
}

func IdentityPart(identity []byte) Part { return Part{kind: Identity, data: identity} }
func TermPart(term string) Part         { return Part{kind: Term, text: term} }
func EncodedTermPart(term []byte) Part  { return Part{kind: EncodedTerm, data: term} }
func OpaquePart(value string) Part      { return Part{kind: Opaque, text: value} }
func EncodedOpaquePart(value string) Part {
	return Part{kind: EncodedOpaque, data: []byte(value)}
}
func TaggedIdentityPart(identity []byte) Part {
	return Part{kind: Tagged, tag: taggedIdentity, data: identity}
}
func TaggedTermPart(term []byte) Part {
	return Part{kind: Tagged, tag: taggedTerm, data: term}
}

// PathKey is the standard-library-only bridge implemented by structural path
// keys. keyspace.Key satisfies it without factkey importing the domain layer.
type PathKey interface {
	String() string
}

// PathPart wraps a structural path/address key as the engine's path-tagged term
// subject. A keyspace.Key represents the path body (for example "sym2"), but
// cannot represent the fixpoint term union: temp/* and scalar/* are not paths,
// and an engine boundary that receives only an already-encoded []byte term has
// no owning KeySpace from which it could legitimately mint a Key. Those
// generic wire terms therefore use TermPart instead.
func PathPart(path PathKey) Part {
	if path == nil {
		return Part{}
	}
	return TermPart("path/" + path.String())
}

// Key is either a complete family key or a typed family prefix. BuildKey is the
// only function that assembles its string representation.
type Key struct {
	family FamilyID
	text   string
}

func (k Key) String() string { return k.text }

// Family returns the declaration that owns this key or prefix.
func (k Key) Family() (Family, bool) {
	family, ok := byID[k.family]
	return family, ok && k.text != ""
}

// BuildKey constructs a complete key when occurrence is non-empty and a typed
// prefix when it is empty. A prefix may stop after any complete subject part;
// this is how a family-wide or subject-wide read is expressed without callers
// concatenating separators themselves. Invalid parts produce the zero Key.
func BuildKey(family Family, parts []Part, occurrence string) Key {
	if family.ID == 0 || family.Prefix == "" || len(parts) > 1+len(family.Qualifiers) {
		return Key{}
	}
	if len(parts) == 0 && occurrence == "" {
		return Key{family: family.ID, text: family.Prefix}
	}
	if occurrence != "" && (len(parts) != 1+len(family.Qualifiers) || strings.Contains(occurrence, "/")) {
		return Key{}
	}
	size := len(family.Prefix)
	for index, part := range parts {
		expected := family.Subject
		if index > 0 {
			expected = family.Qualifiers[index-1]
		}
		if part.kind != expected || !validPart(part) {
			return Key{}
		}
		size += part.encodedLen() + 1
	}
	if occurrence != "" {
		size += len(occurrence)
	}
	var b strings.Builder
	b.Grow(size)
	b.WriteString(family.Prefix)
	for _, part := range parts {
		part.writeTo(&b)
		b.WriteByte('/')
	}
	b.WriteString(occurrence)
	return Key{family: family.ID, text: b.String()}
}

func validPart(part Part) bool {
	switch part.kind {
	case Opaque:
		return part.text != "" && !strings.Contains(part.text, "/")
	case EncodedOpaque, Identity, EncodedTerm:
		return len(part.data) != 0
	case Term:
		first, second, found := strings.Cut(part.text, "/")
		return found && first != "" && second != "" && !strings.Contains(second, "/")
	case Tagged:
		return (part.tag == taggedIdentity || part.tag == taggedTerm) && len(part.data) != 0
	}
	return false
}

func (p Part) encodedLen() int {
	switch p.kind {
	case Opaque, Term:
		return len(p.text)
	case Tagged:
		return len(p.tag) + 1 + base64.RawURLEncoding.EncodedLen(len(p.data))
	default:
		return base64.RawURLEncoding.EncodedLen(len(p.data))
	}
}

func (p Part) writeTo(b *strings.Builder) {
	switch p.kind {
	case Opaque, Term:
		b.WriteString(p.text)
	case Tagged:
		b.WriteString(p.tag)
		b.WriteByte('/')
		writeEncoded(b, p.data)
	default:
		writeEncoded(b, p.data)
	}
}

func writeEncoded(b *strings.Builder, value []byte) {
	const localCapacity = 256
	size := base64.RawURLEncoding.EncodedLen(len(value))
	var local [localCapacity]byte
	var encoded []byte
	if size <= len(local) {
		encoded = local[:size]
	} else {
		encoded = make([]byte, size)
	}
	base64.RawURLEncoding.Encode(encoded, value)
	b.Write(encoded)
}

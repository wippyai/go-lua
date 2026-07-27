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
func CoordinatePart(name string) Part   { return Part{kind: Coordinate, text: name} }
func EncodedOpaquePart(value string) Part {
	return Part{kind: EncodedOpaque, data: []byte(value)}
}
func TaggedIdentityPart(identity []byte) Part {
	return Part{kind: Tagged, tag: taggedIdentity, data: identity}
}
func TaggedTermPart(term []byte) Part {
	return Part{kind: Tagged, tag: taggedTerm, data: term}
}

// RebindSubject projects one semantic subject into another family's declared
// subject kind. Revocation licenses use it when an identity-scoped proof is
// revoked by a tagged identity family (or the inverse). Opaque coordinates are
// intentionally never guessed across kinds.
func RebindSubject(subject Part, family Family) (Part, bool) {
	switch family.Subject {
	case Identity:
		switch {
		case subject.kind == Identity:
			return subject, true
		case subject.kind == Tagged && subject.tag == taggedIdentity:
			return IdentityPart(subject.data), true
		}
	case EncodedTerm:
		switch {
		case subject.kind == EncodedTerm:
			return subject, true
		case subject.kind == Term:
			return EncodedTermPart([]byte(subject.text)), true
		case subject.kind == Tagged && subject.tag == taggedTerm:
			return EncodedTermPart(subject.data), true
		}
	case Term:
		switch {
		case subject.kind == Term:
			return subject, true
		case subject.kind == EncodedTerm:
			return TermPart(string(subject.data)), true
		case subject.kind == Tagged && subject.tag == taggedTerm:
			return TermPart(string(subject.data)), true
		}
	case Tagged:
		switch {
		case subject.kind == Tagged:
			return subject, true
		case subject.kind == Identity:
			return TaggedIdentityPart(subject.data), true
		case subject.kind == EncodedTerm:
			return TaggedTermPart(subject.data), true
		case subject.kind == Term:
			return TaggedTermPart([]byte(subject.text)), true
		}
	default:
		if subject.kind == family.Subject {
			return subject, true
		}
	}
	return Part{}, false
}

// Key is either a complete family key or a typed family prefix. BuildKey is the
// only function that assembles its string representation.
type Key struct {
	family     FamilyID
	text       string
	prefix     string
	subject    string
	occurrence string
}

func (k Key) String() string {
	if k.text != "" {
		return k.text
	}
	if k.prefix == "" {
		return ""
	}
	return k.prefix + k.subject + "/" + k.occurrence
}

// Family returns the declaration that owns this key or prefix.
func (k Key) Family() (Family, bool) {
	family, ok := byID[k.family]
	return family, ok && (k.text != "" || k.prefix != "")
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
		terminalTerm := index == 0 && expected == Term && len(family.Qualifiers) == 0
		if part.kind != expected || !validPart(part, terminalTerm) {
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

// BuildSubjectKey is the fixed-arity form for a family with no qualifiers.
// Keeping the subject out of a temporary slice lets hot value/type publication
// sites retain the single key-string allocation of their displaced spelling.
func BuildSubjectKey(family Family, subject Part, occurrence string) Key {
	if family.ID == 0 || family.Prefix == "" || len(family.Qualifiers) != 0 ||
		subject.kind != family.Subject || !validPart(subject, family.Subject == Term) ||
		(occurrence != "" && strings.Contains(occurrence, "/")) {
		return Key{}
	}
	switch subject.kind {
	case Term, Coordinate, Opaque:
		return Key{
			family: family.ID, prefix: family.Prefix,
			subject: subject.text, occurrence: occurrence,
		}
	}
	size := len(family.Prefix) + subject.encodedLen() + 1 + len(occurrence)
	var b strings.Builder
	b.Grow(size)
	b.WriteString(family.Prefix)
	subject.writeTo(&b)
	b.WriteByte('/')
	b.WriteString(occurrence)
	return Key{family: family.ID, text: b.String()}
}

func validPart(part Part, terminalTerm bool) bool {
	switch part.kind {
	case Opaque, Coordinate:
		return part.text != "" && !strings.Contains(part.text, "/")
	case EncodedOpaque, Identity, EncodedTerm:
		return len(part.data) != 0
	case Term:
		first, second, found := strings.Cut(part.text, "/")
		return found && first != "" && second != "" && (terminalTerm || !strings.Contains(second, "/"))
	case Tagged:
		return (part.tag == taggedIdentity || part.tag == taggedTerm) && len(part.data) != 0
	}
	return false
}

func (p Part) encodedLen() int {
	switch p.kind {
	case Opaque, Term, Coordinate:
		return len(p.text)
	case Tagged:
		return len(p.tag) + 1 + base64.RawURLEncoding.EncodedLen(len(p.data))
	default:
		return base64.RawURLEncoding.EncodedLen(len(p.data))
	}
}

func (p Part) writeTo(b *strings.Builder) {
	switch p.kind {
	case Opaque, Term, Coordinate:
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

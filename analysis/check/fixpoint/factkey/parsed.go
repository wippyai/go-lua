package factkey

import (
	"encoding/base64"
	"strings"
)

// SubjectRef is a zero-copy view of one parsed key position. Spelling aliases
// the fact key and stays valid as long as that key does.
type SubjectRef struct {
	kind     Kind
	spelling string
	tag      string
	encoded  string
}

func (r SubjectRef) Kind() Kind       { return r.kind }
func (r SubjectRef) Spelling() string { return r.spelling }
func (r SubjectRef) Encoded() string  { return r.encoded }
func (r SubjectRef) TaggedIdentity() bool {
	return r.kind == Tagged && r.tag == taggedIdentity
}
func (r SubjectRef) TaggedTerm() bool { return r.kind == Tagged && r.tag == taggedTerm }

// Decode appends the decoded identity, term, or opaque bytes to dst. Literal
// terms and opaque discriminators append their spelling unchanged.
func (r SubjectRef) Decode(dst []byte) ([]byte, bool) {
	switch r.kind {
	case Opaque, Term:
		return append(dst, r.spelling...), r.spelling != ""
	case EncodedOpaque, Identity, EncodedTerm, Tagged:
		if !validRawURL(r.encoded) {
			return dst, false
		}
		start := len(dst)
		decoded, err := base64.RawURLEncoding.AppendDecode(dst, []byte(r.encoded))
		return decoded, err == nil && len(decoded) > start
	default:
		return dst, false
	}
}

// ParsedKey is the allocation-free structural result used by family readers.
// Heap schemas currently have at most one qualifier; the fixed array leaves
// room for future declarations without allocating a qualifier slice.
type ParsedKey struct {
	Subject        SubjectRef
	qualifiers     [2]SubjectRef
	qualifierCount uint8
	Occurrence     string
}

func (p ParsedKey) Qualifier(index int) (SubjectRef, bool) {
	if index < 0 || index >= int(p.qualifierCount) {
		return SubjectRef{}, false
	}
	return p.qualifiers[index], true
}

func (p ParsedKey) QualifierCount() int { return int(p.qualifierCount) }

// ParseKey parses one complete key without splitting it or decoding any
// payload. It is the parser FamilyValues uses for each row selected by the
// partition's binary-searched prefix range.
func (f Family) ParseKey(key string) (ParsedKey, bool) {
	rest, ok := strings.CutPrefix(key, f.Prefix)
	if !ok || rest == "" || len(f.Qualifiers) > 2 {
		return ParsedKey{}, false
	}
	var parsed ParsedKey
	at := 0
	subject, next, ok := parseRef(rest, at, f.Subject)
	if !ok {
		return ParsedKey{}, false
	}
	parsed.Subject, at = subject, next
	for index, kind := range f.Qualifiers {
		ref, next, valid := parseRef(rest, at, kind)
		if !valid {
			return ParsedKey{}, false
		}
		parsed.qualifiers[index], parsed.qualifierCount, at = ref, uint8(index+1), next
	}
	if at >= len(rest) || strings.Contains(rest[at:], "/") {
		return ParsedKey{}, false
	}
	parsed.Occurrence = rest[at:]
	return parsed, parsed.Occurrence != ""
}

func parseRef(rest string, at int, kind Kind) (SubjectRef, int, bool) {
	start := at
	first, next, ok := nextSegment(rest, at)
	if !ok {
		return SubjectRef{}, at, false
	}
	switch kind {
	case Term:
		_, end, valid := nextSegment(rest, next)
		if !valid {
			return SubjectRef{}, at, false
		}
		return SubjectRef{kind: kind, spelling: rest[start : end-1]}, end, true
	case Tagged:
		encoded, end, valid := nextSegment(rest, next)
		if !valid || (first != taggedIdentity && first != taggedTerm) || !validRawURL(encoded) {
			return SubjectRef{}, at, false
		}
		return SubjectRef{kind: kind, spelling: rest[start : end-1], tag: first, encoded: encoded}, end, true
	case Identity, EncodedTerm, EncodedOpaque:
		if !validRawURL(first) {
			return SubjectRef{}, at, false
		}
		return SubjectRef{kind: kind, spelling: first, encoded: first}, next, true
	case Opaque:
		return SubjectRef{kind: kind, spelling: first}, next, true
	default:
		return SubjectRef{}, at, false
	}
}

func nextSegment(text string, at int) (string, int, bool) {
	if at >= len(text) {
		return "", at, false
	}
	end := strings.IndexByte(text[at:], '/')
	if end <= 0 {
		return "", at, false
	}
	end += at
	return text[at:end], end + 1, true
}

func validRawURL(encoded string) bool {
	if encoded == "" || len(encoded)%4 == 1 {
		return false
	}
	for _, char := range []byte(encoded) {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

package static

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programstatic "github.com/wippyai/go-lua/program/static"
)

// TypeValueSeeds is Static's read-only finite observation of executable
// TypeValue occurrences.  It is deliberately not embedded in Authority: the
// occurrence relation has its own owner-fenced handles and cannot be used to
// manufacture Static values or Runtime rows.
type TypeValueSeeds struct{ authority *Authority }

type TypeValueSeed struct {
	owner *Authority
	index uint32
}

type typeValueRow struct {
	value linkboundary.Value
	name  string
	root  keyspace.ContentID
	inner typeauthority.RuntimeInner
	exact bool
	id    keyspace.ContentID
}

func (a *Authority) TypeValueSeeds() TypeValueSeeds { return TypeValueSeeds{authority: a} }

func (s TypeValueSeeds) Count() int {
	if s.authority == nil {
		return 0
	}
	return len(s.authority.typeValues)
}

func (s TypeValueSeeds) At(index int) (TypeValueSeed, bool) {
	if s.authority == nil || index < 0 || index >= len(s.authority.typeValues) {
		return TypeValueSeed{}, false
	}
	return TypeValueSeed{owner: s.authority, index: uint32(index)}, true
}

func (s TypeValueSeeds) Source(seed TypeValueSeed) (linkboundary.Value, bool) {
	row, ok := s.row(seed)
	return row.value, ok
}

func (s TypeValueSeeds) Name(seed TypeValueSeed) (string, bool) {
	row, ok := s.row(seed)
	return row.name, ok && row.name != ""
}

// RootIdentity is the Static-issued semantic representative key: primitives
// share by primitive kind, and named objects only within their exact mounted
// lexical activation.  TypeValue consumes it without rescanning Program.
func (s TypeValueSeeds) RootIdentity(seed TypeValueSeed) (keyspace.ContentID, bool) {
	row, ok := s.row(seed)
	return row.root, ok && row.root.Available()
}

func (s TypeValueSeeds) ExactInner(seed TypeValueSeed) (typeauthority.RuntimeInner, bool) {
	row, ok := s.row(seed)
	return row.inner, ok && row.exact
}

func (s TypeValueSeeds) Identity(seed TypeValueSeed) (keyspace.ContentID, bool) {
	row, ok := s.row(seed)
	return row.id, ok && row.id.Available()
}

func (s TypeValueSeeds) row(seed TypeValueSeed) (typeValueRow, bool) {
	if s.authority == nil || seed.owner != s.authority || uint64(seed.index) >= uint64(len(s.authority.typeValues)) {
		return typeValueRow{}, false
	}
	return s.authority.typeValues[seed.index], true
}

// Runtime is a read-only structural observation owned by Static.  Runtime
// receives no TypeValue occurrence rows and callers cannot supply one.
func (a *Authority) Runtime() (*typeauthority.Runtime, bool) {
	if a == nil || a.runtime == nil || a.runtime.Link() != a.source {
		return nil, false
	}
	return a.runtime, true
}

func staticTypeValueIdentity(source *link.Link, shard linkproject.Shard, p *program.Program, term, target keyspace.Term) (string, keyspace.ContentID, bool) {
	if source == nil || p == nil || term == 0 || target == 0 {
		return "", keyspace.ContentID{}, false
	}
	name, ok := staticTypeValueName(p, target)
	if !ok {
		return "", keyspace.ContentID{}, false
	}
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/typevalue-root\\x00\\x01"))
	if primitive, primitiveOK := p.Static().Types().Primitives().Get(target); primitiveOK {
		_, _ = h.Write([]byte{0, byte(primitive)})
		var id keyspace.ContentID
		copy(id[:], h.Sum(nil))
		return name, id, id.Available()
	}
	module, ok := source.Project().ModuleKey(shard)
	if !ok {
		return "", keyspace.ContentID{}, false
	}
	body, _, _, ok := p.Source().Index().Position(term)
	if !ok {
		return "", keyspace.ContentID{}, false
	}
	activation, ok := p.Flow().Activation().For(body)
	if !ok {
		return "", keyspace.ContentID{}, false
	}
	_, _ = h.Write([]byte{1})
	_, _ = h.Write(module[:])
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(activation))
	_, _ = h.Write(word[:])
	binary.BigEndian.PutUint64(word[:], uint64(len(name)))
	_, _ = h.Write(word[:])
	_, _ = h.Write([]byte(name))
	var id keyspace.ContentID
	copy(id[:], h.Sum(nil))
	return name, id, id.Available()
}

func staticTypeValueName(p *program.Program, target keyspace.Term) (string, bool) {
	if p == nil || target == 0 {
		return "", false
	}
	if primitive, ok := p.Static().Types().Primitives().Get(target); ok {
		switch primitive {
		case programstatic.PrimitiveNil:
			return "nil", true
		case programstatic.PrimitiveBoolean:
			return "boolean", true
		case programstatic.PrimitiveNumber:
			return "number", true
		case programstatic.PrimitiveInteger:
			return "integer", true
		case programstatic.PrimitiveString:
			return "string", true
		case programstatic.PrimitiveAny:
			return "any", true
		case programstatic.PrimitiveUnknown:
			return "unknown", true
		case programstatic.PrimitiveNever:
			return "never", true
		default:
			return "", false
		}
	}
	_, declaration, _, ok := p.Static().References().Get(target)
	if !ok || declaration == 0 {
		return "", false
	}
	if _, _, key, _, alias := p.Static().Declarations().Aliases().Get(declaration); alias {
		return staticExactString(p, key)
	}
	if _, key, _, iface := p.Static().Declarations().Interfaces().Get(declaration); iface {
		return staticExactString(p, key)
	}
	return "", false
}

func staticExactString(p *program.Program, key keyspace.Key) (string, bool) {
	value, ok := p.Source().Keys().Exact(key)
	return value.String, ok && value.Kind == keyspace.LiteralString && value.String != ""
}

func staticTypeValueRowID(source *link.Link, values linkboundary.Values, runtime *typeauthority.Runtime, result Value, row typeValueRow) (keyspace.ContentID, bool) {
	if source == nil || !row.root.Available() {
		return keyspace.ContentID{}, false
	}
	value, ok := values.ID(row.value)
	if !ok {
		return keyspace.ContentID{}, false
	}
	sourceID := source.ContentID()
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/typevalue-row\\x00\\x02"))
	_, _ = h.Write(sourceID[:])
	_, _ = h.Write(value[:])
	_, _ = h.Write(row.root[:])
	resultID, ok := staticResultIdentity(result)
	if !ok {
		return keyspace.ContentID{}, false
	}
	_, _ = h.Write(resultID[:])
	if row.exact {
		if runtime == nil {
			return keyspace.ContentID{}, false
		}
		_, _ = h.Write([]byte{1})
		inner, ok := runtime.Identity(row.inner)
		if !ok {
			return keyspace.ContentID{}, false
		}
		_, _ = h.Write(inner[:])
	} else {
		_, _ = h.Write([]byte{0})
	}
	var id keyspace.ContentID
	copy(id[:], h.Sum(nil))
	return id, id.Available()
}

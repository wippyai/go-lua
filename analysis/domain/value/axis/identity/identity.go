package identity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// TableLiteralSite is the precomputed, full-width lexical scope shared by
// every table literal lowered in one body.
type TableLiteralSite string

// TableLiteralSiteForBody computes the body-scoped Site string once.
func TableLiteralSiteForBody(body lexicalidentity.StableLexicalBodyID) TableLiteralSite {
	if body == (lexicalidentity.StableLexicalBodyID{}) {
		return ""
	}
	const prefix = "lexical-body-expr-v2:"
	var encoded [len(prefix) + sha256.Size*2]byte
	copy(encoded[:], prefix)
	hex.Encode(encoded[len(prefix):], body[:])
	return TableLiteralSite(string(encoded[:]))
}

// LuaTableLiteralAtSite constructs an identity without allocating. site should
// be computed once per prepared body with TableLiteralSiteForBody.
func LuaTableLiteralAtSite(site TableLiteralSite, exprRef uint64) ID {
	if site == "" || exprRef == 0 {
		return ID{}
	}
	return ID{Kind: "lua.table", Site: string(site), Index: exprRef}
}

type state uint8

const (
	bottom state = iota
	singleton
	top
)

// ID is a stable runtime identity token for allocation, object, closure, or
// other value-producing sites. It intentionally carries no type-domain node.
type ID struct {
	Kind  string
	Site  string
	Index uint64
}

// LuaFunction returns the stable identity token for a Lua function expression
// bound by the Lua binder.
func LuaFunction(symbol uint64) ID {
	if symbol == 0 {
		return ID{}
	}
	return ID{Kind: "lua.function", Site: "symbol", Index: symbol}
}

// LuaTableLiteralInBody returns the deterministic identity for a table literal
// in one stable lexical body. It never consumes a process-local CFG instance.
func LuaTableLiteralInBody(body lexicalidentity.StableLexicalBodyID, exprRef uint64) ID {
	return LuaTableLiteralAtSite(TableLiteralSiteForBody(body), exprRef)
}

// ManifestAllocation returns the canonical identity for one operational
// allocation template at a lexical point. Signature lowering and symbolic
// Relation specialization must both use this constructor.
func ManifestAllocation(template string, lexicalPoint uint64) ID {
	if template == "" {
		return ID{}
	}
	return ID{Kind: "manifest.allocation", Site: template, Index: lexicalPoint}
}

// ReturnedAllocationInBody derives a caller-site allocation identity from the
// exact callee template, stable caller body, entry context, and static call
// point. The full SHA-256 scope avoids reducing semantic equality to a
// process-local or 64-bit graph token.
func ReturnedAllocationInBody(template ID, caller lexicalidentity.StableLexicalBodyID, values, facts, references, callPoint uint64) ID {
	if template == (ID{}) || caller == (lexicalidentity.StableLexicalBodyID{}) || callPoint == 0 {
		return ID{}
	}
	var storage [1024]byte
	encoded := storage[:0]
	encoded = appendIdentityHashField(encoded, "wippy.returned-allocation.v2")
	encoded = appendIdentityHashField(encoded, template.Kind)
	encoded = appendIdentityHashField(encoded, template.Site)
	encoded = appendIdentityHashUint(encoded, template.Index)
	encoded = append(encoded, caller[:]...)
	encoded = appendIdentityHashUint(encoded, values)
	encoded = appendIdentityHashUint(encoded, facts)
	encoded = appendIdentityHashUint(encoded, references)
	encoded = appendIdentityHashUint(encoded, callPoint)
	scope := sha256.Sum256(encoded)
	const prefix = "returned-allocation-v2:"
	var site [len(prefix) + sha256.Size*2]byte
	copy(site[:], prefix)
	hex.Encode(site[len(prefix):], scope[:])
	return ID{Kind: template.Kind, Site: string(site[:]), Index: callPoint}
}

// IsReturnedAllocation reports whether id is a caller-site-instantiated
// returned allocation rather than an allocation template.
func IsReturnedAllocation(id ID) bool {
	return id != (ID{}) && strings.HasPrefix(id.Site, "returned-allocation-v2:")
}

func (id ID) String() string {
	return id.Kind + ":" + id.Site + "#" + strconv.FormatUint(id.Index, 10)
}

func (id ID) hash() uint64 {
	h := internal.FnvString("identity.id")
	h = internal.MixHash(h, internal.FnvString(id.Kind))
	h = internal.MixHash(h, internal.FnvString(id.Site))
	return internal.MixHash(h, id.Index)
}

func appendIdentityHashField(out []byte, value string) []byte {
	out = appendIdentityHashUint(out, uint64(len(value)))
	return append(out, value...)
}

func appendIdentityHashUint(out []byte, value uint64) []byte {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	return append(out, raw[:]...)
}

// Value is the runtime identity axis. Its lattice is flat:
//
//	Bottom < Singleton(ID) < Top
//
// Distinct singleton identities are incomparable and join to Top.
type Value struct {
	state state
	id    ID
}

func Bottom() Value {
	return Value{state: bottom}
}

func Top() Value {
	return Value{state: top}
}

func Singleton(id ID) Value {
	return Value{state: singleton, id: id}
}

func (v Value) IsBottom() bool {
	return v.state == bottom
}

func (v Value) IsTop() bool {
	return v.state == top
}

func (v Value) ID() (ID, bool) {
	if v.state != singleton {
		return ID{}, false
	}
	return v.id, true
}

func Equal(a, b Value) bool {
	if a.state != b.state {
		return false
	}
	if a.state != singleton {
		return true
	}
	return a.id == b.id
}

func LessOrEq(a, b Value) bool {
	if Equal(a, b) {
		return true
	}
	return a.state == bottom || b.state == top
}

func (v Value) Covers(other Value) bool {
	return LessOrEq(other, v)
}

func Join(a, b Value) Value {
	if a.state == bottom {
		return b
	}
	if b.state == bottom {
		return a
	}
	if a.state == top || b.state == top {
		return Top()
	}
	if a.id == b.id {
		return a
	}
	return Top()
}

func Meet(a, b Value) Value {
	if a.state == top {
		return b
	}
	if b.state == top {
		return a
	}
	if a.state == bottom || b.state == bottom {
		return Bottom()
	}
	if a.id == b.id {
		return a
	}
	return Bottom()
}

func Widen(prev, next Value) Value {
	return Join(prev, next)
}

func (v Value) Hash() uint64 {
	h := internal.MixHash(internal.FnvString("identity"), uint64(v.state))
	if v.state == singleton {
		h = internal.MixHash(h, v.id.hash())
	}
	return h
}

func (v Value) String() string {
	switch v.state {
	case bottom:
		return "bottom"
	case singleton:
		return "singleton(" + v.id.String() + ")"
	case top:
		return "top"
	default:
		return "identity(invalid)"
	}
}

var Key = axis.NewKey[Value]("identity")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:       Key,
		Bottom:    Bottom,
		Top:       Top,
		Equal:     Equal,
		LessOrEq:  LessOrEq,
		Join:      Join,
		Meet:      Meet,
		Widen:     Widen,
		Hash:      Value.Hash,
		Retention: axis.ImmutableRetention[Value](),
		Boundary:  axis.PortableIdentity,
	}
}

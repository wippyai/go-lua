package identity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"

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

// AllocationTemplate is the finite structural coordinate of one object in a
// sealed lexical allocation operation. It is deliberately distinct from ID:
// an instantiated allocation cannot be converted back into a template and
// therefore cannot grow B(B(...B(T))) identity chains through recursion.
//
// Allocation and object are dense, one-based ordinals in the sealed relation
// arena. They are program structure, not solve depth, entry state, or a digest
// of a previously instantiated identity. The zero value is invalid.
type AllocationTemplate struct {
	owner      lexicalidentity.StableLexicalBodyID
	allocation uint32
	object     uint32
}

const (
	boundaryAllocationSiteKind = "manifest.allocation.site"
	rootAllocationSiteKind     = "manifest.allocation.root"
	returnedAllocationSiteKind = "returned.allocation.site"
)

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

// ManifestAllocationTemplate constructs the typed owner-local coordinate used
// by a sealed relation frame. No string or ID participates in the coordinate,
// so a concrete allocation identity cannot be smuggled back into this phase.
func ManifestAllocationTemplate(owner lexicalidentity.StableLexicalBodyID, allocation, object uint32) AllocationTemplate {
	if owner == (lexicalidentity.StableLexicalBodyID{}) || allocation == 0 || object == 0 {
		return AllocationTemplate{}
	}
	return AllocationTemplate{owner: owner, allocation: allocation, object: object}
}

// Owner returns the stable lexical body that owns this template token.
func (t AllocationTemplate) Owner() lexicalidentity.StableLexicalBodyID { return t.owner }

// AllocationOrdinal and ObjectOrdinal expose the sealed structural coordinate
// without exposing a constructor from concrete identities.
func (t AllocationTemplate) AllocationOrdinal() uint32 { return t.allocation }
func (t AllocationTemplate) ObjectOrdinal() uint32     { return t.object }

// Valid reports whether the token came from ManifestAllocationTemplate.
func (t AllocationTemplate) Valid() bool {
	return t.owner != (lexicalidentity.StableLexicalBodyID{}) && t.allocation != 0 && t.object != 0
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
	return ID{Kind: returnedAllocationSiteKind, Site: string(site[:]), Index: callPoint}
}

// IsReturnedAllocation reports whether id is a caller-site-instantiated
// returned allocation rather than an allocation template.
func IsReturnedAllocation(id ID) bool {
	return id.Kind == returnedAllocationSiteKind && id.Site != "" && id.Index != 0
}

// BoundaryAllocation derives the finite identity of one allocation template
// instantiated at a lexical relation application. The identity depends only on
// stable program structure: the template, caller body, call point, and frozen
// occurrence partition. Re-entering the same call edge through a recursive mu
// equation therefore reuses the same identity instead of growing a
// depth-indexed allocation chain.
//
// Entry-state digests and solve generations are deliberately absent. They are
// orchestration artifacts, not allocation-site semantics.
func BoundaryAllocation(template AllocationTemplate, caller lexicalidentity.StableLexicalBodyID, callPoint, occurrence uint32) ID {
	if !template.Valid() || caller == (lexicalidentity.StableLexicalBodyID{}) || callPoint == 0 {
		return ID{}
	}
	var storage [1024]byte
	encoded := storage[:0]
	encoded = appendIdentityHashField(encoded, "wippy.boundary-allocation.v1")
	encoded = append(encoded, template.owner[:]...)
	encoded = appendIdentityHashUint(encoded, uint64(template.allocation))
	encoded = appendIdentityHashUint(encoded, uint64(template.object))
	encoded = append(encoded, caller[:]...)
	encoded = appendIdentityHashUint(encoded, uint64(callPoint))
	encoded = appendIdentityHashUint(encoded, uint64(occurrence))
	scope := sha256.Sum256(encoded)
	const prefix = "boundary-allocation-v1:"
	var site [len(prefix) + sha256.Size*2]byte
	copy(site[:], prefix)
	hex.Encode(site[len(prefix):], scope[:])
	return ID{Kind: boundaryAllocationSiteKind, Site: string(site[:]), Index: uint64(callPoint)}
}

// RootBoundaryAllocation derives the finite concrete identity used when a
// lexical body is itself an application root. The template's owner and dense
// allocation/object coordinates are already injective program structure; no
// synthetic caller, point, solve generation, or entry digest is introduced.
func RootBoundaryAllocation(template AllocationTemplate) ID {
	if !template.Valid() {
		return ID{}
	}
	var storage [sha256.Size + 8]byte
	copy(storage[:sha256.Size], template.owner[:])
	binary.BigEndian.PutUint32(storage[sha256.Size:], template.allocation)
	binary.BigEndian.PutUint32(storage[sha256.Size+4:], template.object)
	scope := sha256.Sum256(storage[:])
	const prefix = "root-allocation-v1:"
	var site [len(prefix) + sha256.Size*2]byte
	copy(site[:], prefix)
	hex.Encode(site[len(prefix):], scope[:])
	return ID{Kind: rootAllocationSiteKind, Site: string(site[:]), Index: uint64(template.allocation)<<32 | uint64(template.object)}
}

// IsRootBoundaryAllocation reports whether id is a concrete root-route site.
func IsRootBoundaryAllocation(id ID) bool {
	return id.Kind == rootAllocationSiteKind && id.Site != "" && id.Index != 0
}

// IsBoundaryAllocation reports whether id is a lexical relation-application
// allocation rather than an owner-local template.
func IsBoundaryAllocation(id ID) bool {
	return id.Kind == boundaryAllocationSiteKind && id.Site != ""
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
	term  Term
}

func Bottom() Value {
	return Value{state: bottom}
}

func Top() Value {
	return Value{state: top}
}

func Singleton(id ID) Value {
	return SingletonTerm(ConcreteTerm(id))
}

// SingletonTerm constructs an identity-axis singleton from the canonical
// relational atom. Concrete execution normally calls Singleton; formal
// relation construction and allocation freezing use this typed entry point.
func SingletonTerm(term Term) Value {
	if !term.Valid() {
		return Bottom()
	}
	return Value{state: singleton, term: term}
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
	return v.term.Concrete()
}

// Term returns the exact singleton atom, including formal variables and
// allocation templates. Top and Bottom have no singleton term.
func (v Value) Term() (Term, bool) {
	if v.state != singleton || !v.term.Valid() {
		return Term{}, false
	}
	return v.term, true
}

func Equal(a, b Value) bool {
	if a.state != b.state {
		return false
	}
	if a.state != singleton {
		return true
	}
	return a.term == b.term
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
	if a.term == b.term {
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
	if a.term == b.term {
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
		h = internal.MixHash(h, v.term.hash())
	}
	return h
}

func (v Value) String() string {
	switch v.state {
	case bottom:
		return "bottom"
	case singleton:
		return "singleton(" + v.term.String() + ")"
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
		Canonical: canonicalDescriptor(),
		Boundary:  axis.PortableIdentity,
	}
}

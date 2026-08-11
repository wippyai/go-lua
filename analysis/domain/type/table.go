// Package typedomain owns the sealed, authority-local type labels used by the
// analyzer's Type Factor. A recurrent fact carries Handle, never a typ.Type,
// a type string, a selector, or portable bytes.
package typedomain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/domain/type/internal/sequence"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Handle is a one-word Table-local coordinate. It is operational only: it is
// never written to an artifact and cannot be used with another Table.
type Handle = sequence.Handle

var (
	// ErrSealed reports an attempt to extend a published Table.
	ErrSealed = errors.New("typedomain: table is sealed")
	// ErrInvalidHandle reports a forged or foreign local coordinate.
	ErrInvalidHandle = errors.New("typedomain: invalid table handle")
	// ErrOpenType reports a derived graph whose free formal scope is not owned
	// by the Table. It must stay symbolic at its Rule boundary; approximating it
	// as a closed graph would manufacture a false artifact identity.
	ErrOpenType = errors.New("typedomain: open or unsupported derived type")
	// ErrNoTypeProjection reports an attempt to turn the Type Factor's domain
	// top label back into a concrete typ.Type. TypeTop deliberately represents
	// the factor's collapsed order, not either authored `any` or `unknown`.
	ErrNoTypeProjection = errors.New("typedomain: domain top has no concrete type projection")
	// ErrInvalidOrigin reports malformed, foreign, or noncanonical portable
	// origin data. Origins are cold artifact records, never hot handles.
	ErrInvalidOrigin = errors.New("typedomain: invalid type origin")
)

const (
	originDomain  = "wippy.analysis.domain.type.origin"
	originVersion = uint64(2)

	originAuthored = uint64(1)
	originDerived  = uint64(2)
	originTop      = uint64(3)

	// typeTopHash is an operational label hash only. Domain top has no typ.Type
	// graph and therefore must never borrow Unknown's or Any's equality hash.
	typeTopHash = uint64(0x9e3779b97f4a7c15)
)

type coldOriginKind uint8

const (
	coldOriginInvalid coldOriginKind = iota
	coldOriginAuthored
	coldOriginDerived
	coldOriginDomainTop
)

// coldOrigin is exactly one authored Program reference, derived canonical type
// bytes, or the distinct non-typ domain-top record. The latter prevents the
// Type Factor's deliberate Any/Unknown collapse from fabricating a source type
// identity. A zero value is never stored in an entry.
type coldOrigin struct {
	kind      coldOriginKind
	ref       typeauthority.StaticTypeRef
	canonical []byte
}

func authoredOrigin(ref typeauthority.StaticTypeRef) coldOrigin {
	return coldOrigin{kind: coldOriginAuthored, ref: ref}
}
func derivedOrigin(encoded []byte) coldOrigin {
	return coldOrigin{kind: coldOriginDerived, canonical: append([]byte(nil), encoded...)}
}
func domainTopOrigin() coldOrigin { return coldOrigin{kind: coldOriginDomainTop} }

func (o coldOrigin) authored() bool {
	return o.kind == coldOriginAuthored && o.ref.Valid() && len(o.canonical) == 0
}

func (o coldOrigin) derived() bool {
	return o.kind == coldOriginDerived && !o.ref.Valid() && len(o.canonical) != 0
}

func (o coldOrigin) domainTop() bool {
	return o.kind == coldOriginDomainTop && !o.ref.Valid() && len(o.canonical) == 0
}

// entry owns one canonical private graph and exactly one portable cold origin.
// TypeTop is the sole exception: it owns no typ graph because it is a domain
// order label rather than a concrete source type. Other graphs are never
// returned; Project reconstructs an isolated cold copy.
type entry struct {
	value  typ.Type
	hash   uint64
	origin coldOrigin
}

// Table is constructed against one sealed static-type authority, preloads its
// materializable authored roots deterministically, then becomes immutable at
// Seal. It is not a subtype lattice or a process-global interner.
type Table struct {
	mu      sync.RWMutex
	owner   uint32
	static  *typeauthority.Authority
	entries []entry // one-based Handle ordinal, mutable only before Seal
	buckets map[uint64][]Handle

	// published is the one release/acquire boundary from construction to
	// recurrent use. Its entries are immutable forever, so every sealed hot
	// handle operation performs one pointer load and no lock or allocation.
	published atomic.Pointer[sealedTable]

	nil     Handle
	never   Handle
	typeTop Handle
}

type sealedTable struct {
	owner   uint32
	entries []entry
}

func (s *sealedTable) entry(handle Handle) (entry, bool) {
	if s == nil {
		return entry{}, false
	}
	return entryFor(s.owner, s.entries, handle)
}

func entryFor(owner uint32, entries []entry, handle Handle) (entry, bool) {
	if !handle.ValidFor(owner) || uint64(handle.Ordinal()) > uint64(len(entries)) {
		return entry{}, false
	}
	return entries[handle.Ordinal()-1], true
}

var tableSerial atomic.Uint32

// NewTable creates the one local label table for a sealed static authority.
// It preloads every materializable static Ref in that authority's selector
// order. Passing nil is useful only for isolated table laws; production
// analysis passes its sealed typeauthority.Authority.
func NewTable(static *typeauthority.Authority) (*Table, error) {
	owner := allocateOwner(&tableSerial)
	if owner == 0 {
		return nil, errors.New("typedomain: table brand space exhausted")
	}
	table := &Table{
		owner: owner, static: static, buckets: make(map[uint64][]Handle, 16),
	}
	if err := table.preloadStatic(); err != nil {
		return nil, err
	}
	if err := table.installConstants(); err != nil {
		return nil, err
	}
	return table, nil
}

// allocateOwner never reuses a brand. Exhaustion remains exhausted rather
// than wrapping through zero into an old Table's live handle space.
func allocateOwner(serial *atomic.Uint32) uint32 {
	if serial == nil {
		return 0
	}
	for {
		current := serial.Load()
		if current == ^uint32(0) {
			return 0
		}
		next := current + 1
		if serial.CompareAndSwap(current, next) {
			return next
		}
	}
}

func (t *Table) preloadStatic() error {
	if t == nil || t.static == nil {
		return nil
	}
	for index := 0; index < t.static.Count(); index++ {
		selector, ok := t.static.At(index)
		if !ok {
			return fmt.Errorf("%w: static selector %d", ErrInvalidOrigin, index)
		}
		ref, ok := t.static.Ref(selector)
		if !ok || !ref.Valid() {
			return fmt.Errorf("%w: static ref %d", ErrInvalidOrigin, index)
		}
		value, materialized := t.static.Resolve(ref)
		if !materialized {
			// Rule-owned symbolic source forms deliberately remain out of the
			// finite type label universe. They are not silently widened here.
			continue
		}
		if isTypeTop(value) {
			// Top-level gradual types are one domain label. Their authored
			// declaration remains available through the static authority for
			// diagnostics; it does not enter a recurrent type fact.
			continue
		}
		if hasFreeFormal(value) {
			// A naked Program formal is an authored syntax node, not a closed
			// Type Factor label. Its lexical owner/substitution remains in the
			// Rule coordinate. Do not turn its presentation spelling into a
			// detached local graph merely because it has a static Ref.
			continue
		}
		if _, err := t.internAuthored(value, ref); err != nil {
			return fmt.Errorf("typedomain: preload static %d: %w", index, err)
		}
	}
	return nil
}

func (t *Table) installConstants() error {
	if t == nil {
		return ErrInvalidHandle
	}
	// TypeTop intentionally has no typ.Type graph or Any/Unknown bytes. It is a
	// factor-order label; the declaration's exact gradual policy remains in the
	// static type authority and nested gradual leaves remain exact graphs.
	t.typeTop = t.appendDomainTop()
	if !t.typeTop.ValidFor(t.owner) {
		return errors.New("typedomain: handle universe exhausted")
	}
	var err error
	if t.nil, err = t.internDerivedValue(typ.Nil, false); err != nil {
		return err
	}
	if t.never, err = t.internDerivedValue(typ.Never, false); err != nil {
		return err
	}
	return nil
}

func (t *Table) appendDomainTop() Handle {
	if t == nil || uint64(len(t.entries)) >= uint64(^uint32(0))-1 {
		return Handle{}
	}
	handle := sequence.NewHandle(t.owner, uint32(len(t.entries)+1))
	t.entries = append(t.entries, entry{hash: typeTopHash, origin: domainTopOrigin()})
	return handle
}

// Seal publishes the finite local label universe. Subsequent derived/origin
// admissions fail rather than letting fixed-point execution grow a second,
// implicit type graph universe.
func (t *Table) Seal() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.published.Load() == nil {
		t.published.Store(&sealedTable{owner: t.owner, entries: t.entries})
	}
	t.mu.Unlock()
}

func (t *Table) Sealed() bool {
	return t != nil && t.published.Load() != nil
}

func (t *Table) Count() int {
	if t == nil {
		return 0
	}
	if sealed := t.published.Load(); sealed != nil {
		return len(sealed.entries)
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.entries)
}

// DeriveClosed admits a cold, closed Rule result before Seal. It canonicalizes
// and owns the graph once; callers must never invoke it from hot transfer or
// fixed-point code. Open scoped state is explicitly rejected.
func (t *Table) DeriveClosed(value typ.Type) (Handle, error) {
	if t == nil {
		return Handle{}, ErrInvalidHandle
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.published.Load() != nil {
		return Handle{}, ErrSealed
	}
	if isTypeTop(value) {
		return t.typeTop, nil
	}
	if err := typ.ValidateStaticGenericRecurrence(value); err != nil {
		return Handle{}, fmt.Errorf("%w: %v", ErrOpenType, err)
	}
	return t.internDerivedValue(value, true)
}

func (t *Table) internAuthored(value typ.Type, ref typeauthority.StaticTypeRef) (Handle, error) {
	if !ref.Valid() {
		return Handle{}, fmt.Errorf("%w: missing authored ref", ErrInvalidOrigin)
	}
	if hasFreeFormal(value) {
		return Handle{}, fmt.Errorf("%w: free type parameter", ErrOpenType)
	}
	if err := typ.ValidateStaticGenericRecurrence(value); err != nil {
		return Handle{}, fmt.Errorf("%w: %v", ErrOpenType, err)
	}
	owned, err := ownCanonical(value)
	if err != nil {
		return Handle{}, err
	}
	return t.internOwned(owned, authoredOrigin(ref))
}

func (t *Table) internDerivedValue(value typ.Type, rejectOpen bool) (Handle, error) {
	if nilType(value) {
		return Handle{}, fmt.Errorf("%w: nil", ErrOpenType)
	}
	if rejectOpen && hasFreeFormal(value) {
		return Handle{}, fmt.Errorf("%w: free type parameter", ErrOpenType)
	}
	if err := typ.ValidateStaticGenericRecurrence(value); err != nil {
		return Handle{}, fmt.Errorf("%w: %v", ErrOpenType, err)
	}
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil {
		return Handle{}, fmt.Errorf("%w: %v", ErrOpenType, err)
	}
	return t.internDerivedCanonical(encoded, rejectOpen)
}

func (t *Table) internDerivedCanonical(encoded []byte, rejectOpen bool) (Handle, error) {
	if len(encoded) == 0 {
		return Handle{}, fmt.Errorf("%w: empty derived bytes", ErrInvalidOrigin)
	}
	owned, err := typ.DecodeCanonicalStructural(context.Background(), encoded)
	if err != nil || owned == nil {
		if err == nil {
			err = errors.New("nil decoded graph")
		}
		return Handle{}, fmt.Errorf("%w: %v", ErrInvalidOrigin, err)
	}
	if rejectOpen && hasFreeFormal(owned) {
		return Handle{}, fmt.Errorf("%w: free type parameter", ErrOpenType)
	}
	if err := typ.ValidateStaticGenericRecurrence(owned); err != nil {
		return Handle{}, fmt.Errorf("%w: %v", ErrOpenType, err)
	}
	canonical, err := typ.EncodeCanonical(context.Background(), owned)
	if err != nil || !bytes.Equal(encoded, canonical) {
		if err != nil {
			return Handle{}, fmt.Errorf("%w: %v", ErrInvalidOrigin, err)
		}
		return Handle{}, fmt.Errorf("%w: alternate canonical bytes", ErrInvalidOrigin)
	}
	if isTypeTop(owned) {
		return Handle{}, fmt.Errorf("%w: domain top must use its dedicated origin", ErrInvalidOrigin)
	}
	return t.internOwned(owned, derivedOrigin(canonical))
}

// ownCanonical rejects custom implementations and returns an isolated graph.
// Every admitted public graph therefore loses the caller's mutable pointers
// before it can be stored in a Table entry.
func ownCanonical(value typ.Type) (typ.Type, error) {
	if nilType(value) {
		return nil, fmt.Errorf("%w: nil", ErrOpenType)
	}
	if err := typ.ValidateStaticGenericRecurrence(value); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOpenType, err)
	}
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOpenType, err)
	}
	owned, err := typ.DecodeCanonicalStructural(context.Background(), encoded)
	if err != nil || owned == nil {
		if err == nil {
			err = errors.New("nil decoded graph")
		}
		return nil, fmt.Errorf("%w: %v", ErrOpenType, err)
	}
	check, err := typ.EncodeCanonical(context.Background(), owned)
	if err != nil || !bytes.Equal(encoded, check) {
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrOpenType, err)
		}
		return nil, fmt.Errorf("%w: canonical round trip changed bytes", ErrOpenType)
	}
	return owned, nil
}

func (t *Table) internOwned(value typ.Type, origin coldOrigin) (Handle, error) {
	if t == nil || value == nil || (!origin.authored() && !origin.derived()) {
		return Handle{}, ErrInvalidOrigin
	}
	if uint64(len(t.entries)) >= uint64(^uint32(0))-1 {
		return Handle{}, errors.New("typedomain: handle universe exhausted")
	}
	hash := typ.EqualityHash(value)
	for _, candidate := range t.buckets[hash] {
		entry, ok := t.entryLocked(candidate)
		if ok && typ.TypeEquals(entry.value, value) {
			// Static preload is in selector order, before any derived values;
			// hence an equal derived graph reuses the first stable authored Ref.
			return candidate, nil
		}
	}
	handle := sequence.NewHandle(t.owner, uint32(len(t.entries)+1))
	t.entries = append(t.entries, entry{value: value, hash: hash, origin: origin})
	t.buckets[hash] = append(t.buckets[hash], handle)
	return handle, nil
}

// Valid is the hot fence against foreign or forged handles. It performs no
// graph traversal or codec operation.
func (t *Table) Valid(handle Handle) bool {
	if t == nil {
		return false
	}
	_, ok := t.entryForRead(handle)
	return ok
}

// Equal is exact local-label equality. Equal semantic types have already
// canonicalized to one handle at cold admission, so a hot comparison never
// walks a type graph.
func (t *Table) Equal(left, right Handle) bool {
	if t == nil || left != right {
		return false
	}
	_, ok := t.entryForRead(left)
	return ok
}

// Hash returns a collision-tolerant precomputed equality hash. It is only a
// hash; Equal remains the authority for label equality.
func (t *Table) Hash(handle Handle) uint64 {
	if t == nil {
		return 0
	}
	entry, ok := t.entryForRead(handle)
	if !ok {
		return 0
	}
	return entry.hash
}

func (t *Table) Nil() Handle {
	if t == nil {
		return Handle{}
	}
	return t.nil
}

func (t *Table) Never() Handle {
	if t == nil {
		return Handle{}
	}
	return t.never
}

// TypeTop is the sole factor label for top-level static or derived Any and
// Unknown. It does not erase those types when nested inside another graph.
func (t *Table) TypeTop() Handle {
	if t == nil {
		return Handle{}
	}
	return t.typeTop
}

func (t *Table) entryLocked(handle Handle) (entry, bool) {
	return entryFor(t.owner, t.entries, handle)
}

func (t *Table) entryForRead(handle Handle) (entry, bool) {
	if sealed := t.published.Load(); sealed != nil {
		return sealed.entry(handle)
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.entryLocked(handle)
}

// Project is a cold ownership boundary. It never returns a Table entry graph:
// authored values resolve through the sealed Program authority; derived values
// decode their stored canonical bytes into a fresh structural graph. TypeTop is
// intentionally not projectable: it represents a factor-order collapse, not a
// source type spelling.
func (t *Table) Project(handle Handle) (typ.Type, error) {
	if t == nil {
		return nil, ErrInvalidHandle
	}
	t.mu.RLock()
	entry, ok := t.entryLocked(handle)
	static := t.static
	t.mu.RUnlock()
	if !ok {
		return nil, ErrInvalidHandle
	}
	if entry.origin.domainTop() {
		return nil, ErrNoTypeProjection
	}
	if entry.origin.authored() {
		if static == nil {
			return nil, fmt.Errorf("%w: no static authority", ErrInvalidOrigin)
		}
		value, resolved := static.Resolve(entry.origin.ref)
		if !resolved || value == nil {
			return nil, fmt.Errorf("%w: missing authored ref", ErrInvalidOrigin)
		}
		return value, nil
	}
	if !entry.origin.derived() {
		return nil, ErrInvalidOrigin
	}
	value, err := typ.DecodeCanonicalStructural(context.Background(), entry.origin.canonical)
	if err != nil || value == nil {
		if err == nil {
			err = errors.New("nil decoded graph")
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidOrigin, err)
	}
	return value, nil
}

// EncodeOrigin serializes Handle's portable cold origin. It intentionally
// writes neither a local Handle nor a static Selector/Link identity.
func (t *Table) EncodeOrigin(handle Handle) ([]byte, error) {
	if t == nil {
		return nil, ErrInvalidHandle
	}
	t.mu.RLock()
	entry, ok := t.entryLocked(handle)
	t.mu.RUnlock()
	if !ok {
		return nil, ErrInvalidHandle
	}
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), originDomain, originVersion); err != nil {
		return nil, err
	}
	if entry.origin.domainTop() {
		if err := writer.Record(originTop); err != nil {
			return nil, err
		}
	} else if entry.origin.authored() {
		if err := writer.Record(originAuthored); err != nil {
			return nil, err
		}
		owner := entry.origin.ref.Owner()
		if err := writer.Bytes(owner[:]); err != nil {
			return nil, err
		}
		if err := writer.Uint(uint64(entry.origin.ref.Root())); err != nil {
			return nil, err
		}
	} else if entry.origin.derived() {
		if err := writer.Record(originDerived); err != nil {
			return nil, err
		}
		if err := writer.Bytes(entry.origin.canonical); err != nil {
			return nil, err
		}
	} else {
		return nil, ErrInvalidOrigin
	}
	return writer.FinishBytes()
}

// DecodeOrigin validates and re-interns one cold portable origin. It is a
// construction/artifact operation, never a hot Factor operation. A sealed
// table accepts only an already-preloaded/equal handle; it never grows.
func (t *Table) DecodeOrigin(encoded []byte) (Handle, error) {
	if t == nil {
		return Handle{}, ErrInvalidHandle
	}
	origin, err := decodeOrigin(encoded)
	if err != nil {
		return Handle{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch origin.kind {
	case coldOriginDomainTop:
		return t.typeTop, nil
	case coldOriginAuthored:
		if t.static == nil {
			return Handle{}, fmt.Errorf("%w: no static authority", ErrInvalidOrigin)
		}
		selector, found := t.static.Find(origin.owner, origin.root)
		if !found {
			return Handle{}, fmt.Errorf("%w: foreign or missing authored ref", ErrInvalidOrigin)
		}
		ref, found := t.static.Ref(selector)
		if !found {
			return Handle{}, fmt.Errorf("%w: unresolved authored ref", ErrInvalidOrigin)
		}
		value, materialized := t.static.Resolve(ref)
		if !materialized || value == nil {
			return Handle{}, fmt.Errorf("%w: unmaterializable authored ref", ErrInvalidOrigin)
		}
		if isTypeTop(value) {
			return Handle{}, fmt.Errorf("%w: authored gradual top must use the domain-top origin", ErrInvalidOrigin)
		}
		if hasFreeFormal(value) {
			return Handle{}, fmt.Errorf("%w: free authored type parameter", ErrOpenType)
		}
		owned, err := ownCanonical(value)
		if err != nil {
			return Handle{}, err
		}
		hash := typ.EqualityHash(owned)
		if existing := t.findEqualLocked(hash, owned); existing != (Handle{}) {
			return existing, nil
		}
		if t.published.Load() != nil {
			return Handle{}, ErrSealed
		}
		return t.internOwned(owned, authoredOrigin(ref))
	case coldOriginDerived:
		owned, err := decodeDerivedOrigin(origin.canonical)
		if err != nil {
			return Handle{}, err
		}
		if isTypeTop(owned) {
			return Handle{}, fmt.Errorf("%w: derived gradual top must use the domain-top origin", ErrInvalidOrigin)
		}
		hash := typ.EqualityHash(owned)
		if existing := t.findEqualLocked(hash, owned); existing != (Handle{}) {
			return existing, nil
		}
		if t.published.Load() != nil {
			return Handle{}, ErrSealed
		}
		return t.internOwned(owned, derivedOrigin(origin.canonical))
	default:
		return Handle{}, ErrInvalidOrigin
	}
}

func (t *Table) findEqualLocked(hash uint64, value typ.Type) Handle {
	for _, candidate := range t.buckets[hash] {
		entry, ok := t.entryLocked(candidate)
		if ok && typ.TypeEquals(entry.value, value) {
			return candidate
		}
	}
	return Handle{}
}

type decodedOrigin struct {
	kind      coldOriginKind
	owner     keyspace.ContentID
	root      keyspace.Term
	canonical []byte
}

func decodeOrigin(encoded []byte) (decodedOrigin, error) {
	var reader canonical.Reader
	if err := reader.Reset(context.Background(), encoded, originDomain, originVersion); err != nil {
		return decodedOrigin{}, fmt.Errorf("%w: %v", ErrInvalidOrigin, err)
	}
	kind, err := reader.Record()
	if err != nil {
		return decodedOrigin{}, fmt.Errorf("%w: %v", ErrInvalidOrigin, err)
	}
	switch kind {
	case originAuthored:
		rawOwner, err := reader.Bytes()
		if err != nil {
			return decodedOrigin{}, fmt.Errorf("%w: %v", ErrInvalidOrigin, err)
		}
		var owner keyspace.ContentID
		if len(rawOwner) != len(owner) {
			return decodedOrigin{}, fmt.Errorf("%w: authored owner width", ErrInvalidOrigin)
		}
		copy(owner[:], rawOwner)
		root, err := reader.Uint()
		if err != nil || root == 0 || root > uint64(^uint32(0)) {
			return decodedOrigin{}, fmt.Errorf("%w: authored root", ErrInvalidOrigin)
		}
		if err := reader.Finish(); err != nil || !owner.Available() {
			return decodedOrigin{}, fmt.Errorf("%w: authored framing", ErrInvalidOrigin)
		}
		return decodedOrigin{kind: coldOriginAuthored, owner: owner, root: keyspace.Term(root)}, nil
	case originDerived:
		canonicalBytes, err := reader.Bytes()
		if err != nil {
			return decodedOrigin{}, fmt.Errorf("%w: %v", ErrInvalidOrigin, err)
		}
		if err := reader.Finish(); err != nil {
			return decodedOrigin{}, fmt.Errorf("%w: derived framing", ErrInvalidOrigin)
		}
		if len(canonicalBytes) == 0 {
			return decodedOrigin{}, fmt.Errorf("%w: empty derived bytes", ErrInvalidOrigin)
		}
		return decodedOrigin{kind: coldOriginDerived, canonical: canonicalBytes}, nil
	case originTop:
		if err := reader.Finish(); err != nil {
			return decodedOrigin{}, fmt.Errorf("%w: domain-top framing", ErrInvalidOrigin)
		}
		return decodedOrigin{kind: coldOriginDomainTop}, nil
	default:
		return decodedOrigin{}, fmt.Errorf("%w: origin kind", ErrInvalidOrigin)
	}
}

func decodeDerivedOrigin(encoded []byte) (typ.Type, error) {
	value, err := typ.DecodeCanonicalStructural(context.Background(), encoded)
	if err != nil || value == nil {
		if err == nil {
			err = errors.New("nil decoded graph")
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidOrigin, err)
	}
	if hasFreeFormal(value) {
		return nil, fmt.Errorf("%w: free type parameter", ErrOpenType)
	}
	if err := typ.ValidateStaticGenericRecurrence(value); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOpenType, err)
	}
	canonical, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil || !bytes.Equal(canonical, encoded) {
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidOrigin, err)
		}
		return nil, fmt.Errorf("%w: alternate canonical bytes", ErrInvalidOrigin)
	}
	return value, nil
}

func isTypeTop(value typ.Type) bool {
	return !nilType(value) && (typ.TypeEquals(value, typ.Any) || typ.TypeEquals(value, typ.Unknown))
}

func nilType(value typ.Type) bool {
	if value == nil {
		return true
	}
	reflection := reflect.ValueOf(value)
	return reflection.Kind() == reflect.Pointer && reflection.IsNil()
}

// hasFreeFormal checks lexical Generic/Function binders iteratively. It is
// deliberately separate from canonical encoding: derived artifact values have
// no external formal owner, so accepting them merely because a presentation
// name round-trips would be unsound.
func hasFreeFormal(root typ.Type) bool {
	type scope struct {
		parent *scope
		bound  []*typ.TypeParam
		binder uintptr
	}
	type frame struct {
		value typ.Type
		scope *scope
	}
	type visit struct {
		pointer uintptr
		scope   *scope
	}
	type scopeKey struct {
		parent *scope
		binder uintptr
	}
	contains := func(current *scope, target *typ.TypeParam) bool {
		for current != nil {
			for _, candidate := range current.bound {
				if candidate == target {
					return true
				}
			}
			current = current.parent
		}
		return false
	}
	scopes := make(map[scopeKey]*scope)
	scopeFor := func(parent *scope, binder typ.Type, parameters []*typ.TypeParam) *scope {
		pointer := reflect.ValueOf(binder).Pointer()
		for current := parent; current != nil; current = current.parent {
			if current.binder == pointer {
				return current
			}
		}
		key := scopeKey{parent: parent, binder: pointer}
		if existing := scopes[key]; existing != nil {
			return existing
		}
		next := &scope{parent: parent, bound: parameters, binder: pointer}
		scopes[key] = next
		return next
	}
	stack := []frame{{value: root}}
	seen := make(map[visit]struct{})
	for len(stack) != 0 {
		index := len(stack) - 1
		current := stack[index]
		stack = stack[:index]
		if nilType(current.value) {
			return true
		}
		reflection := reflect.ValueOf(current.value)
		if reflection.Kind() == reflect.Pointer {
			key := visit{pointer: reflection.Pointer(), scope: current.scope}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
		}
		switch value := current.value.(type) {
		case *typ.TypeParam:
			if !contains(current.scope, value) {
				return true
			}
			stack = append(stack, frame{value: value.Constraint, scope: current.scope})
		case *typ.Generic:
			next := scopeFor(current.scope, value, value.TypeParams)
			stack = append(stack, frame{value: value.Body, scope: next})
			for _, parameter := range value.TypeParams {
				if parameter != nil {
					stack = append(stack, frame{value: parameter.Constraint, scope: next})
				}
			}
		case *typ.Function:
			next := scopeFor(current.scope, value, value.TypeParams)
			stack = append(stack, frame{value: value.Variadic, scope: next})
			for _, result := range value.Returns {
				stack = append(stack, frame{value: result, scope: next})
			}
			for _, parameter := range value.Params {
				stack = append(stack, frame{value: parameter.Type, scope: next})
			}
			for _, parameter := range value.TypeParams {
				if parameter != nil {
					stack = append(stack, frame{value: parameter.Constraint, scope: next})
				}
			}
		default:
			typ.WalkChildren(current.value, func(child typ.Type) bool {
				stack = append(stack, frame{value: child, scope: current.scope})
				return false
			})
		}
	}
	return false
}

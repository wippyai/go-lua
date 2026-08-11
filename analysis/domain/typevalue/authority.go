package typevalue

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
)

// SchemaID is the cold identity of one immutable TypeValue relation. Runtime
// State values never carry or serialize it.
type SchemaID [sha256.Size]byte

func (id SchemaID) Available() bool { return id != SchemaID{} }

// Authority is one immutable Link-scoped TypeValue relation. Its semantic
// universe is derived in New; callers cannot admit roots, descriptors, atoms,
// names, captures, or summary images.
type Authority struct {
	source  *link.Link
	static  *staticdomain.Authority
	values  linkboundary.Values
	heap    heap.Schema
	runtime *typeauthority.Runtime
	linkID  keyspace.ContentID
	id      SchemaID
	roots   []rootRow
	seeds   []seedRow

	runtimeRoots    map[linkboundary.Value]uint32
	allocationRoots map[heap.Key]uint32

	descriptors     []descriptorRow
	descriptorIndex map[descriptorKey]uint32
	names           []string
	nameIndex       map[string]uint32

	objectEnd uint64
	methodEnd uint64
	atomEnd   uint64
	cursorEnd uint32
}

// New seals the relation from Static's already-grounded occurrence rows and
// Heap's exact allocation schema. Link and Runtime are derived, never passed.
func New(statics *staticdomain.Authority, heaps heap.Schema) (*Authority, bool) {
	if statics == nil || statics.Link() == nil || !statics.LinkID().Available() || !statics.ContentID().Available() {
		return nil, false
	}
	source := statics.Link()
	runtime, runtimeOK := statics.Runtime()
	if source.Boundary() == nil || heaps.LinkContentID() != source.ContentID() || heaps.Link() != source ||
		!runtimeOK || runtime == nil || runtime.Link() != source || runtime.LinkID() != source.ContentID() || !runtime.ContentID().Available() {
		return nil, false
	}
	a := &Authority{source: source, static: statics, values: source.Boundary().Values(), heap: heaps, runtime: runtime, linkID: source.ContentID()}
	if !a.sealRoots() || !a.sealDescriptors() || !a.sealAtomRange() {
		return nil, false
	}
	a.id = a.schemaID()
	if !a.id.Available() {
		return nil, false
	}
	a.nameIndex = nil
	return a, true
}

func (a *Authority) SchemaID() (SchemaID, bool) {
	if a == nil || !a.id.Available() {
		return SchemaID{}, false
	}
	return a.id, true
}

func (a *Authority) LinkID() keyspace.ContentID {
	if a == nil {
		return keyspace.ContentID{}
	}
	return a.linkID
}

func (a *Authority) schemaID() SchemaID {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.typevalue/authority\x00\x04"))
	_, _ = h.Write(a.linkID[:])
	staticID := a.static.ContentID()
	_, _ = h.Write(staticID[:])
	runtimeID := a.runtime.ContentID()
	_, _ = h.Write(runtimeID[:])
	writeUint64(h, uint64(len(a.roots)))
	for _, row := range a.roots {
		_, _ = h.Write([]byte{byte(row.kind)})
		var id keyspace.ContentID
		switch row.kind {
		case rootRuntime:
			valueID, ok := a.values.ID(row.value)
			if !ok {
				return SchemaID{}
			}
			id = valueID
			_, _ = h.Write(id[:])
		case rootFresh:
			id, _ = a.heap.KeyID(row.fresh)
			_, _ = h.Write(id[:])
		}
	}
	writeUint64(h, uint64(len(a.names)))
	for _, name := range a.names {
		writeUint64(h, uint64(len(name)))
		_, _ = h.Write([]byte(name))
	}
	writeUint64(h, uint64(len(a.descriptors)))
	for _, row := range a.descriptors {
		_, _ = h.Write([]byte{byte(row.innerKind), byte(row.nameKind), byte(row.resolverKind)})
		if row.innerKind == innerExact {
			innerID, _ := a.runtime.Identity(row.inner)
			_, _ = h.Write(innerID[:])
		}
		writeUint64(h, uint64(row.name))
		if row.resolverKind == resolverExact {
			resolverID, _ := a.source.Static().Namespaces().ResolverContentID(row.resolver)
			_, _ = h.Write(resolverID[:])
		}
	}
	writeUint64(h, uint64(a.cursorEnd))
	var id SchemaID
	copy(id[:], h.Sum(nil))
	return id
}

func writeUint64(writer interface{ Write([]byte) (int, error) }, value uint64) {
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], value)
	_, _ = writer.Write(word[:])
}

func checkedAdd(left, right uint64) (uint64, bool) {
	if right > math.MaxUint64-left {
		return 0, false
	}
	return left + right, true
}

func checkedMul(left, right uint64) (uint64, bool) {
	if left != 0 && right > math.MaxUint64/left {
		return 0, false
	}
	return left * right, true
}

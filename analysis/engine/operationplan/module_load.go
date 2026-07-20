package operationplan

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

// ModuleLoadResultIndex is the exact call-result slot refined by a successful
// module path lookup.
const ModuleLoadResultIndex = 0

// ModuleLoadContentID is the full-width canonical identity of one immutable
// module-load producer, including its exact argument source and export table.
type ModuleLoadContentID [sha256.Size]byte

func (id ModuleLoadContentID) Available() bool { return id != ModuleLoadContentID{} }

// ModuleLoadExportTableContentID is the full-width canonical identity of one
// body-shared effective module export table.
type ModuleLoadExportTableContentID [sha256.Size]byte

func (id ModuleLoadExportTableContentID) Available() bool {
	return id != ModuleLoadExportTableContentID{}
}

// ModuleLoadExport is one exact-path result in a module-load table. Values are
// detached DTOs; NewModuleLoadExportTable owns and canonically sorts its input.
type ModuleLoadExport struct {
	Path                string
	Value               product.Value
	PostReturnAuthority bool
}

// ModuleLoadExportTable is a small immutable handle to one canonical table.
// Its private authority is safe to share by every module-load site in a Plan;
// detached accessors copy table entries before publishing them.
type ModuleLoadExportTable struct {
	authority *moduleLoadExportTableAuthority
}

type moduleLoadExportTableAuthority struct {
	registry  *axis.Registry
	exports   []ModuleLoadExport
	contentID ModuleLoadExportTableContentID
}

// ModuleLoadOperation is an immutable, state-independent producer descriptor.
// The argument is retained as an exact ValueSource and is resolved by the
// executor against the call's incoming/read states; it is deliberately not
// reduced to a preparation-time literal.
type ModuleLoadOperation struct {
	argument  factflow.ValueSource
	table     ModuleLoadExportTable
	contentID ModuleLoadContentID
}

// ModuleLoadResolution is one exact-path result selected from an immutable
// producer. Private fields prevent a compiler from fabricating authority or
// detaching a result from the operation identity which selected it.
type ModuleLoadResolution struct {
	operationID         ModuleLoadContentID
	resultIndex         int
	value               product.Value
	postReturnAuthority bool
}

// NewModuleLoadExportTable owns, validates, and canonically sorts one
// effective table for sharing across every require site in a prepared body.
func NewModuleLoadExportTable(reg *axis.Registry, exports []ModuleLoadExport) (ModuleLoadExportTable, bool) {
	return NewModuleLoadExportTableContext(context.Background(), reg, exports)
}

func NewModuleLoadExportTableContext(ctx context.Context, reg *axis.Registry, exports []ModuleLoadExport) (ModuleLoadExportTable, bool) {
	if reg == nil || len(exports) == 0 {
		return ModuleLoadExportTable{}, false
	}
	owned := append([]ModuleLoadExport(nil), exports...)
	sort.Slice(owned, func(i, j int) bool { return owned[i].Path < owned[j].Path })
	for index, item := range owned {
		if item.Path == "" || !product.RetentionSafe(reg, item.Value) ||
			(index != 0 && owned[index-1].Path == item.Path) {
			return ModuleLoadExportTable{}, false
		}
	}
	id, ok := deriveModuleLoadExportTableContentID(ctx, reg, owned)
	if !ok {
		return ModuleLoadExportTable{}, false
	}
	return ModuleLoadExportTable{authority: &moduleLoadExportTableAuthority{
		registry: reg, exports: owned, contentID: id,
	}}, true
}

// NewModuleLoadOperation is the standalone convenience constructor. Prepared
// bodies create one NewModuleLoadExportTable and use
// NewModuleLoadOperationWithTable for every site.
func NewModuleLoadOperation(reg *axis.Registry, argument factflow.ValueSource, exports []ModuleLoadExport) (ModuleLoadOperation, bool) {
	return NewModuleLoadOperationContext(context.Background(), reg, argument, exports)
}

func NewModuleLoadOperationContext(ctx context.Context, reg *axis.Registry, argument factflow.ValueSource, exports []ModuleLoadExport) (ModuleLoadOperation, bool) {
	table, ok := NewModuleLoadExportTableContext(ctx, reg, exports)
	if !ok {
		return ModuleLoadOperation{}, false
	}
	return NewModuleLoadOperationWithTableContext(ctx, argument, table)
}

// NewModuleLoadOperationWithTable creates one lightweight per-site producer
// over a body-shared immutable table authority.
func NewModuleLoadOperationWithTable(argument factflow.ValueSource, table ModuleLoadExportTable) (ModuleLoadOperation, bool) {
	return NewModuleLoadOperationWithTableContext(context.Background(), argument, table)
}

func NewModuleLoadOperationWithTableContext(ctx context.Context, argument factflow.ValueSource, table ModuleLoadExportTable) (ModuleLoadOperation, bool) {
	if !argument.Valid() || !table.valid() {
		return ModuleLoadOperation{}, false
	}
	id, ok := deriveModuleLoadContentID(ctx, argument, table.ContentID())
	if !ok {
		return ModuleLoadOperation{}, false
	}
	return ModuleLoadOperation{argument: argument, table: table, contentID: id}, true
}

func (o ModuleLoadOperation) Argument() factflow.ValueSource { return o.argument }
func (o ModuleLoadOperation) ContentID() ModuleLoadContentID { return o.contentID }
func (o ModuleLoadOperation) ResultIndex() int               { return ModuleLoadResultIndex }

// ValidFor reports that this producer is owned by reg and retains a complete
// versioned export-table identity. It is the narrow validation surface used by
// the callback-free factapply transaction; callers cannot inspect or replace
// the private table authority.
func (o ModuleLoadOperation) ValidFor(reg *axis.Registry) bool {
	return o.valid() && reg != nil && o.table.authority.registry == reg
}

// ExportTable returns the immutable shared-table handle. Its public reads are
// detached even though the private authority remains shared.
func (o ModuleLoadOperation) ExportTable() ModuleLoadExportTable { return o.table }

// ResolveArgument is the callback-free execution seam. A compiler evaluates
// Argument() with its owned ValueTerm machinery, passes that product here, and
// receives an exact result/authority tuple only for a single string literal
// present in the canonical table.
func (o ModuleLoadOperation) ResolveArgument(reg *axis.Registry, argument product.Value) (ModuleLoadResolution, bool) {
	if !o.valid() || reg == nil || reg != o.table.authority.registry || !product.BelongsToRegistry(reg, argument) {
		return ModuleLoadResolution{}, false
	}
	path, ok := typevalue.StringLiteralOf(reg, argument)
	if !ok {
		return ModuleLoadResolution{}, false
	}
	export, ok := o.LookupExport(path)
	if !ok {
		return ModuleLoadResolution{}, false
	}
	return ModuleLoadResolution{
		operationID: o.contentID, resultIndex: ModuleLoadResultIndex,
		value: export.Value, postReturnAuthority: export.PostReturnAuthority,
	}, true
}

func (r ModuleLoadResolution) ResultIndex() int          { return r.resultIndex }
func (r ModuleLoadResolution) Value() product.Value      { return r.value }
func (r ModuleLoadResolution) PostReturnAuthority() bool { return r.postReturnAuthority }
func (r ModuleLoadResolution) Matches(operation ModuleLoadOperation) bool {
	return operation.valid() && r.operationID.Available() && r.operationID == operation.ContentID() &&
		r.resultIndex == ModuleLoadResultIndex && product.BelongsToRegistry(operation.table.authority.registry, r.value)
}

// Exports returns a detached canonical path-sorted table.
func (o ModuleLoadOperation) Exports() []ModuleLoadExport {
	return o.table.Exports()
}

// LookupExport performs exact-path lookup over the canonical table.
func (o ModuleLoadOperation) LookupExport(path string) (ModuleLoadExport, bool) {
	return o.table.LookupExport(path)
}

func (t ModuleLoadExportTable) ContentID() ModuleLoadExportTableContentID {
	if t.authority == nil {
		return ModuleLoadExportTableContentID{}
	}
	return t.authority.contentID
}

func (t ModuleLoadExportTable) Exports() []ModuleLoadExport {
	if t.authority == nil {
		return nil
	}
	return append([]ModuleLoadExport(nil), t.authority.exports...)
}

func (t ModuleLoadExportTable) LookupExport(path string) (ModuleLoadExport, bool) {
	if t.authority == nil {
		return ModuleLoadExport{}, false
	}
	index := sort.Search(len(t.authority.exports), func(i int) bool { return t.authority.exports[i].Path >= path })
	if index == len(t.authority.exports) || t.authority.exports[index].Path != path {
		return ModuleLoadExport{}, false
	}
	return t.authority.exports[index], true
}

func (t ModuleLoadExportTable) valid() bool {
	if t.authority == nil || t.authority.registry == nil || !t.authority.contentID.Available() || len(t.authority.exports) == 0 {
		return false
	}
	for index, item := range t.authority.exports {
		if item.Path == "" || !product.RetentionSafe(t.authority.registry, item.Value) ||
			(index != 0 && t.authority.exports[index-1].Path >= item.Path) {
			return false
		}
	}
	return true
}

func (o ModuleLoadOperation) valid() bool {
	return o.argument.Valid() && o.table.valid() && o.contentID.Available()
}

func (o ModuleLoadOperation) clone() ModuleLoadOperation {
	// The table authority is immutable and intentionally shared. All exported
	// reads detach its entry slice.
	return o
}

func (o ModuleLoadOperation) equal(other ModuleLoadOperation) bool {
	if !o.valid() || !other.valid() || o.contentID != other.contentID ||
		!factflow.ValueSourceEqual(o.argument, other.argument) || !o.table.equal(other.table) {
		return false
	}
	return true
}

func (t ModuleLoadExportTable) equal(other ModuleLoadExportTable) bool {
	if !t.valid() || !other.valid() || t.authority.registry != other.authority.registry ||
		t.ContentID() != other.ContentID() || len(t.authority.exports) != len(other.authority.exports) {
		return false
	}
	for index := range t.authority.exports {
		left, right := t.authority.exports[index], other.authority.exports[index]
		if left.Path != right.Path || left.PostReturnAuthority != right.PostReturnAuthority ||
			!product.Equal(t.authority.registry, left.Value, right.Value) {
			return false
		}
	}
	return true
}

func deriveModuleLoadContentID(ctx context.Context, argument factflow.ValueSource, tableID ModuleLoadExportTableContentID) (ModuleLoadContentID, bool) {
	hash := sha256.New()
	writeModuleLoadBytes(hash, []byte("wippy.operationplan.module-load.v2"))
	argumentID, err := factflow.CanonicalValueSourceContentID(ctx, argument)
	if err != nil || !argumentID.Available() {
		return ModuleLoadContentID{}, false
	}
	writeModuleLoadBytes(hash, argumentID[:])
	writeModuleLoadBytes(hash, tableID[:])
	var out ModuleLoadContentID
	copy(out[:], hash.Sum(nil))
	return out, out.Available()
}

func deriveModuleLoadExportTableContentID(ctx context.Context, reg *axis.Registry, exports []ModuleLoadExport) (ModuleLoadExportTableContentID, bool) {
	hash := sha256.New()
	writeModuleLoadBytes(hash, []byte("wippy.operationplan.module-load-export-table.v1"))
	writeModuleLoadUint64(hash, uint64(len(exports)))
	for _, item := range exports {
		encoded, schema, err := product.EncodeCanonical(ctx, reg, item.Value)
		if err != nil {
			return ModuleLoadExportTableContentID{}, false
		}
		writeModuleLoadBytes(hash, []byte(item.Path))
		writeModuleLoadBytes(hash, schema[:])
		writeModuleLoadBytes(hash, encoded)
		writeModuleLoadBool(hash, item.PostReturnAuthority)
	}
	var out ModuleLoadExportTableContentID
	copy(out[:], hash.Sum(nil))
	return out, out.Available()
}

type moduleLoadWriter interface{ Write([]byte) (int, error) }

func writeModuleLoadBytes(writer moduleLoadWriter, value []byte) {
	writeModuleLoadUint64(writer, uint64(len(value)))
	_, _ = writer.Write(value)
}

func writeModuleLoadBool(writer moduleLoadWriter, value bool) {
	if value {
		_, _ = writer.Write([]byte{1})
		return
	}
	_, _ = writer.Write([]byte{0})
}

func writeModuleLoadUint64(writer moduleLoadWriter, value uint64) {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	_, _ = writer.Write(raw[:])
}

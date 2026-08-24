package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

// FormalPort is a Batch-issued, Link-independent boundary Site. Its Site is
// deliberately fixed at the empty decision scope with no initial
// contribution: a later TemplateBinding supplies the exact concrete Site and
// the direction-specific total Reindex proofs. A future non-empty formal
// scope will therefore require explicit maps here instead of an ambient scope
// inference in the eventual materializer.
//
// The private Batch pointer is capability provenance only. It never enters
// the formal Site identity, so an equivalent replay has the same canonical
// key without gaining authority over this handle.
type FormalPort struct {
	batch *Batch
	row   uint32
}

// FormalPortActual is one proposed exact binding. Mode is intentionally
// absent: direction belongs exclusively to the formal port. Reads may only
// replace the dense Local coordinate of an otherwise identical exact-read
// ABI slot.
type FormalPortActual struct {
	Role  FormalPort
	Site  Site
	Reads []PortRead
}

// TemplateBinding is the sole sealed admission authority from every formal
// port in one exact Batch to concrete Sites in one other exact Batch. It does
// not relax Site/Occurrence/Input ownership: ordinary topology continues to
// reject cross-Batch rows until a later template-instantiation transaction
// explicitly consumes this capability.
type TemplateBinding struct{ data *templateBindingData }

type templateBindingAuthority struct{ marker byte }

type templateBindingData struct {
	formals   *Batch
	actuals   *Batch
	key       composition.Key
	authority *templateBindingAuthority
	rows      []templateBindingRow
	bySite    []uint32 // formal Site row -> one-based row in rows
	available bool
}

type templateBindingRow struct {
	formal  FormalPort
	actual  Site
	ingress Reindex // concrete scope -> empty formal scope
	egress  Reindex // empty formal scope -> concrete scope
	reads   []PortRead
}

// AdmitFormalPort adds one exact named boundary to an open Batch. Re-admitting
// the identical role/ABI returns the same capability; changing its direction
// or read slots rejects the entire Batch.
func (batch *Batch) AdmitFormalPort(role composition.Key, mode PortMode, reads []PortRead) (FormalPort, bool) {
	if batch == nil || batch.phase != batchOpen || !role.Available() || !validPortMode(mode) {
		if batch != nil {
			batch.rejectOpen()
		}
		return FormalPort{}, false
	}
	canonicalReads, ok := canonicalFormalPortReads(mode, reads)
	if !ok {
		batch.rejectOpen()
		return FormalPort{}, false
	}
	if existing, found := batch.formalAt[role]; found {
		row, rowOK := batch.openSite(existing)
		if !rowOK || !row.formal || row.formalRole != role || row.formalMode != mode || !samePortReadSlots(row.formalReads, canonicalReads) {
			batch.rejectOpen()
			return FormalPort{}, false
		}
		return FormalPort{batch: batch, row: existing}, true
	}
	source, ok := deriveFormalPortSource(role)
	if !ok {
		batch.rejectOpen()
		return FormalPort{}, false
	}
	if _, collision := batch.siteBySource[source]; collision {
		batch.rejectOpen()
		return FormalPort{}, false
	}
	admitted := siteRow{
		source: source, scope: EmptyScope(), init: FalseExpr(), disposition: InitAbsent,
		formal: true, formalRole: role, formalMode: mode, formalReads: canonicalReads,
	}
	key, ok := deriveSiteKey(admitted)
	if !ok {
		batch.rejectOpen()
		return FormalPort{}, false
	}
	admitted.key = key
	row := uint32(len(batch.sites) + 1)
	batch.sites = append(batch.sites, admitted)
	batch.siteBySource[source] = row
	batch.formalAt[role] = row
	return FormalPort{batch: batch, row: row}, true
}

func validPortMode(mode PortMode) bool {
	return mode == PortImport || mode == PortExport || mode == PortImportExport
}

func canonicalFormalPortReads(mode PortMode, values []PortRead) ([]PortRead, bool) {
	if !mode.imports() && len(values) != 0 {
		return nil, false
	}
	reads, ok := canonicalPortReads(values)
	if !ok {
		return nil, false
	}
	for _, read := range reads {
		if !validFormalPortRead(read) {
			return nil, false
		}
	}
	return reads, true
}

func validFormalPortRead(read PortRead) bool {
	return read.Role.Available() && read.Surface.Available() &&
		read.Surface.Form == SurfaceReadExact && read.Surface.Mode == TargetModeNone &&
		!read.Surface.Semantic.Available() && !read.Surface.Normalizer.Available()
}

func compatibleFormalPortReads(prototype, actual []PortRead) bool {
	if len(prototype) != len(actual) {
		return false
	}
	for index := range prototype {
		left, right := prototype[index], actual[index]
		if left.Role != right.Role || !validFormalPortRead(left) || !validFormalPortRead(right) ||
			left.Surface.Factor != right.Surface.Factor || left.Surface.Form != right.Surface.Form ||
			left.Surface.Mode != right.Surface.Mode || left.Surface.Semantic != right.Surface.Semantic ||
			left.Surface.Normalizer != right.Surface.Normalizer {
			return false
		}
	}
	return true
}

func validFormalPortRow(row siteRow) bool {
	if !row.formal || !row.formalRole.Available() || !validPortMode(row.formalMode) ||
		!sameScope(row.scope, EmptyScope()) || !row.init.IsFalse() || row.disposition != InitAbsent {
		return false
	}
	reads, ok := canonicalFormalPortReads(row.formalMode, row.formalReads)
	if !ok || !samePortReadSlots(reads, row.formalReads) {
		return false
	}
	source, ok := deriveFormalPortSource(row.formalRole)
	return ok && source == row.source
}

func deriveFormalPortSource(role composition.Key) (composition.Key, bool) {
	return identityKey("analysis/engine/equation/formal-port-source", func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, role)
	})
}

func deriveFormalPortSiteKey(row siteRow) (composition.Key, bool) {
	if !validFormalPortRow(row) {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/formal-port-site", func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, row.formalRole) && writer.Uint(uint64(row.formalMode)) == nil &&
			writePortReads(writer, row.formalReads)
	})
}

func (port FormalPort) rowValue() (siteRow, bool) {
	if port.batch == nil || port.row == 0 {
		return siteRow{}, false
	}
	row, ok := port.batch.sealedSite(port.row)
	return row, ok && row.formal && row.formalRole.Available() && validPortMode(row.formalMode)
}

func (port FormalPort) Available() bool {
	_, ok := port.rowValue()
	return ok
}

func (port FormalPort) Site() Site {
	if port.batch == nil || port.row == 0 {
		return Site{}
	}
	if _, ok := port.batch.openSite(port.row); !ok {
		if _, sealed := port.batch.sealedSite(port.row); !sealed {
			return Site{}
		}
	}
	return Site{batch: port.batch, row: port.row}
}

func (port FormalPort) Role() composition.Key {
	row, ok := port.rowValue()
	if !ok {
		return composition.Key{}
	}
	return row.formalRole
}

func (port FormalPort) Mode() PortMode {
	row, ok := port.rowValue()
	if !ok {
		return PortInvalid
	}
	return row.formalMode
}

func (port FormalPort) ReadCount() int {
	row, ok := port.rowValue()
	if !ok {
		return 0
	}
	return len(row.formalReads)
}

func (port FormalPort) ReadAt(index int) (PortRead, bool) {
	row, ok := port.rowValue()
	if !ok || index < 0 || index >= len(row.formalReads) {
		return PortRead{}, false
	}
	return row.formalReads[index], true
}

func (port FormalPort) Same(other FormalPort) bool {
	return port.Available() && other.Available() && port.batch == other.batch && port.row == other.row
}

// FormalPort returns the formal capability behind this exact Site. Dynamic
// Sites and ordinary concrete Sites are rejected.
func (site Site) FormalPort() (FormalPort, bool) {
	if site.dynamic != nil || !site.Available() {
		return FormalPort{}, false
	}
	port := FormalPort{batch: site.batch, row: site.row}
	return port, port.Available()
}

// SealTemplateBinding validates and freezes one total formal-to-concrete
// assignment. The formal and concrete Batches must be distinct sealed
// authorities. Equivalent replay Batches may derive equal keys but cannot
// exchange capabilities.
func SealTemplateBinding(formals, actuals *Batch, values []FormalPortActual) (TemplateBinding, bool) {
	if formals == nil || actuals == nil || formals == actuals || !formals.Sealed() || !actuals.Sealed() {
		return TemplateBinding{}, false
	}
	formalCount := 0
	for _, row := range formals.sites {
		if row.formal {
			formalCount++
		}
	}
	if formalCount == 0 || len(values) != formalCount {
		return TemplateBinding{}, false
	}
	rows := make([]templateBindingRow, len(values))
	seen := make(map[uint32]struct{}, len(values))
	for index, value := range values {
		formalRow, formalOK := value.Role.rowValue()
		actualRow, actualOK := actuals.sealedSite(value.Site.row)
		reads, readsOK := canonicalPortReads(value.Reads)
		if !formalOK || value.Role.batch != formals || !actualOK || value.Site.batch != actuals ||
			actualRow.formal || value.Site.dynamic != nil || !readsOK ||
			!compatibleFormalPortReads(formalRow.formalReads, reads) {
			return TemplateBinding{}, false
		}
		if _, duplicate := seen[value.Role.row]; duplicate {
			return TemplateBinding{}, false
		}
		seen[value.Role.row] = struct{}{}
		formalScope := value.Role.Site().Scope()
		actualScope := value.Site.Scope()
		egress, egressOK := NewReindex(formalScope, actualScope, nil)
		forget := make([]DecisionMap, actualScope.Count())
		for decision := 0; decision < actualScope.Count(); decision++ {
			value, present := actualScope.At(decision)
			if !present {
				return TemplateBinding{}, false
			}
			forget[decision] = Forget(value)
		}
		ingress, ingressOK := NewReindex(actualScope, formalScope, forget)
		if !egressOK || !ingressOK {
			return TemplateBinding{}, false
		}
		rows[index] = templateBindingRow{formal: value.Role, actual: value.Site, ingress: ingress, egress: egress, reads: reads}
	}
	sort.Slice(rows, func(left, right int) bool {
		return lessKey(rows[left].formal.Role(), rows[right].formal.Role())
	})
	for index := 1; index < len(rows); index++ {
		if rows[index-1].formal.Role() == rows[index].formal.Role() {
			return TemplateBinding{}, false
		}
	}
	key, ok := identityKey("analysis/engine/equation/template-binding", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, formals.Key()) || writer.Count(uint64(len(rows))) != nil {
			return false
		}
		for _, row := range rows {
			if !writeSite(writer, row.formal.Site()) || !writeSite(writer, row.actual) ||
				!writeReindex(writer, row.ingress) || !writeReindex(writer, row.egress) || !writePortReads(writer, row.reads) {
				return false
			}
		}
		return true
	})
	if !ok {
		return TemplateBinding{}, false
	}
	bySite := make([]uint32, len(formals.sites))
	for index, row := range rows {
		if row.formal.row == 0 || uint64(row.formal.row) > uint64(len(bySite)) || bySite[row.formal.row-1] != 0 {
			return TemplateBinding{}, false
		}
		bySite[row.formal.row-1] = uint32(index + 1)
	}
	for index, row := range formals.sites {
		if row.formal != (bySite[index] != 0) {
			return TemplateBinding{}, false
		}
	}
	data := &templateBindingData{
		formals: formals, actuals: actuals, key: key,
		authority: &templateBindingAuthority{marker: 1}, rows: rows, bySite: bySite,
	}
	data.available = data.formals != nil && data.actuals != nil && data.formals != data.actuals &&
		data.formals.Sealed() && data.actuals.Sealed() && data.key.Available() && data.authority != nil &&
		data.authority.marker == 1 && len(data.rows) != 0 && len(data.bySite) == len(data.formals.sites)
	return TemplateBinding{data: data}, data.available
}

// Available reports whether binding names a sealed formal-to-concrete
// assignment. SealTemplateBinding is the sole constructor of a non-nil data
// pointer and proves this verdict once there; every wrapper that carries the
// same pointer forward reads that settled bit instead of re-authenticating it.
func (binding TemplateBinding) Available() bool {
	data := binding.data
	return data != nil && data.available
}

func (binding TemplateBinding) Key() composition.Key {
	if !binding.Available() {
		return composition.Key{}
	}
	return binding.data.key
}

func (binding TemplateBinding) Same(other TemplateBinding) bool {
	return binding.Available() && other.Available() && binding.data == other.data
}

func (binding TemplateBinding) resolve(port FormalPort, required PortMode) (templateBindingRow, bool) {
	if !binding.Available() || !port.Available() || port.batch != binding.data.formals ||
		port.row == 0 || uint64(port.row) > uint64(len(binding.data.bySite)) {
		return templateBindingRow{}, false
	}
	mode := port.Mode()
	if required != PortImport && required != PortExport || required == PortImport && !mode.imports() || required == PortExport && !mode.exports() {
		return templateBindingRow{}, false
	}
	rowIndex := binding.data.bySite[port.row-1]
	if rowIndex == 0 || uint64(rowIndex) > uint64(len(binding.data.rows)) {
		return templateBindingRow{}, false
	}
	row := binding.data.rows[rowIndex-1]
	if !row.formal.Same(port) || row.actual.batch != binding.data.actuals || !row.actual.Available() ||
		!row.ingress.Available() || !row.egress.Available() ||
		!sameScope(row.ingress.Source(), row.actual.Scope()) || !sameScope(row.ingress.Target(), port.Site().Scope()) ||
		!sameScope(row.egress.Source(), port.Site().Scope()) || !sameScope(row.egress.Target(), row.actual.Scope()) {
		return templateBindingRow{}, false
	}
	return row, true
}

// ResolveImport returns the exact concrete Site, substituted reads, and the
// issued concrete->formal projection. For the current empty formal scope the
// projection explicitly forgets every concrete decision; callers never infer
// this transport from a scope mismatch.
func (binding TemplateBinding) ResolveImport(port FormalPort) (Site, []PortRead, Reindex, bool) {
	row, ok := binding.resolve(port, PortImport)
	if !ok {
		return Site{}, nil, Reindex{}, false
	}
	return row.actual, append([]PortRead(nil), row.reads...), row.ingress, true
}

// ResolveExport returns the exact concrete Site and the issued
// formal->concrete introduction relation. The relation has no source maps for
// the empty formal scope; target decisions are introduced explicitly by the
// Reindex target rather than by an implicit ambient union.
func (binding TemplateBinding) ResolveExport(port FormalPort) (Site, Reindex, bool) {
	row, ok := binding.resolve(port, PortExport)
	if !ok {
		return Site{}, Reindex{}, false
	}
	return row.actual, row.egress, true
}

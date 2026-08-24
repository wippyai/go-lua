package equation

import (
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

// Batch is the sole admission authority for source topology.  It is a
// single-writer cold builder: source identities may enter only while it is
// open, and every capability becomes readable only after Seal succeeds.
//
// Program/Link validation happens before this boundary.  Consequently this
// package receives only already-canonical entity keys, never a raw AST, edge,
// or application spelling.  Batch deliberately does not retain a generic
// source record; Site, Occurrence, and Operand have independent rows and
// relationships.
type Batch struct {
	phase batchPhase

	sites        []siteRow
	siteBySource map[composition.Key]uint32
	formalAt     map[composition.Key]uint32

	occurrences  []occurrenceRow
	occurrenceAt map[occurrenceIndex]uint32

	operands  []operandRow
	operandAt map[operandIndex]uint32
	targets   targetRows

	key composition.Key
}

type batchPhase uint8

const (
	batchOpen batchPhase = iota + 1
	batchSealed
	batchRejected
)

// InitDisposition says whether a Site has an initial contribution.  The
// formula is retained even for a false initial contribution so source
// topology never recovers initialization from an empty producer range.
type InitDisposition uint8

const (
	InitAbsent InitDisposition = iota + 1
	InitPresent
)

// OccurrenceKind is the closed semantic scheduling kind of one exact source
// occurrence.  It is the closed scheduling classification for source rows.
type OccurrenceKind uint8

const (
	OccurrenceAt OccurrenceKind = iota + 1
	OccurrenceFrom
	OccurrenceRelation
)

// Site, Occurrence, and Operand are opaque capabilities.  Each carries the
// owning Batch plus a private one-based row; a copied key can neither mint a
// capability nor cross a Batch boundary.
type Site struct {
	batch   *Batch
	row     uint32
	dynamic *overlaySite
}

type Occurrence struct {
	batch   *Batch
	row     uint32
	dynamic *overlayOccurrence
}

type Operand struct {
	batch   *Batch
	row     uint32
	dynamic *overlayOperand
}

// Dynamic rows are immutable overlays over a sealed base Batch.  Activation
// never mutates Batch or admits a raw key: it derives a private identity from
// the sealed binding, base row, and selected member.
type overlaySite struct {
	scope       Scope
	init        Expr
	disposition InitDisposition
	key         composition.Key
	binding     composition.Key
	member      composition.Key
	row         composition.Key
}

type overlayOccurrence struct {
	site    Site
	key     composition.Key
	binding composition.Key
	member  composition.Key
	row     composition.Key
}

type overlayOperand struct {
	occurrence Occurrence
	key        composition.Key
	binding    composition.Key
	member     composition.Key
	row        composition.Key
}

type siteRow struct {
	source      composition.Key
	scope       Scope
	init        Expr
	disposition InitDisposition
	formal      bool
	formalRole  composition.Key
	formalMode  PortMode
	formalReads []PortRead
	key         composition.Key
}

type occurrenceRow struct {
	kind   OccurrenceKind
	site   uint32
	entity composition.Key
	key    composition.Key
}

type operandRow struct {
	occurrence uint32
	entity     composition.Key
	key        composition.Key
	realm      composition.Key
}

type occurrenceIndex struct {
	kind   OccurrenceKind
	site   uint32
	entity composition.Key
}

type operandIndex struct {
	occurrence uint32
	entity     composition.Key
	realm      composition.Key
}

// Admission is one complete source-topology row set.  AdmitAll validates an
// entire set against the current open Batch before publishing any of its
// rows, so a source compiler cannot leave a prefix admitted when a later
// member is malformed.
type Admission struct {
	Source      composition.Key
	Scope       Scope
	Init        Expr
	Disposition InitDisposition
	Kind        OccurrenceKind
	Entity      composition.Key
	Operand     composition.Key
}

// NewBatch opens one topology-owned source admission transaction.
func NewBatch() *Batch {
	return &Batch{
		phase:        batchOpen,
		siteBySource: make(map[composition.Key]uint32),
		formalAt:     make(map[composition.Key]uint32),
		occurrenceAt: make(map[occurrenceIndex]uint32),
		operandAt:    make(map[operandIndex]uint32),
	}
}

// Sealed reports whether this exact Batch completed immutable admission.
func (batch *Batch) Sealed() bool {
	return batch != nil && batch.phase == batchSealed && batch.key.Available()
}

// Key is the canonical sealed source-topology identity.  It is intentionally
// unavailable while rows are mutable.
func (batch *Batch) Key() composition.Key {
	if !batch.Sealed() {
		return composition.Key{}
	}
	return batch.key
}

// AdmitSite admits one exact source entity and its already-issued scope plus
// initialization formula.  Re-admitting exactly the same row returns the
// same opaque capability; re-admitting the entity with a different issued
// scope or initialization is a rejected topology, never a second Site.
func (batch *Batch) AdmitSite(source composition.Key, scope Scope, init Expr, disposition InitDisposition) (Site, bool) {
	if batch == nil || batch.phase != batchOpen || !validSiteSource(source, scope, init, disposition) {
		batch.rejectOpen()
		return Site{}, false
	}
	if existing, found := batch.siteBySource[source]; found {
		row, ok := batch.openSite(existing)
		if !ok || row.formal || !sameScope(row.scope, scope) || !sameExpr(row.init, init) || row.disposition != disposition {
			batch.rejectOpen()
			return Site{}, false
		}
		return Site{batch: batch, row: existing}, true
	}
	admitted := siteRow{source: source, scope: scope, init: init, disposition: disposition}
	key, ok := deriveSiteKey(admitted)
	if !ok {
		batch.rejectOpen()
		return Site{}, false
	}
	admitted.key = key
	row := uint32(len(batch.sites) + 1)
	batch.sites = append(batch.sites, admitted)
	batch.siteBySource[source] = row
	return Site{batch: batch, row: row}, true
}

// At admits the exact occurrence of Site's own source entity.
func (batch *Batch) At(site Site) (Occurrence, bool) {
	row, ok := batch.openSiteFor(site)
	if !ok {
		return Occurrence{}, false
	}
	return batch.admitOccurrence(OccurrenceAt, site, row.source)
}

// From admits one exact directed-boundary source entity at Site.
func (batch *Batch) From(site Site, entity composition.Key) (Occurrence, bool) {
	return batch.admitOccurrence(OccurrenceFrom, site, entity)
}

// Relation admits one exact application relation entity at Site.
func (batch *Batch) Relation(site Site, entity composition.Key) (Occurrence, bool) {
	return batch.admitOccurrence(OccurrenceRelation, site, entity)
}

func (batch *Batch) admitOccurrence(kind OccurrenceKind, site Site, entity composition.Key) (Occurrence, bool) {
	if batch == nil || batch.phase != batchOpen || !validOccurrenceKind(kind) || !entity.Available() {
		batch.rejectOpen()
		return Occurrence{}, false
	}
	siteRowValue, ok := batch.openSiteFor(site)
	if !ok {
		return Occurrence{}, false
	}
	index := occurrenceIndex{kind: kind, site: site.row, entity: entity}
	if existing, found := batch.occurrenceAt[index]; found {
		return Occurrence{batch: batch, row: existing}, true
	}
	key, ok := deriveOccurrenceKey(kind, siteRowValue.key, entity)
	if !ok {
		batch.rejectOpen()
		return Occurrence{}, false
	}
	row := uint32(len(batch.occurrences) + 1)
	batch.occurrences = append(batch.occurrences, occurrenceRow{kind: kind, site: site.row, entity: entity, key: key})
	batch.occurrenceAt[index] = row
	return Occurrence{batch: batch, row: row}, true
}

// AdmitOperand admits one independent canonical entity identity for exactly
// one already-admitted Occurrence.  The entity may appear at another
// occurrence only through a distinct Operand row; an Operand cannot be
// reattached after seal or used with a different occurrence.
func (batch *Batch) AdmitOperand(occurrence Occurrence, entity composition.Key) (Operand, bool) {
	return batch.admitOperandInRealm(occurrence, entity, composition.Key{})
}

func (batch *Batch) admitOperandInRealm(occurrence Occurrence, entity, realm composition.Key) (Operand, bool) {
	if batch == nil || batch.phase != batchOpen || !entity.Available() {
		batch.rejectOpen()
		return Operand{}, false
	}
	occurrenceRowValue, ok := batch.openOccurrenceFor(occurrence)
	if !ok {
		return Operand{}, false
	}
	index := operandIndex{occurrence: occurrence.row, entity: entity, realm: realm}
	if existing, found := batch.operandAt[index]; found {
		return Operand{batch: batch, row: existing}, true
	}
	key, ok := deriveOperandKey(occurrenceRowValue.key, entity, realm)
	if !ok {
		batch.rejectOpen()
		return Operand{}, false
	}
	row := uint32(len(batch.operands) + 1)
	batch.operands = append(batch.operands, operandRow{occurrence: occurrence.row, entity: entity, key: key, realm: realm})
	batch.operandAt[index] = row
	return Operand{batch: batch, row: row}, true
}

// Seal validates the complete cross-row topology and derives every canonical
// identity once.  It performs no row sorting or capability rewriting, so
// issued opaque handles remain compact stable row references.
func (batch *Batch) Seal() bool { return !batch.SealWithFailure().Available() }

func (batch *Batch) SealWithFailure() SealFailure {
	if batch == nil || batch.phase != batchOpen || len(batch.sites) == 0 {
		if batch != nil {
			batch.rejectOpen()
		}
		return sealRefused(SealFailureFamilySource, "precondition")
	}
	formalCount := 0
	for index := range batch.sites {
		row := &batch.sites[index]
		if !validSiteRow(*row) {
			batch.rejectOpen()
			return sealRefused(SealFailureFamilySource, "site-row")
		}
		if row.formal {
			formalCount++
			mapped, found := batch.formalAt[row.formalRole]
			if !found || mapped != uint32(index+1) {
				batch.rejectOpen()
				return sealRefused(SealFailureFamilySource, "formal-coverage")
			}
		}
		key, ok := deriveSiteKey(*row)
		if !ok || key != row.key {
			batch.rejectOpen()
			return sealRefused(SealFailureFamilySource, "site-identity")
		}
	}
	if formalCount != len(batch.formalAt) {
		batch.rejectOpen()
		return sealRefused(SealFailureFamilySource, "formal-coverage")
	}
	for index := range batch.occurrences {
		row := &batch.occurrences[index]
		site, ok := batch.openSite(row.site)
		if !ok || !validOccurrenceKind(row.kind) || !row.entity.Available() {
			batch.rejectOpen()
			return sealRefused(SealFailureFamilySource, "occurrence-row")
		}
		key, ok := deriveOccurrenceKey(row.kind, site.key, row.entity)
		if !ok || key != row.key {
			batch.rejectOpen()
			return sealRefused(SealFailureFamilySource, "occurrence-identity")
		}
	}
	for index := range batch.operands {
		row := &batch.operands[index]
		occurrence, ok := batch.openOccurrence(row.occurrence)
		if !ok || !row.entity.Available() {
			batch.rejectOpen()
			return sealRefused(SealFailureFamilySource, "operand-row")
		}
		key, ok := deriveOperandKey(occurrence.key, row.entity, row.realm)
		if !ok || key != row.key {
			batch.rejectOpen()
			return sealRefused(SealFailureFamilySource, "operand-identity")
		}
	}
	key, ok := deriveBatchKey(batch)
	if !ok {
		batch.rejectOpen()
		return sealRefused(SealFailureFamilySource, "batch-identity")
	}
	batch.key, batch.phase = key, batchSealed
	for index := range batch.operands {
		if !batch.operands[index].realm.Available() {
			batch.operands[index].realm = key
		}
	}
	if failure := batch.sealTargetRowsWithFailure(); failure.Available() {
		batch.phase = batchRejected
		return failure
	}
	// Admission indexes are open-phase scratch. Sealed capabilities address
	// immutable rows directly, and dynamic activation derives overlays from
	// those capabilities; retaining these maps would preserve a second lookup
	// plane for the lifetime of every Solver without any semantic consumer.
	batch.siteBySource = nil
	batch.formalAt = nil
	batch.occurrenceAt = nil
	batch.operandAt = nil
	return SealFailure{}
}

// Reject permanently poisons one still-open construction transaction. It is
// used when a higher typed authority detects an overlapping or otherwise
// invalid claim after Batch-local rows may already have been admitted. A
// rejected Batch cannot seal or be repaired; callers must rebuild it.
func (batch *Batch) Reject() bool {
	if batch == nil || batch.phase != batchOpen {
		return false
	}
	batch.rejectOpen()
	return true
}

func (batch *Batch) rejectOpen() {
	if batch != nil && batch.phase == batchOpen {
		batch.phase = batchRejected
		// A rejected transaction can never be repaired or queried. Release both
		// its partial rows and admission indexes at the terminal boundary.
		batch.sites = nil
		batch.siteBySource = nil
		batch.formalAt = nil
		batch.occurrences = nil
		batch.occurrenceAt = nil
		batch.operands = nil
		batch.operandAt = nil
		batch.targets = targetRows{}
	}
}

func validSiteSource(source composition.Key, scope Scope, init Expr, disposition InitDisposition) bool {
	if !source.Available() || !scope.Available() || !init.Available() || disposition != InitAbsent && disposition != InitPresent {
		return false
	}
	for _, decision := range init.Decisions() {
		if !scope.contains(decision) {
			return false
		}
	}
	return disposition != InitAbsent || init.IsFalse()
}

func validSiteRow(row siteRow) bool {
	if row.formal {
		return validFormalPortRow(row)
	}
	return !row.formalRole.Available() && row.formalMode == PortInvalid && len(row.formalReads) == 0 &&
		validSiteSource(row.source, row.scope, row.init, row.disposition)
}

func validOccurrenceKind(kind OccurrenceKind) bool {
	return kind == OccurrenceAt || kind == OccurrenceFrom || kind == OccurrenceRelation
}

func (batch *Batch) openSite(row uint32) (siteRow, bool) {
	if batch == nil || batch.phase != batchOpen || row == 0 || uint64(row) > uint64(len(batch.sites)) {
		return siteRow{}, false
	}
	return batch.sites[row-1], true
}

func (batch *Batch) openSiteFor(site Site) (siteRow, bool) {
	if batch == nil || site.batch != batch {
		batch.rejectOpen()
		return siteRow{}, false
	}
	return batch.openSite(site.row)
}

func (batch *Batch) openOccurrence(row uint32) (occurrenceRow, bool) {
	if batch == nil || batch.phase != batchOpen || row == 0 || uint64(row) > uint64(len(batch.occurrences)) {
		return occurrenceRow{}, false
	}
	return batch.occurrences[row-1], true
}

func (batch *Batch) openOccurrenceFor(occurrence Occurrence) (occurrenceRow, bool) {
	if batch == nil || occurrence.batch != batch {
		batch.rejectOpen()
		return occurrenceRow{}, false
	}
	return batch.openOccurrence(occurrence.row)
}

func (batch *Batch) sealedSite(row uint32) (siteRow, bool) {
	if !batch.Sealed() || row == 0 || uint64(row) > uint64(len(batch.sites)) {
		return siteRow{}, false
	}
	value := batch.sites[row-1]
	return value, value.key.Available()
}

func (batch *Batch) sealedOccurrence(row uint32) (occurrenceRow, bool) {
	if !batch.Sealed() || row == 0 || uint64(row) > uint64(len(batch.occurrences)) {
		return occurrenceRow{}, false
	}
	value := batch.occurrences[row-1]
	return value, value.key.Available()
}

func (batch *Batch) sealedOperand(row uint32) (operandRow, bool) {
	if !batch.Sealed() || row == 0 || uint64(row) > uint64(len(batch.operands)) {
		return operandRow{}, false
	}
	value := batch.operands[row-1]
	return value, value.key.Available()
}

func (batch *Batch) ownsSite(site Site) bool {
	return batch != nil && site.batch == batch && site.Available()
}

func (batch *Batch) ownsConcreteSite(site Site) bool {
	if !batch.ownsSite(site) {
		return false
	}
	row, ok := batch.sealedSite(site.row)
	return ok && !row.formal
}

// OwnsSite proves that site is an issued, sealed capability of this exact
// Batch.  It is intentionally read-only: callers can fence a capability
// provenance without gaining access to the Batch's mutable admission state.
func (batch *Batch) OwnsSite(site Site) bool {
	return batch.ownsSite(site)
}

// OwnsOpenSite proves that site is an issued capability of this exact Batch
// while admission is still open. It is intentionally weaker than OwnsSite:
// no sealed identity is exposed before the target-Batch transaction reaches
// its phase barrier.
func (batch *Batch) OwnsOpenSite(site Site) bool {
	return batch != nil && batch.phase == batchOpen && site.batch == batch && site.row != 0 && uint64(site.row) <= uint64(len(batch.sites))
}

// OwnsOpenOccurrence proves that occurrence is one exact base row admitted by
// this Batch while its source phase is still open. It is the open-phase
// counterpart to OwnsOccurrence; callers gain only provenance validation, not
// access to the mutable row payload.
func (batch *Batch) OwnsOpenOccurrence(occurrence Occurrence) bool {
	if batch == nil || batch.phase != batchOpen || occurrence.batch != batch || occurrence.dynamic != nil {
		return false
	}
	_, ok := batch.openOccurrence(occurrence.row)
	return ok
}

// OwnsOpenOperand proves that operand is one exact base row admitted by this
// Batch while its source phase is still open. Dynamic operands are never
// source-admission witnesses.
func (batch *Batch) OwnsOpenOperand(operand Operand) bool {
	if batch == nil || batch.phase != batchOpen || operand.batch != batch || operand.dynamic != nil || operand.row == 0 || uint64(operand.row) > uint64(len(batch.operands)) {
		return false
	}
	row := batch.operands[operand.row-1]
	return row.occurrence != 0 && row.entity.Available()
}

// OwnsOpenOperandFor proves the open operand is attached to the exact open
// occurrence in this Batch. The relationship is kept inside Batch because
// Operand.Occurrence is intentionally a sealed-phase projection.
func (batch *Batch) OwnsOpenOperandFor(operand Operand, occurrence Occurrence) bool {
	if !batch.OwnsOpenOperand(operand) || !batch.OwnsOpenOccurrence(occurrence) {
		return false
	}
	return batch.operands[operand.row-1].occurrence == occurrence.row
}

func (batch *Batch) ownsOccurrence(occurrence Occurrence) bool {
	return batch != nil && occurrence.batch == batch && occurrence.Available()
}

// OwnsOccurrence proves that occurrence belongs to this exact sealed Batch.
func (batch *Batch) OwnsOccurrence(occurrence Occurrence) bool {
	return batch.ownsOccurrence(occurrence)
}

func (batch *Batch) ownsOperand(operand Operand) bool {
	return batch != nil && operand.batch == batch && operand.Available()
}

// OwnsOperand proves that operand belongs to this exact sealed Batch.
func (batch *Batch) OwnsOperand(operand Operand) bool {
	return batch.ownsOperand(operand)
}

// closesOperandRealms proves that every sealed base Operand participates in
// exactly one static realm: the ordinary base graph, or one activation
// binding's symbolic Template. Multiple Rule schemas may lawfully observe the
// same Operand inside that realm. Dynamic activation overlays are derived
// later and never enter this disposable closure check.
func (batch *Batch) closesOperandRealms(rules []RuleInstance) bool {
	if batch == nil || !batch.Sealed() {
		return false
	}
	// A topology with no ordinary Rules has no
	// operand realms to close. Treat that empty relation as vacuously closed;
	// requiring a synthetic operand would make a valid points/inputs-only
	// topology impossible to seal after target-batch assembly.
	if len(batch.operands) == 0 {
		return len(rules) == 0
	}
	realms := make([]composition.Key, len(batch.operands))
	mark := func(operand Operand, realm composition.Key) bool {
		if !operand.Available() || operand.batch != batch || operand.dynamic != nil || operand.row == 0 || uint64(operand.row) > uint64(len(realms)) {
			return false
		}
		index := int(operand.row - 1)
		if realms[index].Available() && realms[index] != realm {
			return false
		}
		realms[index] = realm
		return true
	}
	for _, rule := range rules {
		if rule.Operand.row == 0 || uint64(rule.Operand.row) > uint64(len(batch.operands)) || !mark(rule.Operand, batch.operands[rule.Operand.row-1].realm) {
			return false
		}
	}
	for index, realm := range realms {
		if !realm.Available() {
			return false
		}
		if row := batch.operands[index].realm; row.Available() && row != realm {
			return false
		}
	}
	return true
}

func (site Site) Available() bool {
	if site.dynamic != nil {
		base, ok := site.batch.sealedSite(site.row)
		return ok && site.dynamic.key.Available() && site.dynamic.binding.Available() && site.dynamic.member.Available() && site.dynamic.row == base.key && validSiteSource(base.source, site.dynamic.scope, site.dynamic.init, site.dynamic.disposition)
	}
	_, ok := site.batch.sealedSite(site.row)
	return ok
}
func (site Site) Key() composition.Key {
	if site.dynamic != nil {
		if !site.Available() {
			return composition.Key{}
		}
		return site.dynamic.key
	}
	row, ok := site.batch.sealedSite(site.row)
	if !ok {
		return composition.Key{}
	}
	return row.key
}
func (site Site) Scope() Scope {
	if site.dynamic != nil {
		if !site.Available() {
			return Scope{}
		}
		return site.dynamic.scope
	}
	row, ok := site.batch.sealedSite(site.row)
	if !ok {
		return Scope{}
	}
	return row.scope
}
func (site Site) Init() (Expr, InitDisposition, bool) {
	if site.dynamic != nil {
		if !site.Available() {
			return Expr{}, 0, false
		}
		return site.dynamic.init, site.dynamic.disposition, true
	}
	row, ok := site.batch.sealedSite(site.row)
	if !ok {
		return Expr{}, 0, false
	}
	return row.init, row.disposition, true
}
func (site Site) Source() composition.Key {
	row, ok := site.batch.sealedSite(site.row)
	if !ok {
		return composition.Key{}
	}
	return row.source
}
func (site Site) Same(other Site) bool {
	return site.Available() && other.Available() && site.batch == other.batch && site.row == other.row && site.Key() == other.Key()
}

func (occurrence Occurrence) Available() bool {
	if occurrence.dynamic != nil {
		base, ok := occurrence.batch.sealedOccurrence(occurrence.row)
		baseSite := Site{batch: occurrence.batch, row: base.site}
		return ok && base.key.Available() && occurrence.dynamic.key.Available() && occurrence.dynamic.site.Available() && occurrence.dynamic.site.dynamic != nil && sameBaseSite(baseSite, occurrence.dynamic.site) && occurrence.dynamic.binding.Available() && occurrence.dynamic.member.Available() && occurrence.dynamic.row.Available() && occurrence.dynamic.site.dynamic.binding == occurrence.dynamic.binding && occurrence.dynamic.site.dynamic.member == occurrence.dynamic.member && occurrence.dynamic.site.dynamic.row == baseSite.Key()
	}
	_, ok := occurrence.batch.sealedOccurrence(occurrence.row)
	return ok
}
func (occurrence Occurrence) Key() composition.Key {
	if occurrence.dynamic != nil {
		if !occurrence.Available() {
			return composition.Key{}
		}
		return occurrence.dynamic.key
	}
	row, ok := occurrence.batch.sealedOccurrence(occurrence.row)
	if !ok {
		return composition.Key{}
	}
	return row.key
}

// IdentityKey is the immutable occurrence identity available during both open
// source admission and after Batch sealing. It exposes no row coordinate and
// is used only to bind deferred surfaces to their pre-seal source.
func (occurrence Occurrence) IdentityKey() composition.Key {
	if occurrence.dynamic != nil {
		return occurrence.Key()
	}
	if occurrence.batch == nil {
		return composition.Key{}
	}
	if row, ok := occurrence.batch.sealedOccurrence(occurrence.row); ok {
		return row.key
	}
	if row, ok := occurrence.batch.openOccurrence(occurrence.row); ok {
		return row.key
	}
	return composition.Key{}
}
func (occurrence Occurrence) Kind() OccurrenceKind {
	row, ok := occurrence.batch.sealedOccurrence(occurrence.row)
	if !ok {
		return 0
	}
	return row.kind
}
func (occurrence Occurrence) Site() Site {
	if occurrence.dynamic != nil {
		if !occurrence.Available() {
			return Site{}
		}
		return occurrence.dynamic.site
	}
	row, ok := occurrence.batch.sealedOccurrence(occurrence.row)
	if !ok {
		return Site{}
	}
	return Site{batch: occurrence.batch, row: row.site}
}
func (occurrence Occurrence) Entity() composition.Key {
	row, ok := occurrence.batch.sealedOccurrence(occurrence.row)
	if !ok {
		return composition.Key{}
	}
	return row.entity
}
func (occurrence Occurrence) Same(other Occurrence) bool {
	return occurrence.Available() && other.Available() && occurrence.batch == other.batch && occurrence.row == other.row && occurrence.Key() == other.Key()
}

func (operand Operand) Available() bool {
	if operand.dynamic != nil {
		base, ok := operand.batch.sealedOperand(operand.row)
		baseOccurrence := Occurrence{batch: operand.batch, row: base.occurrence}
		overlayOccurrence := operand.dynamic.occurrence
		return ok && base.key.Available() && operand.dynamic.key.Available() && overlayOccurrence.Available() && overlayOccurrence.dynamic != nil && sameBaseOccurrence(baseOccurrence, overlayOccurrence) && operand.dynamic.binding.Available() && operand.dynamic.member.Available() && operand.dynamic.row.Available() && overlayOccurrence.dynamic.binding == operand.dynamic.binding && overlayOccurrence.dynamic.member == operand.dynamic.member && overlayOccurrence.dynamic.row == operand.dynamic.row
	}
	_, ok := operand.batch.sealedOperand(operand.row)
	return ok
}
func (operand Operand) Key() composition.Key {
	if operand.dynamic != nil {
		if !operand.Available() {
			return composition.Key{}
		}
		return operand.dynamic.key
	}
	row, ok := operand.batch.sealedOperand(operand.row)
	if !ok {
		return composition.Key{}
	}
	return row.key
}

// IdentityKey is Operand's open-or-sealed immutable admission identity.
func (operand Operand) IdentityKey() composition.Key {
	if operand.dynamic != nil {
		return operand.Key()
	}
	if operand.batch == nil {
		return composition.Key{}
	}
	if row, ok := operand.batch.sealedOperand(operand.row); ok {
		return row.key
	}
	if operand.batch.phase == batchOpen && operand.row != 0 && uint64(operand.row) <= uint64(len(operand.batch.operands)) {
		return operand.batch.operands[operand.row-1].key
	}
	return composition.Key{}
}
func (operand Operand) Occurrence() Occurrence {
	if operand.dynamic != nil {
		if !operand.Available() {
			return Occurrence{}
		}
		return operand.dynamic.occurrence
	}
	row, ok := operand.batch.sealedOperand(operand.row)
	if !ok {
		return Occurrence{}
	}
	return Occurrence{batch: operand.batch, row: row.occurrence}
}
func (operand Operand) Entity() composition.Key {
	row, ok := operand.batch.sealedOperand(operand.row)
	if !ok {
		return composition.Key{}
	}
	return row.entity
}
func (operand Operand) Same(other Operand) bool {
	return operand.Available() && other.Available() && operand.batch == other.batch && operand.row == other.row && operand.Key() == other.Key()
}

func sameBaseSite(left, right Site) bool {
	return left.Available() && right.Available() && left.batch == right.batch && left.row == right.row
}

func sameBaseOccurrence(left, right Occurrence) bool {
	return left.Available() && right.Available() && left.batch == right.batch && left.row == right.row
}

// sourceRowIdentityDerivations counts derivations of one source row's
// portable identity. A row owes exactly one at admission, where the identity
// is issued into the row, and one at seal, where the stored identity is
// authenticated. Every other observer reads the stored field, so the number
// of surfaces anchoring a row never enters this count.
var sourceRowIdentityDerivations atomic.Uint64

func deriveSiteKey(row siteRow) (composition.Key, bool) {
	sourceRowIdentityDerivations.Add(1)
	if row.formal {
		return deriveFormalPortSiteKey(row)
	}
	return identityKey("analysis/engine/equation/site", func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, row.source) && writeScope(writer, row.scope) && writeExpr(writer, row.init) && writer.Uint(uint64(row.disposition)) == nil
	})
}

func deriveOccurrenceKey(kind OccurrenceKind, site, entity composition.Key) (composition.Key, bool) {
	sourceRowIdentityDerivations.Add(1)
	return identityKey("analysis/engine/equation/occurrence", func(writer *canonical.DigestWriter) bool {
		return validOccurrenceKind(kind) && writeKey(writer, site) && writeKey(writer, entity) && writer.Uint(uint64(kind)) == nil
	})
}

func deriveOperandKey(occurrence, entity, realm composition.Key) (composition.Key, bool) {
	sourceRowIdentityDerivations.Add(1)
	return identityKey("analysis/engine/equation/operand", func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, occurrence) && writeKey(writer, entity) && (!realm.Available() || writeKey(writer, realm))
	})
}

func deriveBatchKey(batch *Batch) (composition.Key, bool) {
	if batch == nil || len(batch.sites) == 0 {
		return composition.Key{}, false
	}
	sites := make([]composition.Key, len(batch.sites))
	occurrences := make([]composition.Key, len(batch.occurrences))
	operands := make([]composition.Key, len(batch.operands))
	for index, row := range batch.sites {
		sites[index] = row.key
	}
	for index, row := range batch.occurrences {
		occurrences[index] = row.key
	}
	for index, row := range batch.operands {
		operands[index] = row.key
	}
	for _, values := range [][]composition.Key{sites, occurrences, operands} {
		sort.Slice(values, func(left, right int) bool { return lessKey(values[left], values[right]) })
		for index, key := range values {
			if !key.Available() || index > 0 && key == values[index-1] {
				return composition.Key{}, false
			}
		}
	}
	return identityKey("analysis/engine/equation/source-batch", func(writer *canonical.DigestWriter) bool {
		for _, values := range [][]composition.Key{sites, occurrences, operands} {
			if writer.Count(uint64(len(values))) != nil {
				return false
			}
			for _, key := range values {
				if !writeKey(writer, key) {
					return false
				}
			}
		}
		return true
	})
}

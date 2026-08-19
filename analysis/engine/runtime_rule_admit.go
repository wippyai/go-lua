// runtime_rule_admit.go holds the surface-placement value plane and the
// mount-qualified source identities one program issuance is addressed by.
//
// A placement is a value: RuleSourceTransaction accumulates the owner-issued
// surfaces of one issuance and is discarded when that issuance's declaration
// leaves. It retains no construction handle, admits nothing into a Batch, and
// cannot be carried from one issuance to another.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
)

// AnchoredSelectedReadSurface issues the ReadSelect surface from the exact
// admitted occurrence and operand. ReadSelect has no exact target unit: its
// local is a sealed identity of this occurrence/operand/read proof, not a
// caller-selected Ref.
type AnchoredSelectedReadFailure uint8

const (
	AnchoredSelectedReadFailureNone AnchoredSelectedReadFailure = iota
	AnchoredSelectedReadFailureArguments
	AnchoredSelectedReadFailureReceipt
	AnchoredSelectedReadFailureOwner
	AnchoredSelectedReadFailureSemantic
	AnchoredSelectedReadFailureDependencies
	AnchoredSelectedReadFailureDependencySurface
	AnchoredSelectedReadFailureFactor
	AnchoredSelectedReadFailureDuplicate
)

func (transaction *RuleSourceTransaction) AnchoredSelectedReadSurface(receipt schemaSelectedRead, dependencies []RuleReadSurface) (RuleReadSurface, bool) {
	surface, failure := transaction.AnchoredSelectedReadSurfaceWithFailure(receipt, dependencies)
	return surface, failure == AnchoredSelectedReadFailureNone
}

func (transaction *RuleSourceTransaction) AnchoredSelectedReadSurfaceWithFailure(receipt schemaSelectedRead, dependencies []RuleReadSurface) (RuleReadSurface, AnchoredSelectedReadFailure) {
	if transaction == nil || transaction.state == nil || transaction.schema == nil {
		return RuleReadSurface{}, AnchoredSelectedReadFailureArguments
	}
	if !receipt.Valid() || receipt.fence.authority == nil {
		return RuleReadSurface{}, AnchoredSelectedReadFailureReceipt
	}
	if receipt.fence.authority != transaction.authority || receipt.fence.schema != transaction.schema {
		return RuleReadSurface{}, AnchoredSelectedReadFailureOwner
	}
	ruleSemantic, ruleSemanticOK := semanticKeyFromComposition(receipt.fence.schema.ruleSemanticAt(receipt.fence.rule))
	if !ruleSemanticOK || ruleSemantic != transaction.semantic {
		return RuleReadSurface{}, AnchoredSelectedReadFailureSemantic
	}
	if len(dependencies) != int(receipt.dependencyCount) {
		return RuleReadSurface{}, AnchoredSelectedReadFailureDependencies
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() {
		return RuleReadSurface{}, AnchoredSelectedReadFailureFactor
	}
	for index, dependency := range dependencies {
		readIndex, ok := receipt.fence.schema.ruleReadDependencyAt(receipt.fence.rule, receipt.read, uint64(index))
		shape, shapeOK := receipt.fence.schema.ruleReadShapeAt(receipt.fence.rule, readIndex)
		if !ok || !shapeOK || dependency.authority != receipt.fence.authority || dependency.value.Mode != equation.TargetModeNone || dependency.value.Factor != shape.Factor || !dependency.value.LocalAvailable() || !validSelectedDependencySurface(shape, dependency.value) {
			return RuleReadSurface{}, AnchoredSelectedReadFailureDependencySurface
		}
	}
	content, contentOK := anchoredSelectedContent(transaction.anchor.occurrence, transaction.anchor.operand, receipt)
	if !contentOK {
		return RuleReadSurface{}, AnchoredSelectedReadFailureReceipt
	}
	for _, existing := range transaction.reads {
		if existing.value.Factor == factor && existing.value.Form == equation.SurfaceReadSelect && existing.value.Content == content {
			return RuleReadSurface{}, AnchoredSelectedReadFailureDuplicate
		}
	}
	surface := equation.Surface{Factor: factor, Form: equation.SurfaceReadSelect, Content: content, Semantic: factor}
	return RuleReadSurface{value: surface, authority: receipt.fence.authority, anchored: true}, AnchoredSelectedReadFailureNone
}

// AnchoredRouteWriteSurface is the route sibling of AnchoredSelectedReadSurface:
// the output has no single exact Ref because runtime chooses zero-or-many
// selected targets. Its local is tied to the admitted occurrence/operand and
// the sealed route proof.
func (transaction *RuleSourceTransaction) AnchoredRouteWriteSurface(receipt schemaRouteWrite) (RuleWriteSurface, bool) {
	if transaction == nil || transaction.state == nil || transaction.schema == nil || !receipt.Valid() || receipt.fence.authority == nil || receipt.fence.authority != transaction.authority || receipt.fence.schema != transaction.schema {
		return RuleWriteSurface{}, false
	}
	ruleSemantic, ruleSemanticOK := semanticKeyFromComposition(receipt.fence.schema.ruleSemanticAt(receipt.fence.rule))
	if !ruleSemanticOK || ruleSemantic != transaction.semantic {
		return RuleWriteSurface{}, false
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() {
		return RuleWriteSurface{}, false
	}
	content, contentOK := anchoredRouteContent(transaction.anchor.occurrence, transaction.anchor.operand, receipt)
	if !contentOK {
		return RuleWriteSurface{}, false
	}
	surface := equation.Surface{Factor: factor, Form: equation.SurfaceWriteRoute, Content: content}
	return RuleWriteSurface{value: surface, authority: receipt.fence.authority, route: &receipt, anchored: true}, true
}

// RuleSourceTransaction is the closed, issuance-owned surface placement
// envelope. It records only owner-issued geometry against the anchor the
// engine minted; no equation coordinate, cold factor ordinal, or construction
// handle can be supplied through this API.
type RuleSourceTransaction struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	schema    *Schema
	semantic  identity.SemanticKey
	anchor    ruleSurfaceAnchor
	reads     []RuleReadSurface
	writes    []RuleWriteSurface
	carries   uint64
}

func (transaction *RuleSourceTransaction) AddRead(surface RuleReadSurface) bool {
	if transaction == nil || !surface.value.Available() {
		return false
	}
	transaction.reads = append(transaction.reads, surface)
	return true
}

// AddExactRead is the generic typed convenience wrapper. Go does not permit
// type parameters on methods, so domain packages call this closed function.
func AddExactRead[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, ref Ref[K]) bool {
	surface, ok := ExactReadSurface(ref)
	return ok && transaction != nil && transaction.AddRead(surface)
}

func AddSummaryRead[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, receipt schemaSummaryRead, refs *ClosedRefs[K]) bool {
	surface, ok := SummaryReadSurface(receipt, refs)
	return ok && transaction != nil && transaction.AddRead(surface)
}

type summaryReadRefs interface {
	placeSummaryRead(transaction *RuleSourceTransaction, receipt schemaSummaryRead) bool
}

func (refs *ClosedRefs[K]) placeSummaryRead(transaction *RuleSourceTransaction, receipt schemaSummaryRead) bool {
	return AddSummaryRead(transaction, receipt, refs)
}

func addSummaryReadRefs(transaction *RuleSourceTransaction, receipt schemaSummaryRead, refs any) bool {
	placed, ok := refs.(summaryReadRefs)
	return ok && placed.placeSummaryRead(transaction, receipt)
}

func AddSelectedRead[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, receipt schemaSelectedRead, ref Ref[K], dependencies []RuleReadSurface) bool {
	surface, ok := SelectedReadSurface(receipt, ref, dependencies)
	return ok && transaction != nil && transaction.AddRead(surface)
}

func (transaction *RuleSourceTransaction) AddCarry() bool {
	if transaction == nil {
		return false
	}
	transaction.carries++
	return true
}

func (transaction *RuleSourceTransaction) AddWrite(surface RuleWriteSurface) bool {
	if transaction == nil || !surface.value.Available() {
		return false
	}
	transaction.writes = append(transaction.writes, surface)
	return true
}

func AddExactWrite[K ~uint32 | ~uint64](transaction *RuleSourceTransaction, ref Ref[K]) bool {
	surface, ok := ExactWriteSurface(ref)
	return ok && transaction != nil && transaction.AddWrite(surface)
}

func AddAnchoredRouteWrite(transaction *RuleSourceTransaction, receipt schemaRouteWrite) bool {
	surface, ok := transaction.AnchoredRouteWriteSurface(receipt)
	return ok && transaction.AddWrite(surface)
}

// resolveDeclaredRuleInstance folds one issuance's declared surfaces into the
// sealed equation row. The cold schema decides the shape: a declaration that
// places a different read count, an unowned Factor, or a route write without
// its sealed route proof has no row.
func resolveDeclaredRuleInstance(schema *Schema, authority *schemaBindingAuthority, semantic, family composition.Key, anchor ruleSurfaceAnchor, surfaces declaredRuleSurfaces) (equation.RuleInstance, bool) {
	if schema == nil || schema.cold == nil || authority == nil || !semantic.Available() || !family.Available() {
		return equation.RuleInstance{}, false
	}
	ordinal, ruleOK := schema.cold.RuleIndex(semantic)
	rule, rowOK := schema.cold.RuleAt(ordinal)
	if !ruleOK || !rowOK || rule.OperandFamily != family ||
		len(surfaces.reads) != len(rule.Reads) || len(surfaces.writes) != len(rule.Writes) ||
		surfaces.carries != uint64(len(rule.Carries)) || len(rule.Supports) != 0 || len(rule.Prunes) != 0 {
		return equation.RuleInstance{}, false
	}
	reads := make([]equation.ResolvedRead, len(rule.Reads))
	for index, read := range rule.Reads {
		surface := surfaces.reads[index]
		if surface.authority != authority || !surface.value.Available() || surface.value.Factor != read.Factor {
			return equation.RuleInstance{}, false
		}
		reads[index] = equation.ResolvedRead{Index: uint64(index), Surface: surface.value}
	}
	carries := make([]equation.ResolvedCarry, len(rule.Carries))
	for index := range carries {
		carries[index] = equation.ResolvedCarry{Index: uint64(index)}
	}
	writes := make([]equation.ResolvedWrite, len(rule.Writes))
	for index, write := range rule.Writes {
		surface := surfaces.writes[index]
		if surface.authority != authority || !surface.value.Available() || surface.value.Factor != write.Factor {
			return equation.RuleInstance{}, false
		}
		resolved := equation.ResolvedWrite{Index: uint64(index), Surface: surface.value}
		switch write.Kind {
		case composition.WriteExact:
			if surface.route != nil || surface.value.Form != equation.SurfaceWriteExact || surface.value.Mode != equation.TargetModeStrong {
				return equation.RuleInstance{}, false
			}
		case composition.WriteRoute:
			route := surface.route
			if route == nil || !route.Valid() || route.fence.authority != authority || route.fence.schema != schema || route.write != uint64(index) ||
				surface.value.Form != equation.SurfaceWriteRoute || surface.value.Mode != equation.TargetModeNone {
				return equation.RuleInstance{}, false
			}
			resolved.Route = route.read + 1
		default:
			return equation.RuleInstance{}, false
		}
		writes[index] = resolved
	}
	row := equation.RuleInstance{
		Schema: semantic, OperandFamily: family,
		Occurrence: anchor.occurrence, Operand: anchor.operand,
		Reads: reads, Carries: carries, Writes: writes,
	}
	if !validateBindingRuleRows(schema, row) {
		return equation.RuleInstance{}, false
	}
	return row, true
}

// declaredSummaryMappings collects the summary surfaces one issuance declared,
// in read order.
func declaredSummaryMappings(surfaces declaredRuleSurfaces) []RuleReadSurface {
	var mapped []RuleReadSurface
	for _, read := range surfaces.reads {
		if read.summary == nil || read.summary.receipt == nil {
			continue
		}
		mapped = append(mapped, read)
	}
	return mapped
}

const (
	mountedRuleMemberDomain     = "analysis/engine/rule-member"
	mountedRuleActivationDomain = "analysis/engine/activation-member"
	mountedRuleInputDomain      = "analysis/engine/rule-input"
	mountedRuleOccurrenceDomain = "analysis/engine/rule-occurrence"
	linkRuleOccurrenceDomain    = "analysis/engine/link-rule-occurrence"
	linkRuleMemberDomain        = "analysis/engine/link-rule-member"

	ruleSourceIdentityVersion uint64 = 3
)

func mountedRuleMemberID(role RuleSlotCapability, mount, point, occurrence identity.ContentID) identity.ContentID {
	if !role.mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return identity.ContentID{}
	}
	return framedContentID(mountedRuleMemberDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeRuleSlotCapability(writer, role) && writeContentIDs(writer, mount, point, occurrence)
	})
}

func mountedRuleActivationID(role RuleSlotCapability, mount, point, occurrence identity.ContentID) identity.ContentID {
	if !role.mounted() || !mount.Available() || !point.Available() || !occurrence.Available() {
		return identity.ContentID{}
	}
	return framedContentID(mountedRuleActivationDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeRuleSlotCapability(writer, role) && writeContentIDs(writer, mount, point, occurrence)
	})
}

func mountedRuleInputKey(member, input identity.ContentID, slot uint64) (composition.Key, bool) {
	if !member.Available() || !input.Available() {
		return composition.Key{}, false
	}
	id := framedContentID(mountedRuleInputDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeContentIDs(writer, member, input) && writer.Uint(slot) == nil
	})
	if !id.Available() {
		return composition.Key{}, false
	}
	return artifactSourceKey(artifactOccurrenceSource, id)
}

func linkRuleOccurrenceKey(role RuleSlotCapability, occurrence identity.ContentID) (composition.Key, bool) {
	if !role.link() || !occurrence.Available() {
		return composition.Key{}, false
	}
	id := framedContentID(linkRuleOccurrenceDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeRuleSlotCapability(writer, role) && writeContentIDs(writer, occurrence)
	})
	if !id.Available() {
		return composition.Key{}, false
	}
	return artifactSourceKey(artifactOccurrenceSource, id)
}

func linkRuleMemberID(role RuleSlotCapability, owner, point, occurrence identity.ContentID) identity.ContentID {
	if !role.link() || !owner.Available() || !point.Available() || !occurrence.Available() {
		return identity.ContentID{}
	}
	return framedContentID(linkRuleMemberDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeRuleSlotCapability(writer, role) && writeContentIDs(writer, owner, point, occurrence)
	})
}

// mountedRuleOccurrenceKey keeps the Batch occurrence entity family-local.
// One authored occurrence can feed several closed Rule roles; sharing the
// raw artifact ID would alias those independent semantic rows.
func mountedRuleOccurrenceKey(role RuleSlotCapability, occurrence identity.ContentID) (composition.Key, bool) {
	if !role.mounted() || !occurrence.Available() {
		return composition.Key{}, false
	}
	id := framedContentID(mountedRuleOccurrenceDomain, ruleSourceIdentityVersion, func(writer *canonical.DigestWriter) bool {
		return writeRuleSlotCapability(writer, role) && writeContentIDs(writer, occurrence)
	})
	if !id.Available() {
		return composition.Key{}, false
	}
	return artifactSourceKey(artifactOccurrenceSource, id)
}

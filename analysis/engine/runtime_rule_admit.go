// runtime_rule_admit.go holds the surface-placement value plane and the
// mount-qualified source identities one program issuance is addressed by.
//
// A placement is a value: the declaration returns an immutable row bundle of
// owner-issued surfaces for one issuance and is discarded when that
// declaration leaves. It retains no construction handle, admits nothing into
// a Batch, and cannot be carried from one issuance to another.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
)

// anchoredSelectedReadSurface issues the ReadSelect surface from the exact
// admitted occurrence and operand. ReadSelect has no exact target unit: its
// local is a sealed identity of this occurrence/operand/read proof, not a
// caller-selected Ref. The declaration passes its already-emitted reads so
// duplicate anchored coordinates can be rejected without a mutable builder.
func anchoredSelectedReadSurface(state *schemaBindingState, authority *schemaBindingAuthority, semantic identity.SemanticKey, anchor ruleSurfaceAnchor, receipt schemaSelectedRead, dependencies []RuleReadSurface, reads []RuleReadSurface) (RuleReadSurface, bool) {
	if state == nil || state.schema == nil || authority == nil || !semantic.Available() || !receipt.Valid() || receipt.fence.authority == nil || receipt.fence.authority != authority || receipt.fence.schema != state.schema {
		return RuleReadSurface{}, false
	}
	ruleSemantic, ruleSemanticOK := semanticKeyFromComposition(receipt.fence.schema.ruleSemanticAt(receipt.fence.rule))
	if !ruleSemanticOK || ruleSemantic != semantic || len(dependencies) != int(receipt.dependencyCount) {
		return RuleReadSurface{}, false
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() {
		return RuleReadSurface{}, false
	}
	for index, dependency := range dependencies {
		readIndex, ok := receipt.fence.schema.ruleReadDependencyAt(receipt.fence.rule, receipt.read, uint64(index))
		shape, shapeOK := receipt.fence.schema.ruleReadShapeAt(receipt.fence.rule, readIndex)
		if !ok || !shapeOK || dependency.authority != authority || dependency.value.Mode != equation.TargetModeNone || dependency.value.Factor != shape.Factor || !dependency.value.LocalAvailable() || !validSelectedDependencySurface(shape, dependency.value) {
			return RuleReadSurface{}, false
		}
	}
	content, contentOK := anchoredSelectedContent(anchor.occurrence, anchor.operand, receipt)
	if !contentOK {
		return RuleReadSurface{}, false
	}
	for _, existing := range reads {
		if existing.value.Factor == factor && existing.value.Form == equation.SurfaceReadSelect && existing.value.Content == content {
			return RuleReadSurface{}, false
		}
	}
	surface := equation.Surface{Factor: factor, Form: equation.SurfaceReadSelect, Content: content, Semantic: factor}
	return RuleReadSurface{value: surface, authority: authority, anchored: true}, true
}

// anchoredRouteWriteSurface is the route sibling of anchoredSelectedReadSurface:
// the output has no single exact Ref because runtime chooses zero-or-many
// selected targets. Its local is tied to the admitted occurrence/operand and
// the sealed route proof.
func anchoredRouteWriteSurface(state *schemaBindingState, authority *schemaBindingAuthority, semantic identity.SemanticKey, anchor ruleSurfaceAnchor, receipt schemaRouteWrite) (RuleWriteSurface, bool) {
	if state == nil || state.schema == nil || authority == nil || !semantic.Available() || !receipt.Valid() || receipt.fence.authority == nil || receipt.fence.authority != authority || receipt.fence.schema != state.schema {
		return RuleWriteSurface{}, false
	}
	ruleSemantic, ruleSemanticOK := semanticKeyFromComposition(receipt.fence.schema.ruleSemanticAt(receipt.fence.rule))
	if !ruleSemanticOK || ruleSemantic != semantic {
		return RuleWriteSurface{}, false
	}
	factor := receipt.fence.schema.factorSemanticAt(receipt.factor)
	if !factor.Available() {
		return RuleWriteSurface{}, false
	}
	content, contentOK := anchoredRouteContent(anchor.occurrence, anchor.operand, receipt)
	if !contentOK {
		return RuleWriteSurface{}, false
	}
	surface := equation.Surface{Factor: factor, Form: equation.SurfaceWriteRoute, Content: content}
	return RuleWriteSurface{value: surface, authority: authority, route: &receipt, anchored: true}, true
}

// summaryReadSurface is the callback-free type-erasure seam for generic
// ClosedRefs. It returns a row value; it cannot append to or retain a caller's
// construction state.
type summaryReadSurface interface {
	summaryReadSurface(schemaSummaryRead) (RuleReadSurface, bool)
}

func (refs *ClosedRefs[K]) summaryReadSurface(receipt schemaSummaryRead) (RuleReadSurface, bool) {
	return SummaryReadSurface(receipt, refs)
}

func readSummarySurface(receipt schemaSummaryRead, refs any) (RuleReadSurface, bool) {
	provider, ok := refs.(summaryReadSurface)
	if !ok || provider == nil {
		return RuleReadSurface{}, false
	}
	return provider.summaryReadSurface(receipt)
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
		if read.summary == nil || read.summary.binding == nil {
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

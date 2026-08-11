package collector

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/program/flow/role"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// ValueClaim admits the executable half of a claim. target is the optional
// Static sidecar target; this operation coordinates both owners atomically
// and Flow stores no static duplicate.
func (w FlowOperandsWriter) ValueClaim(span source.Span, owner keyspace.Term, claimKind kind.ValueClaimKind, operand, target keyspace.Term) keyspace.Term {
	return w.valueClaim(span, owner, claimKind, operand, target, false)
}

// valueClaim is the single Flow/Static coordination point. allowMissing is
// reserved for the lowerer's declare-then-fill path; the public one-shot
// operation still requires a TypeAs/TypeIs target at admission.
func (w FlowOperandsWriter) valueClaim(span source.Span, owner keyspace.Term, claimKind kind.ValueClaimKind, operand, target keyspace.Term, allowMissing bool) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || claimKind < kind.ValueClaimTypeAs || claimKind > kind.ValueClaimNonNil || !flowrole.ValueOccurrence(c.counts, operand) {
		return rejectTermMutationf(c, "program/lower/collector: invalid ValueClaim admission")
	}
	if claimKind == kind.ValueClaimNonNil && target != 0 {
		return rejectTermMutationf(c, "program/lower/collector: NonNil ValueClaim has target")
	}
	if claimKind != kind.ValueClaimNonNil && target == 0 && !allowMissing {
		return rejectTermMutationf(c, "program/lower/collector: ValueClaim target is missing")
	}
	if claimKind != kind.ValueClaimNonNil && target != 0 && !staticExistingNode(StaticRoot{collector: c}, target) {
		return rejectTermMutationf(c, "program/lower/collector: invalid ValueClaim target")
	}
	term := c.mint(keyspace.FamilyValueClaim, span)
	if term == 0 {
		return 0
	}
	c.flow.operands.claims = append(c.flow.operands.claims, flow.ValueClaim{Owner: owner, Operand: operand, Kind: claimKind})
	if claimKind != kind.ValueClaimNonNil {
		var err error
		if target == 0 {
			err = c.static.ClaimDeclare(term, term)
		} else {
			err = c.static.ClaimOneShot(term, term, target)
		}
		if err != nil {
			c.fail(err)
			return 0
		}
	}
	return term
}

func (w FlowOperandsWriter) DeclareValueClaim(span source.Span, owner keyspace.Term, claimKind kind.ValueClaimKind, operand keyspace.Term) keyspace.Term {
	return w.valueClaim(span, owner, claimKind, operand, 0, true)
}

// FillValueClaimTarget keeps the Flow occurrence and Static target as one
// explicit atomic collector coordination. NonNil claims cannot be filled.
func (w FlowOperandsWriter) FillValueClaimTarget(claim, target keyspace.Term) bool {
	c := w.collector
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, claim, keyspace.FamilyValueClaim) || !staticExistingNode(StaticRoot{collector: c}, target) {
		return rejectMutationf(c, "program/lower/collector: invalid ValueClaim fill")
	}
	if keyspace.TermOrdinal(claim) > uint32(len(c.flow.operands.claims)) || c.flow.operands.claims[keyspace.TermOrdinal(claim)-1].Kind == kind.ValueClaimNonNil {
		return rejectMutationf(c, "program/lower/collector: invalid NonNil ValueClaim fill")
	}
	if err := c.static.FillClaimTarget(claim, claim, target); err != nil {
		c.fail(err)
		return false
	}
	return true
}

func (w FlowOperandsWriter) TypeValue(span source.Span, owner, target keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !staticExistingNode(StaticRoot{collector: c}, target) || !validStaticTypeValueTarget(&c.static, c.counts, target) {
		return rejectTermMutationf(c, "program/lower/collector: invalid TypeValue admission")
	}
	term := c.mint(keyspace.FamilyTypeValue, span)
	if term == 0 {
		return 0
	}
	c.flow.operands.typeValues = append(c.flow.operands.typeValues, flow.TypeValue{Owner: owner})
	if err := c.static.TypeValueTarget(term, target); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

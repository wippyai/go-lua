package assembly

import (
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// ValueClaim admits the executable half of a claim. target is the optional
// Static sidecar target; this operation coordinates both owners atomically
// and Flow stores no static duplicate.
func (c *Collector) ValueClaim(span source.Span, owner keyspace.Term, claimKind kind.ValueClaimKind, operand, target keyspace.Term) keyspace.Term {
	return c.valueClaim(span, owner, claimKind, operand, target, false)
}

// valueClaim is the single Flow/Static coordination point. allowMissing is
// reserved for the lowerer's declare-then-fill path; the public one-shot
// operation still requires a TypeAs/TypeIs target at admission.
func (c *Collector) valueClaim(span source.Span, owner keyspace.Term, claimKind kind.ValueClaimKind, operand, target keyspace.Term, allowMissing bool) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	targetValid := true
	if target != 0 {
		targetValid = staticExistingNode(c, target)
		if !targetValid {
			return 0
		}
	}
	term := c.mint(keyspace.FamilyValueClaim, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitClaim(c.counts, term, owner, claimKind, operand, target, allowMissing, targetValid); err != nil {
		c.fail(err)
		return 0
	}
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

func (c *Collector) DeclareValueClaim(span source.Span, owner keyspace.Term, claimKind kind.ValueClaimKind, operand keyspace.Term) keyspace.Term {
	return c.valueClaim(span, owner, claimKind, operand, 0, true)
}

// FillValueClaimTarget keeps the Flow occurrence and Static target as one
// explicit atomic collector coordination. NonNil claims cannot be filled.
func (c *Collector) FillValueClaimTarget(claim, target keyspace.Term) bool {
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, claim, keyspace.FamilyValueClaim) || !staticExistingNode(c, target) {
		return rejectMutationf(c, "program/lower/collector: invalid ValueClaim fill")
	}
	claimRow, ok := c.flow.ClaimAt(int(keyspace.TermOrdinal(claim) - 1))
	if !ok || claimRow.Kind == kind.ValueClaimNonNil {
		return rejectMutationf(c, "program/lower/collector: invalid NonNil ValueClaim fill")
	}
	if err := c.static.FillClaimTarget(claim, claim, target); err != nil {
		c.fail(err)
		return false
	}
	return true
}

func (c *Collector) TypeValue(span source.Span, owner, target keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !staticExistingNode(c, target) || !c.static.ValidTypeValueTarget(c.counts, target) {
		return rejectTermMutationf(c, "program/lower/collector: invalid TypeValue admission")
	}
	term := c.mint(keyspace.FamilyTypeValue, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitTypeValue(c.counts, term, owner, target, true); err != nil {
		c.fail(err)
		return 0
	}
	if err := c.static.TypeValueTarget(term, target); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

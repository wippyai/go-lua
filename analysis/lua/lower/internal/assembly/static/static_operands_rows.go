package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
)

// Annotation is the only public Static operand construction path. Flow owns
// ClaimOneShot/FillClaimTarget and TypeValueTarget admission and calls the
// private row methods below while it mints their corresponding Flow terms.
func (rows *staticRows) AnnotationDeclare(term, scope, target keyspace.Term, name string) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyAnnotation, len(rows.annotations)); err != nil {
		return err
	}
	key, err := rawString(name)
	if err != nil {
		return err
	}
	if scope == 0 || target == 0 {
		return errors.New("program/lower/collector: incomplete annotation")
	}
	rows.annotations = append(rows.annotations, staticRawAnnotation{scope: scope, target: target, name: key})
	return nil
}

func (rows *staticRows) AnnotationFill(term, values keyspace.Term) error {
	index, err := denseOrdinal(term, keyspace.FamilyAnnotation, len(rows.annotations))
	if err != nil {
		return err
	}
	if rows.annotations[index].filled || values == 0 {
		return errors.New("program/lower/collector: annotation filled twice or missing Values")
	}
	rows.annotations[index].values, rows.annotations[index].filled = values, true
	return nil
}

// ClaimDeclare reserves one sparse Static sidecar for a TypeAs/TypeIs Flow
// claim. The zero Target is an intentional unfinished state; freeze rejects
// it unless the one FillClaimTarget operation arrives first.
func (rows *staticRows) ClaimDeclare(term, claim keyspace.Term) error {
	if rows == nil || term == 0 || keyspace.TermFamily(term) != keyspace.FamilyValueClaim || claim != term {
		return errors.New("program/lower/collector: invalid sparse Claim declaration")
	}
	for _, row := range rows.claims {
		if row.Claim == claim {
			return errors.New("program/lower/collector: duplicate sparse Claim declaration")
		}
	}
	rows.claims = append(rows.claims, staticoperands.ClaimTarget{Claim: claim})
	return nil
}

// ClaimOneShot appends a complete sparse sidecar. It is intentionally
// separate from FillClaimTarget: a direct ValueClaim must never silently
// convert into a declaration or overwrite an existing declaration.
func (rows *staticRows) ClaimOneShot(term, claim, target keyspace.Term) error {
	if rows == nil || term == 0 || keyspace.TermFamily(term) != keyspace.FamilyValueClaim || claim != term || target == 0 {
		return errors.New("program/lower/collector: invalid one-shot sparse Claim target")
	}
	for _, row := range rows.claims {
		if row.Claim != claim {
			continue
		}
		return errors.New("program/lower/collector: duplicate one-shot sparse Claim")
	}
	rows.claims = append(rows.claims, staticoperands.ClaimTarget{Claim: claim, Target: target})
	return nil
}

// FillClaimTarget completes only a previously declared zero-target sidecar.
// Absence is an error rather than an implicit declaration, which keeps the
// Flow declare/fill protocol explicit and state-machine complete.
func (rows *staticRows) FillClaimTarget(term, claim, target keyspace.Term) error {
	if rows == nil || term == 0 || keyspace.TermFamily(term) != keyspace.FamilyValueClaim || claim != term || target == 0 {
		return errors.New("program/lower/collector: invalid sparse Claim fill")
	}
	for index, row := range rows.claims {
		if row.Claim != claim {
			continue
		}
		if row.Target != 0 {
			return errors.New("program/lower/collector: sparse Claim target filled twice")
		}
		rows.claims[index].Target = target
		return nil
	}
	return errors.New("program/lower/collector: sparse Claim fill requires declaration")
}

func (rows *staticRows) TypeValueTarget(term, target keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeValue, len(rows.typeValues)); err != nil {
		return err
	}
	if target == 0 {
		return errors.New("program/lower/collector: missing TypeValue target")
	}
	rows.typeValues = append(rows.typeValues, staticoperands.TypeValueTarget{Target: target})
	return nil
}

package flow

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

// resolveBreakTargets is the lower assembly's one canonical owner/target
// boundary. Break rows retain their lexical Body owner, while the authored
// target is selected from the immutable Source Body nesting and the authored
// Loop→Body rows before Flow is built. No causal projection is consulted.
func (r *Rows) resolveBreakTargets(preimage programsource.Preimage, counts [keyspace.FamilyCount]uint32) error {
	if r == nil {
		return errors.New("program/lower/collector: nil Flow rows")
	}
	bodyCount := int(counts[keyspace.FamilyBody])
	parents := make([]keyspace.Term, bodyCount+1)
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		length, ok := preimage.Order().BodyLen(body)
		if !ok {
			return fmt.Errorf("program/lower/collector: missing Source order for Body %v", body)
		}
		for index := 0; index < length; index++ {
			child, childOK := preimage.Order().BodyAt(body, index)
			if !childOK || keyspace.TermFamily(child) != keyspace.FamilyBody {
				continue
			}
			childOrdinal := keyspace.TermOrdinal(child)
			if childOrdinal == 0 || int(childOrdinal) > bodyCount || (parents[childOrdinal] != 0 && parents[childOrdinal] != body) {
				return fmt.Errorf("program/lower/collector: invalid Source Body parent for %v", child)
			}
			parents[childOrdinal] = body
		}
	}
	setParent := func(child, parent keyspace.Term) error {
		childOrdinal := keyspace.TermOrdinal(child)
		parentOrdinal := keyspace.TermOrdinal(parent)
		if keyspace.TermFamily(child) != keyspace.FamilyBody || childOrdinal == 0 || int(childOrdinal) > bodyCount ||
			keyspace.TermFamily(parent) != keyspace.FamilyBody || parentOrdinal == 0 || int(parentOrdinal) > bodyCount {
			return fmt.Errorf("program/lower/collector: invalid Body parent relation %v -> %v", parent, child)
		}
		if parents[childOrdinal] != 0 && parents[childOrdinal] != parent {
			return fmt.Errorf("program/lower/collector: conflicting Body parents for %v", child)
		}
		parents[childOrdinal] = parent
		return nil
	}
	for _, row := range r.control.branches {
		if err := setParent(row.WhenTrue, row.Owner); err != nil {
			return err
		}
		if err := setParent(row.WhenFalse, row.Owner); err != nil {
			return err
		}
	}

	loopByBody := make([]keyspace.Term, bodyCount+1)
	for index, row := range r.control.loops {
		bodyOrdinal := keyspace.TermOrdinal(row.Body)
		if bodyOrdinal == 0 || int(bodyOrdinal) > bodyCount {
			return fmt.Errorf("program/lower/collector: Loop %d has invalid Body", index+1)
		}
		if loopByBody[bodyOrdinal] != 0 {
			return fmt.Errorf("program/lower/collector: Body %v has duplicate Loop owners", row.Body)
		}
		loopByBody[bodyOrdinal] = keyspace.MakeTerm(keyspace.FamilyLoop, uint32(index+1))
		if err := setParent(row.Body, row.Owner); err != nil {
			return err
		}
	}
	functionBody := make([]bool, bodyCount+1)
	for _, row := range r.functions.rows {
		bodyOrdinal := keyspace.TermOrdinal(row.Body)
		if bodyOrdinal == 0 || int(bodyOrdinal) > bodyCount {
			return errors.New("program/lower/collector: Function has invalid Body")
		}
		functionBody[bodyOrdinal] = true
		if err := setParent(row.Body, row.Owner); err != nil {
			return err
		}
	}

	for index := range r.control.breaks {
		owner := r.control.breaks[index].Owner
		ownerOrdinal := keyspace.TermOrdinal(owner)
		if keyspace.TermFamily(owner) != keyspace.FamilyBody || ownerOrdinal == 0 || int(ownerOrdinal) > bodyCount {
			return fmt.Errorf("program/lower/collector: Break %d has invalid Body owner", index+1)
		}
		current := owner
		target := keyspace.Term(0)
		for current != 0 {
			currentOrdinal := keyspace.TermOrdinal(current)
			if functionBody[currentOrdinal] {
				break
			}
			if loop := loopByBody[currentOrdinal]; loop != 0 {
				target = loop
				break
			}
			parent := parents[currentOrdinal]
			if parent == 0 {
				break
			}
			current = parent
		}
		if target == 0 {
			return fmt.Errorf("program/lower/collector: Break %d has no same-function Loop target", index+1)
		}
		if r.control.breaks[index].Target != 0 && r.control.breaks[index].Target != target {
			return fmt.Errorf("program/lower/collector: Break %d target disagrees with Body topology", index+1)
		}
		r.control.breaks[index].Target = target
	}
	return nil
}

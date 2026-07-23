package transformer

import "github.com/wippyai/go-lua/analysis/lexicalidentity"

// RelationBodyPlanObservation is the detached scheduling classification of one
// frozen lexical body.  It carries no executor, solve state, or mutable WTO
// workspace.
type RelationBodyPlanObservation struct {
	Body    lexicalidentity.StableLexicalBodyID
	Acyclic bool
}

// BodyPlanObservations returns the canonical per-body WTO classification.
// A body is acyclic precisely when it is absent from the frozen recursive SCC
// membership.  The returned slice is in canonical RelationProgram body order.
func (p *RelationProgram) BodyPlanObservations() []RelationBodyPlanObservation {
	if p == nil {
		return nil
	}
	recursive := make(map[relationVar]struct{})
	for _, component := range p.recursiveSCCs {
		for _, member := range component {
			recursive[member] = struct{}{}
		}
	}
	out := make([]RelationBodyPlanObservation, 0, len(p.bodies))
	for index, body := range p.bodies {
		_, cyclic := recursive[relationVar(index+1)]
		out = append(out, RelationBodyPlanObservation{Body: body.body, Acyclic: !cyclic})
	}
	return out
}

// BodyAcyclic reports the frozen WTO classification for body.  The second
// result is false when body is not owned by this relation program.
func (p *RelationProgram) BodyAcyclic(body lexicalidentity.StableLexicalBodyID) (acyclic bool, ok bool) {
	if p == nil {
		return false, false
	}
	variable, ok := p.byBody[body]
	if !ok {
		return false, false
	}
	for _, component := range p.recursiveSCCs {
		for _, member := range component {
			if member == variable {
				return false, true
			}
		}
	}
	return true, true
}

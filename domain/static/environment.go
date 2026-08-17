package static

import "github.com/wippyai/go-lua/analysis/identity"

// Environment is the evaluator's internal static context.  The canonical
// Static authority currently admits only the empty environment: call-site
// substitution belongs to selected endpoint Rules, not this declaration
// classifier.
type Environment struct {
	owner *Authority
	id    identity.ContentID
}

func (e Environment) Valid() bool { return e.owner == nil && !e.id.Available() }

func (e Environment) ContentID() identity.ContentID {
	if !e.Valid() {
		return identity.ContentID{}
	}
	return e.id
}

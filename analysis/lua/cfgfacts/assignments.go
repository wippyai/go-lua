package cfgfacts

import "github.com/wippyai/go-lua/analysis/symbol"

// AssignmentFact describes an assignment target.
type AssignmentFact struct {
	Target symbol.ID
}

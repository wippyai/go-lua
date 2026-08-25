package relation

import (
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/materialization"
)

// RouteFact is the typed payload one observed receiver route carries: the kind
// of route the sealed topology denotes and the role its root is materialized
// under.
type RouteFact struct {
	Kind indexdomain.RouteKind
	Role materialization.Role
}

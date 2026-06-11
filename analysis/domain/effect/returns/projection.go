package returns

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/type/projection"
)

type TypeProjection struct {
	Source     effect.ParamRef
	Projection projection.Projection
}

func (TypeProjection) returnType() {}
func (p TypeProjection) String() string {
	path := p.Projection.String()
	if path == "" {
		return fmt.Sprintf("project_type(%s)", p.Source)
	}
	return fmt.Sprintf("project_type(%s.%s)", p.Source, path)
}

package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
)

func infallibleMaterializedBuild(build func(summary.Reader) body.Config) func(summary.Reader) (body.Config, error) {
	return func(reader summary.Reader) (body.Config, error) {
		return build(reader), nil
	}
}

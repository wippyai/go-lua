package engine

import (
	"fmt"
	"os"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/semantic"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/stage"
)

func DbgPlaneReset() {
	diagram.DbgReset()
	semantic.DbgReset()
	stage.DbgReset()
}

func DbgPlaneDump(tag string) {
	fmt.Fprintf(os.Stderr, "=== PLANE PROBE %s ===\n%s%s%s", tag, diagram.DbgReport(), semantic.DbgReport(), stage.DbgReport())
}

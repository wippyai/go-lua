package semanticpath

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
)

func TestIndexBodyRelationsKeepsUnavailableRowsClosed(t *testing.T) {
	rows, children, roots, err := indexBodyRelations(authored.View{}, &body.Result{})
	if err != nil {
		t.Fatalf("empty Body relation index returned error: %v", err)
	}
	if len(rows) != 0 || len(children) != 0 || len(roots) != 0 {
		t.Fatalf("empty Body relation index = %d rows, %d child lists, %d roots; want empty", len(rows), len(children), len(roots))
	}
}

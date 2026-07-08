package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

type localAssignmentReader interface {
	LocalAssignment(cfg.Point) (semantics.LocalAssignmentFact, bool)
}

type localAssignmentViewReader interface {
	LocalAssignmentView(cfg.Point) (semantics.LocalAssignmentFactView, bool)
}

type ordinaryAssignmentReader interface {
	OrdinaryAssignment(cfg.Point) (semantics.OrdinaryAssignmentFact, bool)
}

type ordinaryAssignmentViewReader interface {
	OrdinaryAssignmentView(cfg.Point) (semantics.OrdinaryAssignmentFactView, bool)
}

func hasOrdinaryAssignmentFactReader(result ResultReader) bool {
	if _, ok := result.(ordinaryAssignmentViewReader); ok {
		return true
	}
	_, ok := result.(ordinaryAssignmentReader)
	return ok
}

func localAssignmentFactAt(result ResultReader, point cfg.Point) (semantics.LocalAssignmentFact, bool) {
	if reader, ok := result.(localAssignmentViewReader); ok {
		view, ok := reader.LocalAssignmentView(point)
		if !ok {
			return semantics.LocalAssignmentFact{}, false
		}
		return view.Borrowed()
	}
	if reader, ok := result.(localAssignmentReader); ok {
		return reader.LocalAssignment(point)
	}
	return semantics.LocalAssignmentFact{}, false
}

func ordinaryAssignmentFactAt(result ResultReader, point cfg.Point) (semantics.OrdinaryAssignmentFact, bool) {
	if reader, ok := result.(ordinaryAssignmentViewReader); ok {
		view, ok := reader.OrdinaryAssignmentView(point)
		if !ok {
			return semantics.OrdinaryAssignmentFact{}, false
		}
		return view.Borrowed()
	}
	if reader, ok := result.(ordinaryAssignmentReader); ok {
		return reader.OrdinaryAssignment(point)
	}
	return semantics.OrdinaryAssignmentFact{}, false
}

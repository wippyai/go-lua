package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// WIRAssignmentTargetIssueKind identifies why the WIR target-shadow oracle could
// not match the existing fact lane for a write point.
type WIRAssignmentTargetIssueKind uint8

const (
	// WIRAssignmentTargetMissing means the point has a path-assignment fact but
	// no WIR assignment instruction naming the same write-local target.
	WIRAssignmentTargetMissing WIRAssignmentTargetIssueKind = iota
	// WIRAssignmentTargetMismatch means WIR named a write target, but it resolved
	// to a different write-local state key than the existing fact lane.
	WIRAssignmentTargetMismatch
)

// WIRAssignmentTargetIssue is emitted by the optional WIR assignment-target
// shadow. It is an internal migration oracle: production transfer semantics do
// not change when a reporter is absent.
type WIRAssignmentTargetIssue struct {
	Kind     WIRAssignmentTargetIssueKind
	Point    cfg.Point
	FactPath pathdom.Path
	FactKey  keyspace.Key
	WIROp    wir.Op
	WIRKey   keyspace.Key
}

// WIRAssignmentTargetIssueReporter receives WIR assignment-target shadow issues.
// Tests and migration harnesses use it to prove WIR and the legacy fact lane
// address the same point-local state cell before a semantic flip.
type WIRAssignmentTargetIssueReporter func(WIRAssignmentTargetIssue)

type wirAssignmentTargetShadow struct {
	body      *wir.Body
	resolver  *visibility.Resolver
	addresses wir.AddressResolver
	report    WIRAssignmentTargetIssueReporter
}

func newWIRAssignmentTargetShadow(body *wir.Body, resolver *visibility.Resolver, report WIRAssignmentTargetIssueReporter) *wirAssignmentTargetShadow {
	if body == nil || resolver == nil || report == nil {
		return nil
	}
	return &wirAssignmentTargetShadow{
		body:      body,
		resolver:  resolver,
		addresses: wir.NewCachingResolver(NewWIRAddressResolver(body, resolver)),
		report:    report,
	}
}

func (s *wirAssignmentTargetShadow) CheckPathWrite(point cfg.Point, factPath pathdom.Path) {
	if s == nil || factPath.IsEmpty() || len(factPath.Segments) == 0 {
		return
	}
	factKey, ok := visibility.AddressAt(s.resolver, point, factPath).VisibleLocalKeyspaceKey()
	if !ok {
		return
	}
	var firstIssue WIRAssignmentTargetIssue
	hasIssue := false
	for _, inst := range s.body.PointInstructions(point) {
		if !wirInstructionMayWritePathTarget(inst.Op) {
			continue
		}
		if inst.Dst.Kind != wir.OperandPath {
			continue
		}
		wirKey, ok := s.addresses.Resolve(point, inst.Dst, wir.AccessWriteLocal)
		if !ok {
			continue
		}
		if wirKey == factKey {
			return
		}
		if !hasIssue {
			firstIssue = WIRAssignmentTargetIssue{
				Kind:     WIRAssignmentTargetMismatch,
				Point:    point,
				FactPath: factPath,
				FactKey:  factKey,
				WIROp:    inst.Op,
				WIRKey:   wirKey,
			}
			hasIssue = true
		}
	}
	if hasIssue {
		s.report(firstIssue)
		return
	}
	s.report(WIRAssignmentTargetIssue{
		Kind:     WIRAssignmentTargetMissing,
		Point:    point,
		FactPath: factPath,
		FactKey:  factKey,
	})
}

func wirInstructionMayWritePathTarget(op wir.Op) bool {
	switch op {
	case wir.OpAssign, wir.OpStaticMemberWrite:
		return true
	default:
		return false
	}
}

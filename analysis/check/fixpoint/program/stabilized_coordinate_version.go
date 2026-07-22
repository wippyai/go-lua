package program

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// stabilizedCoordinateFingerprint is the common route-free semantic carrier
// used to version both the retiring application DTO and the canonical lexical
// body DTO. Prefix fields retain application-only boundary observations while
// the maps are the shared Result publication surface.
type stabilizedCoordinateFingerprint struct {
	prefixReachable []bool
	prefixStates    []state.State
	pointInputs     map[cfg.Point]state.State
	pointOutputs    map[cfg.Point]state.State
	pointReachable  map[cfg.Point]bool
	outputReachable map[cfg.Point]bool
	edgeNormal      map[cfg.Edge]bool
	callOutcomes    map[cfg.Point]callpayload.CallOutcome
	diagnostics     callpayload.DiagnosticOutput
}

func stabilizedCoordinateSemanticVersion(
	ctx context.Context,
	factory *body.ExecutionFactory,
	label string,
	coordinates stabilizedCoordinateFingerprint,
) (uint64, error) {
	if ctx == nil || factory == nil || factory.Registry() == nil || factory.KeySpace() == nil {
		return 0, fmt.Errorf("program: %s has no fingerprint authority", label)
	}
	h := fnv.New64a()
	var scratch [8]byte
	writeUint64 := func(value uint64) {
		binary.LittleEndian.PutUint64(scratch[:], value)
		_, _ = h.Write(scratch[:])
	}
	writeState := func(value state.State) error {
		digest, err := state.SemanticFingerprint(state.FingerprintConfig{
			Context: ctx, Registry: factory.Registry(), KeySpace: factory.KeySpace(),
		}, value)
		if err != nil {
			return err
		}
		writeUint64(digest)
		return nil
	}
	for _, reachable := range coordinates.prefixReachable {
		if reachable {
			writeUint64(1)
		} else {
			writeUint64(0)
		}
	}
	for _, value := range coordinates.prefixStates {
		if err := writeState(value); err != nil {
			return 0, err
		}
	}
	writeStateMap := func(values map[cfg.Point]state.State) error {
		points := make([]int, 0, len(values))
		for point := range values {
			points = append(points, int(point))
		}
		sort.Ints(points)
		writeUint64(uint64(len(points)))
		for _, raw := range points {
			writeUint64(uint64(raw))
			if err := writeState(values[cfg.Point(raw)]); err != nil {
				return err
			}
		}
		return nil
	}
	if err := writeStateMap(coordinates.pointInputs); err != nil {
		return 0, err
	}
	if err := writeStateMap(coordinates.pointOutputs); err != nil {
		return 0, err
	}
	writeBoolMap := func(values map[cfg.Point]bool) {
		points := make([]int, 0, len(values))
		for point := range values {
			points = append(points, int(point))
		}
		sort.Ints(points)
		writeUint64(uint64(len(points)))
		for _, raw := range points {
			writeUint64(uint64(raw))
			if values[cfg.Point(raw)] {
				writeUint64(1)
			} else {
				writeUint64(0)
			}
		}
	}
	writeBoolMap(coordinates.pointReachable)
	writeBoolMap(coordinates.outputReachable)
	edges := make([]cfg.Edge, 0, len(coordinates.edgeNormal))
	for edge := range coordinates.edgeNormal {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	writeUint64(uint64(len(edges)))
	for _, edge := range edges {
		writeUint64(uint64(edge.From))
		writeUint64(uint64(edge.To))
		if coordinates.edgeNormal[edge] {
			writeUint64(1)
		} else {
			writeUint64(0)
		}
	}
	callPoints := make([]int, 0, len(coordinates.callOutcomes))
	for point := range coordinates.callOutcomes {
		callPoints = append(callPoints, int(point))
	}
	sort.Ints(callPoints)
	writeUint64(uint64(len(callPoints)))
	for _, raw := range callPoints {
		writeUint64(uint64(raw))
		digest, err := summary.CanonicalCallOutcomeDigestContext(ctx, factory.Registry(), factory.KeySpace(), coordinates.callOutcomes[cfg.Point(raw)])
		if err != nil {
			return 0, err
		}
		writeUint64(uint64(digest))
	}
	writeUint64(coordinates.diagnostics.Fingerprint(factory.Registry()))
	version := h.Sum64()
	if version == 0 {
		return 0, fmt.Errorf("program: %s has zero stabilized version", label)
	}
	return version, nil
}

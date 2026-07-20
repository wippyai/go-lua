package transformer

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

type formalApplyCoordinateStaticTrace struct {
	image, selector state.CoordinateFactorInventory
	cells           []state.CoordinateFactorInventory
	sourceOwned     []state.CoordinateSlot
	footprint       state.BoundaryCoordinateFootprintTrace
}

var formalApplyCoordinateTraceSpec = strings.TrimSpace(os.Getenv("GOLUA_TRACE_APPLY_COORDINATES"))

func formalApplyTraceConfigured() bool { return formalApplyCoordinateTraceSpec != "" }

func formalApplyTraceEnabled(owner relationVar, frame callFrameTerm) bool {
	raw := formalApplyCoordinateTraceSpec
	if raw == "" {
		return false
	}
	parts := strings.Split(raw, ":")
	wantOwner, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil || relationVar(wantOwner) != owner {
		return false
	}
	if len(parts) == 1 {
		return true
	}
	wantFrame, err := strconv.ParseUint(parts[1], 10, 32)
	return err == nil && callFrameTerm(wantFrame) == frame
}

func formalApplyFactorSlots(domain state.ProductDomain, keys *keyspace.KeySpace, groups []formalFiberGroupDescriptor, factors []state.LaneFactor) ([]state.CoordinateSlot, error) {
	var out []state.CoordinateSlot
	for index, group := range groups {
		if index >= len(factors) {
			return nil, fmt.Errorf("factor trace width differs from groups")
		}
		families, err := domain.CoordinateFamilies(group.lane)
		if err != nil {
			return nil, err
		}
		for _, family := range families {
			_, scalars, err := domain.DecomposeCoordinateFamily(factors[index], family, keys)
			if err != nil {
				return nil, err
			}
			for _, scalar := range scalars {
				out = append(out, scalar.Slot())
			}
		}
	}
	return out, nil
}

func firstCoordinateInventoryDifference(domain state.ProductDomain, runtime []state.CoordinateSlot, frozen state.CoordinateFactorInventory) (state.CoordinateSlot, bool, error) {
	for _, slot := range runtime {
		present, err := frozen.Contains(domain, slot)
		if err != nil {
			return state.CoordinateSlot{}, false, err
		}
		if !present {
			return slot, true, nil
		}
	}
	return state.CoordinateSlot{}, false, nil
}

func coordinateInventoryDifferences(domain state.ProductDomain, runtime []state.CoordinateSlot, frozen state.CoordinateFactorInventory) ([]state.CoordinateSlot, error) {
	out := make([]state.CoordinateSlot, 0)
	for _, slot := range runtime {
		present, err := frozen.Contains(domain, slot)
		if err != nil {
			return nil, err
		}
		if !present {
			out = append(out, slot)
		}
	}
	return out, nil
}

func coordinateSlotPresent(domain state.ProductDomain, inventory []state.CoordinateSlot, wanted state.CoordinateSlot) bool {
	for _, candidate := range inventory {
		equal, err := domain.CoordinateSlotEqual(candidate, wanted)
		if err == nil && equal {
			return true
		}
	}
	return false
}

func traceFormalApplyCoordinates(program *RelationProgram, step *formalApplyStep, region formalApplyCorrelatedRegion, before, source, after []state.LaneFactor, runtimeRootMap state.BoundaryRootMap) {
	if program == nil || step == nil || !formalApplyTraceEnabled(step.owner, step.frame) {
		return
	}
	span, ok := program.formalFibers.span(step.owner)
	if !ok {
		return
	}
	beforeSlots, beforeErr := formalApplyFactorSlots(region.caller.authority.product, span.keys, region.caller.layout.nonValues, before)
	sourceSlots, sourceErr := formalApplyFactorSlots(region.target.authority.product, region.target.authority.coordinateKeys, region.target.layout.nonValues, source)
	afterSlots, afterErr := formalApplyFactorSlots(region.caller.authority.product, span.keys, region.caller.layout.nonValues, after)
	escaped, found, diffErr := firstCoordinateInventoryDifference(region.caller.authority.product, afterSlots, span.coordinates)
	static := program.formalFibers.applyCoordinateTrace[formalFrameFootprintKey{variable: step.owner, frame: step.frame}]
	sourceMissing, sourceDiffErr := coordinateInventoryDifferences(region.target.authority.product, sourceSlots, static.selector)
	fmt.Fprintf(os.Stderr, "APPLY_COORD_TRACE owner=%d frame=%d target=%d before=%d source=%d after=%d frozen=%d selector=%d beforeErr=%v sourceErr=%v afterErr=%v diffErr=%v sourceDiffErr=%v\n", step.owner, step.frame, step.target, len(beforeSlots), len(sourceSlots), len(afterSlots), span.coordinates.Len(), static.selector.Len(), beforeErr, sourceErr, afterErr, diffErr, sourceDiffErr)
	for index, missing := range sourceMissing {
		hash, _ := region.target.authority.product.CoordinateSlotHash(missing)
		paths, _ := region.target.authority.product.PathCoordinateSupportPaths([]state.CoordinateSlot{missing})
		fmt.Fprintf(os.Stderr, "APPLY_COORD_SOURCE_DIFF index=%d family=%s hash=%016x paths=%v inBoundaryOwned=%t\n", index, missing.Family().ID(), hash, formatFormalCoordinatePaths(region.target.authority.coordinateKeys, paths), coordinateSlotPresent(region.target.authority.product, static.sourceOwned, missing))
	}
	if found {
		hash, _ := region.caller.authority.product.CoordinateSlotHash(escaped)
		paths, _ := region.caller.authority.product.PathCoordinateSupportPaths([]state.CoordinateSlot{escaped})
		inImage, _ := static.image.Contains(region.caller.authority.product, escaped)
		inBody, _ := span.coordinates.Contains(region.caller.authority.product, escaped)
		inCallerBefore := coordinateSlotPresent(region.caller.authority.product, beforeSlots, escaped)
		fmt.Fprintf(os.Stderr, "APPLY_COORD_DIFF family=%s hash=%016x paths=%v inFrameImage=%t inBody=%t inCallerBefore=%t\n", escaped.Family().ID(), hash, formatFormalCoordinatePaths(span.keys, paths), inImage, inBody, inCallerBefore)
		for index, candidate := range sourceSlots {
			candidateHash, _ := region.target.authority.product.CoordinateSlotHash(candidate)
			candidatePaths, _ := region.target.authority.product.PathCoordinateSupportPaths([]state.CoordinateSlot{candidate})
			fmt.Fprintf(os.Stderr, "APPLY_COORD_SOURCE index=%d family=%s hash=%016x paths=%v\n", index, candidate.Family().ID(), candidateHash, formatFormalCoordinatePaths(region.target.authority.coordinateKeys, candidatePaths))
		}
		for index, cell := range static.cells {
			present, _ := cell.Contains(region.caller.authority.product, escaped)
			fmt.Fprintf(os.Stderr, "APPLY_COORD_CELL index=%d size=%d contains=%t\n", index, cell.Len(), present)
			for _, candidate := range cell.Slots() {
				candidatePaths, _ := region.target.authority.product.PathCoordinateSupportPaths([]state.CoordinateSlot{candidate})
				hash, _ := region.target.authority.product.CoordinateSlotHash(candidate)
				fmt.Fprintf(os.Stderr, "APPLY_COORD_CELL_SOURCE cell=%d family=%s hash=%016x paths=%v\n", index, candidate.Family().ID(), hash, formatFormalCoordinatePaths(cell.KeySpace(), candidatePaths))
			}
		}
		for index, target := range static.footprint.Targets {
			equal, _ := region.caller.authority.product.CoordinateSlotEqual(target.Slot, escaped)
			if equal {
				fmt.Fprintf(os.Stderr, "APPLY_COORD_TARGET index=%d required=%d satisfied=%d emitted=%t requiredFibers=%v satisfiedFibers=%v\n", index, target.Required, target.Satisfied, target.Emitted, target.RequiredFibers, target.SatisfiedFibers)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "APPLY_COORD_STATIC roots=%v sources=%v sourceSeen=%d destinationSeen=%d image=%d\n", static.footprint.RootMap, static.footprint.SourceRoots, static.footprint.SourceSeen.Len(), static.footprint.DestinationSeen.Len(), static.footprint.Image.Len())
	fmt.Fprintf(os.Stderr, "APPLY_COORD_RUNTIME roots=%v\n", runtimeRootMap)
	for index, edge := range step.linked.boundary.edges {
		fmt.Fprintf(os.Stderr, "APPLY_COORD_EDGE index=%d kind=%d source=%v destination=%d target=%v\n", index, edge.kind, edge.source, edge.destination, step.linked.boundary.destinations[edge.destination])
	}
}

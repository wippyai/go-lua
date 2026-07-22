package transformer

import "time"

// FreezePhase identifies one architectural phase of relation-program freeze.
type FreezePhase uint8

const (
	FreezePhaseInputValidation FreezePhase = iota
	FreezePhaseLocalSyntax
	FreezePhaseSCCClosureLinking
	FreezePhaseRegionWTO
	FreezePhaseCoordinateClosure
	FreezePhasePathDependencyPlanning
	FreezePhaseFiberLayout
	FreezePhaseObservableQuotient
	FreezePhaseTemplateBinding
	// FreezePhaseObservationContract brackets demand canonicalization before
	// tier-3 construction.
	FreezePhaseObservationContract
	// FreezePhaseDependencyFreeze brackets tier-3 full-result-v1 construction.
	// The component phases remain separately observable for later demand cuts.
	FreezePhaseDependencyFreeze
)

// FreezePhaseStats is caller-owned observational state for one freeze phase.
type FreezePhaseStats struct {
	Calls   int
	Elapsed time.Duration
}

// FreezeTelemetry is optional caller-owned diagnostic state. It is never
// retained by a RelationProgram and therefore cannot become artifact state or
// participate in structural identity.
type FreezeTelemetry struct {
	InputValidation        FreezePhaseStats
	LocalSyntax            FreezePhaseStats
	SCCClosureLinking      FreezePhaseStats
	RegionWTO              FreezePhaseStats
	CoordinateClosure      FreezePhaseStats
	PathDependencyPlanning FreezePhaseStats
	FiberLayout            FreezePhaseStats
	ObservableQuotient     FreezePhaseStats
	TemplateBinding        FreezePhaseStats
	ObservationContract    FreezePhaseStats
	DependencyFreeze       FreezePhaseStats
}

func (t *FreezeTelemetry) phase(phase FreezePhase) *FreezePhaseStats {
	if t == nil {
		return nil
	}
	switch phase {
	case FreezePhaseInputValidation:
		return &t.InputValidation
	case FreezePhaseLocalSyntax:
		return &t.LocalSyntax
	case FreezePhaseSCCClosureLinking:
		return &t.SCCClosureLinking
	case FreezePhaseRegionWTO:
		return &t.RegionWTO
	case FreezePhaseCoordinateClosure:
		return &t.CoordinateClosure
	case FreezePhasePathDependencyPlanning:
		return &t.PathDependencyPlanning
	case FreezePhaseFiberLayout:
		return &t.FiberLayout
	case FreezePhaseObservableQuotient:
		return &t.ObservableQuotient
	case FreezePhaseTemplateBinding:
		return &t.TemplateBinding
	case FreezePhaseObservationContract:
		return &t.ObservationContract
	case FreezePhaseDependencyFreeze:
		return &t.DependencyFreeze
	default:
		return nil
	}
}

func (t *FreezeTelemetry) begin(phase FreezePhase) time.Time {
	if t == nil {
		return time.Time{}
	}
	return time.Now()
}

func (t *FreezeTelemetry) end(phase FreezePhase, started time.Time) {
	if started.IsZero() {
		return
	}
	if stats := t.phase(phase); stats != nil {
		stats.Calls++
		stats.Elapsed += time.Since(started)
	}
}

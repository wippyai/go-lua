package state

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
)

// TransferInputAccess is one input role's exact observable State footprint.
// Input order is owned by the frozen transfer equation; this carrier does not
// invent a second role vocabulary.
type TransferInputAccess struct {
	Values                   []statekey.Value
	Lanes                    LaneSet
	TypestateResourceQueries []TypestateResourceQuery
	Diagnostics              bool
	Reachable                bool
}

// TransferAccessConfig is the construction-only form of TransferAccess.
// ProviderInputs are exactly the components observable by semantic provider
// code. LaneCarryReads is separate transaction authority: it names the lanes
// read from LaneCarry solely to preserve or mutate them during commit. Negative
// carry indexes mean that the corresponding component is written by the
// transfer; otherwise it is retained structurally from that exact input.
type TransferAccessConfig struct {
	ProviderInputs []TransferInputAccess

	ValueWrites    []statekey.Value
	LaneCarryReads LaneSet
	LaneWrites     LaneSet

	ValueCarry      int
	LaneCarry       int
	DiagnosticCarry int
	ReachableCarry  int

	DiagnosticWrites bool
	ReachableWrites  bool
}

type transferAccessSeal struct{}

// TransferAccess is the provider-owned, immutable State observation and write
// authority for one transfer equation. An evaluator may receive only the
// components named here; the patch transaction rejects every undeclared write.
type TransferAccess struct {
	providerInputs []TransferInputAccess

	valueWrites    []statekey.Value
	laneCarryReads LaneSet
	laneWrites     LaneSet

	valueCarry, laneCarry, diagnosticCarry, reachableCarry int
	diagnosticWrites, reachableWrites                      bool
	seal                                                   *transferAccessSeal
}

// SealTransferAccess validates, sorts, deduplicates and detaches one access
// contract. There is no whole-State form: every observable slot and lane must
// be named by the semantic provider or transaction that owns it.
func SealTransferAccess(domain ProductDomain, config TransferAccessConfig) (TransferAccess, error) {
	if !domain.Valid() || len(config.ProviderInputs) == 0 {
		return TransferAccess{}, fmt.Errorf("state: transfer access has no inputs")
	}
	validCarry := func(carry int) bool { return carry >= -1 && carry < len(config.ProviderInputs) }
	if !validCarry(config.ValueCarry) || !validCarry(config.LaneCarry) ||
		!validCarry(config.DiagnosticCarry) || !validCarry(config.ReachableCarry) {
		return TransferAccess{}, fmt.Errorf("state: transfer access has an invalid carry role")
	}
	if config.DiagnosticWrites == (config.DiagnosticCarry >= 0) || config.ReachableWrites == (config.ReachableCarry >= 0) {
		return TransferAccess{}, fmt.Errorf("state: transfer access must either write or carry diagnostics and reachability")
	}
	valueWrites, err := canonicalTransferSlots(config.ValueWrites)
	if err != nil {
		return TransferAccess{}, err
	}
	enabled := domain.Lanes()
	if !transferLanesRegistered(enabled, config.LaneCarryReads) {
		return TransferAccess{}, fmt.Errorf("state: transfer access carries an unregistered lane")
	}
	if !transferLanesRegistered(enabled, config.LaneWrites) {
		return TransferAccess{}, fmt.Errorf("state: transfer access writes an unregistered lane")
	}
	if config.LaneCarry < 0 && config.LaneCarryReads.Len() != 0 {
		return TransferAccess{}, fmt.Errorf("state: transfer access reads lanes without a carry role")
	}
	for _, lane := range config.LaneWrites.IDs() {
		if config.LaneCarry >= 0 && !config.LaneCarryReads.Has(lane) {
			return TransferAccess{}, fmt.Errorf("state: transfer access writes lane %q without transaction read authority", lane)
		}
	}
	out := TransferAccess{
		providerInputs: make([]TransferInputAccess, len(config.ProviderInputs)),
		valueWrites:    valueWrites, laneCarryReads: cloneLaneSet(config.LaneCarryReads),
		laneWrites: cloneLaneSet(config.LaneWrites),
		valueCarry: config.ValueCarry, laneCarry: config.LaneCarry,
		diagnosticCarry: config.DiagnosticCarry, reachableCarry: config.ReachableCarry,
		diagnosticWrites: config.DiagnosticWrites, reachableWrites: config.ReachableWrites,
		seal: &transferAccessSeal{},
	}
	for index, input := range config.ProviderInputs {
		values, err := canonicalTransferSlots(input.Values)
		if err != nil {
			return TransferAccess{}, fmt.Errorf("state: transfer input %d: %w", index, err)
		}
		if !transferLanesRegistered(enabled, input.Lanes) {
			return TransferAccess{}, fmt.Errorf("state: transfer input %d reads an unregistered lane", index)
		}
		queries := append([]TypestateResourceQuery(nil), input.TypestateResourceQueries...)
		sort.Slice(queries, func(i, j int) bool { return queries[i].Less(queries[j]) })
		write := 0
		for _, query := range queries {
			if !query.ValidFor(domain) {
				return TransferAccess{}, fmt.Errorf("state: transfer input %d has a foreign typestate resource query", index)
			}
			if write != 0 && !queries[write-1].Less(query) && !query.Less(queries[write-1]) {
				if queries[write-1].Equal(query) {
					continue
				}
				return TransferAccess{}, fmt.Errorf("state: transfer input %d has comparator-equal foreign typestate queries", index)
			}
			queries[write], write = query, write+1
		}
		queries = queries[:write]
		out.providerInputs[index] = TransferInputAccess{
			Values: values, Lanes: cloneLaneSet(input.Lanes), TypestateResourceQueries: queries,
			Diagnostics: input.Diagnostics, Reachable: input.Reachable,
		}
	}
	return out, nil
}

func canonicalTransferSlots(input []statekey.Value) ([]statekey.Value, error) {
	out := append([]statekey.Value(nil), input...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	write := 0
	for _, slot := range out {
		if slot == 0 {
			return nil, fmt.Errorf("transfer access contains the invalid zero Value slot")
		}
		if write != 0 && out[write-1] == slot {
			continue
		}
		out[write] = slot
		write++
	}
	return out[:write], nil
}

func transferLanesRegistered(enabled, selected LaneSet) bool {
	for _, lane := range selected.IDs() {
		if !enabled.Has(lane) {
			return false
		}
	}
	return true
}

func cloneLaneSet(input LaneSet) LaneSet { return NewLaneSet(input.IDs()...) }

func (a TransferAccess) Valid() bool             { return a.seal != nil && len(a.providerInputs) != 0 }
func (a TransferAccess) ProviderInputCount() int { return len(a.providerInputs) }
func (a TransferAccess) ProviderInput(index int) (TransferInputAccess, bool) {
	if !a.Valid() || index < 0 || index >= len(a.providerInputs) {
		return TransferInputAccess{}, false
	}
	in := a.providerInputs[index]
	in.Values = append([]statekey.Value(nil), in.Values...)
	in.Lanes = cloneLaneSet(in.Lanes)
	in.TypestateResourceQueries = append([]TypestateResourceQuery(nil), in.TypestateResourceQueries...)
	return in, true
}
func (a TransferAccess) ValueWrites() []statekey.Value {
	return append([]statekey.Value(nil), a.valueWrites...)
}
func (a TransferAccess) LaneCarryReads() LaneSet { return cloneLaneSet(a.laneCarryReads) }
func (a TransferAccess) LaneWrites() LaneSet     { return cloneLaneSet(a.laneWrites) }
func (a TransferAccess) ValueCarry() int         { return a.valueCarry }
func (a TransferAccess) LaneCarry() int          { return a.laneCarry }
func (a TransferAccess) DiagnosticCarry() int    { return a.diagnosticCarry }
func (a TransferAccess) ReachableCarry() int     { return a.reachableCarry }
func (a TransferAccess) DiagnosticWrites() bool  { return a.diagnosticWrites }
func (a TransferAccess) ReachableWrites() bool   { return a.reachableWrites }

package verify

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

func verifyGates(request Request, postFiles map[string]SourceFile, postDigest string) (Report, error) {
	requested := map[cutplan.Gate]bool{}
	for _, gate := range request.RequestedGates {
		if !knownGate(gate) {
			return Report{}, fmt.Errorf("unknown requested gate %q", gate)
		}
		if requested[gate] {
			return Report{}, fmt.Errorf("duplicate requested gate %q", gate)
		}
		requested[gate] = true
	}
	dispositions := map[cutplan.Gate]GateDisposition{}
	for _, disposition := range request.Dispositions {
		if !requested[disposition.Gate] {
			return Report{}, fmt.Errorf("disposition supplied for unrequested gate %q", disposition.Gate)
		}
		if _, duplicate := dispositions[disposition.Gate]; duplicate {
			return Report{}, fmt.Errorf("duplicate disposition for gate %q", disposition.Gate)
		}
		dispositions[disposition.Gate] = disposition
	}
	for gate := range requested {
		disposition, exists := dispositions[gate]
		if !exists {
			return Report{}, fmt.Errorf("requested gate %q has no disposition", gate)
		}
		if err := verifyGateDisposition(gate, disposition, request, postFiles); err != nil {
			return Report{}, err
		}
	}
	executed := make([]cutplan.Gate, 0, len(requested))
	for gate := range requested {
		executed = append(executed, gate)
	}
	sort.Slice(executed, func(i, j int) bool { return executed[i] < executed[j] })
	evidence := make([]GateEvidence, 0, len(executed))
	for _, gate := range executed {
		disposition := dispositions[gate]
		digest := postDigest
		if disposition.External != nil {
			digest = disposition.External.Digest
		}
		evidence = append(evidence, GateEvidence{Gate: gate, Digest: digest})
	}
	return Report{Executed: executed, Evidence: evidence}, nil
}

func verifyGateDisposition(gate cutplan.Gate, disposition GateDisposition, request Request, postFiles map[string]SourceFile) error {
	switch gate {
	case cutplan.GateImportDAG:
		if disposition.External != nil {
			return fmt.Errorf("import-dag must use structural verification")
		}
		return nil // Verify performs the one graph check before gate dispatch.
	case cutplan.GateDiagnostics, cutplan.GateResidue:
		if disposition.External == nil || disposition.External.Kind != gate || !disposition.External.Passed || disposition.External.Digest == "" {
			return fmt.Errorf("%s requires successful typed external evidence", gate)
		}
		return nil
	default:
		return fmt.Errorf("unknown gate %q", gate)
	}
}

func knownGate(gate cutplan.Gate) bool {
	switch gate {
	case cutplan.GateDiagnostics, cutplan.GateImportDAG, cutplan.GateResidue:
		return true
	default:
		return false
	}
}

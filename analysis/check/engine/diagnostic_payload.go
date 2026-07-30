package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

const diagnosticPayloadPrefix = "engine/diagnostic/v1/"

type frozenMutationAction uint8

const (
	frozenMutationWrite frozenMutationAction = iota + 1
	frozenMutationCall
)

func frozenMutationFact(key string, action frozenMutationAction, display string) equation.Fact {
	var message string
	switch action {
	case frozenMutationWrite:
		message = fmt.Sprintf("cannot mutate frozen table %q", display)
	case frozenMutationCall:
		message = fmt.Sprintf("cannot call mutator on frozen table %q", display)
	default:
		panic("engine: unknown frozen mutation action")
	}
	return equation.Fact{Key: key, Value: []byte(message)}
}

type optionalAccessAction uint8

const (
	optionalAccessWrite optionalAccessAction = iota + 1
	optionalAccessCall
)

func optionalAccessFact(key string, action optionalAccessAction, display string) equation.Fact {
	var message string
	switch action {
	case optionalAccessWrite:
		message = fmt.Sprintf("cannot assign through optional %s without nil check", display)
	case optionalAccessCall:
		message = fmt.Sprintf("cannot call method on an optional value without a nil check: %s may be nil", display)
	default:
		panic("engine: unknown optional access action")
	}
	return equation.Fact{Key: key, Value: []byte(message)}
}

// DiagnosticFlags are closed semantic properties of a diagnostic. They are
// carried across the equation boundary so presentation never has to infer a
// branch from rendered prose.
type DiagnosticFlags uint32

const (
	DiagnosticMayBeNil DiagnosticFlags = 1 << iota
	DiagnosticMapReadMissing
	DiagnosticAnyBoundary
	DiagnosticHasTransition
)

const (
	diagnosticAssignmentMismatch    = "assignment_mismatch"
	diagnosticAssignmentBoundary    = "assignment_boundary"
	diagnosticFunctionWriteMismatch = "function_write_mismatch"
	diagnosticReturnContract        = "return_contract"
	diagnosticCallArgument          = "call_argument"
	diagnosticCallGenericConflict   = "call_generic_conflict"
	diagnosticCallNotCallable       = "call_not_callable"
	diagnosticCallArity             = "call_arity"
	diagnosticMemberMissing         = "member_missing"
	diagnosticChannelLifecycle      = "channel_lifecycle"
	diagnosticTypestateRequirement  = "typestate_requirement"
	diagnosticTypestateTransition   = "typestate_transition"
	diagnosticTypestateUnproven     = "typestate_unproven"
	diagnosticResourceUnreleased    = "resource_unreleased"
	diagnosticClaimUnproven         = "claim_unproven"
	diagnosticSendIsolation         = "send_isolation"
)

// DiagnosticConflict is the structured generic-binding contradiction proved
// by a call kernel.
type DiagnosticConflict struct {
	Parameter  string `json:"parameter"`
	Bound      string `json:"bound"`
	BoundAt    string `json:"bound_at"`
	Demanded   string `json:"demanded"`
	DemandedAt string `json:"demanded_at"`
}

// DiagnosticPayload is the semantic row carried by a diagnostic fact. Source
// and Subject are authored/display identities; Observed and Required are the
// two closed type/value terms used by contract diagnostics.
type DiagnosticPayload struct {
	Version  uint8               `json:"version"`
	Kind     string              `json:"kind"`
	Name     string              `json:"name,omitempty"`
	Source   string              `json:"source,omitempty"`
	Subject  string              `json:"subject,omitempty"`
	Observed string              `json:"observed,omitempty"`
	Required string              `json:"required,omitempty"`
	Expected int                 `json:"expected,omitempty"`
	Actual   int                 `json:"actual,omitempty"`
	Flags    DiagnosticFlags     `json:"flags,omitempty"`
	Conflict *DiagnosticConflict `json:"conflict,omitempty"`
}

func encodeDiagnosticPayload(payload DiagnosticPayload) ([]byte, error) {
	payload.Version = 1
	if err := validateDiagnosticPayload(payload); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("engine: encode diagnostic payload: %w", err)
	}
	return append([]byte(diagnosticPayloadPrefix), encoded...), nil
}

func decodeDiagnosticPayload(value []byte) (DiagnosticPayload, bool) {
	if !strings.HasPrefix(string(value), diagnosticPayloadPrefix) {
		return DiagnosticPayload{}, false
	}
	var payload DiagnosticPayload
	if err := front.DecodeRequiredWireJSON(value[len(diagnosticPayloadPrefix):], &payload); err != nil || payload.Version != 1 {
		return DiagnosticPayload{}, false
	}
	if err := validateDiagnosticPayload(payload); err != nil {
		return DiagnosticPayload{}, false
	}
	return payload, true
}

func validateDiagnosticPayload(payload DiagnosticPayload) error {
	if payload.Version != 1 || payload.Kind == "" {
		return fmt.Errorf("engine: malformed diagnostic payload")
	}
	switch payload.Kind {
	case diagnosticAssignmentMismatch:
		if payload.Source == "" || payload.Flags&DiagnosticMayBeNil == 0 && (payload.Observed == "" || payload.Required == "") {
			return fmt.Errorf("engine: malformed assignment diagnostic payload")
		}
	case diagnosticAssignmentBoundary:
		if payload.Source == "" {
			return fmt.Errorf("engine: malformed assignment boundary payload")
		}
	case diagnosticFunctionWriteMismatch:
		if payload.Source == "" || payload.Observed == "" || payload.Required == "" {
			return fmt.Errorf("engine: malformed function write diagnostic payload")
		}
	case diagnosticReturnContract:
		if payload.Subject == "" || payload.Required == "" || payload.Flags&DiagnosticAnyBoundary == 0 && payload.Flags&DiagnosticMayBeNil == 0 && payload.Observed == "" {
			return fmt.Errorf("engine: malformed return diagnostic payload")
		}
	case diagnosticCallArgument:
		if payload.Subject == "" || payload.Required == "" || payload.Flags&DiagnosticMayBeNil == 0 && payload.Observed == "" {
			return fmt.Errorf("engine: malformed call argument payload")
		}
	case diagnosticCallGenericConflict:
		if payload.Conflict == nil || payload.Conflict.Parameter == "" || payload.Conflict.Bound == "" || payload.Conflict.BoundAt == "" || payload.Conflict.Demanded == "" || payload.Conflict.DemandedAt == "" {
			return fmt.Errorf("engine: malformed generic conflict payload")
		}
	case diagnosticCallNotCallable:
		if payload.Source == "" || payload.Flags&DiagnosticMayBeNil == 0 && payload.Flags&DiagnosticAnyBoundary == 0 && payload.Observed == "" {
			return fmt.Errorf("engine: malformed non-callable payload")
		}
	case diagnosticCallArity:
		if payload.Source == "" || payload.Expected < 0 || payload.Actual < 0 {
			return fmt.Errorf("engine: malformed call arity payload")
		}
	case diagnosticMemberMissing:
		if payload.Observed == "" || payload.Name == "" {
			return fmt.Errorf("engine: malformed missing-member payload")
		}
	case diagnosticChannelLifecycle:
		if payload.Source == "" || (payload.Name != "send" && payload.Name != "close") {
			return fmt.Errorf("engine: malformed channel lifecycle payload")
		}
	case diagnosticTypestateRequirement, diagnosticTypestateTransition:
		if payload.Source == "" || payload.Required == "" || payload.Observed == "" {
			return fmt.Errorf("engine: malformed typestate payload")
		}
	case diagnosticTypestateUnproven:
		if payload.Source == "" || payload.Required == "" {
			return fmt.Errorf("engine: malformed typestate proof payload")
		}
	case diagnosticResourceUnreleased:
		if payload.Source == "" {
			return fmt.Errorf("engine: malformed unreleased resource payload")
		}
	case diagnosticClaimUnproven:
		if payload.Required == "" {
			return fmt.Errorf("engine: malformed unproven claim payload")
		}
	case diagnosticSendIsolation:
		if payload.Name != "isolated" && payload.Name != "immutable" && payload.Name != "escaped" && payload.Name != "fallback" {
			return fmt.Errorf("engine: malformed send isolation payload")
		}
	default:
		return fmt.Errorf("engine: unknown diagnostic payload kind %q", payload.Kind)
	}
	return nil
}

func diagnosticFact(key string, payload DiagnosticPayload) equation.Fact {
	encoded, err := encodeDiagnosticPayload(payload)
	if err != nil {
		panic(err)
	}
	return equation.Fact{Key: key, Value: encoded}
}

// renderDiagnosticPayload is the only formatter for structured equation
// diagnostics. Producers publish semantic fields and flags only.
func renderDiagnosticPayload(payload DiagnosticPayload) string {
	switch payload.Kind {
	case diagnosticAssignmentMismatch:
		if payload.Flags&DiagnosticMayBeNil != 0 {
			return fmt.Sprintf("cannot assign %s because it may be nil", payload.Source)
		}
		return fmt.Sprintf("cannot assign %s because it is %s, not %s", payload.Source, payload.Observed, payload.Required)
	case diagnosticAssignmentBoundary:
		return fmt.Sprintf("cannot assign %s because %s comes from any/unknown; no proof shows it satisfies the declared type", payload.Source, payload.Source)
	case diagnosticFunctionWriteMismatch:
		return fmt.Sprintf("cannot assign %s because assigned value is %s, not %s", payload.Source, payload.Observed, payload.Required)
	case diagnosticReturnContract:
		switch {
		case payload.Flags&DiagnosticAnyBoundary != 0 && payload.Name == "member":
			return fmt.Sprintf("%s comes from any/unknown; no proof shows it satisfies declared return type %s", payload.Subject, payload.Required)
		case payload.Flags&DiagnosticAnyBoundary != 0:
			return fmt.Sprintf("%s comes from any/unknown; no proof shows it is %s", payload.Subject, payload.Required)
		case payload.Flags&DiagnosticMayBeNil != 0:
			return fmt.Sprintf("%s may be nil, not %s", payload.Subject, payload.Required)
		default:
			return fmt.Sprintf("%s is %s, not %s", payload.Subject, payload.Observed, payload.Required)
		}
	case diagnosticCallArgument:
		if payload.Flags&DiagnosticMayBeNil != 0 {
			return fmt.Sprintf("%s may be nil, not %s", payload.Subject, payload.Required)
		}
		return fmt.Sprintf("%s is %s, not %s", payload.Subject, payload.Observed, payload.Required)
	case diagnosticCallGenericConflict:
		conflict := payload.Conflict
		return fmt.Sprintf("%s requires %s to be %s, but %s requires %s to be %s", conflict.DemandedAt, conflict.Parameter, conflict.Demanded, conflict.BoundAt, conflict.Parameter, conflict.Bound)
	case diagnosticCallNotCallable:
		switch {
		case payload.Flags&DiagnosticMayBeNil != 0:
			return fmt.Sprintf("cannot call %s because it may be nil", payload.Source)
		case payload.Flags&DiagnosticAnyBoundary != 0:
			return fmt.Sprintf("cannot call %s because it comes from any/unknown; no proof shows it is callable", payload.Source)
		default:
			return fmt.Sprintf("%s is %s, not callable", payload.Source, payload.Observed)
		}
	case diagnosticCallArity:
		return fmt.Sprintf("%s expects %d arguments, got %d", payload.Source, payload.Expected, payload.Actual)
	case diagnosticMemberMissing:
		return fmt.Sprintf("%s has no member %q", payload.Observed, payload.Name)
	case diagnosticChannelLifecycle:
		if payload.Name == "close" {
			return fmt.Sprintf("cannot close already closed channel `%s`", payload.Source)
		}
		return fmt.Sprintf("cannot send on closed channel `%s`", payload.Source)
	case diagnosticTypestateRequirement:
		return fmt.Sprintf("invalid typestate requirement for resource `%s` in protocol connection: expected `%s`, found `%s`", payload.Source, payload.Required, payload.Observed)
	case diagnosticTypestateTransition:
		return fmt.Sprintf("invalid transition for resource `%s` in protocol transaction: expected `%s`, found `%s`", payload.Source, payload.Required, payload.Observed)
	case diagnosticTypestateUnproven:
		return fmt.Sprintf("cannot prove typestate requirement for resource `%s`: expected `%s`", payload.Source, payload.Required)
	case diagnosticResourceUnreleased:
		if payload.Flags&DiagnosticHasTransition != 0 {
			return fmt.Sprintf("resource `%s` remains in a non-final connection state at function exit; expected `closed`", payload.Source)
		}
		return fmt.Sprintf("resource `%s` remains in connection state `open` at function exit; expected `closed`", payload.Source)
	case diagnosticClaimUnproven:
		return fmt.Sprintf("claim %s is not proven", payload.Required)
	case diagnosticSendIsolation:
		switch payload.Name {
		case "isolated":
			return "send payload is proven isolated for zero-copy transfer"
		case "immutable":
			return "send payload is proven immutable for zero-copy sharing"
		case "escaped":
			return "send payload has a proven escaping alias; zero-copy transfer is rejected"
		default:
			return "send payload is not proven isolated or immutable; runtime will copy"
		}
	default:
		return ""
	}
}

func diagnosticMessage(value []byte) (string, DiagnosticPayload) {
	payload, ok := decodeDiagnosticPayload(value)
	if !ok {
		return string(value), DiagnosticPayload{}
	}
	return renderDiagnosticPayload(payload), payload
}

func publicDiagnosticFacts(facts []equation.Fact) []equation.Fact {
	out := cloneFacts(facts)
	for index := range out {
		message, payload := diagnosticMessage(out[index].Value)
		if payload.Kind != "" {
			out[index].Value = []byte(message)
		}
	}
	return out
}

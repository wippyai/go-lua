package wire

import (
	"encoding/json"
	"fmt"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/types/signature/wire"
)

type typestateProtocolWire struct {
	Name        string                    `json:"name"`
	States      []string                  `json:"states"`
	FinalStates []string                  `json:"finalStates,omitempty"`
	Transitions []typestateTransitionWire `json:"transitions,omitempty"`
}

type typestateTransitionWire struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func encodeTypestateProtocol(def typestate.Definition) (typestateProtocolWire, error) {
	if err := def.Validate(); err != nil {
		return typestateProtocolWire{}, err
	}
	normalized := def.Normalized()
	out := typestateProtocolWire{
		Name:        normalized.Protocol.String(),
		States:      wire.EncodeTypestateStates(normalized.States),
		FinalStates: wire.EncodeTypestateStates(normalized.FinalStates),
		Transitions: make([]typestateTransitionWire, 0, len(normalized.Transitions)),
	}
	for _, transition := range normalized.Transitions {
		out.Transitions = append(out.Transitions, typestateTransitionWire{
			From: transition.From.String(),
			To:   transition.To.String(),
		})
	}
	return out, nil
}

// CanonicalTypestateDefinitionBytes encodes a typestate definition through the
// manifest wire codec. The returned JSON is stable across declaration order
// and is suitable for content digests.
func CanonicalTypestateDefinitionBytes(def typestate.Definition) ([]byte, error) {
	w, err := encodeTypestateProtocol(def)
	if err != nil {
		return nil, err
	}
	return json.Marshal(w)
}

func decodeTypestateProtocol(w typestateProtocolWire) (typestate.Definition, error) {
	protocol, ok := typestate.ProtocolFromString(w.Name)
	if !ok {
		return typestate.Definition{}, fmt.Errorf("missing protocol name")
	}
	states, err := wire.DecodeTypestateStates(w.States, "state")
	if err != nil {
		return typestate.Definition{}, err
	}
	finals, err := wire.DecodeTypestateStates(w.FinalStates, "final state")
	if err != nil {
		return typestate.Definition{}, err
	}
	transitions := make([]typestate.TransitionDecl, 0, len(w.Transitions))
	for _, transition := range w.Transitions {
		from, ok := typestate.StateFromString(transition.From)
		if !ok {
			return typestate.Definition{}, fmt.Errorf("transition missing source state")
		}
		to, ok := typestate.StateFromString(transition.To)
		if !ok {
			return typestate.Definition{}, fmt.Errorf("transition missing target state")
		}
		transitions = append(transitions, typestate.TransitionDecl{From: from, To: to})
	}
	return typestate.Definition{
		Protocol:    protocol,
		States:      states,
		FinalStates: finals,
		Transitions: transitions,
	}, nil
}

func validateManifestTypestateUsage(m *Manifest) error {
	if m == nil {
		return nil
	}
	defs := make(map[typestate.Protocol]typestate.Definition, len(m.TypestateProtocols))
	for protocol, def := range m.TypestateProtocols {
		if def.Protocol != protocol {
			return fmt.Errorf("manifest: typestate protocol key %q does not match declaration %q", protocol, def.Protocol)
		}
		if err := def.Validate(); err != nil {
			return fmt.Errorf("manifest: typestate protocol %q: %w", protocol, err)
		}
		defs[protocol] = def.Normalized()
	}
	for name, sig := range m.FunctionSignatures {
		if err := validateEffectRowTypestateUsage(defs, sig.Effect); err != nil {
			return fmt.Errorf("manifest: function signature %q effect: %w", name, err)
		}
	}
	return nil
}

// validateEffectRowTypestateUsage walks the lifecycle labels of one manifest
// row, checks the row's own well-formedness and protocol declaration, and
// defers the conformance relation to the declared state machine.
func validateEffectRowTypestateUsage(defs map[typestate.Protocol]typestate.Definition, row effect.Row) error {
	for _, label := range row.Labels {
		switch l := effect.NormalizeLabel(label).(type) {
		case lifecycle.Acquire:
			if l.State == "" {
				return fmt.Errorf("lifecycle acquire missing state")
			}
			def, err := declaredTypestateProtocol(defs, l.Protocol)
			if err != nil {
				return err
			}
			if err := def.AdmitsAcquire(l.State, l.Obligation); err != nil {
				return err
			}
		case lifecycle.Transition:
			if l.From == "" {
				return fmt.Errorf("lifecycle transition missing source state")
			}
			if l.To == "" {
				return fmt.Errorf("lifecycle transition missing target state")
			}
			def, err := declaredTypestateProtocol(defs, l.Protocol)
			if err != nil {
				return err
			}
			if err := def.AdmitsTransition(l.From, l.To); err != nil {
				return err
			}
		case lifecycle.Escape:
			if _, err := declaredTypestateProtocol(defs, l.Protocol); err != nil {
				return err
			}
		}
	}
	return nil
}

func declaredTypestateProtocol(defs map[typestate.Protocol]typestate.Definition, protocol typestate.Protocol) (typestate.Definition, error) {
	if protocol == "" {
		return typestate.Definition{}, fmt.Errorf("missing protocol")
	}
	def, ok := defs[protocol]
	if !ok {
		return typestate.Definition{}, fmt.Errorf("lifecycle protocol %q is not declared as a typestate FSM", protocol)
	}
	return def, nil
}

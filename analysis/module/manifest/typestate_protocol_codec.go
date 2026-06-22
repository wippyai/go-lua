package manifest

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/module/signature"
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
		States:      encodeTypestateStates(normalized.States),
		FinalStates: encodeTypestateStates(normalized.FinalStates),
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

func decodeTypestateProtocol(w typestateProtocolWire) (typestate.Definition, error) {
	protocol, ok := typestate.ProtocolFromString(w.Name)
	if !ok {
		return typestate.Definition{}, fmt.Errorf("missing protocol name")
	}
	states, err := decodeTypestateStates(w.States, "state")
	if err != nil {
		return typestate.Definition{}, err
	}
	finals, err := decodeTypestateStates(w.FinalStates, "final state")
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

func encodeTypestateStates(states []typestate.State) []string {
	if len(states) == 0 {
		return nil
	}
	out := make([]string, 0, len(states))
	for _, state := range states {
		out = append(out, state.String())
	}
	sort.Strings(out)
	return out
}

func decodeTypestateStates(raw []string, role string) ([]typestate.State, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]typestate.State, 0, len(raw))
	for _, name := range raw {
		state, ok := typestate.StateFromString(name)
		if !ok {
			return nil, fmt.Errorf("empty %s", role)
		}
		out = append(out, state)
	}
	return out, nil
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
		if sig.OperationalEffects != nil {
			if err := validateOperationalTypestateUsage(defs, *sig.OperationalEffects); err != nil {
				return fmt.Errorf("manifest: function signature %q operational effects: %w", name, err)
			}
		}
	}
	return nil
}

func validateEffectRowTypestateUsage(defs map[typestate.Protocol]typestate.Definition, row effect.Row) error {
	for _, label := range row.Labels {
		switch l := effect.NormalizeLabel(label).(type) {
		case lifecycle.Acquire:
			if err := validateLifecycleAcquire(defs, l.Protocol, l.State, l.Obligation.Final); err != nil {
				return err
			}
		case lifecycle.Transition:
			if err := validateLifecycleTransition(defs, l.Protocol, l.From, l.To); err != nil {
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

func validateOperationalTypestateUsage(defs map[typestate.Protocol]typestate.Definition, effects signature.OperationalEffects) error {
	for _, fact := range effects.LifecycleEffects {
		switch fact.Kind {
		case signature.LifecycleAcquire:
			if err := validateLifecycleAcquire(defs, fact.Protocol, fact.To, fact.Obligation.Final); err != nil {
				return err
			}
		case signature.LifecycleTransition:
			if err := validateLifecycleTransition(defs, fact.Protocol, fact.From, fact.To); err != nil {
				return err
			}
		case signature.LifecycleEscape:
			if _, err := declaredTypestateProtocol(defs, fact.Protocol); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported lifecycle kind %d", fact.Kind)
		}
	}
	return nil
}

func validateLifecycleAcquire(defs map[typestate.Protocol]typestate.Definition, protocol typestate.Protocol, state, final typestate.State) error {
	if state == "" {
		return fmt.Errorf("lifecycle acquire missing state")
	}
	def, err := declaredTypestateProtocol(defs, protocol)
	if err != nil {
		return err
	}
	if !def.HasState(state) {
		return fmt.Errorf("protocol %q does not declare acquire state %q", protocol, state)
	}
	if final != "" && !def.IsFinal(final) {
		return fmt.Errorf("protocol %q does not declare obligation final state %q", protocol, final)
	}
	return nil
}

func validateLifecycleTransition(defs map[typestate.Protocol]typestate.Definition, protocol typestate.Protocol, from, to typestate.State) error {
	if from == "" {
		return fmt.Errorf("lifecycle transition missing source state")
	}
	if to == "" {
		return fmt.Errorf("lifecycle transition missing target state")
	}
	def, err := declaredTypestateProtocol(defs, protocol)
	if err != nil {
		return err
	}
	if !def.HasState(to) {
		return fmt.Errorf("protocol %q does not declare transition target state %q", protocol, to)
	}
	if !def.HasState(from) {
		return fmt.Errorf("protocol %q does not declare transition source state %q", protocol, from)
	}
	if !def.AllowsTransition(from, to) {
		return fmt.Errorf("protocol %q does not declare transition %q -> %q", protocol, from, to)
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

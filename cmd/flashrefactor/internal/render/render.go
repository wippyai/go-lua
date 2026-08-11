package render

import (
	"fmt"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

// Renderer is an immutable pure compiler. Its handler registry is private and
// constructed from the finite built-in set, so callers cannot install a
// dynamic handler with ambient filesystem or executor capability.
type Renderer struct{ registry handlerRegistry }

// New returns the canonical immutable renderer.
func New() Renderer { return Renderer{registry: builtinRegistry()} }

// Render translates one reviewed cut into its complete virtual post-state.
// It never writes to disk, starts a process, creates a lock, or invokes a
// verifier. A caller receives no partial output when any handler rejects.
func (renderer Renderer) Render(input Input) (Output, error) {
	if err := cutplan.ValidateIntent(input.Intent); err != nil {
		return Output{}, err
	}
	intent, err := cutplan.CanonicalIntent(input.Intent)
	if err != nil {
		return Output{}, err
	}
	requirements, err := indexRequirements(intent)
	if err != nil {
		return Output{}, err
	}
	state, err := newRenderState(input.Snapshot.Workspace, cutplan.WritePaths(intent), input.Registry)
	if err != nil {
		return Output{}, err
	}
	registry := renderer.registry
	if len(registry.edits) == 0 && len(registry.relocations) == 0 {
		registry = builtinRegistry()
	}
	compiler := &compiler{
		state: state, requirements: requirements, registry: registry,
		consumed: make(map[string]string), emitted: make(map[string]struct{}), snapshot: input.Snapshot,
	}
	if err := compiler.preflight(intent); err != nil {
		return Output{}, err
	}
	if err := compiler.rewrite(intent); err != nil {
		return Output{}, err
	}
	if err := compiler.materializeWitnesses(); err != nil {
		return Output{}, err
	}
	return state.output()
}

// Compile is a concise convenience for the canonical renderer. It creates no
// mutable global state and is equivalent to New().Render(input).
func Compile(input Input) (Output, error) { return New().Render(input) }

func (compiler *compiler) preflight(intent cutplan.Intent) error {
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			handler, err := compiler.registry.edit(edit)
			if err != nil {
				return fmt.Errorf("operation %s: %w", operation.ID, err)
			}
			sites, err := handler.preflight(compiler, operation, edit)
			if err != nil {
				return fmt.Errorf("operation %s handler %s: %w", operation.ID, handler.name(), err)
			}
			if err := compiler.claim(operation.ID, sites); err != nil {
				return err
			}
		}
		if err := compiler.preflightRoutes(operation); err != nil {
			return fmt.Errorf("operation %s routes: %w", operation.ID, err)
		}
		if err := compiler.state.collectHazards(operation.Footprint.Read); err != nil {
			return fmt.Errorf("operation %s hazards: %w", operation.ID, err)
		}
	}
	return compiler.captureWitnesses(intent)
}

func (compiler *compiler) preflightRoutes(operation cutplan.Operation) error {
	byConsumer := map[string]routeSet{}
	for _, binding := range operation.Bindings {
		entry := byConsumer[binding.Consumer]
		entry.bindings = append(entry.bindings, binding)
		byConsumer[binding.Consumer] = entry
	}
	for _, route := range operation.Imports {
		entry := byConsumer[route.Consumer]
		entry.imports = append(entry.imports, route)
		byConsumer[route.Consumer] = entry
	}
	for consumer, routes := range byConsumer {
		file, _, _, err := compiler.state.existingFile(consumer)
		if err != nil {
			return err
		}
		if _, err := compiler.state.routePlanForFile(compiler.requirements, file, routes); err != nil {
			return err
		}
	}
	return nil
}

func (compiler *compiler) rewrite(intent cutplan.Intent) error {
	for _, operation := range intent.Operations {
		for _, edit := range operation.Edits {
			handler, err := compiler.registry.edit(edit)
			if err != nil {
				return err
			}
			if err := handler.rewrite(compiler, operation, edit); err != nil {
				return fmt.Errorf("operation %s handler %s: %w", operation.ID, handler.name(), err)
			}
			if err := compiler.emit(handler.provenance(operation, edit)); err != nil {
				return err
			}
		}
		if err := compiler.state.applyRoutes(compiler.requirements, operation); err != nil {
			return fmt.Errorf("operation %s routes: %w", operation.ID, err)
		}
		if err := compiler.emitRoutes(operation); err != nil {
			return err
		}
	}
	return nil
}

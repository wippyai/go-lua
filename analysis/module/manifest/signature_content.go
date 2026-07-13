package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SignatureContentSchema versions the canonical semantic projection used by
// signature content identity. It is intentionally independent of the manifest
// document schema: changing either projection requires an explicit version
// decision instead of silently re-keying caches.
const SignatureContentSchema = "go-lua.signature.content/v1"

type signatureContentWire struct {
	Schema                string                  `json:"schema"`
	HasType               bool                    `json:"hasType"`
	Type                  *typeWire               `json:"type,omitempty"`
	Effect                *effectRowWire          `json:"effect,omitempty"`
	HasOperationalEffects bool                    `json:"hasOperationalEffects"`
	OperationalEffects    *operationalEffectsWire `json:"operationalEffects,omitempty"`
}

type semanticOperationalTypeContextKey struct{}

func semanticOperationalTypeContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, semanticOperationalTypeContextKey{}, true)
}

func encodeOperationalEffectType(ctx context.Context, value typ.Type) (*typeWire, error) {
	if ctx == nil {
		return encodeType(value)
	}
	semantic, _ := ctx.Value(semanticOperationalTypeContextKey{}).(bool)
	if !semantic {
		return encodeType(value)
	}
	if err := validateCanonicalSignatureTypeGraph(ctx, value); err != nil {
		return nil, err
	}
	return encodeSemanticType(value)
}

// CanonicalFunctionSignatureBytesContext returns the canonical, collision-safe
// semantic projection of sig. The presence bits are significant because
// Function.Equals distinguishes nil from a present empty operational row.
// Embedded operational types use equality identity so recursive in-memory
// graphs do not leak allocation identity into the result.
func CanonicalFunctionSignatureBytesContext(ctx context.Context, sig signature.Function) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := operationalEffectsContextErr(ctx); err != nil {
		return nil, err
	}
	var encodedType *typeWire
	if sig.Type != nil {
		if err := validateCanonicalSignatureTypeGraph(ctx, sig.Type); err != nil {
			return nil, err
		}
		var err error
		encodedType, err = encodeSemanticType(sig.Type)
		if err != nil {
			return nil, err
		}
	}
	if err := operationalEffectsContextErr(ctx); err != nil {
		return nil, err
	}
	encodedEffect, err := encodeEffectRow(sig.Effect)
	if err != nil {
		return nil, err
	}
	encodedOperational, err := encodeSignatureContentOperationalEffects(semanticOperationalTypeContext(ctx), sig.OperationalEffects)
	if err != nil {
		return nil, err
	}
	return json.Marshal(signatureContentWire{
		Schema:                SignatureContentSchema,
		HasType:               sig.Type != nil,
		Type:                  encodedType,
		Effect:                encodedEffect,
		HasOperationalEffects: sig.OperationalEffects != nil,
		OperationalEffects:    encodedOperational,
	})
}

type signatureTypeVisitKey struct {
	typeName reflect.Type
	pointer  uintptr
}

// validateCanonicalSignatureTypeGraph rejects structural pointer cycles that
// are not represented by typ.Recursive binders. The manifest type codec owns
// stable recursive binders, but arbitrary object cycles have no canonical wire
// meaning and historically recurse until stack exhaustion. Content sealing must
// turn that condition into ordinary fail-closed unavailability instead.
func validateCanonicalSignatureTypeGraph(ctx context.Context, root typ.Type) error {
	active := make(map[signatureTypeVisitKey]bool)
	complete := make(map[signatureTypeVisitKey]bool)
	var visit func(typ.Type) error
	visit = func(current typ.Type) error {
		if current == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		value := reflect.ValueOf(current)
		if value.Kind() != reflect.Pointer || value.IsNil() {
			return nil
		}
		key := signatureTypeVisitKey{typeName: value.Type(), pointer: value.Pointer()}
		if active[key] {
			if _, recursiveBinder := current.(*typ.Recursive); recursiveBinder {
				return nil
			}
			return fmt.Errorf("manifest: signature type contains unbound structural cycle at %T", current)
		}
		if complete[key] {
			return nil
		}
		active[key] = true
		var childErr error
		typ.WalkChildren(current, func(child typ.Type) bool {
			childErr = visit(child)
			return childErr != nil
		})
		delete(active, key)
		if childErr != nil {
			return childErr
		}
		complete[key] = true
		return nil
	}
	return visit(root)
}

// encodeSignatureContentOperationalEffects deliberately does not apply
// OperationalEffects.IsEmpty. The manifest document may omit a certification
// rider that has no accompanying fact, but content identity must preserve it:
// OperationalEffects.Equals observes every registered lane.
func encodeSignatureContentOperationalEffects(ctx context.Context, effects *signature.OperationalEffects) (*operationalEffectsWire, error) {
	if effects == nil {
		return nil, nil
	}
	out := &operationalEffectsWire{}
	for _, lane := range operationalEffectsWireLanes {
		if err := operationalEffectsContextErr(ctx); err != nil {
			return nil, err
		}
		if err := lane.encode(ctx, effects, out); err != nil {
			return nil, err
		}
	}
	if err := canonicalizeOperationalEffectsWireContext(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// CanonicalAllocationTemplatesBytesContext returns the canonical projection of
// the separately composable allocation-template lane.
func CanonicalAllocationTemplatesBytesContext(ctx context.Context, templates []signature.ReturnAllocationTemplate) ([]byte, error) {
	effects := &signature.OperationalEffects{ReturnAllocationTemplates: templates}
	wire, err := encodeSignatureContentOperationalEffects(semanticOperationalTypeContext(ctx), effects)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wire.ReturnAllocationTemplates)
}

// CanonicalFunctionSignatureBytes is the non-cancelable convenience form.
func CanonicalFunctionSignatureBytes(sig signature.Function) ([]byte, error) {
	return CanonicalFunctionSignatureBytesContext(context.Background(), sig)
}

package wire

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// SignatureContentSchema versions the canonical semantic projection used by
// signature content identity. It is intentionally independent of the manifest
// document schema: changing either projection requires an explicit version
// decision instead of silently re-keying caches.
const SignatureContentSchema = "go-lua.signature.content/v2"

type signatureContentWire struct {
	Schema  string         `json:"schema"`
	HasType bool           `json:"hasType"`
	Type    *TypeWire      `json:"type,omitempty"`
	Effect  *effectRowWire `json:"effect,omitempty"`
}

// CanonicalFunctionSignatureBytesContext returns the canonical, collision-safe
// semantic projection of sig. Embedded types use equality identity so
// recursive in-memory graphs do not leak allocation identity into the result.
func CanonicalFunctionSignatureBytesContext(ctx context.Context, sig signature.Function) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var encodedType *TypeWire
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encodedEffect, err := encodeEffectRow(sig.Effect)
	if err != nil {
		return nil, err
	}
	return json.Marshal(signatureContentWire{
		Schema:  SignatureContentSchema,
		HasType: sig.Type != nil,
		Type:    encodedType,
		Effect:  encodedEffect,
	})
}

type signatureTypeVisitKey struct {
	typeName reflect.Type
	pointer  uintptr
}

// validateCanonicalSignatureTypeGraph rejects structural pointer cycles that
// are not represented by typ.Recursive binders. This package's type codec owns
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
			return fmt.Errorf("signature/wire: signature type contains unbound structural cycle at %T", current)
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

// CanonicalFunctionSignatureBytes is the non-cancelable convenience form.
func CanonicalFunctionSignatureBytes(sig signature.Function) ([]byte, error) {
	return CanonicalFunctionSignatureBytesContext(context.Background(), sig)
}

package lua

import (
	"fmt"

	"github.com/wippyai/go-lua/types/typ"
)

type runtimeArgumentContract struct {
	params   []*LType
	names    []string
	variadic *LType
}

// CheckArguments validates an external call's arguments using the same semantics
// as Type:is. It does not change normal VM calls or infer contracts from exports.
// A malformed contract fails closed. Metadata must be immutable after first use.
func (fp *FunctionProto) CheckArguments(l *LState, args []LValue) error {
	if fp == nil || len(fp.ArgumentInfo) == 0 {
		return nil
	}
	fp.argumentOnce.Do(func() {
		manifest := safeDecodeManifest(fp.ArgumentInfo)
		if manifest == nil {
			fp.argumentError = "invalid argument contract"
			return
		}
		fn, ok := manifest.Export.(*typ.Function)
		if !ok || len(fn.Params) != int(fp.NumParameters) {
			fp.argumentError = "invalid argument contract signature"
			return
		}
		resolver := &typeResolver{path: manifest.Path, types: manifest.Types}
		contract := &runtimeArgumentContract{params: make([]*LType, len(fn.Params)), names: make([]string, len(fn.Params))}
		for i, p := range fn.Params {
			contract.params[i] = newRuntimeTypeValue(p.Type, "", resolver)
			contract.names[i] = p.Name
		}
		if fn.Variadic != nil {
			contract.variadic = newRuntimeTypeValue(fn.Variadic, "", resolver)
		}
		fp.argumentContract = contract
	})
	if fp.argumentError != "" {
		return NewError(fp.argumentError).WithKind(Invalid).WithRetryable(false)
	}
	c := fp.argumentContract
	for i, t := range c.params {
		v := LValue(LNil)
		if i < len(args) {
			v = args[i]
		}
		if err := checkArgument(l, t, v, i, c.names[i]); err != nil {
			return err
		}
	}
	if c.variadic != nil {
		for i := len(c.params); i < len(args); i++ {
			if err := checkArgument(l, c.variadic, args[i], i, "..."); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkArgument(l *LState, t *LType, v LValue, index int, name string) error {
	if v == nil {
		v = LNil
	}
	if t.Validate(l, v) {
		return nil
	}
	path := fmt.Sprintf("args[%d]", index+1)
	_, detail := validateWithError(v, t.inner, t.resolver, path)
	if detail == nil {
		detail = newTypeError(path, t.String(), luaTypeName(v))
	}
	err := toValidationLuaError(l, detail)
	err.details["argument"] = index + 1
	err.details["parameter"] = name
	return err.WithRetryable(false)
}

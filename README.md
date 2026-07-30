# go-lua

Heavily modified fork of [gopher-lua](https://github.com/yuin/gopher-lua).

The former checker and typed runtime/compiler path have been quarantined while
the canonical Program and formal engine are built. The live tree currently
contains only the analysis-independent Lua runtime, parser, AST, source
utilities, bytecode cache, inspector, and pattern matcher. Typed syntax is
parsed but rejected explicitly by the temporary compiler; there is no live
type-checking API.

```bash
go get github.com/wippyai/go-lua
```

## Running the VM

```go
L := lua.NewState()
defer L.Close()

if err := L.DoString(`print("hello")`); err != nil {
    panic(err)
}
```

## Error Metadata

`WrapError` and `WrapErrorWithLua` preserve metadata from wrapped `*lua.Error` values.
For non-`*lua.Error` chains (for example errors from another package), register a process-wide metadata extractor once:

```go
import (
    "errors"

    lua "github.com/wippyai/go-lua"
)

func init() {
    lua.ConfigureErrorMetadataExtractor(func(err error) *lua.ErrorMetadata {
        for e := err; e != nil; e = errors.Unwrap(e) {
            kindProvider, hasKind := e.(interface{ ErrorKind() string })
            retryProvider, hasRetry := e.(interface{ ErrorRetryable() (bool, bool) })
            detailsProvider, hasDetails := e.(interface{ ErrorDetails() map[string]any })
            if !hasKind && !hasRetry && !hasDetails {
                continue
            }

            meta := &lua.ErrorMetadata{}
            if hasKind {
                meta.Kind = lua.Kind(kindProvider.ErrorKind())
            }
            if hasRetry {
                if b, ok := retryProvider.ErrorRetryable(); ok {
                    v := b
                    meta.Retryable = &v
                }
            }
            if hasDetails {
                meta.Details = detailsProvider.ErrorDetails()
            }
            return meta
        }
        return nil
    })
}
```

`ConfigureErrorMetadataExtractor` is one-time (subsequent calls are ignored).
For one-off calls, use `WrapErrorWithMetadata(err, context, extractor)` instead of changing global process state.

## License

MIT — see LICENSE. Based on gopher-lua by Yusuke Inuzuka.

## Disclaimer

This project includes AI-generated and AI-assisted implementation work.

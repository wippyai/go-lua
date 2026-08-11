# Architectural-cut compiler

This package deliberately accepts one declaration shape: extract exact direct
fields of a source struct into one newly-created contained child struct.

```go
declaration := architecture.Declaration{
    Name: "link-state-cut",
    Boundary: architecture.Boundary{ID: "link-state", From: "link", To: "link-state"},
    Parent: cutplan.SymbolRef{Object: "example/link#package:Link"},
    Fields: []cutplan.SymbolRef{
        {Object: "example/link#type:Link/field:edges"},
        {Object: "example/link#type:Link/field:ports"},
    },
    Destination: architecture.ContainmentDestination{
        Path: "program/link/state.go", ImportPath: "example/link", Package: "link",
        Child: "linkState", Through: "state",
    },
    Laws: []cutplan.Law{{ID: "state-law", Package: "./program/link", Test: "TestStateLaw"}},
}
survey, err := architecture.CollectSurvey(ctx, session, declaration)
intent, err := architecture.Compile(declaration, survey)
```

`intent` is review material, not write authority. Normal workbench Prepare
turns it into the only mutable authority, a generated Lock.

The compiler derives target field identities (same member names on `Child`),
all supported field routes from resolver evidence, the source import only if
the destination package differs, read/write closure, and the complete three
structural gates. It rejects promoted fields, non-source keyed literals,
unsafe source-literal regrouping, unresolved or mismatched source identities,
existing target children, and destination package ambiguity.

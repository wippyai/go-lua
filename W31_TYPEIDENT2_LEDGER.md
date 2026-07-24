# W31 type-identity continuation ledger

Frozen fixture data and `__legacy` were not changed. All retained work is in the isolated clone.

| Fixture | Status | Seam trace |
| --- | --- | --- |
| `modules/host-global-qualified-type` | fixed | The fixture host registry now publishes the `stream` manifest's exact `Stream` definition and its `stream` global value type. Lint includes selected host manifests in qualified type lookup and passes their `GlobalTypes` through the dedicated engine capability. The engine resolves `stream.open` only from that published global type, then preserves the exact return summary. Missing globals, missing members, and malformed provider paths remain unavailable. |
| `realworld/service-locator` | fixed / already green | Passed in the base 500/673 oracle and in this verification run. No change was needed or made. |
| `realworld/module-with-generics` | fixed | Structural canonical decoding now allocates `Generic` binders before their bodies, just as it already does recursive binders. A recursive generic callable can therefore cross the closed export seam and is reattached to the manifest's exact declaration by the existing scope logic. At a zero-argument constructor call, only unbound type parameters become `unknown`; parameter-independent members such as `Collection<T>.count` keep their proven `number` return type. |

The same generic-binder repair also removed four related generic fixture failures. No fixture data or `__legacy` code was changed.

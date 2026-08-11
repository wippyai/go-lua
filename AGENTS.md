# Repository test safety

- Never run `go test`, an engine test binary, or another repository test command directly.
- Run every test through `scripts/bounded_test.sh`. Its process-tree RSS and wall-clock ceilings are mandatory, including for focused tests and race tests.
- Start with the narrowest named test. Invoke the runner directly, for example `scripts/bounded_test.sh go test ./analysis/engine -run '^TestName$' -count=1`; do not add per-run environment prefixes.
- The runner owns its writable Go build cache and defaults to a 20 GiB hard process-tree RSS ceiling. Broaden scope only after the focused test converges under the runner.
- Exit status 125 is a safety failure. Stop and diagnose the convergence/allocation defect; do not retry repeatedly or raise a limit to make it pass.
- Do not override the default memory or time ceilings without explicit approval from the root orchestrator after reporting the measured peak and reason.

type Proc = {
    write_stdin: (Proc, string) -> unknown,
    wait: (Proc) -> unknown,
    stdout_stream: (Proc) -> any,
    stderr_stream: (Proc) -> any,
    start: (Proc) -> boolean,
    kill: (Proc) -> unknown,
}
type Shell = {
    exec: (Shell, string) -> (Proc?, string?),
    release: (Shell) -> unknown,
}
local M = {}
function M.get(_): (Shell?, string?) return nil, nil end
return M

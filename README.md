# go-lua

Heavily modified Fork of [gopher-lua](https://github.com/yuin/gopher-lua). Lua 5.1 VM with integers/bitwise from 5.3, plus a custom flow-sensitive type checker. Optimized for actor runtimes with shared immutable stdlib and low per-state overhead.

```bash
go get github.com/wippyai/go-lua
```

## Type Annotations

```lua
local x: number = 42
local name: string = "alice"
local items: {number} = {1, 2, 3}
local lookup: {[string]: number} = {a = 1, b = 2}

type Point = {x: number, y: number}
local p: Point = {x = 10, y = 20}

local fn: (number, number) -> number = function(a, b) return a + b end
```

## Generics

```lua
local function first<T>(arr: {T}): T?
    return arr[1]
end

local n = first({1, 2, 3})      -- integer?
local s = first({"a", "b"})     -- string?

type Box<T> = {value: T}
local box: Box<string> = {value = "hello"}
```

## Flow Narrowing

The checker tracks types through control flow. After a nil check, the type is narrowed:

```lua
local function process(data: {value: number}?)
    if not data then return end
    print(data.value)  -- data is not nil here
end
```

Works with union types too:

```lua
type Exit = {kind: "exit", code: number}
type Message = {kind: "message", text: string}
type Event = Exit | Message

local function handle(e: Event)
    if e.kind == "exit" then
        print(e.code)  -- e is Exit
    else
        print(e.text)  -- e is Message
    end
end
```

## Effects

Functions track side effects. The stdlib is annotated with what each function does — mutations, errors, I/O. This lets the checker understand code like:

```lua
assert(x ~= nil)
print(x.field)  -- x narrowed after assert
```

## Running the VM

```go
L := lua.NewState()
defer L.Close()

if err := L.DoString(`print("hello")`); err != nil {
    panic(err)
}
```

## Type Checking

```go
import (
    "github.com/ponyruntime/go-lua/compiler/parse"
    "github.com/ponyruntime/go-lua/types"
)

chunk, _ := parse.Parse(reader, "script.lua")
for _, d := range types.CheckChunk(chunk, types.WithStdlib()) {
    fmt.Printf("%s:%d: %s\n", d.Source, d.Line, d.Message)
}
```

## License

MIT — see LICENSE. Based on gopher-lua by Yusuke Inuzuka.

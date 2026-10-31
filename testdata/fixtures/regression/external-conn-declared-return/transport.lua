-- External transport declarations have no Lua return paths.
type Conn = {id: string}
type Transport = {
    connect: (string?) -> (Conn?, string?),
    get_org: (Conn) -> any,
}
return {} :: Transport

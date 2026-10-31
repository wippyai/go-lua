type Response = {
    status_code: integer,
    body: string,
    headers: {[string]: string},
    stream: any,
}
local http_client = {}
function http_client.get(url: string, opts: any?): (Response, error?) return nil :: any, nil end
function http_client.put(url: string, opts: any?): (Response, error?) return nil :: any, nil end
function http_client.post(url: string, opts: any?): (Response, error?) return nil :: any, nil end
function http_client.request(method: string, url: string, opts: any?): (Response, error?) return nil :: any, nil end
return http_client

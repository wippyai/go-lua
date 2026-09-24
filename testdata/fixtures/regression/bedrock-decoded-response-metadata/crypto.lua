local function hmac_sha256(key: string, data: string): (string, error?) return "", nil end
return { hmac = { sha256 = hmac_sha256 } }

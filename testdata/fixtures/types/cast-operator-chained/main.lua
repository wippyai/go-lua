local function gg(result: any): string
    if type(result) ~= "table" or not (result :: { success: boolean }).success then
        return "fail"
    end
    return (result :: { id: string }).id
end
return gg({ success = true, id = "x1" })

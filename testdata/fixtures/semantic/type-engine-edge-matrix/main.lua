type Profile = {
    id: string,
    count: number,
    flag: boolean,
    label: string?,
    tags: {[string]: string},
}

type Admin = {
    kind: "admin",
    id: string,
    level: number,
}

type Guest = {
    kind: "guest",
    id: string,
    expires: number,
}

type Principal = Admin | Guest
type Result<T> = {ok: true, value: T} | {ok: false, error: string}

local function profile(id: string, count: number, label: string?): Profile
    local tags: {[string]: string} = {}
    tags["source"] = id
    return {id = id, count = count, flag = count > 0, label = label, tags = tags}
end

local function identity<T>(value: T): T
    return value
end

local function ok<T>(value: T): Result<T>
    return {ok = true, value = value}
end

local function err<T>(message: string): Result<T>
    return {ok = false, error = message}
end

local function map_result<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
    if result.ok then
        return ok(fn(result.value))
    end
    return err(result.error)
end

local function bind_result<T, U>(result: Result<T>, fn: (T) -> Result<U>): Result<U>
    if result.ok then
        return fn(result.value)
    end
    return err(result.error)
end

local function consume_profile(p: Profile, fn: (Profile) -> string): string
    return fn(p)
end

local function principal(index: number): Principal
    if index % 2 == 0 then
        return {kind = "admin", id = "a" .. tostring(index), level = index}
    end
    return {kind = "guest", id = "g" .. tostring(index), expires = index}
end

-- case 001: primitive positive inference
local s_1: string = "s1"
-- case 002: primitive negative assignment
local bad_s_1: number = "s1" -- expect-error
-- case 003: primitive positive inference
local s_2: string = "s2"
-- case 004: primitive negative assignment
local bad_s_2: number = "s2" -- expect-error
-- case 005: primitive positive inference
local s_3: string = "s3"
-- case 006: primitive negative assignment
local bad_s_3: number = "s3" -- expect-error
-- case 007: primitive positive inference
local s_4: string = "s4"
-- case 008: primitive negative assignment
local bad_s_4: number = "s4" -- expect-error
-- case 009: primitive positive inference
local s_5: string = "s5"
-- case 010: primitive negative assignment
local bad_s_5: number = "s5" -- expect-error
-- case 011: primitive positive inference
local s_6: string = "s6"
-- case 012: primitive negative assignment
local bad_s_6: number = "s6" -- expect-error
-- case 013: primitive positive inference
local s_7: string = "s7"
-- case 014: primitive negative assignment
local bad_s_7: number = "s7" -- expect-error
-- case 015: primitive positive inference
local s_8: string = "s8"
-- case 016: primitive negative assignment
local bad_s_8: number = "s8" -- expect-error
-- case 017: primitive positive inference
local s_9: string = "s9"
-- case 018: primitive negative assignment
local bad_s_9: number = "s9" -- expect-error
-- case 019: primitive positive inference
local s_10: string = "s10"
-- case 020: primitive negative assignment
local bad_s_10: number = "s10" -- expect-error
-- case 021: primitive positive inference
local s_11: string = "s11"
-- case 022: primitive negative assignment
local bad_s_11: number = "s11" -- expect-error
-- case 023: primitive positive inference
local s_12: string = "s12"
-- case 024: primitive negative assignment
local bad_s_12: number = "s12" -- expect-error
-- case 025: primitive positive inference
local s_13: string = "s13"
-- case 026: primitive negative assignment
local bad_s_13: number = "s13" -- expect-error
-- case 027: primitive positive inference
local s_14: string = "s14"
-- case 028: primitive negative assignment
local bad_s_14: number = "s14" -- expect-error
-- case 029: primitive positive inference
local s_15: string = "s15"
-- case 030: primitive negative assignment
local bad_s_15: number = "s15" -- expect-error
-- case 031: primitive positive inference
local s_16: string = "s16"
-- case 032: primitive negative assignment
local bad_s_16: number = "s16" -- expect-error
-- case 033: primitive positive inference
local s_17: string = "s17"
-- case 034: primitive negative assignment
local bad_s_17: number = "s17" -- expect-error
-- case 035: primitive positive inference
local s_18: string = "s18"
-- case 036: primitive negative assignment
local bad_s_18: number = "s18" -- expect-error
-- case 037: primitive positive inference
local s_19: string = "s19"
-- case 038: primitive negative assignment
local bad_s_19: number = "s19" -- expect-error
-- case 039: primitive positive inference
local s_20: string = "s20"
-- case 040: primitive negative assignment
local bad_s_20: number = "s20" -- expect-error
-- case 041: numeric arithmetic remains number
local n_1: number = 1 + 2
-- case 042: numeric arithmetic not string
local bad_n_1: string = 1 + 2 -- expect-error
-- case 043: numeric arithmetic remains number
local n_2: number = 2 + 3
-- case 044: numeric arithmetic not string
local bad_n_2: string = 2 + 3 -- expect-error
-- case 045: numeric arithmetic remains number
local n_3: number = 3 + 4
-- case 046: numeric arithmetic not string
local bad_n_3: string = 3 + 4 -- expect-error
-- case 047: numeric arithmetic remains number
local n_4: number = 4 + 5
-- case 048: numeric arithmetic not string
local bad_n_4: string = 4 + 5 -- expect-error
-- case 049: numeric arithmetic remains number
local n_5: number = 5 + 6
-- case 050: numeric arithmetic not string
local bad_n_5: string = 5 + 6 -- expect-error
-- case 051: numeric arithmetic remains number
local n_6: number = 6 + 7
-- case 052: numeric arithmetic not string
local bad_n_6: string = 6 + 7 -- expect-error
-- case 053: numeric arithmetic remains number
local n_7: number = 7 + 8
-- case 054: numeric arithmetic not string
local bad_n_7: string = 7 + 8 -- expect-error
-- case 055: numeric arithmetic remains number
local n_8: number = 8 + 9
-- case 056: numeric arithmetic not string
local bad_n_8: string = 8 + 9 -- expect-error
-- case 057: numeric arithmetic remains number
local n_9: number = 9 + 10
-- case 058: numeric arithmetic not string
local bad_n_9: string = 9 + 10 -- expect-error
-- case 059: numeric arithmetic remains number
local n_10: number = 10 + 11
-- case 060: numeric arithmetic not string
local bad_n_10: string = 10 + 11 -- expect-error
-- case 061: generic identity preserves string
local id_s_1: string = identity("id1")
-- case 062: generic identity rejects wrong target
local bad_id_s_1: number = identity("id1") -- expect-error
-- case 063: generic identity preserves string
local id_s_2: string = identity("id2")
-- case 064: generic identity rejects wrong target
local bad_id_s_2: number = identity("id2") -- expect-error
-- case 065: generic identity preserves string
local id_s_3: string = identity("id3")
-- case 066: generic identity rejects wrong target
local bad_id_s_3: number = identity("id3") -- expect-error
-- case 067: generic identity preserves string
local id_s_4: string = identity("id4")
-- case 068: generic identity rejects wrong target
local bad_id_s_4: number = identity("id4") -- expect-error
-- case 069: generic identity preserves string
local id_s_5: string = identity("id5")
-- case 070: generic identity rejects wrong target
local bad_id_s_5: number = identity("id5") -- expect-error
-- case 071: generic identity preserves string
local id_s_6: string = identity("id6")
-- case 072: generic identity rejects wrong target
local bad_id_s_6: number = identity("id6") -- expect-error
-- case 073: generic identity preserves string
local id_s_7: string = identity("id7")
-- case 074: generic identity rejects wrong target
local bad_id_s_7: number = identity("id7") -- expect-error
-- case 075: generic identity preserves string
local id_s_8: string = identity("id8")
-- case 076: generic identity rejects wrong target
local bad_id_s_8: number = identity("id8") -- expect-error
-- case 077: generic identity preserves string
local id_s_9: string = identity("id9")
-- case 078: generic identity rejects wrong target
local bad_id_s_9: number = identity("id9") -- expect-error
-- case 079: generic identity preserves string
local id_s_10: string = identity("id10")
-- case 080: generic identity rejects wrong target
local bad_id_s_10: number = identity("id10") -- expect-error
-- case 081: generic identity preserves string
local id_s_11: string = identity("id11")
-- case 082: generic identity rejects wrong target
local bad_id_s_11: number = identity("id11") -- expect-error
-- case 083: generic identity preserves string
local id_s_12: string = identity("id12")
-- case 084: generic identity rejects wrong target
local bad_id_s_12: number = identity("id12") -- expect-error
-- case 085: generic identity preserves string
local id_s_13: string = identity("id13")
-- case 086: generic identity rejects wrong target
local bad_id_s_13: number = identity("id13") -- expect-error
-- case 087: generic identity preserves string
local id_s_14: string = identity("id14")
-- case 088: generic identity rejects wrong target
local bad_id_s_14: number = identity("id14") -- expect-error
-- case 089: generic identity preserves string
local id_s_15: string = identity("id15")
-- case 090: generic identity rejects wrong target
local bad_id_s_15: number = identity("id15") -- expect-error
-- case 091: generic identity preserves string
local id_s_16: string = identity("id16")
-- case 092: generic identity rejects wrong target
local bad_id_s_16: number = identity("id16") -- expect-error
-- case 093: generic identity preserves string
local id_s_17: string = identity("id17")
-- case 094: generic identity rejects wrong target
local bad_id_s_17: number = identity("id17") -- expect-error
-- case 095: generic identity preserves string
local id_s_18: string = identity("id18")
-- case 096: generic identity rejects wrong target
local bad_id_s_18: number = identity("id18") -- expect-error
-- case 097: generic identity preserves string
local id_s_19: string = identity("id19")
-- case 098: generic identity rejects wrong target
local bad_id_s_19: number = identity("id19") -- expect-error
-- case 099: generic identity preserves string
local id_s_20: string = identity("id20")
-- case 100: generic identity rejects wrong target
local bad_id_s_20: number = identity("id20") -- expect-error
local p_1 = profile("p1", 1, "label1")
-- case 101: constructor record field positive
local p_id_1: string = p_1.id
-- case 102: constructor record field negative
local bad_p_id_1: number = p_1.id -- expect-error
local p_2 = profile("p2", 2, "label2")
-- case 103: constructor record field positive
local p_id_2: string = p_2.id
-- case 104: constructor record field negative
local bad_p_id_2: number = p_2.id -- expect-error
local p_3 = profile("p3", 3, "label3")
-- case 105: constructor record field positive
local p_id_3: string = p_3.id
-- case 106: constructor record field negative
local bad_p_id_3: number = p_3.id -- expect-error
local p_4 = profile("p4", 4, "label4")
-- case 107: constructor record field positive
local p_id_4: string = p_4.id
-- case 108: constructor record field negative
local bad_p_id_4: number = p_4.id -- expect-error
local p_5 = profile("p5", 5, "label5")
-- case 109: constructor record field positive
local p_id_5: string = p_5.id
-- case 110: constructor record field negative
local bad_p_id_5: number = p_5.id -- expect-error
local p_6 = profile("p6", 6, "label6")
-- case 111: constructor record field positive
local p_id_6: string = p_6.id
-- case 112: constructor record field negative
local bad_p_id_6: number = p_6.id -- expect-error
local p_7 = profile("p7", 7, "label7")
-- case 113: constructor record field positive
local p_id_7: string = p_7.id
-- case 114: constructor record field negative
local bad_p_id_7: number = p_7.id -- expect-error
local p_8 = profile("p8", 8, "label8")
-- case 115: constructor record field positive
local p_id_8: string = p_8.id
-- case 116: constructor record field negative
local bad_p_id_8: number = p_8.id -- expect-error
local p_9 = profile("p9", 9, "label9")
-- case 117: constructor record field positive
local p_id_9: string = p_9.id
-- case 118: constructor record field negative
local bad_p_id_9: number = p_9.id -- expect-error
local p_10 = profile("p10", 10, "label10")
-- case 119: constructor record field positive
local p_id_10: string = p_10.id
-- case 120: constructor record field negative
local bad_p_id_10: number = p_10.id -- expect-error
local p_11 = profile("p11", 11, "label11")
-- case 121: constructor record field positive
local p_id_11: string = p_11.id
-- case 122: constructor record field negative
local bad_p_id_11: number = p_11.id -- expect-error
local p_12 = profile("p12", 12, "label12")
-- case 123: constructor record field positive
local p_id_12: string = p_12.id
-- case 124: constructor record field negative
local bad_p_id_12: number = p_12.id -- expect-error
local p_13 = profile("p13", 13, "label13")
-- case 125: constructor record field positive
local p_id_13: string = p_13.id
-- case 126: constructor record field negative
local bad_p_id_13: number = p_13.id -- expect-error
local p_14 = profile("p14", 14, "label14")
-- case 127: constructor record field positive
local p_id_14: string = p_14.id
-- case 128: constructor record field negative
local bad_p_id_14: number = p_14.id -- expect-error
local p_15 = profile("p15", 15, "label15")
-- case 129: constructor record field positive
local p_id_15: string = p_15.id
-- case 130: constructor record field negative
local bad_p_id_15: number = p_15.id -- expect-error
local p_16 = profile("p16", 16, "label16")
-- case 131: constructor record field positive
local p_id_16: string = p_16.id
-- case 132: constructor record field negative
local bad_p_id_16: number = p_16.id -- expect-error
local p_17 = profile("p17", 17, "label17")
-- case 133: constructor record field positive
local p_id_17: string = p_17.id
-- case 134: constructor record field negative
local bad_p_id_17: number = p_17.id -- expect-error
local p_18 = profile("p18", 18, "label18")
-- case 135: constructor record field positive
local p_id_18: string = p_18.id
-- case 136: constructor record field negative
local bad_p_id_18: number = p_18.id -- expect-error
local p_19 = profile("p19", 19, "label19")
-- case 137: constructor record field positive
local p_id_19: string = p_19.id
-- case 138: constructor record field negative
local bad_p_id_19: number = p_19.id -- expect-error
local p_20 = profile("p20", 20, "label20")
-- case 139: constructor record field positive
local p_id_20: string = p_20.id
-- case 140: constructor record field negative
local bad_p_id_20: number = p_20.id -- expect-error
local p_21 = profile("p21", 21, "label21")
-- case 141: constructor record field positive
local p_id_21: string = p_21.id
-- case 142: constructor record field negative
local bad_p_id_21: number = p_21.id -- expect-error
local p_22 = profile("p22", 22, "label22")
-- case 143: constructor record field positive
local p_id_22: string = p_22.id
-- case 144: constructor record field negative
local bad_p_id_22: number = p_22.id -- expect-error
local p_23 = profile("p23", 23, "label23")
-- case 145: constructor record field positive
local p_id_23: string = p_23.id
-- case 146: constructor record field negative
local bad_p_id_23: number = p_23.id -- expect-error
local p_24 = profile("p24", 24, "label24")
-- case 147: constructor record field positive
local p_id_24: string = p_24.id
-- case 148: constructor record field negative
local bad_p_id_24: number = p_24.id -- expect-error
local p_25 = profile("p25", 25, "label25")
-- case 149: constructor record field positive
local p_id_25: string = p_25.id
-- case 150: constructor record field negative
local bad_p_id_25: number = p_25.id -- expect-error
local opt_1 = profile("opt1", 1, "present1")
if opt_1.label then
-- case 151: optional field narrows under truthy guard
    local label_1: string = opt_1.label
-- case 152: optional narrowed value rejects wrong target
    local bad_label_1: number = opt_1.label -- expect-error
end
local opt_2 = profile("opt2", 2, "present2")
if opt_2.label then
-- case 153: optional field narrows under truthy guard
    local label_2: string = opt_2.label
-- case 154: optional narrowed value rejects wrong target
    local bad_label_2: number = opt_2.label -- expect-error
end
local opt_3 = profile("opt3", 3, "present3")
if opt_3.label then
-- case 155: optional field narrows under truthy guard
    local label_3: string = opt_3.label
-- case 156: optional narrowed value rejects wrong target
    local bad_label_3: number = opt_3.label -- expect-error
end
local opt_4 = profile("opt4", 4, "present4")
if opt_4.label then
-- case 157: optional field narrows under truthy guard
    local label_4: string = opt_4.label
-- case 158: optional narrowed value rejects wrong target
    local bad_label_4: number = opt_4.label -- expect-error
end
local opt_5 = profile("opt5", 5, "present5")
if opt_5.label then
-- case 159: optional field narrows under truthy guard
    local label_5: string = opt_5.label
-- case 160: optional narrowed value rejects wrong target
    local bad_label_5: number = opt_5.label -- expect-error
end
local opt_6 = profile("opt6", 6, "present6")
if opt_6.label then
-- case 161: optional field narrows under truthy guard
    local label_6: string = opt_6.label
-- case 162: optional narrowed value rejects wrong target
    local bad_label_6: number = opt_6.label -- expect-error
end
local opt_7 = profile("opt7", 7, "present7")
if opt_7.label then
-- case 163: optional field narrows under truthy guard
    local label_7: string = opt_7.label
-- case 164: optional narrowed value rejects wrong target
    local bad_label_7: number = opt_7.label -- expect-error
end
local opt_8 = profile("opt8", 8, "present8")
if opt_8.label then
-- case 165: optional field narrows under truthy guard
    local label_8: string = opt_8.label
-- case 166: optional narrowed value rejects wrong target
    local bad_label_8: number = opt_8.label -- expect-error
end
local opt_9 = profile("opt9", 9, "present9")
if opt_9.label then
-- case 167: optional field narrows under truthy guard
    local label_9: string = opt_9.label
-- case 168: optional narrowed value rejects wrong target
    local bad_label_9: number = opt_9.label -- expect-error
end
local opt_10 = profile("opt10", 10, "present10")
if opt_10.label then
-- case 169: optional field narrows under truthy guard
    local label_10: string = opt_10.label
-- case 170: optional narrowed value rejects wrong target
    local bad_label_10: number = opt_10.label -- expect-error
end
local opt_11 = profile("opt11", 11, "present11")
if opt_11.label then
-- case 171: optional field narrows under truthy guard
    local label_11: string = opt_11.label
-- case 172: optional narrowed value rejects wrong target
    local bad_label_11: number = opt_11.label -- expect-error
end
local opt_12 = profile("opt12", 12, "present12")
if opt_12.label then
-- case 173: optional field narrows under truthy guard
    local label_12: string = opt_12.label
-- case 174: optional narrowed value rejects wrong target
    local bad_label_12: number = opt_12.label -- expect-error
end
local opt_13 = profile("opt13", 13, "present13")
if opt_13.label then
-- case 175: optional field narrows under truthy guard
    local label_13: string = opt_13.label
-- case 176: optional narrowed value rejects wrong target
    local bad_label_13: number = opt_13.label -- expect-error
end
local opt_14 = profile("opt14", 14, "present14")
if opt_14.label then
-- case 177: optional field narrows under truthy guard
    local label_14: string = opt_14.label
-- case 178: optional narrowed value rejects wrong target
    local bad_label_14: number = opt_14.label -- expect-error
end
local opt_15 = profile("opt15", 15, "present15")
if opt_15.label then
-- case 179: optional field narrows under truthy guard
    local label_15: string = opt_15.label
-- case 180: optional narrowed value rejects wrong target
    local bad_label_15: number = opt_15.label -- expect-error
end
local opt_16 = profile("opt16", 16, "present16")
if opt_16.label then
-- case 181: optional field narrows under truthy guard
    local label_16: string = opt_16.label
-- case 182: optional narrowed value rejects wrong target
    local bad_label_16: number = opt_16.label -- expect-error
end
local opt_17 = profile("opt17", 17, "present17")
if opt_17.label then
-- case 183: optional field narrows under truthy guard
    local label_17: string = opt_17.label
-- case 184: optional narrowed value rejects wrong target
    local bad_label_17: number = opt_17.label -- expect-error
end
local opt_18 = profile("opt18", 18, "present18")
if opt_18.label then
-- case 185: optional field narrows under truthy guard
    local label_18: string = opt_18.label
-- case 186: optional narrowed value rejects wrong target
    local bad_label_18: number = opt_18.label -- expect-error
end
local opt_19 = profile("opt19", 19, "present19")
if opt_19.label then
-- case 187: optional field narrows under truthy guard
    local label_19: string = opt_19.label
-- case 188: optional narrowed value rejects wrong target
    local bad_label_19: number = opt_19.label -- expect-error
end
local opt_20 = profile("opt20", 20, "present20")
if opt_20.label then
-- case 189: optional field narrows under truthy guard
    local label_20: string = opt_20.label
-- case 190: optional narrowed value rejects wrong target
    local bad_label_20: number = opt_20.label -- expect-error
end
local tag_1 = p_1.tags["source"]
if tag_1 then
-- case 191: map indexed value narrows after local guard
    local tag_text_1: string = tag_1
-- case 192: map indexed value rejects wrong target
    local bad_tag_text_1: number = tag_1 -- expect-error
end
local tag_2 = p_2.tags["source"]
if tag_2 then
-- case 193: map indexed value narrows after local guard
    local tag_text_2: string = tag_2
-- case 194: map indexed value rejects wrong target
    local bad_tag_text_2: number = tag_2 -- expect-error
end
local tag_3 = p_3.tags["source"]
if tag_3 then
-- case 195: map indexed value narrows after local guard
    local tag_text_3: string = tag_3
-- case 196: map indexed value rejects wrong target
    local bad_tag_text_3: number = tag_3 -- expect-error
end
local tag_4 = p_4.tags["source"]
if tag_4 then
-- case 197: map indexed value narrows after local guard
    local tag_text_4: string = tag_4
-- case 198: map indexed value rejects wrong target
    local bad_tag_text_4: number = tag_4 -- expect-error
end
local tag_5 = p_5.tags["source"]
if tag_5 then
-- case 199: map indexed value narrows after local guard
    local tag_text_5: string = tag_5
-- case 200: map indexed value rejects wrong target
    local bad_tag_text_5: number = tag_5 -- expect-error
end
local tag_6 = p_6.tags["source"]
if tag_6 then
-- case 201: map indexed value narrows after local guard
    local tag_text_6: string = tag_6
-- case 202: map indexed value rejects wrong target
    local bad_tag_text_6: number = tag_6 -- expect-error
end
local tag_7 = p_7.tags["source"]
if tag_7 then
-- case 203: map indexed value narrows after local guard
    local tag_text_7: string = tag_7
-- case 204: map indexed value rejects wrong target
    local bad_tag_text_7: number = tag_7 -- expect-error
end
local tag_8 = p_8.tags["source"]
if tag_8 then
-- case 205: map indexed value narrows after local guard
    local tag_text_8: string = tag_8
-- case 206: map indexed value rejects wrong target
    local bad_tag_text_8: number = tag_8 -- expect-error
end
local tag_9 = p_9.tags["source"]
if tag_9 then
-- case 207: map indexed value narrows after local guard
    local tag_text_9: string = tag_9
-- case 208: map indexed value rejects wrong target
    local bad_tag_text_9: number = tag_9 -- expect-error
end
local tag_10 = p_10.tags["source"]
if tag_10 then
-- case 209: map indexed value narrows after local guard
    local tag_text_10: string = tag_10
-- case 210: map indexed value rejects wrong target
    local bad_tag_text_10: number = tag_10 -- expect-error
end
local tag_11 = p_11.tags["source"]
if tag_11 then
-- case 211: map indexed value narrows after local guard
    local tag_text_11: string = tag_11
-- case 212: map indexed value rejects wrong target
    local bad_tag_text_11: number = tag_11 -- expect-error
end
local tag_12 = p_12.tags["source"]
if tag_12 then
-- case 213: map indexed value narrows after local guard
    local tag_text_12: string = tag_12
-- case 214: map indexed value rejects wrong target
    local bad_tag_text_12: number = tag_12 -- expect-error
end
local tag_13 = p_13.tags["source"]
if tag_13 then
-- case 215: map indexed value narrows after local guard
    local tag_text_13: string = tag_13
-- case 216: map indexed value rejects wrong target
    local bad_tag_text_13: number = tag_13 -- expect-error
end
local tag_14 = p_14.tags["source"]
if tag_14 then
-- case 217: map indexed value narrows after local guard
    local tag_text_14: string = tag_14
-- case 218: map indexed value rejects wrong target
    local bad_tag_text_14: number = tag_14 -- expect-error
end
local tag_15 = p_15.tags["source"]
if tag_15 then
-- case 219: map indexed value narrows after local guard
    local tag_text_15: string = tag_15
-- case 220: map indexed value rejects wrong target
    local bad_tag_text_15: number = tag_15 -- expect-error
end
local r_1 = ok(profile("r1", 1, "rl1"))
if r_1.ok then
-- case 221: generic Result preserves record value
    local r_profile_1: Profile = r_1.value
-- case 222: generic Result record field rejects wrong target
    local bad_r_profile_1: string = r_1.value.count -- expect-error
end
local mapped_1 = map_result(r_1, function(item: Profile): string
    return item.id
end)
if mapped_1.ok then
-- case 223: generic map callback return is preserved
    local mapped_text_1: string = mapped_1.value
-- case 224: generic map callback return rejects wrong target
    local bad_mapped_text_1: number = mapped_1.value -- expect-error
end
local r_2 = ok(profile("r2", 2, "rl2"))
if r_2.ok then
-- case 225: generic Result preserves record value
    local r_profile_2: Profile = r_2.value
-- case 226: generic Result record field rejects wrong target
    local bad_r_profile_2: string = r_2.value.count -- expect-error
end
local mapped_2 = map_result(r_2, function(item: Profile): string
    return item.id
end)
if mapped_2.ok then
-- case 227: generic map callback return is preserved
    local mapped_text_2: string = mapped_2.value
-- case 228: generic map callback return rejects wrong target
    local bad_mapped_text_2: number = mapped_2.value -- expect-error
end
local r_3 = ok(profile("r3", 3, "rl3"))
if r_3.ok then
-- case 229: generic Result preserves record value
    local r_profile_3: Profile = r_3.value
-- case 230: generic Result record field rejects wrong target
    local bad_r_profile_3: string = r_3.value.count -- expect-error
end
local mapped_3 = map_result(r_3, function(item: Profile): string
    return item.id
end)
if mapped_3.ok then
-- case 231: generic map callback return is preserved
    local mapped_text_3: string = mapped_3.value
-- case 232: generic map callback return rejects wrong target
    local bad_mapped_text_3: number = mapped_3.value -- expect-error
end
local r_4 = ok(profile("r4", 4, "rl4"))
if r_4.ok then
-- case 233: generic Result preserves record value
    local r_profile_4: Profile = r_4.value
-- case 234: generic Result record field rejects wrong target
    local bad_r_profile_4: string = r_4.value.count -- expect-error
end
local mapped_4 = map_result(r_4, function(item: Profile): string
    return item.id
end)
if mapped_4.ok then
-- case 235: generic map callback return is preserved
    local mapped_text_4: string = mapped_4.value
-- case 236: generic map callback return rejects wrong target
    local bad_mapped_text_4: number = mapped_4.value -- expect-error
end
local r_5 = ok(profile("r5", 5, "rl5"))
if r_5.ok then
-- case 237: generic Result preserves record value
    local r_profile_5: Profile = r_5.value
-- case 238: generic Result record field rejects wrong target
    local bad_r_profile_5: string = r_5.value.count -- expect-error
end
local mapped_5 = map_result(r_5, function(item: Profile): string
    return item.id
end)
if mapped_5.ok then
-- case 239: generic map callback return is preserved
    local mapped_text_5: string = mapped_5.value
-- case 240: generic map callback return rejects wrong target
    local bad_mapped_text_5: number = mapped_5.value -- expect-error
end
local r_6 = ok(profile("r6", 6, "rl6"))
if r_6.ok then
-- case 241: generic Result preserves record value
    local r_profile_6: Profile = r_6.value
-- case 242: generic Result record field rejects wrong target
    local bad_r_profile_6: string = r_6.value.count -- expect-error
end
local mapped_6 = map_result(r_6, function(item: Profile): string
    return item.id
end)
if mapped_6.ok then
-- case 243: generic map callback return is preserved
    local mapped_text_6: string = mapped_6.value
-- case 244: generic map callback return rejects wrong target
    local bad_mapped_text_6: number = mapped_6.value -- expect-error
end
local r_7 = ok(profile("r7", 7, "rl7"))
if r_7.ok then
-- case 245: generic Result preserves record value
    local r_profile_7: Profile = r_7.value
-- case 246: generic Result record field rejects wrong target
    local bad_r_profile_7: string = r_7.value.count -- expect-error
end
local mapped_7 = map_result(r_7, function(item: Profile): string
    return item.id
end)
if mapped_7.ok then
-- case 247: generic map callback return is preserved
    local mapped_text_7: string = mapped_7.value
-- case 248: generic map callback return rejects wrong target
    local bad_mapped_text_7: number = mapped_7.value -- expect-error
end
local r_8 = ok(profile("r8", 8, "rl8"))
if r_8.ok then
-- case 249: generic Result preserves record value
    local r_profile_8: Profile = r_8.value
-- case 250: generic Result record field rejects wrong target
    local bad_r_profile_8: string = r_8.value.count -- expect-error
end
local mapped_8 = map_result(r_8, function(item: Profile): string
    return item.id
end)
if mapped_8.ok then
-- case 251: generic map callback return is preserved
    local mapped_text_8: string = mapped_8.value
-- case 252: generic map callback return rejects wrong target
    local bad_mapped_text_8: number = mapped_8.value -- expect-error
end
local r_9 = ok(profile("r9", 9, "rl9"))
if r_9.ok then
-- case 253: generic Result preserves record value
    local r_profile_9: Profile = r_9.value
-- case 254: generic Result record field rejects wrong target
    local bad_r_profile_9: string = r_9.value.count -- expect-error
end
local mapped_9 = map_result(r_9, function(item: Profile): string
    return item.id
end)
if mapped_9.ok then
-- case 255: generic map callback return is preserved
    local mapped_text_9: string = mapped_9.value
-- case 256: generic map callback return rejects wrong target
    local bad_mapped_text_9: number = mapped_9.value -- expect-error
end
local r_10 = ok(profile("r10", 10, "rl10"))
if r_10.ok then
-- case 257: generic Result preserves record value
    local r_profile_10: Profile = r_10.value
-- case 258: generic Result record field rejects wrong target
    local bad_r_profile_10: string = r_10.value.count -- expect-error
end
local mapped_10 = map_result(r_10, function(item: Profile): string
    return item.id
end)
if mapped_10.ok then
-- case 259: generic map callback return is preserved
    local mapped_text_10: string = mapped_10.value
-- case 260: generic map callback return rejects wrong target
    local bad_mapped_text_10: number = mapped_10.value -- expect-error
end
-- case 261: callback parameter annotation and return positive
local cb_1: string = consume_profile(profile("cb1", 1, nil), function(item: Profile): string
    return item.id
end)
-- case 262: callback return rejects wrong assignment
local bad_cb_1: number = consume_profile(profile("cbx1", 1, nil), function(item: Profile): string -- expect-error
    return item.id
end)
-- case 263: callback parameter annotation and return positive
local cb_2: string = consume_profile(profile("cb2", 2, nil), function(item: Profile): string
    return item.id
end)
-- case 264: callback return rejects wrong assignment
local bad_cb_2: number = consume_profile(profile("cbx2", 2, nil), function(item: Profile): string -- expect-error
    return item.id
end)
-- case 265: callback parameter annotation and return positive
local cb_3: string = consume_profile(profile("cb3", 3, nil), function(item: Profile): string
    return item.id
end)
-- case 266: callback return rejects wrong assignment
local bad_cb_3: number = consume_profile(profile("cbx3", 3, nil), function(item: Profile): string -- expect-error
    return item.id
end)
-- case 267: callback parameter annotation and return positive
local cb_4: string = consume_profile(profile("cb4", 4, nil), function(item: Profile): string
    return item.id
end)
-- case 268: callback return rejects wrong assignment
local bad_cb_4: number = consume_profile(profile("cbx4", 4, nil), function(item: Profile): string -- expect-error
    return item.id
end)
-- case 269: callback parameter annotation and return positive
local cb_5: string = consume_profile(profile("cb5", 5, nil), function(item: Profile): string
    return item.id
end)
-- case 270: callback return rejects wrong assignment
local bad_cb_5: number = consume_profile(profile("cbx5", 5, nil), function(item: Profile): string -- expect-error
    return item.id
end)
-- case 271: callback parameter annotation and return positive
local cb_6: string = consume_profile(profile("cb6", 6, nil), function(item: Profile): string
    return item.id
end)
-- case 272: callback return rejects wrong assignment
local bad_cb_6: number = consume_profile(profile("cbx6", 6, nil), function(item: Profile): string -- expect-error
    return item.id
end)
-- case 273: callback parameter annotation and return positive
local cb_7: string = consume_profile(profile("cb7", 7, nil), function(item: Profile): string
    return item.id
end)
-- case 274: callback return rejects wrong assignment
local bad_cb_7: number = consume_profile(profile("cbx7", 7, nil), function(item: Profile): string -- expect-error
    return item.id
end)
-- case 275: callback parameter annotation and return positive
local cb_8: string = consume_profile(profile("cb8", 8, nil), function(item: Profile): string
    return item.id
end)
-- case 276: callback return rejects wrong assignment
local bad_cb_8: number = consume_profile(profile("cbx8", 8, nil), function(item: Profile): string -- expect-error
    return item.id
end)
-- case 277: callback parameter annotation and return positive
local cb_9: string = consume_profile(profile("cb9", 9, nil), function(item: Profile): string
    return item.id
end)
-- case 278: callback return rejects wrong assignment
local bad_cb_9: number = consume_profile(profile("cbx9", 9, nil), function(item: Profile): string -- expect-error
    return item.id
end)
-- case 279: callback parameter annotation and return positive
local cb_10: string = consume_profile(profile("cb10", 10, nil), function(item: Profile): string
    return item.id
end)
-- case 280: callback return rejects wrong assignment
local bad_cb_10: number = consume_profile(profile("cbx10", 10, nil), function(item: Profile): string -- expect-error
    return item.id
end)
local bound_1 = bind_result(ok(profile("b1", 1, nil)), function(item: Profile): Result<number>
    return ok(item.count + 1)
end)
if bound_1.ok then
-- case 281: generic bind preserves callback Result value
    local bound_number_1: number = bound_1.value
-- case 282: generic bind value rejects wrong target
    local bad_bound_number_1: string = bound_1.value -- expect-error
end
local bound_2 = bind_result(ok(profile("b2", 2, nil)), function(item: Profile): Result<number>
    return ok(item.count + 1)
end)
if bound_2.ok then
-- case 283: generic bind preserves callback Result value
    local bound_number_2: number = bound_2.value
-- case 284: generic bind value rejects wrong target
    local bad_bound_number_2: string = bound_2.value -- expect-error
end
local bound_3 = bind_result(ok(profile("b3", 3, nil)), function(item: Profile): Result<number>
    return ok(item.count + 1)
end)
if bound_3.ok then
-- case 285: generic bind preserves callback Result value
    local bound_number_3: number = bound_3.value
-- case 286: generic bind value rejects wrong target
    local bad_bound_number_3: string = bound_3.value -- expect-error
end
local bound_4 = bind_result(ok(profile("b4", 4, nil)), function(item: Profile): Result<number>
    return ok(item.count + 1)
end)
if bound_4.ok then
-- case 287: generic bind preserves callback Result value
    local bound_number_4: number = bound_4.value
-- case 288: generic bind value rejects wrong target
    local bad_bound_number_4: string = bound_4.value -- expect-error
end
local bound_5 = bind_result(ok(profile("b5", 5, nil)), function(item: Profile): Result<number>
    return ok(item.count + 1)
end)
if bound_5.ok then
-- case 289: generic bind preserves callback Result value
    local bound_number_5: number = bound_5.value
-- case 290: generic bind value rejects wrong target
    local bad_bound_number_5: string = bound_5.value -- expect-error
end
local bound_6 = bind_result(ok(profile("b6", 6, nil)), function(item: Profile): Result<number>
    return ok(item.count + 1)
end)
if bound_6.ok then
-- case 291: generic bind preserves callback Result value
    local bound_number_6: number = bound_6.value
-- case 292: generic bind value rejects wrong target
    local bad_bound_number_6: string = bound_6.value -- expect-error
end
local bound_7 = bind_result(ok(profile("b7", 7, nil)), function(item: Profile): Result<number>
    return ok(item.count + 1)
end)
if bound_7.ok then
-- case 293: generic bind preserves callback Result value
    local bound_number_7: number = bound_7.value
-- case 294: generic bind value rejects wrong target
    local bad_bound_number_7: string = bound_7.value -- expect-error
end
local bound_8 = bind_result(ok(profile("b8", 8, nil)), function(item: Profile): Result<number>
    return ok(item.count + 1)
end)
if bound_8.ok then
-- case 295: generic bind preserves callback Result value
    local bound_number_8: number = bound_8.value
-- case 296: generic bind value rejects wrong target
    local bad_bound_number_8: string = bound_8.value -- expect-error
end
local bound_9 = bind_result(ok(profile("b9", 9, nil)), function(item: Profile): Result<number>
    return ok(item.count + 1)
end)
if bound_9.ok then
-- case 297: generic bind preserves callback Result value
    local bound_number_9: number = bound_9.value
-- case 298: generic bind value rejects wrong target
    local bad_bound_number_9: string = bound_9.value -- expect-error
end
local bound_10 = bind_result(ok(profile("b10", 10, nil)), function(item: Profile): Result<number>
    return ok(item.count + 1)
end)
if bound_10.ok then
-- case 299: generic bind preserves callback Result value
    local bound_number_10: number = bound_10.value
-- case 300: generic bind value rejects wrong target
    local bad_bound_number_10: string = bound_10.value -- expect-error
end
local u_1: Principal = principal(1)
if u_1.kind == "admin" then
-- case 301: discriminated union admin branch positive
    local level_1: number = u_1.level
-- case 302: discriminated union admin branch rejects wrong field type
    local bad_level_1: string = u_1.level -- expect-error
else
-- case 303: discriminated union guest branch positive
    local expires_1: number = u_1.expires
-- case 304: discriminated union guest branch rejects missing admin field
    local bad_guest_level_1: number = u_1.level -- expect-error
end
local u_2: Principal = principal(2)
if u_2.kind == "admin" then
-- case 305: discriminated union admin branch positive
    local level_2: number = u_2.level
-- case 306: discriminated union admin branch rejects wrong field type
    local bad_level_2: string = u_2.level -- expect-error
else
-- case 307: discriminated union guest branch positive
    local expires_2: number = u_2.expires
-- case 308: discriminated union guest branch rejects missing admin field
    local bad_guest_level_2: number = u_2.level -- expect-error
end
local u_3: Principal = principal(3)
if u_3.kind == "admin" then
-- case 309: discriminated union admin branch positive
    local level_3: number = u_3.level
-- case 310: discriminated union admin branch rejects wrong field type
    local bad_level_3: string = u_3.level -- expect-error
else
-- case 311: discriminated union guest branch positive
    local expires_3: number = u_3.expires
-- case 312: discriminated union guest branch rejects missing admin field
    local bad_guest_level_3: number = u_3.level -- expect-error
end
local u_4: Principal = principal(4)
if u_4.kind == "admin" then
-- case 313: discriminated union admin branch positive
    local level_4: number = u_4.level
-- case 314: discriminated union admin branch rejects wrong field type
    local bad_level_4: string = u_4.level -- expect-error
else
-- case 315: discriminated union guest branch positive
    local expires_4: number = u_4.expires
-- case 316: discriminated union guest branch rejects missing admin field
    local bad_guest_level_4: number = u_4.level -- expect-error
end
local u_5: Principal = principal(5)
if u_5.kind == "admin" then
-- case 317: discriminated union admin branch positive
    local level_5: number = u_5.level
-- case 318: discriminated union admin branch rejects wrong field type
    local bad_level_5: string = u_5.level -- expect-error
else
-- case 319: discriminated union guest branch positive
    local expires_5: number = u_5.expires
-- case 320: discriminated union guest branch rejects missing admin field
    local bad_guest_level_5: number = u_5.level -- expect-error
end
local u_6: Principal = principal(6)
if u_6.kind == "admin" then
-- case 321: discriminated union admin branch positive
    local level_6: number = u_6.level
-- case 322: discriminated union admin branch rejects wrong field type
    local bad_level_6: string = u_6.level -- expect-error
else
-- case 323: discriminated union guest branch positive
    local expires_6: number = u_6.expires
-- case 324: discriminated union guest branch rejects missing admin field
    local bad_guest_level_6: number = u_6.level -- expect-error
end
local u_7: Principal = principal(7)
if u_7.kind == "admin" then
-- case 325: discriminated union admin branch positive
    local level_7: number = u_7.level
-- case 326: discriminated union admin branch rejects wrong field type
    local bad_level_7: string = u_7.level -- expect-error
else
-- case 327: discriminated union guest branch positive
    local expires_7: number = u_7.expires
-- case 328: discriminated union guest branch rejects missing admin field
    local bad_guest_level_7: number = u_7.level -- expect-error
end
local u_8: Principal = principal(8)
if u_8.kind == "admin" then
-- case 329: discriminated union admin branch positive
    local level_8: number = u_8.level
-- case 330: discriminated union admin branch rejects wrong field type
    local bad_level_8: string = u_8.level -- expect-error
else
-- case 331: discriminated union guest branch positive
    local expires_8: number = u_8.expires
-- case 332: discriminated union guest branch rejects missing admin field
    local bad_guest_level_8: number = u_8.level -- expect-error
end
local u_9: Principal = principal(9)
if u_9.kind == "admin" then
-- case 333: discriminated union admin branch positive
    local level_9: number = u_9.level
-- case 334: discriminated union admin branch rejects wrong field type
    local bad_level_9: string = u_9.level -- expect-error
else
-- case 335: discriminated union guest branch positive
    local expires_9: number = u_9.expires
-- case 336: discriminated union guest branch rejects missing admin field
    local bad_guest_level_9: number = u_9.level -- expect-error
end
local u_10: Principal = principal(10)
if u_10.kind == "admin" then
-- case 337: discriminated union admin branch positive
    local level_10: number = u_10.level
-- case 338: discriminated union admin branch rejects wrong field type
    local bad_level_10: string = u_10.level -- expect-error
else
-- case 339: discriminated union guest branch positive
    local expires_10: number = u_10.expires
-- case 340: discriminated union guest branch rejects missing admin field
    local bad_guest_level_10: number = u_10.level -- expect-error
end
local raw_1: any = {id = "raw1", count = 1, flag = true, tags = {source = "raw"}}
-- case 341: untrusted any cannot prove record
local trusted_raw_1: Profile = raw_1 -- expect-error
if type(raw_1.id) == "string" then
-- case 342: single field guard admits guarded scalar
    local raw_id_1: string = raw_1.id
-- case 343: single field guard does not prove whole record
    local raw_profile_1: Profile = raw_1 -- expect-error
end
local raw_2: any = {id = "raw2", count = 2, flag = true, tags = {source = "raw"}}
-- case 344: untrusted any cannot prove record
local trusted_raw_2: Profile = raw_2 -- expect-error
if type(raw_2.id) == "string" then
-- case 345: single field guard admits guarded scalar
    local raw_id_2: string = raw_2.id
-- case 346: single field guard does not prove whole record
    local raw_profile_2: Profile = raw_2 -- expect-error
end
local raw_3: any = {id = "raw3", count = 3, flag = true, tags = {source = "raw"}}
-- case 347: untrusted any cannot prove record
local trusted_raw_3: Profile = raw_3 -- expect-error
if type(raw_3.id) == "string" then
-- case 348: single field guard admits guarded scalar
    local raw_id_3: string = raw_3.id
-- case 349: single field guard does not prove whole record
    local raw_profile_3: Profile = raw_3 -- expect-error
end
local raw_4: any = {id = "raw4", count = 4, flag = true, tags = {source = "raw"}}
-- case 350: untrusted any cannot prove record
local trusted_raw_4: Profile = raw_4 -- expect-error
if type(raw_4.id) == "string" then
-- case 351: single field guard admits guarded scalar
    local raw_id_4: string = raw_4.id
-- case 352: single field guard does not prove whole record
    local raw_profile_4: Profile = raw_4 -- expect-error
end
local raw_5: any = {id = "raw5", count = 5, flag = true, tags = {source = "raw"}}
-- case 353: untrusted any cannot prove record
local trusted_raw_5: Profile = raw_5 -- expect-error
if type(raw_5.id) == "string" then
-- case 354: single field guard admits guarded scalar
    local raw_id_5: string = raw_5.id
-- case 355: single field guard does not prove whole record
    local raw_profile_5: Profile = raw_5 -- expect-error
end
local raw_6: any = {id = "raw6", count = 6, flag = true, tags = {source = "raw"}}
-- case 356: untrusted any cannot prove record
local trusted_raw_6: Profile = raw_6 -- expect-error
if type(raw_6.id) == "string" then
-- case 357: single field guard admits guarded scalar
    local raw_id_6: string = raw_6.id
-- case 358: single field guard does not prove whole record
    local raw_profile_6: Profile = raw_6 -- expect-error
end
local raw_7: any = {id = "raw7", count = 7, flag = true, tags = {source = "raw"}}
-- case 359: untrusted any cannot prove record
local trusted_raw_7: Profile = raw_7 -- expect-error
if type(raw_7.id) == "string" then
-- case 360: single field guard admits guarded scalar
    local raw_id_7: string = raw_7.id
-- case 361: single field guard does not prove whole record
    local raw_profile_7: Profile = raw_7 -- expect-error
end
local raw_8: any = {id = "raw8", count = 8, flag = true, tags = {source = "raw"}}
-- case 362: untrusted any cannot prove record
local trusted_raw_8: Profile = raw_8 -- expect-error
if type(raw_8.id) == "string" then
-- case 363: single field guard admits guarded scalar
    local raw_id_8: string = raw_8.id
-- case 364: single field guard does not prove whole record
    local raw_profile_8: Profile = raw_8 -- expect-error
end
local raw_9: any = {id = "raw9", count = 9, flag = true, tags = {source = "raw"}}
-- case 365: untrusted any cannot prove record
local trusted_raw_9: Profile = raw_9 -- expect-error
if type(raw_9.id) == "string" then
-- case 366: single field guard admits guarded scalar
    local raw_id_9: string = raw_9.id
-- case 367: single field guard does not prove whole record
    local raw_profile_9: Profile = raw_9 -- expect-error
end
local raw_10: any = {id = "raw10", count = 10, flag = true, tags = {source = "raw"}}
-- case 368: untrusted any cannot prove record
local trusted_raw_10: Profile = raw_10 -- expect-error
if type(raw_10.id) == "string" then
-- case 369: single field guard admits guarded scalar
    local raw_id_10: string = raw_10.id
-- case 370: single field guard does not prove whole record
    local raw_profile_10: Profile = raw_10 -- expect-error
end

return "ok"

-- character/regen.lua — HP/MP regeneration formulas
-- Java reference: HpRegeneration.java, MpRegeneration.java

-- HP regen interval table (seconds between regen events).
-- Index: 1=Lv1, 2=Lv2, ..., 10=Lv18+. Lower = faster regen.
-- Java: lvlTable = {30,25,20,16,14,12,11,10,9,3}
local hp_regen_interval_table = {30, 25, 20, 16, 14, 12, 11, 10, 9, 3}

-- Knight class type constant
local CLASS_KNIGHT = 1

-- get_hp_regen_interval(level, class_type) -> seconds
-- Returns seconds between HP regen events.
-- Knight Lv30+ gets 2 seconds (fastest tier).
function get_hp_regen_interval(level, class_type)
    -- Knight Lv30+ special case
    if level >= 30 and class_type == CLASS_KNIGHT then
        return 2
    end
    local idx = level
    if idx < 1 then idx = 1 end
    if idx > 10 then idx = 10 end
    return hp_regen_interval_table[idx]
end

-- calc_hp_regen_amount(ctx) -> {amount}
-- ctx = {level, con, hpr, food, weight_pct, has_exotic_vitalize, has_additional_fire,
--        has_golden_wing, has_berserkers}
-- weight_pct = Weight242 value (0-242 scale)
--
-- Java HpRegeneration.regenHp (Eddy) 第 127-134 行：
--   if (food < 3 || isOverWeight(pc) || hasSkillEffect(BERSERKERS)) { bonus = 0; equipHpr = min(equipHpr, 0) }
-- isOverWeight 第 198-209 行：EXOTIC_VITALIZE / ADDITIONAL_FIRE / 裝備 20049 視為未超重，
--   否則 121 <= getWeight242()。
--   CON bonus: Lv12+, CON >= 14: random(CON-12)+1, cap 14
function calc_hp_regen_amount(ctx)
    -- 負重判定（HP 門檻 121）
    local overweight = ctx.weight_pct >= 121
    if ctx.has_exotic_vitalize or ctx.has_additional_fire or ctx.has_golden_wing then
        overweight = false
    end

    -- 食物、負重、狂暴任一成立即停止基礎回復（Java 三者為 OR，彼此獨立）
    local blocked = ctx.food < 3 or overweight or (ctx.has_berserkers == true)

    local bonus = 0
    local equip_hpr = ctx.hpr or 0

    if blocked then
        -- Blocked: zero base regen, zero positive equipment bonus
        -- Negative equipment HPR still applies (cursed items)
        if equip_hpr > 0 then
            equip_hpr = 0
        end
    else
        -- CON bonus: only Lv12+, CON >= 14
        local max_bonus = 1
        if ctx.level > 11 and ctx.con >= 14 then
            max_bonus = ctx.con - 12
            if max_bonus > 14 then
                max_bonus = 14
            end
        end
        bonus = math.random(1, max_bonus)
    end

    return { amount = bonus + equip_hpr }
end

-- calc_mp_regen_amount(ctx) -> {amount}
-- ctx = {wis, mpr, food, weight_pct, has_exotic_vitalize, has_additional_fire, has_blue_potion}
--
-- Java MpRegeneration.regenMp (Eddy) 第 104-109 行：
--   if (food < 3 || isOverWeight(pc)) { baseMpr = 0; itemMpr = min(itemMpr, 0) }
-- isOverWeight 第 118-126 行：EXOTIC_VITALIZE / ADDITIONAL_FIRE 視為未超重，
--   否則 120 <= getWeight242()（MP 門檻 120，與 HP 的 121 不同；MP 沒有 20049 例外）。
--   WIS 15-16 → 2, WIS >= 17 → 3, else 1
--   Blue Potion bonus: effective WIS min 11, +WIS-10
function calc_mp_regen_amount(ctx)
    local overweight = ctx.weight_pct >= 120
    if ctx.has_exotic_vitalize or ctx.has_additional_fire then
        overweight = false
    end

    -- 食物與負重彼此獨立（Java 為 OR）
    local blocked = ctx.food < 3 or overweight

    local base_mpr = 0
    local equip_mpr = ctx.mpr or 0

    if blocked then
        if equip_mpr > 0 then
            equip_mpr = 0
        end
    else
        -- WIS-based MP regen
        local wis = ctx.wis
        if wis >= 17 then
            base_mpr = 3
        elseif wis >= 15 then
            base_mpr = 2
        else
            base_mpr = 1
        end

        -- Blue Potion bonus (skill 1002)
        if ctx.has_blue_potion then
            local eff_wis = wis
            if eff_wis < 11 then
                eff_wis = 11
            end
            base_mpr = base_mpr + (eff_wis - 10)
        end
    end

    return { amount = base_mpr + equip_mpr }
end

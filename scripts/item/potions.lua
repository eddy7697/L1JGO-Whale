-- Potion effect definitions
-- Go engine calls get_potion_effect(item_id) to get potion type and parameters
--
-- All values verified against l1j_java Potion.java + C_ItemUSe.java (Taiwan 3.80C).
-- DECAY_POTION (skill 71) blocks all drinkable potions — checked in Go handler.

POTIONS = {
    -- ========== Heal potions ==========
    -- Java: Potion.UseHeallingPotion(pc, item, healHp, gfxid)
    -- gfx: 189 = small blue sparkle, 194 = medium green sparkle, 197 = large gold sparkle
    -- Go handler applies Gaussian random ±20%: amount *= (gaussian/5 + 1)

    -- Basic heal potions
    [40010]  = { type = "heal", amount = 15,  gfx = 189 },  -- 治癒藥水
    [140010] = { type = "heal", amount = 25,  gfx = 189 },  -- 受祝福的治癒藥水
    [240010] = { type = "heal", amount = 10,  gfx = 189 },  -- 受咀咒的治癒藥水
    [40011]  = { type = "heal", amount = 45,  gfx = 194 },  -- 強力治癒藥水
    [140011] = { type = "heal", amount = 55,  gfx = 194 },  -- 受祝福的強力治癒藥水
    [40012]  = { type = "heal", amount = 75,  gfx = 197 },  -- 終極治癒藥水
    [140012] = { type = "heal", amount = 85,  gfx = 197 },  -- 受祝福的終極治癒藥水
    [40029]  = { type = "heal", amount = 15,  gfx = 189 },  -- 象牙塔治癒藥水

    -- Concentrated potions (same as base potions — Java values)
    [40019]  = { type = "heal", amount = 15,  gfx = 189 },  -- 濃縮體力恢復劑
    [40020]  = { type = "heal", amount = 45,  gfx = 194 },  -- 濃縮強力體力恢復劑
    [40021]  = { type = "heal", amount = 75,  gfx = 197 },  -- 濃縮終極體力恢復劑

    -- Ancient potions (lower than base — Java values)
    [40022]  = { type = "heal", amount = 20,  gfx = 189 },  -- 古代體力恢復劑
    [40023]  = { type = "heal", amount = 30,  gfx = 194 },  -- 古代強力體力恢復劑
    [40024]  = { type = "heal", amount = 55,  gfx = 197 },  -- 古代終極體力恢復劑

    -- Food / special heal items
    [40026]  = { type = "heal", amount = 25,  gfx = 189 },  -- 香蕉汁
    [40027]  = { type = "heal", amount = 25,  gfx = 189 },  -- 橘子汁
    [40028]  = { type = "heal", amount = 25,  gfx = 189 },  -- 蘋果汁
    [40058]  = { type = "heal", amount = 30,  gfx = 189 },  -- 煙燻的麵包屑
    [40071]  = { type = "heal", amount = 70,  gfx = 197 },  -- 烤焦的麵包屑
    [40506]  = { type = "heal", amount = 70,  gfx = 197 },  -- 安特的水果
    [140506] = { type = "heal", amount = 80,  gfx = 197 },  -- 受祝福的安特的水果
    [40734]  = { type = "heal", amount = 50,  gfx = 189 },  -- 信賴貨幣
    [40043]  = { type = "heal", amount = 600, gfx = 189 },  -- 兔子的肝
    [41403]  = { type = "heal", amount = 300, gfx = 189 },  -- 庫傑的糧食
    [41337]  = { type = "heal", amount = 85,  gfx = 197 },  -- 受祝福的五穀麵包
    [49268]  = { type = "heal", amount = 30,  gfx = 189 },  -- 愛瑪伊的畫像
    [49269]  = { type = "heal", amount = 30,  gfx = 189 },  -- 伊森之畫像
    [41298]  = { type = "heal", amount = 4,   gfx = 189 },  -- 鱈魚
    [41299]  = { type = "heal", amount = 15,  gfx = 194 },  -- 虎斑帶魚
    [41300]  = { type = "heal", amount = 35,  gfx = 197 },  -- 鮪魚
    [41411]  = { type = "heal", amount = 10,  gfx = 189 },  -- 銀のチョンズ
    [47114]  = { type = "heal", amount = 75,  gfx = 197 },  -- 凝聚的化合物
    [49137]  = { type = "heal", amount = 75,  gfx = 197 },  -- 鮮奶油蛋糕
    [41141]  = { type = "heal", amount = 75,  gfx = 197 },  -- 神秘的體力藥水
    [41417]  = { type = "heal", amount = 90,  gfx = 197 },  -- 刨冰1
    [41418]  = { type = "heal", amount = 90,  gfx = 197 },  -- 刨冰2
    [41419]  = { type = "heal", amount = 90,  gfx = 197 },  -- 刨冰3
    [41420]  = { type = "heal", amount = 90,  gfx = 197 },  -- 刨冰4
    [41421]  = { type = "heal", amount = 90,  gfx = 197 },  -- 刨冰5

    -- ========== Mana potions ==========
    -- Java: Potion.UseMpPotion(pc, item, mpAmount, range)
    -- range > 0: mp = Random.nextInt(range) + mpAmount  (random range)
    -- range == 0: mp = mpAmount (fixed)
    -- gfx = 190, msg = 338

    [40042]  = { type = "mana", amount = 50, range = 0  },  -- 精神藥水 (fixed 50)
    [41142]  = { type = "mana", amount = 50, range = 0  },  -- 神秘的魔力藥水 (fixed 50)
    [40735]  = { type = "mana", amount = 60, range = 0  },  -- 勇氣貨幣 (fixed 60)
    [41404]  = { type = "mana", amount = 80, range = 21 },  -- 庫傑的靈藥 (80 + rand(21))
    [40066]  = { type = "mana", amount = 7,  range = 6  },  -- 年糕 (7 + rand(6))
    [41413]  = { type = "mana", amount = 7,  range = 6  },  -- 月餅 (7 + rand(6))
    [40067]  = { type = "mana", amount = 15, range = 16 },  -- 艾草年糕 (15 + rand(16))
    [41414]  = { type = "mana", amount = 15, range = 16 },  -- 福月餅 (15 + rand(16))
    [41412]  = { type = "mana", amount = 5,  range = 16 },  -- 金粽子 (5 + rand(16))

    -- ========== Blue potions (MP regen buff) — gfx 190 ==========
    -- Java: Potion.useBluePotion → STATUS_BLUE_POTION (1002)
    [40015]  = { type = "blue_potion", duration = 600,  gfx = 190 },  -- 藍色藥水 (10 min)
    [140015] = { type = "blue_potion", duration = 1200, gfx = 190 },  -- 受祝福的藍色藥水（Eddy Potion.java）
    [40736]  = { type = "blue_potion", duration = 600,  gfx = 190 },  -- 智慧貨幣
    [49306]  = { type = "blue_potion", duration = 2400, gfx = 190 },  -- 福利藍色藥水 (40 min)

    -- ========== Haste (speed) potions — gfx 191 ==========
    -- Java: Potion.useGreenPotion → STATUS_HASTE (1001)
    -- Clears HASTE/GREATER_HASTE/SLOW/MASS_SLOW/ENTANGLE conflicts
    [40013]  = { type = "haste", duration = 300,  gfx = 191 },  -- 自我加速藥水 (5 min)
    [140013] = { type = "haste", duration = 350,  gfx = 191 },  -- 受祝福的自我加速藥水
    [40018]  = { type = "haste", duration = 1800, gfx = 191 },  -- 強化自我加速藥水 (30 min)
    [140018] = { type = "haste", duration = 2100, gfx = 191 },  -- 受祝福的強化自我加速藥水
    [40030]  = { type = "haste", duration = 300,  gfx = 191 },  -- 象牙塔加速藥水
    [40039]  = { type = "haste", duration = 600,  gfx = 191 },  -- 紅酒 (10 min)
    [40040]  = { type = "haste", duration = 900,  gfx = 191 },  -- 威士忌 (15 min)
    [49302]  = { type = "haste", duration = 1200, gfx = 191 },  -- 福利加速藥水 (20 min)
    [41338]  = { type = "haste", duration = 1800, gfx = 191 },  -- 受祝福的葡萄酒 (30 min)
    [41342]  = { type = "haste", duration = 1800, gfx = 191 },  -- 梅杜莎之血 (30 min)
    [49140]  = { type = "haste", duration = 1800, gfx = 191 },  -- 綠茶蛋糕卷 (30 min)
    -- Shop food haste items (very short)
    [41261]  = { type = "haste", duration = 30,   gfx = 191 },  -- 飯團
    [41262]  = { type = "haste", duration = 30,   gfx = 191 },  -- 雞肉串燒
    [41268]  = { type = "haste", duration = 30,   gfx = 191 },  -- 小比薩
    [41269]  = { type = "haste", duration = 30,   gfx = 191 },  -- 烤玉米
    [41271]  = { type = "haste", duration = 30,   gfx = 191 },  -- 爆米花
    [41272]  = { type = "haste", duration = 30,   gfx = 191 },  -- 甜不辣
    [41273]  = { type = "haste", duration = 30,   gfx = 191 },  -- 鬆餅

    -- ========== Brave potions — gfx 751 ==========
    -- Java: Potion.Brave → buff_brave → STATUS_BRAVE(1000) / STATUS_ELFBRAVE(1016)
    -- brave_type: 1 = brave (atk speed 1.33x), 3 = elf brave (atk speed 1.15x)
    -- class_restrict: "knight" / "elf" / "crown" / "notDKIL" / "DKIL" / nil (no restriction)

    -- Knight only (brave type 1)
    [40014]  = { type = "brave", duration = 300,  brave_type = 1, gfx = 751, class_restrict = "knight" },   -- 勇敢藥水 (5 min)
    [140014] = { type = "brave", duration = 350,  brave_type = 1, gfx = 751, class_restrict = "knight" },   -- 受祝福的勇敢藥水
    [41415]  = { type = "brave", duration = 1800, brave_type = 1, gfx = 751, class_restrict = "knight" },   -- 強化勇氣的藥水 (30 min)
    [49305]  = { type = "brave", duration = 1200, brave_type = 1, gfx = 751, class_restrict = "knight" },   -- 福利勇敢藥水 (20 min)

    -- Crown/Royal only (brave type 1)
    [40031]  = { type = "brave", duration = 600,  brave_type = 1, gfx = 751, class_restrict = "crown" },    -- 惡魔之血 (10 min)

    -- Everyone except DragonKnight & Illusionist (brave type 1)
    [40733]  = { type = "brave", duration = 600,  brave_type = 1, gfx = 751, class_restrict = "notDKIL" },  -- 名譽貨幣 (10 min)

    -- Elf only (brave type 3)
    [40068]  = { type = "brave", duration = 480,  brave_type = 3, gfx = 751, class_restrict = "elf" },      -- 精靈餅乾 (8 min)
    [140068] = { type = "brave", duration = 700,  brave_type = 3, gfx = 751, class_restrict = "elf" },      -- 受祝福的精靈餅乾
    [49304]  = { type = "brave", duration = 1920, brave_type = 3, gfx = 751, class_restrict = "elf" },      -- 福利森林藥水 (32 min)

    -- DragonKnight & Illusionist only — STATUS_RIBRAVE (brave type 5)
    [49158]  = { type = "brave", duration = 480,  brave_type = 5, gfx = 751, class_restrict = "DKIL" },     -- 生命之樹果實 (8 min)

    -- ========== Wisdom potions (SP+2) — gfx 750 ==========
    -- Java: Potion.useWisdomPotion → STATUS_WISDOM_POTION (1004)
    -- Wizard only; non-wizard: msg 79, NOT consumed
    [40016]  = { type = "wisdom", duration = 300,  gfx = 750, sp = 2 },   -- 慎重藥水 (5 min)
    [140016] = { type = "wisdom", duration = 600,  gfx = 750, sp = 2 },   -- 受祝福的慎重藥水（Eddy Potion.java）
    [49307]  = { type = "wisdom", duration = 1000, gfx = 750, sp = 2 },   -- 福利慎重藥水 (~16.7 min)

    -- ========== Eva breath potions (underwater) ==========
    -- Java: Potion.useBlessOfEva → STATUS_UNDERWATER_BREATH (1003)
    -- Duration STACKS (existing + new), capped at 7200 seconds max.
    [40032]  = { type = "eva_breath", duration = 1800, gfx = 190 },  -- 伊娃的祝福 (30 min)
    [40041]  = { type = "eva_breath", duration = 300,  gfx = 190 },  -- 人魚之鱗 (5 min)
    [41344]  = { type = "eva_breath", duration = 2100, gfx = 190 },  -- 水中的水 (35 min)
    [49303]  = { type = "eva_breath", duration = 7200, gfx = 190 },  -- 福利呼吸藥水 (2 hr max)

    -- ========== Third speed potion (3段加速) ==========
    -- Java: Potion.ThirdSpeed → STATUS_THIRD_SPEED (1027)
    -- gfx 7976, sends S_Liquor(8), msg 1065
    [49138]  = { type = "third_speed", duration = 600, gfx = 7976 },  -- 巧克力蛋糕 (10 min)

    -- ========== Blind potion (curse) ==========
    -- Java: Potion.useBlindPotion → CURSE_BLIND, fixed 16 seconds
    [40025]  = { type = "blind", duration = 16 },  -- 黑色藥水

    -- ========== Antidote (cure poison) ==========
    [40017]  = { type = "cure_poison", gfx = 192 },  -- 翡翠藥水
    [40507]  = { type = "cure_poison", gfx = 192 },  -- 解毒藥水 (variant)
}

function get_potion_effect(item_id)
    return POTIONS[item_id]
end

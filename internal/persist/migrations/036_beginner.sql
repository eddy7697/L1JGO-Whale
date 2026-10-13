-- +goose Up
-- Java `beginner` 資料表：新角色建立時依 activate ('A'=全職業, P/K/E/W/D/R/I=職業) 發放初始物品。
-- 伺服器每次建角直接查詢本表，後台修改資料列即時生效，無需重啟。
CREATE TABLE IF NOT EXISTS beginner (
    id           SERIAL PRIMARY KEY,
    item_id      INTEGER NOT NULL DEFAULT 0,
    count        INTEGER NOT NULL DEFAULT 0,
    charge_count INTEGER NOT NULL DEFAULT 0,
    enchantlvl   INTEGER NOT NULL DEFAULT 0,
    item_name    VARCHAR(50) NOT NULL DEFAULT '',
    activate     CHAR(1) NOT NULL DEFAULT 'A',
    bless        INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS beginner_activate_idx ON beginner(activate);

-- Eddy l1jtw_InnoDB.sql 原版 35 列
INSERT INTO beginner (id, item_id, count, charge_count, enchantlvl, item_name, activate, bless) VALUES
(1, 40005, 1, 0, 0, '蠟燭', 'A', 1),
(2, 40005, 1, 0, 0, '蠟燭', 'A', 1),
(3, 40641, 1, 0, 0, '說話卷軸', 'A', 1),
(4, 35, 1, 0, 0, '象牙塔單手劍', 'K', 1),
(5, 48, 1, 0, 0, '象牙塔雙手劍', 'K', 1),
(6, 147, 1, 0, 0, '象牙塔斧頭', 'K', 1),
(7, 105, 1, 0, 0, '象牙塔長矛', 'K', 1),
(8, 174, 1, 0, 0, '象牙塔石弓', 'K', 1),
(9, 7, 1, 0, 0, '象牙塔短劍', 'K', 1),
(10, 49309, 1, 0, 0, '象牙塔箭筒', 'K', 1),
(11, 35, 1, 0, 0, '象牙塔單手劍', 'P', 1),
(12, 48, 1, 0, 0, '象牙塔雙手劍', 'P', 1),
(13, 147, 1, 0, 0, '象牙塔斧頭', 'P', 1),
(14, 7, 1, 0, 0, '象牙塔短劍', 'P', 1),
(15, 35, 1, 0, 0, '象牙塔單手劍', 'E', 1),
(16, 175, 1, 0, 0, '象牙塔長弓', 'E', 1),
(17, 174, 1, 0, 0, '象牙塔石弓', 'E', 1),
(18, 7, 1, 0, 0, '象牙塔短劍', 'E', 1),
(19, 49309, 1, 0, 0, '象牙塔箭筒', 'E', 1),
(20, 35, 1, 0, 0, '象牙塔單手劍', 'W', 1),
(21, 224, 1, 0, 0, '象牙塔魔杖', 'W', 1),
(22, 7, 1, 0, 0, '象牙塔短劍', 'W', 1),
(23, 35, 1, 0, 0, '象牙塔單手劍', 'D', 1),
(24, 174, 1, 0, 0, '象牙塔石弓', 'D', 1),
(25, 73, 1, 0, 0, '象牙塔雙刀', 'D', 1),
(26, 156, 1, 0, 0, '象牙塔鋼爪', 'D', 1),
(27, 7, 1, 0, 0, '象牙塔短劍', 'D', 1),
(28, 49309, 1, 0, 0, '象牙塔箭筒', 'D', 1),
(29, 35, 1, 0, 0, '象牙塔單手劍', 'R', 1),
(30, 48, 1, 0, 0, '象牙塔雙手劍', 'R', 1),
(31, 147, 1, 0, 0, '象牙塔斧頭', 'R', 1),
(32, 147, 1, 0, 0, '象牙塔斧頭', 'I', 1),
(33, 174, 1, 0, 0, '象牙塔石弓', 'I', 1),
(34, 224, 1, 0, 0, '象牙塔魔杖', 'I', 1),
(35, 49309, 1, 0, 0, '象牙塔箭筒', 'I', 1)
ON CONFLICT (id) DO NOTHING;
SELECT setval('beginner_id_seq', GREATEST((SELECT MAX(id) FROM beginner), 1));

-- +goose Down
DROP TABLE IF EXISTS beginner;

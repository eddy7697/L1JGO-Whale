# 新手初始物品（beginner 資料表）

更新：2026-10-13。實作提交：`ca269f8`（分支 `codex/beginner-items`，已併入 `master`）。

本文件記錄 Go 版「建角初始物品」的資料表、伺服器邏輯與 Java 對照，並列出後續後台（admin）管理此功能時的規劃與注意事項。

## 對照來源

以 `E:\L1JTW-Eddy` 為準：

| Java 來源 | 用途 |
|---|---|
| `src/l1j/server/server/model/Beginner.java` | `GiveItem(pc)`：查 `beginner` 表並逐列寫入 `character_items` |
| `src/l1j/server/server/clientpackets/C_CreateChar.java` | 建角流程中呼叫 `Beginner.getInstance().GiveItem(pc)` |
| `db/l1jtw_InnoDB.sql` | `beginner` 表結構與原版 35 列資料 |

## Go 實作位置

| 檔案 | 內容 |
|---|---|
| `internal/persist/migrations/036_beginner.sql` | 建表、索引、seed 35 列 |
| `internal/persist/beginner_repo.go` | `BeginnerRepo`：`LoadForClass`、`GiveItems`，以及純函式 `beginnerActivateCode`、`beginnerInsertValues` |
| `internal/handler/createchar.go` | 角色寫入成功後呼叫 `GiveItems` |
| `internal/handler/context.go`、`cmd/l1jgo/main.go` | `Deps.BeginnerRepo` 接線 |
| `internal/persist/beginner_repo_test.go` | 單元測試與選擇性 PostgreSQL 整合測試 |

## 資料表結構

PostgreSQL 表 `beginner`，欄位名稱與 Java MySQL 版相同：

| 欄位 | 型別 | 預設 | 說明 |
|---|---|---|---|
| `id` | SERIAL PK | 自動 | 列編號，決定發放順序 |
| `item_id` | INTEGER | 0 | 物品模板 ID，需存在於 Go 物品資料 |
| `count` | INTEGER | 0 | 數量 |
| `charge_count` | INTEGER | 0 | 充能次數（魔杖等） |
| `enchantlvl` | INTEGER | 0 | 強化值 |
| `item_name` | VARCHAR(50) | '' | 僅供後台辨識，伺服器邏輯不使用 |
| `activate` | CHAR(1) | 'A' | 發放對象：`A` 全職業，或下表職業碼 |
| `bless` | INTEGER | 1 | 保留欄位，伺服器不讀取（見「與 Java 的差異」） |

索引：`beginner_activate_idx (activate)`。

### 職業碼

| `characters.class_type` | 職業 | `activate` |
|---|---|---|
| 0 | 王族 | P |
| 1 | 騎士 | K |
| 2 | 精靈 | E |
| 3 | 法師 | W |
| 4 | 黑暗精靈 | D |
| 5 | 龍騎士 | R |
| 6 | 幻術師 | I |
| 其他 | 無 | 回退為 A |

`characters` 表有 `valid_class_type` 限制（0 到 6），回退分支只在防禦性情境發生。

## 伺服器邏輯

建角流程（`HandleCreateChar`）在 `characters` 寫入成功並取得角色 ID 後執行：

1. 查詢 `SELECT ... FROM beginner WHERE activate IN ('A', <職業碼>) ORDER BY id`。
2. 每列寫入一筆 `character_items`，值如下。
3. 單列失敗記錄 error 並繼續下一列。整體失敗也只記錄，建角不會失敗。
4. 最後記錄一筆 info：角色名、職業、實際寫入筆數。

寫入 `character_items` 的值：

| 欄位 | 值 | 來源 |
|---|---|---|
| `char_id` | 新角色 ID | 建角結果 |
| `item_id` | `beginner.item_id` | 資料表 |
| `count` | `beginner.count` | 資料表 |
| `enchant_lvl` | `beginner.enchantlvl` | 資料表 |
| `charge_count` | `beginner.charge_count` | 資料表 |
| `bless` | 1（祝福狀態 1 = 一般） | Java 固定值 |
| `equipped` | false | Java `is_equipped=0` |
| `identified` | false | Java `is_id=0`，未鑑定 |
| `durability` | 0 | Java 固定值 |
| `equip_slot` | 0 | 未裝備 |
| `obj_id` | 0 | 進入世界時由伺服器配發 |

物品在角色第一次進入世界時由 `loadInventoryFromDB` 載入。若 `item_id` 不存在於 Go 物品資料，該列會被略過不顯示，但資料列仍留在 `character_items`。

**不需重啟**：伺服器每次建角都直接查資料庫，後台修改 `beginner` 後下一個新角色立即生效。沒有快取，也沒有 reload 指令。

## 與 Java 的差異

- Java 會把 `item_name` 一併寫入 `character_items.item_name`。Go 的 `character_items` 沒有此欄位，`beginner.item_name` 只供後台閱讀。
- Java 與 Go 都忽略 `beginner.bless`，實際寫入固定 1。保留此欄位是為了與 Java 結構一致，後台若要讓它生效需同步修改 `beginnerInsertValues`。
- Java 以 `IdFactory` 配發物品 ID；Go 由 `character_items.id` 自增，`obj_id` 於進入世界時配發。
- Eddy 的後續 migration 另外新增自訂物品 600001「自動練功機器人」與 600002「背包整理券」。Go 版這兩個 ID 是力量符石且對應功能不存在，故未 seed。

## 原版 seed 內容

全職業（`A`）：蠟燭 40005 兩列、說話卷軸 40641。

| 職業 | 物品 |
|---|---|
| 騎士 K | 象牙塔單手劍 35、雙手劍 48、斧頭 147、長矛 105、石弓 174、短劍 7、箭筒 49309 |
| 王族 P | 單手劍 35、雙手劍 48、斧頭 147、短劍 7 |
| 精靈 E | 單手劍 35、長弓 175、石弓 174、短劍 7、箭筒 49309 |
| 法師 W | 單手劍 35、魔杖 224、短劍 7 |
| 黑暗精靈 D | 單手劍 35、石弓 174、雙刀 73、鋼爪 156、短劍 7、箭筒 49309 |
| 龍騎士 R | 單手劍 35、雙手劍 48、斧頭 147 |
| 幻術師 I | 斧頭 147、石弓 174、魔杖 224、箭筒 49309 |

Seed 使用 `ON CONFLICT (id) DO NOTHING`，重跑 migration 不會覆蓋後台已修改的列；migration 之後會把序列調到現有最大 `id`。

## 目前的管理方式

尚無 GM 指令或後台介面。直接以 SQL 操作：

```sql
-- 查看
SELECT * FROM beginner ORDER BY id;

-- 新增：所有職業送 10 個紅色藥水
INSERT INTO beginner (item_id, count, item_name, activate) VALUES (40010, 10, '紅色藥水', 'A');

-- 修改數量
UPDATE beginner SET count = 5 WHERE id = 36;

-- 刪除
DELETE FROM beginner WHERE id = 36;
```

修改後立即生效，只影響之後建立的角色。已存在的角色不會補發。

## 後台（admin）功能規劃

以下是日後開發後台時，此功能可提供的管理面向。目前皆未實作。

### 必要

- **列表與編輯**：對 `beginner` 的 CRUD。`activate` 以下拉選單限制為 `A`、`P`、`K`、`E`、`W`、`D`、`R`、`I`。
- **物品驗證**：儲存前確認 `item_id` 存在於 Go 物品資料（`data.ItemTable.Get`），並自動帶入 `item_name`。避免寫入不存在的 ID 造成進入世界時被略過。
- **數值限制**：`count` 不小於 1；非堆疊物品 `count` 固定 1（多件請用多列）。`enchantlvl`、`charge_count` 不小於 0。
- **職業預覽**：依職業碼顯示該職業實際會拿到的完整清單（`A` 加該職業碼），順序依 `id`。

### 建議

- **匯入匯出**：以 CSV 或 JSON 匯出整表，便於換版本時搬移。
- **變更紀錄**：記錄誰在何時改了哪一列，可共用後台既有的稽核機制。
- **補發工具**：對既有角色依目前資料表補發（Java 沒有此功能）。需在角色離線時操作，或透過伺服器 API 走現有背包流程，避免與線上背包存檔衝突。
- **啟用開關**：全表或單列的停用旗標。目前表無此欄位，加入時需同步修改 `LoadForClass` 的查詢條件。

### 技術注意事項

- 伺服器與後台共用同一個 PostgreSQL，後台直接寫表即可，不需通知伺服器。
- 若日後改成伺服器端快取，必須加入 reload 機制（可比照 `.cpshopreload` 的 `CustomPointShopCache` 模式）。
- `bless` 欄位目前不生效。若後台提供祝福、詛咒選項，須同步修改 `beginnerInsertValues` 並補測試。
- `obj_id` 不由後台處理；寫入 0 即可。

## 驗證方式

```powershell
# 純函式單元測試
go test ./internal/persist/ -run 'TestBeginner'

# 整合測試：在獨立 schema 跑完整 migration，驗證 seed 與寫入值；不碰正式 schema
$env:L1JGO_TEST_BEGINNER_DSN = "postgres://postgres:<password>@localhost:5432/l1jgo?sslmode=disable"
go test ./internal/persist/ -run 'TestBeginnerPostgresGiveItems' -v
```

實機驗證：建立一名騎士後登入，背包應有蠟燭 2、說話卷軸 1 與 7 件象牙塔武器，皆為未鑑定狀態。

# GM 身分與指令

更新：2026-10-13。功能分支：`codex/gm-identity-commands`。

## 對照來源與範圍

以 `E:\L1JTW-Eddy` 的 `MySqlCharacterStorage.java`、`C_Chat.java`、`GMCommands.java`、`command/executor/`、基準資料庫 `commands` 表與 `GMCommands.xml` 為準。基準表有 116 個中英文名稱、58 種 executor；除來源缺少實作的 `crazy/狂暴` 外，名稱與最低權限均已對應到 Go。Go 原有測試指令亦保留，需管理權限。

另外補齊 Java 後續新增的 7 個管理 executor：`marquee`、`cpadd`、`cpquery`、`cpshopreload`、`advpolygive`、`advpolyunlock`、`advpolyreset`。加上保留的 Go 指令，共 90 項主指令。基準 `L1Save/L1System` 缺 executor 原始碼，Go 分別實作全玩家存檔與程序狀態。

本次範圍是 GM 身分與管理指令。自訂點數的玩家購買流程／金流 HTTP API，以及進階變身的玩家卡冊選單尚未移植；下列管理操作及持久化資料可獨立使用，不能據此宣稱上述玩家子系統已完成。

`crazy` 不列入可執行 help；呼叫時明確說明缺少 Java 實作，不回報成功。

## 建立 GM 或監察員

角色 `characters.access_level` 決定身分：

| 值 | 身分 | 行為 |
|---|---|---|
| 0 | 玩家 | 聊天中的點號不會取得管理權限 |
| 100 | 監察員 | 可用最低權限 ≤100 的指令，不會被怪物主動鎖定 |
| 200 | GM | 可用全部已實裝指令，具 Java GM 遊戲特權 |

其他數值不授予 GM／監察員身分。帳號的 access_level 不繼承到角色，新建角色維持玩家。

建立第一位 GM（角色須已建立；使用該伺服器的設定檔）：

```powershell
go run ./cmd/gm -config config/server.local.toml -name "角色名稱" -level 200
```

Docker 映像亦包含管理工具：

```powershell
docker compose exec l1jgo ./gm -name "角色名稱" -level 200
```

使用 100 設監察員、0 撤銷。CLI 修改後重新登入生效，不會更改帳號、密碼或其他角色。CLI 未指定 `-config` 時支援 `L1JGO_CONFIG`。

遊戲內管理員可用 `.setting 角色名稱 gm 100`、`.setting 角色名稱 gm 0`；先成功寫資料庫再套用線上狀態，立即生效。撤權會移除不再可用的 GM 隱身／偵測／血條／陷阱視圖。

## 遊戲特權與指令入口

- `.gm` 切換遊戲特權，`.gm on/off` 可明確設定；指令授權仍以角色權限為準，所以 off 後可以 on。
- Java GM 承受非致命傷，致命傷回滿 HP；扣 MP 時回滿 MP。關閉 GM 特權後恢復一般傷害／耗魔／怪物鎖定。
- `.invisible` 與 `.visible` 分別開／關 GM 隱身，重複執行不反轉。與隱身術分離，取消魔法與脫隱斗篷不會解除 GM 隱身。
- `.findinvis on` 可看普通隱身術；不能看見 GM 隱身，不能跨副本。
- `.help [頁碼]` 只列出角色有權使用的指令；`.help 指令` 查用法與中文別名。
- `.r` 重複前一指令，`.r 新參數` 替換參數；再次檢查權限。
- `.f set move % % %` 登記最愛，`.f 33442 32797 4` 代入並執行。禁止透過 f/r 或中文別名遞迴。
- 所有已授權執行的指令記錄角色 ID、名稱、權限與參數。
- 禁言阻擋一般聊天及密語；保留管理指令入口供解除禁言。世界頻道關閉時 GM 仍可使用。

## 常用操作

```text
.help
.help 2
.item 40010 10
.item 31 +7 1
.setting me str 35
.setting 角色名稱 hp 2000
.addskill
.buff me 43 3600
.allbuff 角色名稱
.clearallbuff 角色名稱
.topc 角色名稱
.recall 角色名稱
.gmroom Giran
.hpbar on
.invisible
.visible
.chatng 角色名稱 10
.chatng 角色名稱 0
.banip 192.0.2.10 add
.banip 192.0.2.10 del
.present 帳號 40010 0 10
.lvpresent 1 50 40010 0 10
.marquee 伺服器活動即將開始
.cpadd 帳號 MP 1000 活動贈點
.cpquery 帳號 MP
.cpshopreload
.advpolygive 角色名稱 600101
.advpolyunlock 角色名稱 13715
.advpolyreset 角色名稱
.save
.shutdown 60
.shutdown abort
```

`.buff 技能ID` 依 Java 套用附近玩家，對自己請指定 me。`.allbuff` 包含 Java 的 31 項輔助、加速與變身；可省略目標作用於自己。`.ress` 治療附近玩家，死亡玩家收到復活確認；保留 Go 的 `.rez [角色]` 直接復活。

Java 將中文「公告」映射到 echo，因此 `.公告 文字` 只回顯本人。贈品指令送往個人倉庫，依帳號去重；lvpresent 以角色等級選帳號。

`.marquee` 廣播到所有線上玩家（含不同地圖／副本）。客戶端需將 `string-c.tbl` 訊息 3440 設為 `%0`，才能依 Java 顯示完整文字。

`.cpadd` 需要已存在的帳號、啟用幣別、非零整數及稽核備註；支援負值扣點，餘額不得為負。預設幣別 MP。餘額和操作流水在同一交易提交；`.cpquery` 查詢餘額與最近 10 筆異動。`.cpshopreload` 從資料庫重載商店與啟用商品，任一讀取或驗證失敗時保留整份舊快取；資料庫不預設建立營運商店，且玩家端 CP 商城購買流程尚未接入。

進階變身管理指令只接受線上角色：give 發一張對應卡；unlock 增加收藏的解鎖次數；reset 清除收藏而不解除當前變身。移植 50 組卡片／變身資料，卡片 ID 為 600101–600150。Go 既有 600100 為涼鞋，因此保留原資料，沒有覆蓋為 Java 卡冊；玩家端卡片收錄、卡冊入口與選單仍需移植。

## 永久資料與部署

啟動時自動套用 migration 032–035：新增 IP 封鎖、城鎮結算、自訂點數／流水、進階變身收藏與點數商店目錄。既有角色身分欄位不需要補設預設 GM；本次未替任何真實角色授權。

`data/xml/GmCommands/GMCommands.xml` 保留 Java 的 84 個命名傳送點與 DKSet，可自行增修套裝及地點；下次啟動載入。

`.insert mob|npc NPCID` 先原子存檔，再生成 NPC。執行期新增點存於 `data/runtime/gm_spawn_list.yaml`；Docker 使用 `gmdata` volume，重建容器仍保留。此資料已排除 Git 與映像建置內容，需納入備份。副本內拒絕永久 insert。怪物沿用 Go 重生格式，範圍 12 格、重生 120 秒（Java 是 60–120 秒）；一般 NPC 不重生。

`.tospawn` 以 Java 生成點 ID 查詢，npc 表優先；舊版 `spawn_list.yaml` 未保留 ID，因此舊資料以 1-based 項次查詢，指令會明示此差異。SQL 匯出工具已補原始 id/source，重新匯出後可用原始 ID。

`.save` 儲存所有線上玩家，包括角色、物品、魔法、書籤、buff；任一步失敗不回報成功，保留待存狀態。解除禁言後即使 buff 為空，換角／登出也會清除資料庫舊狀態。`.shutdown` 經主迴圈存檔後退出；存檔失敗會取消 GM 關機。

`.hometown daily/monthly` 可執行城鎮稅率、營收、村長選舉與分紅結算；Go 目前沒有居民登記、貢獻累積、領取分紅的完整玩法，不能將此指令視為整個城鎮系統已完成。

## 指令表

別名與 canonical 共用同一權限檢查。最低權限 50/100 的指令實際可由 100、200 使用；150/200 由 200 使用。

| 指令 | 最低權限 | 別名 | 用法 |
|---|---:|---|---|
| `.echo` | 200 | 公告 | <文字>（回顯本人） |
| `.setting` | 200 | 設定 | <玩家\|me> <hp\|mp\|str\|dex\|con\|wis\|int\|cha\|ac\|mr\|hit\|dmg\|lawful\|karma\|gm> <值> |
| `.summon` | 150 | 召寵 | <NPC ID\|名稱> [數量] |
| `.cleaning` | 200 | 清除 | 無參數 |
| `.addskill` | 200 | 技能 allskill | 無參數 |
| `.level` | 150 | 等級 | <1-99> |
| `.loc` | 50 | 座標 pos coord | [玩家] |
| `.desc` | 200 | 詳情 | [玩家\|me] |
| `.who` | 100 | 玩家 | 無參數 |
| `.allbuff` | 200 | 全輔助 | [玩家\|me] |
| `.clearallbuff` | 200 | 清除輔助 clearbuff | [玩家\|me] |
| `.speed` | 50 | 加速 | [0-4] |
| `.adena` | 150 | 金幣 gold | <數量> |
| `.hpbar` | 50 | 血條 | <on\|off> |
| `.resettrap` | 200 | 重設陷阱 | 無參數 |
| `.reloadtrap` | 200 | 重讀陷阱 | 無參數 |
| `.showtrap` | 200 | 顯示陷阱 | <on\|off> |
| `.castgfx` | 200 | 施法動畫 | <動畫 ID> |
| `.gfxid` | 200 | 影像動畫 | <起始 ID> <數量> |
| `.invgfxid` | 200 | 背包影像 | <起始 ID> <數量> |
| `.hometown` | 150 | 村莊 | <daily\|monthly> |
| `.gm` | 150 | 遊戲管理員 | [on\|off] |
| `.present` | 200 | 贈送 | <帳號\|*> <物品 ID> <強化> <數量> |
| `.lvpresent` | 200 | 限等贈送 | <最低等級> <最高等級> <物品 ID> <強化> <數量> |
| `.shutdown` | 200 | 關機 | <秒數\|now\|abort> |
| `.item` | 150 | 物品 | <物品 ID\|名稱> [數量] [強化] |
| `.itemset` | 150 | 套裝 | <套裝名稱> |
| `.buff` | 100 | 輔助 | [me\|all] <技能 ID> [秒數] |
| `.patrol` | 50 | 查詢 | 無參數 |
| `.banip` | 150 | 封鎖IP | <IP> [add\|del] |
| `.chat` | 50 | 廣播頻道 | <on\|off> |
| `.chatng` | 50 | 禁言 | <玩家> <分鐘，0=解除> |
| `.skick` | 150 | skick1 | <玩家> |
| `.kick` | 50 | kick1 | <玩家> |
| `.powerkick` | 150 | powerkick1 | <玩家> |
| `.accbankick` | 150 | 封鎖帳號 | <玩家> |
| `.poly` | 150 | 變身 | <玩家> <變身 ID> 或 <變身 ID> [玩家] |
| `.ress` | 100 | 恢復 | 無參數 |
| `.death` | 200 | 死亡 | <玩家> |
| `.gmroom` | 50 | 房間 | <1-5\|房間名稱> |
| `.topc` | 50 | 前往玩家 goto | <玩家> |
| `.move` | 50 | 移動 warp teleport | <x> <y> [地圖] |
| `.weather` | 150 | 天氣 | <天氣代碼> |
| `.tospawn` | 200 | 前往怪物 | [生成點 ID\|+\|-] |
| `.f` | 50 | 書籤 | <set 指令 [參數]\|show\|代入參數> |
| `.recall` | 100 | 召回 | <玩家> |
| `.visible` | 50 | 解除隱身 | 無參數 |
| `.partyrecall` | 100 | 召團 | <玩家> |
| `.invisible` | 50 | 隱身 | 無參數 |
| `.spawn` | 100 | 召怪 | <NPC ID\|名稱> [數量] [範圍] |
| `.insert` | 200 | 創怪 | <mob\|npc> <NPC ID> |
| `.help` | 50 | 說明 | [頁碼\|指令] |
| `.action` | 200 | 動作 | <動作 ID> |
| `.tile` | 200 | tile1 | 無參數 |
| `.findinvis` | 200 | findinvis1 | <on\|off> |
| `.save` | 200 | 儲存 | 無參數 |
| `.system` | 200 | 系統 | 無參數 |
| `.marquee` | 200 | — | <訊息內容> |
| `.cpadd` | 200 | — | <帳號> [幣別] <正負點數> <備註> |
| `.cpquery` | 200 | — | <帳號> [幣別] |
| `.cpshopreload` | 200 | — | 無參數 |
| `.advpolygive` | 200 | — | <玩家> <變身 ID\|卡片 ID> |
| `.advpolyunlock` | 200 | — | <玩家> <變身 ID> |
| `.advpolyreset` | 200 | — | <玩家> |
| `.hp` | 200 | — | <HP> |
| `.mp` | 200 | — | <MP> |
| `.heal` | 200 | — | 無參數 |
| `.stat` | 200 | — | <屬性> <值> |
| `.lawful` | 200 | law l | <正負增量> [玩家] |
| `.spell` | 200 | — | <技能 ID\|0> |
| `.kill` | 200 | — | 無參數 |
| `.killall` | 200 | — | 無參數 |
| `.exp` | 200 | — | <經驗值> |
| `.class` | 200 | — | <0-6> |
| `.rez` | 200 | resurrect | [玩家] |
| `.ac` | 200 | — | 無參數 |
| `.polygfx` | 200 | — | <影像 ID> |
| `.undopoly` | 150 | — | [玩家] |
| `.wall` | 200 | — | [1\|2\|3] |
| `.clearwall` | 200 | — | 無參數 |
| `.poison` | 200 | — | [damage\|silence\|para] |
| `.broken` | 200 | — | [1-127] |
| `.water` | 200 | — | [on\|off] |
| `.stresstest` | 200 | — | <NPC ID> [數量] [半徑] |
| `.cleartest` | 200 | — | 無參數 |
| `.slottest` | 200 | — | <參數> |
| `.slotexpand` | 200 | — | <type> <value> |
| `.slotexpand2` | 200 | — | <type> <value> |
| `.time` | 200 | — | <時間> |
| `.r` | 50 | — | [替換參數] |

## 驗證

Go 回歸涵蓋身分邊界、權限拒絕、別名與重複指令、隱身／副本隔離、GM 戰鬥特權、指令解析、復活、生成／召喚、原子寫入／重載、禁言與關機倒數。

```powershell
go test ./... -timeout 180s
go build ./cmd/l1jgo ./cmd/gm
```

`scripts/test_gm_admin_sql.ps1` 在 PostgreSQL 15 使用隔離 schema 與測試資料驗證 migration、IP 封鎖、倉庫贈品、城鎮結算、空 buff 刪除及進階變身解鎖／重置，最後 ROLLBACK 並確認沒有遺留 schema。未操作正式角色資料。

`scripts/test_gm_points.ps1` 呼叫真正的點數 Repo，使用獨立 schema 驗證餘額／流水一起提交、餘額不足與溢位拒絕、流水寫入失敗全回滾、20 次併發加點、最新 10 筆查詢，完成後清除測試 schema。一般 `go test` 沒有指定測試資料庫時會跳過此項整合測試。

尚需以客戶端實機驗收 GM 視窗、血條、動畫、隱身及復活顯示。

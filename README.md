# L1JGO-Whale

天堂 (Lineage 1) 3.80C 遊戲伺服器 — 使用 Go 語言從零重寫。

以 L1J 3.80C Java 伺服器為行為參考，但採用全新的 Go 架構：ECS 實體系統、Lua 腳本驅動遊戲邏輯、YAML 靜態資料、PostgreSQL 持久化。

> **開發階段**：目前已完成至 Phase 5（技能 / Buff / NPC AI），功能持續開發中。

## 已實現功能

| 類別 | 功能 |
|------|------|
| **帳號系統** | 登入驗證、自動建帳、封鎖帳號、bcrypt 密碼 |
| **角色系統** | 建立 / 刪除 / 切換角色、配點、角色列表 |
| **世界系統** | 進入世界、AOI 視野管理、地圖傳送門 |
| **移動系統** | 8 方向移動、地圖碰撞檢測、速度驗證 |
| **戰鬥系統** | 近戰 / 遠程攻擊、Lua 傷害公式、PK 系統 |
| **NPC 系統** | NPC 生成 / 重生、對話系統、商店 NPC |
| **NPC AI** | 仇恨目標偵測、追擊 / 攻擊 / 遊走（Lua 驅動） |
| **道具系統** | 背包管理、裝備穿脫、消耗品、地面物品 |
| **商店系統** | 購買 / 販賣、多層稅率計算 |
| **倉庫系統** | 存入 / 取出、堆疊道具 |
| **交易系統** | 玩家間交易、WAL 經濟安全 |
| **衝裝系統** | 武器 / 防具衝裝、祝福保護、碎裂機制 |
| **技能系統** | 技能施放、魔法商店學習、技能書學習 |
| **Buff 系統** | 加速藥水、技能 Buff、定時到期 |
| **怪物技能** | 怪物魔法攻擊、遠程攻擊、技能觸發 |
| **組隊系統** | 建立 / 加入 / 離開隊伍、隊長轉移、小地圖位置同步 |
| **聊天系統** | 一般 / 廣播 / 密語 / 隊伍頻道 |
| **傳送系統** | NPC 傳送、記憶座標（書籤）、傳送門 |
| **死亡系統** | 玩家死亡、經驗懲罰、重新開始 |
| **GM 指令** | 等級 / 屬性 / 傳送 / 生成道具 / 踢人等 |
| **自動存檔** | 定時存檔角色資料、斷線自動保存 |

## 技術架構

```
Go Engine（網路層、ECS 實體系統、世界管理、持久化）
  + Lua Scripts（所有遊戲邏輯：戰鬥公式、AI、技能）
  + YAML（靜態遊戲資料：道具、NPC、技能、地圖、商店）
  + TOML（運行時設定：倍率、伺服器參數）
  + PostgreSQL（資料持久化、WAL 經濟安全機制）
```

| 技術 | 用途 |
|------|------|
| Go 1.23+ | 伺服器核心 |
| [gopher-lua](https://github.com/yuin/gopher-lua) | Lua 腳本引擎 |
| [pgx](https://github.com/jackc/pgx) | PostgreSQL 驅動 |
| [goose](https://github.com/pressly/goose) | 資料庫遷移 |
| [zap](https://go.uber.org/zap) | 結構化日誌 |
| [yaml.v3](https://gopkg.in/yaml.v3) | YAML 解析 |
| [toml](https://github.com/BurntSushi/toml) | TOML 設定 |
| [bcrypt](https://golang.org/x/crypto/bcrypt) | 密碼雜湊 |

## 快速開始

### Docker Compose（僅供開發環境使用）

專案根目錄的 `docker-compose.yml` 僅供開發環境使用，預設將 PostgreSQL 的 `5432` 埠開放至主機網路介面，不適合直接用於正式環境。

安裝並啟動 Docker 後，先在專案根目錄複製本機設定範本（已有 `.env` 時直接編輯，不要覆蓋）：

```powershell
Copy-Item .env.example .env
```

Linux / macOS 可使用 `cp .env.example .env`。編輯 `.env` 的 `POSTGRES_USER`、`POSTGRES_PASSWORD`、`POSTGRES_DB` 與 `POSTGRES_HOST_PORT`，再啟動服務：

```bash
docker compose up -d --build
```

`.env.example` 的密碼刻意留白，未填寫時 Compose 會拒絕啟動。密碼可用單引號包住（例如 `POSTGRES_PASSWORD='你的密碼'`），讓 `$`、`#` 與空白保持原值。Compose 會將同一組帳密提供給資料庫與遊戲伺服器，不必另外修改 TOML 中的帳密。

`.env`、`.env.*` 及 `config/*.local.toml` 已由 `.gitignore` 與 `.dockerignore` 排除；只有不含密碼的 `.env.example` 範本納入版控。Compose 啟動時讀取 `.env`，既有 shell 中同名的環境變數優先於 `.env`。檢查設定可用 `docker compose config --quiet`；不帶 `--quiet` 會輸出包含密碼的解析結果。

- PostgreSQL：主機連線預設使用 `localhost:5432`，可透過 `POSTGRES_HOST_PORT` 修改主機埠；帳號與資料庫名稱以 `.env` 為準。其他電腦可使用主機 IP 連線，須由防火牆與網路允許。容器間固定使用 `postgres:5432`。
- 遊戲伺服器：主機連線使用 `localhost:7002`，映射至容器的 `7001` 埠。
- 資料庫持久化：`pgdata` named volume 掛載至 `/var/lib/postgresql/data`，重啟或重建容器後資料仍會保留。資料由 Docker 管理，不在專案目錄內，因此不會被 Git 追蹤，無須加入 `.gitignore`。
- 血盟紋章持久化：專案內的 `./emblem/` 掛載至 `/app/emblem`，已在 `.gitignore` 與 `.dockerignore` 排除，避免提交或加入映像建構內容。

停止服務可執行 `docker compose down`，資料庫 volume 會保留。**`docker compose down -v` 會刪除資料庫 volume 與其中的資料**，僅在確定要清除開發資料時使用。

`POSTGRES_USER`、`POSTGRES_PASSWORD` 與 `POSTGRES_DB` 僅在資料 volume 首次初始化時建立帳號與資料庫。**既有 volume 不會因修改 `.env` 而自動變更帳密**；需先在資料庫內修改，再同步更新 `.env` 並重新執行 `docker compose up -d`。若要保留原資料，請勿用刪除 volume 的方式換密碼。舊版曾提交的密碼仍可能存在 Git 歷史中，此設定方式不會清除歷史紀錄。

以下為不使用 Docker Compose 時的本機啟動方式。

### 環境需求

- **Go** 1.23 以上
- **PostgreSQL** 14 以上
- **天堂 3.80C 客戶端**（台版）
- **地圖資料**（從客戶端 `.s32` 解析後的 `.txt` 檔，放置於 `map/` 目錄）

本專案目前的封包格式對應 `E:\lin-golang\Login38` 登入器，須啟用 `hp_mp_limit_enabled` 與 `ac_mr_limit_enabled`（Login38 預設皆為 `true`）。角色狀態封包的 HP／MP、AC，以及魔防更新的 MR 與洗點封包的 AC 使用 32 位元欄位。若關閉 AC／MR 擴充，客戶端與伺服器會從不同位置讀取遊戲時間，可能造成野外明暗交替及狀態顯示錯誤；角色列表與屬性抗性封包則維持各自原有格式。

### 1. 建立資料庫

```sql
CREATE DATABASE l1jgo;
```

### 2. 修改設定檔

將 `config/server.toml` 複製為 `config/server.local.toml`，再於這份不納入版控的檔案設定個人帳密：

```toml
[database]
dsn = "postgres://你的帳號:你的密碼@localhost:5432/l1jgo?sslmode=disable"

[network]
bind_address = "0.0.0.0:7001"  # 客戶端連線埠號

[rates]
exp_rate = 1      # 經驗值倍率（1 = 正常）
drop_rate = 1     # 掉寶倍率
gold_rate = 1     # 金幣倍率
```

完整設定說明請參考 [config/server.toml](config/server.toml)，每個欄位都有中文註解。也可保留 `dsn = "sslmode=disable"` 並在執行環境設定 `PGHOST`、`PGPORT`、`PGUSER`、`PGPASSWORD`、`PGDATABASE`。Go 執行檔不會自動載入 Compose 使用的 `.env`；TOML 中明確指定的連線欄位優先於 `PG*` 環境變數。

### 3. 編譯並啟動

```bash
go build -o l1jgo ./cmd/l1jgo
L1JGO_CONFIG=config/server.local.toml ./l1jgo
```

PowerShell：

```powershell
go build -o l1jgo.exe ./cmd/l1jgo
$env:L1JGO_CONFIG = "config/server.local.toml"
./l1jgo.exe
```

或使用 Makefile（同樣在專案根目錄執行）：

```bash
L1JGO_CONFIG=config/server.local.toml make run
```

### 4. 連線遊戲

將客戶端的伺服器位址指向 `127.0.0.1:7001`，啟動客戶端即可登入。

首次登入時輸入任意帳號密碼，伺服器會自動建立帳號（可在設定檔關閉此功能）。

## 啟動畫面

```
  ┌───────────────────────────────────────────┐
  │           L1JGO-Whale  v0.3.6             │
  │      天堂 3.80C · Go 遊戲伺服器           │
  └───────────────────────────────────────────┘

  伺服器: L1JGO-Whale (編號: 1)

  ── 資料庫 ───────────────────────────────────
  ✓ PostgreSQL 連線成功
  ✓ 資料庫遷移完成

  ── 資料載入 ─────────────────────────────────
  NPC 模板 ····························· 3428
  地圖資料 ······························ 599
  NPC 生成 ···························· 29407
  ...
  ✓ Lua 腳本載入完成

  ── 伺服器就緒 ───────────────────────────────
  ▶ 監聽位址 0.0.0.0:7001
  ▶ 遊戲迴圈啟動 (tick: 200ms)
```

## 專案結構

```
L1JGO-Whale/
├── server/
│   ├── cmd/l1jgo/main.go          # 程式進入點
│   ├── config/server.toml         # 伺服器設定檔
│   ├── data/yaml/                 # 靜態遊戲資料（YAML）
│   ├── scripts/                   # Lua 遊戲邏輯腳本
│   │   ├── ai/                    #   NPC AI
│   │   ├── combat/                #   戰鬥公式
│   │   ├── core/                  #   核心查表（STR/DEX/經驗）
│   │   ├── character/             #   角色相關
│   │   ├── item/                  #   道具使用
│   │   ├── skill/                 #   技能處理
│   │   └── world/                 #   世界事件
│   ├── internal/
│   │   ├── config/                # TOML 設定載入
│   │   ├── core/ecs/              # ECS 實體組件系統
│   │   ├── core/event/            # 事件匯流排
│   │   ├── core/system/           # 系統執行器
│   │   ├── net/                   # 網路層（TCP、加密、封包）
│   │   ├── handler/               # 封包處理器
│   │   ├── world/                 # 世界狀態、AOI
│   │   ├── scripting/             # Lua 引擎封裝
│   │   ├── persist/               # PostgreSQL 持久化
│   │   └── data/                  # YAML 資料載入器
│   ├── seed/                      # 開發測試資料
│   ├── Makefile
│   └── go.mod
├── map/                           # 地圖資料（從客戶端 .s32 解析後的 .txt）
└── l1j_java/                      # Java 參考原始碼（僅供參考）
```

## 設定檔說明

設定檔位於 `server/config/server.toml`，主要區塊：

| 區塊 | 說明 |
|------|------|
| `[server]` | 伺服器名稱、編號、語系 |
| `[database]` | PostgreSQL 連線字串、連線池 |
| `[network]` | 監聽位址、tick rate、佇列大小 |
| `[rates]` | 經驗 / 掉寶 / 金幣倍率 |
| `[enchant]` | 衝裝成功率 |
| `[character]` | 角色欄位數、自動建帳、刪除等待期 |
| `[logging]` | 日誌等級與格式 |
| `[rate_limit]` | 流量限制 |

## GM 指令

在遊戲內聊天輸入（需 GM 權限）：

角色權限 `200` 為 GM、`100` 為監察員、`0` 為玩家。首位 GM 可用 `go run ./cmd/gm -config config/server.local.toml -name "角色名稱" -level 200` 設定，重新登入後生效。遊戲內 `.help` 依權限列出指令；完整中英文指令、Java 對照差異與部署方式見 [GM 操作說明](docs/GM_COMMANDS.md)。

| 指令 | 說明 |
|------|------|
| `.level <n>` | 設定等級 |
| `.setting me <str/dex/con/wis/cha/int> <n>` | 設定基礎屬性 |
| `.hp/mp <n>` | 設定 HP/MP |
| `.move <x> <y> <map>` | 傳送到座標 |
| `.item <id> [count] [enchant]` | 生成道具 |
| `.kick <name>` | 踢出玩家 |
| `.kill` | 擊殺目標 |
| `.heal` | 回滿 HP/MP |
| `.speed <n>` | 設定移動速度 |
| `.spawn <npc_id>` | 生成 NPC |
| `.weather <type>` | 設定天氣 |
| `.who` | 列出線上玩家 |

## 開發備註

- **協議層**：完全遵循 3.80C 客戶端的封包格式、操作碼、欄位順序，不可更改
- **架構層**：使用 Go 的 ECS + Lua + Event Bus 架構，與 Java 原始碼架構完全不同
- **Lua 腳本**：所有遊戲數值邏輯（傷害公式、AI 決策、技能效果）都在 Lua 中實現，方便熱更新
- **WAL 安全機制**：交易、商店等經濟操作使用 Write-Ahead Log，先寫 DB 再改記憶體，防止當機造成的經濟漏洞

## License

本專案僅供學習與研究用途。

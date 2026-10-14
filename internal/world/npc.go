package world

import "sync/atomic"

// npcIDCounter generates unique NPC object IDs.
// Starts at 200_000_000 to avoid collision with character DB IDs.
var npcIDCounter atomic.Int32

func init() {
	npcIDCounter.Store(200_000_000)
}

// NextNpcID returns a unique object ID for an NPC instance.
func NextNpcID() int32 {
	return npcIDCounter.Add(1)
}

const (
	NpcHiddenNone     = 0
	NpcHiddenSink     = 1
	NpcHiddenFly      = 2
	NpcHiddenIce      = 3
	NpcHiddenKirtas   = 4
	NpcHiddenShellman = 5
)

// NpcActionStatus 回傳 S_NPCPack 的動作狀態 byte。
func NpcActionStatus(npc *NpcInfo) byte {
	if npc == nil {
		return 0
	}
	if npc.HiddenStatus != NpcHiddenNone && npc.HiddenActionStatus != 0 {
		return npc.HiddenActionStatus
	}
	switch npc.HiddenStatus {
	case NpcHiddenSink:
		return 13
	case NpcHiddenFly, NpcHiddenIce, NpcHiddenShellman:
		return 4
	default:
		return 0
	}
}

// NpcInfo holds runtime data for an NPC currently in-world.
// Accessed only from the game loop goroutine — no locks.
type NpcInfo struct {
	ID                int32  // unique object ID (from NextNpcID)
	NpcID             int32  // template ID
	Impl              string // L1Monster, L1Merchant, L1Guard, etc.
	GfxID             int32
	LightSize         byte // 光源半徑（路燈 NPC 用，0=無光源）
	Name              string
	NameID            string // client string table key (e.g. "$936")
	Level             int16
	X                 int32
	Y                 int32
	MapID             int16
	Heading           int16
	HP                int32
	MaxHP             int32
	MP                int32
	MaxMP             int32
	AC                int16
	STR               int16
	DEX               int16
	Intel             int16
	Exp               int32 // exp reward on kill
	Lawful            int32
	Size              string // "small" or "large"
	MR                int16
	Undead            bool
	UndeadType        int16
	TurnUndeadable    bool
	TurnUndeadableSet bool
	Hard              bool
	CantResurrect     bool
	Agro              bool // true = aggressive, attacks players on sight
	Family            string
	AgroFamily        int16
	AtkDmg            int32 // damage per attack (simplified: Level + STR/3)
	Ranged            int16 // attack range (1 = melee, >1 = ranged attacker)
	AtkSpeed          int16 // attack animation speed (ms, 0 = default)
	AtkMagicSpeed     int16 // attack magic animation delay (ms, 0 = fallback)
	SubMagicSpeed     int16 // non-directional magic animation delay (ms, 0 = fallback)
	MoveSpeed         int16 // passive/move speed (ms, 0 = default)
	PoisonAtk         byte  // 怪物施毒能力（從模板載入）: 0=無, 1=傷害毒, 2=沉默毒, 4=麻痺毒
	FireRes           int16 // 火抗
	WaterRes          int16 // 水抗
	WindRes           int16 // 風抗
	EarthRes          int16 // 地抗
	WeakAttr          int16 // 害怕屬性 bitmask：1=地, 2=火, 4=水, 8=風
	WeaponRequired    int32 // 副本武器需求：玩家右手裝備 item_id 必須等於此值才能造成傷害（0=不限）

	// Spawn data for respawning
	SpawnX       int32
	SpawnY       int32
	SpawnMapID   int16
	RespawnDelay int   // seconds
	MobGroupID   int32 // 群體 ID（隊長記憶，重生時重新建立群體）
	SpawnBaseX   int32 // spawnlist 原始中心 X
	SpawnBaseY   int32 // spawnlist 原始中心 Y
	SpawnCount   int
	SpawnRandomX int32
	SpawnRandomY int32
	SpawnLocX1   int32
	SpawnLocY1   int32
	SpawnLocX2   int32
	SpawnLocY2   int32
	SpawnAvoidPC bool
	SpawnScreen  bool

	// State
	Dead               bool
	DeleteTimer        int  // ticks until S_RemoveObject is sent (Java: NPC_DELETION_TIME, default 10s = 50 ticks)
	RespawnTimer       int  // ticks remaining until respawn
	HiddenStatus       int  // Java L1NpcInstance.HIDDEN_STATUS_*，0=none, 1=sink
	HiddenActionStatus byte // Java setStatus() for hidden NPC packs; 0 uses the default mapping.
	KirtasBarrierTicks int  // Java KIRTAS_Timer _barrierTime，以 200ms tick 計數。

	// AI state — 仇恨系統
	AggroTarget uint64           // SessionID of hate target (0 = no target)，由仇恨列表驅動
	HateList    map[uint64]int32 // 仇恨列表 — key=SessionID, value=累積傷害仇恨值
	// CompanionHate 記錄寵物／召喚獸各自造成的仇恨（key=寵物或召喚獸物件 ID）。
	// 其值已併入主人的 HateList，僅供經驗分配時拆分主人與寵物的份額（Java: CalcExp 以 L1PetInstance 個別計算）。
	CompanionHate map[int32]int32
	AttackTimer int              // ticks until next attack (cooldown)
	MoveTimer   int              // ticks until next move towards target
	StuckTicks  int              // consecutive ticks blocked by another entity (for stuck detection)

	MobSkillUseCounts      map[int]int // mobskill act_no -> successful uses (Java: _skillUseCount)
	MobSkillDelayTicks     map[int]int // mobskill act_no -> remaining reuseDelay ticks
	MobSkillTypeDelayTicks map[int]int // mobskill type -> remaining type-level reuseDelay ticks

	// Idle wandering state (Java: _randomMoveDistance / _randomMoveDirection)
	WanderDist  int   // remaining tiles to walk in current wander direction
	WanderDir   int16 // current wander heading (0-7)
	WanderTimer int   // ticks until next wander step

	// 負面狀態（debuff）
	Paralyzed     bool          // 麻痺/凍結/暈眩 — 跳過所有 AI 行為
	Sleeped       bool          // 睡眠 — 跳過所有 AI 行為，受傷時解除
	WeaponBroken  bool          // 壞物術狀態；Java L1NpcInstance.isWeaponBreaked()
	ActiveDebuffs map[int32]int // skillID → 剩餘 ticks（NPC 不需 stat delta，只需計時）

	ElementalFallDownAttr int16 // 弱化屬性降低的屬性：1=地, 2=火, 4=水, 8=風
	PolyOriginalGfxID     int32 // 變形術作用於 NPC 時保存原始 GFX，0=未變形

	// 家具系統
	FurnitureItemObjID int32 // 對應的道具 objectID（0 = 非家具 NPC）

	// 法術中毒系統（Java L1DamagePoison 對 NPC）
	PoisonDmgAmt      int32  // 每次扣血量（0=無毒）
	PoisonDmgTimer    int    // 距下次扣血的 tick 計數（每 15 tick 扣一次）
	PoisonAttackerSID uint64 // 施毒者 SessionID（仇恨歸屬用）

	// NPC 聊天計時器（Java: L1NpcChatTimer）
	ChatTiming        int  // 目前啟用的聊天時機（0=出現, 1=死亡, 4=被攻擊）
	ChatDelayTicks    int  // 首次延遲剩餘 ticks（0=已開始）
	ChatStep          int  // 目前播放到第幾個 chatId (0-based)
	ChatIntervalTicks int  // 對話間隔剩餘 ticks
	ChatRepeatTicks   int  // 重複間隔剩餘 ticks
	ChatActive        bool // 聊天計時器是否啟用
	ChatFirstAttack   bool // 是否已觸發過首次被攻擊聊天
	ChatAppearStarted bool // 是否已嘗試過出現聊天（防重複啟動）

	// 投石車砲彈冷卻（Java: L1CatapultInstance）
	ShellDamageTime  int64 // 上次普通砲彈發射 Unix 秒
	ShellSilenceTime int64 // 上次沉默砲彈發射 Unix 秒

	// 怪物群體系統（Java: L1MobGroupInfo）
	GroupInfo *MobGroupInfo // 所屬群體資訊（nil=不屬於任何群體）
	IsMinion  bool          // true=隊員（不獨立重生）

	// ShowID 副本實例隔離標籤（對應 Java L1NpcInstance.set_showId）。
	// 0 = 主世界（預設）；>=100 = 副本實例 ID。
	// AOI 過濾用：相同 ShowID 才互相可見（同 mapID 多副本實例隔離）。
	// 副本內生成的 NPC 必須由 QuestInstance 設定 ShowID；主世界 NPC 維持 0。
	ShowID int32

	// Transient 標記是否為「副本內暫態 NPC」（不進入持久化、副本結束時直接銷毀）。
	// 主世界生成的 NPC 預設 false。
	Transient bool
}

// MobGroupInfo 怪物群體運行時狀態。
// Java: L1MobGroupInfo — 記錄群體成員、隊長、解散規則。
type MobGroupInfo struct {
	Leader             *NpcInfo   // 隊長 NPC
	Members            []*NpcInfo // 所有成員（含隊長）
	RemoveGroupOnDeath bool       // 隊長死亡時是否解散群體
}

// CanReceiveDamageFrom 判斷 NPC 是否能被持有指定武器 item_id 的玩家造成傷害。
// 對應火龍窟副本「必須裝備真死亡騎士烈炎之劍」機制：副本怪設定 WeaponRequired，
// 玩家右手武器 item_id 不符時所有玩家來源傷害（近戰/技能/武器 proc）一律歸 0。
// weaponItemID = 0 表示空手或未持武器；NPC WeaponRequired = 0 表示不設限。
func (n *NpcInfo) CanReceiveDamageFrom(weaponItemID int32) bool {
	if n == nil {
		return false
	}
	if n.WeaponRequired == 0 {
		return true
	}
	return weaponItemID == n.WeaponRequired
}

// HasDebuff 檢查 NPC 是否有指定 debuff。
func (n *NpcInfo) HasDebuff(skillID int32) bool {
	if n.ActiveDebuffs == nil {
		return false
	}
	_, ok := n.ActiveDebuffs[skillID]
	return ok
}

// AddDebuff 對 NPC 施加 debuff（skillID → ticks）。
func (n *NpcInfo) AddDebuff(skillID int32, ticks int) {
	if n.ActiveDebuffs == nil {
		n.ActiveDebuffs = make(map[int32]int)
	}
	n.ActiveDebuffs[skillID] = ticks
}

// RemoveDebuff 移除 NPC 的指定 debuff。
func (n *NpcInfo) RemoveDebuff(skillID int32) {
	if n.ActiveDebuffs != nil {
		delete(n.ActiveDebuffs, skillID)
	}
}

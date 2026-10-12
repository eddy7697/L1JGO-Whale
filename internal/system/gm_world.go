package system

import (
	"fmt"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// SpawnGMDebugGfx 建立與 Java L1GfxId 相同的靜止圖形測試 NPC。
func (s *GMCommandSystem) SpawnGMDebugGfx(player *world.PlayerInfo, gfxID int32, count int) error {
	if s.deps.Npcs == nil || s.deps.Npcs.Get(45001) == nil {
		return fmt.Errorf("找不到測試 NPC 模板 45001")
	}
	for i := 0; i < count; i++ {
		npc := newRuntimeNpcFromTemplate(s.deps.Npcs.Get(45001), player.X+int32(i*2), player.Y+int32(i*2), player.MapID, 4, player.ShowID, 0, s.deps.SprTable)
		npc.GfxID = gfxID + int32(i)
		npc.NameID = ""
		npc.Agro, npc.MoveSpeed = false, 0
		s.deps.World.AddNpc(npc)
		for _, viewer := range companionViewersAt(s.deps.World, npc.X, npc.Y, npc.MapID, npc.ShowID) {
			handler.SendNpcPack(viewer.Session, npc)
		}
	}
	return nil
}

// InsertGMSpawn 持久化成功才產生 NPC；不能將 I/O 失敗回報為成功。
func (s *GMCommandSystem) InsertGMSpawn(player *world.PlayerInfo, source string, npcID int32) (int32, error) {
	if player.ShowID != 0 {
		return 0, fmt.Errorf("副本中不能永久新增生成點，請先返回一般世界")
	}
	if s.deps.GMSpawns == nil || s.deps.Npcs == nil {
		return 0, fmt.Errorf("生成點儲存未初始化")
	}
	tmpl := s.deps.Npcs.Get(npcID)
	if tmpl == nil {
		return 0, fmt.Errorf("找不到符合條件的 NPC")
	}
	if source != "mob" && source != "npc" {
		return 0, fmt.Errorf("生成類型必須為 mob 或 npc")
	}
	if source == "mob" && tmpl.Impl != "L1Monster" {
		return 0, fmt.Errorf("指定的 NPC 不是 L1Monster 類型")
	}
	// Java storeSpawn 的怪物範圍為12、重生60-120秒；現有 Go spawn格式取上限。
	delay := 0
	randomXY := int32(0)
	if source == "mob" {
		delay, randomXY = 120, 12
	}
	entry, err := s.deps.GMSpawns.Insert(data.SpawnEntry{Source: source, NpcID: npcID, MapID: player.MapID, X: player.X, Y: player.Y, Heading: player.Heading, Count: 1, RespawnDelay: delay, RandomX: randomXY, RandomY: randomXY})
	if err != nil {
		return 0, err
	}
	npc := newRuntimeNpcFromTemplate(tmpl, entry.X, entry.Y, entry.MapID, entry.Heading, 0, entry.RespawnDelay, s.deps.SprTable)
	npc.SpawnBaseX, npc.SpawnBaseY, npc.SpawnCount = entry.X, entry.Y, 1
	npc.SpawnRandomX, npc.SpawnRandomY = entry.RandomX, entry.RandomY
	s.deps.World.AddNpc(npc)
	for _, viewer := range companionViewersAt(s.deps.World, npc.X, npc.Y, npc.MapID, npc.ShowID) {
		handler.SendNpcPack(viewer.Session, npc)
	}
	return entry.ID, nil
}

// GMSummon 與 Java L1Summon 相同：不受 CHA 限制、petcost=0、使用既有召喚獸 AI 與登出清理。
func (s *SummonSystem) GMSummon(player *world.PlayerInfo, npcID int32, count int) error {
	if s.deps.Npcs == nil {
		return fmt.Errorf("NPC 資料未載入")
	}
	tmpl := s.deps.Npcs.Get(npcID)
	if tmpl == nil {
		return fmt.Errorf("找不到符合條件的 NPC")
	}
	for i := 0; i < count; i++ {
		sum := &world.SummonInfo{
			ID: world.NextNpcID(), OwnerCharID: player.CharID, NpcID: npcID,
			GfxID: tmpl.GfxID, NameID: tmpl.NameID, Name: tmpl.Name, Level: tmpl.Level,
			HP: tmpl.HP, MaxHP: tmpl.HP, MP: tmpl.MP, MaxMP: tmpl.MP,
			AC: tmpl.AC, STR: tmpl.STR, DEX: tmpl.DEX, MR: tmpl.MR,
			AtkDmg: int32(tmpl.Level) + int32(tmpl.STR)/3, AtkSpeed: tmpl.AtkSpeed, MoveSpd: tmpl.PassiveSpeed,
			Ranged: tmpl.Ranged, Lawful: tmpl.Lawful, Size: tmpl.Size,
			X: player.X + int32(world.RandInt(5)) - 2, Y: player.Y + int32(world.RandInt(5)) - 2,
			MapID: player.MapID, ShowID: player.ShowID, Heading: player.Heading,
			Status: world.SummonAggressive, PetCost: 0, GMSummoned: true, TimerTicks: 3600 * 5,
		}
		s.deps.World.AddSummon(sum)
		for _, viewer := range companionViewersAt(s.deps.World, sum.X, sum.Y, sum.MapID, sum.ShowID) {
			handler.SendSummonPack(viewer.Session, sum, viewer.CharID == player.CharID, player.Name)
		}
	}
	return nil
}

// sqlconv converts L1JTW MySQL SQL dump files to Whale YAML format.
//
// Usage:
//
//	go run ./cmd/sqlconv <command> [-sqldir path] [-outdir path]
//
// Commands: npc, spawn, drop, shop, mapids, skills, items, mobskill, all
package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// YAML output structs
// ---------------------------------------------------------------------------

// --- NPC ---
type npcListYAML struct {
	Npcs []npcEntryYAML `yaml:"npcs"`
}
type npcEntryYAML struct {
	NpcID          int32  `yaml:"npc_id"`
	Name           string `yaml:"name"`
	NameID         string `yaml:"nameid"`
	Impl           string `yaml:"impl"`
	GfxID          int32  `yaml:"gfx_id"`
	Level          int16  `yaml:"level"`
	HP             int32  `yaml:"hp"`
	MP             int32  `yaml:"mp"`
	AC             int16  `yaml:"ac"`
	STR            int16  `yaml:"str"`
	DEX            int16  `yaml:"dex"`
	CON            int16  `yaml:"con"`
	WIS            int16  `yaml:"wis"`
	Intel          int16  `yaml:"intel"`
	MR             int16  `yaml:"mr"`
	Exp            int32  `yaml:"exp"`
	Lawful         int32  `yaml:"lawful"`
	Size           string `yaml:"size"`
	WeakAttr       int16  `yaml:"weak_attr"`
	Ranged         int16  `yaml:"ranged"`
	AtkSpeed       int16  `yaml:"atk_speed"`
	AtkMagicSpeed  int16  `yaml:"atk_magic_speed,omitempty"`
	SubMagicSpeed  int16  `yaml:"sub_magic_speed,omitempty"`
	PassiveSpeed   int16  `yaml:"passive_speed"`
	Undead         bool   `yaml:"undead"`
	UndeadType     int16  `yaml:"undead_type"`
	TurnUndeadable bool   `yaml:"turn_undeadable"`
	Agro           bool   `yaml:"agro"`
	Family         string `yaml:"family,omitempty"`
	AgroFamily     int16  `yaml:"agro_family,omitempty"`
	Tameable       bool   `yaml:"tameable"`
}

// --- Spawn ---
type spawnListYAML struct {
	Spawns []spawnEntryYAML `yaml:"spawns"`
}
type spawnEntryYAML struct {
	ID            int32  `yaml:"id,omitempty"`
	Source        string `yaml:"source,omitempty"`
	NpcID         int32  `yaml:"npc_id"`
	MapID         int16  `yaml:"map_id"`
	X             int32  `yaml:"x"`
	Y             int32  `yaml:"y"`
	Count         int    `yaml:"count"`
	RandomX       int32  `yaml:"randomx"`
	RandomY       int32  `yaml:"randomy"`
	LocX1         int32  `yaml:"locx1"`
	LocY1         int32  `yaml:"locy1"`
	LocX2         int32  `yaml:"locx2"`
	LocY2         int32  `yaml:"locy2"`
	Heading       int16  `yaml:"heading"`
	RespawnDelay  int    `yaml:"respawn_delay"`
	MobGroupID    int32  `yaml:"mob_group_id"`
	RespawnScreen bool   `yaml:"respawn_screen"`
	MovementDist  int    `yaml:"movement_distance"`
	Rest          bool   `yaml:"rest"`
	AvoidPC       bool   `yaml:"avoid_pc"`
	Spread        string `yaml:"spread,omitempty"`
}

// --- Drop ---
type dropListYAML struct {
	Drops []mobDropYAML `yaml:"drops"`
}
type mobDropYAML struct {
	MobID int32          `yaml:"mob_id"`
	Items []dropItemYAML `yaml:"items"`
}
type dropItemYAML struct {
	ItemID       int32 `yaml:"item_id"`
	Min          int   `yaml:"min"`
	Max          int   `yaml:"max"`
	Chance       int   `yaml:"chance"`
	EnchantLevel int   `yaml:"enchant_level"`
}

// drop_item.yaml（Java DropItemTable：單品掉落倍率）
type dropItemRateListYAML struct {
	Items []dropItemRateYAML `yaml:"items"`
}
type dropItemRateYAML struct {
	ItemID     int32   `yaml:"item_id"`
	DropRate   float64 `yaml:"drop_rate"`
	DropAmount float64 `yaml:"drop_amount"`
	Note       string  `yaml:"note"`
}

// --- Shop ---
type shopListYAML struct {
	Shops []npcShopYAML `yaml:"shops"`
}
type npcShopYAML struct {
	NpcID int32          `yaml:"npc_id"`
	Items []shopItemYAML `yaml:"items"`
}
type shopItemYAML struct {
	ItemID          int32 `yaml:"item_id"`
	Order           int   `yaml:"order"`
	SellingPrice    int   `yaml:"selling_price"`
	PackCount       int   `yaml:"pack_count"`
	PurchasingPrice int   `yaml:"purchasing_price"`
}

// --- MapIDs ---
type mapListYAML struct {
	Maps []mapEntryYAML `yaml:"maps"`
}
type mapEntryYAML struct {
	MapID         int32   `yaml:"map_id"`
	Name          string  `yaml:"name"`
	StartX        int32   `yaml:"start_x"`
	EndX          int32   `yaml:"end_x"`
	StartY        int32   `yaml:"start_y"`
	EndY          int32   `yaml:"end_y"`
	MonsterAmount float64 `yaml:"monster_amount"`
	DropRate      float64 `yaml:"drop_rate"`
	Underwater    bool    `yaml:"underwater"`
	Markable      bool    `yaml:"markable"`
	Teleportable  bool    `yaml:"teleportable"`
	Escapable     bool    `yaml:"escapable"`
	Resurrection  bool    `yaml:"resurrection"`
	Painwand      bool    `yaml:"painwand"`
	Penalty       bool    `yaml:"penalty"`
	TakePets      bool    `yaml:"take_pets"`
	RecallPets    bool    `yaml:"recall_pets"`
	UsableItem    bool    `yaml:"usable_item"`
	UsableSkill   bool    `yaml:"usable_skill"`
}

// --- Skills ---
type skillListYAML struct {
	Skills []skillEntryYAML `yaml:"skills"`
}
type skillEntryYAML struct {
	SkillID          int32  `yaml:"skill_id"`
	Name             string `yaml:"name"`
	SkillLevel       int    `yaml:"skill_level"`
	SkillNumber      int    `yaml:"skill_number"`
	MpConsume        int    `yaml:"mp_consume"`
	HpConsume        int    `yaml:"hp_consume"`
	ItemConsumeID    int32  `yaml:"item_consume_id"`
	ItemConsumeCount int    `yaml:"item_consume_count"`
	ReuseDelay       int    `yaml:"reuse_delay"`
	BuffDuration     int    `yaml:"buff_duration"`
	Target           string `yaml:"target"`
	TargetTo         int    `yaml:"target_to"`
	DamageValue      int    `yaml:"damage_value"`
	DamageDice       int    `yaml:"damage_dice"`
	DamageDiceCount  int    `yaml:"damage_dice_count"`
	ProbabilityValue int    `yaml:"probability_value"`
	ProbabilityDice  int    `yaml:"probability_dice"`
	Attr             int    `yaml:"attr"`
	Type             int    `yaml:"type"`
	Lawful           int    `yaml:"lawful"`
	Ranged           int    `yaml:"ranged"`
	Area             int    `yaml:"area"`
	Through          int    `yaml:"through"`
	ID               int32  `yaml:"id"`
	NameID           string `yaml:"name_id"`
	ActionID         int    `yaml:"action_id"`
	CastGfx          int32  `yaml:"cast_gfx"`
	CastGfx2         int32  `yaml:"cast_gfx2"`
	SysMsgHappen     int32  `yaml:"sys_msg_happen"`
	SysMsgStop       int32  `yaml:"sys_msg_stop"`
	SysMsgFail       int32  `yaml:"sys_msg_fail"`
}

// --- Weapon ---
type weaponListYAML struct {
	Weapons []weaponEntryYAML `yaml:"weapons"`
}
type weaponEntryYAML struct {
	ItemID           int32  `yaml:"item_id"`
	Name             string `yaml:"name"`
	Type             string `yaml:"type"`
	Material         string `yaml:"material"`
	Weight           int32  `yaml:"weight"`
	InvGfx           int32  `yaml:"inv_gfx"`
	GrdGfx           int32  `yaml:"grd_gfx"`
	ItemDescID       int32  `yaml:"itemdesc_id"`
	DmgSmall         int    `yaml:"dmg_small"`
	DmgLarge         int    `yaml:"dmg_large"`
	Range            int    `yaml:"range"`
	SafeEnchant      int    `yaml:"safe_enchant"`
	UseRoyal         bool   `yaml:"use_royal"`
	UseKnight        bool   `yaml:"use_knight"`
	UseMage          bool   `yaml:"use_mage"`
	UseElf           bool   `yaml:"use_elf"`
	UseDarkElf       bool   `yaml:"use_darkelf"`
	UseDragonKnight  bool   `yaml:"use_dragonknight"`
	UseIllusionist   bool   `yaml:"use_illusionist"`
	HitModifier      int    `yaml:"hit_modifier"`
	DmgModifier      int    `yaml:"dmg_modifier"`
	AddStr           int    `yaml:"add_str"`
	AddCon           int    `yaml:"add_con"`
	AddDex           int    `yaml:"add_dex"`
	AddInt           int    `yaml:"add_int"`
	AddWis           int    `yaml:"add_wis"`
	AddCha           int    `yaml:"add_cha"`
	AddHP            int    `yaml:"add_hp"`
	AddMP            int    `yaml:"add_mp"`
	AddHPR           int    `yaml:"add_hpr"`
	AddMPR           int    `yaml:"add_mpr"`
	AddSP            int    `yaml:"add_sp"`
	MDef             int    `yaml:"m_def"`
	HasteItem        int    `yaml:"haste_item"`
	DoubleDmgChance  int    `yaml:"double_dmg_chance"`
	MagicDmgModifier int    `yaml:"magic_dmg_modifier"`
	CanBeDmg         int    `yaml:"can_be_dmg"`
	MinLevel         int    `yaml:"min_level"`
	MaxLevel         int    `yaml:"max_level"`
	Bless            int    `yaml:"bless"`
	Tradeable        bool   `yaml:"tradeable"`
	CantDelete       bool   `yaml:"cant_delete"`
	MaxUseTime       int    `yaml:"max_use_time"`
}

// --- Armor ---
type armorListYAML struct {
	Armors []armorEntryYAML `yaml:"armors"`
}
type armorEntryYAML struct {
	ItemID          int32  `yaml:"item_id"`
	Name            string `yaml:"name"`
	Type            string `yaml:"type"`
	Material        string `yaml:"material"`
	Weight          int32  `yaml:"weight"`
	InvGfx          int32  `yaml:"inv_gfx"`
	GrdGfx          int32  `yaml:"grd_gfx"`
	ItemDescID      int32  `yaml:"itemdesc_id"`
	AC              int    `yaml:"ac"`
	SafeEnchant     int    `yaml:"safe_enchant"`
	UseRoyal        bool   `yaml:"use_royal"`
	UseKnight       bool   `yaml:"use_knight"`
	UseMage         bool   `yaml:"use_mage"`
	UseElf          bool   `yaml:"use_elf"`
	UseDarkElf      bool   `yaml:"use_darkelf"`
	UseDragonKnight bool   `yaml:"use_dragonknight"`
	UseIllusionist  bool   `yaml:"use_illusionist"`
	AddStr          int    `yaml:"add_str"`
	AddCon          int    `yaml:"add_con"`
	AddDex          int    `yaml:"add_dex"`
	AddInt          int    `yaml:"add_int"`
	AddWis          int    `yaml:"add_wis"`
	AddCha          int    `yaml:"add_cha"`
	AddHP           int    `yaml:"add_hp"`
	AddMP           int    `yaml:"add_mp"`
	AddHPR          int    `yaml:"add_hpr"`
	AddMPR          int    `yaml:"add_mpr"`
	AddSP           int    `yaml:"add_sp"`
	MinLevel        int    `yaml:"min_level"`
	MaxLevel        int    `yaml:"max_level"`
	MDef            int    `yaml:"m_def"`
	HasteItem       int    `yaml:"haste_item"`
	DamageReduction int    `yaml:"damage_reduction"`
	WeightReduction int    `yaml:"weight_reduction"`
	HitModifier     int    `yaml:"hit_modifier"`
	DmgModifier     int    `yaml:"dmg_modifier"`
	BowHitModifier  int    `yaml:"bow_hit_modifier"`
	BowDmgModifier  int    `yaml:"bow_dmg_modifier"`
	Bless           int    `yaml:"bless"`
	Tradeable       bool   `yaml:"tradeable"`
	CantDelete      bool   `yaml:"cant_delete"`
	MaxUseTime      int    `yaml:"max_use_time"`
	DefenseWater    int    `yaml:"defense_water"`
	DefenseWind     int    `yaml:"defense_wind"`
	DefenseFire     int    `yaml:"defense_fire"`
	DefenseEarth    int    `yaml:"defense_earth"`
	RegistStun      int    `yaml:"regist_stun"`
	RegistStone     int    `yaml:"regist_stone"`
	RegistSleep     int    `yaml:"regist_sleep"`
	RegistFreeze    int    `yaml:"regist_freeze"`
	RegistSustain   int    `yaml:"regist_sustain"`
	RegistBlind     int    `yaml:"regist_blind"`
	Grade           int    `yaml:"grade"`
}

// --- EtcItem ---
type etcItemListYAML struct {
	Items []etcItemEntryYAML `yaml:"items"`
}
type etcItemEntryYAML struct {
	ItemID         int32  `yaml:"item_id"`
	Name           string `yaml:"name"`
	ItemType       string `yaml:"item_type"`
	UseType        string `yaml:"use_type"`
	Material       string `yaml:"material"`
	Weight         int32  `yaml:"weight"`
	InvGfx         int32  `yaml:"inv_gfx"`
	GrdGfx         int32  `yaml:"grd_gfx"`
	ItemDescID     int32  `yaml:"itemdesc_id"`
	Stackable      bool   `yaml:"stackable"`
	MaxChargeCount int    `yaml:"max_charge_count"`
	DmgSmall       int    `yaml:"dmg_small"`
	DmgLarge       int    `yaml:"dmg_large"`
	MinLevel       int    `yaml:"min_level"`
	MaxLevel       int    `yaml:"max_level"`
	LocX           int32  `yaml:"loc_x"`
	LocY           int32  `yaml:"loc_y"`
	MapID          int32  `yaml:"map_id"`
	Bless          int    `yaml:"bless"`
	Tradeable      bool   `yaml:"tradeable"`
	CantDelete     bool   `yaml:"cant_delete"`
	CanSeal        bool   `yaml:"can_seal"`
	DelayID        int    `yaml:"delay_id"`
	DelayTime      int    `yaml:"delay_time"`
	DelayEffect    int    `yaml:"delay_effect"`
	FoodVolume     int    `yaml:"food_volume"`
	SaveAtOnce     bool   `yaml:"save_at_once"`
}

// --- MobSkill ---
type mobSkillListYAML struct {
	MobSkills []mobSkillGroupYAML `yaml:"mob_skills"`
}
type mobSkillGroupYAML struct {
	MobID  int32               `yaml:"mob_id"`
	Skills []mobSkillEntryYAML `yaml:"skills"`
}
type mobSkillEntryYAML struct {
	ActNo              int    `yaml:"act_no"`
	Name               string `yaml:"name"`
	Type               int    `yaml:"type"`
	MpConsume          int    `yaml:"mp_consume"`
	TriggerRandom      int    `yaml:"trigger_random"`
	TriggerHP          int    `yaml:"trigger_hp"`
	TriggerCompanionHP int    `yaml:"trigger_companion_hp"`
	TriggerRange       int    `yaml:"trigger_range"`
	TriggerCount       int    `yaml:"trigger_count"`
	ChangeTarget       int    `yaml:"change_target"`
	Range              int    `yaml:"range"`
	AreaWidth          int    `yaml:"area_width"`
	AreaHeight         int    `yaml:"area_height"`
	Leverage           int    `yaml:"leverage"`
	SkillID            int32  `yaml:"skill_id"`
	SkillArea          int    `yaml:"skill_area"`
	GfxID              int32  `yaml:"gfx_id"`
	ActID              int    `yaml:"act_id"`
	SummonID           int32  `yaml:"summon_id"`
	SummonMin          int    `yaml:"summon_min"`
	SummonMax          int    `yaml:"summon_max"`
	PolyID             int32  `yaml:"poly_id"`
	ReuseDelay         int    `yaml:"reuse_delay,omitempty"`
}

// --- NPC Action (dialog) ---
type npcActionListYAML struct {
	Actions []npcActionEntryYAML `yaml:"actions"`
}
type npcActionEntryYAML struct {
	NpcID        int32  `yaml:"npc_id"`
	NormalAction string `yaml:"normal_action"`
	CaoticAction string `yaml:"caotic_action"`
	TeleportURL  string `yaml:"teleport_url,omitempty"`
	TeleportURLA string `yaml:"teleport_urla,omitempty"`
}

// ---------------------------------------------------------------------------
// SQL parsing helpers
// ---------------------------------------------------------------------------

// parseValues extracts column values from a single INSERT INTO ... VALUES (...) line.
func parseValues(line string) []string {
	upper := strings.ToUpper(line)
	idx := strings.Index(upper, "VALUES")
	if idx == -1 {
		return nil
	}
	rest := line[idx+6:]
	start := strings.IndexByte(rest, '(')
	if start == -1 {
		return nil
	}
	end := strings.LastIndexByte(rest, ')')
	if end == -1 || end <= start {
		return nil
	}
	inner := rest[start+1 : end]

	var values []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if inQuote {
			if ch == '\'' {
				if i+1 < len(inner) && inner[i+1] == '\'' {
					cur.WriteByte('\'')
					i++
				} else {
					inQuote = false
				}
			} else {
				cur.WriteByte(ch)
			}
		} else {
			switch ch {
			case '\'':
				inQuote = true
			case ',':
				values = append(values, strings.TrimSpace(cur.String()))
				cur.Reset()
			default:
				cur.WriteByte(ch)
			}
		}
	}
	values = append(values, strings.TrimSpace(cur.String()))

	for i, v := range values {
		if strings.EqualFold(v, "null") {
			values[i] = ""
		}
	}
	return values
}

// parseAllInserts reads a SQL file and returns all parsed INSERT rows.
func parseAllInserts(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var rows [][]string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "INSERT INTO") {
			continue
		}
		vals := parseValues(line)
		if vals != nil {
			rows = append(rows, vals)
		}
	}
	return rows, nil
}

func parseInt(s string) int {
	if s == "" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

func parseInt32(s string) int32 { return int32(parseInt(s)) }
func parseInt16(s string) int16 { return int16(parseInt(s)) }

func parseFloat64(s string) float64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseBool01(s string) bool { return s != "" && s != "0" }

func normalizeAgroFamily(family string, agroFamily int16) int16 {
	if strings.TrimSpace(family) == "" && agroFamily == 1 {
		return 0
	}
	return agroFamily
}

// ---------------------------------------------------------------------------
// YAML writer
// ---------------------------------------------------------------------------

func writeYAML(path string, data interface{}, comment string) error {
	out, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if comment != "" {
		fmt.Fprintln(f, comment)
		fmt.Fprintln(f)
	}
	_, err = f.Write(out)
	return err
}

func loadNpcIDsFromYAML(path string) (map[int32]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f npcListYAML
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	ids := make(map[int32]bool, len(f.Npcs))
	for _, npc := range f.Npcs {
		ids[npc.NpcID] = true
	}
	return ids, nil
}

func loadNpcImplsFromYAML(path string) (map[int32]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f npcListYAML
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	impls := make(map[int32]string, len(f.Npcs))
	for _, npc := range f.Npcs {
		impls[npc.NpcID] = npc.Impl
	}
	return impls, nil
}

func filterSpawnsByNpcList(spawns []spawnEntryYAML, npcImpls map[int32]string) ([]spawnEntryYAML, int) {
	if len(npcImpls) == 0 {
		return spawns, 0
	}
	out := spawns[:0]
	removed := 0
	for _, spawn := range spawns {
		if _, ok := npcImpls[spawn.NpcID]; ok {
			out = append(out, spawn)
			continue
		}
		removed++
	}
	return out, removed
}

func loadSpawnListYAML(path string) ([]spawnEntryYAML, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f spawnListYAML
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return f.Spawns, nil
}

func mergeBaseNormalNpcSpawns(spawns []spawnEntryYAML, base []spawnEntryYAML, npcImpls map[int32]string) ([]spawnEntryYAML, int) {
	added := 0
	for _, spawn := range base {
		if !isNormalNpcSpawn(spawn, npcImpls) {
			continue
		}
		spawns = append(spawns, spawn)
		added++
	}
	return spawns, added
}

func isNormalNpcSpawn(spawn spawnEntryYAML, npcImpls map[int32]string) bool {
	impl, ok := npcImpls[spawn.NpcID]
	return ok && impl != "L1Monster"
}

func spawnKey(spawn spawnEntryYAML) string {
	return fmt.Sprintf("%d:%d:%d:%d", spawn.NpcID, spawn.MapID, spawn.X, spawn.Y)
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------

func convertNpc(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "npc.sql"))
	if err != nil {
		return err
	}
	var npcs []npcEntryYAML
	for _, r := range rows {
		var entry npcEntryYAML
		if sqlFormat == "yiwei" {
			// Yiwei: npcid(0) name(1) nameid(2) classname(3) note(4) impl(5)
			// gfxid(6) lvl(7) hp(8) mp(9) ac(10) str(11) con(12) dex(13)
			// wis(14) intel(15) mr(16) exp(17) lawful(18) size(19) weakAttr(20)
			// ranged(21) tamable(22) passispeed(23) atkspeed(24) atk_magic_speed(25) sub_magic_speed(26)
			// undead(27) ... agro(30) ... IsTU(54)
			if len(r) < 55 {
				continue
			}
			entry = npcEntryYAML{
				NpcID:          parseInt32(r[0]),
				Name:           r[1],
				NameID:         r[2],
				Impl:           r[5],
				GfxID:          parseInt32(r[6]),
				Level:          parseInt16(r[7]),
				HP:             parseInt32(r[8]),
				MP:             parseInt32(r[9]),
				AC:             parseInt16(r[10]),
				STR:            parseInt16(r[11]),
				CON:            parseInt16(r[12]),
				DEX:            parseInt16(r[13]),
				WIS:            parseInt16(r[14]),
				Intel:          parseInt16(r[15]),
				MR:             parseInt16(r[16]),
				Exp:            parseInt32(r[17]),
				Lawful:         parseInt32(r[18]),
				Size:           r[19],
				WeakAttr:       parseInt16(r[20]),
				Ranged:         parseInt16(r[21]),
				AtkSpeed:       parseInt16(r[24]),
				AtkMagicSpeed:  parseInt16(r[25]),
				SubMagicSpeed:  parseInt16(r[26]),
				PassiveSpeed:   parseInt16(r[23]),
				Undead:         parseBool01(r[27]),
				UndeadType:     parseInt16(r[27]),
				TurnUndeadable: parseBool01(r[54]),
				Agro:           parseBool01(r[30]),
				Family:         strings.TrimSpace(r[33]),
				AgroFamily:     normalizeAgroFamily(r[33], parseInt16(r[34])),
				Tameable:       parseBool01(r[22]),
			}
		} else {
			// Taiwan: npcid(0) name(1) nameid(2) note(3) impl(4) gfxid(5)
			// lvl(6) hp(7) mp(8) ac(9) str(10) con(11) dex(12) wis(13)
			// intel(14) mr(15) exp(16) lawful(17) size(18) ... ranged(20)
			// tamable(21) passispeed(22) atkspeed(23) atk_magic_speed(24) sub_magic_speed(25) undead(27) ... agro(30) ... IsTU(54)
			if len(r) < 55 {
				continue
			}
			entry = npcEntryYAML{
				NpcID:          parseInt32(r[0]),
				Name:           r[1],
				NameID:         r[2],
				Impl:           r[4],
				GfxID:          parseInt32(r[5]),
				Level:          parseInt16(r[6]),
				HP:             parseInt32(r[7]),
				MP:             parseInt32(r[8]),
				AC:             parseInt16(r[9]),
				STR:            parseInt16(r[10]),
				DEX:            parseInt16(r[12]),
				CON:            parseInt16(r[11]),
				WIS:            parseInt16(r[13]),
				Intel:          parseInt16(r[14]),
				MR:             parseInt16(r[15]),
				Exp:            parseInt32(r[16]),
				Lawful:         parseInt32(r[17]),
				Size:           r[18],
				WeakAttr:       parseInt16(r[19]),
				Ranged:         parseInt16(r[20]),
				AtkSpeed:       parseInt16(r[23]),
				AtkMagicSpeed:  parseInt16(r[24]),
				SubMagicSpeed:  parseInt16(r[25]),
				PassiveSpeed:   parseInt16(r[22]),
				Undead:         parseBool01(r[27]),
				UndeadType:     parseInt16(r[27]),
				TurnUndeadable: parseBool01(r[54]),
				Agro:           parseBool01(r[30]),
				Family:         strings.TrimSpace(r[33]),
				AgroFamily:     normalizeAgroFamily(r[33], parseInt16(r[34])),
				Tameable:       parseBool01(r[21]),
			}
		}
		npcs = append(npcs, entry)
	}
	sort.Slice(npcs, func(i, j int) bool { return npcs[i].NpcID < npcs[j].NpcID })
	fmt.Printf("  npc: %d entries (from %d total rows)\n", len(npcs), len(rows))
	return writeYAML(filepath.Join(outDir, "npc_list.yaml"),
		npcListYAML{Npcs: npcs},
		"# NPC templates - converted from npc.sql")
}

func convertSpawn(sqlDir, outDir string) error {
	// --- Monster spawns from spawnlist.sql ---
	rows, err := parseAllInserts(filepath.Join(sqlDir, "spawnlist.sql"))
	if err != nil {
		return err
	}
	var spawns []spawnEntryYAML
	for _, r := range rows {
		if len(r) < 21 {
			continue
		}
		count := parseInt(r[2])
		if count == 0 {
			continue
		}
		minDelay := parseInt(r[14])
		maxDelay := parseInt(r[15])
		delay := maxDelay
		if minDelay > maxDelay {
			delay = minDelay
		}
		spawns = append(spawns, spawnEntryYAML{
			ID:            parseInt32(r[0]),
			Source:        "mob",
			NpcID:         parseInt32(r[3]),
			MapID:         parseInt16(r[16]),
			X:             parseInt32(r[5]),
			Y:             parseInt32(r[6]),
			Count:         count,
			RandomX:       parseInt32(r[7]),
			RandomY:       parseInt32(r[8]),
			LocX1:         parseInt32(r[9]),
			LocY1:         parseInt32(r[10]),
			LocX2:         parseInt32(r[11]),
			LocY2:         parseInt32(r[12]),
			Heading:       parseInt16(r[13]),
			RespawnDelay:  delay,
			MobGroupID:    parseInt32(r[4]),
			RespawnScreen: parseBool01(r[17]),
			MovementDist:  parseInt(r[18]),
			Rest:          parseBool01(r[19]),
			AvoidPC:       parseBool01(r[20]),
		})
	}
	monsterCount := len(spawns)
	fmt.Printf("  spawn (monsters): %d entries (from %d rows)\n", monsterCount, len(rows))

	npcImpls, err := loadNpcImplsFromYAML(filepath.Join(outDir, "npc_list.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load npc_list for spawn merge: %w", err)
	}

	if existing, err := loadSpawnListYAML(filepath.Join(outDir, "spawn_list.yaml")); err == nil && len(npcImpls) > 0 {
		var added int
		spawns, added = mergeBaseNormalNpcSpawns(spawns, existing, npcImpls)
		if added > 0 {
			fmt.Printf("  spawn preserve: restored %d base normal NPC entries\n", added)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load existing spawn_list for base NPC preserve: %w", err)
	} else {
		// Fallback for first-time conversion when no base spawn_list.yaml exists.
		npcRows, err := parseAllInserts(filepath.Join(sqlDir, "spawnlist_npc.sql"))
		if err != nil {
			fmt.Printf("  spawn (npcs): skipped (file not found)\n")
		} else {
			npcCount := 0
			for _, r := range npcRows {
				if len(r) < 12 {
					continue
				}
				count := parseInt(r[2])
				if count == 0 {
					continue
				}
				spawns = append(spawns, spawnEntryYAML{
					ID:           parseInt32(r[0]),
					Source:       "npc",
					NpcID:        parseInt32(r[3]),  // npc_templateid
					MapID:        parseInt16(r[10]), // mapid
					X:            parseInt32(r[4]),  // locx
					Y:            parseInt32(r[5]),  // locy
					Count:        count,
					RandomX:      parseInt32(r[6]), // randomx
					RandomY:      parseInt32(r[7]), // randomy
					Heading:      parseInt16(r[8]), // heading
					RespawnDelay: parseInt(r[9]),   // respawn_delay
					MovementDist: parseInt(r[11]),  // movement_distance
				})
				npcCount++
			}
			fmt.Printf("  spawn (npcs): %d entries (from %d rows)\n", npcCount, len(npcRows))
		}
	}

	if len(npcImpls) > 0 {
		var removed int
		spawns, removed = filterSpawnsByNpcList(spawns, npcImpls)
		if removed > 0 {
			fmt.Printf("  spawn filter: removed %d entries with missing npc templates\n", removed)
		}
	}

	sort.Slice(spawns, func(i, j int) bool {
		if spawns[i].MapID != spawns[j].MapID {
			return spawns[i].MapID < spawns[j].MapID
		}
		return spawns[i].NpcID < spawns[j].NpcID
	})
	fmt.Printf("  spawn total: %d entries\n", len(spawns))
	return writeYAML(filepath.Join(outDir, "spawn_list.yaml"),
		spawnListYAML{Spawns: spawns},
		"# NPC spawn list - converted from spawnlist.sql + spawnlist_npc.sql")
}

// convertDropItem 轉換 drop_item.sql → drop_item.yaml（item_id, drop_rate, drop_amount, note）。
func convertDropItem(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "drop_item.sql"))
	if err != nil {
		return err
	}
	var items []dropItemRateYAML
	for _, r := range rows {
		if len(r) < 3 {
			continue
		}
		item := dropItemRateYAML{ItemID: parseInt32(r[0]), DropRate: parseFloat64(r[1]), DropAmount: parseFloat64(r[2])}
		if len(r) > 3 {
			item.Note = r[3]
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ItemID < items[j].ItemID })
	fmt.Printf("  dropitem: %d items\n", len(items))
	return writeYAML(filepath.Join(outDir, "drop_item.yaml"),
		dropItemRateListYAML{Items: items},
		"# Per-item drop rate / amount multipliers - converted from drop_item.sql (Java DropItemTable)")
}

func convertDrop(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "droplist.sql"))
	if err != nil {
		return err
	}
	groups := make(map[int32][]dropItemYAML)
	for _, r := range rows {
		mobID := parseInt32(r[0])
		var item dropItemYAML
		if sqlFormat == "yiwei" {
			// Yiwei: mobId(0) note(1) itemId(2) 物品名稱(3) min(4) max(5) chance(6)
			// No enchantlvl column.
			if len(r) < 7 {
				continue
			}
			item = dropItemYAML{
				ItemID: parseInt32(r[2]),
				Min:    parseInt(r[4]),
				Max:    parseInt(r[5]),
				Chance: parseInt(r[6]),
			}
		} else {
			// Taiwan: mobId(0) itemId(1) min(2) max(3) chance(4) enchantlvl(5)
			if len(r) < 6 {
				continue
			}
			item = dropItemYAML{
				ItemID:       parseInt32(r[1]),
				Min:          parseInt(r[2]),
				Max:          parseInt(r[3]),
				Chance:       parseInt(r[4]),
				EnchantLevel: parseInt(r[5]),
			}
		}
		groups[mobID] = append(groups[mobID], item)
	}
	var mobIDs []int32
	for id := range groups {
		mobIDs = append(mobIDs, id)
	}
	sort.Slice(mobIDs, func(i, j int) bool { return mobIDs[i] < mobIDs[j] })
	var drops []mobDropYAML
	for _, id := range mobIDs {
		drops = append(drops, mobDropYAML{MobID: id, Items: groups[id]})
	}
	fmt.Printf("  drop: %d mobs, %d total items\n", len(drops), len(rows))
	return writeYAML(filepath.Join(outDir, "drop_list.yaml"),
		dropListYAML{Drops: drops},
		"# Monster drop list - converted from droplist.sql")
}

func convertShop(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "shop.sql"))
	if err != nil {
		return err
	}
	groups := make(map[int32][]shopItemYAML)
	for _, r := range rows {
		if len(r) < 6 {
			continue
		}
		npcID := parseInt32(r[0])
		groups[npcID] = append(groups[npcID], shopItemYAML{
			ItemID:          parseInt32(r[1]),
			Order:           parseInt(r[2]),
			SellingPrice:    parseInt(r[3]),
			PackCount:       parseInt(r[4]),
			PurchasingPrice: parseInt(r[5]),
		})
	}
	var npcIDs []int32
	for id := range groups {
		npcIDs = append(npcIDs, id)
	}
	sort.Slice(npcIDs, func(i, j int) bool { return npcIDs[i] < npcIDs[j] })
	var shops []npcShopYAML
	for _, id := range npcIDs {
		shops = append(shops, npcShopYAML{NpcID: id, Items: groups[id]})
	}
	fmt.Printf("  shop: %d NPCs, %d total items\n", len(shops), len(rows))
	return writeYAML(filepath.Join(outDir, "shop_list.yaml"),
		shopListYAML{Shops: shops},
		"# NPC shop list - converted from shop.sql")
}

func convertMapIDs(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "mapids.sql"))
	if err != nil {
		return err
	}
	var maps []mapEntryYAML
	for _, r := range rows {
		if len(r) < 19 {
			continue
		}
		maps = append(maps, mapEntryYAML{
			MapID:         parseInt32(r[0]),
			Name:          r[1],
			StartX:        parseInt32(r[2]),
			EndX:          parseInt32(r[3]),
			StartY:        parseInt32(r[4]),
			EndY:          parseInt32(r[5]),
			MonsterAmount: parseFloat64(r[6]),
			DropRate:      parseFloat64(r[7]),
			Underwater:    parseBool01(r[8]),
			Markable:      parseBool01(r[9]),
			Teleportable:  parseBool01(r[10]),
			Escapable:     parseBool01(r[11]),
			Resurrection:  parseBool01(r[12]),
			Painwand:      parseBool01(r[13]),
			Penalty:       parseBool01(r[14]),
			TakePets:      parseBool01(r[15]),
			RecallPets:    parseBool01(r[16]),
			UsableItem:    parseBool01(r[17]),
			UsableSkill:   parseBool01(r[18]),
		})
	}
	sort.Slice(maps, func(i, j int) bool { return maps[i].MapID < maps[j].MapID })
	fmt.Printf("  mapids: %d maps\n", len(maps))
	return writeYAML(filepath.Join(outDir, "map_list.yaml"),
		mapListYAML{Maps: maps},
		"# Map definitions - converted from mapids.sql")
}

func convertSkills(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "skills.sql"))
	if err != nil {
		return err
	}
	var skills []skillEntryYAML
	for _, r := range rows {
		if len(r) < 31 {
			continue
		}
		skills = append(skills, skillEntryYAML{
			SkillID:          parseInt32(r[0]),
			Name:             r[1],
			SkillLevel:       parseInt(r[2]),
			SkillNumber:      parseInt(r[3]),
			MpConsume:        parseInt(r[4]),
			HpConsume:        parseInt(r[5]),
			ItemConsumeID:    parseInt32(r[6]),
			ItemConsumeCount: parseInt(r[7]),
			ReuseDelay:       parseInt(r[8]),
			BuffDuration:     parseInt(r[9]),
			Target:           r[10],
			TargetTo:         parseInt(r[11]),
			DamageValue:      parseInt(r[12]),
			DamageDice:       parseInt(r[13]),
			DamageDiceCount:  parseInt(r[14]),
			ProbabilityValue: parseInt(r[15]),
			ProbabilityDice:  parseInt(r[16]),
			Attr:             parseInt(r[17]),
			Type:             parseInt(r[18]),
			Lawful:           parseInt(r[19]),
			Ranged:           parseInt(r[20]),
			Area:             parseInt(r[21]),
			Through:          parseInt(r[22]),
			ID:               parseInt32(r[23]),
			NameID:           r[24],
			ActionID:         parseInt(r[25]),
			CastGfx:          parseInt32(r[26]),
			CastGfx2:         parseInt32(r[27]),
			SysMsgHappen:     parseInt32(r[28]),
			SysMsgStop:       parseInt32(r[29]),
			SysMsgFail:       parseInt32(r[30]),
		})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].SkillID < skills[j].SkillID })
	fmt.Printf("  skills: %d entries\n", len(skills))
	return writeYAML(filepath.Join(outDir, "skill_list.yaml"),
		skillListYAML{Skills: skills},
		"# Skill definitions - converted from skills.sql")
}

func convertWeapons(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "weapon.sql"))
	if err != nil {
		return err
	}
	var weapons []weaponEntryYAML
	for _, r := range rows {
		var entry weaponEntryYAML
		if sqlFormat == "yiwei" {
			// Yiwei: item_id(0) name(1) classname(2) name_id(3) type(4) material(5)
			// weight(6) invgfx(7) grdgfx(8) itemdesc_id(9) dmg_small(10) dmg_large(11)
			// range(12) safenchant(13) 王族(14) 騎士(15) 法師(16) 妖精(17) 黑妖(18)
			// 龍騎士(19) 幻術師(20) 戰士(21) hitmodifier(22) dmgmodifier(23)
			// add_str(24)..add_cha(29) add_hp(30)..add_mpr(33) add_sp(34)
			// m_def(35) haste_item(36) double_dmg_chance(37) magicdmgmodifier(38)
			// canbedmg(39) min_lvl(40) max_lvl(41) bless(42) trade(43) cant_delete(44)
			// defense_water(45)..defense_earth(48) regist_stun(49)..regist_blind(54)
			// max_use_time(55) ...custom(56-64)
			if len(r) < 56 {
				continue
			}
			entry = weaponEntryYAML{
				ItemID:           parseInt32(r[0]),
				Name:             r[1],
				Type:             r[4],
				Material:         r[5],
				Weight:           parseInt32(r[6]),
				InvGfx:           parseInt32(r[7]),
				GrdGfx:           parseInt32(r[8]),
				ItemDescID:       parseInt32(r[9]),
				DmgSmall:         parseInt(r[10]),
				DmgLarge:         parseInt(r[11]),
				Range:            parseInt(r[12]),
				SafeEnchant:      parseInt(r[13]),
				UseRoyal:         parseBool01(r[14]),
				UseKnight:        parseBool01(r[15]),
				UseMage:          parseBool01(r[16]),
				UseElf:           parseBool01(r[17]),
				UseDarkElf:       parseBool01(r[18]),
				UseDragonKnight:  parseBool01(r[19]),
				UseIllusionist:   parseBool01(r[20]),
				HitModifier:      parseInt(r[22]),
				DmgModifier:      parseInt(r[23]),
				AddStr:           parseInt(r[24]),
				AddCon:           parseInt(r[25]),
				AddDex:           parseInt(r[26]),
				AddInt:           parseInt(r[27]),
				AddWis:           parseInt(r[28]),
				AddCha:           parseInt(r[29]),
				AddHP:            parseInt(r[30]),
				AddMP:            parseInt(r[31]),
				AddHPR:           parseInt(r[32]),
				AddMPR:           parseInt(r[33]),
				AddSP:            parseInt(r[34]),
				MDef:             parseInt(r[35]),
				HasteItem:        parseInt(r[36]),
				DoubleDmgChance:  parseInt(r[37]),
				MagicDmgModifier: parseInt(r[38]),
				CanBeDmg:         parseInt(r[39]),
				MinLevel:         parseInt(r[40]),
				MaxLevel:         parseInt(r[41]),
				Bless:            parseInt(r[42]),
				Tradeable:        parseBool01(r[43]),
				CantDelete:       parseBool01(r[44]),
				MaxUseTime:       parseInt(r[55]),
			}
		} else {
			// Taiwan: item_id(0) name(1) unid_name(2) id_name(3) type(4) material(5)
			// weight(6) invgfx(7) grdgfx(8) itemdesc_id(9) dmg_small(10) dmg_large(11)
			// range(12) safenchant(13) classes(14-20) hit(21) dmg(22) stats(23-33)
			// m_def(34) haste(35) dbl_dmg(36) magicdmg(37) canbedmg(38)
			// min/max_lvl(39-40) bless(41) trade(42) cant_delete(43) max_use_time(44)
			if len(r) < 45 {
				continue
			}
			entry = weaponEntryYAML{
				ItemID:           parseInt32(r[0]),
				Name:             r[1],
				Type:             r[4],
				Material:         r[5],
				Weight:           parseInt32(r[6]),
				InvGfx:           parseInt32(r[7]),
				GrdGfx:           parseInt32(r[8]),
				ItemDescID:       parseInt32(r[9]),
				DmgSmall:         parseInt(r[10]),
				DmgLarge:         parseInt(r[11]),
				Range:            parseInt(r[12]),
				SafeEnchant:      parseInt(r[13]),
				UseRoyal:         parseBool01(r[14]),
				UseKnight:        parseBool01(r[15]),
				UseMage:          parseBool01(r[16]),
				UseElf:           parseBool01(r[17]),
				UseDarkElf:       parseBool01(r[18]),
				UseDragonKnight:  parseBool01(r[19]),
				UseIllusionist:   parseBool01(r[20]),
				HitModifier:      parseInt(r[21]),
				DmgModifier:      parseInt(r[22]),
				AddStr:           parseInt(r[23]),
				AddCon:           parseInt(r[24]),
				AddDex:           parseInt(r[25]),
				AddInt:           parseInt(r[26]),
				AddWis:           parseInt(r[27]),
				AddCha:           parseInt(r[28]),
				AddHP:            parseInt(r[29]),
				AddMP:            parseInt(r[30]),
				AddHPR:           parseInt(r[31]),
				AddMPR:           parseInt(r[32]),
				AddSP:            parseInt(r[33]),
				MDef:             parseInt(r[34]),
				HasteItem:        parseInt(r[35]),
				DoubleDmgChance:  parseInt(r[36]),
				MagicDmgModifier: parseInt(r[37]),
				CanBeDmg:         parseInt(r[38]),
				MinLevel:         parseInt(r[39]),
				MaxLevel:         parseInt(r[40]),
				Bless:            parseInt(r[41]),
				Tradeable:        parseBool01(r[42]),
				CantDelete:       parseBool01(r[43]),
				MaxUseTime:       parseInt(r[44]),
			}
		}
		weapons = append(weapons, entry)
	}
	sort.Slice(weapons, func(i, j int) bool { return weapons[i].ItemID < weapons[j].ItemID })
	fmt.Printf("  weapon: %d entries\n", len(weapons))
	return writeYAML(filepath.Join(outDir, "weapon_list.yaml"),
		weaponListYAML{Weapons: weapons},
		"# Weapon definitions - converted from weapon.sql")
}

func convertArmors(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "armor.sql"))
	if err != nil {
		return err
	}
	var armors []armorEntryYAML
	for _, r := range rows {
		var entry armorEntryYAML
		if sqlFormat == "yiwei" {
			// Yiwei: item_id(0) name(1) exp_point(2) classname(3) name_id(4)
			// type(5) material(6) weight(7) invgfx(8) grdgfx(9) itemdesc_id(10)
			// ac(11) safenchant(12) 王族(13)..幻術師(19) 戰士(20)
			// add_str(21)..add_cha(26) add_hp(27) add_mp(28) add_hpr(29) add_mpr(30) add_sp(31)
			// min_lvl(32) max_lvl(33) m_def(34) haste_item(35)
			// damage_reduction(36) weight_reduction(37) hit_modifier(38) dmg_modifier(39)
			// bow_hit(40) bow_dmg(41) bless(42) trade(43) cant_delete(44) max_use_time(45)
			// defense_water(46)..defense_earth(49) regist_stun(50)..regist_blind(55)
			// greater(56) ...custom(57-71)
			if len(r) < 57 {
				continue
			}
			entry = armorEntryYAML{
				ItemID:          parseInt32(r[0]),
				Name:            r[1],
				Type:            r[5],
				Material:        r[6],
				Weight:          parseInt32(r[7]),
				InvGfx:          parseInt32(r[8]),
				GrdGfx:          parseInt32(r[9]),
				ItemDescID:      parseInt32(r[10]),
				AC:              parseInt(r[11]),
				SafeEnchant:     parseInt(r[12]),
				UseRoyal:        parseBool01(r[13]),
				UseKnight:       parseBool01(r[14]),
				UseMage:         parseBool01(r[15]),
				UseElf:          parseBool01(r[16]),
				UseDarkElf:      parseBool01(r[17]),
				UseDragonKnight: parseBool01(r[18]),
				UseIllusionist:  parseBool01(r[19]),
				AddStr:          parseInt(r[21]),
				AddCon:          parseInt(r[22]),
				AddDex:          parseInt(r[23]),
				AddInt:          parseInt(r[24]),
				AddWis:          parseInt(r[25]),
				AddCha:          parseInt(r[26]),
				AddHP:           parseInt(r[27]),
				AddMP:           parseInt(r[28]),
				AddHPR:          parseInt(r[29]),
				AddMPR:          parseInt(r[30]),
				AddSP:           parseInt(r[31]),
				MinLevel:        parseInt(r[32]),
				MaxLevel:        parseInt(r[33]),
				MDef:            parseInt(r[34]),
				HasteItem:       parseInt(r[35]),
				DamageReduction: parseInt(r[36]),
				WeightReduction: parseInt(r[37]),
				HitModifier:     parseInt(r[38]),
				DmgModifier:     parseInt(r[39]),
				BowHitModifier:  parseInt(r[40]),
				BowDmgModifier:  parseInt(r[41]),
				Bless:           parseInt(r[42]),
				Tradeable:       parseBool01(r[43]),
				CantDelete:      parseBool01(r[44]),
				MaxUseTime:      parseInt(r[45]),
				DefenseWater:    parseInt(r[46]),
				DefenseWind:     parseInt(r[47]),
				DefenseFire:     parseInt(r[48]),
				DefenseEarth:    parseInt(r[49]),
				RegistStun:      parseInt(r[50]),
				RegistStone:     parseInt(r[51]),
				RegistSleep:     parseInt(r[52]),
				RegistFreeze:    parseInt(r[53]),
				RegistSustain:   parseInt(r[54]),
				RegistBlind:     parseInt(r[55]),
				Grade:           parseInt(r[56]),
			}
		} else {
			// Taiwan: item_id(0) name(1) unid_name(2) id_name(3) type(4) material(5)
			// weight(6) invgfx(7) grdgfx(8) itemdesc_id(9) ac(10) safenchant(11)
			// classes(12-18) stats(19-29) min/max_lvl(30-31) m_def(32) haste(33)
			// dmg_red(34) wt_red(35) hit/dmg(36-39) bow_hit/dmg(38-39)
			// bless(40) trade(41) cant_delete(42) max_use_time(43)
			// defenses(44-47) resists(48-53) grade(54)
			if len(r) < 55 {
				continue
			}
			entry = armorEntryYAML{
				ItemID:          parseInt32(r[0]),
				Name:            r[1],
				Type:            r[4],
				Material:        r[5],
				Weight:          parseInt32(r[6]),
				InvGfx:          parseInt32(r[7]),
				GrdGfx:          parseInt32(r[8]),
				ItemDescID:      parseInt32(r[9]),
				AC:              parseInt(r[10]),
				SafeEnchant:     parseInt(r[11]),
				UseRoyal:        parseBool01(r[12]),
				UseKnight:       parseBool01(r[13]),
				UseMage:         parseBool01(r[14]),
				UseElf:          parseBool01(r[15]),
				UseDarkElf:      parseBool01(r[16]),
				UseDragonKnight: parseBool01(r[17]),
				UseIllusionist:  parseBool01(r[18]),
				AddStr:          parseInt(r[19]),
				AddCon:          parseInt(r[20]),
				AddDex:          parseInt(r[21]),
				AddInt:          parseInt(r[22]),
				AddWis:          parseInt(r[23]),
				AddCha:          parseInt(r[24]),
				AddHP:           parseInt(r[25]),
				AddMP:           parseInt(r[26]),
				AddHPR:          parseInt(r[27]),
				AddMPR:          parseInt(r[28]),
				AddSP:           parseInt(r[29]),
				MinLevel:        parseInt(r[30]),
				MaxLevel:        parseInt(r[31]),
				MDef:            parseInt(r[32]),
				HasteItem:       parseInt(r[33]),
				DamageReduction: parseInt(r[34]),
				WeightReduction: parseInt(r[35]),
				HitModifier:     parseInt(r[36]),
				DmgModifier:     parseInt(r[37]),
				BowHitModifier:  parseInt(r[38]),
				BowDmgModifier:  parseInt(r[39]),
				Bless:           parseInt(r[40]),
				Tradeable:       parseBool01(r[41]),
				CantDelete:      parseBool01(r[42]),
				MaxUseTime:      parseInt(r[43]),
				DefenseWater:    parseInt(r[44]),
				DefenseWind:     parseInt(r[45]),
				DefenseFire:     parseInt(r[46]),
				DefenseEarth:    parseInt(r[47]),
				RegistStun:      parseInt(r[48]),
				RegistStone:     parseInt(r[49]),
				RegistSleep:     parseInt(r[50]),
				RegistFreeze:    parseInt(r[51]),
				RegistSustain:   parseInt(r[52]),
				RegistBlind:     parseInt(r[53]),
				Grade:           parseInt(r[54]),
			}
		}
		armors = append(armors, entry)
	}
	sort.Slice(armors, func(i, j int) bool { return armors[i].ItemID < armors[j].ItemID })
	fmt.Printf("  armor: %d entries\n", len(armors))
	return writeYAML(filepath.Join(outDir, "armor_list.yaml"),
		armorListYAML{Armors: armors},
		"# Armor definitions - converted from armor.sql")
}

func convertEtcItems(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "etcitem.sql"))
	if err != nil {
		return err
	}
	var items []etcItemEntryYAML
	for _, r := range rows {
		var entry etcItemEntryYAML
		if sqlFormat == "yiwei" {
			// Yiwei: item_id(0) name(1) classname(2) name_id(3) item_type(4) use_type(5)
			// material(6) weight(7) invgfx(8) grdgfx(9) itemdesc_id(10)
			// stackable(11) max_charge_count(12) max_use_time(13) dmg_small(14) dmg_large(15)
			// min_lvl(16) max_lvl(17) bless(18) trade(19) cant_delete(20)
			// delay_id(21) delay_time(22) delay_effect(23) food_volume(24) save_at_once(25)
			// 增加水屬性(26)..增加地屬性(29) 此商品是否販賣(30) MeteLevel(31) MeteLevelMAX(32) 職業判斷(33)
			// Yiwei lacks: locx, locy, mapid, can_seal
			if len(r) < 26 {
				continue
			}
			entry = etcItemEntryYAML{
				ItemID:         parseInt32(r[0]),
				Name:           r[1],
				ItemType:       r[4],
				UseType:        r[5],
				Material:       r[6],
				Weight:         parseInt32(r[7]),
				InvGfx:         parseInt32(r[8]),
				GrdGfx:         parseInt32(r[9]),
				ItemDescID:     parseInt32(r[10]),
				Stackable:      parseBool01(r[11]),
				MaxChargeCount: parseInt(r[12]),
				DmgSmall:       parseInt(r[14]),
				DmgLarge:       parseInt(r[15]),
				MinLevel:       parseInt(r[16]),
				MaxLevel:       parseInt(r[17]),
				LocX:           0, // not in yiwei
				LocY:           0,
				MapID:          0,
				Bless:          parseInt(r[18]),
				Tradeable:      parseBool01(r[19]),
				CantDelete:     parseBool01(r[20]),
				CanSeal:        false, // not in yiwei
				DelayID:        parseInt(r[21]),
				DelayTime:      parseInt(r[22]),
				DelayEffect:    parseInt(r[23]),
				FoodVolume:     parseInt(r[24]),
				SaveAtOnce:     parseBool01(r[25]),
			}
		} else {
			// Taiwan: item_id(0) name(1) unid_name(2) id_name(3) item_type(4) use_type(5)
			// material(6) weight(7) invgfx(8) grdgfx(9) itemdesc_id(10)
			// stackable(11) max_charge_count(12) dmg_small(13) dmg_large(14)
			// min_lvl(15) max_lvl(16) locx(17) locy(18) mapid(19)
			// bless(20) trade(21) cant_delete(22) can_seal(23)
			// delay_id(24) delay_time(25) delay_effect(26) food_volume(27) save_at_once(28)
			if len(r) < 29 {
				continue
			}
			entry = etcItemEntryYAML{
				ItemID:         parseInt32(r[0]),
				Name:           r[1],
				ItemType:       r[4],
				UseType:        r[5],
				Material:       r[6],
				Weight:         parseInt32(r[7]),
				InvGfx:         parseInt32(r[8]),
				GrdGfx:         parseInt32(r[9]),
				ItemDescID:     parseInt32(r[10]),
				Stackable:      parseBool01(r[11]),
				MaxChargeCount: parseInt(r[12]),
				DmgSmall:       parseInt(r[13]),
				DmgLarge:       parseInt(r[14]),
				MinLevel:       parseInt(r[15]),
				MaxLevel:       parseInt(r[16]),
				LocX:           parseInt32(r[17]),
				LocY:           parseInt32(r[18]),
				MapID:          parseInt32(r[19]),
				Bless:          parseInt(r[20]),
				Tradeable:      parseBool01(r[21]),
				CantDelete:     parseBool01(r[22]),
				CanSeal:        parseBool01(r[23]),
				DelayID:        parseInt(r[24]),
				DelayTime:      parseInt(r[25]),
				DelayEffect:    parseInt(r[26]),
				FoodVolume:     parseInt(r[27]),
				SaveAtOnce:     parseBool01(r[28]),
			}
		}
		items = append(items, entry)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ItemID < items[j].ItemID })
	fmt.Printf("  etcitem: %d entries\n", len(items))
	return writeYAML(filepath.Join(outDir, "etcitem_list.yaml"),
		etcItemListYAML{Items: items},
		"# EtcItem definitions - converted from etcitem.sql")
}

func convertItems(sqlDir, outDir string) error {
	if err := convertWeapons(sqlDir, outDir); err != nil {
		return fmt.Errorf("weapon: %w", err)
	}
	if err := convertArmors(sqlDir, outDir); err != nil {
		return fmt.Errorf("armor: %w", err)
	}
	if err := convertEtcItems(sqlDir, outDir); err != nil {
		return fmt.Errorf("etcitem: %w", err)
	}
	return nil
}

func convertMobSkill(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "mobskill.sql"))
	if err != nil {
		return err
	}
	groups := make(map[int32][]mobSkillEntryYAML)
	for _, r := range rows {
		mobID := parseInt32(r[0])
		var entry mobSkillEntryYAML
		if sqlFormat == "yiwei" {
			// Yiwei: mobid(0) actNo(1) mobname(2) Type(3) TriRnd(4) TriHp(5)
			// TriCompanionHp(6) TriRange(7) TriCount(8) ChangeTarget(9)
			// Range(10) AreaWidth(11) AreaHeight(12) Leverage(13)
			// SkillId(14) Gfxid(15) ActId(16) SummonId(17) SummonMin(18)
			// SummonMax(19) PolyId(20) reuseDelay(21)
			// Yiwei lacks: mpConsume, SkillArea
			if len(r) < 22 {
				continue
			}
			entry = mobSkillEntryYAML{
				ActNo:              parseInt(r[1]),
				Name:               r[2],
				Type:               parseInt(r[3]),
				MpConsume:          0, // not in yiwei
				TriggerRandom:      parseInt(r[4]),
				TriggerHP:          parseInt(r[5]),
				TriggerCompanionHP: parseInt(r[6]),
				TriggerRange:       parseInt(r[7]),
				TriggerCount:       parseInt(r[8]),
				ChangeTarget:       parseInt(r[9]),
				Range:              parseInt(r[10]),
				AreaWidth:          parseInt(r[11]),
				AreaHeight:         parseInt(r[12]),
				Leverage:           parseInt(r[13]),
				SkillID:            parseInt32(r[14]),
				SkillArea:          0, // not in yiwei
				GfxID:              parseInt32(r[15]),
				ActID:              parseInt(r[16]),
				SummonID:           parseInt32(r[17]),
				SummonMin:          parseInt(r[18]),
				SummonMax:          parseInt(r[19]),
				PolyID:             parseInt32(r[20]),
				ReuseDelay:         parseInt(r[21]),
			}
		} else {
			// Taiwan: mobid(0) actNo(1) mobname(2) Type(3) mpConsume(4)
			// TriRnd(5) TriHp(6) TriCompanionHp(7) TriRange(8) TriCount(9)
			// ChangeTarget(10) Range(11) AreaWidth(12) AreaHeight(13) Leverage(14)
			// SkillId(15) SkillArea(16) Gfxid(17) ActId(18)
			// SummonId(19) SummonMin(20) SummonMax(21) PolyId(22)
			if len(r) < 23 {
				continue
			}
			entry = mobSkillEntryYAML{
				ActNo:              parseInt(r[1]),
				Name:               r[2],
				Type:               parseInt(r[3]),
				MpConsume:          parseInt(r[4]),
				TriggerRandom:      parseInt(r[5]),
				TriggerHP:          parseInt(r[6]),
				TriggerCompanionHP: parseInt(r[7]),
				TriggerRange:       parseInt(r[8]),
				TriggerCount:       parseInt(r[9]),
				ChangeTarget:       parseInt(r[10]),
				Range:              parseInt(r[11]),
				AreaWidth:          parseInt(r[12]),
				AreaHeight:         parseInt(r[13]),
				Leverage:           parseInt(r[14]),
				SkillID:            parseInt32(r[15]),
				SkillArea:          parseInt(r[16]),
				GfxID:              parseInt32(r[17]),
				ActID:              parseInt(r[18]),
				SummonID:           parseInt32(r[19]),
				SummonMin:          parseInt(r[20]),
				SummonMax:          parseInt(r[21]),
				PolyID:             parseInt32(r[22]),
			}
		}
		groups[mobID] = append(groups[mobID], entry)
	}
	var mobIDs []int32
	for id := range groups {
		mobIDs = append(mobIDs, id)
	}
	sort.Slice(mobIDs, func(i, j int) bool { return mobIDs[i] < mobIDs[j] })
	var result []mobSkillGroupYAML
	for _, id := range mobIDs {
		result = append(result, mobSkillGroupYAML{MobID: id, Skills: groups[id]})
	}
	fmt.Printf("  mobskill: %d mobs, %d total skills\n", len(result), len(rows))
	return writeYAML(filepath.Join(outDir, "mob_skill_list.yaml"),
		mobSkillListYAML{MobSkills: result},
		"# Monster skill list - converted from mobskill.sql")
}

func convertNpcAction(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "npcaction.sql"))
	if err != nil {
		return err
	}
	// npcaction columns: 0:npcid 1:normal_action 2:caotic_action 3:teleport_url 4:teleport_urla
	var actions []npcActionEntryYAML
	for _, r := range rows {
		if len(r) < 5 {
			continue
		}
		actions = append(actions, npcActionEntryYAML{
			NpcID:        parseInt32(r[0]),
			NormalAction: r[1],
			CaoticAction: r[2],
			TeleportURL:  r[3],
			TeleportURLA: r[4],
		})
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].NpcID < actions[j].NpcID })
	fmt.Printf("  npcaction: %d entries\n", len(actions))
	return writeYAML(filepath.Join(outDir, "npc_action_list.yaml"),
		npcActionListYAML{Actions: actions},
		"# NPC dialog actions - converted from npcaction.sql")
}

// ---------------------------------------------------------------------------
// Additional converters (portal, polymorph, spr, door, pettype, petitem, teleport, doll, itemmaking)
// ---------------------------------------------------------------------------

func convertPortal(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "dungeon.sql"))
	if err != nil {
		return err
	}
	// dungeon: src_x(0) src_y(1) src_mapid(2) new_x(3) new_y(4) new_mapid(5) new_heading(6) note(7)
	type portalYAML struct {
		SrcX       int32  `yaml:"src_x"`
		SrcY       int32  `yaml:"src_y"`
		SrcMapID   int16  `yaml:"src_map_id"`
		DstX       int32  `yaml:"dst_x"`
		DstY       int32  `yaml:"dst_y"`
		DstMapID   int16  `yaml:"dst_map_id"`
		DstHeading int16  `yaml:"dst_heading"`
		Note       string `yaml:"note"`
	}
	var portals []portalYAML
	for _, r := range rows {
		if len(r) < 7 {
			continue
		}
		note := ""
		if len(r) > 7 {
			note = r[7]
		}
		portals = append(portals, portalYAML{
			SrcX:       parseInt32(r[0]),
			SrcY:       parseInt32(r[1]),
			SrcMapID:   parseInt16(r[2]),
			DstX:       parseInt32(r[3]),
			DstY:       parseInt32(r[4]),
			DstMapID:   parseInt16(r[5]),
			DstHeading: parseInt16(r[6]),
			Note:       note,
		})
	}
	fmt.Printf("  portal: %d entries\n", len(portals))
	// PortalTable loader expects a flat array (no wrapper key)
	return writeYAML(filepath.Join(outDir, "portal_list.yaml"), portals,
		"# Portal list — converted from dungeon.sql")
}

func convertPolymorph(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "polymorphs.sql"))
	if err != nil {
		return err
	}
	// polymorphs: id(0) name(1) polyid(2) minlevel(3) weaponequip(4) armorequip(5)
	// isSkillUse(6) cause(7) note(8) 轉生限制(9)
	type polyEntry struct {
		PolyID      int32  `yaml:"poly_id"`
		Name        string `yaml:"name"`
		MinLevel    int    `yaml:"min_level"`
		WeaponEquip int    `yaml:"weapon_equip"`
		ArmorEquip  int    `yaml:"armor_equip"`
		CanUseSkill bool   `yaml:"can_use_skill"`
		Cause       int    `yaml:"cause"`
	}
	type polyFile struct {
		Polymorphs []polyEntry `yaml:"polymorphs"`
	}
	var polys []polyEntry
	for _, r := range rows {
		if len(r) < 8 {
			continue
		}
		polys = append(polys, polyEntry{
			PolyID:      parseInt32(r[2]),
			Name:        r[1],
			MinLevel:    parseInt(r[3]),
			WeaponEquip: parseInt(r[4]),
			ArmorEquip:  parseInt(r[5]),
			CanUseSkill: parseBool01(r[6]),
			Cause:       parseInt(r[7]),
		})
	}
	sort.Slice(polys, func(i, j int) bool { return polys[i].PolyID < polys[j].PolyID })
	fmt.Printf("  polymorph: %d entries\n", len(polys))
	return writeYAML(filepath.Join(outDir, "polymorph_list.yaml"),
		polyFile{Polymorphs: polys},
		"# Polymorph form definitions - converted from polymorphs.sql")
}

func convertSpr(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "spr_action.sql"))
	if err != nil {
		return err
	}
	// spr_action: spr_id(0) act_id(1) framecount(2) framerate(3)
	type sprEntry struct {
		SprID      int `yaml:"spr_id"`
		ActID      int `yaml:"act_id"`
		FrameCount int `yaml:"framecount"`
		FrameRate  int `yaml:"framerate"`
	}
	type sprFile struct {
		Actions []sprEntry `yaml:"spr_actions"`
	}
	var actions []sprEntry
	for _, r := range rows {
		if len(r) < 4 {
			continue
		}
		actions = append(actions, sprEntry{
			SprID:      parseInt(r[0]),
			ActID:      parseInt(r[1]),
			FrameCount: parseInt(r[2]),
			FrameRate:  parseInt(r[3]),
		})
	}
	fmt.Printf("  spr_action: %d entries\n", len(actions))
	return writeYAML(filepath.Join(outDir, "spr_action.yaml"),
		sprFile{Actions: actions}, "")
}

func convertDoor(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "spawnlist_door.sql"))
	if err != nil {
		return err
	}
	// spawnlist_door: id(0) location(1) gfxid(2) locx(3) locy(4) mapid(5)
	// direction(6) left_edge_location(7) right_edge_location(8) hp(9) keeper(10)
	type doorSpawnYAML struct {
		ID        int32 `yaml:"id"`
		GfxID     int32 `yaml:"gfxid"`
		X         int32 `yaml:"x"`
		Y         int32 `yaml:"y"`
		MapID     int16 `yaml:"map_id"`
		HP        int32 `yaml:"hp"`
		Keeper    int32 `yaml:"keeper"`
		IsOpening bool  `yaml:"is_opening"`
	}
	type doorGfxYAML struct {
		GfxID           int32  `yaml:"gfxid"`
		Note            string `yaml:"note"`
		Direction       int    `yaml:"direction"`
		LeftEdgeOffset  int    `yaml:"left_edge_offset"`
		RightEdgeOffset int    `yaml:"right_edge_offset"`
	}
	type doorSpawnFile struct {
		Doors []doorSpawnYAML `yaml:"doors"`
	}
	type doorGfxFile struct {
		DoorGfxs []doorGfxYAML `yaml:"door_gfxs"`
	}

	var spawns []doorSpawnYAML
	gfxMap := make(map[int32]doorGfxYAML)
	for _, r := range rows {
		if len(r) < 10 {
			continue
		}
		gfxID := parseInt32(r[2])
		keeper := int32(0)
		if len(r) > 10 {
			keeper = parseInt32(r[10])
		}
		spawns = append(spawns, doorSpawnYAML{
			ID:        parseInt32(r[0]),
			GfxID:     gfxID,
			X:         parseInt32(r[3]),
			Y:         parseInt32(r[4]),
			MapID:     parseInt16(r[5]),
			HP:        parseInt32(r[9]),
			Keeper:    keeper,
			IsOpening: true,
		})
		if _, exists := gfxMap[gfxID]; !exists {
			gfxMap[gfxID] = doorGfxYAML{
				GfxID:           gfxID,
				Note:            r[1],
				Direction:       parseInt(r[6]),
				LeftEdgeOffset:  parseInt(r[7]),
				RightEdgeOffset: parseInt(r[8]),
			}
		}
	}
	sort.Slice(spawns, func(i, j int) bool { return spawns[i].ID < spawns[j].ID })
	fmt.Printf("  door_spawn: %d entries\n", len(spawns))
	if err := writeYAML(filepath.Join(outDir, "door_spawn.yaml"),
		doorSpawnFile{Doors: spawns}, ""); err != nil {
		return err
	}

	// door_gfx.yaml
	var gfxIDs []int32
	for id := range gfxMap {
		gfxIDs = append(gfxIDs, id)
	}
	sort.Slice(gfxIDs, func(i, j int) bool { return gfxIDs[i] < gfxIDs[j] })
	var gfxList []doorGfxYAML
	for _, id := range gfxIDs {
		gfxList = append(gfxList, gfxMap[id])
	}
	fmt.Printf("  door_gfx: %d entries\n", len(gfxList))
	return writeYAML(filepath.Join(outDir, "door_gfx.yaml"),
		doorGfxFile{DoorGfxs: gfxList}, "")
}

func convertPetType(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "pettypes.sql"))
	if err != nil {
		return err
	}
	// pettypes: BaseNpcId(0) Name(1) 是否裝備(2) ItemIdForTaming(3)
	// HpUpMin(4) HpUpMax(5) MpUpMin(6) MpUpMax(7)
	// EvolvItemId(8) NpcIdForEvolving(9) MessageId1-5(10-14) DefyMessageId(15)
	type petTypeYAML struct {
		BaseNpcID    int32  `yaml:"base_npc_id"`
		Name         string `yaml:"name"`
		CanEquip     bool   `yaml:"can_equip"`
		TamingItemID int32  `yaml:"taming_item_id"`
		HPUpMin      int    `yaml:"hp_up_min"`
		HPUpMax      int    `yaml:"hp_up_max"`
		MPUpMin      int    `yaml:"mp_up_min"`
		MPUpMax      int    `yaml:"mp_up_max"`
		EvolvItemID  int32  `yaml:"evolv_item_id"`
		EvolvNpcID   int32  `yaml:"evolv_npc_id"`
		MsgIDs       []int  `yaml:"msg_ids"`
		DefyMsgID    int    `yaml:"defy_msg_id"`
	}
	type petTypeFile struct {
		PetTypes []petTypeYAML `yaml:"pet_types"`
	}
	var pets []petTypeYAML
	for _, r := range rows {
		if len(r) < 16 {
			continue
		}
		msgIDs := []int{parseInt(r[10]), parseInt(r[11]), parseInt(r[12]), parseInt(r[13]), parseInt(r[14])}
		pets = append(pets, petTypeYAML{
			BaseNpcID:    parseInt32(r[0]),
			Name:         r[1],
			CanEquip:     parseBool01(r[2]),
			TamingItemID: parseInt32(r[3]),
			HPUpMin:      parseInt(r[4]),
			HPUpMax:      parseInt(r[5]),
			MPUpMin:      parseInt(r[6]),
			MPUpMax:      parseInt(r[7]),
			EvolvItemID:  parseInt32(r[8]),
			EvolvNpcID:   parseInt32(r[9]),
			MsgIDs:       msgIDs,
			DefyMsgID:    parseInt(r[15]),
		})
	}
	sort.Slice(pets, func(i, j int) bool { return pets[i].BaseNpcID < pets[j].BaseNpcID })
	fmt.Printf("  pettype: %d entries\n", len(pets))
	return writeYAML(filepath.Join(outDir, "pet_types.yaml"),
		petTypeFile{PetTypes: pets},
		"# Pet type definitions - converted from pettypes.sql")
}

func convertPetItem(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "petitem.sql"))
	if err != nil {
		return err
	}
	// petitem: item_id(0) note(1) 裝備屬性(2) hitmodifier(3) dmgmodifier(4) ac(5)
	// add_str(6) add_con(7) add_dex(8) add_int(9) add_wis(10)
	// add_hp(11) add_mp(12) add_sp(13) m_def(14) isweapon(15) ishigher(16)
	type petItemYAML struct {
		ItemID   int32  `yaml:"item_id"`
		Name     string `yaml:"name"`
		UseType  string `yaml:"use_type"`
		IsHigher bool   `yaml:"is_higher"`
		Hit      int    `yaml:"hit"`
		Dmg      int    `yaml:"dmg"`
		AC       int    `yaml:"ac"`
		AddStr   int    `yaml:"add_str"`
		AddCon   int    `yaml:"add_con"`
		AddDex   int    `yaml:"add_dex"`
		AddInt   int    `yaml:"add_int"`
		AddWis   int    `yaml:"add_wis"`
		AddHP    int    `yaml:"add_hp"`
		AddMP    int    `yaml:"add_mp"`
		AddSP    int    `yaml:"add_sp"`
		MDef     int    `yaml:"m_def"`
	}
	type petItemFile struct {
		PetItems []petItemYAML `yaml:"pet_items"`
	}
	var items []petItemYAML
	for _, r := range rows {
		if len(r) < 17 {
			continue
		}
		useType := "armor"
		if parseBool01(r[15]) {
			useType = "tooth"
		}
		items = append(items, petItemYAML{
			ItemID:   parseInt32(r[0]),
			Name:     r[1],
			UseType:  useType,
			IsHigher: parseBool01(r[16]),
			Hit:      parseInt(r[3]),
			Dmg:      parseInt(r[4]),
			AC:       parseInt(r[5]),
			AddStr:   parseInt(r[6]),
			AddCon:   parseInt(r[7]),
			AddDex:   parseInt(r[8]),
			AddInt:   parseInt(r[9]),
			AddWis:   parseInt(r[10]),
			AddHP:    parseInt(r[11]),
			AddMP:    parseInt(r[12]),
			AddSP:    parseInt(r[13]),
			MDef:     parseInt(r[14]),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ItemID < items[j].ItemID })
	fmt.Printf("  petitem: %d entries\n", len(items))
	return writeYAML(filepath.Join(outDir, "pet_items.yaml"),
		petItemFile{PetItems: items},
		"# Pet equipment definitions - converted from petitem.sql")
}

// --- XML structs for Teleporter.xml ---
type xmlNpcActionList struct {
	XMLName   xml.Name      `xml:"NpcActionList"`
	Actions   []xmlAction   `xml:"Action"`
	ShowHtmls []xmlShowHtml `xml:"ShowHtml"`
	MakeItems []xmlMakeItem `xml:"MakeItem"` // standalone MakeItem (e.g., Tower of Insolence)
}
type xmlAction struct {
	Name     string       `xml:"Name,attr"`
	NpcId    string       `xml:"NpcId,attr"`
	LevelMin int          `xml:"LevelMin,attr"`
	LevelMax int          `xml:"LevelMax,attr"`
	Teleport *xmlTeleport `xml:"Teleport"`
	MakeItem *xmlMakeItem `xml:"MakeItem"` // nested MakeItem (item-gated teleport)
}
type xmlTeleport struct {
	X       int32 `xml:"X,attr"`
	Y       int32 `xml:"Y,attr"`
	Map     int16 `xml:"Map,attr"`
	Heading int16 `xml:"Heading,attr"`
	Price   int32 `xml:"Price,attr"`
}
type xmlShowHtml struct {
	Name   string    `xml:"Name,attr"`
	HtmlId string    `xml:"HtmlId,attr"`
	NpcId  string    `xml:"NpcId,attr"`
	Data   []xmlData `xml:"Data"`
}
type xmlData struct {
	Value string `xml:"Value,attr"`
}
type xmlMakeItem struct {
	Name    string      `xml:"Name,attr"`
	NpcId   string      `xml:"NpcId,attr"`
	Succeed *xmlSucceed `xml:"Succeed"`
}
type xmlSucceed struct {
	Teleport *xmlTeleport `xml:"Teleport"`
}

// parseNpcIds splits a potentially comma-separated NpcId string into int32 slice.
func parseNpcIds(s string) []int32 {
	parts := strings.Split(s, ",")
	var ids []int32
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseInt(p, 10, 32)
		if err != nil {
			continue
		}
		ids = append(ids, int32(v))
	}
	return ids
}

func convertTeleport(sqlDir, outDir string) error {
	// Derive XML path: sqlDir is .../db_split, XML is in .../data/xml/NpcActions/
	xmlPath := filepath.Join(filepath.Dir(sqlDir), "data", "xml", "NpcActions", "Teleporter.xml")

	raw, err := os.ReadFile(xmlPath)
	if err != nil {
		return fmt.Errorf("read Teleporter.xml: %w (expected at %s)", err, xmlPath)
	}

	var actionList xmlNpcActionList
	if err := xml.Unmarshal(raw, &actionList); err != nil {
		return fmt.Errorf("parse Teleporter.xml: %w", err)
	}

	type teleportDestYAML struct {
		Action  string `yaml:"action"`
		NpcID   int32  `yaml:"npc_id"`
		X       int32  `yaml:"x"`
		Y       int32  `yaml:"y"`
		MapID   int16  `yaml:"map_id"`
		Heading int16  `yaml:"heading"`
		Price   int32  `yaml:"price"`
	}
	type teleportFile struct {
		Teleports []teleportDestYAML `yaml:"teleports"`
	}
	type teleportHtmlYAML struct {
		ActionName string   `yaml:"action_name"`
		HtmlID     string   `yaml:"html_id"`
		NpcID      int32    `yaml:"npc_id"`
		Data       []string `yaml:"data"`
	}
	type teleportHtmlFile struct {
		HtmlData []teleportHtmlYAML `yaml:"html_data"`
	}

	var teleports []teleportDestYAML
	var htmlData []teleportHtmlYAML
	// Dedup key: (npcID, action)
	type teleKey struct {
		npcID  int32
		action string
	}
	seen := make(map[teleKey]bool)

	// Process <Action> elements
	for _, act := range actionList.Actions {
		npcIds := parseNpcIds(act.NpcId)

		// Direct <Teleport> child
		if act.Teleport != nil {
			for _, nid := range npcIds {
				key := teleKey{npcID: nid, action: act.Name}
				if seen[key] {
					continue
				}
				seen[key] = true
				teleports = append(teleports, teleportDestYAML{
					Action:  act.Name,
					NpcID:   nid,
					X:       act.Teleport.X,
					Y:       act.Teleport.Y,
					MapID:   act.Teleport.Map,
					Heading: act.Teleport.Heading,
					Price:   act.Teleport.Price,
				})
			}
		}

		// Nested <MakeItem> with <Succeed><Teleport> (item-gated teleport)
		if act.MakeItem != nil && act.MakeItem.Succeed != nil && act.MakeItem.Succeed.Teleport != nil {
			tp := act.MakeItem.Succeed.Teleport
			for _, nid := range npcIds {
				key := teleKey{npcID: nid, action: act.Name}
				if seen[key] {
					continue
				}
				seen[key] = true
				teleports = append(teleports, teleportDestYAML{
					Action:  act.Name,
					NpcID:   nid,
					X:       tp.X,
					Y:       tp.Y,
					MapID:   tp.Map,
					Heading: tp.Heading,
					Price:   tp.Price,
				})
			}
		}
	}

	// Process standalone <MakeItem> elements (e.g., Tower of Insolence NPC 81297)
	for _, mi := range actionList.MakeItems {
		if mi.Succeed == nil || mi.Succeed.Teleport == nil {
			continue
		}
		npcIds := parseNpcIds(mi.NpcId)
		tp := mi.Succeed.Teleport
		for _, nid := range npcIds {
			key := teleKey{npcID: nid, action: mi.Name}
			if seen[key] {
				continue
			}
			seen[key] = true
			teleports = append(teleports, teleportDestYAML{
				Action:  mi.Name,
				NpcID:   nid,
				X:       tp.X,
				Y:       tp.Y,
				MapID:   tp.Map,
				Heading: tp.Heading,
				Price:   tp.Price,
			})
		}
	}

	// Process <ShowHtml> elements
	for _, sh := range actionList.ShowHtmls {
		npcIds := parseNpcIds(sh.NpcId)
		var data []string
		for _, d := range sh.Data {
			data = append(data, d.Value)
		}
		for _, nid := range npcIds {
			htmlData = append(htmlData, teleportHtmlYAML{
				ActionName: sh.Name,
				HtmlID:     sh.HtmlId,
				NpcID:      nid,
				Data:       data,
			})
		}
	}

	sort.Slice(teleports, func(i, j int) bool {
		if teleports[i].NpcID != teleports[j].NpcID {
			return teleports[i].NpcID < teleports[j].NpcID
		}
		return teleports[i].Action < teleports[j].Action
	})
	fmt.Printf("  teleport: %d entries (from %d XML Actions + %d MakeItems)\n",
		len(teleports), len(actionList.Actions), len(actionList.MakeItems))
	if err := writeYAML(filepath.Join(outDir, "teleport_list.yaml"),
		teleportFile{Teleports: teleports},
		"# Teleport destinations - converted from Teleporter.xml"); err != nil {
		return err
	}

	sort.Slice(htmlData, func(i, j int) bool {
		if htmlData[i].NpcID != htmlData[j].NpcID {
			return htmlData[i].NpcID < htmlData[j].NpcID
		}
		if htmlData[i].ActionName != htmlData[j].ActionName {
			return htmlData[i].ActionName < htmlData[j].ActionName
		}
		return htmlData[i].HtmlID < htmlData[j].HtmlID
	})
	fmt.Printf("  teleport_html: %d entries (from %d XML ShowHtmls)\n",
		len(htmlData), len(actionList.ShowHtmls))
	return writeYAML(filepath.Join(outDir, "teleport_html.yaml"),
		teleportHtmlFile{HtmlData: htmlData},
		"# Teleport HTML data values - converted from Teleporter.xml")
}

func convertDoll(sqlDir, outDir string) error {
	// Step 1: Load doll power definitions
	powerRows, err := parseAllInserts(filepath.Join(sqlDir, "etcitem_doll_power.sql"))
	if err != nil {
		return err
	}
	// etcitem_doll_power: id(0) classname(1) type1(2) type2(3) type3(4) note(5)
	type powerDef struct {
		className string
		type1     int
		type2     int
		type3     int
	}
	powers := make(map[int]powerDef)
	for _, r := range powerRows {
		if len(r) < 5 {
			continue
		}
		powers[parseInt(r[0])] = powerDef{
			className: r[1],
			type1:     parseInt(r[2]),
			type2:     parseInt(r[3]),
			type3:     parseInt(r[4]),
		}
	}

	// Java classname → Go power type mapping
	// Includes both Taiwan and yiwei naming conventions.
	classToType := map[string]string{
		// Core stats (Taiwan: Doll_Str, yiwei: Doll_Stat_Str)
		"Doll_Ac": "ac", "Doll_Hp": "hp", "Doll_Mp": "mp",
		"Doll_Str": "str", "Doll_Dex": "dex", "Doll_Con": "con",
		"Doll_Wis": "wis", "Doll_Int": "int", "Doll_Cha": "cha",
		"Doll_Stat_Str": "str", "Doll_Stat_Dex": "dex", "Doll_Stat_Con": "con",
		"Doll_Stat_Wis": "wis", "Doll_Stat_Int": "int", "Doll_Stat_Cha": "cha",
		// Combat (Taiwan: Doll_BowHit, yiwei: Doll_HitBow)
		"Doll_Hit": "hit", "Doll_Dmg": "dmg", "Doll_Mr": "mr", "Doll_Sp": "sp",
		"Doll_BowHit": "bow_hit", "Doll_BowDmg": "bow_dmg",
		"Doll_HitBow": "bow_hit", "Doll_DmgBow": "bow_dmg",
		"Doll_DmgR": "dmg_rate", "Doll_DmgDown": "dmg_reduction",
		"Doll_DmgReduction": "dmg_reduction", "Doll_DmgDownR": "dmg_reduction_rate",
		"Doll_FoeSlayerDmg": "foe_slayer_dmg", "Doll_Skill_Hit": "skill_hit",
		"Doll_StunLevel": "stun_level", "Doll_ArmorBreakLevel": "armor_break",
		"Doll_TatinHP": "tatin_hp",
		// Regen (Taiwan: Doll_HpRegen, yiwei: Doll_HpTR)
		"Doll_HpRegen": "hpr", "Doll_MpRegen": "mpr", "Doll_MpR": "mpr",
		"Doll_HpRegenTick": "hp_regen_tick", "Doll_MpRegenTick": "mp_regen_tick",
		"Doll_HpTR": "hp_regen_tick", "Doll_MpTR": "mp_regen_tick",
		// Elemental resist (Taiwan: Doll_FireRes, yiwei: Doll_DefenseFire)
		"Doll_FireRes": "fire_res", "Doll_WaterRes": "water_res",
		"Doll_WindRes": "wind_res", "Doll_EarthRes": "earth_res",
		"Doll_DefenseFire": "fire_res", "Doll_DefenseWater": "water_res",
		"Doll_DefenseWind": "wind_res", "Doll_DefenseEarth": "earth_res",
		"Doll_Water": "water_res",
		// Status resist (Taiwan: Doll_StunResist, yiwei: Doll_Regist_Stun)
		"Doll_StunResist": "stun_resist", "Doll_FreezeResist": "freeze_resist",
		"Doll_SleepResist": "sleep_resist", "Doll_StoneResist": "stone_resist",
		"Doll_BlindResist": "blind_resist",
		"Doll_Regist_Stun": "stun_resist", "Doll_Regist_Freeze": "freeze_resist",
		"Doll_Regist_Sustain": "sustain_resist",
		// Utility / evasion (Taiwan: Doll_Dodge, yiwei: Doll_Evasion)
		"Doll_Dodge": "dodge", "Doll_Evasion": "dodge",
		"Doll_Weight": "weight", "Doll_Exp": "exp",
		// Skill
		"Doll_Skill": "skill", "Doll_Add_Skill": "skill",
		"Doll_Speed": "speed",
	}

	// Step 2: Load doll type definitions
	typeRows, err := parseAllInserts(filepath.Join(sqlDir, "etcitem_doll_type.sql"))
	if err != nil {
		return err
	}
	// etcitem_doll_type: itemid(0) note(1) powers(2) need(3) count(4) time(5) gfxid(6) nameid(7)
	type dollPowerYAML struct {
		Type   string `yaml:"type"`
		Value  int    `yaml:"value"`
		Param  int    `yaml:"param,omitempty"`
		Chance int    `yaml:"chance,omitempty"`
	}
	type dollYAML struct {
		ItemID   int32           `yaml:"item_id"`
		GfxID    int32           `yaml:"gfx_id"`
		NameID   string          `yaml:"name_id"`
		Name     string          `yaml:"name"`
		Duration int             `yaml:"duration"`
		Tier     int             `yaml:"tier"`
		Powers   []dollPowerYAML `yaml:"powers"`
	}
	type dollFile struct {
		Dolls []dollYAML `yaml:"dolls"`
	}

	var dolls []dollYAML
	for _, r := range typeRows {
		if len(r) < 8 {
			continue
		}
		itemID := parseInt32(r[0])
		nameID := r[7]
		name := r[1]
		duration := parseInt(r[5])
		gfxID := parseInt32(r[6])

		// Parse CSV powers field
		var dollPowers []dollPowerYAML
		powerIDsStr := strings.TrimSpace(r[2])
		if powerIDsStr != "" {
			for _, idStr := range strings.Split(powerIDsStr, ",") {
				idStr = strings.TrimSpace(idStr)
				pID := parseInt(idStr)
				if pID == 0 {
					continue
				}
				pd, ok := powers[pID]
				if !ok {
					continue
				}
				goType, ok := classToType[pd.className]
				if !ok {
					goType = pd.className // fallback
				}
				dp := dollPowerYAML{Type: goType, Value: pd.type1}
				if pd.type2 != 0 {
					dp.Param = pd.type2
				}
				if pd.type3 != 0 {
					dp.Chance = pd.type3
				}
				dollPowers = append(dollPowers, dp)
			}
		}

		// Estimate tier from power count
		tier := 1
		if len(dollPowers) >= 4 {
			tier = 3
		} else if len(dollPowers) >= 2 {
			tier = 2
		}

		dolls = append(dolls, dollYAML{
			ItemID:   itemID,
			GfxID:    gfxID,
			NameID:   nameID,
			Name:     name,
			Duration: duration,
			Tier:     tier,
			Powers:   dollPowers,
		})
	}
	sort.Slice(dolls, func(i, j int) bool { return dolls[i].ItemID < dolls[j].ItemID })
	fmt.Printf("  doll: %d entries (from %d powers)\n", len(dolls), len(powers))
	return writeYAML(filepath.Join(outDir, "dolls.yaml"),
		dollFile{Dolls: dolls},
		"# Magic doll definitions - converted from etcitem_doll_type + etcitem_doll_power")
}

func convertItemMaking(sqlDir, outDir string) error {
	rows, err := parseAllInserts(filepath.Join(sqlDir, "item_making.sql"))
	if err != nil {
		return err
	}
	// 道具製造系統: id(0) npcid(1) npcname(2) action(3) new_item(4) note(5)
	// new_item_counts(6) new_Enchantlvl_SW(7) new_item_Enchantlvl(8) new_item_Bless(9)
	// bonus_item(10) bonus_item_count(11) bonus_item_enchant(12)
	// checkLevel(13) checkClass(14) rnd(15) hpConsume(16) mpConsume(17)
	// inputaddrnd(18) addchanceitem(19) addchance(20) addmaxcount(21)
	// materials(22) materials_count(23) materials_enchants(24)
	// residue_item(25) residue_count(26) residue_enchant(27)
	// replacement_count(28) input_amount(29) all_in_once(30)
	// sucess_html(31) fail_html(32) 公告(33)
	type craftMat struct {
		ItemID int32 `yaml:"item_id"`
		Amount int32 `yaml:"amount"`
	}
	type craftOut struct {
		ItemID int32 `yaml:"item_id"`
		Amount int32 `yaml:"amount"`
	}
	type recipeYAML struct {
		Action          string     `yaml:"action"`
		NpcID           int32      `yaml:"npc_id"`
		AmountInputable bool       `yaml:"amount_inputable"`
		Items           []craftOut `yaml:"items"`
		Materials       []craftMat `yaml:"materials"`
	}
	type recipeFile struct {
		Recipes []recipeYAML `yaml:"recipes"`
	}

	var recipes []recipeYAML
	for _, r := range rows {
		if len(r) < 30 {
			continue
		}
		action := r[3]
		npcID := parseInt32(r[1])
		amountInputable := parseBool01(r[29])

		// Parse output items (CSV)
		var items []craftOut
		newItemStr := strings.TrimSpace(r[4])
		newCountStr := strings.TrimSpace(r[6])
		if newItemStr != "" {
			itemIDs := strings.Split(newItemStr, ",")
			counts := strings.Split(newCountStr, ",")
			for i, idStr := range itemIDs {
				idStr = strings.TrimSpace(idStr)
				id := parseInt32(idStr)
				if id == 0 {
					continue
				}
				amount := int32(1)
				if i < len(counts) {
					amount = parseInt32(strings.TrimSpace(counts[i]))
					if amount == 0 {
						amount = 1
					}
				}
				items = append(items, craftOut{ItemID: id, Amount: amount})
			}
		}

		// Parse materials (CSV)
		var materials []craftMat
		matStr := strings.TrimSpace(r[22])
		matCountStr := strings.TrimSpace(r[23])
		if matStr != "" {
			matIDs := strings.Split(matStr, ",")
			matCounts := strings.Split(matCountStr, ",")
			for i, idStr := range matIDs {
				idStr = strings.TrimSpace(idStr)
				id := parseInt32(idStr)
				if id == 0 {
					continue
				}
				amount := int32(1)
				if i < len(matCounts) {
					amount = parseInt32(strings.TrimSpace(matCounts[i]))
					if amount == 0 {
						amount = 1
					}
				}
				materials = append(materials, craftMat{ItemID: id, Amount: amount})
			}
		}

		recipes = append(recipes, recipeYAML{
			Action:          action,
			NpcID:           npcID,
			AmountInputable: amountInputable,
			Items:           items,
			Materials:       materials,
		})
	}
	fmt.Printf("  item_making: %d recipes\n", len(recipes))
	return writeYAML(filepath.Join(outDir, "item_making_list.yaml"),
		recipeFile{Recipes: recipes},
		"# Item crafting recipes - converted from item_making.sql")
}

// ---------------------------------------------------------------------------
// Format flag — "taiwan" (default) or "yiwei"
// ---------------------------------------------------------------------------

var sqlFormat string

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func printUsage() {
	fmt.Println("Usage: sqlconv <command> [-sqldir path] [-outdir path] [-format taiwan|yiwei]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  npc       Convert npc.sql -> npc_list.yaml")
	fmt.Println("  spawn     Convert spawnlist.sql + spawnlist_npc.sql -> spawn_list.yaml")
	fmt.Println("  drop      Convert droplist.sql -> drop_list.yaml")
	fmt.Println("  dropitem  Convert drop_item.sql -> drop_item.yaml")
	fmt.Println("  shop      Convert shop.sql -> shop_list.yaml")
	fmt.Println("  mapids    Convert mapids.sql -> map_list.yaml")
	fmt.Println("  skills    Convert skills.sql -> skill_list.yaml")
	fmt.Println("  items     Convert weapon/armor/etcitem.sql -> 3 YAML files")
	fmt.Println("  mobskill  Convert mobskill.sql -> mob_skill_list.yaml")
	fmt.Println("  npcaction Convert npcaction.sql -> npc_action_list.yaml")
	fmt.Println("  all       Run all conversions")
	fmt.Println()
	fmt.Println("Formats:")
	fmt.Println("  taiwan    L1JTW SQL dumps (default, individual files in db/Taiwan/)")
	fmt.Println("  yiwei     Yiwei private-server dump (split per-table files)")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		printUsage()
		return
	}

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	sqlDir := fs.String("sqldir", filepath.Join("..", "..", "l1j_java", "db", "Taiwan"), "SQL source directory")
	outDir := fs.String("outdir", filepath.Join("..", "data", "yaml"), "YAML output directory")
	fmtFlag := fs.String("format", "taiwan", "SQL format: taiwan or yiwei")
	_ = fs.Parse(os.Args[2:])
	sqlFormat = *fmtFlag

	converters := map[string]func(string, string) error{
		"npc":        convertNpc,
		"spawn":      convertSpawn,
		"drop":       convertDrop,
		"dropitem":   convertDropItem,
		"shop":       convertShop,
		"mapids":     convertMapIDs,
		"skills":     convertSkills,
		"items":      convertItems,
		"mobskill":   convertMobSkill,
		"npcaction":  convertNpcAction,
		"portal":     convertPortal,
		"polymorph":  convertPolymorph,
		"spr":        convertSpr,
		"door":       convertDoor,
		"pettype":    convertPetType,
		"petitem":    convertPetItem,
		"teleport":   convertTeleport,
		"doll":       convertDoll,
		"itemmaking": convertItemMaking,
	}

	// Ordered list for "all" (deterministic output)
	allOrder := []string{
		"npc", "spawn", "drop", "dropitem", "shop", "mapids", "skills", "items", "mobskill", "npcaction",
		"portal", "polymorph", "spr", "door", "pettype", "petitem", "teleport", "doll", "itemmaking",
	}

	fmt.Printf("Format: %s\n", sqlFormat)

	if cmd == "all" {
		fmt.Println("Converting all SQL -> YAML...")
		for _, name := range allOrder {
			if err := converters[name](*sqlDir, *outDir); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR [%s]: %v\n", name, err)
				os.Exit(1)
			}
		}
		fmt.Println("Done!")
		return
	}

	fn, ok := converters[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
	if err := fn(*sqlDir, *outDir); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Done!")
}

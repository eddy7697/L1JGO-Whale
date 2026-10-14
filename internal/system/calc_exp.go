package system

import (
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// ==================== 擊殺經驗分配（Java: CalcExp.calcExp / AddExp / AddExpPet） ====================

// Java 經驗加成用的技能效果 ID（L1SkillId）。
const (
	skillCooking17N     int32 = 3007 // COOKING_1_7_N
	skillCooking17S     int32 = 3015 // COOKING_1_7_S
	skillCooking27N     int32 = 3023 // COOKING_2_7_N
	skillCooking27S     int32 = 3031 // COOKING_2_7_S
	skillCooking37N     int32 = 3039 // COOKING_3_7_N
	skillCooking37S     int32 = 3047 // COOKING_3_7_S
	skillPotionOfExp150 int32 = 4001 // EFFECT_POTION_OF_EXP_150
	skillPotionOfExp175 int32 = 4002
	skillPotionOfExp200 int32 = 4003
	skillPotionOfExp225 int32 = 4004
	skillPotionOfExp250 int32 = 4005
	skillPotionOfBattle int32 = 4007  // EFFECT_POTION_OF_BATTLE
	partyCrownLeaderExp       = 0.059 // 王族隊長加成（Java: pri_bonus）
	partyMemberExpBonus       = 0.04  // 每位可見隊員加成（Java: pt_bonus）
)

// expAcquisitor 為一位參與擊殺的玩家與其（含寵物、召喚獸）累積仇恨。
type expAcquisitor struct {
	player *world.PlayerInfo
	hate   int64 // HateList 總仇恨（含寵物、召喚獸）
	pcHate int64 // 扣除寵物、召喚獸後玩家本人的仇恨（Java: L1PcInstance 的 hate）
}

// expFoodBonusLikeJava 回傳魔法料理經驗加成（Java: CalcExp.AddExp foodBonus）。
func expFoodBonusLikeJava(p *world.PlayerInfo) float64 {
	bonus := 1.0
	if p.HasBuff(skillCooking17N) || p.HasBuff(skillCooking17S) {
		bonus = 1.01
	}
	if p.HasBuff(skillCooking27N) || p.HasBuff(skillCooking27S) {
		bonus = 1.02
	}
	if p.HasBuff(skillCooking37N) || p.HasBuff(skillCooking37S) {
		bonus = 1.03
	}
	return bonus
}

// expPotionBonusLikeJava 回傳戰鬥藥水、神力藥水經驗加成（Java: CalcExp.AddExp expBonus）。
func expPotionBonusLikeJava(p *world.PlayerInfo) float64 {
	switch {
	case p.HasBuff(skillPotionOfBattle):
		return 1.2
	case p.HasBuff(skillPotionOfExp150):
		return 2.5
	case p.HasBuff(skillPotionOfExp175):
		return 2.75
	case p.HasBuff(skillPotionOfExp200):
		return 3.0
	case p.HasBuff(skillPotionOfExp225):
		return 3.25
	case p.HasBuff(skillPotionOfExp250):
		return 3.5
	}
	return 1.0
}

// awardNpcExpLikeJava 把一份擊殺經驗與善惡值給玩家（Java: CalcExp.AddExp）：
// 善惡依倍率反向加入；經驗依序套用高等級懲罰、伺服器倍率、料理與藥水加成後無條件捨去。
func awardNpcExpLikeJava(p *world.PlayerInfo, exp, lawful int64, deps *handler.Deps) int32 {
	if lawful != 0 && deps.PvP != nil {
		deps.PvP.AddLawfulFromNpc(p, int32(lawful))
	}
	rate := 1.0
	var divisors []int
	if deps.Config != nil {
		if deps.Config.Rates.ExpRate > 0 {
			rate = deps.Config.Rates.ExpRate
		}
		divisors = deps.Config.Rates.HighLevelExpDivisors
	}
	penalty := ExpPenaltyRateLikeJava(p.Level, divisors)
	add := int32(float64(exp) * penalty * rate * expFoodBonusLikeJava(p) * expPotionBonusLikeJava(p))
	if add > 0 {
		addExp(p, add, deps)
	}
	return add
}

// awardPetExpLikeJava 把一份擊殺經驗給寵物（Java: CalcExp.AddExpPet）：
// Java 只乘 RATE_XP；Go 另保留 pet_exp_rate 作額外倍率（預設 1.0 與 Java 相同）。
func awardPetExpLikeJava(owner *world.PlayerInfo, pet *world.PetInfo, exp int64, deps *handler.Deps) {
	if exp <= 0 || deps.PetLife == nil {
		return
	}
	rate := 1.0
	if deps.Config != nil {
		if deps.Config.Rates.ExpRate > 0 {
			rate *= deps.Config.Rates.ExpRate
		}
		if deps.Config.Rates.PetExpRate > 0 {
			rate *= deps.Config.Rates.PetExpRate
		}
	}
	add := int32(float64(exp) * rate)
	if add <= 0 {
		return
	}
	deps.PetLife.AddPetExp(pet, add)
	if owner != nil && owner.Session != nil {
		handler.SendPetHpMeter(owner.Session, pet.ID, pet.HP, pet.MaxHP)
	}
}

// companionHateOf 回傳該玩家名下寵物與召喚獸在此 NPC 上的仇恨合計。
func companionHateOf(acq *expAcquisitor, npc *world.NpcInfo, deps *handler.Deps) (petHate, summonHate int64) {
	if len(npc.CompanionHate) == 0 || deps.World == nil {
		return 0, 0
	}
	for _, pet := range deps.World.GetPetsByOwner(acq.player.CharID) {
		petHate += int64(npc.CompanionHate[pet.ID])
	}
	for _, sum := range deps.World.GetSummonsByOwner(acq.player.CharID) {
		summonHate += int64(npc.CompanionHate[sum.ID])
	}
	return petHate, summonHate
}

// awardGroupExpLikeJava 把分給「玩家＋其寵物／召喚獸」這一組的經驗依各自仇恨拆分（Java: calcExp 的 ownHateExp 迴圈）。
// 沒參與攻擊（hate=0）時全數給玩家；召喚獸的份額依 Java 不發放。善惡值整份給玩家。
func awardGroupExpLikeJava(acq *expAcquisitor, groupExp, groupLawful int64, npc *world.NpcInfo, deps *handler.Deps) {
	p := acq.player
	if acq.hate <= 0 {
		awardNpcExpLikeJava(p, groupExp, groupLawful, deps)
		return
	}
	awardNpcExpLikeJava(p, groupExp*acq.pcHate/acq.hate, groupLawful, deps)
	if len(npc.CompanionHate) == 0 || deps.World == nil {
		return
	}
	for _, pet := range deps.World.GetPetsByOwner(p.CharID) {
		if pet.Dead {
			continue
		}
		if h := int64(npc.CompanionHate[pet.ID]); h > 0 {
			awardPetExpLikeJava(p, pet, groupExp*h/acq.hate, deps)
		}
	}
}

// collectExpAcquisitors 從仇恨列表取出仍在線且存活的玩家（Java: calcExp 先剔除 null／死亡者）。
func collectExpAcquisitors(npc *world.NpcInfo, deps *handler.Deps) ([]*expAcquisitor, int64, int64) {
	var list []*expAcquisitor
	var totalHate, totalPcHate int64
	for sid, hate := range npc.HateList {
		p := deps.World.GetBySession(sid)
		if p == nil || p.Dead || hate <= 0 {
			continue
		}
		acq := &expAcquisitor{player: p, hate: int64(hate)}
		petHate, summonHate := companionHateOf(acq, npc, deps)
		acq.pcHate = acq.hate - petHate - summonHate
		if acq.pcHate < 0 {
			acq.pcHate = 0
		}
		list = append(list, acq)
		totalHate += acq.hate
		totalPcHate += acq.pcHate
	}
	return list, totalHate, totalPcHate
}

// distributeNpcExpLikeJava 依 Java CalcExp.calcExp 分配擊殺經驗與善惡：
//   - 未組隊：每位參與者依仇恨比例分得經驗；善惡依玩家本人仇恨比例分配。
//   - 組隊：非隊員先依仇恨比例領走；隊伍份額乘上王族隊長 5.9% 與每位可見隊員 4% 加成後，
//     依等級平方比例分給 killer 與其可見隊員，再在各自的寵物間按仇恨拆分。
func distributeNpcExpLikeJava(killer *world.PlayerInfo, npc *world.NpcInfo, deps *handler.Deps) {
	exp := int64(npc.Exp)
	lawful := int64(npc.Lawful)
	acquisitors, totalHate, totalPcHate := collectExpAcquisitors(npc, deps)
	if totalHate == 0 {
		return
	}
	lawfulShare := func(pcHate int64) int64 {
		if totalPcHate <= 0 {
			return 0
		}
		return lawful * pcHate / totalPcHate
	}

	var party *world.PartyInfo
	if deps.World.Parties != nil && killer != nil {
		party = deps.World.Parties.GetParty(killer.CharID)
	}
	if party == nil {
		for _, acq := range acquisitors {
			awardGroupExpLikeJava(acq, exp*acq.hate/totalHate, lawfulShare(acq.pcHate), npc, deps)
		}
		return
	}

	isMember := func(charID int32) bool {
		for _, id := range party.Members {
			if id == charID {
				return true
			}
		}
		return false
	}
	// Java knownsObject：killer 視野內的玩家。
	known := make(map[int32]bool)
	for _, v := range deps.World.GetNearbyPlayersInShow(killer.X, killer.Y, killer.MapID, killer.SessionID, killer.ShowID) {
		known[v.CharID] = true
	}

	// 非隊員直接依仇恨領走，隊員仇恨彙總為隊伍份額。
	var partyHate, partyPcHate int64
	hateOf := make(map[int32]*expAcquisitor)
	for _, acq := range acquisitors {
		if acq.player == killer || isMember(acq.player.CharID) {
			partyHate += acq.hate
			partyPcHate += acq.pcHate
			hateOf[acq.player.CharID] = acq
			continue
		}
		awardGroupExpLikeJava(acq, exp*acq.hate/totalHate, lawfulShare(acq.pcHate), npc, deps)
	}
	partyExp := exp * partyHate / totalHate
	partyLawful := int64(0)
	if totalPcHate > 0 {
		partyLawful = lawful * partyPcHate / totalPcHate
	}

	// 王族隊長加成與隊員人數加成。
	priBonus := 0.0
	if leader := deps.World.GetByCharID(party.LeaderID); leader != nil && leader.ClassType == 0 && (leader == killer || known[leader.CharID]) {
		priBonus = partyCrownLeaderExp
	}
	var members []*world.PlayerInfo
	partyLevel := 0.0
	ptBonus := 0.0
	for _, id := range party.Members {
		m := deps.World.GetByCharID(id)
		if m == nil {
			continue
		}
		if m == killer || known[m.CharID] {
			partyLevel += float64(m.Level) * float64(m.Level)
			members = append(members, m)
		}
		if m != killer && known[m.CharID] {
			ptBonus += partyMemberExpBonus
		}
	}
	if !isMember(killer.CharID) {
		// 防禦：隊伍資料與玩家不一致時仍讓 killer 領取。
		partyLevel += float64(killer.Level) * float64(killer.Level)
		members = append(members, killer)
	}
	partyExp = int64(float64(partyExp) * (1 + ptBonus + priBonus))

	// 依等級平方比例分給每位可見隊員，再在其寵物間拆分。
	for _, m := range members {
		dist := 0.0
		if partyLevel > 0 {
			dist = float64(m.Level) * float64(m.Level) / partyLevel
		}
		memberExp := int64(float64(partyExp) * dist)
		memberLawful := int64(float64(partyLawful) * dist)
		acq := hateOf[m.CharID]
		if acq == nil {
			acq = &expAcquisitor{player: m}
		}
		awardGroupExpLikeJava(acq, memberExp, memberLawful, npc, deps)
	}
}

package world

const (
	AccessLevelPlayer  int16 = 0
	AccessLevelMonitor int16 = 100
	AccessLevelGM      int16 = 200
)

// IsGM 與 Java MySqlCharacterStorage 的 case 200 一致。
// 指令最低權限比較與執行期 GM 遊戲特權是不同概念。
func (p *PlayerInfo) IsGM() bool {
	return p != nil && p.AccessLevel == AccessLevelGM && !p.GMModeOff
}

func (p *PlayerInfo) IsMonitor() bool {
	return p != nil && p.AccessLevel == AccessLevelMonitor
}

func (p *PlayerInfo) CanUseGMCommands() bool {
	return p != nil && (p.AccessLevel == AccessLevelMonitor || p.AccessLevel == AccessLevelGM)
}

func (p *PlayerInfo) SetGM(enabled bool) { p.GMModeOff = !enabled }

func (p *PlayerInfo) IsGMInvisible() bool { return p != nil && p.GMInvisible }

func (p *PlayerInfo) IsInvisible() bool { return p != nil && (p.Invisible || p.GMInvisible) }

// CanSeePlayer 是玩家出現封包共用規則。GM 隱身不會被 findinvis 揭露。
func (p *PlayerInfo) CanSeePlayer(other *PlayerInfo) bool {
	if p == nil || other == nil {
		return false
	}
	if p.CharID == other.CharID {
		return true
	}
	return p.MapID == other.MapID && p.ShowID == other.ShowID && !other.GMInvisible && (!other.Invisible || p.GMFindInvis)
}

// ApplyDamage 對齊 Java receiveDamage：GM 仍受非致命傷，致命傷回滿 HP。
func (p *PlayerInfo) ApplyDamage(damage int32) {
	p.HP -= damage
	if p.HP <= 0 && p.IsGM() {
		p.HP = p.MaxHP
	}
}

// SpendMP 對齊 Java setCurrentMp：GM 每次扣魔後回滿 MP。
func (p *PlayerInfo) SpendMP(amount int32) {
	p.MP -= amount
	if p.IsGM() {
		p.MP = p.MaxMP
	}
}

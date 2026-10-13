package handler

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

// Login38's HitPointExpansionPatch and ArmourResistanceExpansionPatch make
// HP/MP and AC four-byte fields. This is the client-side S_STATUS layout;
// narrowing AC shifts the clock into food/weight/lawful and changes daylight.
type login38Status struct {
	CharID       int32
	Level        byte
	Exp          int32
	Stats        [6]byte
	HPMP         [4]int32
	AC           int32
	GameTime     int32
	Food         byte
	Weight       byte
	Lawful       int16
	Resistances  [4]int16
	MonsterKills int32
}

func decodeLogin38Status(t *testing.T, payload []byte) login38Status {
	t.Helper()
	if len(payload) != 56 || payload[0] != packet.S_OPCODE_STATUS {
		t.Fatalf("Login38 S_STATUS must be 56 bytes, got %x", payload)
	}
	var status login38Status
	r := bytes.NewReader(payload[1:])
	if err := binary.Read(r, binary.LittleEndian, &status); err != nil {
		t.Fatalf("decode Login38 S_STATUS: %v", err)
	}
	if r.Len() != 0 {
		t.Fatalf("unexpected trailing status bytes: %d", r.Len())
	}
	return status
}

func TestPlayerStatusMatchesLogin38Layout(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{
		CharID: 0x10203040, Level: 80, Exp: 123456789,
		Str: 20, Intel: 21, Wis: 22, Dex: 23, Con: 20, Cha: 25,
		HP: 70001, MaxHP: 100002, MP: 80003, MaxMP: 120004,
		AC: -300, Food: 225, Lawful: -12345,
		FireRes: 101, WaterRes: -102, WindRes: 103, EarthRes: -104,
		ClassType: 6, // Illusionist: STR/CON 20 earn no original-stat weight bonus (Java resetOriginal*WeightReduction)
		Inv:       world.NewInventory(),
	}
	// STR/CON 20 give 3150 capacity; 1575 weight is exactly half full.
	p.Inv.AddItem(1, 1575, "weight", 0, 1000, true, 0)
	before := world.GameTimeNow().Seconds()
	SendPlayerStatus(sess, p)
	after := world.GameTimeNow().Seconds()
	packets := drainHandlerTestPackets(sess)
	if len(packets) != 1 {
		t.Fatalf("want one status packet, got %d", len(packets))
	}
	got := decodeLogin38Status(t, packets[0])
	if got.GameTime%300 != 0 || int(got.GameTime) < before-before%300 || int(got.GameTime) > after-after%300 {
		t.Errorf("status clock %d is not the current five-minute game-time bucket [%d, %d]", got.GameTime, before, after)
	}
	want := login38Status{
		CharID: 0x10203040, Level: 80, Exp: 123456789,
		Stats: [6]byte{20, 21, 22, 23, 20, 25},
		HPMP:  [4]int32{70001, 100002, 80003, 120004}, AC: -300,
		GameTime: got.GameTime, Food: 225, Weight: 121, Lawful: -12345,
		Resistances: [4]int16{101, -102, 103, -104},
	}
	if got != want {
		t.Errorf("Login38 decoded status = %+v, want %+v", got, want)
	}
}

func TestStatusUpdatesKeepLogin38ClockAligned(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ac     int16
		food   int16
		weight int32
		lawful int32
	}{
		{"empty", 10, 0, 0, 0},
		{"half", -129, 113, 1575, -32768},
		{"full", -300, 225, 3150, 32767},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess := newHandlerTestSession(t, 1)
			p := &world.PlayerInfo{
				Level: 80, Str: 20, Con: 20, AC: tc.ac, ClassType: 6,
				Food: tc.food, Lawful: tc.lawful, Inv: world.NewInventory(),
			}
			if tc.weight != 0 {
				p.Inv.AddItem(1, tc.weight, "weight", 0, 1000, true, 0)
			}
			before := world.GameTimeNow().Seconds()
			sendGameTime(sess, before)
			SendPlayerStatus(sess, p)
			after := world.GameTimeNow().Seconds()
			sendGameTime(sess, after)
			packets := drainHandlerTestPackets(sess)
			if len(packets) != 3 {
				t.Fatalf("want time/status/time packets, got %d", len(packets))
			}
			for _, i := range []int{0, 2} {
				if len(packets[i]) != 8 || packets[i][0] != packet.S_OPCODE_TIME {
					t.Fatalf("packet %d is not S_TIME: %x", i, packets[i])
				}
			}
			first := int32(binary.LittleEndian.Uint32(packets[0][1:5]))
			last := int32(binary.LittleEndian.Uint32(packets[2][1:5]))
			statusTime := decodeLogin38Status(t, packets[1]).GameTime
			if first != int32(before) || last != int32(after) {
				t.Fatalf("S_TIME changed the supplied clock: got %d/%d, want %d/%d", first, last, before, after)
			}
			if statusTime%300 != 0 || statusTime < first-first%300 || statusTime > last-last%300 {
				t.Errorf("status clock %d jumps away from adjacent S_TIME clocks %d/%d (AC=%d food=%d weight=%d lawful=%d)", statusTime, first, last, tc.ac, tc.food, tc.weight, tc.lawful)
			}
		})
	}
}

func TestCharResetMatchesLogin38Layout(t *testing.T) {
	t.Run("initialization", func(t *testing.T) {
		sess := newHandlerTestSession(t, 1)
		SendCharResetInit(sess, 70001, 80002, 80)
		packets := drainHandlerTestPackets(sess)
		if len(packets) != 1 {
			t.Fatalf("want one reset packet, got %d", len(packets))
		}
		got := packets[0]
		if len(got) != 16 || got[0] != packet.S_OPCODE_CHARSYNACK || got[1] != 1 {
			t.Fatalf("invalid reset initialization: %x", got)
		}
		if hp, mp, ac := binary.LittleEndian.Uint32(got[2:6]), binary.LittleEndian.Uint32(got[6:10]), int32(binary.LittleEndian.Uint32(got[10:14])); hp != 70001 || mp != 80002 || ac != 10 || got[14] != 80 || got[15] != 0 {
			t.Errorf("reset initialization has misaligned wide HP/MP/AC or level: %x", got)
		}
	})
	t.Run("level", func(t *testing.T) {
		sess := newHandlerTestSession(t, 1)
		p := &world.PlayerInfo{
			ResetTempLevel: 60, ResetMaxLevel: 80, MaxHP: 70001, MaxMP: 80002,
			AC: -300, Str: 20, Intel: 21, Wis: 22, Dex: 23, Con: 24, Cha: 25,
		}
		SendCharResetLevel(sess, p)
		packets := drainHandlerTestPackets(sess)
		if len(packets) != 1 {
			t.Fatalf("want one reset packet, got %d", len(packets))
		}
		got := packets[0]
		if len(got) != 24 || got[0] != packet.S_OPCODE_CHARSYNACK || got[1] != 2 || got[2] != 60 || got[3] != 80 {
			t.Fatalf("invalid reset level response: %x", got)
		}
		if hp, mp, ac := binary.LittleEndian.Uint32(got[4:8]), binary.LittleEndian.Uint32(got[8:12]), int32(binary.LittleEndian.Uint32(got[12:16])); hp != 70001 || mp != 80002 || ac != -300 {
			t.Errorf("reset level response truncated HP/MP/AC: %x", got)
		}
		if !bytes.Equal(got[16:], []byte{20, 21, 22, 23, 24, 25, 0, 0}) {
			t.Errorf("reset attributes shifted after AC: %x", got)
		}
	})
}

func TestMagicStatusMatchesLogin38Layout(t *testing.T) {
	for _, mr := range []int32{-70001, 70002} {
		sess := newHandlerTestSession(t, 1)
		sendMagicStatus(sess, 17, mr)
		packets := drainHandlerTestPackets(sess)
		if len(packets) != 1 {
			t.Fatalf("want one magic status packet, got %d", len(packets))
		}
		got := packets[0]
		if len(got) != 8 || got[0] != packet.S_OPCODE_MAGIC_STATUS || got[1] != 17 {
			t.Fatalf("invalid magic status: %x", got)
		}
		if decoded := int32(binary.LittleEndian.Uint32(got[2:6])); decoded != mr {
			t.Errorf("Login38 decoded MR = %d, want %d", decoded, mr)
		}
		if got[6] != 0 || got[7] != 0 {
			t.Errorf("unexpected magic status padding: %x", got[6:])
		}
	}
}

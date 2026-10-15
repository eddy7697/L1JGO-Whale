package handler

import (
	"bytes"
	"testing"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

type capturePetTarget struct {
	PetLifecycleManager
	ids []int32
}

func (c *capturePetTarget) SelectPetTarget(_ *net.Session, _ *world.PlayerInfo, _ *world.PetInfo, id int32) {
	c.ids = append(c.ids, id)
}

func TestSelectTargetPacketMatchesJava(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	sendSelectTarget(sess, 0x12345678)
	packets := drainHandlerTestPackets(sess)
	want := []byte{54, 0x78, 0x56, 0x34, 0x12, 0, 0, 0}
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Fatalf("S_SelectTarget = %x, want %x", packets, want)
	}
}

func TestSelectTargetDispatchAndOwnership(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	ws := world.NewState()
	player := &world.PlayerInfo{SessionID: sess.ID, Session: sess, CharID: 1001, MapID: 4, ShowID: 7}
	ws.AddPlayer(player)
	pet := &world.PetInfo{ID: 2001, OwnerCharID: player.CharID, MapID: 4, ShowID: 7}
	ws.AddPet(pet)
	capture := &capturePetTarget{}
	reg := packet.NewRegistry(zap.NewNop())
	RegisterAll(reg, &Deps{World: ws, PetLife: capture})
	w := packet.NewWriterWithOpcode(223)
	w.WriteD(pet.ID)
	w.WriteC(99) // Java ignores this byte; it is not part of targetID.
	w.WriteD(0x12345678)
	payload := w.RawBytes()
	for n := 1; n < len(payload); n++ {
		if err := reg.Dispatch(sess, packet.StateInWorld, payload[:n]); err != nil {
			t.Fatal(err)
		}
	}
	if len(capture.ids) != 0 {
		t.Fatalf("truncated request dispatched: %v", capture.ids)
	}
	if err := reg.Dispatch(sess, packet.StateAuthenticated, payload); err == nil {
		t.Fatal("selection must require an in-world session")
	}
	for _, mutate := range []func(){
		func() { pet.OwnerCharID++ },
		func() { pet.OwnerCharID = player.CharID; pet.MapID++ },
		func() { pet.MapID = player.MapID; pet.ShowID++ },
		func() { pet.ShowID = player.ShowID; pet.Dead = true },
		func() { pet.Dead = false; player.Dead = true },
		func() { player.Dead = false; player.HasTeleport = true },
	} {
		mutate()
		if err := reg.Dispatch(sess, packet.StateInWorld, payload); err != nil {
			t.Fatal(err)
		}
	}
	if len(capture.ids) != 0 {
		t.Fatalf("invalid owner/session dispatched: %v", capture.ids)
	}
	player.HasTeleport = false
	if err := reg.Dispatch(sess, packet.StateInWorld, payload); err != nil {
		t.Fatal(err)
	}
	if len(capture.ids) != 1 || capture.ids[0] != 0x12345678 {
		t.Fatalf("valid target not decoded: %v", capture.ids)
	}
}

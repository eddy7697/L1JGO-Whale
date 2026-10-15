package handler

import (
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
)

// HandleSelectTarget handles C_SelectTarget (223): D(petID), C(unused), D(targetID).
func HandleSelectTarget(sess *net.Session, r *packet.Reader, deps *Deps) {
	if r.Remaining() < 9 {
		return
	}
	petID := r.ReadD()
	_ = r.ReadC()
	targetID := r.ReadD()
	player := deps.World.GetBySession(sess.ID)
	if player == nil || player.Dead || player.HasTeleport {
		return
	}
	pet := deps.World.GetPet(petID)
	if pet == nil || pet.Dead || pet.OwnerCharID != player.CharID ||
		pet.MapID != player.MapID || pet.ShowID != player.ShowID {
		return
	}
	if deps.PetLife != nil {
		deps.PetLife.SelectPetTarget(sess, player, pet, targetID)
	}
}

package handler

import "github.com/l1jgo/server/internal/net/packet"

// BuildElfAttributeMessage follows the executable Java C_NPCAction mapping:
// fire=1, water=2, air=3, earth=4 (the S_PacketBox comment is off by one).
func BuildElfAttributeMessage(message byte) []byte {
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_EVENT)
	w.WriteC(15) // MSG_ELF
	w.WriteC(message)
	w.WriteC(0)
	return w.Bytes()
}

// BuildDeleteElfAttributeSkills removes elemental elf skills from the client
// spell window while preserving the neutral elf spells, exactly as Java init.
func BuildDeleteElfAttributeSkills() []byte {
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_DEL_SPELL)
	w.WriteC(32)
	var masks [28]byte
	masks[18], masks[19], masks[20], masks[21] = 248, 252, 252, 255
	for _, mask := range masks {
		w.WriteC(mask)
	}
	w.WriteD(0)
	w.WriteD(0)
	return w.Bytes()
}

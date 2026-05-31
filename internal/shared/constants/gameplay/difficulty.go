// Package gameplay menyediakan konstanta terkait gameplay mechanics.
package gameplay

// Difficulty names dan values
const (
	// DifficultyPeaceful adalah nama untuk peaceful difficulty
	DifficultyPeaceful = "peaceful"

	// DifficultyEasy adalah nama untuk easy difficulty
	DifficultyEasy = "easy"

	// DifficultyNormal adalah nama untuk normal difficulty
	DifficultyNormal = "normal"

	// DifficultyHard adalah nama untuk hard difficulty
	DifficultyHard = "hard"
)

// Difficulty byte values (Minecraft protocol)
const (
	// DifficultyBytePeaceful adalah byte value untuk peaceful (0)
	DifficultyBytePeaceful byte = 0

	// DifficultyByteEasy adalah byte value untuk easy (1)
	DifficultyByteEasy byte = 1

	// DifficultyByteNormal adalah byte value untuk normal (2)
	DifficultyByteNormal byte = 2

	// DifficultyByteHard adalah byte value untuk hard (3)
	DifficultyByteHard byte = 3
)

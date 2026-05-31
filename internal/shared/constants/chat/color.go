// Package chat menyediakan konstanta untuk kode format teks Minecraft
// (kode "section sign" §, U+00A7). Memakai konstanta ini menggantikan magic
// string §-code yang sebelumnya tersebar di paket command dan player, sehingga
// warna pesan punya satu sumber kebenaran.
package chat

// Legacy formatting codes used in system messages and command output. Each
// value is exactly the literal it replaces, so emitted text is unchanged.
const (
	ColorYellow = "§e" // notices: join/leave, headings
	ColorGreen  = "§a" // success feedback
	ColorRed    = "§c" // errors / denials
	ColorGray   = "§7" // secondary text
	Reset       = "§r" // reset formatting
)

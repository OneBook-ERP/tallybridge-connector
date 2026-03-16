//go:build ignore

// patch-pe.go patches a Windows PE executable's subsystem field from Console (3)
// to GUI (2), suppressing the terminal window that Windows allocates for console apps.
//
// Usage: go run ./build/patch-pe.go <exe-path>
//
// Background: Go's -H windowsgui and zig cc's -mwindows are supposed to set this,
// but with CGO + zig cross-compilation the external linker doesn't always honour them.
// This patcher sets the two bytes directly — safe for regular user-space executables
// (Windows only validates the PE checksum for kernel drivers and system DLLs).

package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

const (
	subsystemGUI     uint16 = 2 // IMAGE_SUBSYSTEM_WINDOWS_GUI
	subsystemConsole uint16 = 3 // IMAGE_SUBSYSTEM_WINDOWS_CUI
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./build/patch-pe.go <exe>")
		os.Exit(1)
	}
	path := os.Args[1]

	f, err := os.OpenFile(path, os.O_RDWR, 0755)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer f.Close()

	// 1. Verify MZ signature (DOS header)
	var mz [2]byte
	if _, err = f.ReadAt(mz[:], 0); err != nil || mz[0] != 'M' || mz[1] != 'Z' {
		fmt.Fprintln(os.Stderr, "not a PE file (no MZ signature)")
		os.Exit(1)
	}

	// 2. Read e_lfanew (offset 60) to find the PE header
	var lfanew [4]byte
	if _, err = f.ReadAt(lfanew[:], 60); err != nil {
		fmt.Fprintln(os.Stderr, "read e_lfanew:", err)
		os.Exit(1)
	}
	peOff := int64(binary.LittleEndian.Uint32(lfanew[:]))

	// 3. Verify PE signature
	var peSig [4]byte
	if _, err = f.ReadAt(peSig[:], peOff); err != nil || string(peSig[:]) != "PE\x00\x00" {
		fmt.Fprintln(os.Stderr, "invalid PE signature at offset", peOff)
		os.Exit(1)
	}

	// 4. The Subsystem field is at offset 68 inside the Optional Header,
	//    which starts at peOff+4 (PE sig) + 20 (COFF header) = peOff+24.
	//    This offset is the same for both PE32 (0x10b) and PE32+ (0x20b).
	subsystemOff := peOff + 24 + 68

	var sub [2]byte
	if _, err = f.ReadAt(sub[:], subsystemOff); err != nil {
		fmt.Fprintln(os.Stderr, "read subsystem field:", err)
		os.Exit(1)
	}
	current := binary.LittleEndian.Uint16(sub[:])

	if current == subsystemGUI {
		fmt.Printf("%s: subsystem already GUI (2) — nothing to patch\n", path)
		return
	}

	binary.LittleEndian.PutUint16(sub[:], subsystemGUI)
	if _, err = f.WriteAt(sub[:], subsystemOff); err != nil {
		fmt.Fprintln(os.Stderr, "write subsystem field:", err)
		os.Exit(1)
	}
	fmt.Printf("%s: patched subsystem %d → GUI (2)\n", path, current)
}

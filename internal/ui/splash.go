package ui

import (
	_ "embed"
	"fmt"
	"strings"
	"time"
)

//go:embed ansi1.txt
var ansiArtRaw string

// convertPrintfToANSI converts the printf-style ANSI art to actual ANSI codes
func convertPrintfToANSI(raw string) string {
	// Remove printf wrapper
	art := strings.TrimPrefix(raw, "printf \"")
	art = strings.TrimSuffix(art, "\";")
	art = strings.TrimSpace(art)
	
	// Convert \e[ to actual ANSI escape codes
	art = strings.ReplaceAll(art, `\e[`, "\x1b[")
	
	return art
}

// ShowSplash displays the splash screen for 3 seconds
// Note: Keypress detection would require raw terminal mode, so we just wait
func ShowSplash() {
	art := convertPrintfToANSI(ansiArtRaw)
	
	// Clear screen and move cursor to top
	fmt.Print("\033[2J\033[H")
	
	// Print the ANSI art
	fmt.Println(art)
	
	// Wait for 3 seconds
	time.Sleep(3 * time.Second)
	
	// Clear screen before continuing
	fmt.Print("\033[2J\033[H")
}

package main

import (
	"io/ioutil"
	"strings"
)

func main() {
	// Read the printf-style ANSI art
	content, err := ioutil.ReadFile("ansiart.ansi")
	if err != nil {
		panic(err)
	}

	raw := string(content)
	
	// Remove printf wrapper from first line and "; from last line
	raw = strings.TrimPrefix(raw, "printf \"")
	raw = strings.TrimSuffix(raw, "\";")
	raw = strings.TrimSpace(raw)
	
	// Convert \e to actual ESC character
	raw = strings.ReplaceAll(raw, `\e`, "\x1b")
	
	// Write the converted file
	err = ioutil.WriteFile("ansiart_converted.ansi", []byte(raw), 0644)
	if err != nil {
		panic(err)
	}
}


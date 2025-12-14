// Package main implements a TUI style linter for gitsome-ng.
// It scans internal/ui/*.go files for violations of the TUI style guide
// as defined in docs/TUI_STYLE_GUIDE.md and .kilocode/rules/tui-rules.md
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Violation severity levels
const (
	SeverityError   = "ERROR"
	SeverityWarning = "WARNING"
)

// Violation represents a single style violation
type Violation struct {
	File     string
	Line     int
	Column   int
	Severity string
	Rule     string
	Message  string
	Content  string
}

// LintResult holds the results of linting
type LintResult struct {
	Violations []Violation
	FileCount  int
	ErrorCount int
	WarnCount  int
}

// Pattern definitions for violations
var (
	// Rule 1: Hardcoded width/height numbers
	// Matches Width(50), Height(80), etc. - any numeric literal
	hardcodedWidthPattern  = regexp.MustCompile(`\.Width\(\s*(\d+)\s*\)`)
	hardcodedHeightPattern = regexp.MustCompile(`\.Height\(\s*(\d+)\s*\)`)

	// Rule 2: Fixed-length dividers with strings.Repeat
	// Matches strings.Repeat("-", 50) or strings.Repeat("─", 80)
	fixedRepeatPattern = regexp.MustCompile(`strings\.Repeat\([^,]+,\s*(\d+)\s*\)`)

	// Rule 3: Hardcoded fmt.Sprintf width specifiers
	// Matches %-50s, %50s, %-30d, %10d, etc.
	hardcodedSprintfPattern = regexp.MustCompile(`%-?\d+[sdvfgx]`)

	// Rule 4: Fixed text input widths
	// Matches ti.Width = 40, textInput.Width = 50, etc.
	fixedTextInputPattern = regexp.MustCompile(`\.\s*Width\s*=\s*(\d+)`)

	// Allowed patterns (exceptions)
	// Minimum width safeguards like: if width < 40 { width = 40 }
	minWidthSafeguardPattern = regexp.MustCompile(`if\s+\w+\s*<\s*\d+\s*\{`)
	// Constants definitions
	constPattern = regexp.MustCompile(`^\s*(const|var)\s+\w+\s*=\s*\d+`)
	// Column width constants like ColWidthTag = 5
	colWidthConstPattern = regexp.MustCompile(`ColWidth\w+\s*=\s*\d+`)
	// Minimum/default constants like MinViewportWidth = 80
	minMaxConstPattern = regexp.MustCompile(`(Min|Max|Default)\w*\s*=\s*\d+`)
	// Table height constants
	tableHeightConstPattern = regexp.MustCompile(`TableHeight\s*=\s*\d+`)
	// Padding constants
	paddingConstPattern = regexp.MustCompile(`Padding\s*=\s*\d+`)
	// Column separators constant
	colSepPattern = regexp.MustCompile(`ColSeparators\s*=\s*\d+`)
)

// isComment checks if a line is a comment
func isComment(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*")
}

// isTestFile checks if a file is a test file
func isTestFile(filename string) bool {
	return strings.HasSuffix(filename, "_test.go")
}

// isInComment checks if a match position is within a comment on the line
func isInComment(line string, matchStart int) bool {
	// Check if there's a // before the match
	commentIdx := strings.Index(line, "//")
	if commentIdx >= 0 && commentIdx < matchStart {
		return true
	}
	return false
}

// isAllowedException checks if a match is an allowed exception
func isAllowedException(line string, fullContent []string, lineNum int) bool {
	// Check if it's a constant definition
	if constPattern.MatchString(line) {
		return true
	}

	// Check for column width constants
	if colWidthConstPattern.MatchString(line) {
		return true
	}

	// Check for min/max/default constants
	if minMaxConstPattern.MatchString(line) {
		return true
	}

	// Check for table height constants
	if tableHeightConstPattern.MatchString(line) {
		return true
	}

	// Check for padding constants
	if paddingConstPattern.MatchString(line) {
		return true
	}

	// Check for column separators constants
	if colSepPattern.MatchString(line) {
		return true
	}

	// Check for minimum width safeguards (look at surrounding context)
	if minWidthSafeguardPattern.MatchString(line) {
		return true
	}

	// Check for safeguard patterns like: if totalW < 50 { totalW = 50 }
	if strings.Contains(line, "if") && strings.Contains(line, "<") {
		return true
	}

	// Check previous line for safeguard context
	if lineNum > 0 {
		prevLine := fullContent[lineNum-1]
		if minWidthSafeguardPattern.MatchString(prevLine) {
			return true
		}
	}

	// Check if this is setting a minimum: if x < N { x = N }
	// Pattern: variable = number after an if check
	if lineNum > 0 {
		prevLine := fullContent[lineNum-1]
		if strings.Contains(prevLine, "if") && strings.Contains(prevLine, "<") {
			if strings.Contains(line, "=") && !strings.Contains(line, "==") {
				return true
			}
		}
	}

	return false
}

// lintFile lints a single Go file and returns violations
func lintFile(filepath string) ([]Violation, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var violations []Violation
	var lines []string

	// Read all lines first for context
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for lineNum, line := range lines {
		// Skip comments
		if isComment(line) {
			continue
		}

		// Check for hardcoded Width() calls
		if matches := hardcodedWidthPattern.FindAllStringSubmatchIndex(line, -1); matches != nil {
			for _, match := range matches {
				if !isInComment(line, match[0]) && !isAllowedException(line, lines, lineNum) {
					// Extract the number
					numStr := line[match[2]:match[3]]
					violations = append(violations, Violation{
						File:     filepath,
						Line:     lineNum + 1,
						Column:   match[0] + 1,
						Severity: SeverityError,
						Rule:     "hardcoded-width",
						Message:  fmt.Sprintf("Hardcoded width value '%s'. Use m.layout.InnerWidth or m.layout.TableWidth instead.", numStr),
						Content:  strings.TrimSpace(line),
					})
				}
			}
		}

		// Check for hardcoded Height() calls (with more lenient checking for minimum safeguards)
		if matches := hardcodedHeightPattern.FindAllStringSubmatchIndex(line, -1); matches != nil {
			for _, match := range matches {
				if !isInComment(line, match[0]) && !isAllowedException(line, lines, lineNum) {
					// Check if this looks like a minimum safeguard
					numStr := line[match[2]:match[3]]
					// Allow small numbers (like 10) that are typically minimum safeguards
					if numStr != "10" && numStr != "5" {
						violations = append(violations, Violation{
							File:     filepath,
							Line:     lineNum + 1,
							Column:   match[0] + 1,
							Severity: SeverityError,
							Rule:     "hardcoded-height",
							Message:  fmt.Sprintf("Hardcoded height value '%s'. Use m.layout.TableHeight or calculated availableHeight instead.", numStr),
							Content:  strings.TrimSpace(line),
						})
					}
				}
			}
		}

		// Check for fixed-length strings.Repeat
		if matches := fixedRepeatPattern.FindAllStringSubmatchIndex(line, -1); matches != nil {
			for _, match := range matches {
				if !isInComment(line, match[0]) && !isAllowedException(line, lines, lineNum) {
					numStr := line[match[2]:match[3]]
					violations = append(violations, Violation{
						File:     filepath,
						Line:     lineNum + 1,
						Column:   match[0] + 1,
						Severity: SeverityError,
						Rule:     "fixed-repeat",
						Message:  fmt.Sprintf("Fixed-length strings.Repeat with '%s'. Use m.layout.InnerWidth for divider width.", numStr),
						Content:  strings.TrimSpace(line),
					})
				}
			}
		}

		// Check for hardcoded fmt.Sprintf width specifiers (warning only - some are legitimate)
		if matches := hardcodedSprintfPattern.FindAllStringIndex(line, -1); matches != nil {
			for _, match := range matches {
				if !isInComment(line, match[0]) {
					// Only warn if it looks like it's for display formatting, not parsing
					if strings.Contains(line, "fmt.Sprintf") || strings.Contains(line, "Printf") {
						matchStr := line[match[0]:match[1]]
						violations = append(violations, Violation{
							File:     filepath,
							Line:     lineNum + 1,
							Column:   match[0] + 1,
							Severity: SeverityWarning,
							Rule:     "hardcoded-sprintf-width",
							Message:  fmt.Sprintf("Hardcoded width specifier '%s' in format string. Consider using dynamic width if for display.", matchStr),
							Content:  strings.TrimSpace(line),
						})
					}
				}
			}
		}

		// Check for fixed text input widths (like ti.Width = 40)
		if matches := fixedTextInputPattern.FindAllStringSubmatchIndex(line, -1); matches != nil {
			for _, match := range matches {
				if !isInComment(line, match[0]) && !isAllowedException(line, lines, lineNum) {
					// Make sure it's not a table column width (which is calculated)
					if !strings.Contains(line, "columns") && !strings.Contains(line, "Column") {
						numStr := line[match[2]:match[3]]
						violations = append(violations, Violation{
							File:     filepath,
							Line:     lineNum + 1,
							Column:   match[0] + 1,
							Severity: SeverityError,
							Rule:     "fixed-input-width",
							Message:  fmt.Sprintf("Fixed text input width '%s'. Text inputs must resize dynamically on tea.WindowSizeMsg.", numStr),
							Content:  strings.TrimSpace(line),
						})
					}
				}
			}
		}
	}

	return violations, nil
}

// formatViolation formats a violation for output
func formatViolation(v Violation) string {
	return fmt.Sprintf("%s:%d:%d: %s [%s] %s\n    %s",
		v.File, v.Line, v.Column, v.Severity, v.Rule, v.Message, v.Content)
}

// printSummary prints a summary of the lint results
func printSummary(result LintResult) {
	fmt.Printf("\n%s\n", strings.Repeat("─", 60))
	fmt.Printf("TUI Style Lint Summary\n")
	fmt.Printf("%s\n", strings.Repeat("─", 60))
	fmt.Printf("Files checked:  %d\n", result.FileCount)
	fmt.Printf("Errors found:   %d\n", result.ErrorCount)
	fmt.Printf("Warnings found: %d\n", result.WarnCount)
	fmt.Printf("%s\n", strings.Repeat("─", 60))

	if result.ErrorCount > 0 {
		fmt.Println("\n[FAIL] TUI style check FAILED")
		fmt.Println("\nRefer to docs/TUI_STYLE_GUIDE.md for correct patterns.")
	} else if result.WarnCount > 0 {
		fmt.Println("\n[WARN] TUI style check passed with warnings")
	} else {
		fmt.Println("\n[PASS] TUI style check PASSED")
	}
}

func main() {
	// Parse flags
	verbose := flag.Bool("v", false, "Verbose output")
	showWarnings := flag.Bool("w", true, "Show warnings (not just errors)")
	help := flag.Bool("h", false, "Show help")
	flag.Parse()

	if *help {
		fmt.Println("TUI Style Linter for gitsome-ng")
		fmt.Println()
		fmt.Println("Usage: lint-tui [options] [files...]")
		fmt.Println()
		fmt.Println("If no files are specified, lints internal/ui/*.go")
		fmt.Println()
		fmt.Println("Options:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Rules checked:")
		fmt.Println("  hardcoded-width     Hardcoded Width() values")
		fmt.Println("  hardcoded-height    Hardcoded Height() values")
		fmt.Println("  fixed-repeat        Fixed-length strings.Repeat dividers")
		fmt.Println("  hardcoded-sprintf   Hardcoded width in format specifiers")
		fmt.Println("  fixed-input-width   Fixed text input widths")
		os.Exit(0)
	}

	// Determine files to lint
	var files []string
	if flag.NArg() > 0 {
		files = flag.Args()
	} else {
		// Default: lint internal/ui/*.go
		pattern := filepath.Join("internal", "ui", "*.go")
		var err error
		files, err = filepath.Glob(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error finding files: %v\n", err)
			os.Exit(1)
		}
	}

	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "No files to lint\n")
		os.Exit(1)
	}

	// Filter out test files
	var filesToLint []string
	for _, f := range files {
		if !isTestFile(f) {
			filesToLint = append(filesToLint, f)
		}
	}

	result := LintResult{
		FileCount: len(filesToLint),
	}

	// Lint each file
	for _, f := range filesToLint {
		if *verbose {
			fmt.Printf("Checking %s...\n", f)
		}

		violations, err := lintFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error linting %s: %v\n", f, err)
			continue
		}

		for _, v := range violations {
			if v.Severity == SeverityError {
				result.ErrorCount++
				fmt.Println(formatViolation(v))
			} else if *showWarnings && v.Severity == SeverityWarning {
				result.WarnCount++
				fmt.Println(formatViolation(v))
			}
		}
		result.Violations = append(result.Violations, violations...)
	}

	// Print summary
	printSummary(result)

	// Exit with error code if violations found
	if result.ErrorCount > 0 {
		os.Exit(1)
	}
	os.Exit(0)
}

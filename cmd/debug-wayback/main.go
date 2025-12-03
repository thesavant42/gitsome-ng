// Debug tool to test Wayback CDX fetching directly
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/thesavant42/gitsome-ng/internal/api"
)

func main() {
	domain := "raspberrypi.com"
	if len(os.Args) > 1 {
		domain = os.Args[1]
	}

	logger := log.NewWithOptions(os.Stderr, log.Options{
		Level:           log.DebugLevel,
		ReportTimestamp: true,
	})

	fmt.Printf("Testing CDX fetch for domain: %s\n", domain)
	fmt.Printf("Query: %s\n", api.BuildCDXQuery(domain, ""))

	client := api.NewWaybackClient(logger)

	// Single page fetch
	fmt.Println("\n--- Fetching single page ---")
	resp, err := client.FetchCDX(domain, "")
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Records: %d\n", len(resp.Records))
	fmt.Printf("HasMore: %v\n", resp.HasMore)
	fmt.Printf("ResumeKey: %s\n", resp.ResumeKey)

	// Show first 3 records
	fmt.Println("\nFirst records:")
	for i, rec := range resp.Records {
		if i >= 3 {
			fmt.Printf("  ... and %d more\n", len(resp.Records)-3)
			break
		}
		fmt.Printf("  %d. %s (status: %v)\n", i+1, rec.URL, rec.StatusCode)
	}
}

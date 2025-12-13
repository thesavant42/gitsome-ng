package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/thesavant42/gitsome-ng/internal/db"
)

func main() {
	// Open database
	dbPath := filepath.Join(".", "generic.db")
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Get all target domains
	domains, err := database.GetTargetDomainsWithCounts()
	if err != nil {
		log.Fatalf("Failed to get target domains: %v", err)
	}

	// Create markdown file
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("subdomains-export-%s.md", timestamp)
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer f.Close()

	// Write header
	fmt.Fprintf(f, "# Subdomain Export\n\n")
	fmt.Fprintf(f, "Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(f, "Total Domains: %d\n\n", len(domains))

	// Process each domain
	for _, domain := range domains {
		fmt.Fprintf(f, "## %s\n\n", domain.Domain)
		fmt.Fprintf(f, "- **Subdomain Count**: %d\n", domain.SubdomainCount)
		fmt.Fprintf(f, "- **VirusTotal Enumerated**: %v\n", domain.VTEnumerated)
		fmt.Fprintf(f, "- **crt.sh Enumerated**: %v\n", domain.CrtshEnumerated)
		fmt.Fprintf(f, "- **Added**: %s\n\n", domain.AddedAt.Format("2006-01-02 15:04:05"))

		// Get subdomains for this domain
		subdomains, err := database.GetSubdomains(domain.Domain)
		if err != nil {
			log.Printf("Failed to get subdomains for %s: %v", domain.Domain, err)
			continue
		}

		if len(subdomains) > 0 {
			fmt.Fprintf(f, "### Subdomains\n\n")
			fmt.Fprintf(f, "| Subdomain | Source | CNAMEs | Cert Expired | CDX Indexed | Discovered |\n")
			fmt.Fprintf(f, "|-----------|--------|--------|--------------|-------------|------------|\n")

			for _, sub := range subdomains {
				cnames := sub.CNAMEs
				if cnames == "" {
					cnames = "-"
				}
				certExpired := "No"
				if sub.CertExpired {
					certExpired = "Yes"
				}
				cdxIndexed := "No"
				if sub.CDXIndexed {
					cdxIndexed = "Yes"
				}
				discovered := sub.DiscoveredAt.Format("2006-01-02")

				fmt.Fprintf(f, "| %s | %s | %s | %s | %s | %s |\n",
					sub.Subdomain, sub.Source, cnames, certExpired, cdxIndexed, discovered)
			}
			fmt.Fprintf(f, "\n")
		} else {
			fmt.Fprintf(f, "*No subdomains found*\n\n")
		}

		fmt.Fprintf(f, "---\n\n")
	}

	fmt.Printf("✓ Exported to %s\n", filename)
}

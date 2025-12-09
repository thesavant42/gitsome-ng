package main

import (
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "raspberrypi.db", "Path to SQLite database")
	outputPath := flag.String("output", "emails.csv", "Output CSV file")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT DISTINCT author_email as email
		FROM commits
		WHERE author_email IS NOT NULL AND author_email != ''
		UNION
		SELECT DISTINCT committer_email as email
		FROM commits
		WHERE committer_email IS NOT NULL AND committer_email != ''
		ORDER BY 1
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query database: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	f, err := os.Create(*outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"email"}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write header: %v\n", err)
		os.Exit(1)
	}

	count := 0
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to scan row: %v\n", err)
			continue
		}
		if err := w.Write([]string{email}); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write row: %v\n", err)
			continue
		}
		count++
	}

	fmt.Printf("Exported %d emails to %s\n", count, *outputPath)
}

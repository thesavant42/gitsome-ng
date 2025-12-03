package main

import (
	"fmt"
	"net/url"
)

func main() {
	// Test 1: Setting RawQuery
	u := &url.URL{
		Scheme:   "https",
		Host:     "web.archive.org",
		Path:     "/cdx/search/cdx",
		RawQuery: "url=*.example.com&output=json",
	}

	fmt.Println("RawQuery:", u.RawQuery)
	fmt.Println("String():", u.String())
	fmt.Println()

	// Test 2: What happens when we use this URL in http.Request
	fmt.Println("When passed to HTTP client, URL is:", u.String())
}

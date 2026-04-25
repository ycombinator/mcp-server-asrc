package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var knownPages = []struct {
	path     string
	keywords []string
}{
	{"/facility-hours", []string{"hours", "open", "close", "schedule", "when", "time"}},
	{"/membership", []string{"membership", "member", "join", "fee", "cost", "price", "dues"}},
	{"/tennis", []string{"tennis", "court", "racquet"}},
	{"/tennis-pros", []string{"pro", "coach", "instructor", "staff", "tennis pro"}},
	{"/tennis-lessons", []string{"lesson", "learn", "beginner", "tennis lesson"}},
	{"/adult-clinics", []string{"clinic", "adult", "drill"}},
	{"/drop-in", []string{"drop-in", "drop in", "open play", "walk-in"}},
	{"/junior-tennis-camp", []string{"camp", "junior", "kid", "child", "summer camp", "tennis camp"}},
	{"/junior-tennis-program", []string{"junior program", "junior tennis", "youth"}},
	{"/tennis-guests", []string{"guest", "visitor", "non-member"}},
	{"/usta", []string{"usta", "league", "team", "interclub"}},
	{"/swim", []string{"swim", "pool", "aquatic"}},
	{"/pool-schedule", []string{"pool schedule", "lap swim", "pool hour"}},
	{"/pool-rules", []string{"pool rule", "pool policy", "diving", "pool safety"}},
	{"/swimming-lessons", []string{"swim lesson", "learn to swim"}},
	{"/quicksilver-swim-team-and-masters", []string{"swim team", "quicksilver", "masters", "competitive"}},
	{"/alma-swim-team-records", []string{"record", "swim record", "alma"}},
	{"/red-cross-lifeguard-training", []string{"lifeguard", "red cross", "certification", "training"}},
	{"/pickleball", []string{"pickleball", "pickle"}},
	{"/bocce", []string{"bocce"}},
	{"/fitness-center", []string{"fitness", "gym", "workout", "exercise", "weight"}},
	{"/summer-camp-2026", []string{"summer camp", "camp 2026", "day camp"}},
	{"/job-opportunities", []string{"job", "employ", "hire", "work", "career", "position"}},
	{"/partners", []string{"partner", "sponsor"}},
	{"/club-socials", []string{"social", "event", "party", "gathering"}},
	{"/contact-us", []string{"contact", "phone", "email", "address", "location", "direction"}},
	{"/about", []string{"about", "club", "history", "info", "overview"}},
}

func handleWebsiteInfo(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := request.Params.Arguments.(map[string]any)
	query, _ := args["query"].(string)

	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	pages := selectPages(query)
	if len(pages) == 0 {
		pages = []string{"/facility-hours"}
	}

	var results []string
	for _, page := range pages {
		content, err := fetchPageContent(page)
		if err != nil {
			continue
		}
		results = append(results, fmt.Sprintf("=== Page: %s ===\n%s", page, content))
	}

	if len(results) == 0 {
		return mcp.NewToolResultError("failed to fetch any pages from asrc.org"), nil
	}

	return mcp.NewToolResultText(strings.Join(results, "\n\n")), nil
}

func selectPages(query string) []string {
	q := strings.ToLower(query)

	type scored struct {
		path  string
		score int
	}
	var matches []scored

	for _, p := range knownPages {
		score := 0
		for _, kw := range p.keywords {
			if strings.Contains(q, kw) {
				score += len(kw)
			}
		}
		if score > 0 {
			matches = append(matches, scored{p.path, score})
		}
	}

	for i := range matches {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].score > matches[i].score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	var pages []string
	for i, m := range matches {
		if i >= 3 {
			break
		}
		pages = append(pages, m.path)
	}
	return pages
}

func fetchPageContent(path string) (string, error) {
	url := "https://r.jina.ai/https://www.asrc.org" + path

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("X-Wait-For-Selector", ".cs-component")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %s fetching %s", resp.Status, path)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return extractContent(string(body)), nil
}

func extractContent(raw string) string {
	content := raw

	// Skip past Jina metadata header
	if idx := strings.Index(content, "Markdown Content:"); idx != -1 {
		content = content[idx+len("Markdown Content:"):]
	}

	// The page content starts at the last "# Title" heading (after all nav).
	// Find it by scanning for the last H1 that isn't the site-wide title.
	lines := strings.Split(content, "\n")
	contentStart := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && !strings.Contains(trimmed, "Almaden Swim") {
			contentStart = i
			break
		}
	}
	if contentStart > 0 {
		content = strings.Join(lines[contentStart:], "\n")
	}

	// The footer starts with "Connect Online" or the home link before it.
	if idx := strings.Index(content, "\nConnect Online"); idx != -1 {
		content = content[:idx]
	}
	// Also strip trailing home link that appears right before footer
	if idx := strings.LastIndex(content, "\n[](https://www.asrc.org/)"); idx != -1 {
		content = content[:idx]
	}

	return strings.TrimSpace(content)
}

var _ server.ToolHandlerFunc = handleWebsiteInfo

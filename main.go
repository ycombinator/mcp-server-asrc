package main

import (
	"log"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"asrc-tennis",
		"1.0.0",
	)

	asrcEmail := os.Getenv("ASRC_EMAIL")
	asrcPassword := os.Getenv("ASRC_PASSWORD")
	if asrcEmail == "" || asrcPassword == "" {
		log.Fatal("ASRC_EMAIL and ASRC_PASSWORD environment variables are required")
	}

	s.AddTool(
		mcp.NewTool("check_court_availability",
			mcp.WithDescription("Check tennis court availability and bookings at Almaden Swim & Racquet Club (ASRC) for a given date. Returns all current bookings showing which courts are reserved, the time slots, and who booked them. Use this to find open courts or check existing reservations."),
			mcp.WithString("date", mcp.Required(), mcp.Description("Date to check in YYYY-MM-DD format")),
			mcp.WithTitleAnnotation("Check ASRC Court Availability"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		),
		makeCheckAvailabilityHandler(asrcEmail, asrcPassword),
	)

	s.AddTool(
		mcp.NewTool("lookup_website_info",
			mcp.WithDescription("Look up information from the Almaden Swim & Racquet Club (ASRC) website (asrc.org). Use this to answer questions about the club such as facility hours, membership, tennis programs, swim programs, pool rules, fitness center, pickleball, events, contact info, and more."),
			mcp.WithString("query", mcp.Required(), mcp.Description("The question or topic to look up, e.g. 'what are the pool hours' or 'tennis lesson prices'")),
			mcp.WithTitleAnnotation("Look Up ASRC Website Info"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		),
		handleWebsiteInfo,
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	httpServer := server.NewStreamableHTTPServer(s)
	log.Printf("ASRC Tennis MCP server listening on :%s", port)
	if err := httpServer.Start(":" + port); err != nil {
		log.Fatal(err)
	}
}

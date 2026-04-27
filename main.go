package main

import (
	"log"
	"net/http"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	store := newTokenStore()
	oauth := &oauthServer{
		baseURL: baseURL,
		store:   store,
		club: clubConfig{
			Name:    "Almaden Swim & Racquet Club",
			Short:   "ASRC",
			Website: "asrc.org",
		},
	}

	s := server.NewMCPServer(
		"asrc-tennis",
		"1.0.0",
	)

	s.AddTool(
		mcp.NewTool("check_court_availability",
			mcp.WithDescription("Check tennis court availability and bookings at Almaden Swim & Racquet Club (ASRC) for a given date. Returns all current bookings showing which courts are reserved, the time slots, and who booked them. Use this to find open courts or check existing reservations."),
			mcp.WithString("date", mcp.Required(), mcp.Description("Date to check in YYYY-MM-DD format")),
			mcp.WithTitleAnnotation("Check ASRC Court Availability"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		),
		handleCheckAvailability,
	)

	s.AddTool(
		mcp.NewTool("cancel_reservation",
			mcp.WithDescription("Cancel a court reservation at Almaden Swim & Racquet Club (ASRC). Requires the reservation_id, which can be obtained from the check_court_availability tool. Only the member who made the reservation can cancel it."),
			mcp.WithString("reservation_id", mcp.Required(), mcp.Description("The reservation ID to cancel (from check_court_availability results)")),
			mcp.WithTitleAnnotation("Cancel ASRC Court Reservation"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		),
		handleCancelReservation,
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

	mcpHandler := server.NewStreamableHTTPServer(s)

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource", oauth.handleProtectedResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server", oauth.handleAuthServerMetadata)
	mux.HandleFunc("/oauth/authorize", oauth.handleAuthorize)
	mux.HandleFunc("/oauth/token", oauth.handleToken)
	mux.HandleFunc("/oauth/register", oauth.handleRegister)
	mux.Handle("/mcp", oauth.authRequired(mcpHandler))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("ASRC Tennis MCP server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

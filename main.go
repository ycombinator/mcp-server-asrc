package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	installationID = "725c7126-0dda-4963-82fe-eebeee058749"
	applicationID  = "myclubspot2017"
	clientVersion  = "js4.3.1-forked-1.0"
	clubID         = "Gh2Ho6cZZu"
	timezone       = "America/Los_Angeles"
)

var courtIDs = []string{
	"PzkiohWQ8H", "WME1tave0u", "SgmwbZONPB", "euKGjKtd0s",
	"27ks1jSh7r", "bAyZ5cywrR", "UjfvbxzhII", "ooU1Q2LbIT", "zyY2B7LQlP",
}

var courtTypeIDs = []string{"iB7UPulFKi", "03yM1ht69n", "n2SZQ0kS5x"}

type Booking struct {
	Court     string `json:"court"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	BookedBy  string `json:"booked_by"`
}

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

func makeCheckAvailabilityHandler(email, password string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleCheckAvailability(ctx, request, email, password)
	}
}

func handleCheckAvailability(ctx context.Context, request mcp.CallToolRequest, email, password string) (*mcp.CallToolResult, error) {
	args, _ := request.Params.Arguments.(map[string]any)
	dateStr, _ := args["date"].(string)

	if dateStr == "" {
		return mcp.NewToolResultError("date is required"), nil
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return mcp.NewToolResultError("internal error: failed to load timezone"), nil
	}

	targetDate, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return mcp.NewToolResultError("invalid date format — use YYYY-MM-DD (e.g. 2026-04-24)"), nil
	}

	sessionToken, err := login(email, password)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("login failed: %v", err)), nil
	}

	bookings, err := fetchBookings(sessionToken, targetDate, loc)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to fetch bookings: %v", err)), nil
	}

	if len(bookings) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No bookings found for %s — all courts are open!", targetDate.Format("Mon Jan 2, 2006"))), nil
	}

	result, _ := json.MarshalIndent(bookings, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Bookings for %s:\n\n%s", targetDate.Format("Mon Jan 2, 2006"), string(result))), nil
}

func login(email, password string) (string, error) {
	username, err := resolveUsername(email)
	if err != nil {
		return "", fmt.Errorf("resolving username: %w", err)
	}

	body, err := postJSON("https://theclubspot.com/parse/login", map[string]any{
		"username":        username,
		"password":        password,
		"_ApplicationId":  applicationID,
		"_InstallationId": installationID,
	})
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}

	var resp struct {
		SessionToken string `json:"sessionToken"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if resp.SessionToken == "" {
		return "", errors.New("invalid credentials")
	}
	return resp.SessionToken, nil
}

func resolveUsername(email string) (string, error) {
	body, err := postJSON("https://theclubspot.com/parse/functions/retrieveUsersByEmailOrMobileNumber", map[string]any{
		"email":           email,
		"clubID":          clubID,
		"_ApplicationId":  applicationID,
		"_ClientVersion":  clientVersion,
		"_InstallationId": installationID,
	})
	if err != nil {
		return "", err
	}

	var resp struct {
		Result []struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if len(resp.Result) == 0 {
		return "", errors.New("no ASRC account found for this email")
	}
	return resp.Result[0].Username, nil
}

func fetchBookings(sessionToken string, date time.Time, loc *time.Location) ([]Booking, error) {
	body, err := postJSON("https://theclubspot.com/parse/functions/retrieve_court_availability", map[string]any{
		"day":             int(date.Weekday()),
		"date_string":     date.Format("Jan-2-2006"),
		"club_id":         clubID,
		"court_ids":       courtIDs,
		"court_type_ids":  courtTypeIDs,
		"include_billing": true,
		"flow":            "member",
		"_ApplicationId":  applicationID,
		"_ClientVersion":  clientVersion,
		"_InstallationId": installationID,
		"_SessionToken":   sessionToken,
	})
	if err != nil {
		return nil, err
	}

	var response struct {
		Result []struct {
			CourtObject struct {
				Name string `json:"name"`
			} `json:"courtObject"`
			Options []struct {
				StartTime string `json:"start_time"`
				EndTime   string `json:"end_time"`
				BookedBy  []struct {
					ObjectID          string `json:"objectId"`
					ParticipantsArray []struct {
						FirstName string `json:"firstName"`
					} `json:"participantsArray"`
				} `json:"booked_by"`
			} `json:"options"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	type rawBooking struct {
		court, bookedBy       string
		startTime, endTime    time.Time
	}
	type groupKey struct{ bookedBy, court string }
	groups := make(map[groupKey][]rawBooking)

	for _, court := range response.Result {
		for _, opt := range court.Options {
			if len(opt.BookedBy) == 0 {
				continue
			}
			start, err := parseHHMM(opt.StartTime, date, loc)
			if err != nil {
				continue
			}
			end, err := parseHHMM(opt.EndTime, date, loc)
			if err != nil {
				continue
			}

			name := "Unknown"
			if len(opt.BookedBy[0].ParticipantsArray) > 0 {
				name = opt.BookedBy[0].ParticipantsArray[0].FirstName
			}

			k := groupKey{name, court.CourtObject.Name}
			groups[k] = append(groups[k], rawBooking{
				court:     court.CourtObject.Name,
				bookedBy:  name,
				startTime: start,
				endTime:   end,
			})
		}
	}

	var bookings []Booking
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			return group[i].startTime.Before(group[j].startTime)
		})
		cur := group[0]
		for _, next := range group[1:] {
			if !next.startTime.After(cur.endTime) {
				if next.endTime.After(cur.endTime) {
					cur.endTime = next.endTime
				}
			} else {
				bookings = append(bookings, Booking{
					Court:     cur.court,
					StartTime: cur.startTime.Format("3:04 PM"),
					EndTime:   cur.endTime.Format("3:04 PM"),
					BookedBy:  cur.bookedBy,
				})
				cur = next
			}
		}
		bookings = append(bookings, Booking{
			Court:     cur.court,
			StartTime: cur.startTime.Format("3:04 PM"),
			EndTime:   cur.endTime.Format("3:04 PM"),
			BookedBy:  cur.bookedBy,
		})
	}

	sort.Slice(bookings, func(i, j int) bool {
		return bookings[i].StartTime < bookings[j].StartTime
	})
	return bookings, nil
}

func parseHHMM(hhmm string, ref time.Time, loc *time.Location) (time.Time, error) {
	if len(hhmm) != 4 {
		return time.Time{}, fmt.Errorf("unexpected time format: %q", hhmm)
	}
	hour, err := strconv.Atoi(hhmm[:2])
	if err != nil {
		return time.Time{}, err
	}
	min, err := strconv.Atoi(hhmm[2:])
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(ref.Year(), ref.Month(), ref.Day(), hour, min, 0, 0, loc), nil
}

func postJSON(url string, payload map[string]any) ([]byte, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Origin", "https://almadenswimracquetclub.theclubspot.com")
	req.Header.Set("Referer", "https://almadenswimracquetclub.theclubspot.com/")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s from %s", resp.Status, url)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

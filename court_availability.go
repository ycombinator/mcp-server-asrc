package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

	var bookings []Booking
	var lastErr error
	for attempt := range 3 {
		sessionToken, err := login(email, password)
		if err != nil {
			if attempt < 2 {
				log.Printf("login attempt %d failed: %v, retrying", attempt+1, err)
				continue
			}
			return mcp.NewToolResultError(fmt.Sprintf("login failed: %v", err)), nil
		}

		bookings, err = fetchBookings(sessionToken, targetDate, loc)
		if err != nil {
			lastErr = err
			log.Printf("fetchBookings attempt %d failed: %v, retrying with fresh login", attempt+1, err)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to fetch bookings: %v", lastErr)), nil
	}

	if len(bookings) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No bookings found for %s — all courts are open!", targetDate.Format("Mon Jan 2, 2006"))), nil
	}

	result, _ := json.MarshalIndent(bookings, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Bookings for %s:\n\n%s", targetDate.Format("Mon Jan 2, 2006"), string(result))), nil
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
				TimeBlocksArray []struct {
					TimeBlockName string `json:"time_block_name"`
				} `json:"time_blocks_array"`
			} `json:"options"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	type rawBooking struct {
		court, bookedBy    string
		startTime, endTime time.Time
	}
	type groupKey struct{ bookedBy, court string }
	groups := make(map[groupKey][]rawBooking)

	for _, court := range response.Result {
		for _, opt := range court.Options {
			start, err := parseHHMM(opt.StartTime, date, loc)
			if err != nil {
				continue
			}
			end, err := parseHHMM(opt.EndTime, date, loc)
			if err != nil {
				continue
			}

			if len(opt.BookedBy) > 0 {
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

			for _, tb := range opt.TimeBlocksArray {
				if tb.TimeBlockName == "" {
					continue
				}
				k := groupKey{tb.TimeBlockName, court.CourtObject.Name}
				groups[k] = append(groups[k], rawBooking{
					court:     court.CourtObject.Name,
					bookedBy:  tb.TimeBlockName,
					startTime: start,
					endTime:   end,
				})
			}
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

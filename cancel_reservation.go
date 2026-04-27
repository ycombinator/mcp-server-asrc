package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func handleCancelReservation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session, ok := userSessionFromContext(ctx)
	if !ok {
		return mcp.NewToolResultError("Authentication required. Please sign in with your ClubSpot credentials."), nil
	}

	args, _ := request.Params.Arguments.(map[string]any)
	reservationID, _ := args["reservation_id"].(string)

	if reservationID == "" {
		return mcp.NewToolResultError("reservation_id is required"), nil
	}

	body, err := postJSON("https://theclubspot.com/parse/functions/cancel_reservation_as_member", map[string]any{
		"reservation_id":  reservationID,
		"club_id":         clubID,
		"is_admin":        false,
		"_ApplicationId":  applicationID,
		"_ClientVersion":  clientVersion,
		"_InstallationId": installationID,
		"_SessionToken":   session.clubspotToken,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to cancel reservation: %v", err)), nil
	}

	var response struct {
		Result struct {
			Status      string `json:"status"`
			SubunitName string `json:"subunitName"`
			StartTime   int    `json:"startTime"`
			EndTime     int    `json:"endTime"`
			DateString  string `json:"dateString"`
			FirstName   string `json:"firstName"`
			LastName    string `json:"lastName"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("unexpected response: %s", string(body))), nil
	}

	r := response.Result
	if r.Status != "canceled" {
		return mcp.NewToolResultError(fmt.Sprintf("cancellation may have failed — status: %s", r.Status)), nil
	}

	startStr := formatHHMM(r.StartTime)
	endStr := formatHHMM(r.EndTime)

	return mcp.NewToolResultText(fmt.Sprintf(
		"Reservation cancelled: %s on %s, %s — %s (%s %s)",
		r.SubunitName, r.DateString, startStr, endStr, r.FirstName, r.LastName,
	)), nil
}

func formatHHMM(hhmm int) string {
	h := hhmm / 100
	m := hhmm % 100
	suffix := "AM"
	if h >= 12 {
		suffix = "PM"
		if h > 12 {
			h -= 12
		}
	}
	if h == 0 {
		h = 12
	}
	return fmt.Sprintf("%d:%02d %s", h, m, suffix)
}

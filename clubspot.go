package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	installationID = "725c7126-0dda-4963-82fe-eebeee058749"
	applicationID  = "myclubspot2017"
	clientVersion  = "js4.3.1-forked-1.0"
	clubID         = "Gh2Ho6cZZu"
	timezone       = "America/Los_Angeles"
)

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

func postJSON(url string, payload map[string]any) ([]byte, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
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
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %s from %s", resp.Status, url)
			continue
		}

		return body, nil
	}
	return nil, lastErr
}

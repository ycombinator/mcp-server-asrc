package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"sync"
	"time"
)

type contextKey string

const userSessionCtxKey contextKey = "user-session"

type userSession struct {
	clubspotToken string
	email         string
	createdAt     time.Time
}

func userSessionFromContext(ctx context.Context) (*userSession, bool) {
	s, ok := ctx.Value(userSessionCtxKey).(*userSession)
	return s, ok && s != nil
}

type authCode struct {
	session             *userSession
	codeChallenge       string
	codeChallengeMethod string
	redirectURI         string
	clientID            string
	expiresAt           time.Time
}

type registeredClient struct {
	clientID     string
	redirectURIs []string
}

type tokenStore struct {
	mu        sync.RWMutex
	tokens    map[string]*userSession
	authCodes map[string]*authCode
	clients   map[string]*registeredClient
}

func newTokenStore() *tokenStore {
	return &tokenStore{
		tokens:    make(map[string]*userSession),
		authCodes: make(map[string]*authCode),
		clients:   make(map[string]*registeredClient),
	}
}

func (ts *tokenStore) storeAuthCode(code string, ac *authCode) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.authCodes[code] = ac
}

func (ts *tokenStore) consumeAuthCode(code string) (*authCode, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ac, ok := ts.authCodes[code]
	if !ok {
		return nil, false
	}
	delete(ts.authCodes, code)
	if time.Now().After(ac.expiresAt) {
		return nil, false
	}
	return ac, true
}

func (ts *tokenStore) storeToken(token string, session *userSession) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.tokens[token] = session
}

func (ts *tokenStore) lookupToken(token string) (*userSession, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	s, ok := ts.tokens[token]
	return s, ok
}

func (ts *tokenStore) invalidateToken(token string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.tokens, token)
}

func (ts *tokenStore) registerClient(c *registeredClient) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.clients[c.clientID] = c
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func verifyPKCE(verifier, challenge string) bool {
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return computed == challenge
}

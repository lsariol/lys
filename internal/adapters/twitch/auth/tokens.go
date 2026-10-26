package auth

import "sync"

// SafeTwitchTokens holds the current tokens for concurrent readers.
type SafeTwitchTokens struct {
	mu               sync.RWMutex
	userAccessToken  string
	userRefreshToken string
	appAccessToken   string
}

func NewSafeTwitchTokens() *SafeTwitchTokens {
	return &SafeTwitchTokens{}
}

func (s *SafeTwitchTokens) GetUserAccessToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userAccessToken
}

func (s *SafeTwitchTokens) GetUserRefreshToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userRefreshToken
}

func (s *SafeTwitchTokens) GetAppAccessToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.appAccessToken
}

func (s *SafeTwitchTokens) SetUserAccessTokens(accessToken string, refreshToken string) {
	s.mu.Lock()
	s.userAccessToken = accessToken
	s.userRefreshToken = refreshToken
	s.mu.Unlock()
}

func (s *SafeTwitchTokens) SetAppAccessTokens(appAccessToken string) {
	s.mu.Lock()
	s.appAccessToken = appAccessToken
	s.mu.Unlock()
}

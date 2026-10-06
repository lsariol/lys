package auth

import (
	"sync"

	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
)

type SafeTwitchTokens struct {
	mu               sync.RWMutex
	userAccessTokens twitchdb.TwitchUserAccessTokens
	appAccessTokens  twitchdb.TwitchAppAccessToken
}

func NewSafeTwitchTokens() *SafeTwitchTokens {
	return &SafeTwitchTokens{
		userAccessTokens: twitchdb.TwitchUserAccessTokens{},
		appAccessTokens:  twitchdb.TwitchAppAccessToken{},
	}
}

func (s *SafeTwitchTokens) GetUserAccessToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userAccessTokens.UserAccessToken
}

func (s *SafeTwitchTokens) GetUserRefreshToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userAccessTokens.UserRefreshToken
}

func (s *SafeTwitchTokens) GetAppAccessToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.appAccessTokens.TwitchAppAccessToken
}

func (s *SafeTwitchTokens) SetUserAccessTokens(accessToken string, refreshToken string) {
	s.mu.Lock()
	s.userAccessTokens.UserAccessToken = accessToken
	s.userAccessTokens.UserRefreshToken = refreshToken
	s.mu.Unlock()
}

func (s *SafeTwitchTokens) SetAppAccessTokens(appAccessToken string) {
	s.mu.Lock()
	s.appAccessTokens.TwitchAppAccessToken = appAccessToken
	s.mu.Unlock()
}

// func (s *SafeTwitchTokens) UserAccessToken() string {
// 	s.mu.RLock()
// 	defer s.mu.RUnlock()
// 	return s.tokens.UserAccessToken
// }

// func (s *SafeTwitchTokens) UserRefreshToken() string {
// 	s.mu.RLock()
// 	defer s.mu.RUnlock()
// 	return s.tokens.UserRefreshToken
// }

// func (s *SafeTwitchTokens) Tokens() (access, refresh string) {
// 	s.mu.RLock()
// 	defer s.mu.RUnlock()
// 	return s.tokens.UserAccessToken, s.tokens.UserRefreshToken
// }

// func (s *SafeTwitchTokens) SetTokens(access string, refresh string) {
// 	s.mu.Lock()
// 	s.tokens.UserAccessToken = access
// 	s.tokens.UserRefreshToken = refresh
// 	s.mu.Unlock()
// }

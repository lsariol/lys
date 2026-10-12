package auth

func (a *AuthClient) updateTokens() error {

	a.Cove.UpdateSecret("LYS_TWITCH_ACCESS_TOKEN", a.Config.TokenPair.AccessToken)
	a.Cove.UpdateSecret("LYS_TWITCH_REFRESH_TOKEN", a.Config.TokenPair.RefreshToken)

	return nil
}

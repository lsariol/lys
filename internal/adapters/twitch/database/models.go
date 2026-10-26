package twitchdb

type AddChannelParams struct {
	UserID       string
	Username     string
	AccessToken  string
	RefreshToken string
}

type TwitchChannel struct {
	UserID         string
	Username       string
	Role           string
	SessionID      string
	SubscriptionID string
}

type TwitchChannelSettings struct {
	UserID        string
	Username      string
	CommandPrefix string
}

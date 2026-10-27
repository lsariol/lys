package broker

import "github.com/jackc/pgx/v5/pgxpool"

type Config struct {
	Address string
	Port    string
}

type APIStore struct {
	pool             *pgxpool.Pool
	connectionString string
}

// Letterboxd
type LetterboxdNotification struct {
	Message       string   `json:"message"`
	AlertChannels []string `json:"alert_channels"`
	UserId        string   `json:"user_id"`
	GUID          string   `json:"guid"`
}

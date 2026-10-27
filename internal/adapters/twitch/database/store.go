package twitchdb

import "github.com/jackc/pgx/v5/pgxpool"

type Store struct {
	pool             *pgxpool.Pool
	connectionString string
}

func NewStore(p *pgxpool.Pool, c string) *Store {
	return &Store{
		pool:             p,
		connectionString: c,
	}
}

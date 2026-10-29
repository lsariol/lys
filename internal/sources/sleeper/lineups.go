package sleeper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/lsariol/lys/internal/features/sleeper"
	"github.com/lsariol/lys/internal/sources"
)

// Lineups is the source that reminds each league's channels to set their
// lineups: 1 day and 1 hour before the week's first game, and 1 hour before
// Sunday's first game when that's a different one. Sleeper locks each player
// at their own game's kickoff, so these are the two times most lineups lock.
// Reminders are queued for the sender (sender.go), like every announcement.
type Lineups struct {
	client *api.Client
	db     *pgxpool.Pool
	now    func() time.Time
	every  time.Duration // the longest it waits, so new leagues and moved kickoffs are seen
}

var _ sources.Source = (*Lineups)(nil)

func NewLineups(client *api.Client, db *pgxpool.Pool) *Lineups {
	return &Lineups{client: client, db: db, now: time.Now, every: 10 * time.Minute}
}

func (l *Lineups) Name() string { return "sleeper lineups" }

// lateBy is how late a reminder may still go out (after a restart, say);
// later than that, a "1 day" reminder hours before kickoff would mislead.
const lateBy = 30 * time.Minute

// regularWeeks is the length of the NFL regular season.
const regularWeeks = 18

func (l *Lineups) Run(ctx context.Context, _ sources.Publisher) error {
	for {
		next, err := l.tick(ctx)
		if err != nil {
			log.Printf("[Sleeper] lineup reminders: %v", err)
		}
		wait := l.every
		if !next.IsZero() {
			wait = min(wait, max(next.Sub(l.now()), time.Second))
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

// reminder is one lineup reminder for every league.
type reminder struct {
	at   time.Time
	id   string // unique per season, week, game and offset
	text string
}

// tick queues the reminders that are due and returns when the next one is.
func (l *Lineups) tick(ctx context.Context) (time.Time, error) {
	state, err := l.client.NFLState(ctx)
	if err != nil {
		return time.Time{}, err
	}
	var weeks []int
	switch state.SeasonType {
	case "regular":
		weeks = []int{state.Week}
		if state.Week < regularWeeks {
			weeks = append(weeks, state.Week+1) // its "1 day" reminder can fall in this week
		}
	case "pre":
		weeks = []int{1}
	default:
		return time.Time{}, nil
	}

	now := l.now()
	var due []reminder
	var next time.Time
	for _, week := range weeks {
		games, err := l.client.Games(ctx, state.Season, week)
		if err != nil {
			log.Printf("[Sleeper] lineup reminders: %v", err)
			continue
		}
		for _, r := range reminders(state.Season, week, games) {
			switch {
			case !now.Before(r.at) && now.Before(r.at.Add(lateBy)):
				due = append(due, r)
			case r.at.After(now) && (next.IsZero() || r.at.Before(next)):
				next = r.at
			}
		}
	}
	if len(due) == 0 {
		return next, nil
	}

	rows, err := l.db.Query(ctx, `
		SELECT sleeper_league_id FROM lys.sleeper_leagues
		WHERE enabled AND community_id IS NOT NULL AND COALESCE((notifications->>'lineups')::boolean, true)`)
	if err != nil {
		return next, fmt.Errorf("database: read Sleeper leagues: %w", err)
	}
	leagues, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return next, fmt.Errorf("database: read Sleeper leagues: %w", err)
	}

	for _, r := range due {
		data, err := json.Marshal(queuedText{Text: r.text})
		if err != nil {
			return next, err
		}
		for _, league := range leagues {
			ev := event{league: league, kind: "lineup_reminder", id: r.id}
			if _, err := record(ctx, l.db, ev, data, r.at, true); err != nil {
				return next, err
			}
		}
	}
	return next, nil
}

// reminders plans a week's reminders from its games, earliest first.
func reminders(season string, week int, games []api.Game) []reminder {
	if len(games) == 0 {
		return nil
	}
	id := func(g api.Game, offset string) string {
		return fmt.Sprintf("%s-w%d-%d-%s", season, week, g.Kickoff.Unix(), offset)
	}
	at := func(g api.Game, format string) string {
		return g.Name() + ", " + g.Kickoff.In(api.Eastern).Format(format) + " ET"
	}

	first := games[0]
	out := []reminder{
		{first.Kickoff.Add(-24 * time.Hour), id(first, "1d"),
			fmt.Sprintf("🏈 Week %d starts in 1 day: %s. Set your lineups!", week, at(first, "Mon 3:04 PM"))},
		{first.Kickoff.Add(-time.Hour), id(first, "1h"),
			fmt.Sprintf("🏈 1 hour until Week %d's first game: %s. Set your lineups!", week, at(first, "3:04 PM"))},
	}
	for _, g := range games {
		if g.Kickoff.In(api.Eastern).Weekday() != time.Sunday {
			continue
		}
		if !g.Kickoff.Equal(first.Kickoff) {
			out = append(out, reminder{g.Kickoff.Add(-time.Hour), id(g, "1h"),
				fmt.Sprintf("🏈 1 hour until Sunday's first game: %s. Set your lineups!", at(g, "3:04 PM"))})
		}
		break
	}
	return out
}

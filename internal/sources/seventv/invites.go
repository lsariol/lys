// Package seventv is the source that keeps Lys's 7TV account in order: it
// accepts editor invites from the channels Lys serves, picks up a new
// session from Cove, and warns before the session expires. It publishes
// nothing.
package seventv

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/lsariol/lys/internal/core"
	api "github.com/lsariol/lys/internal/features/seventv"
	"github.com/lsariol/lys/internal/sources"
)

// Invites is the source; one check every few minutes.
type Invites struct {
	client   *api.Client
	channels core.Directory
	every    time.Duration
	now      func() time.Time

	logged map[string]bool // invites left pending, by 7TV user ID, logged once each
	warned time.Time       // when the expiry was last warned about
	failed string          // the last error logged, so a lasting one is logged once
}

var _ sources.Source = (*Invites)(nil)

func NewInvites(client *api.Client, channels core.Directory) *Invites {
	return &Invites{client: client, channels: channels, every: 5 * time.Minute, now: time.Now, logged: map[string]bool{}}
}

func (s *Invites) Name() string { return "7tv invites" }

func (s *Invites) Run(ctx context.Context, _ sources.Publisher) error {
	t := time.NewTicker(s.every)
	defer t.Stop()
	for {
		s.check(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// check reads the session again, warns if it expires soon, and accepts the
// pending invites from channels Lys serves.
func (s *Invites) check(ctx context.Context) {
	if err := s.client.Reload(ctx); err != nil {
		s.fail(err)
		return
	}
	s.warnExpiry(ctx)

	invites, err := s.client.Invites(ctx)
	if err != nil {
		s.fail(err)
		return
	}
	channels, err := s.channels.Channels(ctx, "twitch")
	if err != nil {
		s.fail(err)
		return
	}
	s.failed = ""
	served := make(map[string]string, len(channels))
	for _, c := range channels {
		served[c.ID] = c.Name
	}

	for _, in := range invites {
		if in.State != api.Pending {
			continue
		}
		name, ok := served[in.TwitchID]
		if in.TwitchID == "" || !ok {
			if !s.logged[in.UserID] {
				s.logged[in.UserID] = true
				log.Printf("! [7TV] left the editor invite from %q (Twitch %s, 7TV %s) pending: Lys isn't in that channel", in.Login, in.TwitchID, in.UserID)
			}
			continue
		}
		if err := s.client.AcceptInvite(ctx, in.UserID); err != nil {
			log.Printf("✗ [7TV] accept the editor invite from %q (Twitch %s, 7TV %s): %v", name, in.TwitchID, in.UserID, err)
			continue
		}
		log.Printf("[7TV] accepted the editor invite from %q (Twitch %s, 7TV %s)", name, in.TwitchID, in.UserID)
	}
}

// warnExpiry logs, once a day, that the session expires within a week.
func (s *Invites) warnExpiry(ctx context.Context) {
	exp, ok := s.client.Expiry(ctx)
	now := s.now()
	if !ok || exp.Sub(now) > api.WarnBefore || now.Sub(s.warned) < 24*time.Hour {
		return
	}
	s.warned = now
	log.Printf("! %s", api.Renewal(exp, now))
}

// fail logs an error unless it's the one logged last.
func (s *Invites) fail(err error) {
	if err.Error() == s.failed || errors.Is(err, context.Canceled) {
		return
	}
	s.failed = err.Error()
	log.Printf("! [7TV] invites: %v", err)
}

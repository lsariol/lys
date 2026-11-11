// Package seventv is the source that accepts 7TV editor invites from served channels and watches the session.
package seventv

import (
	"context"
	"errors"
	"log/slog"
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
	log    *slog.Logger
}

var _ sources.Source = (*Invites)(nil)

func NewInvites(client *api.Client, channels core.Directory, log *slog.Logger) *Invites {
	return &Invites{client: client, channels: channels, every: 5 * time.Minute, now: time.Now, logged: map[string]bool{},
		log: log.With("component", "seventv")}
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

// check reads the session again, warns if it expires soon, and accepts invites from channels Lys serves.
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
				s.log.Warn("left an editor invite pending: Lys isn't in that channel", "login", in.Login, "channel_id", in.TwitchID, "seventv_id", in.UserID)
			}
			continue
		}
		if err := s.client.AcceptInvite(ctx, in.UserID); err != nil {
			s.log.Error("accepting an editor invite failed", "channel", name, "channel_id", in.TwitchID, "seventv_id", in.UserID, "err", err)
			continue
		}
		s.log.Info("accepted an editor invite", "channel", name, "channel_id", in.TwitchID, "seventv_id", in.UserID)
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
	s.log.Warn("7TV session expires within a week or has expired: log into 7tv.app as Lys's bot account, then cove shell: update "+api.TokenKey, "expires", exp)
}

// fail logs an error unless it's the one logged last.
func (s *Invites) fail(err error) {
	if err.Error() == s.failed || errors.Is(err, context.Canceled) {
		return
	}
	s.failed = err.Error()
	if errors.Is(err, api.ErrNoToken) || errors.Is(err, api.ErrSession) {
		s.log.Error("7TV session unusable: the 7TV commands can't change anything", "err", err)
		return
	}
	s.log.Warn("checking the editor invites failed", "err", err)
}

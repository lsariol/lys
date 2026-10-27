package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/lsariol/coveclient"
	"github.com/lsariol/lys/internal/adapters/twitch/auth"
	"github.com/lsariol/lys/internal/botaccounts"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/connections"
	"github.com/lsariol/lys/internal/database"
)

// Services is what commands reach, loaded on first use.
type Services struct {
	Config *config.Config
	Cove   *coveclient.Client
	HTTP   *http.Client
	// Database connects as the app role and checks the schema is current.
	Database func(ctx context.Context) (*database.Database, error)
}

// timeFormat is how the CLI shows times.
const timeFormat = "2006-01-02 15:04"

// defaultRedirect is the redirect address registered on Lys's Twitch app for the bot.
const defaultRedirect = "http://localhost:3000"

var table []Command

func init() {
	table = []Command{
		{
			Names:   []string{"channel list", "channels"},
			Group:   "Channels",
			Summary: "List the channels Lys is in, with their state and prefix",
			Usage:   []string{"channel list"},
			Run:     runChannelList,
		},
		{
			Names:   []string{"channel add"},
			Group:   "Channels",
			Summary: "Add a Twitch channel by its login, or turn a removed one back on",
			Usage:   []string{"channel add <login>"},
			Examples: []Example{
				{"channel add shylaughhaha", "Add the channel; a running Lys joins it within seconds."},
			},
			Run: runChannelAdd,
		},
		{
			Names:   []string{"channel block"},
			Group:   "Channels",
			Summary: "Stop Lys from joining a channel, whatever its owner wants",
			Usage:   []string{"channel block <channel> [--reason <text>] [--yes]"},
			Flags: []Flag{
				{Name: "reason", Value: "<text>", Summary: "Why; shown by channel list while it's blocked"},
				{Name: "yes", Short: "y", Summary: "Block without asking"},
			},
			Examples: []Example{
				{`channel block spambot --reason "spam"`, "Block by name; a Twitch ID works too."},
			},
			Run: runChannelBlock,
		},
		{
			Names:   []string{"channel unblock"},
			Group:   "Channels",
			Summary: "Let Lys join a blocked channel again",
			Usage:   []string{"channel unblock <channel>"},
			Run:     runChannelUnblock,
		},
		{
			Names:   []string{"bot authorize"},
			Group:   "Bot",
			Summary: "Sign Lys in as its Twitch bot account and store the tokens in Cove",
			Usage:   []string{"bot authorize [--redirect-uri <address>]", "bot authorize --code <code> [--redirect-uri <address>]"},
			Flags: []Flag{
				{Name: "redirect-uri", Value: "<address>", Summary: "The redirect address registered on the Twitch app (default " + defaultRedirect + ")"},
				{Name: "code", Value: "<code>", Summary: "The code from Twitch's redirect, when there's no terminal to paste it into"},
			},
			Examples: []Example{
				{"bot authorize", "Prints an address to open signed in as the bot, then asks for the address Twitch sends you back to."},
			},
			Run: runBotAuthorize,
		},
		{
			Names:   []string{"migrate status", "migrate"},
			Group:   "Database",
			Summary: "List the database migrations and when each was applied",
			Usage:   []string{"migrate status"},
			Run:     runMigrateStatus,
		},
		{
			Names:   []string{"migrate up"},
			Group:   "Database",
			Summary: "Apply pending migrations (Lys also does this when it starts)",
			Usage:   []string{"migrate up"},
			Run:     runMigrateUp,
		},
		{
			Names:    []string{"help"},
			Group:    "Help",
			Summary:  "Show the commands, or how to use one",
			Usage:    []string{"help [command | guide]"},
			Examples: []Example{{"help channel block", "Usage, flags and examples for one command."}},
			Run:      runHelp,
			Complete: completeCommands,
		},
	}
}

func connectionsStore(ctx context.Context, env *Env) (*Services, *connections.Store, error) {
	s, err := env.Services(ctx)
	if err != nil {
		return nil, nil, err
	}
	db, err := s.Database(ctx)
	if err != nil {
		return nil, nil, err
	}
	return s, connections.New(db.Pool), nil
}

func runChannelList(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) > 0 {
		return usageError("channel list takes no arguments.")
	}
	_, conns, err := connectionsStore(ctx, env)
	if err != nil {
		return err
	}
	all, err := conns.List(ctx)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(env.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PLATFORM\tID\tNAME\tSTATE\tPREFIX\tCONNECTED")
	for _, c := range all {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", c.Platform, c.ExternalID, c.Name, state(c), c.Prefix, c.ConnectedAt.Local().Format(timeFormat))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "%d %s.\n", len(all), plural(len(all), "channel", "channels"))
	return nil
}

func state(c connections.Connection) string {
	var parts []string
	switch {
	case c.AdminBlocked && c.BlockReason != "":
		parts = append(parts, "blocked ("+c.BlockReason+")")
	case c.AdminBlocked:
		parts = append(parts, "blocked")
	}
	switch {
	case !c.Authorized && c.WasAuthorized:
		parts = append(parts, "revoked")
	case !c.Authorized:
		parts = append(parts, "not authorized")
	}
	if !c.Enabled {
		parts = append(parts, "disabled")
	}
	if len(parts) == 0 {
		return "active"
	}
	return strings.Join(parts, ", ")
}

func runChannelAdd(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) != 1 {
		return usageError("Name one Twitch login.")
	}
	s, conns, err := connectionsStore(ctx, env)
	if err != nil {
		return err
	}

	a := auth.New(s.Cove, s.Config.Twitch, s.HTTP)
	if err := a.RefreshAppAccessToken(ctx); err != nil {
		return err
	}
	u, err := a.LookupUser(ctx, strings.TrimPrefix(in.Args[0], "@"))
	if err != nil {
		return err
	}

	c, created, err := conns.Enroll(ctx, connections.Enrollment{Platform: "twitch", ExternalID: u.ID, Name: u.DisplayName, Login: u.Login})
	if err != nil {
		return err
	}
	if created {
		env.Success("Added %q (twitch %s).", c.Name, c.ExternalID)
	} else {
		env.Success("%q (twitch %s) was already a channel; it's turned on again.", c.Name, c.ExternalID)
	}
	if c.AdminBlocked {
		env.Warn("%q is blocked, so Lys won't join it: lys channel unblock %s.", c.Name, c.ExternalID)
		return nil
	}
	env.Warn("A running Lys joins it within seconds. It can reply once the broadcaster mods the bot or grants channel:bot; see lys help new-channel.")
	return nil
}

func runChannelBlock(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) != 1 {
		return usageError("Name one channel, by name or Twitch ID.")
	}
	_, conns, err := connectionsStore(ctx, env)
	if err != nil {
		return err
	}
	c, err := conns.Find(ctx, "twitch", in.Args[0])
	if err != nil {
		return err
	}
	if c.AdminBlocked {
		env.Success("%q is already blocked.", c.Name)
		return nil
	}
	if err := env.Confirm(in, "Block", fmt.Sprintf("Block %q (twitch %s)? Lys stops joining it.", c.Name, c.ExternalID)); err != nil {
		return err
	}
	if err := conns.SetBlocked(ctx, c.ID, true, in.Flags["reason"]); err != nil {
		return err
	}
	env.Success("Blocked %q. A running Lys leaves it within seconds.", c.Name)
	return nil
}

func runChannelUnblock(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) != 1 {
		return usageError("Name one channel, by name or Twitch ID.")
	}
	_, conns, err := connectionsStore(ctx, env)
	if err != nil {
		return err
	}
	c, err := conns.Find(ctx, "twitch", in.Args[0])
	if err != nil {
		return err
	}
	if !c.AdminBlocked {
		env.Success("%q isn't blocked.", c.Name)
		return nil
	}
	if err := conns.SetBlocked(ctx, c.ID, false, ""); err != nil {
		return err
	}
	env.Success("Unblocked %q. A running Lys joins it within seconds.", c.Name)
	return nil
}

func runBotAuthorize(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) > 0 {
		return usageError("bot authorize takes no arguments.")
	}
	redirect := in.Flags["redirect-uri"]
	if redirect == "" {
		redirect = defaultRedirect
	}

	s, err := env.Services(ctx)
	if err != nil {
		return err
	}
	db, err := s.Database(ctx)
	if err != nil {
		return err
	}
	bots := botaccounts.New(db.Pool)
	cfg := *s.Config.Twitch
	previous, hadBot, err := bots.Default(ctx, "twitch")
	if err != nil {
		return err
	}
	if hadBot {
		cfg.AccessTokenKey = previous.AccessTokenKey
		if previous.RefreshTokenKey != "" {
			cfg.RefreshTokenKey = previous.RefreshTokenKey
		}
	}

	code := in.Flags["code"]
	if code == "" {
		if !env.Terminal {
			return usageError("Without a terminal to paste into, pass the code with --code.")
		}
		state, err := randomState()
		if err != nil {
			return err
		}
		fmt.Fprintln(env.Err, "1. Open this address in a browser signed in to Twitch as the bot account:")
		fmt.Fprintln(env.Out, auth.AuthorizeURL(cfg.ClientID, redirect, state, auth.BotScopes))
		fmt.Fprintf(env.Err, "2. Approve. Twitch sends the browser to %s (the page may not load).\n", redirect)
		fmt.Fprintln(env.Err, "3. Copy the whole address from the address bar.")
		pasted, err := env.Ask("Paste it here:")
		if err != nil {
			return err
		}
		if code, err = auth.ParseRedirect(pasted, state); err != nil {
			return err
		}
	}

	a := auth.New(s.Cove, &cfg, s.HTTP)
	access, refresh, err := a.ExchangeCode(ctx, code, redirect)
	if err != nil {
		return err
	}
	info, err := auth.ValidateToken(access, s.HTTP)
	if err != nil {
		return fmt.Errorf("Twitch: validate the new token: %w", err)
	}

	// Refresh token first: it's the one that can't be got back.
	for _, kv := range [][2]string{{cfg.RefreshTokenKey, refresh}, {cfg.AccessTokenKey, access}} {
		if err := storeSecret(s.Cove, kv[0], kv[1]); err != nil {
			return err
		}
	}

	if err := bots.SaveDefault(ctx, botaccounts.Account{
		Platform: "twitch", ExternalID: info.UserId, Login: info.Login,
		AccessTokenKey: cfg.AccessTokenKey, RefreshTokenKey: cfg.RefreshTokenKey, Scopes: info.Scopes,
	}); err != nil {
		return err
	}

	env.Success("Authorized %q (twitch %s) with %s; its tokens are in %s and %s.", info.Login, info.UserId, strings.Join(info.Scopes, " "), cfg.AccessTokenKey, cfg.RefreshTokenKey)
	if hadBot && previous.ExternalID != info.UserId {
		env.Warn("The bot account changed from %q to %q.", previous.Login, info.Login)
	}
	env.Warn("Restart Lys to use the new tokens: docker restart lys.")
	return nil
}

// storeSecret updates a Cove key, or creates it if it doesn't exist.
func storeSecret(cove *coveclient.Client, key, value string) error {
	err := cove.UpdateSecret(key, value)
	if errors.Is(err, coveclient.ErrNotFound) {
		_, err = cove.AddSecret(key, value)
	}
	if err != nil {
		return fmt.Errorf("Cove: store %s: %w (Lys's Cove token needs write access to it)", key, err)
	}
	return nil
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("make a state value: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func migratorURL(ctx context.Context, env *Env) (string, error) {
	s, err := env.Services(ctx)
	if err != nil {
		return "", err
	}
	u := s.Config.Database.MigratorConnectionString
	if u == "" {
		return "", errors.New("LYS_DATABASE_MIGRATOR_URL isn't in Cove. Add the lys_migrator connection string there (cove shell: create LYS_DATABASE_MIGRATOR_URL)")
	}
	return u, nil
}

func runMigrateStatus(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) > 0 {
		return usageError("migrate status takes no arguments.")
	}
	u, err := migratorURL(ctx, env)
	if err != nil {
		return err
	}
	return database.PrintMigrationStatus(ctx, u, env.Out)
}

func runMigrateUp(ctx context.Context, env *Env, in Input) error {
	if len(in.Args) > 0 {
		return usageError("migrate up takes no arguments.")
	}
	u, err := migratorURL(ctx, env)
	if err != nil {
		return err
	}
	return database.Migrate(ctx, u)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

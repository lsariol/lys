package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/lsariol/blackboxlog"
	"github.com/lsariol/coveclient"
	"github.com/lsariol/lys/internal/activity"
	twitchbot "github.com/lsariol/lys/internal/adapters/twitch/bot"
	"github.com/lsariol/lys/internal/botaccounts"
	"github.com/lsariol/lys/internal/channelconfig"
	"github.com/lsariol/lys/internal/cli"
	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/commands/cmds/autopost"
	"github.com/lsariol/lys/internal/commands/cmds/catfacts"
	"github.com/lsariol/lys/internal/commands/cmds/chatgpt"
	"github.com/lsariol/lys/internal/commands/cmds/help"
	"github.com/lsariol/lys/internal/commands/cmds/letterboxd"
	"github.com/lsariol/lys/internal/commands/cmds/oneuppuzzle"
	"github.com/lsariol/lys/internal/commands/cmds/ping"
	"github.com/lsariol/lys/internal/commands/cmds/randomcat"
	"github.com/lsariol/lys/internal/commands/cmds/randomdog"
	"github.com/lsariol/lys/internal/commands/cmds/remind"
	"github.com/lsariol/lys/internal/commands/cmds/request"
	"github.com/lsariol/lys/internal/commands/cmds/settings"
	sevencmd "github.com/lsariol/lys/internal/commands/cmds/seventv"
	"github.com/lsariol/lys/internal/commands/cmds/sleeper"
	"github.com/lsariol/lys/internal/commands/cmds/uptime"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/connections"
	"github.com/lsariol/lys/internal/database"
	"github.com/lsariol/lys/internal/enroll"
	sevenapi "github.com/lsariol/lys/internal/features/seventv"
	sleeperapi "github.com/lsariol/lys/internal/features/sleeper"
	"github.com/lsariol/lys/internal/identity"
	"github.com/lsariol/lys/internal/router"
	"github.com/lsariol/lys/internal/scheduler"
	"github.com/lsariol/lys/internal/server"
	"github.com/lsariol/lys/internal/sources"
	"github.com/lsariol/lys/internal/sources/inlet"
	letterboxdfeeds "github.com/lsariol/lys/internal/sources/letterboxd"
	sevenfeed "github.com/lsariol/lys/internal/sources/seventv"
	sleeperfeed "github.com/lsariol/lys/internal/sources/sleeper"
)

// workers is how many commands may run at once.
const workers = 32

func main() {
	args := os.Args[1:]
	started := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	_ = godotenv.Load(".env")

	if len(args) > 0 {
		os.Exit(cli.Run(ctx, cli.NewEnv(cliServices()), args))
	}
	log := blackboxlog.New(blackboxlog.Options{})
	slog.SetDefault(log)
	log.Info("Lys starting")
	fail := func(err error) {
		log.Error("startup failed", "err", err)
		os.Exit(1)
	}

	cove, err := newCove()
	if err != nil {
		fail(err)
	}
	cfg := config.New()
	if err := cfg.Initialize(cove); err != nil {
		fail(fmt.Errorf("initialize config: %w", err))
	}

	dblog := log.With("component", "database")
	if migratorURL := cfg.Database.MigratorConnectionString; migratorURL != "" {
		applied, err := database.Migrate(ctx, migratorURL)
		for _, m := range applied {
			dblog.Info("migration applied", "file", m.File, "duration_ms", m.Duration.Milliseconds())
		}
		if err != nil {
			fail(err)
		}
		if len(applied) == 0 {
			dblog.Info("schema up to date")
		}
	} else {
		dblog.Warn("LYS_DATABASE_MIGRATOR_URL isn't in Cove: skipping migrations")
	}

	db := database.NewDatabase(cfg.Database)
	if err := db.Connect(ctx); err != nil {
		fail(err)
	}
	defer db.Close()

	if err := db.CheckSchemaVersion(ctx); err != nil {
		fail(err)
	}

	if err := run(ctx, log, cfg, cove, db, started); err != nil {
		log.Error("Lys stopped", "err", err)
		db.Close()
		os.Exit(1)
	}
	log.Info("shutting down")
}

// run wires the router, adapters, commands, sources and HTTP server, and blocks until ctx ends or an adapter fails.
func run(ctx context.Context, log *slog.Logger, cfg *config.Config, cove *coveclient.Client, db *database.Database, started time.Time) error {

	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 10 * time.Second,
	}

	conns := connections.New(db.Pool)
	channels := channelconfig.New(conns)
	if err := channels.Load(ctx); err != nil {
		return err
	}

	bots := botaccounts.New(db.Pool)
	if err := useBotAccount(ctx, bots, cfg.Twitch); err != nil {
		return err
	}

	jobs := scheduler.NewStore(db.Pool)

	// Shared, so the player catalog is kept once and the 7TV session read once.
	sleeperClient := sleeperapi.New(httpClient, log)
	sevenClient := sevenapi.New(httpClient, sevenapi.Endpoint, sevenapi.FromCove(cove))
	registry := commands.NewRegistry()
	for _, c := range []commands.Command{
		catfacts.New(httpClient),
		chatgpt.New(cfg.Commands.ChatGPTKey),
		help.New(registry),
		letterboxd.New(letterboxd.NewStore(db.Pool), httpClient),
		oneuppuzzle.New(db.Pool),
		ping.New(httpClient),
		randomcat.New(httpClient),
		randomdog.New(httpClient),
		remind.New(jobs),
		request.New(request.NewStore(db.Pool)),
		settings.New(channels),
		sevencmd.New(sevenClient, log),
		sleeper.New(sleeperClient, db.Pool, log),
		autopost.New(jobs),
		uptime.New(started),
	} {
		if err := registry.Register(c); err != nil {
			return err
		}
	}

	r := router.New(registry, channels, identity.New(db.Pool), workers, log)
	chat := activity.New(autopost.MinMessages)
	r.Observe(chat)
	twitch := twitchbot.New(httpClient, cfg.Twitch, cove, conns, bots, conns, log)
	r.Add(twitch)

	notifications := inlet.New(os.Getenv("SHARED_LYS_NOTIFICATION_TOKEN"), log)

	port := os.Getenv("NOTIFICATION_PORT")
	if port == "" {
		return errors.New("NOTIFICATION_PORT is not set: set it to the port the HTTP server listens on (2400 in the compose file)")
	}
	routes := map[string]http.Handler{"POST /notifications": notifications}
	for pattern, h := range enroll.New(conns, os.Getenv("SHARED_LYS_ENROLLMENT_TOKEN"), log, twitch).Routes() {
		routes[pattern] = h
	}
	srv := server.New(":"+port, version(), r.Health, routes, log)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channel changes (from the CLI too) reach the settings cache and the adapters within seconds.
	go func() {
		connlog := log.With("component", "connections")
		err := conns.Watch(ctx, log, 5*time.Minute, func(platform string) {
			if err := channels.Load(ctx); err != nil {
				connlog.Error("reloading channel settings failed", "err", err)
			}
			r.SyncChannels(ctx, platform)
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			connlog.Error("watching channel changes stopped: channel changes reach Lys only after a restart", "err", err)
		}
	}()

	go func() {
		if err := srv.Run(ctx); err != nil {
			log.Error("HTTP server stopped", "component", "server", "err", err)
			cancel()
		}
	}()

	schedule := scheduler.New(jobs, map[string]scheduler.Handler{
		remind.Kind:   scheduler.SendMessage,
		autopost.Kind: autopost.Post(chat),
	}, log)

	for _, s := range []sources.Source{notifications, schedule, letterboxdfeeds.New(db.Pool, httpClient, log), sleeperfeed.New(sleeperClient, db.Pool, log), sleeperfeed.NewLineups(sleeperClient, db.Pool, log),
		sleeperfeed.NewRecaps(sleeperClient, db.Pool, log), sleeperfeed.NewSender(sleeperClient, db.Pool, log), sevenfeed.NewInvites(sevenClient, conns, log)} {
		go func() {
			if err := s.Run(ctx, r); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("source stopped", "source", s.Name(), "err", err)
			}
		}()
	}

	return r.Run(ctx)
}

// useBotAccount points the Twitch config at the default bot account; without one Lys can't start (lys bot authorize).
func useBotAccount(ctx context.Context, bots *botaccounts.Store, cfg *config.TwitchConfig) error {
	bot, found, err := bots.Default(ctx, twitchbot.Platform)
	if err != nil || !found {
		return err
	}
	cfg.BotAccountID = bot.ExternalID
	cfg.AccessTokenKey = bot.AccessTokenKey
	if bot.RefreshTokenKey != "" {
		cfg.RefreshTokenKey = bot.RefreshTokenKey
	}
	return nil
}

// version is the release or commit Lighthouse deployed (LYS_VERSION), or "dev".
func version() string {
	if v := os.Getenv("LYS_VERSION"); v != "" {
		return v
	}
	return "dev"
}

// newCove connects to Cove with COVE_URL and COVE_TOKEN.
func newCove() (*coveclient.Client, error) {
	url, token := os.Getenv("COVE_URL"), os.Getenv("COVE_TOKEN")
	if url == "" || token == "" {
		return nil, errors.New("Cove: COVE_URL and COVE_TOKEN must be set (the compose file sets them; for a local run, put them in .env)")
	}
	return coveclient.New(url, token), nil
}

// cliServices loads Cove, the config and the database for CLI commands, once, when a command first needs them.
func cliServices() func(ctx context.Context) (*cli.Services, error) {
	var (
		once     sync.Once
		services *cli.Services
		err      error
	)
	return func(ctx context.Context) (*cli.Services, error) {
		once.Do(func() {
			var cove *coveclient.Client
			if cove, err = newCove(); err != nil {
				return
			}
			cfg := config.New()
			if err = cfg.Initialize(cove); err != nil {
				err = fmt.Errorf("initialize config: %w", err)
				return
			}

			var (
				dbOnce sync.Once
				db     *database.Database
				dbErr  error
			)
			services = &cli.Services{
				Config: cfg,
				Cove:   cove,
				HTTP:   &http.Client{Timeout: 10 * time.Second},
				Database: func(ctx context.Context) (*database.Database, error) {
					dbOnce.Do(func() {
						db = database.NewDatabase(cfg.Database)
						if dbErr = db.Connect(ctx); dbErr != nil {
							return
						}
						dbErr = db.CheckSchemaVersion(ctx)
					})
					return db, dbErr
				},
			}
		})
		return services, err
	}
}

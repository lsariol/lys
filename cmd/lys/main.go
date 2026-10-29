package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
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
	"github.com/lsariol/lys/internal/commands/cmds/randomdog"
	"github.com/lsariol/lys/internal/commands/cmds/remind"
	"github.com/lsariol/lys/internal/commands/cmds/request"
	"github.com/lsariol/lys/internal/commands/cmds/settings"
	"github.com/lsariol/lys/internal/commands/cmds/sleeper"
	"github.com/lsariol/lys/internal/commands/cmds/uptime"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/connections"
	"github.com/lsariol/lys/internal/database"
	"github.com/lsariol/lys/internal/enroll"
	sleeperapi "github.com/lsariol/lys/internal/features/sleeper"
	"github.com/lsariol/lys/internal/identity"
	"github.com/lsariol/lys/internal/router"
	"github.com/lsariol/lys/internal/scheduler"
	"github.com/lsariol/lys/internal/server"
	"github.com/lsariol/lys/internal/sources"
	"github.com/lsariol/lys/internal/sources/inlet"
	letterboxdfeeds "github.com/lsariol/lys/internal/sources/letterboxd"
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
	log.Printf("Lys %s", version())

	cove, err := newCove()
	if err != nil {
		log.Fatalf("✗ %v", err)
	}
	cfg := config.New()
	if err := cfg.Initialize(cove); err != nil {
		log.Fatalf("✗ initialize config: %v", err)
	}

	if migratorURL := cfg.Database.MigratorConnectionString; migratorURL != "" {
		if err := database.Migrate(ctx, migratorURL); err != nil {
			log.Fatalf("✗ %v", err)
		}
	} else {
		log.Println("! LYS_DATABASE_MIGRATOR_URL isn't in Cove: skipping migrations")
	}

	db := database.NewDatabase(cfg.Database)
	if err := db.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.CheckSchemaVersion(ctx); err != nil {
		log.Fatalf("✗ %v", err)
	}

	if err := run(ctx, cfg, cove, db, started); err != nil {
		log.Printf("✗ %v", err)
		db.Close()
		os.Exit(1)
	}
	log.Println("shutting down")
}

// run wires the router, its adapters and commands, the sources and the HTTP
// server, and blocks until ctx is cancelled or an adapter fails for good.
func run(ctx context.Context, cfg *config.Config, cove *coveclient.Client, db *database.Database, started time.Time) error {

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

	sleeperClient := sleeperapi.New(httpClient) // shared, so the player catalog is kept once
	registry := commands.NewRegistry()
	for _, c := range []commands.Command{
		catfacts.New(httpClient),
		chatgpt.New(cfg.Commands.ChatGPTKey),
		help.New(registry),
		letterboxd.New(letterboxd.NewStore(db.Pool), httpClient),
		oneuppuzzle.New(db.Pool),
		ping.New(httpClient),
		randomdog.New(httpClient),
		remind.New(jobs),
		request.New(request.NewStore(db.Pool)),
		settings.New(channels),
		sleeper.New(sleeperClient, db.Pool),
		autopost.New(jobs),
		uptime.New(started),
	} {
		if err := registry.Register(c); err != nil {
			return err
		}
	}

	r := router.New(registry, channels, identity.New(db.Pool), workers)
	chat := activity.New(autopost.MinMessages)
	r.Observe(chat)
	twitch := twitchbot.New(httpClient, cfg.Twitch, cove, conns, bots, conns)
	r.Add(twitch)

	notifications := inlet.New(os.Getenv("SHARED_LYS_NOTIFICATION_TOKEN"))

	port := os.Getenv("NOTIFICATION_PORT")
	if port == "" {
		return errors.New("NOTIFICATION_PORT is not set: set it to the port the HTTP server listens on (2400 in the compose file)")
	}
	routes := map[string]http.Handler{"POST /notifications": notifications}
	for pattern, h := range enroll.New(conns, os.Getenv("SHARED_LYS_ENROLLMENT_TOKEN"), twitch).Routes() {
		routes[pattern] = h
	}
	srv := server.New(":"+port, version(), r.Health, routes)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channel changes (from the CLI too) reach the settings cache and the adapters within seconds.
	go func() {
		err := conns.Watch(ctx, 5*time.Minute, func(platform string) {
			if err := channels.Load(ctx); err != nil {
				log.Printf("✗ reload channel settings: %v", err)
			}
			r.SyncChannels(ctx, platform)
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("✗ watch channel changes: %v", err)
		}
	}()

	go func() {
		if err := srv.Run(ctx); err != nil {
			log.Printf("✗ server: %v", err)
			cancel()
		}
	}()

	schedule := scheduler.New(jobs, map[string]scheduler.Handler{
		remind.Kind:   scheduler.SendMessage,
		autopost.Kind: autopost.Post(chat),
	})

	for _, s := range []sources.Source{notifications, schedule, letterboxdfeeds.New(db.Pool, httpClient), sleeperfeed.New(sleeperClient, db.Pool), sleeperfeed.NewLineups(sleeperClient, db.Pool)} {
		go func() {
			if err := s.Run(ctx, r); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("✗ source %q: %v", s.Name(), err)
			}
		}()
	}

	return r.Run(ctx)
}

// useBotAccount points the Twitch config at the default bot account in
// bot_accounts; without one, Lys can't start (lys bot authorize adds it).
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

// cliServices loads Cove, the config and the database for CLI commands, once
// and only when a command first needs them.
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

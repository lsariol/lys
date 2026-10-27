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
	twitchbot "github.com/lsariol/lys/internal/adapters/twitch/bot"
	"github.com/lsariol/lys/internal/botaccounts"
	"github.com/lsariol/lys/internal/channelconfig"
	"github.com/lsariol/lys/internal/cli"
	"github.com/lsariol/lys/internal/commands"
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
	"github.com/lsariol/lys/internal/commands/cmds/timer"
	"github.com/lsariol/lys/internal/commands/cmds/uptime"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/connections"
	"github.com/lsariol/lys/internal/database"
	"github.com/lsariol/lys/internal/identity"
	"github.com/lsariol/lys/internal/router"
	"github.com/lsariol/lys/internal/scheduler"
	"github.com/lsariol/lys/internal/server"
	"github.com/lsariol/lys/internal/sources"
	"github.com/lsariol/lys/internal/sources/inlet"
	letterboxdfeeds "github.com/lsariol/lys/internal/sources/letterboxd"
)

// workers is how many commands may run at once.
const workers = 8

func main() {
	args := os.Args[1:]
	started := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	_ = godotenv.Load(".env")

	if len(args) > 0 {
		os.Exit(cli.Run(ctx, cli.NewEnv(cliServices()), args))
	}

	cove, err := newCove()
	if err != nil {
		log.Fatalf("✗ %v", err)
	}
	cfg := config.New()
	if err := cfg.Initilize(cove); err != nil {
		log.Fatalf("✗ initilize config: %v", err)
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

	if err := db.LinkLegacyRows(ctx); err != nil {
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
		sleeper.New(httpClient, db.Pool),
		timer.New(jobs),
		uptime.New(started),
	} {
		if err := registry.Register(c); err != nil {
			return err
		}
	}

	r := router.New(registry, channels, identity.New(db.Pool), workers)
	r.Add(twitchbot.New(httpClient, cfg.Twitch, cove, conns, bots))

	notifications := inlet.New(os.Getenv("SHARED_LYS_NOTIFICATION_TOKEN"))

	port := os.Getenv("NOTIFICATION_PORT")
	if port == "" {
		return errors.New("NOTIFICATION_PORT is not set: set it to the port the HTTP server listens on (2400 in the compose file)")
	}
	srv := server.New(":"+port, r.Health, map[string]http.Handler{"POST /notifications": notifications})

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		if err := srv.Run(ctx); err != nil {
			log.Printf("✗ server: %v", err)
			cancel()
		}
	}()

	schedule := scheduler.New(jobs, map[string]scheduler.Handler{
		remind.Kind: scheduler.SendMessage,
		timer.Kind:  scheduler.SendMessage,
	})

	for _, s := range []sources.Source{notifications, schedule, letterboxdfeeds.New(db.Pool, httpClient), sleeper.NewPoller(httpClient, db.Pool)} {
		go func() {
			if err := s.Run(ctx, r); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("✗ source %q: %v", s.Name(), err)
			}
		}()
	}

	return r.Run(ctx)
}

// useBotAccount points the Twitch config at the default bot account in
// bot_accounts. Without one, LYS_TWITCH_BOT_ID from Cove is used, and the
// account is recorded once Twitch confirms its token.
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
			if err = cfg.Initilize(cove); err != nil {
				err = fmt.Errorf("initilize config: %w", err)
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

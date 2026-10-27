package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/lsariol/coveclient"
	twitchbot "github.com/lsariol/lys/internal/adapters/twitch/bot"
	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
	"github.com/lsariol/lys/internal/channelconfig"
	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/commands/cmds/catfacts"
	"github.com/lsariol/lys/internal/commands/cmds/chatgpt"
	"github.com/lsariol/lys/internal/commands/cmds/help"
	"github.com/lsariol/lys/internal/commands/cmds/letterboxd"
	"github.com/lsariol/lys/internal/commands/cmds/oneuppuzzle"
	"github.com/lsariol/lys/internal/commands/cmds/ping"
	"github.com/lsariol/lys/internal/commands/cmds/randomdog"
	"github.com/lsariol/lys/internal/commands/cmds/request"
	"github.com/lsariol/lys/internal/commands/cmds/settings"
	"github.com/lsariol/lys/internal/commands/cmds/sleeper"
	"github.com/lsariol/lys/internal/commands/cmds/uptime"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/database"
	"github.com/lsariol/lys/internal/router"
	"github.com/lsariol/lys/internal/server"
	"github.com/lsariol/lys/internal/sources"
	"github.com/lsariol/lys/internal/sources/inlet"
)

// workers is how many commands may run at once.
const workers = 8

func main() {
	args := os.Args[1:]
	started := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	_ = godotenv.Load(".env")

	cove := coveclient.New(os.Getenv("COVE_URL"), os.Getenv("COVE_TOKEN"))
	cfg := config.New()
	if err := cfg.Initilize(cove); err != nil {
		log.Fatalf("✗ initilize config: %v", err)
	}

	migratorURL := cfg.Database.MigratorConnectionString

	if len(args) > 0 {
		if args[0] != "migrate" {
			fmt.Fprintf(os.Stderr, "! Unknown command %q.\n%s\n", args[0], migrateUsage)
			os.Exit(1)
		}
		os.Exit(runMigrate(ctx, migratorURL, args[1:]))
	}

	if migratorURL != "" {
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

	twitchStore := twitchdb.NewStore(db.Pool, cfg.Database.ConnectionString)
	channels := channelconfig.New(twitchStore)
	if err := channels.Load(ctx); err != nil {
		return err
	}

	registry := commands.NewRegistry()
	for _, c := range []commands.Command{
		catfacts.New(httpClient),
		chatgpt.New(cfg.Commands.ChatGPTKey),
		help.New(registry),
		letterboxd.New(letterboxd.NewStore(db.Pool), httpClient),
		oneuppuzzle.New(db.Pool),
		ping.New(httpClient),
		randomdog.New(httpClient),
		request.New(request.NewStore(db.Pool)),
		settings.New(channels),
		sleeper.New(httpClient, db.Pool),
		uptime.New(started),
	} {
		if err := registry.Register(c); err != nil {
			return err
		}
	}

	r := router.New(registry, channels, workers)
	r.Add(twitchbot.New(httpClient, cfg.Twitch, cove, twitchStore))

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

	for _, s := range []sources.Source{notifications} {
		go func() {
			if err := s.Run(ctx, r); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("✗ source %q: %v", s.Name(), err)
			}
		}()
	}

	return r.Run(ctx)
}

const migrateUsage = "Usage: lys migrate [status|up]"

// runMigrate handles `lys migrate [status|up]` with the migrator role and returns the exit code.
func runMigrate(ctx context.Context, migratorURL string, args []string) int {
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "! "+migrateUsage)
		return 1
	}
	if migratorURL == "" {
		fmt.Fprintln(os.Stderr, "✗ lys migrate: LYS_DATABASE_MIGRATOR_URL isn't in Cove. Add the lys_migrator connection string there (cove shell: create LYS_DATABASE_MIGRATOR_URL).")
		return 1
	}

	command := "status"
	if len(args) == 1 {
		command = args[0]
	}

	var err error
	switch command {
	case "status":
		err = database.PrintMigrationStatus(ctx, migratorURL, os.Stdout)
	case "up":
		err = database.Migrate(ctx, migratorURL)
	default:
		fmt.Fprintf(os.Stderr, "! lys migrate: unknown command %q.\n%s\n", command, migrateUsage)
		return 1
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ lys migrate: %v\n", err)
		return 1
	}
	return 0
}

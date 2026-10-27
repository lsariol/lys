package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	twitchbot "github.com/lsariol/lys/internal/adapters/twitch/bot"
	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
	"github.com/lsariol/lys/internal/app/dependencies"
	"github.com/lsariol/lys/internal/app/registry"
	"github.com/lsariol/lys/internal/app/router"
	"github.com/lsariol/lys/internal/database"
	"github.com/lsariol/lys/internal/feed"
	notificationsource "github.com/lsariol/lys/internal/feed/sources/notification"
	notificationserver "github.com/lsariol/lys/internal/notification"
	"github.com/lsariol/lys/internal/runtime/settings"
)

func main() {
	args := os.Args[1:]

	// Create Dependencies
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps := dependencies.New(ctx)
	err := deps.Initilize()
	if err != nil {
		log.Fatalf("Error loading deps: %v", err)
	}

	migratorURL := deps.Config.Database.MigratorConnectionString

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

	// Create Database Connection
	if err := deps.DB.Connect(ctx); err != nil {
		log.Fatal(err)
	}

	if err := deps.DB.CheckSchemaVersion(ctx); err != nil {
		log.Fatalf("✗ %v", err)
	}

	// Create Registry
	var register *registry.Registry = registry.NewRegistry()
	register.RegisterAll()

	// Create Feed
	feed := feed.NewFeed()
	notifSource := notificationsource.New()

	// Create Router
	var router *router.Router = router.NewRouter(ctx, register, feed)
	feed.SetChannel(router.InboundEvents())
	feed.AddSource(notifSource)

	// Create DBStores
	var twitchDBStore *twitchdb.Store = twitchdb.NewStore(deps.DB.Pool, deps.DB.Config.ConnectionString)

	// Create SettingsStore
	var settingsStore *settings.Store = settings.NewSettings(twitchDBStore)

	deps.Settings = settingsStore

	//Create Adapter clients
	var twitchClient *twitchbot.TwitchClient = twitchbot.New(deps.HTTP, deps.Config.Twitch, deps.Cove, deps.Settings, twitchDBStore, router.InboundCommands(), register.GetReadRegistry())

	// Register Adapters with the router
	router.RegisterAdapter(twitchClient)
	//router.RegisterAdapter(discordClient)

	notifPort := os.Getenv("NOTIFICATION_PORT")
	if notifPort == "" {
		log.Fatal("NOTIFICATION_PORT is not set")
	}
	notifServer := notificationserver.NewServer(":"+notifPort, notifSource)
	notifServer.Start()
	defer notifServer.Shutdown(ctx)

	// Start Router
	go router.Run(ctx, deps)

	// Start Feed
	go feed.Run(ctx, deps)

	//Start Broker

	// Start TwitchBot Reading
	go func() {
		err := twitchClient.Run(ctx)
		if err != nil {
			log.Println(err)
			stop()
		}
	}()

	// time.Sleep(3 * time.Second)
	// RunLiveTests(ctx, twitchClient)

	<-ctx.Done()
	log.Println("shutting down")

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

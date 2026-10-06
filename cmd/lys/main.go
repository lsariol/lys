package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	twitchbot "github.com/lsariol/lys/internal/adapters/twitch/bot"
	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
	"github.com/lsariol/lys/internal/app/dependencies"
	"github.com/lsariol/lys/internal/app/registry"
	"github.com/lsariol/lys/internal/app/router"
	"github.com/lsariol/lys/internal/feed"
	notificationsource "github.com/lsariol/lys/internal/feed/sources/notification"
	notificationserver "github.com/lsariol/lys/internal/notification"
	"github.com/lsariol/lys/internal/runtime/settings"
)

func main() {

	// Create Dependencies
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps := dependencies.New(ctx)
	err := deps.Initilize()
	if err != nil {
		log.Fatalf("Error loading deps: %v", err)
	}

	// Create Database Connection
	if err := deps.DB.Connect(ctx); err != nil {
		log.Fatal(err)
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

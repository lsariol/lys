package bot

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/lsariol/coveclient"
	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/adapters/twitch/auth"
	"github.com/lsariol/lys/internal/adapters/twitch/chat"
	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
	"github.com/lsariol/lys/internal/adapters/twitch/eventsub"
	"github.com/lsariol/lys/internal/app/registry"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/runtime/settings"
)

var _ adapter.Adapter = (*TwitchClient)(nil)

type TwitchClient struct {
	Chat            *chat.ChatClient
	EventSub        *eventsub.EventSubClient
	Auth            *auth.AuthClient
	Config          *config.TwitchConfig
	Settings        *settings.Store
	DB              *twitchdb.Store
	HTTP            *http.Client
	CommandRegistry *registry.ReadRegister

	// systemEvents is owned by BotClient.
	// Subcomponents receive only write access to this channel to report internal changes
	// (e.g., when a user updates a setting or a component’s state changes).
	// BotClient listens on this channel to apply those updates—persisting them if needed—and
	// synchronizes the in-memory settings across all dependent subcomponents.
	systemEvents chan adapter.SystemEvent
	inEvents     <-chan eventsub.EventSubMessage
	outEnvelopes chan<- adapter.Envelope
	inEnvelopes  chan<- adapter.Response
}

func New(http *http.Client, cfg *config.TwitchConfig, cove *coveclient.Client, settingsStore *settings.Store, dbStore *twitchdb.Store, routerSink chan<- adapter.Envelope, rReg *registry.ReadRegister) *TwitchClient {

	auth := auth.New(dbStore, cove, cfg, http)
	es := eventsub.New(http, cfg, auth, dbStore)
	c := chat.New(http, cfg, auth, dbStore)

	return &TwitchClient{
		Chat:            c,             //Chat Client
		EventSub:        es,            //EventSub Client
		Auth:            auth,          //Auth Client
		Config:          cfg,           //Config - Values that never change
		Settings:        settingsStore, //SettingsStore - Values that are tied to the database, which need occasional updating
		DB:              dbStore,       //Connection to twitch Database
		HTTP:            http,          //HTTP Client
		CommandRegistry: rReg,          //Read Register for commands
		inEvents:        es.OutboundEvents(),
		inEnvelopes:     c.InboundResponses(),
		outEnvelopes:    routerSink,
	}
}

func (c *TwitchClient) InboundEvents() <-chan eventsub.EventSubMessage { return c.inEvents }

func (c *TwitchClient) Run(ctx context.Context) error {

	if err := c.Initilize(ctx); err != nil {
		return fmt.Errorf("initilization error: %w", err)
	}

	// Add in supervising eventually
	//go c.supervise(ctx, "chat", c.Chat.Run)

	go c.Chat.Run(ctx)

	go c.EventSub.Run(ctx)

	go c.ingestLoop(ctx, c.InboundEvents())

	<-ctx.Done()
	c.Shutdown(ctx)
	return ctx.Err()

}

// Stores any persistant data living in memory, and closes all connections
func (c *TwitchClient) Shutdown(ctx context.Context) {

	if err := c.Auth.Shutdown(ctx); err != nil {
		log.Println("Error in Auth shutdown: %w", err)
	}
	if err := c.Chat.Shutdown(); err != nil {
		log.Println("Error in Chat shutdown: %w", err)
	}
	if err := c.EventSub.Shutdown(ctx); err != nil {
		log.Println("Error in EventSub shutdown: %w", err)
	}
	if err := c.DB.Shutdown(); err != nil {
		log.Println("Error in Auth shutdown: %w", err)
	}

	close(c.systemEvents)

}

func (c *TwitchClient) Health(ctx context.Context) adapter.HealthStatus {
	status := adapter.HealthStatus{}

	return status
}

func (c *TwitchClient) Name() string {
	return "twitch"
}

func (c *TwitchClient) DeliverResponse(r adapter.Response) {
	c.Chat.InboundResponses() <- r
}

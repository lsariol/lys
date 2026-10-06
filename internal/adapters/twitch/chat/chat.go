package chat

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/adapters/twitch/auth"
	twitchdb "github.com/lsariol/lys/internal/adapters/twitch/database"
	"github.com/lsariol/lys/internal/config"
)

// CLI URLS
// const (
// 	HelixBaseURL       = "https://api.twitch.tv/helix"
// 	EventSubAPIBaseURL = "http://127.0.0.1:8080/eventsub/subscriptions"
// 	EventSubWSURL      = "ws://127.0.0.1:8080/ws"
// )

const (
	HelixBaseURL       = "https://api.twitch.tv/helix"
	EventSubAPIBaseURL = "https://api.twitch.tv/helix/eventsub/subscriptions"
	EventSubWSURL      = "wss://eventsub.wss.twitch.tv/ws"
)

type ChatClient struct {
	HTTP   *http.Client
	Auth   *auth.AuthClient
	Config *config.TwitchConfig
	DB     *twitchdb.Store
	in     chan adapter.Response
}

func New(http *http.Client, cfg *config.TwitchConfig, auth *auth.AuthClient, db *twitchdb.Store) *ChatClient {

	return &ChatClient{
		Auth:   auth,
		HTTP:   http,
		Config: cfg,
		DB:     db,
		in:     make(chan adapter.Response, 100),
	}
}

func (c *ChatClient) InboundResponses() chan<- adapter.Response { return c.in }

func (c *ChatClient) Initilize() error {

	return nil
}

func (c *ChatClient) Run(ctx context.Context) error {

	fmt.Println("[Chat] Running")

	for {
		select {
		case <-ctx.Done():
			c.Shutdown()
			return ctx.Err()

		case response, ok := <-c.in:
			if !ok {
				return nil
			}

			c.DeliverResponse(ctx, response)
		}
	}
}

func (c *ChatClient) Shutdown() error {

	return nil
}

// SendChatMessageResponse
// HelixError

func (c *ChatClient) DeliverResponse(ctx context.Context, r adapter.Response) error {

	//Replace this with a sophisticated rate limiter

	msgResponse, helixErr, err := c.SendChat(ctx, r)
	if err != nil {
		return err
	}

	//TODO: log these errors into a database
	//Log to twitch errors db for now,
	// Message didnt send
	if helixErr != nil {

		fmt.Println(msgResponse)

		switch helixErr.Status {

		// Bad/missing params, also if you set for_soruce_only with a user token
		case 400:
			log.Println("Bad/missing params, also if you set for_soruce_only with a user token")
			log.Printf("400: %s\n", helixErr.Message)

		// Bad/expired token, missing user:write:chat scope, client-ID mismatch
		case 401:
			log.Println("Bad/expired token, missing user:write:chat scope, client-ID mismatch")
			log.Printf("401: %s\n", helixErr.Message)

			// Refresh App Access Token
			if err := c.Auth.RefreshAppAccessToken(ctx); err != nil {
				return fmt.Errorf("refresh app access token: %w", err)
			}
			// Try again
			log.Println("Failed to send message. App Access token has been refreshed, attempting again.")

			time.Sleep(time.Second * 1)
			return c.DeliverResponse(ctx, r)

		// Not permitted to send that (banned)
		case 403:
			log.Println("Bad/expired token, missing user:write:chat scope, client-ID mismatch")
			log.Printf("403: %s\n", helixErr.Message)

		// Message too large
		case 422:
			log.Println("Message too large")
			log.Printf("422: %s\n", helixErr.Message)
		}
	}

	time.Sleep(time.Second * 1)
	return nil
}

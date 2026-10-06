package router

import (
	"context"
	"log"
	"time"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
	"github.com/lsariol/lys/internal/app/middleware"
	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/feed"
)

func (r *Router) DispatchCommand(parentCtx context.Context, envelope adapter.Envelope, cmd commands.Command, deps *dependencies.Deps) {

	ctx := parentCtx

	if t := cmd.Timeout(); t > 0 {

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parentCtx, t)
		defer cancel()
	}

	final := chain(cmd.Execute, middleware.Recovery, middleware.Logging)

	response, err := final(ctx, envelope, deps)

	response.Platform = envelope.Platform
	response.ChannelID = envelope.ChannelID
	response.ChannelName = envelope.ChannelName
	response.Platform = envelope.Platform
	response.Username = envelope.Username
	response.UserID = envelope.UserID
	response.ChannelName = envelope.ChannelName
	response.ChannelID = envelope.ChannelID
	response.TimeStart = envelope.Timestamp
	response.TimeFinished = time.Now()

	if response.SuppressReply {
		return
	}

	if err != nil {
		response.Error = true
		r.adapterRegistry[response.Platform].DeliverResponse(response)
		return
	}

	response.Success = true
	r.adapterRegistry[response.Platform].DeliverResponse(response)

}

// DispatchEvent fans a feed.Event out to every subscriber's platform adapter.
// Add a new case here when a new EventType is introduced.
func (r *Router) DispatchEvent(ctx context.Context, event feed.Event) {
	switch event.Type {

	case feed.EventNotification:
		payload, ok := event.Payload.(feed.NotificationPayload)
		if !ok {
			log.Printf("[Router] EventNotification has unexpected payload type %T", event.Payload)
			return
		}

		for _, sub := range event.Subscribers {
			a, ok := r.adapterRegistry[sub.Platform]
			if !ok {
				log.Printf("[Router] no adapter registered for platform %q — skipping subscriber (channelID=%s)", sub.Platform, sub.ChannelID)
				continue
			}
			a.DeliverResponse(adapter.Response{
				Platform:    sub.Platform,
				ChannelID:   sub.ChannelID,
				ChannelName: sub.ChannelName,
				Text:        payload.Message,
				Success:     true,
			})
		}

	default:
		log.Printf("[Router] unhandled event type %q", event.Type)
	}
}

func chain(next middleware.Handler, mws ...middleware.Middleware) middleware.Handler {
	wrapped := next
	for i := len(mws) - 1; i >= 0; i-- {
		wrapped = mws[i](wrapped)
	}
	return wrapped
}

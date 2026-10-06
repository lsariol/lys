package router

import (
	"context"

	"github.com/lsariol/lys/internal/adapters/adapter"
	"github.com/lsariol/lys/internal/app/dependencies"
	"github.com/lsariol/lys/internal/app/registry"
	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/feed"
)

type Router struct {
	inboundCommands chan adapter.Envelope
	inboundEvents   chan feed.Event
	adapterRegistry map[string]adapter.Adapter
	registry        *registry.Registry
	feed            *feed.Feed
	rootCtx         context.Context
}

func NewRouter(ctx context.Context, reg *registry.Registry, evtFeed *feed.Feed) *Router {

	return &Router{
		inboundCommands: make(chan adapter.Envelope, 100),
		inboundEvents:   make(chan feed.Event, 100),
		adapterRegistry: make(map[string]adapter.Adapter),
		registry:        reg,
		feed:            evtFeed,
		rootCtx:         ctx,
	}
}

func (r *Router) InboundCommands() chan<- adapter.Envelope {
	return r.inboundCommands
}

func (r *Router) InboundEvents() chan<- feed.Event {
	return r.inboundEvents
}

func (r *Router) Run(ctx context.Context, deps *dependencies.Deps) {

	for {
		select {
		case <-ctx.Done():
			return
		case env, ok := <-r.inboundCommands:
			if !ok {
				return
			}

			cmd, ok := r.registryLookUp(env)
			if !ok {
				continue
			}

			r.DispatchCommand(ctx, env, cmd, deps)

		case event, ok := <-r.inboundEvents:
			if !ok {
				return
			}

			r.DispatchEvent(ctx, event)
		}
	}
}

func (r *Router) registryLookUp(envelope adapter.Envelope) (commands.Command, bool) {

	cmd, ok := r.registry.Get(envelope.Command)

	return cmd, ok
}

func (r *Router) RegisterAdapter(a adapter.Adapter) error {

	r.adapterRegistry[a.Name()] = a
	return nil
}

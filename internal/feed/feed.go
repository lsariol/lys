package feed

import (
	"context"

	"github.com/lsariol/lys/internal/app/dependencies"
	"github.com/lsariol/lys/internal/feed/sources"
)

// Outbounder is an optional interface a Source may implement so Feed.AddSource
// can hand it the router's inboundEvents write channel automatically.
// NotificationSource implements this.
type Outbounder interface {
	SetOutbound(out chan<- Event)
}

type Feed struct {
	out     chan<- Event
	Sources []sources.Source
}

func NewFeed() *Feed {
	return &Feed{}
}

// SetChannel receives the write-end of the router's inboundEvents channel.
// Must be called before AddSource.
func (f *Feed) SetChannel(out chan<- Event) {
	f.out = out
}

// AddSource registers a source with the feed.
// If the source implements Outbounder, Feed wires the router channel to it
// automatically so the source can push events without a separate wiring call.
func (f *Feed) AddSource(s sources.Source) {
	if ob, ok := s.(Outbounder); ok {
		ob.SetOutbound(f.out)
	}
	f.Sources = append(f.Sources, s)
}

// Run starts each registered source in its own goroutine and blocks until
// ctx is cancelled. All sources must be added before Run is called.
func (f *Feed) Run(ctx context.Context, deps *dependencies.Deps) {
	for _, s := range f.Sources {
		go s.PullFeed(ctx)
	}
	<-ctx.Done()
}

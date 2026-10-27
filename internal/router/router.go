// Package router turns chat messages into commands and sends their replies,
// and the messages sources publish, out through the right adapter.
package router

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/lsariol/lys/internal/commands"
	"github.com/lsariol/lys/internal/core"
)

// Adapter connects one chat platform: it forwards every message it receives
// and sends what it's given.
type Adapter interface {
	Platform() string
	// Run receives messages into in until ctx is cancelled or the connection fails for good.
	Run(ctx context.Context, in chan<- core.Message) error
	// Send queues a message; it must not block on the platform.
	Send(ctx context.Context, out core.Outbound) error
	// Healthy reports whether the adapter is connected right now.
	Healthy() bool
}

// Queue sizes: past these, new messages are dropped (and logged) rather than
// blocking the platform or the commands.
const (
	inboxSize  = 256
	outboxSize = 256
)

// panicReply is sent when a command panics.
const panicReply = "Something went wrong running that command."

// Router runs commands for incoming messages and delivers outgoing ones.
type Router struct {
	registry   *commands.Registry
	config     core.ChannelConfig
	identities core.Identities
	workers    int

	in       chan core.Message
	adapters map[string]Adapter
	outboxes map[string]chan core.Outbound
}

// New returns a router that runs up to workers commands at once.
func New(registry *commands.Registry, config core.ChannelConfig, identities core.Identities, workers int) *Router {
	return &Router{
		registry:   registry,
		config:     config,
		identities: identities,
		workers:    max(workers, 1),
		in:         make(chan core.Message, inboxSize),
		adapters:   make(map[string]Adapter),
		outboxes:   make(map[string]chan core.Outbound),
	}
}

// Add registers an adapter; call it before Run.
func (r *Router) Add(a Adapter) {
	r.adapters[a.Platform()] = a
	r.outboxes[a.Platform()] = make(chan core.Outbound, outboxSize)
}

// Run starts the adapters, the workers and the outboxes, and blocks until ctx
// is cancelled or an adapter stops with an error.
func (r *Router) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, len(r.adapters))

	for platform, a := range r.adapters {
		wg.Add(2)
		go func() {
			defer wg.Done()
			r.deliver(ctx, a, r.outboxes[platform])
		}()
		go func() {
			defer wg.Done()
			if err := a.Run(ctx, r.in); err != nil && !errors.Is(err, context.Canceled) {
				errs <- fmt.Errorf("%s: %w", platform, err)
			}
		}()
	}

	for range r.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.work(ctx)
		}()
	}

	var err error
	select {
	case <-ctx.Done():
	case err = <-errs:
	}
	cancel()
	wg.Wait()
	return err
}

// Publish queues a message for its platform's adapter. Sources use it.
func (r *Router) Publish(ctx context.Context, out core.Outbound) error {
	if strings.TrimSpace(out.Text) == "" {
		return nil
	}
	box, ok := r.outboxes[out.Platform]
	if !ok {
		return fmt.Errorf("router: no adapter for platform %q", out.Platform)
	}
	select {
	case box <- out:
		return nil
	default:
		return fmt.Errorf("router: the %s outbox is full; dropped a message for %q", out.Platform, out.Channel.Name)
	}
}

// Health reports each adapter's connection, by platform.
func (r *Router) Health() map[string]bool {
	h := make(map[string]bool, len(r.adapters))
	for p, a := range r.adapters {
		h[p] = a.Healthy()
	}
	return h
}

func (r *Router) work(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-r.in:
			r.handle(ctx, m)
		}
	}
}

func (r *Router) deliver(ctx context.Context, a Adapter, box <-chan core.Outbound) {
	for {
		select {
		case <-ctx.Done():
			return
		case out := <-box:
			if err := a.Send(ctx, out); err != nil {
				log.Printf("[Router] send to %s %q: %v", out.Platform, out.Channel.Name, err)
			}
		}
	}
}

// handle runs the command a message calls, if any, and queues its reply.
func (r *Router) handle(ctx context.Context, m core.Message) {
	cmd, req, ok := r.match(m)
	if !ok {
		return
	}

	id, err := r.identities.Resolve(ctx, m.Platform, m.User)
	if err != nil {
		log.Printf("[Router] %v; running %q without an identity", err, req.Command)
	}
	req.Identity = id

	reply := r.run(ctx, cmd, req)
	if err := r.Publish(ctx, core.Outbound{Platform: m.Platform, Channel: m.Channel, Text: reply.Text}); err != nil {
		log.Printf("[Router] %v", err)
	}
}

// match finds the command a message calls: by prefix and name first; a
// message that starts with the prefix but names no command calls nothing.
// Otherwise the first matching pattern.
func (r *Router) match(m core.Message) (commands.Command, commands.Request, bool) {
	prefix := r.config.Prefix(m.Platform, m.Channel.ID)
	conn, _ := r.config.Connection(m.Platform, m.Channel.ID)
	req := commands.Request{Message: m, Prefix: prefix, Connection: conn}

	text := strings.TrimSpace(m.Text)
	if strings.HasPrefix(text, prefix) {
		fields := strings.Fields(text[len(prefix):])
		if len(fields) == 0 {
			return nil, req, false
		}
		cmd, ok := r.registry.Lookup(fields[0])
		if !ok {
			return nil, req, false
		}
		req.Command = cmd.Spec().Name
		req.Args = fields[1:]
		return cmd, req, true
	}

	cmd, args, ok := r.registry.Match(m.Text)
	if !ok {
		return nil, req, false
	}
	req.Command = cmd.Spec().Name
	req.Args = args
	req.Pattern = true
	return cmd, req, true
}

// run calls the command with its timeout, logs any error, and turns a panic
// into an error reply.
func (r *Router) run(ctx context.Context, cmd commands.Command, req commands.Request) (reply commands.Reply) {
	timeout := cmd.Spec().Timeout
	if timeout <= 0 {
		timeout = commands.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	defer func() {
		if p := recover(); p != nil {
			log.Printf("[Router] command %q in %q panicked: %v\n%s", req.Command, req.Channel.Name, p, debug.Stack())
			reply = commands.Reply{Text: panicReply}
		}
	}()

	start := time.Now()
	reply, err := cmd.Run(ctx, req)
	if err != nil {
		log.Printf("[Router] command %q in %q after %s: %v", req.Command, req.Channel.Name, time.Since(start).Round(time.Millisecond), err)
	}
	return reply
}

// Package router turns chat messages into commands and sends their replies,
// and the messages sources publish, out through the right adapter.
package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// Syncer is an adapter that can join and leave channels while it runs. An
// adapter without it picks up channel changes when it next starts.
type Syncer interface {
	SyncChannels(ctx context.Context) error
}

// Observer sees every incoming message, command or not, before it's handled.
// It runs as each message arrives, before any command, so it must be quick.
type Observer interface {
	Observe(m core.Message)
}

// Queue sizes: past these, new messages are dropped (and logged) rather than
// blocking the platform or the commands.
const (
	inboxSize  = 256
	jobsSize   = 256
	outboxSize = 256
)

// panicReply is sent when a command panics.
const panicReply = "Something went wrong running that command."

// resolveTimeout limits looking up who sent a command, which happens before
// the command's own time limit starts.
const resolveTimeout = 5 * time.Second

// busyReply is sent when a command is at its Concurrency, or every worker is busy.
const busyReply = "Busy, try again in a moment."

// Router runs commands for incoming messages and delivers outgoing ones.
type Router struct {
	registry   *commands.Registry
	config     core.ChannelConfig
	identities core.Identities
	workers    int
	resolveIn  time.Duration // resolveTimeout; shorter in tests
	log        *slog.Logger

	in        chan core.Message
	jobs      chan job
	limits    *limits
	adapters  map[string]Adapter
	outboxes  map[string]chan core.Outbound
	observers []Observer
}

// job is an admitted command, waiting for a worker.
type job struct {
	cmd commands.Command
	req commands.Request
}

// New returns a router that runs up to workers commands at once.
func New(registry *commands.Registry, config core.ChannelConfig, identities core.Identities, workers int, log *slog.Logger) *Router {
	return &Router{
		registry:   registry,
		config:     config,
		identities: identities,
		workers:    max(workers, 1),
		resolveIn:  resolveTimeout,
		log:        log.With("component", "router"),
		in:         make(chan core.Message, inboxSize),
		jobs:       make(chan job, jobsSize),
		limits:     newLimits(time.Now),
		adapters:   make(map[string]Adapter),
		outboxes:   make(map[string]chan core.Outbound),
	}
}

// Add registers an adapter; call it before Run.
func (r *Router) Add(a Adapter) {
	r.adapters[a.Platform()] = a
	r.outboxes[a.Platform()] = make(chan core.Outbound, outboxSize)
}

// Observe adds an observer of every incoming message; call it before Run.
func (r *Router) Observe(o Observer) {
	r.observers = append(r.observers, o)
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

	wg.Add(1)
	go func() {
		defer wg.Done()
		r.dispatch(ctx)
	}()
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

// SyncChannels tells the platform's adapter (every adapter, for "") that its
// channel list changed.
func (r *Router) SyncChannels(ctx context.Context, platform string) {
	for p, a := range r.adapters {
		if platform != "" && platform != p {
			continue
		}
		if s, ok := a.(Syncer); ok {
			if err := s.SyncChannels(ctx); err != nil {
				r.log.Warn("sync channels failed", "platform", p, "err", err)
			}
		}
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

// dispatch takes each message as it arrives: the observers see it, and a
// command it calls is checked and queued for a worker. Nothing here waits on a
// command, so a slow one never holds up chat.
func (r *Router) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-r.in:
			r.admit(ctx, m)
		}
	}
}

func (r *Router) work(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-r.jobs:
			r.execute(ctx, j)
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
				r.log.Warn("send failed", "platform", out.Platform, "channel", out.Channel.Name, "err", err)
			}
		}
	}
}

// admit shows a message to the observers and, if it calls a command the
// sender may use now, queues it for a worker.
func (r *Router) admit(ctx context.Context, m core.Message) {
	for _, o := range r.observers {
		o.Observe(m)
	}
	cmd, req, ok := r.match(m)
	if !ok {
		return
	}
	spec := cmd.Spec()

	access := spec.Access
	what := req.Prefix + req.Command
	if !req.Pattern {
		access = spec.SubAccess(req.Args)
		if sub := subcommand(spec, req); sub != "" {
			what += " " + sub
		}
	}
	if !req.Allowed(access) {
		var reply commands.Reply
		if !req.Pattern {
			reply = commands.Refuse(access, "use "+what)
		}
		r.record(spec, req, reply, result{outcome: outcomeRefused}, notRun)
		r.reply(ctx, m, reply)
		return
	}

	switch r.limits.take(spec, req) {
	case cooling:
		r.record(spec, req, commands.Reply{}, result{outcome: outcomeCooling}, notRun) // quietly, as chat expects of a command used too often
		return
	case busy:
		reply := commands.Reply{Text: busyReply}
		r.record(spec, req, reply, result{outcome: outcomeBusy, limit: spec.Concurrency}, notRun)
		r.reply(ctx, m, reply)
		return
	}
	select {
	case r.jobs <- job{cmd: cmd, req: req}:
	default:
		r.limits.release(spec)
		reply := commands.Reply{Text: busyReply}
		r.record(spec, req, reply, result{outcome: outcomeBusy, workers: r.workers}, notRun)
		r.reply(ctx, m, reply)
	}
}

// execute runs an admitted command, records it and queues its reply.
func (r *Router) execute(ctx context.Context, j job) {
	defer r.limits.release(j.cmd.Spec())
	m := j.req.Message
	rctx, cancel := context.WithTimeout(ctx, r.resolveIn)
	id, err := r.identities.Resolve(rctx, m.Platform, m.User)
	cancel()
	if err != nil {
		r.log.Warn("sender lookup failed; running the command without an identity", "command", j.req.Command, "err", err)
	}
	j.req.Identity = id

	start := time.Now()
	reply, res := r.run(ctx, j.cmd, j.req)
	r.record(j.cmd.Spec(), j.req, reply, res, time.Since(start))
	r.reply(ctx, m, reply)
}

// reply answers a message in its channel, Text then More; an empty reply
// sends nothing.
func (r *Router) reply(ctx context.Context, m core.Message, reply commands.Reply) {
	for _, text := range append([]string{reply.Text}, reply.More...) {
		if err := r.Publish(ctx, core.Outbound{Platform: m.Platform, Channel: m.Channel, Text: text, Priority: core.PriorityReply}); err != nil {
			r.log.Warn("reply dropped", "channel", m.Channel.Name, "err", err)
		}
	}
}

// Outcomes of a command use, as its record gives them.
const (
	outcomeOK      = "ok"
	outcomeError   = "error"
	outcomeTimeout = "timeout"
	outcomeRefused = "refused" // the sender may not use it
	outcomeCooling = "cooling" // within a cooldown
	outcomeBusy    = "busy"    // at its cap, or every worker busy
)

// notRun is the run time of a use that never ran.
const notRun time.Duration = -1

// result is how a command use went.
type result struct {
	outcome string
	err     error
	stack   string // set after a panic
	limit   int    // the cap a busy command was at
	workers int    // set when every worker was busy
}

// record logs one use of a command, without its arguments: they're chat
// text. It runs just before the reply is queued.
func (r *Router) record(spec commands.Spec, req commands.Request, reply commands.Reply, res result, ran time.Duration) {
	level := slog.LevelInfo
	switch {
	case res.stack != "":
		level = slog.LevelError
	case res.outcome == outcomeError, res.outcome == outcomeTimeout, res.outcome == outcomeBusy:
		level = slog.LevelWarn
	}
	attrs := []slog.Attr{slog.String("command", req.Command)}
	if sub := subcommand(spec, req); sub != "" {
		attrs = append(attrs, slog.String("subcommand", sub))
	}
	attrs = append(attrs, slog.String("channel", req.Channel.Name), slog.String("outcome", res.outcome))
	if ran != notRun {
		attrs = append(attrs, slog.Int64("duration_ms", ran.Milliseconds()))
	}
	if !req.Time.IsZero() && (strings.TrimSpace(reply.Text) != "" || len(reply.More) > 0) {
		attrs = append(attrs, slog.Int64("reply_ms", time.Since(req.Time).Milliseconds()))
	}
	if res.limit > 0 {
		attrs = append(attrs, slog.Int("limit", res.limit))
	}
	if res.workers > 0 {
		attrs = append(attrs, slog.Int("workers", res.workers))
	}
	if res.err != nil {
		attrs = append(attrs, slog.String("err", res.err.Error()))
	}
	if res.stack != "" {
		attrs = append(attrs, slog.String("stack", res.stack))
	}
	r.log.LogAttrs(context.Background(), level, "command", attrs...)
}

// subcommand is the request's first argument when the command lists it as a
// subcommand, lower-cased; else "".
func subcommand(spec commands.Spec, req commands.Request) string {
	if req.Pattern {
		return ""
	}
	sub := strings.ToLower(first(req.Args))
	if _, ok := spec.Sub[sub]; !ok {
		return ""
	}
	return sub
}

func first(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// match finds the command a message calls: by prefix and name first; a
// message that starts with the prefix but names no command calls nothing.
// Otherwise the first matching pattern.
func (r *Router) match(m core.Message) (commands.Command, commands.Request, bool) {
	prefix := r.config.Prefix(m.Platform, m.Channel.ID)
	conn, _ := r.config.Connection(m.Platform, m.Channel.ID)
	req := commands.Request{Message: m, Prefix: prefix, Connection: conn}

	// Chat clients add U+E0000 to repeat a message past Twitch's duplicate filter.
	text := strings.TrimSpace(strings.ReplaceAll(m.Text, "\U000E0000", ""))
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

// run calls the command with its timeout, and turns a panic into an error
// reply.
func (r *Router) run(ctx context.Context, cmd commands.Command, req commands.Request) (reply commands.Reply, res result) {
	timeout := cmd.Spec().Timeout
	if timeout <= 0 {
		timeout = commands.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	defer func() {
		if p := recover(); p != nil {
			reply = commands.Reply{Text: panicReply}
			res = result{outcome: outcomeError, err: fmt.Errorf("panic: %v", p), stack: string(debug.Stack())}
		}
	}()

	reply, err := cmd.Run(ctx, req)
	switch {
	case err == nil:
		return reply, result{outcome: outcomeOK}
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return reply, result{outcome: outcomeTimeout, err: err}
	}
	return reply, result{outcome: outcomeError, err: err}
}

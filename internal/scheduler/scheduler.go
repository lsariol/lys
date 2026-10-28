package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/lsariol/lys/internal/backoff"
	"github.com/lsariol/lys/internal/core"
	"github.com/lsariol/lys/internal/sources"
)

// Handler runs one job of its kind; an error retries it with backoff.
type Handler func(ctx context.Context, job Job, pub sources.Publisher) error

// Scheduler is the source that runs due jobs.
type Scheduler struct {
	store    *Store
	handlers map[string]Handler
	every    time.Duration
}

var _ sources.Source = (*Scheduler)(nil)

const (
	batch       = 20               // jobs claimed per check
	lease       = 2 * time.Minute  // how long a claim lasts before another instance may take the job
	maxAttempts = 5                // then a one-off job is dropped, a repeating one skips to its next time
	retryBase   = 30 * time.Second // first retry delay
	retryMax    = 30 * time.Minute
)

// New returns a scheduler that checks for due jobs every few seconds.
func New(store *Store, handlers map[string]Handler) *Scheduler {
	return &Scheduler{store: store, handlers: handlers, every: 5 * time.Second}
}

func (s *Scheduler) Name() string { return "scheduler" }

func (s *Scheduler) Run(ctx context.Context, pub sources.Publisher) error {
	t := time.NewTicker(s.every)
	defer t.Stop()
	for {
		s.runDue(ctx, pub)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// runDue runs every due job, a batch at a time.
func (s *Scheduler) runDue(ctx context.Context, pub sources.Publisher) {
	for ctx.Err() == nil {
		jobs, err := s.store.Claim(ctx, batch, lease)
		if err != nil {
			log.Printf("[Scheduler] %v", err)
			return
		}
		for _, j := range jobs {
			s.run(ctx, j, pub)
		}
		if len(jobs) < batch {
			return
		}
	}
}

func (s *Scheduler) run(ctx context.Context, j Job, pub sources.Publisher) {
	failure := ""
	switch h, ok := s.handlers[j.Kind]; {
	case !ok:
		failure = fmt.Sprintf("no handler for kind %q", j.Kind)
	case !j.Active:
		// The channel is blocked or revoked: skip this run without retrying.
		failure = "the channel isn't active"
	default:
		if err := h(ctx, j, pub); err != nil {
			if j.Attempts+1 < maxAttempts {
				at := time.Now().Add(backoff.Delay(j.Attempts+1, retryBase, retryMax))
				log.Printf("[Scheduler] %s job %d: %v; retrying at %s", j.Kind, j.ID, err, at.Format(time.TimeOnly))
				if err := s.store.Retry(ctx, j.ID, err.Error(), at); err != nil {
					log.Printf("[Scheduler] %v", err)
				}
				return
			}
			failure = fmt.Sprintf("gave up after %d attempts: %v", maxAttempts, err)
		}
	}
	if failure != "" {
		log.Printf("[Scheduler] %s job %d in %q: %s", j.Kind, j.ID, j.Channel.Name, failure)
	}
	if err := s.store.Done(ctx, j.ID, failure); err != nil {
		log.Printf("[Scheduler] %v", err)
	}
}

// Message is the payload of a job that posts text to its channel.
type Message struct {
	Text string `json:"text"`
}

// SendMessage is the handler for jobs whose payload is a Message.
func SendMessage(ctx context.Context, j Job, pub sources.Publisher) error {
	return Send(ctx, j, pub, core.PriorityNormal)
}

// Send publishes the job's Message payload in its channel with a priority.
func Send(ctx context.Context, j Job, pub sources.Publisher, p core.Priority) error {
	var m Message
	if err := json.Unmarshal(j.Payload, &m); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	return pub.Publish(ctx, core.Outbound{Platform: j.Platform, Channel: j.Channel, Text: m.Text, Priority: p})
}

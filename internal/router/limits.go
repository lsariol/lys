package router

import (
	"sync"
	"time"

	"github.com/lsariol/lys/internal/commands"
)

// verdict is whether a command may run now.
type verdict int

const (
	allowed verdict = iota
	cooling         // within a cooldown
	busy            // at its Concurrency
)

// forgetAfter is how long a use is remembered once no cooldown can need it.
const forgetAfter = time.Hour

// limits applies the commands' cooldowns and concurrency caps. The channel's
// owner and moderators skip cooldowns, never the caps.
type limits struct {
	now func() time.Time

	mu      sync.Mutex
	used    map[useKey]time.Time // when a command was last used, per channel or person
	running map[string]int       // commands running now, by name
}

type useKey struct {
	command, platform, channel, user string // user is empty for the channel's cooldown
}

func newLimits(now func() time.Time) *limits {
	return &limits{now: now, used: make(map[useKey]time.Time), running: make(map[string]int)}
}

// take admits a use of the command, or says why not. An allowed use counts
// towards the cooldowns and, until release, the cap.
func (l *limits) take(spec commands.Spec, req commands.Request) verdict {
	if spec.Cooldown <= 0 && spec.UserCooldown <= 0 && spec.Concurrency <= 0 {
		return allowed
	}
	now := l.now()
	channel := useKey{command: spec.Name, platform: req.Platform, channel: req.Channel.ID}
	user := channel
	user.user = req.User.ID
	exempt := req.Allowed(commands.Moderator)

	l.mu.Lock()
	defer l.mu.Unlock()
	if !exempt {
		if last, ok := l.used[channel]; ok && spec.Cooldown > 0 && now.Sub(last) < spec.Cooldown {
			return cooling
		}
		if last, ok := l.used[user]; ok && spec.UserCooldown > 0 && now.Sub(last) < spec.UserCooldown {
			return cooling
		}
	}
	if spec.Concurrency > 0 && l.running[spec.Name] >= spec.Concurrency {
		return busy
	}

	if spec.Concurrency > 0 {
		l.running[spec.Name]++
	}
	if spec.Cooldown > 0 {
		l.used[channel] = now
	}
	if spec.UserCooldown > 0 {
		l.used[user] = now
	}
	if len(l.used) > 10_000 {
		for k, t := range l.used {
			if now.Sub(t) > forgetAfter {
				delete(l.used, k)
			}
		}
	}
	return allowed
}

// release ends a use that take allowed.
func (l *limits) release(spec commands.Spec) {
	if spec.Concurrency <= 0 {
		return
	}
	l.mu.Lock()
	if l.running[spec.Name] > 0 {
		l.running[spec.Name]--
	}
	l.mu.Unlock()
}

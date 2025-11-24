package middleware

import (
	"context"
	"io"
	"log"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	burstPerSecondThreshold = 32
	initialBanDuration      = 10 * time.Second
	slowStartThreshold      = 8
	slowDelayStep           = 50 * time.Millisecond
	maxSlowDelay            = 2 * time.Second
)

type BanRecord struct {
	IP       string
	Start    time.Time
	Duration time.Duration
	Reason   string
}

type clientState struct {
	windowStart time.Time
	count       int
	banUntil    time.Time
	banDuration time.Duration
}

type RateLimiter struct {
	mu           sync.Mutex
	clients      map[string]*clientState
	junkRequests chan junkRequest
	banHistory   []BanRecord
}

type junkRequest struct {
	ctx         context.Context
	body        io.ReadCloser
	bannedUntil time.Time
}

var globalRateLimiter = newRateLimiter()

func newRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		clients:      make(map[string]*clientState),
		junkRequests: make(chan junkRequest, 512),
	}
	go rl.consumeJunkRequests()
	return rl
}

// consumeJunkRequests drains abusive requests on a dedicated goroutine and
// waits until the client disconnects or the current ban window elapses,
// effectively ignoring the request body.
func (rl *RateLimiter) consumeJunkRequests() {
	for req := range rl.junkRequests {
		rl.consumeSingleJunk(req)
	}
}

func (rl *RateLimiter) consumeSingleJunk(req junkRequest) {
	// Drain the body to avoid holding server resources. Ignore any errors.
	if req.body != nil {
		_, _ = io.Copy(io.Discard, req.body)
		_ = req.body.Close()
	}

	wait := time.Until(req.bannedUntil)
	if wait < 0 {
		wait = initialBanDuration
	}

	timer := time.NewTimer(wait)
	select {
	case <-req.ctx.Done():
	case <-timer.C:
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

// evaluate checks request limits for an IP. It returns an optional delay to slow down the client
// and whether the IP is currently banned.
func (rl *RateLimiter) evaluate(ip string) (delay time.Duration, bannedUntil time.Time, banned bool) {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	state, ok := rl.clients[ip]
	if !ok {
		state = &clientState{windowStart: now, banDuration: initialBanDuration}
		rl.clients[ip] = state
	}

	if now.Before(state.banUntil) {
		return 0, state.banUntil, true
	}

	if now.Sub(state.windowStart) >= time.Second {
		state.windowStart = now
		state.count = 0
	}

	state.count++

	if state.count >= burstPerSecondThreshold {
		state.banUntil = now.Add(state.banDuration)
		rl.recordBan(ip, now, state.banDuration, "burst exceeded")
		state.banDuration *= 2
		if state.banDuration > time.Hour {
			state.banDuration = time.Hour
		}
		return 0, state.banUntil, true
	}

	if state.count > slowStartThreshold {
		delay = time.Duration(state.count-slowStartThreshold) * slowDelayStep
		if delay > maxSlowDelay {
			delay = maxSlowDelay
		}
	}

	return delay, time.Time{}, false
}

func (rl *RateLimiter) recordBan(ip string, start time.Time, duration time.Duration, reason string) {
	rl.banHistory = append(rl.banHistory, BanRecord{IP: ip, Start: start, Duration: duration, Reason: reason})
	if len(rl.banHistory) > 256 {
		rl.banHistory = rl.banHistory[len(rl.banHistory)-256:]
	}
}

// RateLimitMiddleware enforces per-IP throttling with escalating bans.
func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		delay, bannedUntil, banned := globalRateLimiter.evaluate(ip)

		if banned {
			log.Printf("IP %s banned until %s", ip, bannedUntil.Format(time.RFC3339))
			jr := junkRequest{ctx: c.Request.Context(), body: c.Request.Body, bannedUntil: bannedUntil}
			c.Abort()
			select {
			case globalRateLimiter.junkRequests <- jr:
			default:
				go globalRateLimiter.consumeSingleJunk(jr)
			}
			return
		}

		if delay > 0 {
			time.Sleep(delay)
		}

		c.Next()
	}
}

// BanHistory exposes recent ban events kept in memory for observability.
func BanHistory() []BanRecord {
	globalRateLimiter.mu.Lock()
	defer globalRateLimiter.mu.Unlock()

	history := make([]BanRecord, len(globalRateLimiter.banHistory))
	copy(history, globalRateLimiter.banHistory)
	return history
}

package middleware

import (
	"log"
	"net/http"
	"strconv"
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
	clientStateTTL          = 10 * time.Minute
	cleanupInterval         = time.Minute
)

type BanRecord struct {
	IP       string
	Start    time.Time
	Duration time.Duration
	Reason   string
}

type clientState struct {
	windowStart time.Time
	lastSeen    time.Time
	count       int
	banUntil    time.Time
	banDuration time.Duration
}

type RateLimiter struct {
	mu          sync.Mutex
	clients     map[string]*clientState
	banHistory  []BanRecord
	lastCleanup time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{clients: make(map[string]*clientState)}
}

func (rl *RateLimiter) evaluate(ip string, now time.Time) (delay time.Duration, bannedUntil time.Time, banned bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.lastCleanup.IsZero() || now.Sub(rl.lastCleanup) >= cleanupInterval {
		for key, state := range rl.clients {
			if now.Sub(state.lastSeen) >= clientStateTTL && !now.Before(state.banUntil) {
				delete(rl.clients, key)
			}
		}
		rl.lastCleanup = now
	}

	state, ok := rl.clients[ip]
	if !ok {
		state = &clientState{windowStart: now, lastSeen: now, banDuration: initialBanDuration}
		rl.clients[ip] = state
	}
	if now.Sub(state.lastSeen) >= clientStateTTL {
		state.banDuration = initialBanDuration
	}
	state.lastSeen = now

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
		state.banDuration = min(state.banDuration*2, time.Hour)
		return 0, state.banUntil, true
	}
	if state.count > slowStartThreshold {
		delay = min(time.Duration(state.count-slowStartThreshold)*slowDelayStep, maxSlowDelay)
	}
	return delay, time.Time{}, false
}

func (rl *RateLimiter) recordBan(ip string, start time.Time, duration time.Duration, reason string) {
	rl.banHistory = append(rl.banHistory, BanRecord{IP: ip, Start: start, Duration: duration, Reason: reason})
	if len(rl.banHistory) > 256 {
		rl.banHistory = rl.banHistory[len(rl.banHistory)-256:]
	}
}

func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()
		delay, bannedUntil, banned := rl.evaluate(ip, now)
		if banned {
			retryAfter := max(1, int(time.Until(bannedUntil).Seconds()+0.999))
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			log.Printf("rate limit: client %s blocked for %s", ip, time.Until(bannedUntil).Round(time.Second))
			return
		}

		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-c.Request.Context().Done():
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func (rl *RateLimiter) BanHistory() []BanRecord {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	history := make([]BanRecord, len(rl.banHistory))
	copy(history, rl.banHistory)
	return history
}

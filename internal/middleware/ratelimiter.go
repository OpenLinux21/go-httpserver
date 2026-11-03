package middleware

import (
	"hash/fnv"
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

	// shardCount splits client tracking so the hot path does not serialize
	// on a single lock.
	shardCount = 16
	// maxTrackedClients bounds the memory used for client state. Without a
	// cap, an attacker rotating source addresses (trivial with IPv6) could
	// grow the tracked set without limit.
	maxTrackedClients = 1 << 16
)

type clientState struct {
	windowStart time.Time
	lastSeen    time.Time
	count       int
	banUntil    time.Time
	banDuration time.Duration
}

type clientShard struct {
	mu          sync.Mutex
	clients     map[string]*clientState
	lastCleanup time.Time
}

type RateLimiter struct {
	shards [shardCount]clientShard
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{}
	for i := range rl.shards {
		rl.shards[i].clients = make(map[string]*clientState)
	}
	return rl
}

func (rl *RateLimiter) shardFor(ip string) *clientShard {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(ip))
	return &rl.shards[hash.Sum32()%shardCount]
}

// evaluate records a request from ip and reports whether the client must be
// delayed or is currently banned. newlyBanned is true only on the request
// that starts a ban, so callers log each ban exactly once instead of on
// every rejected request.
func (rl *RateLimiter) evaluate(ip string, now time.Time) (delay time.Duration, bannedUntil time.Time, banned bool, newlyBanned bool) {
	shard := rl.shardFor(ip)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if shard.lastCleanup.IsZero() || now.Sub(shard.lastCleanup) >= cleanupInterval {
		for key, state := range shard.clients {
			if now.Sub(state.lastSeen) >= clientStateTTL && !now.Before(state.banUntil) {
				delete(shard.clients, key)
			}
		}
		shard.lastCleanup = now
	}

	state, ok := shard.clients[ip]
	if !ok {
		if len(shard.clients) >= maxTrackedClients/shardCount {
			shard.evictLocked(now)
		}
		state = &clientState{windowStart: now, lastSeen: now, banDuration: initialBanDuration}
		shard.clients[ip] = state
	}
	if now.Sub(state.lastSeen) >= clientStateTTL {
		state.banDuration = initialBanDuration
	}
	state.lastSeen = now

	if now.Before(state.banUntil) {
		return 0, state.banUntil, true, false
	}
	if now.Sub(state.windowStart) >= time.Second {
		state.windowStart = now
		state.count = 0
	}

	state.count++
	if state.count >= burstPerSecondThreshold {
		state.banUntil = now.Add(state.banDuration)
		state.banDuration = min(state.banDuration*2, time.Hour)
		return 0, state.banUntil, true, true
	}
	if state.count > slowStartThreshold {
		delay = min(time.Duration(state.count-slowStartThreshold)*slowDelayStep, maxSlowDelay)
	}
	return delay, time.Time{}, false, false
}

// evictLocked makes room for one new client. Expired entries go first;
// otherwise an arbitrary entry is dropped so memory stays bounded even under
// address-rotation floods.
func (s *clientShard) evictLocked(now time.Time) {
	for key, state := range s.clients {
		if now.Sub(state.lastSeen) >= clientStateTTL && !now.Before(state.banUntil) {
			delete(s.clients, key)
			return
		}
	}
	for key := range s.clients {
		delete(s.clients, key)
		return
	}
}

func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		delay, bannedUntil, banned, newlyBanned := rl.evaluate(ip, time.Now())
		if banned {
			if newlyBanned {
				log.Printf("rate limit: client %s banned for %s", ip, time.Until(bannedUntil).Round(time.Second))
			}
			retryAfter := max(1, int(time.Until(bannedUntil).Seconds()+0.999))
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
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

package runtime

import (
	"context"
	"crypto/sha512"
	"encoding/binary"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

type Limiter struct {
	seed []byte
	sec  []byte
	per  int

	mu sync.Mutex
	m  map[string]map[int]SectionBehavior
}

func NewLimiterWith(b []byte, s []byte) *Limiter {
	if len(s) == 0 {
		s = dsec()
	}
	return &Limiter{
		seed: dup(b),
		sec:  s,
		per:  20,
		m:    make(map[string]map[int]SectionBehavior),
	}
}

func NewLimiter(b []byte) *Limiter {
	return NewLimiterWith(b, []byte(strings.TrimSpace(os.Getenv("XDL_LIMITER_SECRET"))))
}

func (l *Limiter) SetPagesPerSection(n int) {
	if n <= 0 {
		return
	}
	l.mu.Lock()
	l.per = n
	l.mu.Unlock()
}

func (l *Limiter) BehaviorFor(u string, p int) SectionBehavior {
	if p <= 0 {
		p = 1
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	i := l.idx(p)
	t, ok := l.m[u]
	if !ok {
		t = make(map[int]SectionBehavior)
		l.m[u] = t
	}
	if sb, ok := t[i]; ok {
		return sb
	}
	sb := DeriveSectionBehavior(l.seed, u, i, l.sec)
	t[i] = sb
	return sb
}

func (l *Limiter) SleepBeforeRequest(ctx context.Context, u string, p, r int) {
	sb := l.BehaviorFor(u, p)
	d := sb.BaseDelay
	if d <= 0 {
		return
	}
	h := sha512.New()
	h.Write(l.seed)
	h.Write([]byte(u))
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(p))
	h.Write(b[:])
	binary.BigEndian.PutUint64(b[:], uint64(r))
	h.Write(b[:])
	h.Write(l.sec)
	h.Write([]byte("|j"))
	s := h.Sum(nil)
	v := binary.BigEndian.Uint32(s[:4])
	x := float64(v) / float64(math.MaxUint32)
	j := (x*2 - 1) * sb.JitterFactor
	f := 1 + j
	if f < 0 {
		f = 0
	}
	d = time.Duration(float64(d) * f)
	if sb.BurstEvery > 0 && r > 0 && r%sb.BurstEvery == 0 {
		d += sb.BurstExtra
	}
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return
	case <-t.C:
		return
	}
}

func (l *Limiter) idx(p int) int {
	if l.per <= 0 {
		return 0
	}
	return (p - 1) / l.per
}

func dup(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	o := make([]byte, len(b))
	copy(o, b)
	return o
}

func dsec() []byte {
	s := strings.TrimSpace(os.Getenv("XDL_LIMITER_SECRET"))
	if s == "" {
		s = "xdl-limiter-secret-v1"
	}
	return []byte(s)
}

package runtime

import (
	"crypto/sha512"
	"encoding/binary"
	"math"
	"time"
)

type SectionBehavior struct {
	BaseDelay        time.Duration
	JitterFactor     float64
	BurstEvery       int
	BurstExtra       time.Duration
	FakeRequestProb  float64
	PageShuffleWidth int
}

func mix(seed []byte, u string, idx int, sec []byte) SectionBehavior {
	h := sha512.New()
	h.Write(seed)
	h.Write([]byte("|user:"))
	h.Write([]byte(u))
	h.Write([]byte("|section:"))
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(idx))
	h.Write(b[:])
	if len(sec) > 0 {
		h.Write([]byte("|secret:"))
		h.Write(sec)
	}
	sum := h.Sum(nil)

	pickU := func(o int, m uint32) uint32 {
		if m == 0 {
			return 0
		}
		v := binary.BigEndian.Uint32(sum[o : o+4])
		return v % m
	}
	pickF := func(o int) float64 {
		v := binary.BigEndian.Uint32(sum[o : o+4])
		return float64(v) / float64(math.MaxUint32)
	}

	bdms := 300 + pickU(0, 900)
	jf := 0.2 + pickF(4)*0.4
	be := 15 + int(pickU(8, 45))
	bems := 2000 + pickU(12, 5000)
	fp := pickF(16) * 0.15
	sw := 1 + int(pickU(20, 4))

	return SectionBehavior{
		BaseDelay:        time.Duration(bdms) * time.Millisecond,
		JitterFactor:     jf,
		BurstEvery:       be,
		BurstExtra:       time.Duration(bems) * time.Millisecond,
		FakeRequestProb:  fp,
		PageShuffleWidth: sw,
	}
}

func DeriveSectionBehavior(seed []byte, u string, idx int, sec []byte) SectionBehavior {
	return mix(seed, u, idx, sec)
}

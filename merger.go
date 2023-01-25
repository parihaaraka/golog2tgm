package golog2tgm

import (
	"hash/fnv"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type msgSample struct {
	level   int8
	tsFirst time.Time
	tsLast  time.Time
	message string
	count   int
	pcs     []uintptr
}

type batch struct {
	samples     map[uint64]*msgSample
	chronoIndex []uint64
	tsStart     time.Time
	tsFinish    time.Time
}

type merger struct {
	finalizedBatch batch
	activeBatch    batch
}

func (b *batch) empty() bool {
	return len(b.chronoIndex) == 0
}

func (b *batch) clear() {
	b.samples = nil
	b.chronoIndex = b.chronoIndex[:0]
	b.tsStart = time.Time{}
	b.tsFinish = b.tsStart
}

func (bDst *batch) takeFrom(bSrc *batch) {
	if bSrc.empty() {
		return
	}

	if bDst.empty() {
		*bSrc, *bDst = *bDst, *bSrc
		return
	}

	// merge
	for _, hash := range bSrc.chronoIndex {
		vSrc := bSrc.samples[hash]
		vDst, okDst := bDst.samples[hash]
		if okDst {
			vDst.count += vSrc.count
			if vSrc.tsFirst.Before(vDst.tsFirst) {
				vDst.tsFirst = vSrc.tsFirst
			}
			if vDst.tsLast.Before(vSrc.tsLast) {
				vDst.tsLast = vSrc.tsLast
			}
			continue
		}
		bDst.samples[hash] = vSrc
		bDst.chronoIndex = append(bDst.chronoIndex, hash)
	}

	if bSrc.tsFinish.After(bDst.tsFinish) {
		bDst.tsFinish = bSrc.tsFinish
	}

	sort.Slice(bDst.chronoIndex, func(i, j int) bool {
		return bDst.samples[bDst.chronoIndex[i]].tsFirst.Before(
			bDst.samples[bDst.chronoIndex[j]].tsFirst)
	})

	bSrc.clear()
}

func (b *batch) pushMsg(msg *logMessage) {
	if b.samples == nil {
		b.samples = map[uint64]*msgSample{}
	}

	var i int
	if msg.hash == 0 {
		filterHex := len(msg.message) > 1
		var essence strings.Builder
		essence.Grow(256)

		// level is included in the comparison
		essence.WriteRune(rune(msg.level))
		// skip numbers and special characters (should we ignore base58 addresses?)
		var c rune
		for i, c = range msg.message {
			if !((c >= '0' && c <= '9') ||
				(filterHex && ((c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f'))) ||
				c == '.' || c == '-' || c < 33) {
				essence.WriteRune(c)
				if essence.Len() >= 250 { // limit message and sample size (that was about characters i hope)
					break
				}
			}
		}
		hasher := fnv.New64a()
		hasher.Write([]byte(essence.String()))
		msg.hash = hasher.Sum64()
		i = i + utf8.RuneLen(c)
	}

	ts := time.Now()
	if b.tsStart.IsZero() {
		b.tsStart = ts
	}
	b.tsFinish = ts
	if s, ok := b.samples[msg.hash]; ok {
		s.count++
		s.tsLast = ts
		return
	}

	sample := msgSample{
		level:   msg.level,
		count:   1,
		tsFirst: ts,
		tsLast:  ts,
		pcs:     msg.pcs,
	}

	if i == 0 { // external hash
		runes := []rune(msg.message)
		if 250 < len(runes) {
			var tmp strings.Builder
			tmp.WriteString(string(runes[:250]))
			tmp.WriteRune('…')
			sample.message = tmp.String()
		} else {
			sample.message = msg.message
		}
	} else {
		if i < len(msg.message) {
			var tmp strings.Builder
			tmp.WriteString(msg.message[:i])
			tmp.WriteRune('…')
			sample.message = tmp.String()
		} else {
			sample.message = msg.message
		}
	}
	b.samples[msg.hash] = &sample
	b.chronoIndex = append(b.chronoIndex, msg.hash)
}

func (m *merger) FinalizedBatch() *batch {
	m.finalizedBatch.takeFrom(&m.activeBatch)
	return &m.finalizedBatch
}

func (m *merger) PushMsg(msg *logMessage) {
	if len(msg.message) == 0 {
		return
	}
	m.activeBatch.pushMsg(msg)
}

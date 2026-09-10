// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package simulation

import (
	"math/rand/v2"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/webrtc/v4/pkg/media"
)

const codecFPS = 30

// perfectPayloader chops a frame into MTU sized pieces and does nothing else.
type perfectPayloader struct{}

func (p *perfectPayloader) Payload(mtu uint16, payload []byte) [][]byte {
	if mtu == 0 {
		return nil
	}

	payloads := [][]byte{}
	for len(payload) > 0 {
		size := min(int(mtu), len(payload))
		payloads = append(payloads, payload[:size:size])
		payload = payload[size:]
	}

	return payloads
}

type sampleWriter interface {
	WriteSample(media.Sample) error
}

// perfectCodec implements a simple codec that produces frames at a constant rate
// with sizes exactly matching the target bitrate.
type perfectCodec struct {
	logger logging.LeveledLogger

	writer sampleWriter

	minTargetRateBps int
	maxTargetRateBps int
	fps              int

	mu               sync.Mutex
	targetBitrateBps int

	done chan struct{}
	wg   sync.WaitGroup
}

// newPerfectCodec creates a new PerfectCodec with the specified frame writer and target bitrate.
func newPerfectCodec(
	loggerFactory logging.LoggerFactory,
	writer sampleWriter,
	minTargetRateBps, maxTargetRateBps, initTargetBitrateBps int,
) *perfectCodec {
	return &perfectCodec{
		logger:           loggerFactory.NewLogger("perfect_codec"),
		writer:           writer,
		minTargetRateBps: minTargetRateBps,
		maxTargetRateBps: maxTargetRateBps,
		fps:              codecFPS,
		mu:               sync.Mutex{},
		targetBitrateBps: initTargetBitrateBps,
		done:             make(chan struct{}),
		wg:               sync.WaitGroup{},
	}
}

func (c *perfectCodec) setTargetBitrate(r int) {
	r = max(r, c.minTargetRateBps)
	r = min(r, c.maxTargetRateBps)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.targetBitrateBps = r
}

func (c *perfectCodec) targetBitrate() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.targetBitrateBps
}

// start begins the codec operation, generating frames at the configured frame rate.
func (c *perfectCodec) start() {
	c.wg.Go(func() {
		payloads := rand.NewChaCha8([32]byte{})
		frameInterval := time.Second / time.Duration(c.fps)
		ticker := time.NewTicker(frameInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				size := c.targetBitrate() / (8.0 * c.fps)
				buf := make([]byte, size)
				if _, err := payloads.Read(buf); err != nil {
					c.logger.Errorf("failed to read random bytes: %v", err)

					continue
				}
				if err := c.writer.WriteSample(media.Sample{
					Data:     buf,
					Duration: frameInterval,
				}); err != nil {
					c.logger.Errorf("failed to write sample: %v", err)

					continue
				}
			case <-c.done:
				return
			}
		}
	})
}

// Close stops the codec and cleans up resources.
func (c *perfectCodec) Close() error {
	close(c.done)
	c.wg.Wait()

	return nil
}

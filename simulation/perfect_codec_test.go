// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js

package simulation

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/webrtc/v4/pkg/media"
)

type sampleWriter interface {
	WriteSample(media.Sample) error
}

// perfectCodec implements a simple codec that produces frames at a constant rate
// with sizes exactly matching the target bitrate.
type perfectCodec struct {
	logger logging.LeveledLogger

	writer sampleWriter

	targetBitrateBps int
	fps              int
	bitrateUpdateCh  chan int

	done chan struct{}
	wg   sync.WaitGroup
}

// newPerfectCodec creates a new PerfectCodec with the specified frame writer and target bitrate.
func newPerfectCodec(writer sampleWriter, targetBitrateBps int) *perfectCodec {
	return &perfectCodec{
		logger:           logging.NewDefaultLoggerFactory().NewLogger("perfect_codec"),
		writer:           writer,
		targetBitrateBps: targetBitrateBps,
		fps:              30,
		bitrateUpdateCh:  make(chan int),
		done:             make(chan struct{}),
	}
}

// setTargetBitrate sets the target bitrate to r bits per second.
// func (c *perfectCodec) setTargetBitrate(r int) {
// 	c.wg.Go(func() {
// 		select {
// 		case c.bitrateUpdateCh <- r:
// 		case <-c.done:
// 		}
// 	})
// }

// start begins the codec operation, generating frames at the configured frame rate.
func (c *perfectCodec) start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		msToNextFrame := time.Duration((1.0/float64(c.fps))*1000.0) * time.Millisecond
		ticker := time.NewTicker(msToNextFrame)
		for {
			select {
			case <-ticker.C:
				size := c.targetBitrateBps / (8.0 * c.fps)
				buf := make([]byte, size)
				if _, err := rand.Read(buf); err != nil {
					c.logger.Errorf("failed to read random bytes: %v", err)

					continue
				}
				if err := c.writer.WriteSample(media.Sample{
					Data:     buf,
					Duration: msToNextFrame,
				}); err != nil {
					c.logger.Errorf("failed to write sample: %v", err)

					continue
				}
			case nextRate := <-c.bitrateUpdateCh:
				c.targetBitrateBps = nextRate
			case <-c.done:
				return
			}
		}
	}()
}

// Close stops the codec and cleans up resources.
func (c *perfectCodec) Close() error {
	close(c.done)
	c.wg.Wait()

	return nil
}

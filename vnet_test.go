// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js && go1.25

package bwe_test

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
)

func testLogger(t *testing.T) (*slog.Logger, func()) {
	t.Helper()

	logDir := os.Getenv("BWE_LOG_DIR")
	if logDir == "" {
		logDir = "logs"
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("failed to create log dir %q: %v", logDir, err)
	}

	filename := filepath.Join(logDir, fmt.Sprintf("%s.jsonl", t.Name()))
	file, err := os.Create(filename)
	if err != nil {
		t.Fatalf("failed to create log file %q: %v", filename, err)
	}

	handler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	cleanup := func() {
		file.Sync()
		file.Close()
	}

	return logger, cleanup
}

func TestVnet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		t.Helper()

		logger, cleanup := testLogger(t)
		defer cleanup()

		onTrack := make(chan struct{})
		connected := make(chan struct{})
		done := make(chan struct{})

		network := createVirtualNetwork(t)
		receiver, err := newPeer(
			registerDefaultCodecs(),
			setVNet(network.left, []string{"10.0.1.1"}),
			registerTWCC(),
			onRemoteTrack(func(track *webrtc.TrackRemote) {
				close(onTrack)
				go func() {
					buf := make([]byte, 1500)
					for {
						select {
						case <-done:
							return
						default:
							_, _, err := track.Read(buf)
							if errors.Is(err, io.EOF) {
								return
							}
							assert.NoError(t, err)
						}
					}
				}()
			}),
			registerPacketLogger(logger.With("vantage-point", "receiver")),
			registerCCFB(),
		)
		assert.NoError(t, err)

		err = receiver.addRemoteTrack()
		assert.NoError(t, err)

		var codec *perfectCodec
		sender, err := newPeer(
			registerDefaultCodecs(),
			onConnected(func() { close(connected) }),
			setVNet(network.right, []string{"10.0.2.1"}),
			registerPacketLogger(logger.With("vantage-point", "sender")),
			registerRTPFB(),
			initGCC(func(rate int) {
				codec.setTargetBitrate(rate)
			}),
		)
		assert.NoError(t, err)

		track, err := sender.addLocalTrack()
		assert.NoError(t, err)

		codec = newPerfectCodec(track, 1_000_000)
		go func() {
			<-connected
			codec.start()
		}()

		offer, err := sender.createOffer()
		assert.NoError(t, err)

		err = receiver.setRemoteDescription(offer)
		assert.NoError(t, err)

		answer, err := receiver.createAnswer()
		assert.NoError(t, err)

		err = sender.setRemoteDescription(answer)
		assert.NoError(t, err)

		synctest.Wait()
		select {
		case <-onTrack:
		case <-time.After(time.Second):
			assert.Fail(t, "on track not called")
		}
		time.Sleep(100 * time.Second)
		close(done)

		err = codec.Close()
		assert.NoError(t, err)

		err = sender.pc.Close()
		assert.NoError(t, err)

		err = receiver.pc.Close()
		assert.NoError(t, err)

		err = network.Close()
		assert.NoError(t, err)

		synctest.Wait()
	})
}

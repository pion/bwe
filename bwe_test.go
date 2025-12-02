// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js && go1.25

package bwe_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
)

var logDir string

type vnetFactory func(*testing.T) *virtualNetwork

func TestMain(m *testing.M) {
	logDir = os.Getenv("BWE_LOG_DIR")
	if logDir == "" {
		logDir = "test-web/logs"
	}
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		log.Printf("failed to create log dir %q: %v", logDir, err)
		os.Exit(1)
	}

	ec := m.Run()

	files, err := filepath.Glob(filepath.Join(logDir, "*.jsonl"))
	if err != nil {
		log.Printf("Failed to list JSONL files: %v", err)
	}

	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}

	b, err := json.Marshal(names)
	if err != nil {
		log.Printf("Failed to marshal index.json: %v", err)
		os.Exit(ec)
	}

	indexPath := filepath.Join(logDir, "index.json")
	if err := os.WriteFile(indexPath, b, 0600); err != nil {
		log.Printf("Failed to write index.json: %v", err)
	} else {
		log.Printf("Generated index.json with %d files", len(names))
	}

	os.Exit(ec)
}

func TestBWE(t *testing.T) {
	networks := map[string]vnetFactory{
		"1mbps-1ms":   createVirtualNetwork(1_000_000, 80_000, 1*time.Millisecond),
		"5mbps-1ms":   createVirtualNetwork(5_000_000, 80_000, 1*time.Millisecond),
		"1mbps-10ms":  createVirtualNetwork(1_000_000, 80_000, 10*time.Millisecond),
		"5mbps-10ms":  createVirtualNetwork(5_000_000, 80_000, 10*time.Millisecond),
		"1mbps-50ms":  createVirtualNetwork(1_000_000, 80_000, 50*time.Millisecond),
		"5mbps-50ms":  createVirtualNetwork(5_000_000, 80_000, 50*time.Millisecond),
		"1mbps-150ms": createVirtualNetwork(1_000_000, 80_000, 150*time.Millisecond),
		"5mbps-150ms": createVirtualNetwork(5_000_000, 80_000, 150*time.Millisecond),
		"1mbps-300ms": createVirtualNetwork(1_000_000, 80_000, 300*time.Millisecond),
		"5mbps-300ms": createVirtualNetwork(5_000_000, 80_000, 300*time.Millisecond),
	}
	peerOptions := map[string]struct {
		receiver     []option
		sender       []option
		codecMinRate int
		codecMaxRate int
	}{
		"gcc-ccfb": {
			receiver: []option{
				registerCCFB(),
			},
			sender: []option{
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: math.MaxInt,
		},
		"gcc-twcc": {
			receiver: []option{
				registerTWCC(),
			},
			sender: []option{
				registerTWCCHeaderExtension(),
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: math.MaxInt,
		},
		"gcc-ccfb-applimited1": {
			receiver: []option{
				registerCCFB(),
			},
			sender: []option{
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: 500_000,
		},
		"gcc-twcc-applimited1": {
			receiver: []option{
				registerTWCC(),
			},
			sender: []option{
				registerTWCCHeaderExtension(),
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: 500_000,
		},
		"gcc-ccfb-applimited2": {
			receiver: []option{
				registerCCFB(),
			},
			sender: []option{
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: 1_500_000,
		},
		"gcc-twcc-applimited2": {
			receiver: []option{
				registerTWCC(),
			},
			sender: []option{
				registerTWCCHeaderExtension(),
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: 1_500_00,
		},
		"gcc-ccfb-paced": {
			receiver: []option{
				registerCCFB(),
			},
			sender: []option{
				registerPacer(),
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: math.MaxInt,
		},
		"gcc-twcc-paced": {
			receiver: []option{
				registerTWCC(),
			},
			sender: []option{
				registerPacer(),
				registerTWCCHeaderExtension(),
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: math.MaxInt,
		},
		"gcc-ccfb-applimited1-paced": {
			receiver: []option{
				registerCCFB(),
			},
			sender: []option{
				registerPacer(),
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: 500_000,
		},
		"gcc-twcc-applimited1-paced": {
			receiver: []option{
				registerTWCC(),
			},
			sender: []option{
				registerPacer(),
				registerTWCCHeaderExtension(),
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: 500_000,
		},
		"gcc-ccfb-applimited2-paced": {
			receiver: []option{
				registerCCFB(),
			},
			sender: []option{
				registerPacer(),
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: 1_500_000,
		},
		"gcc-twcc-applimited2-paced": {
			receiver: []option{
				registerTWCC(),
			},
			sender: []option{
				registerPacer(),
				registerTWCCHeaderExtension(),
				initGCC(),
			},
			codecMinRate: 0,
			codecMaxRate: 1_500_00,
		},
	}
	for netName, vnf := range networks {
		for peerName, pos := range peerOptions {
			t.Run(fmt.Sprintf("%v-%v", netName, peerName), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					t.Helper()

					logger, cleanup := testLogger(t)
					defer cleanup()

					onTrack := make(chan struct{})
					connected := make(chan struct{})
					done := make(chan struct{})

					network := vnf(t)

					receiverOptions := []option{
						registerDefaultCodecs(),
						setVNet(network.left, []string{"10.0.1.1"}),
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
					}
					receiverOptions = append(receiverOptions, pos.receiver...)
					receiver, err := newPeer(receiverOptions...)
					assert.NoError(t, err)

					err = receiver.addRemoteTrack()
					assert.NoError(t, err)

					var codec *perfectCodec
					senderOptions := []option{
						registerDefaultCodecs(),
						onConnected(func() { close(connected) }),
						setVNet(network.right, []string{"10.0.2.1"}),
						registerPacketLogger(logger.With("vantage-point", "sender")),
						registerRTPFB(),
						setOnRateCallback(func(rate int) {
							logger.Info("setting codec target bitrate", "rate", rate)
							codec.setTargetBitrate(rate)
						}),
					}
					senderOptions = append(senderOptions, pos.sender...)
					sender, err := newPeer(senderOptions...)
					assert.NoError(t, err)

					track, err := sender.addLocalTrack()
					assert.NoError(t, err)

					codec = newPerfectCodec(
						track,
						pos.codecMinRate,
						pos.codecMaxRate,
						1_000_000,
					)
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
					case <-time.After(5 * time.Second):
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
			})
		}
	}
}

func testLogger(t *testing.T) (*slog.Logger, func()) {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "-")
	filename := filepath.Join(logDir, fmt.Sprintf("%s.jsonl", name))
	file, err := os.Create(filename)
	if err != nil {
		assert.Failf(t, "failed to create log file %q: %v", filename, err)
	}

	handler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	cleanup := func() {
		assert.NoError(t, file.Sync())
		assert.NoError(t, file.Close())
	}

	return logger, cleanup
}

// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package simulation

import (
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
	"github.com/stretchr/testify/require"
)

// logDir is where every scenario writes its two log files. TestMain sets it up
// before any test runs.
var logDir string //nolint:gochecknoglobals // set up once by TestMain

type vnetFactory func(*testing.T) *virtualNetwork

// TestMain has no *testing.T to report through and has to set the process exit
// code itself, hence the plain log and os.Exit calls.
//
//nolint:forbidigo // TestMain reports through the process exit code
func TestMain(m *testing.M) {
	logDir = os.Getenv("BWE_LOG_DIR")
	if logDir == "" {
		logDir = "logs/"
	}
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		log.Printf("failed to create log dir %q: %v", logDir, err)
		os.Exit(1)
	}
	// The pion loggers whose trace output the plots are built from.
	traceLoggers := []string{
		"bwe_send_side_controller",
		"bwe_delay_rate_controller",
		"bwe_test_peer",
		"perfect_codec",
		"ccfb_interceptor",
	}
	if err := os.Setenv("PION_LOG_TRACE", strings.Join(traceLoggers, ",")); err != nil {
		log.Printf("failed to set pion logger environment variable: %v", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// simulationNetworks returns one bottleneck network per rate and one way delay
// combination.
func simulationNetworks() map[string]vnetFactory {
	rates := []int{1_000_000, 5_000_000, 20_000_000}
	delays := []time.Duration{
		1 * time.Millisecond,
		10 * time.Millisecond,
		50 * time.Millisecond,
		150 * time.Millisecond,
		300 * time.Millisecond,
	}

	networks := map[string]vnetFactory{}
	for _, rate := range rates {
		for _, delay := range delays {
			name := fmt.Sprintf("%vmbps-%vms", rate/1_000_000, delay.Milliseconds())
			networks[name] = createVirtualNetwork(rate, delay)
		}
	}

	return networks
}

// peerConfig is one way of setting up the two peers of a scenario.
type peerConfig struct {
	receiver     []option
	sender       []option
	codecMinRate int
	codecMaxRate int
}

// peerConfigurations returns every combination of feedback mechanism,
// application limit and pacing. The options are safe to share between
// configurations: each of them builds its interceptors when it is applied to a
// peer, not when it is created here.
func peerConfigurations() map[string]peerConfig {
	feedbacks := []struct {
		name     string
		receiver []option
		sender   []option
	}{
		{
			name:     "ccfb",
			receiver: []option{registerCCFB()},
			sender:   []option{initGCC()},
		},
		{
			name:     "twcc",
			receiver: []option{registerTWCC()},
			sender:   []option{registerTWCCHeaderExtension(), initGCC()},
		},
	}
	appLimits := []struct {
		name string
		rate int
	}{
		{name: "", rate: math.MaxInt},
		{name: "-applimited500", rate: 500_000},
		{name: "-applimited1500", rate: 1_500_000},
	}
	pacings := []struct {
		name string
		opts []option
	}{
		{name: "", opts: nil},
		{name: "-paced", opts: []option{registerPacer()}},
	}

	configs := map[string]peerConfig{}
	for _, feedback := range feedbacks {
		for _, appLimit := range appLimits {
			for _, pacing := range pacings {
				name := fmt.Sprintf("gcc-%v%v%v", feedback.name, appLimit.name, pacing.name)
				// The pacer has to be registered before the feedback
				// interceptors.
				sender := append([]option{}, pacing.opts...)
				sender = append(sender, feedback.sender...)

				configs[name] = peerConfig{
					receiver:     feedback.receiver,
					sender:       sender,
					codecMinRate: 0,
					codecMaxRate: appLimit.rate,
				}
			}
		}
	}

	return configs
}

func TestBWE(t *testing.T) {
	for netName, newNetwork := range simulationNetworks() {
		for peerName, config := range peerConfigurations() {
			t.Run(fmt.Sprintf("%v-%v", netName, peerName), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					runScenario(t, newNetwork, config)
				})
			})
		}
	}
}

// runScenario runs a single sender to receiver session over the given network
// for 100 seconds, logging every packet and every controller decision.
func runScenario(t *testing.T, newNetwork vnetFactory, config peerConfig) {
	t.Helper()

	logger, cleanup := testLogger(t)
	defer cleanup()

	onTrack := make(chan struct{})
	connected := make(chan struct{})
	done := make(chan struct{})

	network := newNetwork(t)

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
	receiverOptions = append(receiverOptions, config.receiver...)
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
			codec.setTargetBitrate(int(0.9 * float64(rate)))
		}),
	}
	senderOptions = append(senderOptions, config.sender...)
	sender, err := newPeer(senderOptions...)
	assert.NoError(t, err)

	track, err := sender.addLocalTrack()
	assert.NoError(t, err)

	codec = newPerfectCodec(
		track,
		config.codecMinRate,
		config.codecMaxRate,
		1_000_000,
	)
	sender.startRTCPReader()

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
}

func testLogger(t *testing.T) (*slog.Logger, func()) {
	t.Helper()
	name, ok := strings.CutPrefix(t.Name(), "TestBWE/")
	if !ok {
		assert.FailNow(t, "test case with invalid name tried to create logfile")
	}
	name = strings.ReplaceAll(name, "/", "-")
	filename := filepath.Join(logDir, fmt.Sprintf("%s.jsonl", name))
	file, err := os.Create(filename) //nolint:gosec // path is logDir plus the test name
	require.NoError(t, err, "failed to create log file %q", filename)

	handler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Also create a log file for stdout redirects to capture Pions builtin logs
	stderrFileName := filepath.Join(logDir, fmt.Sprintf("%s.stderr", name))
	stderrFile, err := os.Create(stderrFileName) //nolint:gosec // path is logDir plus the test name
	require.NoError(t, err, "failed to create stderr file %q", stderrFileName)
	old := os.Stderr
	os.Stderr = stderrFile

	cleanup := func() {
		os.Stderr = old
		assert.NoError(t, file.Sync())
		assert.NoError(t, file.Close())
		assert.NoError(t, stderrFile.Sync())
		assert.NoError(t, stderrFile.Close())
	}

	return logger, cleanup
}

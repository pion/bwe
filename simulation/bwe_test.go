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
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pion/logging"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logDir is where every scenario writes its two log files. TestMain sets it up
// before any test runs.
var logDir string //nolint:gochecknoglobals // set up once by TestMain

type vnetFactory func(*testing.T, logging.LoggerFactory) *virtualNetwork

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

// namedNetwork is one bottleneck network of the scenario matrix.
type namedNetwork struct {
	name       string
	newNetwork vnetFactory
}

// simulationNetworks returns one bottleneck network per rate and one way delay
// combination, in a deterministic order.
func simulationNetworks() []namedNetwork {
	rates := []int{1_000_000, 5_000_000, 20_000_000}
	delays := []time.Duration{
		1 * time.Millisecond,
		10 * time.Millisecond,
		50 * time.Millisecond,
		150 * time.Millisecond,
		300 * time.Millisecond,
	}

	networks := []namedNetwork{}
	for _, rate := range rates {
		for _, delay := range delays {
			name := fmt.Sprintf("%vmbps-%vms", rate/1_000_000, delay.Milliseconds())
			networks = append(networks, namedNetwork{
				name:       name,
				newNetwork: createVirtualNetwork(rate, delay),
			})
		}
	}

	return networks
}

// namedPeerConfig is one peer setup of the scenario matrix.
type namedPeerConfig struct {
	name   string
	config peerConfig
}

// peerConfig is one way of setting up the two peers of a scenario.
type peerConfig struct {
	receiver     []option
	sender       []option
	codecMinRate int
	codecMaxRate int
}

// peerConfigurations returns every combination of feedback mechanism,
// application limit and pacing, in a deterministic order. The options are safe
// to share between configurations: each of them builds its interceptors when it
// is applied to a peer, not when it is created here.
func peerConfigurations() []namedPeerConfig {
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

	configs := []namedPeerConfig{}
	for _, feedback := range feedbacks {
		for _, appLimit := range appLimits {
			for _, pacing := range pacings {
				name := fmt.Sprintf("gcc-%v%v%v", feedback.name, appLimit.name, pacing.name)
				// The pacer has to be registered before the feedback
				// interceptors.
				sender := append([]option{}, pacing.opts...)
				sender = append(sender, feedback.sender...)

				configs = append(configs, namedPeerConfig{
					name: name,
					config: peerConfig{
						receiver:     feedback.receiver,
						sender:       sender,
						codecMinRate: 0,
						codecMaxRate: appLimit.rate,
					},
				})
			}
		}
	}

	return configs
}

func TestBWE(t *testing.T) {
	for _, network := range simulationNetworks() {
		for _, peer := range peerConfigurations() {
			t.Run(fmt.Sprintf("%v-%v", network.name, peer.name), func(t *testing.T) {
				t.Parallel()
				synctest.Test(t, func(t *testing.T) {
					runScenario(t, network.newNetwork, peer.config)
				})
			})
		}
	}
}

const (
	// scenarioDuration is how long a scenario keeps sending.
	scenarioDuration = 100 * time.Second
)

// runScenario runs a single sender to receiver session over the given network,
// logging every packet and every controller decision, and asserts that the
// controller behaved on it.
func runScenario(t *testing.T, newNetwork vnetFactory, config peerConfig) {
	t.Helper()

	logger, loggerFactory, cleanup := testLogger(t)
	defer cleanup()

	onTrack := make(chan struct{})
	connected := make(chan struct{})
	done := make(chan struct{})

	network := newNetwork(t, loggerFactory)
	collected := newMetrics()

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
						if err == nil {
							continue
						}
						// Teardown closes the peer connection while
						// this read is blocked, and the error then
						// comes from whichever layer closed first.
						select {
						case <-done:
							return
						default:
						}
						if !errors.Is(err, io.EOF) &&
							!errors.Is(err, io.ErrClosedPipe) &&
							!errors.Is(err, net.ErrClosed) {
							assert.NoError(t, err)
						}

						return
					}
				}
			}()
		}),
		registerPacketLogger(logger.With("vantage-point", "receiver"), collected.onArrived, nil),
	}
	receiverOptions = append(receiverOptions, config.receiver...)
	receiver, err := newPeer(loggerFactory, receiverOptions...)
	require.NoError(t, err)

	err = receiver.addRemoteTrack()
	require.NoError(t, err)

	var codec *perfectCodec
	senderOptions := []option{
		registerDefaultCodecs(),
		onConnected(func() { close(connected) }),
		setVNet(network.right, []string{"10.0.2.1"}),
		registerPacketLogger(logger.With("vantage-point", "sender"), nil, collected.onSent),
		registerRTPFB(),
		setOnRateCallback(func(rate int) {
			logger.Info("setting codec target bitrate", "rate", rate)
			collected.onTargetRate(rate)
			codec.setTargetBitrate(int(codecRateFactor * float64(rate)))
		}),
	}
	senderOptions = append(senderOptions, config.sender...)
	sender, err := newPeer(loggerFactory, senderOptions...)
	require.NoError(t, err)

	track, err := sender.addLocalTrack()
	require.NoError(t, err)

	codec = newPerfectCodec(
		loggerFactory,
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
	require.NoError(t, err)

	err = receiver.setRemoteDescription(offer)
	require.NoError(t, err)

	answer, err := receiver.createAnswer()
	require.NoError(t, err)

	err = sender.setRemoteDescription(answer)
	require.NoError(t, err)

	synctest.Wait()

	select {
	case <-onTrack:
	case <-time.After(5 * time.Second):
		require.Fail(t, "on track not called")
	}

	time.Sleep(scenarioDuration)
	close(done)

	logScenarioMetrics(logger, collected, network, config)
	assertScenario(t, collected, network)

	err = codec.Close()
	assert.NoError(t, err)

	err = sender.pc.Close()
	assert.NoError(t, err)

	// Let the packets still in flight arrive before closing the receiver, so
	// that the read goroutine is blocked on the transport and not inside a
	// receiver interceptor when that interceptor stops.
	synctest.Wait()

	err = receiver.pc.Close()
	assert.NoError(t, err)

	err = network.Close()
	assert.NoError(t, err)

	synctest.Wait()
}

// testLogger returns the structured logger the scenario writes its own events
// to and a logger factory that sends the pion trace output to a second file.
// Both are per scenario, so that scenarios can run in parallel.
func testLogger(t *testing.T) (*slog.Logger, logging.LoggerFactory, func()) {
	t.Helper()
	name, ok := strings.CutPrefix(t.Name(), "TestBWE/")
	if !ok {
		require.FailNow(t, "test case with invalid name tried to create logfile")
	}
	name = strings.ReplaceAll(name, "/", "-")
	filename := filepath.Join(logDir, fmt.Sprintf("%s.jsonl", name))
	file, err := os.Create(filename) //nolint:gosec // path is logDir plus the test name
	require.NoError(t, err, "failed to create log file %q", filename)

	handler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Also create a log file to capture Pions builtin logs
	traceFileName := filepath.Join(logDir, fmt.Sprintf("%s.stderr", name))
	traceFile, err := os.Create(traceFileName) //nolint:gosec // path is logDir plus the test name
	require.NoError(t, err, "failed to create trace file %q", traceFileName)
	loggerFactory := logging.NewDefaultLoggerFactory()
	loggerFactory.Writer = traceFile

	cleanup := func() {
		assert.NoError(t, file.Sync())
		assert.NoError(t, file.Close())
		assert.NoError(t, traceFile.Sync())
		assert.NoError(t, traceFile.Close())
	}

	return logger, loggerFactory, cleanup
}

// wireRate converts a payload rate into the rate the same stream is counted at
// by the metrics, which add the RTP header of every packet to its payload.
func wireRate(payloadRate int, overheadFactor float64) int {
	if float64(payloadRate) > math.MaxInt/overheadFactor {
		return payloadRate
	}

	return int(float64(payloadRate) * overheadFactor)
}

func logScenarioMetrics(
	logger *slog.Logger,
	collected *metrics,
	network *virtualNetwork,
	config peerConfig,
) {
	achievable := min(network.rate, wireRate(config.codecMaxRate, collected.headerOverheadFactor()))
	appLimit := config.codecMaxRate
	if appLimit == math.MaxInt {
		appLimit = 0
	}

	logger.Info(
		"scenario metrics",
		"bottleneck-rate", network.rate,
		"one-way-delay", network.delay,
		"queue-size", network.queueSize,
		"app-limit-rate", appLimit,
		"achievable-rate", achievable,
		"ramp-time", collected.rampTime(float64(achievable)),
		"steady-state", collected.window(steadyStateStart, scenarioDuration),
	)
}

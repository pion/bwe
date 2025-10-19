// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js

package bwe_test

import (
	"log/slog"

	"github.com/pion/bwe/gcc"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/pacing"
	"github.com/pion/interceptor/pkg/packetdump"
	"github.com/pion/interceptor/pkg/rfc8888"
	"github.com/pion/interceptor/pkg/rtpfb"
	"github.com/pion/logging"
	"github.com/pion/transport/v3/vnet"
	"github.com/pion/webrtc/v4"
)

type option func(*peer) error

func setVNet(vnet *vnet.Net, publicIPs []string) option {
	return func(p *peer) error {
		p.settingEngine.SetNet(vnet)
		p.settingEngine.SetNAT1To1IPs(publicIPs, webrtc.ICECandidateTypeHost)

		return nil
	}
}

func onRemoteTrack(handler func(*webrtc.TrackRemote)) option {
	return func(p *peer) error {
		p.onRemoteTrack = handler

		return nil
	}
}

func onConnected(handler func()) option {
	return func(p *peer) error {
		p.onConnected = handler

		return nil
	}
}

func registerDefaultCodecs() option {
	return func(p *peer) error {
		return p.mediaEngine.RegisterDefaultCodecs()
	}
}

func registerPacketLogger(logger *slog.Logger) option {
	return func(p *peer) error {
		ipl := newPacketLogger(logger, "in")
		rd, err := packetdump.NewReceiverInterceptor(packetdump.PacketLog(ipl))
		if err != nil {
			return err
		}
		opl := newPacketLogger(logger, "out")
		sd, err := packetdump.NewSenderInterceptor(packetdump.PacketLog(opl))
		if err != nil {
			return err
		}
		p.interceptorRegistry.Add(rd)
		p.interceptorRegistry.Add(sd)

		return nil
	}
}

func registerRTPFB() option {
	return func(p *peer) error {
		rtpfb, err := rtpfb.NewInterceptor()
		if err != nil {
			return err
		}
		p.interceptorRegistry.Add(rtpfb)

		return nil
	}
}

func registerTWCC() option {
	return func(p *peer) error {
		return webrtc.ConfigureTWCCSender(p.mediaEngine, p.interceptorRegistry)
	}
}

func registerTWCCHeaderExtension() option {
	return func(p *peer) error {
		return webrtc.ConfigureTWCCHeaderExtensionSender(p.mediaEngine, p.interceptorRegistry)
	}
}

func registerCCFB() option {
	return func(p *peer) error {
		ccfb, err := rfc8888.NewSenderInterceptor()
		if err != nil {
			return err
		}
		p.interceptorRegistry.Add(ccfb)

		return nil
	}
}

func initGCC() option {
	return func(p *peer) (err error) {
		p.estimator, err = gcc.NewSendSideController(1_000_000, 128_000, 50_000_000)
		if err != nil {
			return err
		}

		return nil
	}
}

func setOnRateCallback(onRateUpdate func(int)) option {
	return func(p *peer) error {
		p.onRateUpdate = onRateUpdate

		return nil
	}
}

func registerPacer() option {
	return func(p *peer) error {
		p.pacer = pacing.NewInterceptor()
		p.interceptorRegistry.Add(p.pacer)

		return nil
	}
}

type peer struct {
	logger logging.LeveledLogger
	pc     *webrtc.PeerConnection

	settingEngine       *webrtc.SettingEngine
	mediaEngine         *webrtc.MediaEngine
	interceptorRegistry *interceptor.Registry

	onRemoteTrack func(*webrtc.TrackRemote)
	onConnected   func()

	pacer        *pacing.InterceptorFactory
	estimator    *gcc.SendSideController
	onRateUpdate func(int)
}

func newPeer(opts ...option) (*peer, error) {
	peer := &peer{
		logger:              logging.NewDefaultLoggerFactory().NewLogger("bwe_test_peer"),
		pc:                  nil,
		settingEngine:       &webrtc.SettingEngine{},
		mediaEngine:         &webrtc.MediaEngine{},
		interceptorRegistry: &interceptor.Registry{},
		onRemoteTrack:       nil,
		onConnected:         nil,
	}
	for _, opt := range opts {
		if err := opt(peer); err != nil {
			return nil, err
		}
	}
	pc, err := webrtc.NewAPI(
		webrtc.WithMediaEngine(peer.mediaEngine),
		webrtc.WithSettingEngine(*peer.settingEngine),
		webrtc.WithInterceptorRegistry(peer.interceptorRegistry),
	).NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	pc.OnNegotiationNeeded(peer.onNegotiationNeeded)
	pc.OnSignalingStateChange(peer.onSignalingStateChange)
	pc.OnICECandidate(peer.onICECandidate)
	pc.OnICEGatheringStateChange(peer.onICEGatheringStateChange)
	pc.OnICEConnectionStateChange(peer.onICEConnectionStateChange)
	pc.OnConnectionStateChange(peer.onConnectionStateChange)
	pc.OnDataChannel(peer.onDataChannel)
	pc.OnTrack(peer.onTrack)

	peer.pc = pc

	return peer, nil
}

// Callbacks

func (p *peer) onNegotiationNeeded() {
	p.logger.Infof("negotiation needed")
}

func (p *peer) onSignalingStateChange(s webrtc.SignalingState) {
	p.logger.Infof("new signaling state: %v", s)
}

func (p *peer) onICECandidate(c *webrtc.ICECandidate) {
	p.logger.Infof("got new ICE candidate: %v", c)
}

func (p *peer) onICEGatheringStateChange(s webrtc.ICEGatheringState) {
	p.logger.Infof("new ICE gathering state: %v", s)
}

func (p *peer) onICEConnectionStateChange(s webrtc.ICEConnectionState) {
	p.logger.Infof("new ICE connection state: %v", s)
}

func (p *peer) onConnectionStateChange(s webrtc.PeerConnectionState) {
	p.logger.Infof("new connection state: %v", s)
	if s == webrtc.PeerConnectionStateConnected && p.onConnected != nil {
		p.onConnected()
	}
}

func (p *peer) onDataChannel(dc *webrtc.DataChannel) {
	p.logger.Infof("got new data channel: id=%v, label=%v", dc.ID(), dc.Label())
}

func (p *peer) onTrack(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
	if p.onRemoteTrack != nil {
		p.onRemoteTrack(track)
	}
}

// Signaling helpers

func (p *peer) createOffer() (*webrtc.SessionDescription, error) {
	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return nil, err
	}
	gc := webrtc.GatheringCompletePromise(p.pc)
	if err = p.pc.SetLocalDescription(offer); err != nil {
		return nil, err
	}
	<-gc

	return p.pc.LocalDescription(), nil
}

func (p *peer) createAnswer() (*webrtc.SessionDescription, error) {
	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}
	gc := webrtc.GatheringCompletePromise(p.pc)
	if err = p.pc.SetLocalDescription(answer); err != nil {
		return nil, err
	}
	<-gc

	return p.pc.LocalDescription(), nil
}

func (p *peer) setRemoteDescription(description *webrtc.SessionDescription) error {
	return p.pc.SetRemoteDescription(*description)
}

// Track management

func (p *peer) addLocalTrack() (*webrtc.TrackLocalStaticSample, error) {
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType:     webrtc.MimeTypeH264,
		ClockRate:    0,
		Channels:     0,
		SDPFmtpLine:  "",
		RTCPFeedback: []webrtc.RTCPFeedback{},
	}, "video", "pion")
	if err != nil {
		return nil, err
	}
	s, err := p.pc.AddTrack(track)
	if err != nil {
		return nil, err
	}
	go p.readRTCP(s)

	return track, err
}

func (p *peer) addRemoteTrack() error {
	_, err := p.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)

	return err
}

func (p *peer) readRTCP(r *webrtc.RTPSender) {
	for {
		_, attr, err := r.ReadRTCP()
		if err != nil {
			return
		}
		report, ok := attr.Get(rtpfb.CCFBAttributesKey).(rtpfb.Report)
		if ok {
			p.updateTargetRate(report)
		}
	}
}

func (p *peer) updateTargetRate(report rtpfb.Report) {
	if p.estimator != nil {
		for _, pr := range report.PacketReports {
			if pr.Arrived {
				p.estimator.OnAck(
					pr.SequenceNumber,
					pr.Size,
					pr.Departure,
					pr.Arrival,
				)
			} else {
				p.estimator.OnLoss()
			}
		}
		rate := p.estimator.OnFeedback(report.Arrival, report.RTT)
		p.logger.Infof("new target rate: %v", rate)
		if p.onRateUpdate != nil {
			p.onRateUpdate(rate)
		}
		if p.pacer != nil {
			p.pacer.SetRate(p.pc.ID(), rate)
		}
	}
}

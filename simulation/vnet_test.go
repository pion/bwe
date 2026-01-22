// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js && go1.25

package simulation

import (
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/v3/vnet"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
)

type network struct {
	wan   *vnet.Router
	left  *vnet.Net
	right *vnet.Net
}

func (n *network) Close() error {
	return n.wan.Stop()
}

func createVirtualNetwork(t *testing.T) *network {
	t.Helper()

	wan, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "0.0.0.0/0",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	assert.NoError(t, err)

	leftRouter, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR: "10.0.1.0/24",
		StaticIPs: []string{
			"10.0.1.1/10.0.1.101",
		},
		LoggerFactory: logging.NewDefaultLoggerFactory(),
		NATType: &vnet.NATType{
			Mode: vnet.NATModeNAT1To1,
		},
	})
	assert.NoError(t, err)
	err = wan.AddRouter(leftRouter)
	assert.NoError(t, err)

	rightRouter, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR: "10.0.2.0/24",
		StaticIPs: []string{
			"10.0.2.1/10.0.2.101",
		},
		LoggerFactory: logging.NewDefaultLoggerFactory(),
		NATType: &vnet.NATType{
			Mode: vnet.NATModeNAT1To1,
		},
	})
	assert.NoError(t, err)
	err = wan.AddRouter(rightRouter)
	assert.NoError(t, err)

	err = wan.Start()
	assert.NoError(t, err)

	leftNet, err := vnet.NewNet(&vnet.NetConfig{
		StaticIPs: []string{"10.0.1.101"},
		StaticIP:  "",
	})
	assert.NoError(t, err)
	err = leftRouter.AddNet(leftNet)
	assert.NoError(t, err)

	rightNet, err := vnet.NewNet(&vnet.NetConfig{
		StaticIPs: []string{"10.0.2.101"},
		StaticIP:  "",
	})
	assert.NoError(t, err)
	err = rightRouter.AddNet(rightNet)
	assert.NoError(t, err)

	return &network{
		wan:   wan,
		left:  leftNet,
		right: rightNet,
	}
}

func TestVnet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		t.Helper()

		onTrack := make(chan struct{})
		connected := make(chan struct{})
		done := make(chan struct{})

		network := createVirtualNetwork(t)
		receiver, err := newPeer(
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
			registerPacketLogger("receiver"),
			registerCCFB(),
		)
		assert.NoError(t, err)

		err = receiver.addRemoteTrack()
		assert.NoError(t, err)

		sender, err := newPeer(
			registerDefaultCodecs(),
			onConnected(func() { close(connected) }),
			setVNet(network.right, []string{"10.0.2.1"}),
			registerPacketLogger("sender"),
			registerRTPFB(),
		)
		assert.NoError(t, err)

		track, err := sender.addLocalTrack()
		assert.NoError(t, err)

		codec := newPerfectCodec(track, 1_000_000)
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
		time.Sleep(10 * time.Second)
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

// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package simulation

import (
	"errors"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/v4/vnet"
	"github.com/stretchr/testify/require"
)

const (
	mtu           = 1500
	tbfBurst      = 2 * mtu
	minQueueDelay = 50 * time.Millisecond
)

type virtualNetwork struct {
	rate      int
	delay     time.Duration
	queueSize int

	wan       *vnet.Router
	left      *vnet.Net
	leftTBF   *vnet.Queue
	leftDelay *vnet.DelayFilter

	right      *vnet.Net
	rightTBF   *vnet.Queue
	rightDelay *vnet.DelayFilter
}

func (n *virtualNetwork) Close() error {
	return errors.Join(
		n.wan.Stop(),
		n.leftTBF.Close(),
		n.leftDelay.Close(),
		n.rightTBF.Close(),
		n.rightDelay.Close(),
	)
}

// queueSizeBytes returns the queue size of a link that holds the given time
// worth of traffic at the given rate.
func queueSizeBytes(rate int, duration time.Duration) int {
	return int(float64(rate) / 8 * duration.Seconds())
}

func createVirtualNetwork(rate int, delay time.Duration) vnetFactory {
	return func(t *testing.T, loggerFactory logging.LoggerFactory) *virtualNetwork {
		t.Helper()

		bottleneckQueueSize := max(
			queueSizeBytes(rate, delay),
			queueSizeBytes(rate, minQueueDelay),
		)

		wan, err := vnet.NewRouter(&vnet.RouterConfig{
			CIDR:          "0.0.0.0/0",
			LoggerFactory: loggerFactory,
		})
		require.NoError(t, err)

		leftRouter, err := vnet.NewRouter(&vnet.RouterConfig{
			CIDR: "10.0.1.0/24",
			StaticIPs: []string{
				"10.0.1.1/10.0.1.101",
			},
			LoggerFactory: loggerFactory,
			NATType: &vnet.NATType{
				Mode: vnet.NATModeNAT1To1,
			},
		})
		require.NoError(t, err)

		leftTBF, err := vnet.NewQueue(
			leftRouter,
			vnet.NewTBFQueue(rate, tbfBurst, int64(bottleneckQueueSize)),
		)
		require.NoError(t, err)

		leftDelay, err := vnet.NewDelayFilter(leftTBF, vnet.WithDelay(delay))
		require.NoError(t, err)

		err = wan.AddNet(leftDelay)
		require.NoError(t, err)

		err = wan.AddChildRouter(leftRouter)
		require.NoError(t, err)

		rightRouter, err := vnet.NewRouter(&vnet.RouterConfig{
			CIDR: "10.0.2.0/24",
			StaticIPs: []string{
				"10.0.2.1/10.0.2.101",
			},
			LoggerFactory: loggerFactory,
			NATType: &vnet.NATType{
				Mode: vnet.NATModeNAT1To1,
			},
		})
		require.NoError(t, err)

		rightTBF, err := vnet.NewQueue(
			rightRouter,
			vnet.NewTBFQueue(rate, tbfBurst, int64(bottleneckQueueSize)),
		)
		require.NoError(t, err)

		rightDelay, err := vnet.NewDelayFilter(rightTBF, vnet.WithDelay(delay))
		require.NoError(t, err)

		err = wan.AddNet(rightDelay)
		require.NoError(t, err)

		err = wan.AddChildRouter(rightRouter)
		require.NoError(t, err)

		err = wan.Start()
		require.NoError(t, err)

		leftNet, err := vnet.NewNet(&vnet.NetConfig{
			StaticIPs: []string{"10.0.1.101"},
		})
		require.NoError(t, err)
		err = leftRouter.AddNet(leftNet)
		require.NoError(t, err)

		rightNet, err := vnet.NewNet(&vnet.NetConfig{
			StaticIPs: []string{"10.0.2.101"},
		})
		require.NoError(t, err)
		err = rightRouter.AddNet(rightNet)
		require.NoError(t, err)

		return &virtualNetwork{
			rate:       rate,
			delay:      delay,
			queueSize:  bottleneckQueueSize,
			wan:        wan,
			left:       leftNet,
			leftTBF:    leftTBF,
			leftDelay:  leftDelay,
			right:      rightNet,
			rightTBF:   rightTBF,
			rightDelay: rightDelay,
		}
	}
}

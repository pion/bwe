// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js && go1.25

package bwe_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/v3/vnet"
	"github.com/stretchr/testify/assert"
)

type virtualNetwork struct {
	wan       *vnet.Router
	left      *vnet.Net
	leftTBF   *vnet.TokenBucketFilter
	leftDelay *vnet.DelayFilter

	right      *vnet.Net
	rightTBF   *vnet.TokenBucketFilter
	rightDelay *vnet.DelayFilter
}

func (n *virtualNetwork) Close() error {
	return errors.Join(
		n.leftTBF.Close(),
		n.leftDelay.Close(),
		n.rightTBF.Close(),
		n.rightDelay.Close(),
		n.wan.Stop(),
	)
}

func createVirtualNetwork(rate, burst int, delay time.Duration) func(*testing.T) *virtualNetwork {
	return func(t *testing.T) *virtualNetwork {
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

		leftTBF, err := vnet.NewTokenBucketFilter(
			leftRouter,
			vnet.TBFRate(rate),
			vnet.TBFMaxBurst(burst),
			vnet.TBFQueueSizeInBytes(int(float64(rate)*delay.Seconds())),
		)
		assert.NoError(t, err)

		leftDelay, err := vnet.NewDelayFilter(leftTBF, delay)
		assert.NoError(t, err)

		err = wan.AddNet(leftDelay)
		assert.NoError(t, err)

		err = wan.AddChildRouter(leftRouter)
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

		rightTBF, err := vnet.NewTokenBucketFilter(
			rightRouter,
			vnet.TBFRate(rate),
			vnet.TBFMaxBurst(burst),
			vnet.TBFQueueSizeInBytes(int(float64(rate)*delay.Seconds())),
		)
		assert.NoError(t, err)

		rightDelay, err := vnet.NewDelayFilter(rightTBF, delay)
		assert.NoError(t, err)

		err = wan.AddNet(rightDelay)
		assert.NoError(t, err)

		err = wan.AddChildRouter(rightRouter)
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

		return &virtualNetwork{
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

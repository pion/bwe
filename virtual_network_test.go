// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js && go1.25

package bwe_test

import (
	"errors"
	"testing"

	"github.com/pion/logging"
	"github.com/pion/transport/v3/vnet"
	"github.com/stretchr/testify/assert"
)

type virtualNetwork struct {
	wan      *vnet.Router
	left     *vnet.Net
	leftTBF  *vnet.TokenBucketFilter
	right    *vnet.Net
	rightTBF *vnet.TokenBucketFilter
}

func (n *virtualNetwork) Close() error {
	return errors.Join(
		n.leftTBF.Close(),
		n.rightTBF.Close(),
		n.wan.Stop(),
	)
}

func createVirtualNetwork(t *testing.T) *virtualNetwork {
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

	leftTBF, err := vnet.NewTokenBucketFilter(leftRouter, vnet.TBFRate(1_000_000), vnet.TBFMaxBurst(80_000))
	assert.NoError(t, err)

	err = wan.AddNet(leftTBF)
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

	rightTBF, err := vnet.NewTokenBucketFilter(rightRouter, vnet.TBFRate(1_000_000), vnet.TBFMaxBurst(80_000))
	assert.NoError(t, err)

	err = wan.AddNet(rightTBF)
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
		wan:      wan,
		left:     leftNet,
		leftTBF:  leftTBF,
		right:    rightNet,
		rightTBF: rightTBF,
	}
}

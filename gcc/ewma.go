// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

type ewma struct {
	initialized bool
	alpha       float64
	average     float64
	variance    float64
}

func newEWMA(alpha float64) *ewma {
	return &ewma{
		initialized: false,
		alpha:       alpha,
		average:     0,
		variance:    0,
	}
}

func (a *ewma) update(sample float64) {
	if !a.initialized {
		a.initialized = true
		a.average = sample

		return
	}
	delta := sample - a.average
	a.average += a.alpha * delta
	a.variance = (1 - a.alpha) * (a.variance + a.alpha*delta*delta)
}

func (a *ewma) avg() float64 {
	return a.average
}

func (a *ewma) varr() float64 {
	return a.variance
}

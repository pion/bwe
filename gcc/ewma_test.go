// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// python to generate test cases:
// import numpy as np
// import pandas as pd
// data = np.random.randint(1, 10, size=10)
// df = pd.DataFrame(data)
// expectedAvg = df.ewm(alpha=0.9, adjust=False).mean()
// expectedVar = df.ewm(alpha=0.9, adjust=False).var(bias=True)

func TestEWMA(t *testing.T) {
	cases := []struct {
		alpha       float64
		samples     []float64
		expectedAvg []float64
		expectedVar []float64
	}{
		{
			alpha:       0.9,
			samples:     []float64{},
			expectedAvg: []float64{},
			expectedVar: []float64{},
		},
		{
			alpha:       0.95,
			samples:     []float64{0, 1, 2, 3, 4},
			expectedAvg: []float64{0, 0.95, 1.9475, 2.947375, 3.947369},
			expectedVar: []float64{0, 0.047500, 0.054744, 0.055356, 0.05539},
		},
		{
			alpha:   0.9,
			samples: []float64{1, 2, 3, 4},
			expectedAvg: []float64{
				1.000,
				1.900,
				2.890,
				3.889,
			},
			expectedVar: []float64{
				0.000000,
				0.090000,
				0.117900,
				0.122679,
			},
		},
		{
			alpha:   0.9,
			samples: []float64{8, 8, 5, 1, 3, 1, 8, 2, 8, 9},
			expectedAvg: []float64{
				8.000000,
				8.000000,
				5.300000,
				1.430000,
				2.843000,
				1.184300,
				7.318430,
				2.531843,
				7.453184,
				8.845318,
			},
			expectedVar: []float64{
				0.000000,
				0.000000,
				0.810000,
				1.745100,
				0.396351,
				0.345334,
				4.215372,
				2.967250,
				2.987792,
				0.514117,
			},
		},
		{
			alpha:   0.9,
			samples: []float64{7, 5, 6, 7, 3, 6, 8, 9, 5, 5},
			expectedAvg: []float64{
				7.000000,
				5.200000,
				5.920000,
				6.892000,
				3.389200,
				5.738920,
				7.773892,
				8.877389,
				5.387739,
				5.038774,
			},
			expectedVar: []float64{
				0.000000,
				0.360000,
				0.093600,
				0.114336,
				1.374723,
				0.750937,
				0.535217,
				0.188822,
				1.371955,
				0.150726,
			},
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			ewma := newEWMA(tc.alpha)
			for i, sample := range tc.samples {
				ewma.update(sample)
				assert.InDelta(t, tc.expectedAvg[i], ewma.avg(), 0.1)
				assert.InDelta(t, tc.expectedVar[i], ewma.varr(), 0.1)
			}
		})
	}
}

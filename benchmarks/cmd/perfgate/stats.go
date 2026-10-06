package main

import (
	"math/big"
	"sort"
)

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// slowerPValue is the exact one-sided Mann-Whitney p-value for the hypothesis
// that head samples tend to be larger (slower) than base samples. Ties count
// as half a win, which makes the exact distribution approximate when present.
func slowerPValue(base, head []float64) float64 {
	m, n := len(head), len(base)
	if m == 0 || n == 0 {
		return 1
	}
	var twiceU int
	for _, h := range head {
		for _, b := range base {
			switch {
			case h > b:
				twiceU += 2
			case h == b:
				twiceU++
			}
		}
	}

	// ways[u] = number of rankings of m head and n base samples whose U is u.
	ways := uDistribution(m, n)
	threshold := (twiceU + 1) / 2
	atLeast, total := new(big.Int), new(big.Int)
	for u, w := range ways {
		total.Add(total, w)
		if u >= threshold {
			atLeast.Add(atLeast, w)
		}
	}
	p, _ := new(big.Rat).SetFrac(atLeast, total).Float64()
	return p
}

func uDistribution(m, n int) []*big.Int {
	// f[i][j][u]: arrangements of i head and j base samples with U = u.
	f := make([][][]*big.Int, m+1)
	for i := range f {
		f[i] = make([][]*big.Int, n+1)
		for j := range f[i] {
			f[i][j] = make([]*big.Int, i*j+1)
			for u := range f[i][j] {
				f[i][j][u] = new(big.Int)
			}
			if i == 0 || j == 0 {
				f[i][j][0].SetInt64(1)
				continue
			}
			// The largest sample is either a head sample (beating all j base
			// samples) or a base sample (beating nothing).
			for u := range f[i][j] {
				if u-j >= 0 && u-j < len(f[i-1][j]) {
					f[i][j][u].Add(f[i][j][u], f[i-1][j][u-j])
				}
				if u < len(f[i][j-1]) {
					f[i][j][u].Add(f[i][j][u], f[i][j-1][u])
				}
			}
		}
	}
	return f[m][n]
}

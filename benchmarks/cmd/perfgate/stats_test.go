package main

import (
	"math"
	"testing"
)

func TestMedian(t *testing.T) {
	for _, tc := range []struct {
		in   []float64
		want float64
	}{
		{[]float64{3, 1, 2}, 2},
		{[]float64{4, 1, 3, 2}, 2.5},
		{[]float64{7}, 7},
	} {
		if got := median(tc.in); got != tc.want {
			t.Errorf("median(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSlowerPValue(t *testing.T) {
	base := []float64{10, 11, 12, 13, 14}
	for _, tc := range []struct {
		name string
		head []float64
		want float64
	}{
		{"all head slower", []float64{20, 21, 22, 23, 24}, 1.0 / 252},
		{"all head faster", []float64{1, 2, 3, 4, 5}, 1},
		{"one base sample beats one head sample", []float64{13.5, 21, 22, 23, 24}, 2.0 / 252},
		{"identical", []float64{10, 11, 12, 13, 14}, 0.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := slowerPValue(base, tc.head)
			if tc.name == "identical" {
				if got < 0.4 || got > 0.7 {
					t.Fatalf("p = %v, want about 0.5", got)
				}
				return
			}
			if math.Abs(got-tc.want) > 1e-12 {
				t.Fatalf("p = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUDistributionSumsToBinomial(t *testing.T) {
	var total int64
	for _, w := range uDistribution(5, 5) {
		total += w.Int64()
	}
	if total != 252 {
		t.Fatalf("total arrangements = %d, want 252", total)
	}
}

func TestAssignShards(t *testing.T) {
	names := []string{
		"BenchmarkA_50K", "BenchmarkB_50K", "BenchmarkC_10K", "BenchmarkD",
		"BenchmarkE", "BenchmarkF_10K", "BenchmarkG", "BenchmarkH_50K_Indexed",
	}
	seen := map[string]int{}
	var weights [3]int
	for shard := 0; shard < 3; shard++ {
		for _, n := range assignShard(names, shard, 3) {
			seen[n]++
			weights[shard] += benchWeight(n)
		}
	}
	for _, n := range names {
		if seen[n] != 1 {
			t.Fatalf("%s assigned %d times", n, seen[n])
		}
	}
	for shard, w := range weights {
		if w > 9 {
			t.Fatalf("shard %d weight %d, shards %v are unbalanced", shard, w, weights)
		}
	}
}

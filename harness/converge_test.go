// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package harness

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOpTime_OrdersByTermThenTimestamp(t *testing.T) {
	cases := []struct {
		name string
		a, b OpTime
		want int
	}{
		{"equal", OpTime{10, 1, 1}, OpTime{10, 1, 1}, 0},
		{"later increment", OpTime{10, 2, 1}, OpTime{10, 1, 1}, 1},
		{"later seconds", OpTime{11, 1, 1}, OpTime{10, 9, 1}, 1},
		{"higher term beats later timestamp", OpTime{1, 1, 2}, OpTime{999, 999, 1}, 1},
		{"lower term loses despite later timestamp", OpTime{999, 999, 1}, OpTime{1, 1, 2}, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.a.Compare(c.b); got != c.want {
				t.Errorf("%s.Compare(%s) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestConvergenceTimeout_ExplainsWhoIsBehind(t *testing.T) {
	watermark := OpTime{Seconds: 100, Increment: 1, Term: 1}
	addrs := []string{"127.0.0.1:1", "127.0.0.1:2", "127.0.0.1:3"}
	last := map[string]memberSample{
		"127.0.0.1:1": {progress: MemberProgress{Addr: "127.0.0.1:1", State: StateSecondary, Applied: watermark}},
		"127.0.0.1:2": {progress: MemberProgress{Addr: "127.0.0.1:2", State: StateSecondary, Applied: OpTime{Seconds: 70, Increment: 1, Term: 1}}},
		"127.0.0.1:3": {progress: MemberProgress{Addr: "127.0.0.1:3", State: StateStartup2}},
	}

	err := convergenceTimeout(watermark, 30*time.Second, addrs, last)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()

	if !contains(msg, "not in the same pass") {
		t.Errorf("a member that reached the watermark should be explained, not omitted:\n%s", msg)
	}
	if !contains(msg, "30 seconds behind") {
		t.Errorf("a lagging member should report how far behind it is:\n%s", msg)
	}
	if !contains(msg, "NO progress at all") {
		t.Errorf("a member reporting no progress should be distinguished from a lagging one:\n%s", msg)
	}
	if !contains(msg, StateStartup2) {
		t.Errorf("the stalled member's state should be named:\n%s", msg)
	}
}

func TestConvergenceTimeout_ReportsUnreachableMembers(t *testing.T) {
	err := convergenceTimeout(OpTime{Seconds: 5, Term: 1}, time.Second,
		[]string{"127.0.0.1:9"}, map[string]memberSample{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !contains(err.Error(), "never sampled") {
		t.Errorf("an unsampled member should be reported as such: %v", err)
	}
}

func TestConvergenceTimeout_NamesAMemberThatStoppedAnswering(t *testing.T) {
	watermark := OpTime{Seconds: 100, Increment: 1, Term: 1}
	last := map[string]memberSample{
		"127.0.0.1:1": {
			progress: MemberProgress{Addr: "127.0.0.1:1", State: StateSecondary, Applied: watermark},
			err:      context.DeadlineExceeded,
		},
	}
	err := convergenceTimeout(watermark, 30*time.Second, []string{"127.0.0.1:1"}, last)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !contains(msg, "not answering") {
		t.Errorf("should report that the member stopped answering:\n%s", msg)
	}
	if !contains(msg, "last known position") {
		t.Errorf("should report the last known position:\n%s", msg)
	}
	if contains(msg, "harness bug") {
		t.Errorf("report should not have fallen through to the empty-list guard:\n%s", msg)
	}
}

func TestConvergenceTimeout_DistinguishesStuckFromSlow(t *testing.T) {
	watermark := OpTime{Seconds: 100, Term: 1}
	behind := MemberProgress{Applied: OpTime{Seconds: 90, Term: 1}, State: StateSecondary}

	stuck := map[string]memberSample{
		"127.0.0.1:1": {progress: behind, first: behind.Applied, samples: 40},
	}
	if got := convergenceTimeout(watermark, 30*time.Second, []string{"127.0.0.1:1"}, stuck).Error(); !strings.Contains(got, "stuck rather than slow") {
		t.Errorf("a member that never advanced was not reported as stuck: %s", got)
	}

	slow := map[string]memberSample{
		"127.0.0.1:1": {progress: behind, first: OpTime{Seconds: 50, Term: 1}, samples: 40},
	}
	got := convergenceTimeout(watermark, 30*time.Second, []string{"127.0.0.1:1"}, slow).Error()
	if !strings.Contains(got, "slow rather than stuck") {
		t.Errorf("a member that advanced was not reported as slow: %s", got)
	}
	if !strings.Contains(got, "{ts:50.0 t:1}") {
		t.Errorf("the slow report does not say where it started: %s", got)
	}
}

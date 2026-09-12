// Copyright 2018 Google Inc. All rights reserved.
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

package terminal

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"android/soong/ui/status"
)

func TestStatusOutput(t *testing.T) {
	tests := []struct {
		name   string
		calls  func(stat status.StatusOutput)
		smart  string
		simple string
	}{
		{
			name:   "two actions",
			calls:  twoActions,
			smart:  "\r\x1b[1m[  0% 0/2] action1\x1b[0m\x1b[K\r\x1b[1m[ 50% 1/2] action1\x1b[0m\x1b[K\r\x1b[1m[ 50% 1/2] action2\x1b[0m\x1b[K\r\x1b[1m[100% 2/2] action2\x1b[0m\x1b[K\n",
			simple: "[ 50% 1/2] action1\n[100% 2/2] action2\n",
		},
		{
			name:   "two parallel actions",
			calls:  twoParallelActions,
			smart:  "\r\x1b[1m[  0% 0/2] action1\x1b[0m\x1b[K\r\x1b[1m[  0% 0/2] action2\x1b[0m\x1b[K\r\x1b[1m[ 50% 1/2] action1\x1b[0m\x1b[K\r\x1b[1m[100% 2/2] action2\x1b[0m\x1b[K\n",
			simple: "[ 50% 1/2] action1\n[100% 2/2] action2\n",
		},
		{
			name:   "action with output",
			calls:  actionsWithOutput,
			smart:  "\r\x1b[1m[  0% 0/3] action1\x1b[0m\x1b[K\r\x1b[1m[ 33% 1/3] action1\x1b[0m\x1b[K\r\x1b[1m[ 33% 1/3] action2\x1b[0m\x1b[K\r\x1b[1m[ 66% 2/3] action2\x1b[0m\x1b[K\noutput1\noutput2\n\r\x1b[1m[ 66% 2/3] action3\x1b[0m\x1b[K\r\x1b[1m[100% 3/3] action3\x1b[0m\x1b[K\n",
			simple: "[ 33% 1/3] action1\n[ 66% 2/3] action2\noutput1\noutput2\n[100% 3/3] action3\n",
		},
		{
			name:   "action with output without newline",
			calls:  actionsWithOutputWithoutNewline,
			smart:  "\r\x1b[1m[  0% 0/3] action1\x1b[0m\x1b[K\r\x1b[1m[ 33% 1/3] action1\x1b[0m\x1b[K\r\x1b[1m[ 33% 1/3] action2\x1b[0m\x1b[K\r\x1b[1m[ 66% 2/3] action2\x1b[0m\x1b[K\noutput1\noutput2\n\r\x1b[1m[ 66% 2/3] action3\x1b[0m\x1b[K\r\x1b[1m[100% 3/3] action3\x1b[0m\x1b[K\n",
			simple: "[ 33% 1/3] action1\n[ 66% 2/3] action2\noutput1\noutput2\n[100% 3/3] action3\n",
		},
		{
			name:   "action with error",
			calls:  actionsWithError,
			smart:  "\r\x1b[1m[  0% 0/3] action1\x1b[0m\x1b[K\r\x1b[1m[ 33% 1/3] action1\x1b[0m\x1b[K\r\x1b[1m[ 33% 1/3] action2\x1b[0m\x1b[K\r\x1b[1m[ 66% 2/3] action2\x1b[0m\x1b[K\nFAILED: f1 f2\ntouch f1 f2\nerror1\nerror2\n\r\x1b[1m[ 66% 2/3] action3\x1b[0m\x1b[K\r\x1b[1m[100% 3/3] action3\x1b[0m\x1b[K\n",
			simple: "[ 33% 1/3] action1\n[ 66% 2/3] action2\nFAILED: f1 f2\ntouch f1 f2\nerror1\nerror2\n[100% 3/3] action3\n",
		},
		{
			name:   "action with empty description",
			calls:  actionWithEmptyDescription,
			smart:  "\r\x1b[1m[  0% 0/1] command1\x1b[0m\x1b[K\r\x1b[1m[100% 1/1] command1\x1b[0m\x1b[K\n",
			simple: "[100% 1/1] command1\n",
		},
		{
			name:   "messages",
			calls:  actionsWithMessages,
			smart:  "\r\x1b[1m[  0% 0/2] action1\x1b[0m\x1b[K\r\x1b[1m[ 50% 1/2] action1\x1b[0m\x1b[K\r\x1b[1mstatus\x1b[0m\x1b[K\r\x1b[Kprint\nFAILED: error\n\r\x1b[1m[ 50% 1/2] action2\x1b[0m\x1b[K\r\x1b[1m[100% 2/2] action2\x1b[0m\x1b[K\n",
			simple: "[ 50% 1/2] action1\nstatus\nprint\nFAILED: error\n[100% 2/2] action2\n",
		},
		{
			name:   "action with long description",
			calls:  actionWithLongDescription,
			smart:  "\r\x1b[1m[  0% 0/2] action with very long descrip\x1b[0m\x1b[K\r\x1b[1m[ 50% 1/2] action with very long descrip\x1b[0m\x1b[K\n",
			simple: "[ 50% 1/2] action with very long description to test eliding\n",
		},
		{
			name:   "action with output with ansi codes",
			calls:  actionWithOuptutWithAnsiCodes,
			smart:  "\r\x1b[1m[  0% 0/1] action1\x1b[0m\x1b[K\r\x1b[1m[100% 1/1] action1\x1b[0m\x1b[K\n\x1b[31mcolor\x1b[0m\n",
			simple: "[100% 1/1] action1\ncolor\n",
		},
	}

	os.Setenv(tableHeightEnVar, "")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			t.Run("smart", func(t *testing.T) {
				smart := &fakeSmartTerminal{termWidth: 40}
				stat := NewStatusOutput(smart, "", false, false)
				tt.calls(stat)
				stat.Flush()

				if g, w := smart.String(), tt.smart; g != w {
					t.Errorf("want:\n%q\ngot:\n%q", w, g)
				}
			})

			t.Run("simple", func(t *testing.T) {
				simple := &bytes.Buffer{}
				stat := NewStatusOutput(simple, "", false, false)
				tt.calls(stat)
				stat.Flush()

				if g, w := simple.String(), tt.simple; g != w {
					t.Errorf("want:\n%q\ngot:\n%q", w, g)
				}
			})

			t.Run("force simple", func(t *testing.T) {
				smart := &fakeSmartTerminal{termWidth: 40}
				stat := NewStatusOutput(smart, "", true, false)
				tt.calls(stat)
				stat.Flush()

				if g, w := smart.String(), tt.simple; g != w {
					t.Errorf("want:\n%q\ngot:\n%q", w, g)
				}
			})
		})
	}
}

type runner struct {
	counts status.Counts
	stat   status.StatusOutput
}

func newRunner(stat status.StatusOutput, totalActions int) *runner {
	return &runner{
		counts: status.Counts{TotalActions: totalActions},
		stat:   stat,
	}
}

func (r *runner) startAction(action *status.Action) {
	r.counts.StartedActions++
	r.counts.RunningActions++
	r.stat.StartAction(action, r.counts)
}

func (r *runner) finishAction(result status.ActionResult) {
	r.counts.FinishedActions++
	r.counts.RunningActions--
	r.stat.FinishAction(result, r.counts)
}

func (r *runner) finishAndStartAction(result status.ActionResult, action *status.Action) {
	r.counts.FinishedActions++
	r.stat.FinishAction(result, r.counts)

	r.counts.StartedActions++
	r.stat.StartAction(action, r.counts)
}

var (
	action1 = &status.Action{Description: "action1"}
	result1 = status.ActionResult{Action: action1}
	action2 = &status.Action{Description: "action2"}
	result2 = status.ActionResult{Action: action2}
	action3 = &status.Action{Description: "action3"}
	result3 = status.ActionResult{Action: action3}
)

func twoActions(stat status.StatusOutput) {
	runner := newRunner(stat, 2)
	runner.startAction(action1)
	runner.finishAction(result1)
	runner.startAction(action2)
	runner.finishAction(result2)
}

func twoParallelActions(stat status.StatusOutput) {
	runner := newRunner(stat, 2)
	runner.startAction(action1)
	runner.startAction(action2)
	runner.finishAction(result1)
	runner.finishAction(result2)
}

func actionsWithOutput(stat status.StatusOutput) {
	result2WithOutput := status.ActionResult{Action: action2, Output: "output1\noutput2\n"}

	runner := newRunner(stat, 3)
	runner.startAction(action1)
	runner.finishAction(result1)
	runner.startAction(action2)
	runner.finishAction(result2WithOutput)
	runner.startAction(action3)
	runner.finishAction(result3)
}

func actionsWithOutputWithoutNewline(stat status.StatusOutput) {
	result2WithOutputWithoutNewline := status.ActionResult{Action: action2, Output: "output1\noutput2"}

	runner := newRunner(stat, 3)
	runner.startAction(action1)
	runner.finishAction(result1)
	runner.startAction(action2)
	runner.finishAction(result2WithOutputWithoutNewline)
	runner.startAction(action3)
	runner.finishAction(result3)
}

func actionsWithError(stat status.StatusOutput) {
	action2WithError := &status.Action{Description: "action2", Outputs: []string{"f1", "f2"}, Command: "touch f1 f2"}
	result2WithError := status.ActionResult{Action: action2WithError, Output: "error1\nerror2\n", Error: fmt.Errorf("error1")}

	runner := newRunner(stat, 3)
	runner.startAction(action1)
	runner.finishAction(result1)
	runner.startAction(action2WithError)
	runner.finishAction(result2WithError)
	runner.startAction(action3)
	runner.finishAction(result3)
}

func actionWithEmptyDescription(stat status.StatusOutput) {
	action1 := &status.Action{Command: "command1"}
	result1 := status.ActionResult{Action: action1}

	runner := newRunner(stat, 1)
	runner.startAction(action1)
	runner.finishAction(result1)
}

func actionsWithMessages(stat status.StatusOutput) {
	runner := newRunner(stat, 2)

	runner.startAction(action1)
	runner.finishAction(result1)

	stat.Message(status.VerboseLvl, "verbose")
	stat.Message(status.StatusLvl, "status")
	stat.Message(status.PrintLvl, "print")
	stat.Message(status.ErrorLvl, "error")

	runner.startAction(action2)
	runner.finishAction(result2)
}

func actionWithLongDescription(stat status.StatusOutput) {
	action1 := &status.Action{Description: "action with very long description to test eliding"}
	result1 := status.ActionResult{Action: action1}

	runner := newRunner(stat, 2)

	runner.startAction(action1)

	runner.finishAction(result1)
}

func actionWithOuptutWithAnsiCodes(stat status.StatusOutput) {
	result1WithOutputWithAnsiCodes := status.ActionResult{Action: action1, Output: "\x1b[31mcolor\x1b[0m"}

	runner := newRunner(stat, 1)
	runner.startAction(action1)
	runner.finishAction(result1WithOutputWithAnsiCodes)
}

func TestSmartStatusOutputWidthChange(t *testing.T) {
	os.Setenv(tableHeightEnVar, "")

	smart := &fakeSmartTerminal{termWidth: 40}
	stat := NewStatusOutput(smart, "", false, false)
	smartStat := stat.(*smartStatusOutput)
	smartStat.sigwinchHandled = make(chan bool)

	runner := newRunner(stat, 2)

	action := &status.Action{Description: "action with very long description to test eliding"}
	result := status.ActionResult{Action: action}

	runner.startAction(action)
	smart.termWidth = 30
	// Fake a SIGWINCH
	smartStat.sigwinch <- syscall.SIGWINCH
	<-smartStat.sigwinchHandled
	runner.finishAction(result)

	stat.Flush()

	w := "\r\x1b[1m[  0% 0/2] action with very long descrip\x1b[0m\x1b[K\r\x1b[1m[ 50% 1/2] action with very lo\x1b[0m\x1b[K\n"

	if g := smart.String(); g != w {
		t.Errorf("want:\n%q\ngot:\n%q", w, g)
	}
}

func TestRemainingTimeString(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{45 * time.Second, "45s remaining"},
		{12*time.Minute + 34*time.Second, "12m34s remaining"},
		{1*time.Hour + 4*time.Minute + 20*time.Second, "1h04m20s remaining"},
		{0, "0s remaining"},
	}
	for _, tt := range tests {
		if got := remainingTimeString(tt.d); got != tt.expected {
			t.Errorf("remainingTimeString(%v) = %q, want %q", tt.d, got, tt.expected)
		}
	}
}

func TestEtaEstimator(t *testing.T) {
	f := newFormatter("", false)
	simTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	f.eta.nowFunc = func() time.Time { return simTime }

	// 1. Small actions during warmup should not output ETA, but records progress
	if eta, ok := f.eta.estimateRemaining(status.Counts{StartedActions: 5, FinishedActions: 5, TotalActions: 100}); ok {
		t.Errorf("expected no ETA for 5 actions, got %q", eta)
	}

	// 2. Warm-up rollback: if finished actions decreases during warm-up, estimator resets
	f.eta.estimateRemaining(status.Counts{StartedActions: 10, FinishedActions: 10, TotalActions: 100})
	if f.eta.firstActionTime.IsZero() {
		t.Errorf("expected firstActionTime to be set during warmup")
	}
	f.eta.estimateRemaining(status.Counts{StartedActions: 2, FinishedActions: 2, TotalActions: 100})
	if !f.eta.firstActionTime.IsZero() {
		t.Errorf("expected warm-up rollback to reset firstActionTime")
	}

	// 3. Initial valid estimate: phase starts with action 1 starting at simTime
	f.eta.estimateRemaining(status.Counts{StartedActions: 1, FinishedActions: 0, TotalActions: 100})
	// Advance simulated time by 20s with 50/100 actions finished (rate = 50 / 20 = 2.5 actions/s)
	// Remaining: 50 actions / 2.5 = 20s
	simTime = simTime.Add(20 * time.Second)
	eta1, ok1 := f.eta.estimateRemaining(status.Counts{StartedActions: 60, FinishedActions: 50, TotalActions: 100})
	if !ok1 || !strings.Contains(eta1, "remaining") {
		t.Errorf("expected valid first ETA estimate, got %q (ok=%v)", eta1, ok1)
	}
	if f.eta.lastRemaining != 20*time.Second {
		t.Errorf("expected initial remaining to be 20s, got %v", f.eta.lastRemaining)
	}
	initialRemaining := f.eta.lastRemaining

	// 4. Deterministic anti-jitter countdown: advance time by 5 seconds with NO new actions completed.
	// The remaining duration MUST decrease deterministically by exactly 5 seconds (20s - 5s = 15s).
	simTime = simTime.Add(5 * time.Second)
	eta2, ok2 := f.eta.estimateRemaining(status.Counts{StartedActions: 60, FinishedActions: 50, TotalActions: 100})
	if !ok2 || !strings.Contains(eta2, "remaining") {
		t.Errorf("expected valid second estimate during countdown, got %q (ok=%v)", eta2, ok2)
	}
	expectedRemaining := initialRemaining - 5*time.Second
	if f.eta.lastRemaining != expectedRemaining {
		t.Errorf("expected countdown duration to decrease to %v, got %v", expectedRemaining, f.eta.lastRemaining)
	}

	// 5. TotalActions growth during long-running action:
	// New targets are discovered (100 -> 150), remaining actions jump from 50 to 100.
	// Raw duration = 100 / 2.5 = 40s.
	// Estimator must smooth upward toward the new workload instead of continuing downward.
	simTime = simTime.Add(1 * time.Second)
	f.eta.estimateRemaining(status.Counts{StartedActions: 60, FinishedActions: 50, TotalActions: 150})
	if f.eta.lastRemaining <= expectedRemaining-1*time.Second {
		t.Errorf("expected remaining duration to increase when TotalActions expands, got %v", f.eta.lastRemaining)
	}

	// 6. Rolling window blend: replace samples in strict chronological order older than simTime.
	// Window: 10s duration, 40 actions finished -> recentRate = 4.0 actions/s
	// Blended rate = 0.70 * 4.0 + 0.30 * 2.8 = 3.64 actions/s
	f.eta.samples = []rateSample{
		{t: simTime.Add(-10 * time.Second), finished: 30},
		{t: simTime.Add(-5 * time.Second), finished: 55},
	}
	simTime = simTime.Add(3 * time.Second)
	eta3, ok3 := f.eta.estimateRemaining(status.Counts{StartedActions: 75, FinishedActions: 70, TotalActions: 100})
	if !ok3 || !strings.Contains(eta3, "remaining") {
		t.Errorf("expected valid third estimate with rolling rate, got %q (ok=%v)", eta3, ok3)
	}
	if f.eta.lastRemaining < 5*time.Second || f.eta.lastRemaining > 20*time.Second {
		t.Errorf("expected blended remaining duration in bounded range [5s, 20s], got %v", f.eta.lastRemaining)
	}

	// 7. Late-stage weighting: >80% progress (e.g., 95/100).
	// Progress is 95%, weight is 1.0 + (0.95-0.80)*3.3 = 1.495.
	simTime = simTime.Add(2 * time.Second)
	etaLate, okLate := f.eta.estimateRemaining(status.Counts{StartedActions: 96, FinishedActions: 95, TotalActions: 100})
	if !okLate || !strings.Contains(etaLate, "remaining") {
		t.Errorf("expected valid late-stage estimate, got %q (ok=%v)", etaLate, okLate)
	}

	// 8. Phase completion reset: 100/100 actions resets the estimator state.
	etaDone, okDone := f.eta.estimateRemaining(status.Counts{StartedActions: 100, FinishedActions: 100, TotalActions: 100})
	if okDone || etaDone != "" {
		t.Errorf("expected no ETA when total actions completed, got %q", etaDone)
	}
	if f.eta.initialized || len(f.eta.samples) != 0 || !f.eta.firstActionTime.IsZero() {
		t.Errorf("expected estimator state to be completely reset on phase completion")
	}

	// 9. Subsequent phase baseline tracking:
	// New phase arrives with cumulative counts 105/200 started, 100 finished.
	// Estimator should set baseFinishedActions to 100, so subsequent rate uses phase delta.
	simTime = simTime.Add(5 * time.Second)
	f.eta.estimateRemaining(status.Counts{StartedActions: 105, FinishedActions: 100, TotalActions: 200})
	if f.eta.baseFinishedActions != 100 {
		t.Errorf("expected baseFinishedActions to be 100, got %d", f.eta.baseFinishedActions)
	}
	simTime = simTime.Add(15 * time.Second)
	etaPhase2, okPhase2 := f.eta.estimateRemaining(status.Counts{StartedActions: 140, FinishedActions: 130, TotalActions: 200})
	if !okPhase2 || !strings.Contains(etaPhase2, "remaining") {
		t.Errorf("expected valid phase 2 estimate, got %q (ok=%v)", etaPhase2, okPhase2)
	}
}

func TestEtaFormatPlaceholder(t *testing.T) {
	// 1. Warm-up fallback: should output '?' for %l
	f := newFormatter("[%l]", false)
	got := f.progress(status.Counts{FinishedActions: 2, TotalActions: 100})
	if got != "[?]" {
		t.Errorf("expected '[?]' during warm-up, got %q", got)
	}

	// 2. Active estimate: should output formatted remaining time
	simTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	f.eta.nowFunc = func() time.Time { return simTime }
	f.eta.firstActionTime = simTime.Add(-25 * time.Second)
	gotActive := f.progress(status.Counts{FinishedActions: 40, TotalActions: 100})
	if !strings.Contains(gotActive, "remaining") {
		t.Errorf("expected active ETA in placeholder, got %q", gotActive)
	}
}

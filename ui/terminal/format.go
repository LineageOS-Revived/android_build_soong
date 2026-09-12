// Copyright 2019 Google Inc. All rights reserved.
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
	"fmt"
	"strings"
	"sync"
	"time"

	"android/soong/ui/status"
)

type rateSample struct {
	t        time.Time
	finished int
}

type etaEstimator struct {
	sync.Mutex
	samples        []rateSample
	lastUpdateTime time.Time
	lastRemaining  time.Duration
	initialized    bool
}

func newEtaEstimator() *etaEstimator {
	return &etaEstimator{
		samples: make([]rateSample, 0, 64),
	}
}

func (e *etaEstimator) estimateRemaining(counts status.Counts, start time.Time) (string, bool) {
	if e == nil {
		return "", false
	}
	now := time.Now()
	elapsed := now.Sub(start)

	// Don't show ETA during early warm-up or when nearly done / empty
	if counts.FinishedActions <= 15 || counts.TotalActions <= counts.FinishedActions || elapsed.Seconds() < 5.0 {
		return "", false
	}

	e.Lock()
	defer e.Unlock()

	// Append sample
	e.samples = append(e.samples, rateSample{t: now, finished: counts.FinishedActions})

	// Prune samples older than 25 seconds
	cutoff := now.Add(-25 * time.Second)
	idx := 0
	for idx < len(e.samples) && e.samples[idx].t.Before(cutoff) {
		idx++
	}
	if idx > 0 && idx < len(e.samples) {
		e.samples = e.samples[idx:]
	}

	// 1. Overall cumulative rate
	overallRate := float64(counts.FinishedActions) / elapsed.Seconds()
	if overallRate <= 0 {
		return "", false
	}

	// 2. Rolling window rate
	blendedRate := overallRate
	if len(e.samples) >= 2 {
		oldest := e.samples[0]
		winDuration := now.Sub(oldest.t).Seconds()
		winActions := counts.FinishedActions - oldest.finished
		if winDuration >= 3.0 && winActions > 0 {
			recentRate := float64(winActions) / winDuration
			// Weight recent rate 70% and overall rate 30%
			blendedRate = 0.70*recentRate + 0.30*overallRate
		}
	}

	remainingActions := float64(counts.TotalActions - counts.FinishedActions)
	if remainingActions <= 0 {
		return "", false
	}

	// 3. Late-stage non-linear scaling:
	// Actions near the end of an Android build (packaging, dexing, brotli, images)
	// take significantly longer per action than early compilation tasks.
	progressRatio := float64(counts.FinishedActions) / float64(counts.TotalActions)
	weight := 1.0
	if progressRatio > 0.80 {
		// Gradually increase weight from 1.0 at 80% to 1.6 at 98%
		weight = 1.0 + (progressRatio-0.80)*3.3
	}

	rawSecs := (remainingActions * weight) / blendedRate
	if rawSecs < 1 {
		rawSecs = 1
	}
	rawDuration := time.Duration(rawSecs) * time.Second

	// 4. Smooth clock countdown (anti-jitter)
	var finalDuration time.Duration
	if !e.initialized {
		finalDuration = rawDuration
		e.initialized = true
		e.lastRemaining = finalDuration
		e.lastUpdateTime = now
	} else {
		dt := now.Sub(e.lastUpdateTime)
		expected := e.lastRemaining - dt
		if expected < time.Second {
			expected = time.Second
		}

		// Exponential Moving Average smoothing on duration:
		// 80% weight on expected countdown, 20% on new instantaneous estimate
		alpha := 0.20
		smoothedSecs := float64(expected.Seconds())*(1.0-alpha) + float64(rawDuration.Seconds())*alpha
		if smoothedSecs < 1.0 {
			smoothedSecs = 1.0
		}
		finalDuration = time.Duration(smoothedSecs) * time.Second
		e.lastRemaining = finalDuration
		e.lastUpdateTime = now
	}

	return remainingTimeString(finalDuration), true
}

type formatter struct {
	format string
	quiet  bool
	start  time.Time
	eta    *etaEstimator
}

// newFormatter returns a formatter for formatting output to
// the terminal in a format similar to Ninja.
// format takes nearly all the same options as NINJA_STATUS.
// %c is currently unsupported.
func newFormatter(format string, quiet bool) formatter {
	return formatter{
		format: format,
		quiet:  quiet,
		start:  time.Now(),
		eta:    newEtaEstimator(),
	}
}

func (s formatter) message(level status.MsgLevel, message string) string {
	if level >= status.ErrorLvl {
		return fmt.Sprintf("FAILED: %s", message)
	} else if level > status.StatusLvl {
		return fmt.Sprintf("%s%s", level.Prefix(), message)
	} else if level == status.StatusLvl {
		return message
	}
	return ""
}

func remainingTimeString(duration time.Duration) string {
	duration = duration.Round(time.Second)
	if duration < 0 {
		duration = 0
	}
	h := duration / time.Hour
	duration -= h * time.Hour
	m := duration / time.Minute
	duration -= m * time.Minute
	s := duration / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds remaining", h, m, s)
	} else if m > 0 {
		return fmt.Sprintf("%dm%02ds remaining", m, s)
	}
	return fmt.Sprintf("%ds remaining", s)
}

func (s formatter) progress(counts status.Counts) string {
	if s.format == "" {
		percent := 0
		if counts.TotalActions > 0 {
			percent = 100 * counts.FinishedActions / counts.TotalActions
		}
		prefix := fmt.Sprintf("[%3d%% %d/%d", percent, counts.FinishedActions, counts.TotalActions)

		if s.eta != nil {
			if etaStr, ok := s.eta.estimateRemaining(counts, s.start); ok {
				prefix += " " + etaStr
			}
		}
		return prefix + "] "
	}

	buf := &strings.Builder{}
	for i := 0; i < len(s.format); i++ {
		c := s.format[i]
		if c != '%' {
			buf.WriteByte(c)
			continue
		}

		i = i + 1
		if i == len(s.format) {
			buf.WriteByte(c)
			break
		}

		c = s.format[i]
		switch c {
		case '%':
			buf.WriteByte(c)
		case 's':
			fmt.Fprintf(buf, "%d", counts.StartedActions)
		case 't':
			fmt.Fprintf(buf, "%d", counts.TotalActions)
		case 'r':
			fmt.Fprintf(buf, "%d", counts.RunningActions)
		case 'u':
			fmt.Fprintf(buf, "%d", counts.TotalActions-counts.StartedActions)
		case 'f':
			fmt.Fprintf(buf, "%d", counts.FinishedActions)
		case 'o':
			fmt.Fprintf(buf, "%.1f", float64(counts.FinishedActions)/time.Since(s.start).Seconds())
		case 'c':
			// TODO: implement?
			buf.WriteRune('?')
		case 'p':
			fmt.Fprintf(buf, "%3d%%", 100*counts.FinishedActions/counts.TotalActions)
		case 'e':
			fmt.Fprintf(buf, "%.3f", time.Since(s.start).Seconds())
		case 'l':
			if s.eta != nil {
				if etaStr, ok := s.eta.estimateRemaining(counts, s.start); ok {
					buf.WriteString(etaStr)
				} else {
					buf.WriteRune('?')
				}
			} else {
				buf.WriteRune('?')
			}
		default:
			buf.WriteString("unknown placeholder '")
			buf.WriteByte(c)
			buf.WriteString("'")
		}
	}
	return buf.String()
}

func (s formatter) result(result status.ActionResult) string {
	var ret string
	if result.Error != nil {
		targets := strings.Join(result.Outputs, " ")
		if s.quiet || result.Command == "" {
			ret = fmt.Sprintf("FAILED: %s\n%s", targets, result.Output)
		} else {
			ret = fmt.Sprintf("FAILED: %s\n%s\n%s", targets, result.Command, result.Output)
		}
	} else if result.Output != "" {
		ret = result.Output
	}

	if len(ret) > 0 && ret[len(ret)-1] != '\n' {
		ret += "\n"
	}

	return ret
}

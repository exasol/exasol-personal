// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner_test

import (
	"bytes"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/exasol/exasol-personal/internal/util"
	"github.com/stretchr/testify/require"
)

func writeRegexLogger(t *testing.T, regexLogger io.Writer, data []byte) {
	t.Helper()

	count, err := regexLogger.Write(data)
	require.NoError(t, err)
	require.Equal(t, len(data), count)
}

type patternSpec struct {
	regex   string
	message string
}

type writeExpectation struct {
	data        string
	wantWrapped string
	wantLog     string
}

type regexLoggerTestCase struct {
	name     string
	patterns []patternSpec
	node     string
	writes   []writeExpectation
}

func buildRegexLogs(patterns []patternSpec) []*presets.RegexLog {
	logs := make([]*presets.RegexLog, len(patterns))
	for i, p := range patterns {
		logs[i] = &presets.RegexLog{
			Regex:         p.regex,
			Message:       p.message,
			CompiledRegex: regexp.MustCompile(p.regex),
		}
	}

	return logs
}

//nolint:paralleltest // Not parallel due to overriding slog
func TestRegexLogger(t *testing.T) {
	// slog.SetLogLoggerLevel(slog.LevelDebug)
	cases := []regexLoggerTestCase{
		{
			name: "waitsForFullLine",
			patterns: []patternSpec{{
				regex:   ".*",
				message: "any line",
			}},
			writes: []writeExpectation{
				{data: "HelloWorld", wantWrapped: "HelloWorld", wantLog: ""},
				{data: "\n", wantWrapped: "HelloWorld\n", wantLog: "any line"},
			},
		},
		{
			name: "multipleLinesInSingleChunk",
			patterns: []patternSpec{{
				regex:   ".*",
				message: "any line",
			}},
			writes: []writeExpectation{
				{
					data:        strings.Repeat("\n", 3),
					wantWrapped: strings.Repeat("\n", 3),
					wantLog:     strings.Repeat("any line", 3),
				},
			},
		},
		{
			name: "carriesRemaindersAcrossWrites",
			patterns: []patternSpec{{
				regex:   "specific_message",
				message: "specific_message",
			}},
			writes: []writeExpectation{
				{
					data:        "specific_message\nspecific_",
					wantWrapped: "specific_message\nspecific_",
					wantLog:     "specific_message",
				},
				{
					data:        "message\n",
					wantWrapped: "specific_message\nspecific_message\n",
					wantLog:     "specific_messagespecific_message",
				},
			},
		},
		{
			name: "expandsCapturedGroups",
			patterns: []patternSpec{{
				regex:   `^STATUS: (\S+) (\S+) - (.+)`,
				message: "Phase: $1, State: $2, Detail: $3",
			}},
			writes: []writeExpectation{
				{
					data:        "STATUS: prepare in_progress - Creating directories\n",
					wantWrapped: "STATUS: prepare in_progress - Creating directories\n",
					wantLog:     "Phase: prepare, State: in_progress, Detail: Creating directories",
				},
			},
		},
		{
			name: "capturesWholeLine",
			patterns: []patternSpec{{
				regex:   "^STATUS:.*",
				message: "$0",
			}},
			writes: []writeExpectation{
				{
					data:        "STATUS: prepare in_progress - Creating directories\n",
					wantWrapped: "STATUS: prepare in_progress - Creating directories\n",
					wantLog:     "STATUS: prepare in_progress - Creating directories",
				},
			},
		},
		{
			name: "addsNodePrefix",
			node: "n11",
			patterns: []patternSpec{{
				regex:   `.*SERVICE-EVENT: (.+)`,
				message: "$1",
			}},
			writes: []writeExpectation{
				{
					data:        "2025-11-29T00:00:00+0000: SERVICE-EVENT: Starting worker...\n",
					wantWrapped: "2025-11-29T00:00:00+0000: SERVICE-EVENT: Starting worker...\n",
					wantLog:     "host(n11): Starting worker...",
				},
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var wrapped bytes.Buffer
			logsChan := make(chan slog.Record, 100)

			restore := ReplaceLogger(t, logsChan)
			defer restore()

			patterns := buildRegexLogs(testCase.patterns)

			var regexWriter task_runner.RegexLogger
			if testCase.node != "" {
				regexWriter = task_runner.NewRegexLoggerWithNode(patterns, testCase.node)
			} else {
				regexWriter = task_runner.NewRegexLogger(patterns)
			}

			writer := util.CombineWriters(regexWriter, &wrapped)

			logOutput := ""
			for resultIndex, write := range testCase.writes {
				writeRegexLogger(t, writer, []byte(write.data))

				require.Equalf(
					t,
					write.wantWrapped,
					wrapped.String(),
					"wrapped output mismatch after write %d",
					resultIndex,
				)
				DrainChannelNoBlock(logsChan, func(record slog.Record) {
					logOutput += record.Message
				})

				require.Equalf(
					t,
					write.wantLog,
					logOutput,
					"log output mismatch after write %d",
					resultIndex,
				)
			}
		})
	}
}

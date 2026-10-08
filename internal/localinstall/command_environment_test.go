// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type commandEnvironmentResult struct {
	Arguments []string `json:"arguments"`
	Input     []byte   `json:"input"`
	Value     string   `json:"value"`
	Empty     bool     `json:"empty"`
	Inherited string   `json:"inherited"`
}

//nolint:revive // A subprocess must exit before the test harness writes to stdout.
func TestCommandEnvironmentHelper(t *testing.T) {
	t.Parallel()
	// Given
	marker := slices.Index(os.Args, "--environment-test")
	if marker < 0 {
		return
	}
	// When
	input, err := io.ReadAll(os.Stdin)
	value, present := os.LookupEnv("EXASOL_TEST_EMPTY")
	result := commandEnvironmentResult{
		Arguments: os.Args[marker+1:], Input: input,
		Value: os.Getenv("EXASOL_TEST_VALUE"), Empty: present && value == "",
		Inherited: os.Getenv("EXASOL_TEST_INHERITED"),
	}
	// Then
	if err != nil {
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		os.Exit(1)
	}
	_, _ = io.WriteString(os.Stderr, "diagnostic")
	os.Exit(23)
}

//nolint:paralleltest // Inherited environment is part of the execution contract.
func TestCommandEnvironmentPreservesValuesArgumentsAndStreams(t *testing.T) {
	// Given
	t.Setenv("EXASOL_TEST_INHERITED", "inherited")
	t.Setenv("EXASOL_TEST_VALUE", "original")
	t.Setenv("EXASOL_TEST_EMPTY", "original")
	executable, err := os.Executable()
	require.NoError(t, err)
	arguments := []string{"", "a'b", "a\"b", "$(UNSET)", "first\nsecond", "end\\", "é"}
	input := strings.Repeat("stdin\x00\n", 8192)
	value := "a'\"b $(`literal`) \\ é\nsecond\n\n"
	for _, transport := range []string{"direct", "shell"} {
		for _, overrides := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/overrides=%t", transport, overrides), func(t *testing.T) {
				// Given
				if transport == "shell" && runtime.GOOS == "windows" {
					t.Skip("POSIX runtime transport")
				}
				command := append([]string{
					executable, "-test.run=^TestCommandEnvironmentHelper$",
					"--", "--environment-test",
				}, arguments...)
				var env map[string]string
				if overrides {
					env = map[string]string{"EXASOL_TEST_VALUE": value, "EXASOL_TEST_EMPTY": ""}
				}
				var stdin io.Reader = strings.NewReader(input)
				var stdout, stderr bytes.Buffer
				// When
				if transport == "shell" {
					command, stdin, err = CommandEnvironment(env, stdin, command)
					require.NoError(t, err)
					require.NotContains(t, strings.Join(command, " "), value)
					env = nil
				}
				err = NewDirectExecutionEnvironment(
					nil,
				).Run(t.Context(), env, stdin, &stdout, &stderr, command...)
				// Then
				var exitError *exec.ExitError
				require.ErrorAs(t, err, &exitError)
				require.Equal(t, 23, exitError.ExitCode())
				require.Equal(t, "diagnostic", stderr.String())
				var actual commandEnvironmentResult
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
				require.Equal(t, arguments, actual.Arguments)
				require.Equal(t, []byte(input), actual.Input)
				require.Equal(t, "inherited", actual.Inherited)
				require.Equal(t, overrides, actual.Empty)
				want := "original"
				if overrides {
					want = value
				}
				require.Equal(t, want, actual.Value)
				require.Equal(t, "original", os.Getenv("EXASOL_TEST_VALUE"))
			})
		}
	}
}

func TestCommandEnvironmentRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, env := range []map[string]string{{"bad=name": "value"}, {"NAME": "nul\x00"}} {
		t.Run(fmt.Sprint(env), func(t *testing.T) {
			t.Parallel()
			// Given
			command := []string{"podman", "run"}
			// When
			_, _, err := CommandEnvironment(env, nil, command)
			// Then
			require.EqualError(t, err, "invalid command environment")
		})
	}
}

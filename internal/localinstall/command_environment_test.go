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

type commandEnvironmentCase struct {
	executable string
	arguments  []string
	input      string
	value      string
	transport  string
	overrides  bool
}

func (testCase commandEnvironmentCase) environment() map[string]string {
	if !testCase.overrides {
		return nil
	}

	return map[string]string{"EXASOL_TEST_VALUE": testCase.value, "EXASOL_TEST_EMPTY": ""}
}

func (testCase commandEnvironmentCase) run(t *testing.T) {
	t.Helper()
	// Given
	if testCase.transport == "shell" && runtime.GOOS == "windows" {
		t.Skip("POSIX runtime transport")
	}
	command := append([]string{
		testCase.executable, "-test.run=^TestCommandEnvironmentHelper$",
		"--", "--environment-test",
	}, testCase.arguments...)
	environment := testCase.environment()
	var stdin io.Reader = strings.NewReader(testCase.input)
	var stdout, stderr bytes.Buffer
	// When
	if testCase.transport == "shell" {
		var err error
		command, stdin, err = CommandEnvironment(environment, stdin, command)
		require.NoError(t, err)
		require.NotContains(t, strings.Join(command, " "), testCase.value)
		environment = nil
	}
	err := NewDirectExecutionEnvironment(
		nil,
	).Run(t.Context(), environment, stdin, &stdout, &stderr, command...)
	// Then
	testCase.verify(t, err, &stdout, &stderr)
}

func (testCase commandEnvironmentCase) verify(
	t *testing.T, err error, stdout, stderr *bytes.Buffer,
) {
	t.Helper()
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	require.Equal(t, 23, exitError.ExitCode())
	require.Equal(t, "diagnostic", stderr.String())
	var actual commandEnvironmentResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
	require.Equal(t, testCase.arguments, actual.Arguments)
	require.Equal(t, []byte(testCase.input), actual.Input)
	require.Equal(t, "inherited", actual.Inherited)
	require.Equal(t, testCase.overrides, actual.Empty)
	want := "original"
	if testCase.overrides {
		want = testCase.value
	}
	require.Equal(t, want, actual.Value)
	require.Equal(t, "original", os.Getenv("EXASOL_TEST_VALUE"))
}

//nolint:paralleltest // Inherited environment is part of the execution contract.
func TestCommandEnvironmentPreservesValuesArgumentsAndStreams(t *testing.T) {
	// Given
	t.Setenv("EXASOL_TEST_INHERITED", "inherited")
	t.Setenv("EXASOL_TEST_VALUE", "original")
	t.Setenv("EXASOL_TEST_EMPTY", "original")
	executable, err := os.Executable()
	require.NoError(t, err)
	fixture := commandEnvironmentCase{
		executable: executable,
		arguments:  []string{"", "a'b", "a\"b", "$(UNSET)", "first\nsecond", "end\\", "é"},
		input:      strings.Repeat("stdin\x00\n", 8192),
		value:      "a'\"b $(`literal`) \\ é\nsecond\n\n",
	}
	for _, transport := range []string{"direct", "shell"} {
		for _, overrides := range []bool{false, true} {
			testCase := fixture
			testCase.transport, testCase.overrides = transport, overrides
			// When, Then
			t.Run(fmt.Sprintf("%s/overrides=%t", transport, overrides), testCase.run)
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

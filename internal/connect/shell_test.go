// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/connect/types/typesfakes"
	"github.com/stretchr/testify/require"
)

var (
	errTest            = errors.New("error")
	errInputsProcessor = errors.New("inputs processor error")
)

type mockInputsProcessor struct {
	callCount int
	inputs    []string
	retErr    bool
}

func (mp *mockInputsProcessor) processInput(input string) error {
	mp.callCount++
	if mp.retErr {
		return errInputsProcessor
	}

	mp.inputs = append(mp.inputs, input)

	return nil
}

func (mp *mockInputsProcessor) returnError() { mp.retErr = true }

// nolint: revive
func TestRunShell(t *testing.T) {
	t.Parallel()

	type mocks struct {
		shell           *typesfakes.FakeLineReader
		inputsProcessor *mockInputsProcessor
	}

	for _, test := range []struct {
		name  string
		given func(*mocks)
		then  func(*testing.T, *mocks, error)
	}{
		{
			name: "single query",
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "SELECT * FROM Dual", nil)
				mocks.shell.ReadlineReturnsOnCall(1, "", errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTest)
				require.Equal(t, 2, mocks.shell.ReadlineCallCount())
				require.Equal(t, 1, mocks.inputsProcessor.callCount)
				require.Equal(t, []string{"SELECT * FROM Dual"}, mocks.inputsProcessor.inputs)
			},
		},
		{
			name: "interrupt",
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "", types.ErrInterrupt)
				mocks.shell.ReadlineReturnsOnCall(1, "", errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTest)
				require.Equal(t, 2, mocks.shell.ReadlineCallCount())
				require.Equal(t, 0, mocks.inputsProcessor.callCount)
			},
		},
		{
			name: "multiple queries",
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "OPEN SCHEMA dummy  ", nil)
				mocks.shell.ReadlineReturnsOnCall(1, "  SELECT * FROM exa", nil)
				mocks.shell.ReadlineReturnsOnCall(2, "SELECT * FROM Dual", nil)
				mocks.shell.ReadlineReturnsOnCall(3, "", errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTest)
				require.Equal(t, []string{
					"OPEN SCHEMA dummy",
					"SELECT * FROM exa",
					"SELECT * FROM Dual",
				}, mocks.inputsProcessor.inputs)
			},
		},
		{
			name: "input processor error",
			given: func(mocks *mocks) {
				mocks.shell.ReadlineReturnsOnCall(0, "SELECT * FROM Dual", nil)
				mocks.shell.ReadlineReturnsOnCall(1, "", errTest)
				mocks.inputsProcessor.returnError()
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()

				// Inputs processor error shouldn't stop it.
				require.ErrorIs(t, err, errTest)
				require.Equal(t, 2, mocks.shell.ReadlineCallCount())
				require.Equal(t, 1, mocks.inputsProcessor.callCount)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			mocks := &mocks{&typesfakes.FakeLineReader{}, &mockInputsProcessor{}}
			test.given(mocks)

			err := runShellImpl(mocks.shell, mocks.inputsProcessor.processInput)

			test.then(t, mocks, err)
		})
	}
}

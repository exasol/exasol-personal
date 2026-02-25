// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"
	"time"
)

type LogRecord struct {
	Time    time.Time
	Message []byte
}

type LogBuffer struct {
	Records []LogRecord
	mtx     sync.Mutex
}

func NewLogBuffer() *LogBuffer {
	return &LogBuffer{}
}

var _ io.Writer = (*LogBuffer)(nil)

func (logBuffer *LogBuffer) Write(data []byte) (int, error) {
	logBuffer.mtx.Lock()
	defer logBuffer.mtx.Unlock()

	return logBuffer.writePartsRecursive(data), nil
}

func (logBuffer *LogBuffer) Clear() {
	logBuffer.mtx.Lock()
	defer logBuffer.mtx.Unlock()

	logBuffer.Records = nil
}

func (logBuffer *LogBuffer) ReplayLogMessages(ctx context.Context) {
	logBuffer.mtx.Lock()
	defer logBuffer.mtx.Unlock()

	handler := slog.Default().Handler()
	for _, record := range logBuffer.Records {
		err := handler.Handle(ctx,
			slog.NewRecord(record.Time, slog.LevelError, string(record.Message), 0))
		if err != nil {
			slog.Error("failed to replay log messages", "error", err.Error())
		}
	}
}

func (logBuffer *LogBuffer) latestRecord() *LogRecord {
	if len(logBuffer.Records) == 0 {
		return logBuffer.newRecord()
	}

	return &logBuffer.Records[len(logBuffer.Records)-1]
}

func (logBuffer *LogBuffer) newRecord() *LogRecord {
	logBuffer.Records = append(logBuffer.Records, LogRecord{
		Time:    time.Now(),
		Message: nil,
	})

	return logBuffer.latestRecord()
}

func (logBuffer *LogBuffer) writePartsRecursive(data []byte) int {
	if len(data) == 0 {
		return 0
	}

	latestRecord := logBuffer.latestRecord()

	idx := bytes.IndexByte(data, '\n')
	msg := data
	var remainder []byte

	if idx != -1 {
		msg = data[:idx]
		remainder = data[idx+1:]
	}

	latestRecord.Message = append(latestRecord.Message, msg...)
	latestRecord.Time = time.Now()

	if idx != -1 {
		logBuffer.newRecord()
	}

	bytes_consumed := len(data) - len(remainder)

	return bytes_consumed + logBuffer.writePartsRecursive(remainder)
}

// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package types

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

type SQLErrorPosition struct {
	Line   *int `json:"line,omitempty"`
	Column *int `json:"column,omitempty"`
}

type StructuredSQLError struct {
	ErrorCode string            `json:"errorCode"`
	SQLState  string            `json:"sqlState"`
	Message   string            `json:"message"`
	SessionID *string           `json:"sessionId"`
	Position  *SQLErrorPosition `json:"position"`
}

type structuredSQLErrorCarrier interface {
	StructuredSQLError() StructuredSQLError
}

const (
	driverErrorMatchCount = 4
	submatchValueCount    = 2
)

var (
	sqlDriverErrorPattern = regexp.MustCompile(
		`(?s)([A-Z]-[A-Z0-9-]+): ` +
			`execution failed with SQL error code '([^']*)' and message '(.*)'$`,
	)
	errorCodePattern = regexp.MustCompile(`^([A-Z]+-\d+):`)
	sessionIDPattern = regexp.MustCompile(`(?i)\bsession(?:\s+id)?\s*[:=]?\s*(\d+)\b`)
	linePattern      = regexp.MustCompile(`(?i)\bline\s+(\d+)\b`)
	columnPattern    = regexp.MustCompile(`(?i)\bcolumn\s+(\d+)\b`)
)

func StructuredSQLErrorFromError(err error) StructuredSQLError {
	if err == nil {
		return StructuredSQLError{}
	}

	var carrier structuredSQLErrorCarrier
	if errors.As(err, &carrier) {
		return carrier.StructuredSQLError()
	}

	structured := StructuredSQLError{Message: err.Error()}

	matches := findDriverErrorMatch(err)
	if len(matches) == driverErrorMatchCount {
		structured.SQLState = matches[2]
		structured.Message = matches[3]
	}

	messageCode := errorCodePattern.FindStringSubmatch(structured.Message)
	if len(messageCode) == submatchValueCount {
		structured.ErrorCode = messageCode[1]
	}

	sessionID := sessionIDPattern.FindStringSubmatch(structured.Message)
	if len(sessionID) == submatchValueCount {
		value := sessionID[1]
		structured.SessionID = &value
	}

	line := parseFirstMatch(linePattern, structured.Message)
	column := parseFirstMatch(columnPattern, structured.Message)
	if line != nil || column != nil {
		structured.Position = &SQLErrorPosition{Line: line, Column: column}
	}

	if structured.ErrorCode == "" {
		if structured.SQLState != "" {
			structured.ErrorCode = structured.SQLState
		} else {
			structured.ErrorCode = "UNKNOWN"
		}
	}

	return structured
}

func findDriverErrorMatch(err error) []string {
	if err == nil {
		return nil
	}

	queue := []error{err}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == nil {
			continue
		}

		matches := sqlDriverErrorPattern.FindStringSubmatch(current.Error())
		if len(matches) == driverErrorMatchCount {
			return matches
		}

		if typedErr, ok := any(current).(interface{ Unwrap() []error }); ok {
			queue = append(queue, typedErr.Unwrap()...)
			continue
		}

		if typedErr, ok := any(current).(interface{ Unwrap() error }); ok {
			queue = append(queue, typedErr.Unwrap())
		}
	}

	return nil
}

func parsePositiveInt(value string) *int {
	if value == "" {
		return nil
	}

	result, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || result <= 0 {
		return nil
	}

	return &result
}

func parseFirstMatch(pattern *regexp.Regexp, message string) *int {
	match := pattern.FindStringSubmatch(message)
	if len(match) != submatchValueCount {
		return nil
	}

	return parsePositiveInt(match[1])
}

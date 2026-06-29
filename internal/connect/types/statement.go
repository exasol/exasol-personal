// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package types

import (
	"strings"
	"unicode"
)

const leadingKeywordLimit = 2

type StatementType string

const (
	StatementTypeUnknown     StatementType = "UNKNOWN"
	StatementTypeSelect      StatementType = "SELECT"
	StatementTypeWith        StatementType = "WITH"
	StatementTypeExplain     StatementType = "EXPLAIN"
	StatementTypeInsert      StatementType = "INSERT"
	StatementTypeUpdate      StatementType = "UPDATE"
	StatementTypeDelete      StatementType = "DELETE"
	StatementTypeMerge       StatementType = "MERGE"
	StatementTypeImport      StatementType = "IMPORT"
	StatementTypeExport      StatementType = "EXPORT"
	StatementTypeCreate      StatementType = "CREATE"
	StatementTypeAlter       StatementType = "ALTER"
	StatementTypeDrop        StatementType = "DROP"
	StatementTypeTruncate    StatementType = "TRUNCATE"
	StatementTypeOpenSchema  StatementType = "OPEN_SCHEMA"
	StatementTypeCloseSchema StatementType = "CLOSE_SCHEMA"
	StatementTypeSet         StatementType = "SET"
	StatementTypeCommit      StatementType = "COMMIT"
	StatementTypeRollback    StatementType = "ROLLBACK"
	StatementTypeGrant       StatementType = "GRANT"
	StatementTypeRevoke      StatementType = "REVOKE"
)

func ClassifyStatement(sql string) StatementType {
	keywords := leadingKeywords(sql, leadingKeywordLimit)
	if len(keywords) == 0 {
		return StatementTypeUnknown
	}

	switch keywords[0] {
	case "SELECT":
		return StatementTypeSelect
	case "WITH":
		return StatementTypeWith
	case "EXPLAIN":
		return StatementTypeExplain
	case "INSERT":
		return StatementTypeInsert
	case "UPDATE":
		return StatementTypeUpdate
	case "DELETE":
		return StatementTypeDelete
	case "MERGE":
		return StatementTypeMerge
	case "IMPORT":
		return StatementTypeImport
	case "EXPORT":
		return StatementTypeExport
	case "CREATE":
		return StatementTypeCreate
	case "ALTER":
		return StatementTypeAlter
	case "DROP":
		return StatementTypeDrop
	case "TRUNCATE":
		return StatementTypeTruncate
	case "SET":
		return StatementTypeSet
	case "COMMIT":
		return StatementTypeCommit
	case "ROLLBACK":
		return StatementTypeRollback
	case "GRANT":
		return StatementTypeGrant
	case "REVOKE":
		return StatementTypeRevoke
	case "OPEN":
		if len(keywords) > 1 && keywords[1] == "SCHEMA" {
			return StatementTypeOpenSchema
		}
	case "CLOSE":
		if len(keywords) > 1 && keywords[1] == "SCHEMA" {
			return StatementTypeCloseSchema
		}
	default:
		return StatementTypeUnknown
	}

	return StatementTypeUnknown
}

func (statementType StatementType) UsesExecPath() bool {
	switch statementType {
	case StatementTypeUnknown,
		StatementTypeSelect,
		StatementTypeWith,
		StatementTypeExplain:
		return false
	case StatementTypeInsert,
		StatementTypeUpdate,
		StatementTypeDelete,
		StatementTypeMerge,
		StatementTypeImport,
		StatementTypeExport,
		StatementTypeCreate,
		StatementTypeAlter,
		StatementTypeDrop,
		StatementTypeTruncate,
		StatementTypeOpenSchema,
		StatementTypeCloseSchema,
		StatementTypeSet,
		StatementTypeCommit,
		StatementTypeRollback,
		StatementTypeGrant,
		StatementTypeRevoke:
		return true
	default:
		return false
	}
}

func leadingKeywords(sql string, limit int) []string {
	trimmed := stripLeadingSQLComments(strings.TrimSpace(sql))
	if trimmed == "" || limit <= 0 {
		return nil
	}

	keywords := make([]string, 0, limit)
	var builder strings.Builder

	flush := func() bool {
		if builder.Len() == 0 {
			return false
		}

		keywords = append(keywords, strings.ToUpper(builder.String()))
		builder.Reset()

		return len(keywords) >= limit
	}

	for _, char := range trimmed {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' {
			_, _ = builder.WriteRune(char)
			continue
		}
		if flush() {
			break
		}
	}

	flush()

	return keywords
}

func stripLeadingSQLComments(sql string) string {
	for {
		trimmed := strings.TrimSpace(sql)
		switch {
		case strings.HasPrefix(trimmed, "--"):
			newline := strings.IndexByte(trimmed, '\n')
			if newline < 0 {
				return ""
			}
			sql = trimmed[newline+1:]
		case strings.HasPrefix(trimmed, "/*"):
			end := strings.Index(trimmed, "*/")
			if end < 0 {
				return ""
			}
			sql = trimmed[end+2:]
		default:
			return trimmed
		}
	}
}

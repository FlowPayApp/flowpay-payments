package repository

import (
	"database/sql"
	"encoding/json"
	"strings"
)

func nullInt64(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}

func nullStringVal(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func nullInt16Val(n int16) any {
	if n == 0 {
		return nil
	}
	return n
}

func nullInt32Val(n int32) any {
	return n
}

func nullableJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

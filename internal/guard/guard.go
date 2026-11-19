// Package guard blocks writes on connections marked read-only. It's a safety
// net for production profiles, not a security boundary — use a read-only
// database user for that.
package guard

import (
	"errors"
	"regexp"
	"strings"
)

var errReadOnly = errors.New("this connection is read-only — edit it to allow writes")

var sqlWrite = regexp.MustCompile(`(?i)\b(insert|update|delete|merge|upsert|replace|create|alter|drop|truncate|grant|revoke|rename|attach|detach|vacuum|reindex|comment|copy|call|exec|execute|lock|set)\b`)

var mongoWrite = regexp.MustCompile(`\.(insert\w*|update\w*|replace\w*|delete\w*|remove|drop\w*|createIndex\w*|rename\w*|bulkWrite|findOneAnd\w+)\s*\(|\$out\b|\$merge\b`)

var redisRead = map[string]bool{
	"get": true, "mget": true, "strlen": true, "getrange": true, "exists": true, "type": true, "ttl": true, "pttl": true,
	"keys": true, "scan": true, "hget": true, "hmget": true, "hgetall": true, "hkeys": true, "hvals": true, "hlen": true,
	"hscan": true, "lrange": true, "llen": true, "lindex": true, "smembers": true, "scard": true, "sismember": true,
	"sscan": true, "zrange": true, "zrangebyscore": true, "zrevrange": true, "zcard": true, "zscore": true, "zscan": true,
	"xrange": true, "xrevrange": true, "xlen": true, "xinfo": true, "info": true, "dbsize": true, "ping": true,
	"memory": true, "object": true, "time": true, "client": false, "randomkey": true, "dump": false,
}

// ReadOnly returns an error if query could modify data.
func ReadOnly(language, query string) error {
	switch language {
	case "sql":
		if sqlWrite.MatchString(stripStrings(query)) {
			return errReadOnly
		}
	case "mongodb":
		if mongoWrite.MatchString(query) {
			return errReadOnly
		}
		if strings.HasPrefix(strings.TrimSpace(query), "{") {
			return errors.New("raw commands are disabled on read-only connections")
		}
	case "redis":
		for _, line := range strings.Split(query, "\n") {
			f := strings.Fields(line)
			if len(f) == 0 || strings.HasPrefix(f[0], "#") {
				continue
			}
			if !redisRead[strings.ToLower(f[0])] {
				return errReadOnly
			}
		}
	}
	return nil
}

// stripStrings removes quoted literals so `WHERE note = 'drop'` isn't flagged.
func stripStrings(q string) string {
	var b strings.Builder
	var quote rune
	for _, r := range q {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"' || r == '`':
			quote = r
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Package search provides Gmail-like search query parsing.
package search

import (
	"strconv"
	"strings"
	"time"
)

var dateFormats = [...]string{
	"2006-01-02",
	"2006/01/02",
	"01/02/2006",
	"02/01/2006",
}

type Query struct {
	TextTerms     []string
	FromAddrs     []string
	ToAddrs       []string
	CcAddrs       []string
	BccAddrs      []string
	SubjectTerms  []string
	Labels        []string
	HasAttachment *bool
	BeforeDate    *time.Time
	AfterDate     *time.Time
	LargerThan    *int64
	SmallerThan   *int64
	AccountID     *int64
	HideDeleted   bool
}

func (q *Query) IsEmpty() bool {
	return len(q.TextTerms) == 0 &&
		len(q.FromAddrs) == 0 &&
		len(q.ToAddrs) == 0 &&
		len(q.CcAddrs) == 0 &&
		len(q.BccAddrs) == 0 &&
		len(q.SubjectTerms) == 0 &&
		len(q.Labels) == 0 &&
		q.HasAttachment == nil &&
		q.BeforeDate == nil &&
		q.AfterDate == nil &&
		q.LargerThan == nil &&
		q.SmallerThan == nil
}

func toLowerFast(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			b := make([]byte, len(s))
			copy(b, s[:i])
			b[i] = c + 32
			for i++; i < len(s); i++ {
				c = s[i]
				if c >= 'A' && c <= 'Z' {
					b[i] = c + 32
				} else {
					b[i] = c
				}
			}
			return string(b)
		}
	}
	return s
}

var trueVal = true

func applyOperator(q *Query, op, value string, now *time.Time) bool {
	switch op {
	case "from":
		q.FromAddrs = append(q.FromAddrs, toLowerFast(value))
	case "to":
		q.ToAddrs = append(q.ToAddrs, toLowerFast(value))
	case "cc":
		q.CcAddrs = append(q.CcAddrs, toLowerFast(value))
	case "bcc":
		q.BccAddrs = append(q.BccAddrs, toLowerFast(value))
	case "subject":
		q.SubjectTerms = append(q.SubjectTerms, value)
	case "label", "l":
		if v := strings.TrimSpace(value); v != "" {
			q.Labels = append(q.Labels, v)
		}
	case "has":
		if low := toLowerFast(value); low == "attachment" || low == "attachments" {
			q.HasAttachment = &trueVal
		}
	case "before":
		if t := parseDate(value); t != nil {
			q.BeforeDate = t
		}
	case "after":
		if t := parseDate(value); t != nil {
			q.AfterDate = t
		}
	case "older_than":
		if now.IsZero() {
			*now = time.Now().UTC()
		}
		if t := parseRelativeDate(value, *now); t != nil {
			q.BeforeDate = t
		}
	case "newer_than":
		if now.IsZero() {
			*now = time.Now().UTC()
		}
		if t := parseRelativeDate(value, *now); t != nil {
			q.AfterDate = t
		}
	case "larger":
		if size := parseSize(value); size != nil {
			q.LargerThan = size
		}
	case "smaller":
		if size := parseSize(value); size != nil {
			q.SmallerThan = size
		}
	default:
		return false
	}
	return true
}

type Parser struct {
	Now func() time.Time
}

func NewParser() *Parser {
	return &Parser{Now: func() time.Time { return time.Now().UTC() }}
}

func (p *Parser) Parse(queryStr string) *Query {
	q := &Query{}
	var now time.Time
	if p.Now != nil {
		now = p.Now()
	}
	tokens := tokenize(queryStr)
	for _, token := range tokens {
		if isQuotedPhrase(token) {
			q.TextTerms = append(q.TextTerms, unquote(token))
			continue
		}
		if idx := strings.IndexByte(token, ':'); idx != -1 {
			op := toLowerFast(token[:idx])
			value := unquote(token[idx+1:])
			if !applyOperator(q, op, value, &now) {
				q.TextTerms = append(q.TextTerms, token)
			}
			continue
		}
		q.TextTerms = append(q.TextTerms, token)
	}
	return q
}

var defaultParser = &Parser{Now: nil}

func Parse(queryStr string) *Query {
	return defaultParser.Parse(queryStr)
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func isQuotedPhrase(token string) bool {
	return len(token) > 2 && token[0] == '"' && token[len(token)-1] == '"'
}

func containsQuote(s string) bool {
	return strings.IndexByte(s, '"') >= 0 || strings.IndexByte(s, '\'') >= 0
}

func tokenize(queryStr string) []string {
	if len(queryStr) == 0 {
		return nil
	}
	if !containsQuote(queryStr) {
		nspaces := strings.Count(queryStr, " ")
		tokens := make([]string, 0, nspaces+1)
		start := 0
		for i := 0; i < len(queryStr); i++ {
			if queryStr[i] == ' ' {
				if i > start {
					tokens = append(tokens, queryStr[start:i])
				}
				start = i + 1
			}
		}
		if start < len(queryStr) {
			tokens = append(tokens, queryStr[start:])
		}
		return tokens
	}
	var tokens []string
	var current strings.Builder
	inQuotes := false
	var quoteChar byte
	afterColon := false
	opQuoted := false
	for i := 0; i < len(queryStr); i++ {
		b := queryStr[i]
		if (b == '"' || b == '\'') && !inQuotes {
			inQuotes = true
			quoteChar = b
			opQuoted = afterColon
			if !afterColon && current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			if afterColon {
				current.WriteByte(b)
			}
			afterColon = false
		} else if b == quoteChar && inQuotes {
			inQuotes = false
			if opQuoted {
				current.WriteByte(b)
				tokens = append(tokens, current.String())
				current.Reset()
			} else if current.Len() > 0 {
				tokens = append(tokens, "\""+current.String()+"\"")
				current.Reset()
			}
			quoteChar = 0
			opQuoted = false
		} else if b == ' ' && !inQuotes {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			afterColon = false
		} else {
			current.WriteByte(b)
			afterColon = (b == ':')
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func parseDate(value string) *time.Time {
	value = strings.TrimSpace(value)
	for _, format := range dateFormats {
		if t, err := time.Parse(format, value); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}

func parseRelativeDate(value string, now time.Time) *time.Time {
	value = strings.TrimSpace(value)
	n := len(value)
	if n < 2 {
		return nil
	}
	unit := value[n-1]
	if unit >= 'A' && unit <= 'Z' {
		unit += 32
	}
	if unit != 'd' && unit != 'w' && unit != 'm' && unit != 'y' {
		return nil
	}
	amount := 0
	for i := 0; i < n-1; i++ {
		c := value[i]
		if c < '0' || c > '9' {
			return nil
		}
		amount = amount*10 + int(c-'0')
	}
	if amount == 0 {
		return nil
	}
	var result time.Time
	switch unit {
	case 'd':
		result = now.AddDate(0, 0, -amount)
	case 'w':
		result = now.AddDate(0, 0, -amount*7)
	case 'm':
		result = now.AddDate(0, -amount, 0)
	case 'y':
		result = now.AddDate(-amount, 0, 0)
	}
	return &result
}

func toUpperByte(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return c
}

func parseSize(value string) *int64 {
	value = strings.TrimSpace(value)
	n := len(value)
	if n == 0 {
		return nil
	}
	var mult int64
	var numStr string
	if n >= 3 {
		c1, c2 := toUpperByte(value[n-2]), toUpperByte(value[n-1])
		if c2 == 'B' {
			switch c1 {
			case 'K':
				mult, numStr = 1024, value[:n-2]
			case 'M':
				mult, numStr = 1024*1024, value[:n-2]
			case 'G':
				mult, numStr = 1024*1024*1024, value[:n-2]
			}
		}
	}
	if mult == 0 && n >= 2 {
		switch toUpperByte(value[n-1]) {
		case 'K':
			mult, numStr = 1024, value[:n-1]
		case 'M':
			mult, numStr = 1024*1024, value[:n-1]
		case 'G':
			mult, numStr = 1024*1024*1024, value[:n-1]
		}
	}
	if mult != 0 {
		if num, err := strconv.ParseFloat(numStr, 64); err == nil {
			result := int64(num * float64(mult))
			return &result
		}
		return nil
	}
	if num, err := strconv.ParseInt(value, 10, 64); err == nil {
		return &num
	}
	return nil
}

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

// matchHasAttachment checks if value is "attachment" or "attachments"
// case-insensitively.
func matchHasAttachment(s string) bool {
	ls := toLowerFast(s)
	return ls == "attachment" || ls == "attachments"
}

// queryStore bundles all per-Parse allocations into a single heap object.
type queryStore struct {
	query       Query
	beforeDate  time.Time
	afterDate   time.Time
	largerThan  int64
	smallerThan int64
	textBuf     [4]string
	fromBuf     [2]string
	toBuf       [2]string
	ccBuf       [1]string
	bccBuf      [1]string
	subjectBuf  [2]string
	labelBuf    [2]string
}

func newQueryStore() *queryStore {
	s := &queryStore{}
	q := &s.query
	q.TextTerms = s.textBuf[:0]
	q.FromAddrs = s.fromBuf[:0]
	q.ToAddrs = s.toBuf[:0]
	q.CcAddrs = s.ccBuf[:0]
	q.BccAddrs = s.bccBuf[:0]
	q.SubjectTerms = s.subjectBuf[:0]
	q.Labels = s.labelBuf[:0]
	return s
}

// dispatchToken processes a single token. colonRel is the position of the
// first colon relative to token start (-1 if none). Performs inline operator
// matching and value dispatch in a single switch, avoiding a separate function
// call for operator identification.
func dispatchToken(s *queryStore, token string, colonRel int, now *time.Time) {
	q := &s.query
	if colonRel < 0 {
		q.TextTerms = append(q.TextTerms, token)
		return
	}
	opN := colonRel
	value := unquote(token[colonRel+1:])
	switch opN {
	case 1:
		if token[0]|0x20 == 'l' {
			if v := strings.TrimSpace(value); v != "" {
				q.Labels = append(q.Labels, v)
			}
			return
		}
	case 2:
		b0, b1 := token[0]|0x20, token[1]|0x20
		if b0 == 't' && b1 == 'o' {
			q.ToAddrs = append(q.ToAddrs, toLowerFast(value))
			return
		}
		if b0 == 'c' && b1 == 'c' {
			q.CcAddrs = append(q.CcAddrs, toLowerFast(value))
			return
		}
	case 3:
		b0, b1, b2 := token[0]|0x20, token[1]|0x20, token[2]|0x20
		if b0 == 'h' && b1 == 'a' && b2 == 's' {
			if matchHasAttachment(value) {
				q.HasAttachment = &trueVal
			}
			return
		}
		if b0 == 'b' && b1 == 'c' && b2 == 'c' {
			q.BccAddrs = append(q.BccAddrs, toLowerFast(value))
			return
		}
	case 4:
		if token[0]|0x20 == 'f' && token[1]|0x20 == 'r' && token[2]|0x20 == 'o' && token[3]|0x20 == 'm' {
			q.FromAddrs = append(q.FromAddrs, toLowerFast(value))
			return
		}
	case 5:
		b0 := token[0] | 0x20
		if b0 == 'l' && token[1]|0x20 == 'a' && token[2]|0x20 == 'b' && token[3]|0x20 == 'e' && token[4]|0x20 == 'l' {
			if v := strings.TrimSpace(value); v != "" {
				q.Labels = append(q.Labels, v)
			}
			return
		}
		if b0 == 'a' && token[1]|0x20 == 'f' && token[2]|0x20 == 't' && token[3]|0x20 == 'e' && token[4]|0x20 == 'r' {
			if t, ok := parseDateValue(value); ok {
				s.afterDate = t
				q.AfterDate = &s.afterDate
			}
			return
		}
	case 6:
		b0 := token[0] | 0x20
		if b0 == 'b' && token[1]|0x20 == 'e' && token[2]|0x20 == 'f' && token[3]|0x20 == 'o' && token[4]|0x20 == 'r' && token[5]|0x20 == 'e' {
			if t, ok := parseDateValue(value); ok {
				s.beforeDate = t
				q.BeforeDate = &s.beforeDate
			}
			return
		}
		if b0 == 'l' && token[1]|0x20 == 'a' && token[2]|0x20 == 'r' && token[3]|0x20 == 'g' && token[4]|0x20 == 'e' && token[5]|0x20 == 'r' {
			if v, ok := parseSizeValue(value); ok {
				s.largerThan = v
				q.LargerThan = &s.largerThan
			}
			return
		}
	case 7:
		b0, b1 := token[0]|0x20, token[1]|0x20
		if b0 == 's' {
			if b1 == 'u' && token[2]|0x20 == 'b' && token[3]|0x20 == 'j' && token[4]|0x20 == 'e' && token[5]|0x20 == 'c' && token[6]|0x20 == 't' {
				q.SubjectTerms = append(q.SubjectTerms, value)
				return
			}
			if b1 == 'm' && token[2]|0x20 == 'a' && token[3]|0x20 == 'l' && token[4]|0x20 == 'l' && token[5]|0x20 == 'e' && token[6]|0x20 == 'r' {
				if v, ok := parseSizeValue(value); ok {
					s.smallerThan = v
					q.SmallerThan = &s.smallerThan
				}
				return
			}
		}
	case 10:
		if token[5] == '_' {
			b0 := token[0] | 0x20
			if b0 == 'o' && token[1]|0x20 == 'l' && token[2]|0x20 == 'd' && token[3]|0x20 == 'e' && token[4]|0x20 == 'r' &&
				token[6]|0x20 == 't' && token[7]|0x20 == 'h' && token[8]|0x20 == 'a' && token[9]|0x20 == 'n' {
				if now.IsZero() {
					*now = time.Now().UTC()
				}
				if t := parseRelativeDate(value, *now); t != nil {
					s.beforeDate = *t
					q.BeforeDate = &s.beforeDate
				}
				return
			}
			if b0 == 'n' && token[1]|0x20 == 'e' && token[2]|0x20 == 'w' && token[3]|0x20 == 'e' && token[4]|0x20 == 'r' &&
				token[6]|0x20 == 't' && token[7]|0x20 == 'h' && token[8]|0x20 == 'a' && token[9]|0x20 == 'n' {
				if now.IsZero() {
					*now = time.Now().UTC()
				}
				if t := parseRelativeDate(value, *now); t != nil {
					s.afterDate = *t
					q.AfterDate = &s.afterDate
				}
				return
			}
		}
	}
	q.TextTerms = append(q.TextTerms, token)
}

type Parser struct {
	Now func() time.Time
}

func NewParser() *Parser {
	return &Parser{Now: func() time.Time { return time.Now().UTC() }}
}

func (p *Parser) Parse(queryStr string) *Query {
	s := newQueryStore()
	q := &s.query
	var now time.Time
	if p.Now != nil {
		now = p.Now()
	}
	n := len(queryStr)
	if n == 0 {
		return q
	}

	start := 0
	colonRel := -1
	for i := 0; i < n; i++ {
		b := queryStr[i]
		if b == ' ' {
			if i > start {
				dispatchToken(s, queryStr[start:i], colonRel, &now)
			}
			start = i + 1
			colonRel = -1
		} else if b == ':' && colonRel < 0 {
			colonRel = i - start
		} else if b == '"' || b == '\'' {
			afterColon := i > start && queryStr[i-1] == ':'
			if !afterColon && i > start {
				dispatchToken(s, queryStr[start:i], colonRel, &now)
			}
			quoteChar := b
			ci := strings.IndexByte(queryStr[i+1:], quoteChar)
			if ci >= 0 {
				endQ := i + 1 + ci
				if afterColon {
					dispatchToken(s, queryStr[start:endQ+1], colonRel, &now)
				} else {
					if i+1 < endQ {
						q.TextTerms = append(q.TextTerms, queryStr[i+1:endQ])
					}
				}
				i = endQ
				start = endQ + 1
				colonRel = -1
			} else {
				if afterColon {
					dispatchToken(s, queryStr[start:], colonRel, &now)
				} else {
					tok := queryStr[i+1:]
					dispatchToken(s, tok, strings.IndexByte(tok, ':'), &now)
				}
				return q
			}
		}
	}
	if start < n {
		dispatchToken(s, queryStr[start:], colonRel, &now)
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
	nspaces := strings.Count(queryStr, " ")
	tokens := make([]string, 0, nspaces+1)
	inQuotes := false
	var quoteChar byte
	afterColon := false
	opQuoted := false
	start := -1
	for i := 0; i < len(queryStr); i++ {
		b := queryStr[i]
		if (b == '"' || b == '\'') && !inQuotes {
			inQuotes = true
			quoteChar = b
			opQuoted = afterColon
			if !afterColon && start >= 0 {
				tokens = append(tokens, queryStr[start:i])
				start = -1
			}
			if !afterColon {
				start = i
			}
			afterColon = false
		} else if b == quoteChar && inQuotes {
			inQuotes = false
			if opQuoted {
				if start >= 0 {
					tokens = append(tokens, queryStr[start:i+1])
					start = -1
				}
			} else if quoteChar == '"' {
				if start >= 0 && i > start+1 {
					tokens = append(tokens, queryStr[start:i+1])
				}
				start = -1
			} else {
				if start >= 0 && i > start+1 {
					tokens = append(tokens, "\""+queryStr[start+1:i]+"\"")
				}
				start = -1
			}
			quoteChar = 0
			opQuoted = false
		} else if b == ' ' && !inQuotes {
			if start >= 0 {
				tokens = append(tokens, queryStr[start:i])
				start = -1
			}
			afterColon = false
		} else {
			if start < 0 {
				start = i
			}
			afterColon = (b == ':')
		}
	}
	if start >= 0 {
		off := 0
		if inQuotes && !opQuoted && quoteChar != 0 {
			off = 1
		}
		tokens = append(tokens, queryStr[start+off:])
	}
	return tokens
}

// dateToUnixDays computes days since Unix epoch (1970-01-01) for a valid date.
// Uses the civil_from_days algorithm (Howard Hinnant). Inputs must be valid
// (month 1-12, day 1-31, year >= 1) — no normalization is performed.
func dateToUnixDays(year, month, day int) int64 {
	y := int64(year)
	m := int64(month)
	if m <= 2 {
		y--
	}
	era := y / 400
	yoe := y - era*400
	var doy int64
	if m > 2 {
		doy = (153*(m-3)+2)/5 + int64(day) - 1
	} else {
		doy = (153*(m+9)+2)/5 + int64(day) - 1
	}
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// makeUTCDate constructs a UTC time.Time from year/month/day without the
// overhead of time.Date's normalization logic. Uses dateToUnixDays + time.Unix.
func makeUTCDate(year, month, day int) time.Time {
	return time.Unix(dateToUnixDays(year, month, day)*86400, 0).UTC()
}

// digits2 parses a 2-digit decimal number from s[0:2].
func digits2(s string) int {
	return int(s[0]-'0')*10 + int(s[1]-'0')
}

// digits4 parses a 4-digit decimal number from s[0:4].
func digits4(s string) int {
	return int(s[0]-'0')*1000 + int(s[1]-'0')*100 + int(s[2]-'0')*10 + int(s[3]-'0')
}

func parseDateValue(value string) (time.Time, bool) {
	if len(value) > 0 && (value[0] == ' ' || value[len(value)-1] == ' ') {
		value = strings.TrimSpace(value)
	}
	if len(value) != 10 {
		return time.Time{}, false
	}
	if value[4] == '-' && value[7] == '-' {
		year, month, day := digits4(value[0:4]), digits2(value[5:7]), digits2(value[8:10])
		if month >= 1 && month <= 12 && day >= 1 && day <= 31 {
			return makeUTCDate(year, month, day), true
		}
	} else if value[4] == '/' && value[7] == '/' {
		year, month, day := digits4(value[0:4]), digits2(value[5:7]), digits2(value[8:10])
		if month >= 1 && month <= 12 && day >= 1 && day <= 31 {
			return makeUTCDate(year, month, day), true
		}
	} else if value[2] == '/' && value[5] == '/' {
		a, b, year := digits2(value[0:2]), digits2(value[3:5]), digits4(value[6:10])
		if a >= 1 && a <= 12 && b >= 1 && b <= 31 {
			return makeUTCDate(year, a, b), true
		}
		if b >= 1 && b <= 12 && a >= 1 && a <= 31 {
			return makeUTCDate(year, b, a), true
		}
	}
	return time.Time{}, false
}

func parseDate(value string) *time.Time {
	if t, ok := parseDateValue(value); ok {
		return &t
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

// parseIntManual parses a non-negative integer from s without heap allocation.
func parseIntManual(s string) (int64, bool) {
	var n int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return n, len(s) > 0
}

func parseSizeValue(value string) (int64, bool) {
	if len(value) > 0 && (value[0] == ' ' || value[len(value)-1] == ' ') {
		value = strings.TrimSpace(value)
	}
	n := len(value)
	if n == 0 {
		return 0, false
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
		if num, ok := parseIntManual(numStr); ok {
			return num * mult, true
		}
		if num, err := strconv.ParseFloat(numStr, 64); err == nil {
			return int64(num * float64(mult)), true
		}
		return 0, false
	}
	if num, ok := parseIntManual(value); ok {
		return num, true
	}
	if num, err := strconv.ParseInt(value, 10, 64); err == nil {
		return num, true
	}
	return 0, false
}

func parseSize(value string) *int64 {
	if v, ok := parseSizeValue(value); ok {
		return &v
	}
	return nil
}

// Package search provides Gmail-like search query parsing.
package search

import (
	"strconv"
	"strings"
	"time"
)

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
	// AccountID is intentionally excluded: it's injected by the UI,
	// not the query string, so it doesn't count as a query filter.
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

// toLowerCopy allocates and lowercases s (slow path).
//
//go:noinline
func toLowerCopy(s string) string {
	b := make([]byte, len(s))
	for j := range b {
		if s[j]-'A' < 26 {
			b[j] = s[j] + 32
		} else {
			b[j] = s[j]
		}
	}
	return string(b)
}

// needsLower returns true if s contains any ASCII uppercase letter.
// Kept small enough for the compiler to inline (no function calls).
func needsLower(s string) bool {
	n := len(s)
	i := 0
	for ; i+4 <= n; i += 4 {
		if s[i]-'A' < 26 || s[i+1]-'A' < 26 || s[i+2]-'A' < 26 || s[i+3]-'A' < 26 {
			return true
		}
	}
	for ; i < n; i++ {
		if s[i]-'A' < 26 {
			return true
		}
	}
	return false
}

// toLowerFast returns s lowercased; zero-alloc when already lowercase.
func toLowerFast(s string) string {
	if needsLower(s) {
		return toLowerCopy(s)
	}
	return s
}

var trueVal = true

// matchHasAttachment checks if value is "attachment" or "attachments"
// case-insensitively.
func matchHasAttachment(s string) bool {
	n := len(s)
	if n == 10 {
		return s[0]|0x20 == 'a' && s[1]|0x20 == 't' && s[2]|0x20 == 't' &&
			s[3]|0x20 == 'a' && s[4]|0x20 == 'c' && s[5]|0x20 == 'h' &&
			s[6]|0x20 == 'm' && s[7]|0x20 == 'e' && s[8]|0x20 == 'n' &&
			s[9]|0x20 == 't'
	}
	if n == 11 {
		return s[0]|0x20 == 'a' && s[1]|0x20 == 't' && s[2]|0x20 == 't' &&
			s[3]|0x20 == 'a' && s[4]|0x20 == 'c' && s[5]|0x20 == 'h' &&
			s[6]|0x20 == 'm' && s[7]|0x20 == 'e' && s[8]|0x20 == 'n' &&
			s[9]|0x20 == 't' && s[10]|0x20 == 's'
	}
	return false
}

// queryStore bundles all per-Parse allocations into a single heap object.
// Uses a single flat buffer for all slice backing arrays.
type queryStore struct {
	query       Query
	beforeDate  time.Time
	afterDate   time.Time
	largerThan  int64
	smallerThan int64
	buf         [6]string // fits 384-byte allocator class
}

func newQueryStore() *queryStore {
	s := &queryStore{}
	q := &s.query
	q.TextTerms = s.buf[0:0:1]
	q.FromAddrs = s.buf[1:1:2]
	q.ToAddrs = s.buf[2:2:3]
	q.CcAddrs = s.buf[3:3:4]
	q.SubjectTerms = s.buf[4:4:5]
	q.Labels = s.buf[5:5:6]
	return s
}

// dispatchToken processes a single token with inline operator matching.
func dispatchToken(s *queryStore, token string, colonRel int, now *time.Time) {
	q := &s.query
	if colonRel < 0 {
		q.TextTerms = append(q.TextTerms, token)
		return
	}
	value := token[colonRel+1:]
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	switch colonRel {
	case 1:
		if token[0]|0x20 == 'l' {
			if len(value) > 0 {
				q.Labels = append(q.Labels, value)
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
			if len(value) > 0 {
				q.Labels = append(q.Labels, value)
			}
			return
		}
		if b0 == 'a' && token[1]|0x20 == 'f' && token[2]|0x20 == 't' && token[3]|0x20 == 'e' && token[4]|0x20 == 'r' {
			if len(value) == 10 {
				if t, ok := parseDateInline(value); ok {
					s.afterDate = t
					q.AfterDate = &s.afterDate
				}
			}
			return
		}
	case 6:
		b0 := token[0] | 0x20
		if b0 == 'b' && token[1]|0x20 == 'e' && token[2]|0x20 == 'f' && token[3]|0x20 == 'o' && token[4]|0x20 == 'r' && token[5]|0x20 == 'e' {
			if len(value) == 10 {
				if t, ok := parseDateInline(value); ok {
					s.beforeDate = t
					q.BeforeDate = &s.beforeDate
				}
			}
			return
		}
		if b0 == 'l' && token[1]|0x20 == 'a' && token[2]|0x20 == 'r' && token[3]|0x20 == 'g' && token[4]|0x20 == 'e' && token[5]|0x20 == 'r' {
			if v, ok := parseSizeFast(value); ok {
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
				if v, ok := parseSizeFast(value); ok {
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
		if b > ':' || b-'-' < 13 {
			continue
		}
		if b == ' ' {
			if i > start {
				if colonRel < 0 {
					q.TextTerms = append(q.TextTerms, queryStr[start:i])
				} else {
					token := queryStr[start:i]
					value := token[colonRel+1:]
					switch colonRel {
					case 1:
						if token[0]|0x20 == 'l' {
							if len(value) > 0 {
								q.Labels = append(q.Labels, value)
							}
						} else {
							q.TextTerms = append(q.TextTerms, token)
						}
					case 2:
						c0, c1 := token[0]|0x20, token[1]|0x20
						if c0 == 't' && c1 == 'o' {
							v := value
							if needsLower(v) {
								v = toLowerCopy(v)
							}
							q.ToAddrs = append(q.ToAddrs, v)
						} else if c0 == 'c' && c1 == 'c' {
							v := value
							if needsLower(v) {
								v = toLowerCopy(v)
							}
							q.CcAddrs = append(q.CcAddrs, v)
						} else {
							q.TextTerms = append(q.TextTerms, token)
						}
					case 3:
						c0, c1, c2 := token[0]|0x20, token[1]|0x20, token[2]|0x20
						if c0 == 'h' && c1 == 'a' && c2 == 's' {
							nv := len(value)
							if nv >= 10 && nv <= 11 &&
								value[0]|0x20 == 'a' && value[1]|0x20 == 't' && value[2]|0x20 == 't' &&
								value[3]|0x20 == 'a' && value[4]|0x20 == 'c' && value[5]|0x20 == 'h' &&
								value[6]|0x20 == 'm' && value[7]|0x20 == 'e' && value[8]|0x20 == 'n' &&
								value[9]|0x20 == 't' && (nv == 10 || value[10]|0x20 == 's') {
								q.HasAttachment = &trueVal
							}
						} else if c0 == 'b' && c1 == 'c' && c2 == 'c' {
							v := value
							if needsLower(v) {
								v = toLowerCopy(v)
							}
							q.BccAddrs = append(q.BccAddrs, v)
						} else {
							q.TextTerms = append(q.TextTerms, token)
						}
					case 4:
						if token[0]|0x20 == 'f' && token[1]|0x20 == 'r' && token[2]|0x20 == 'o' && token[3]|0x20 == 'm' {
							v := value
							if needsLower(v) {
								v = toLowerCopy(v)
							}
							q.FromAddrs = append(q.FromAddrs, v)
						} else {
							q.TextTerms = append(q.TextTerms, token)
						}
					case 5:
						c0 := token[0] | 0x20
						if c0 == 'l' && token[1]|0x20 == 'a' && token[2]|0x20 == 'b' && token[3]|0x20 == 'e' && token[4]|0x20 == 'l' {
							if len(value) > 0 {
								q.Labels = append(q.Labels, value)
							}
						} else if c0 == 'a' && token[1]|0x20 == 'f' && token[2]|0x20 == 't' && token[3]|0x20 == 'e' && token[4]|0x20 == 'r' {
							if len(value) == 10 && value[4] == '-' && value[7] == '-' {
								y := int(value[0]-'0')*1000 + int(value[1]-'0')*100 + int(value[2]-'0')*10 + int(value[3]-'0')
								m := int(value[5]-'0')*10 + int(value[6]-'0')
								d := int(value[8]-'0')*10 + int(value[9]-'0')
								if m >= 1 && m <= 12 && d >= 1 && d <= 31 {
									s.afterDate = time.Unix(dateToUnixDays(y, m, d)*86400, 0).UTC()
									q.AfterDate = &s.afterDate
								}
							} else if len(value) == 10 {
								if t, ok := parseDateInline(value); ok {
									s.afterDate = t
									q.AfterDate = &s.afterDate
								}
							}
						} else {
							q.TextTerms = append(q.TextTerms, token)
						}
					case 6:
						c0 := token[0] | 0x20
						if c0 == 'b' && token[1]|0x20 == 'e' && token[2]|0x20 == 'f' && token[3]|0x20 == 'o' && token[4]|0x20 == 'r' && token[5]|0x20 == 'e' {
							if len(value) == 10 && value[4] == '-' && value[7] == '-' {
								y := int(value[0]-'0')*1000 + int(value[1]-'0')*100 + int(value[2]-'0')*10 + int(value[3]-'0')
								m := int(value[5]-'0')*10 + int(value[6]-'0')
								d := int(value[8]-'0')*10 + int(value[9]-'0')
								if m >= 1 && m <= 12 && d >= 1 && d <= 31 {
									s.beforeDate = time.Unix(dateToUnixDays(y, m, d)*86400, 0).UTC()
									q.BeforeDate = &s.beforeDate
								}
							} else if len(value) == 10 {
								if t, ok := parseDateInline(value); ok {
									s.beforeDate = t
									q.BeforeDate = &s.beforeDate
								}
							}
						} else if c0 == 'l' && token[1]|0x20 == 'a' && token[2]|0x20 == 'r' && token[3]|0x20 == 'g' && token[4]|0x20 == 'e' && token[5]|0x20 == 'r' {
							if nv := len(value); nv >= 2 {
								last := value[nv-1] | 0x20
								if last == 'k' || last == 'm' || last == 'g' {
									if num, ok := parseIntManual(value[:nv-1]); ok {
										var mult int64
										switch last {
										case 'k':
											mult = 1024
										case 'm':
											mult = 1024 * 1024
										case 'g':
											mult = 1024 * 1024 * 1024
										}
										s.largerThan = num * mult
										q.LargerThan = &s.largerThan
									} else if v, ok := parseSizeFast(value); ok {
										s.largerThan = v
										q.LargerThan = &s.largerThan
									}
								} else if v, ok := parseSizeFast(value); ok {
									s.largerThan = v
									q.LargerThan = &s.largerThan
								}
							} else if v, ok := parseSizeFast(value); ok {
								s.largerThan = v
								q.LargerThan = &s.largerThan
							}
						} else {
							q.TextTerms = append(q.TextTerms, token)
						}
					case 7:
						c0, c1 := token[0]|0x20, token[1]|0x20
						if c0 == 's' {
							if c1 == 'u' && token[2]|0x20 == 'b' && token[3]|0x20 == 'j' && token[4]|0x20 == 'e' && token[5]|0x20 == 'c' && token[6]|0x20 == 't' {
								q.SubjectTerms = append(q.SubjectTerms, value)
							} else if c1 == 'm' && token[2]|0x20 == 'a' && token[3]|0x20 == 'l' && token[4]|0x20 == 'l' && token[5]|0x20 == 'e' && token[6]|0x20 == 'r' {
								if nv := len(value); nv >= 2 {
									last := value[nv-1] | 0x20
									if last == 'k' || last == 'm' || last == 'g' {
										if num, ok := parseIntManual(value[:nv-1]); ok {
											var mult int64
											switch last {
											case 'k':
												mult = 1024
											case 'm':
												mult = 1024 * 1024
											case 'g':
												mult = 1024 * 1024 * 1024
											}
											s.smallerThan = num * mult
											q.SmallerThan = &s.smallerThan
										} else if v, ok := parseSizeFast(value); ok {
											s.smallerThan = v
											q.SmallerThan = &s.smallerThan
										}
									} else if v, ok := parseSizeFast(value); ok {
										s.smallerThan = v
										q.SmallerThan = &s.smallerThan
									}
								} else if v, ok := parseSizeFast(value); ok {
									s.smallerThan = v
									q.SmallerThan = &s.smallerThan
								}
							} else {
								q.TextTerms = append(q.TextTerms, token)
							}
						} else {
							q.TextTerms = append(q.TextTerms, token)
						}
					default:
						dispatchToken(s, token, colonRel, &now)
					}
				}
			}
			start = i + 1
			colonRel = -1
		} else if b == ':' {
			if colonRel < 0 {
				colonRel = i - start
			}
		} else if b == '"' || b == '\'' {
			afterColon := i > start && queryStr[i-1] == ':'
			if !afterColon && i > start {
				dispatchToken(s, queryStr[start:i], colonRel, &now)
			}
			quoteChar := b
			endQ := -1
			if off := strings.IndexByte(queryStr[i+1:], quoteChar); off >= 0 {
				endQ = i + 1 + off
			}
			if endQ >= 0 {
				if afterColon {
					qtoken := queryStr[start:endQ+1]
					qvalue := qtoken[colonRel+1:]
					if len(qvalue) >= 2 && qvalue[0] == '"' && qvalue[len(qvalue)-1] == '"' {
						qvalue = qvalue[1 : len(qvalue)-1]
					}
					if colonRel == 7 && qtoken[0]|0x20 == 's' && qtoken[1]|0x20 == 'u' &&
						qtoken[2]|0x20 == 'b' && qtoken[3]|0x20 == 'j' &&
						qtoken[4]|0x20 == 'e' && qtoken[5]|0x20 == 'c' &&
						qtoken[6]|0x20 == 't' {
						q.SubjectTerms = append(q.SubjectTerms, qvalue)
					} else {
						dispatchToken(s, qtoken, colonRel, &now)
					}
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
		if colonRel < 0 {
			q.TextTerms = append(q.TextTerms, queryStr[start:])
		} else {
			dispatchToken(s, queryStr[start:], colonRel, &now)
		}
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

// dateToUnixDays computes days since Unix epoch (1970-01-01).
// Uses int arithmetic to stay within the Go compiler's inlining budget.
func dateToUnixDays(year, month, day int) int64 {
	if month <= 2 {
		year--
		month += 9
	} else {
		month -= 3
	}
	era := year / 400
	yoe := year - era*400
	doy := (153*month+2)/5 + day - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return int64(era)*146097 + int64(doe) - 719468
}

func makeUTCDate(year, month, day int) time.Time {
	return time.Unix(dateToUnixDays(year, month, day)*86400, 0).UTC()
}

func digits2(s string) int {
	return int(s[0]-'0')*10 + int(s[1]-'0')
}

func digits4(s string) int {
	return int(s[0]-'0')*1000 + int(s[1]-'0')*100 + int(s[2]-'0')*10 + int(s[3]-'0')
}

// parseDateInline is a fast-path date parser that skips TrimSpace and
// inlines digit extraction. Caller must ensure len(value)==10.
func parseDateInline(value string) (time.Time, bool) {
	if value[4] == '-' && value[7] == '-' {
		y := int(value[0]-'0')*1000 + int(value[1]-'0')*100 + int(value[2]-'0')*10 + int(value[3]-'0')
		m := int(value[5]-'0')*10 + int(value[6]-'0')
		d := int(value[8]-'0')*10 + int(value[9]-'0')
		if m >= 1 && m <= 12 && d >= 1 && d <= 31 {
			return time.Unix(dateToUnixDays(y, m, d)*86400, 0).UTC(), true
		}
		return time.Time{}, false
	}
	if value[4] == '/' && value[7] == '/' {
		y := int(value[0]-'0')*1000 + int(value[1]-'0')*100 + int(value[2]-'0')*10 + int(value[3]-'0')
		m := int(value[5]-'0')*10 + int(value[6]-'0')
		d := int(value[8]-'0')*10 + int(value[9]-'0')
		if m >= 1 && m <= 12 && d >= 1 && d <= 31 {
			return time.Unix(dateToUnixDays(y, m, d)*86400, 0).UTC(), true
		}
		return time.Time{}, false
	}
	if value[2] == '/' && value[5] == '/' {
		a := int(value[0]-'0')*10 + int(value[1]-'0')
		b := int(value[3]-'0')*10 + int(value[4]-'0')
		y := int(value[6]-'0')*1000 + int(value[7]-'0')*100 + int(value[8]-'0')*10 + int(value[9]-'0')
		if a >= 1 && a <= 12 && b >= 1 && b <= 31 {
			return time.Unix(dateToUnixDays(y, a, b)*86400, 0).UTC(), true
		}
		if b >= 1 && b <= 12 && a >= 1 && a <= 31 {
			return time.Unix(dateToUnixDays(y, b, a)*86400, 0).UTC(), true
		}
	}
	return time.Time{}, false
}

func parseDateValue(value string) (time.Time, bool) {
	if len(value) > 0 && (value[0] == ' ' || value[len(value)-1] == ' ') {
		value = strings.TrimSpace(value)
	}
	if len(value) != 10 {
		return time.Time{}, false
	}
	return parseDateInline(value)
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

// parseSizeFast parses size values without TrimSpace.
func parseSizeFast(value string) (int64, bool) {
	n := len(value)
	if n == 0 {
		return 0, false
	}
	var mult int64
	var numStr string
	if n >= 3 {
		c1, c2 := value[n-2]|0x20, value[n-1]|0x20
		if c2 == 'b' {
			switch c1 {
			case 'k':
				mult, numStr = 1024, value[:n-2]
			case 'm':
				mult, numStr = 1024*1024, value[:n-2]
			case 'g':
				mult, numStr = 1024*1024*1024, value[:n-2]
			}
		}
	}
	if mult == 0 && n >= 2 {
		switch value[n-1] | 0x20 {
		case 'k':
			mult, numStr = 1024, value[:n-1]
		case 'm':
			mult, numStr = 1024*1024, value[:n-1]
		case 'g':
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

func parseSizeValue(value string) (int64, bool) {
	if len(value) > 0 && (value[0] == ' ' || value[len(value)-1] == ' ') {
		value = strings.TrimSpace(value)
	}
	return parseSizeFast(value)
}

func parseSize(value string) *int64 {
	if v, ok := parseSizeValue(value); ok {
		return &v
	}
	return nil
}

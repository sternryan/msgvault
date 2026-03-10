package mbox

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"os"
	"net/textproto"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wesm/msgvault/internal/gmail"
)

const defaultPageSize = 500

// Client reads MBOX files and implements gmail.API so it can be used
// with the existing sync infrastructure.
type Client struct {
	paths      []string
	sourceName string
	logger     *slog.Logger
	afterDate  time.Time
	beforeDate time.Time
	limit      int
	returned   int

	// Index built during initialization.
	index   []indexEntry
	labels  map[string]bool
	idToIdx map[string]int // messageID → index position
}

type indexEntry struct {
	FilePath  string
	Offset    int64
	MessageID string
	ThreadID  string
	Date      time.Time
	Labels    []string
	Subject   string
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithMboxLogger sets the logger for the client.
func WithMboxLogger(l *slog.Logger) ClientOption {
	return func(c *Client) { c.logger = l }
}

// WithMboxAfterDate filters to messages on or after this date.
func WithMboxAfterDate(t time.Time) ClientOption {
	return func(c *Client) { c.afterDate = t }
}

// WithMboxBeforeDate filters to messages before this date.
func WithMboxBeforeDate(t time.Time) ClientOption {
	return func(c *Client) { c.beforeDate = t }
}

// WithMboxLimit sets the maximum number of messages to return.
func WithMboxLimit(n int) ClientOption {
	return func(c *Client) { c.limit = n }
}

// NewClient creates a client from MBOX file paths.
// It performs an index pass reading only headers to build a message index.
func NewClient(paths []string, sourceName string, opts ...ClientOption) (*Client, error) {
	c := &Client{
		paths:      paths,
		sourceName: sourceName,
		logger:     slog.Default(),
		labels:     make(map[string]bool),
		idToIdx:    make(map[string]int),
	}
	for _, opt := range opts {
		opt(c)
	}

	if err := c.buildIndex(); err != nil {
		return nil, fmt.Errorf("build mbox index: %w", err)
	}

	return c, nil
}

// Close is a no-op for MBOX files.
func (c *Client) Close() error {
	return nil
}

// GetProfile returns a profile with the source name and message count from the index.
func (c *Client) GetProfile(ctx context.Context) (*gmail.Profile, error) {
	return &gmail.Profile{
		EmailAddress:  c.sourceName,
		MessagesTotal: int64(len(c.index)),
		HistoryID:     0,
	}, nil
}

// ListLabels returns labels found in X-Gmail-Labels headers.
func (c *Client) ListLabels(ctx context.Context) ([]*gmail.Label, error) {
	var result []*gmail.Label
	for label := range c.labels {
		result = append(result, &gmail.Label{
			ID:   label,
			Name: label,
			Type: "user",
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// ListMessages returns a page of message IDs from the index.
// The pageToken is the string offset into the index. Default page size is 500.
func (c *Client) ListMessages(ctx context.Context, query string, pageToken string) (*gmail.MessageListResponse, error) {
	if c.limit > 0 && c.returned >= c.limit {
		return &gmail.MessageListResponse{}, nil
	}

	startIdx := 0
	if pageToken != "" {
		var err error
		startIdx, err = strconv.Atoi(pageToken)
		if err != nil {
			return nil, fmt.Errorf("invalid page token: %w", err)
		}
	}

	if startIdx >= len(c.index) {
		return &gmail.MessageListResponse{}, nil
	}

	pageSize := defaultPageSize
	if c.limit > 0 {
		remaining := c.limit - c.returned
		if remaining < pageSize {
			pageSize = remaining
		}
	}

	endIdx := startIdx + pageSize
	if endIdx > len(c.index) {
		endIdx = len(c.index)
	}

	var messages []gmail.MessageID
	for i := startIdx; i < endIdx; i++ {
		entry := &c.index[i]
		messages = append(messages, gmail.MessageID{
			ID:       entry.MessageID,
			ThreadID: entry.ThreadID,
		})
	}

	c.returned += len(messages)

	var nextPageToken string
	if endIdx < len(c.index) && (c.limit == 0 || c.returned < c.limit) {
		nextPageToken = strconv.Itoa(endIdx)
	}

	totalEstimate := int64(len(c.index))
	if c.limit > 0 && int64(c.limit) < totalEstimate {
		totalEstimate = int64(c.limit)
	}

	return &gmail.MessageListResponse{
		Messages:           messages,
		NextPageToken:      nextPageToken,
		ResultSizeEstimate: totalEstimate,
	}, nil
}

// GetMessageRaw reads the full message from disk at the recorded offset.
func (c *Client) GetMessageRaw(ctx context.Context, messageID string) (*gmail.RawMessage, error) {
	idx, ok := c.idToIdx[messageID]
	if !ok {
		return nil, &gmail.NotFoundError{Path: "/messages/" + messageID}
	}
	entry := &c.index[idx]

	raw, err := c.readMessageAt(entry.FilePath, entry.Offset)
	if err != nil {
		return nil, fmt.Errorf("read message %s: %w", messageID, err)
	}

	internalDate := int64(0)
	if !entry.Date.IsZero() {
		internalDate = entry.Date.UnixMilli()
	}

	return &gmail.RawMessage{
		ID:           messageID,
		ThreadID:     entry.ThreadID,
		LabelIDs:     entry.Labels,
		Snippet:      "",
		HistoryID:    0,
		InternalDate: internalDate,
		SizeEstimate: int64(len(raw)),
		Raw:          raw,
	}, nil
}

// GetMessagesRawBatch fetches multiple messages sequentially.
func (c *Client) GetMessagesRawBatch(ctx context.Context, messageIDs []string) ([]*gmail.RawMessage, error) {
	results := make([]*gmail.RawMessage, len(messageIDs))
	for i, id := range messageIDs {
		msg, err := c.GetMessageRaw(ctx, id)
		if err != nil {
			c.logger.Warn("failed to fetch message", "id", id, "error", err)
			continue
		}
		results[i] = msg
	}
	return results, nil
}

// ListHistory is not applicable for MBOX files.
func (c *Client) ListHistory(ctx context.Context, startHistoryID uint64, pageToken string) (*gmail.HistoryResponse, error) {
	return &gmail.HistoryResponse{
		HistoryID: startHistoryID,
	}, nil
}

// TrashMessage is not supported for MBOX files.
func (c *Client) TrashMessage(ctx context.Context, messageID string) error {
	return fmt.Errorf("mbox: deletion not supported")
}

// DeleteMessage is not supported for MBOX files.
func (c *Client) DeleteMessage(ctx context.Context, messageID string) error {
	return fmt.Errorf("mbox: deletion not supported")
}

// BatchDeleteMessages is not supported for MBOX files.
func (c *Client) BatchDeleteMessages(ctx context.Context, messageIDs []string) error {
	return fmt.Errorf("mbox: deletion not supported")
}

// Ensure Client implements gmail.API.
var _ gmail.API = (*Client)(nil)

// buildIndex reads headers from all MBOX files and builds the message index.
func (c *Client) buildIndex() error {
	// threadMap tracks In-Reply-To/References chains for thread resolution.
	// Key: message-id from References/In-Reply-To, Value: resolved thread ID.
	threadMap := make(map[string]string)

	for _, path := range c.paths {
		if err := c.indexFile(path, threadMap); err != nil {
			return fmt.Errorf("index %s: %w", path, err)
		}
	}

	// Second pass: resolve threads that reference earlier messages via In-Reply-To/References.
	c.resolveThreads(threadMap)

	// Build the id→index lookup.
	for i := range c.index {
		c.idToIdx[c.index[i].MessageID] = i
	}

	c.logger.Info("mbox index complete",
		"files", len(c.paths),
		"messages", len(c.index),
		"labels", len(c.labels),
	)

	return nil
}

// indexFile reads one MBOX file and appends entries to the index.
func (c *Client) indexFile(path string, threadMap map[string]string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	rdr := NewReader(f)

	for {
		entry, err := rdr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		hdr, err := parseHeaders(entry.Raw)
		if err != nil {
			c.logger.Warn("failed to parse headers", "file", path, "offset", entry.Offset, "error", err)
			continue
		}

		msgDate := parseDate(hdr.Get("Date"))

		// Apply date filters.
		if !c.afterDate.IsZero() && msgDate.Before(c.afterDate) {
			continue
		}
		if !c.beforeDate.IsZero() && !msgDate.Before(c.beforeDate) {
			continue
		}

		messageID := cleanMessageID(hdr.Get("Message-Id"))
		if messageID == "" {
			// Generate a deterministic ID from the raw content.
			messageID = fmt.Sprintf("<%x@mbox-generated>", md5.Sum(entry.Raw))
		}

		subject := hdr.Get("Subject")
		labels := parseGmailLabels(hdr.Get("X-Gmail-Labels"))
		for _, l := range labels {
			c.labels[l] = true
		}

		// Thread resolution: priority is X-GM-THRID > In-Reply-To/References > subject.
		threadID := resolveThreadID(hdr, messageID, threadMap)

		ie := indexEntry{
			FilePath:  path,
			Offset:    entry.Offset,
			MessageID: messageID,
			ThreadID:  threadID,
			Date:      msgDate,
			Labels:    labels,
			Subject:   subject,
		}
		c.index = append(c.index, ie)
	}

	return nil
}

// resolveThreads does a second pass over the index to merge thread chains.
// Messages that reference each other should share the same thread ID.
func (c *Client) resolveThreads(threadMap map[string]string) {
	// Build a union-find style resolution: follow chains to their root.
	resolve := func(id string) string {
		seen := make(map[string]bool)
		for {
			next, ok := threadMap[id]
			if !ok || next == id || seen[id] {
				return id
			}
			seen[id] = true
			id = next
		}
	}

	for i := range c.index {
		c.index[i].ThreadID = resolve(c.index[i].ThreadID)
	}
}

// readMessageAt reads the full raw message from an MBOX file at the given offset.
func (c *Client) readMessageAt(path string, offset int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	rdr := NewReader(f)

	entry, err := rdr.Next()
	if err != nil {
		return nil, err
	}
	return entry.Raw, nil
}

// parseHeaders reads just the header portion of a raw message.
func parseHeaders(raw []byte) (textproto.MIMEHeader, error) {
	// Headers end at the first blank line.
	idx := bytes.Index(raw, []byte("\n\n"))
	if idx == -1 {
		idx = bytes.Index(raw, []byte("\r\n\r\n"))
	}
	var headerBytes []byte
	if idx >= 0 {
		headerBytes = raw[:idx+1] // include the final newline
	} else {
		headerBytes = raw // all headers, no body
	}

	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(headerBytes)))
	hdr, err := reader.ReadMIMEHeader()
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("parse headers: %w", err)
	}
	return hdr, nil
}

// rePrefix matches Re:/Fwd:/Fw: prefixes for subject normalization.
var rePrefix = regexp.MustCompile(`(?i)^(re|fwd|fw)\s*:\s*`)

// resolveThreadID determines the thread ID for a message.
// Priority: X-GM-THRID > In-Reply-To/References chain > normalized subject hash.
func resolveThreadID(hdr textproto.MIMEHeader, messageID string, threadMap map[string]string) string {
	// 1. Google's native thread ID.
	if thrid := hdr.Get("X-Gm-Thrid"); thrid != "" {
		threadMap[messageID] = thrid
		return thrid
	}

	// 2. In-Reply-To / References chain.
	inReplyTo := cleanMessageID(hdr.Get("In-Reply-To"))
	refs := parseReferences(hdr.Get("References"))

	if inReplyTo != "" {
		// Link this message to the one it replies to.
		if existing, ok := threadMap[inReplyTo]; ok {
			threadMap[messageID] = existing
			return existing
		}
		// Use the In-Reply-To message ID as the thread root.
		threadMap[messageID] = inReplyTo
		return inReplyTo
	}

	if len(refs) > 0 {
		// The first reference is typically the thread root.
		root := refs[0]
		if existing, ok := threadMap[root]; ok {
			threadMap[messageID] = existing
			return existing
		}
		threadMap[messageID] = root
		return root
	}

	// 3. Normalized subject fallback.
	subject := hdr.Get("Subject")
	normalized := normalizeSubject(subject)
	if normalized != "" {
		threadID := fmt.Sprintf("subj-%x", md5.Sum([]byte(normalized)))
		threadMap[messageID] = threadID
		return threadID
	}

	// No threading info — use the message's own ID.
	threadMap[messageID] = messageID
	return messageID
}

// normalizeSubject strips Re:/Fwd: prefixes and lowercases for thread grouping.
func normalizeSubject(subject string) string {
	s := strings.TrimSpace(subject)
	for {
		stripped := rePrefix.ReplaceAllString(s, "")
		stripped = strings.TrimSpace(stripped)
		if stripped == s {
			break
		}
		s = stripped
	}
	return strings.ToLower(s)
}

// cleanMessageID extracts a message ID, stripping angle brackets.
func cleanMessageID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Extract the first <...> token.
	if start := strings.Index(raw, "<"); start >= 0 {
		if end := strings.Index(raw[start:], ">"); end >= 0 {
			return raw[start : start+end+1]
		}
	}
	// No angle brackets — use as-is.
	return raw
}

// parseReferences extracts message IDs from a References header.
func parseReferences(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var refs []string
	for _, part := range strings.Fields(raw) {
		id := cleanMessageID(part)
		if id != "" {
			refs = append(refs, id)
		}
	}
	return refs
}

// parseGmailLabels parses the X-Gmail-Labels header (CSV format with optional quoting).
func parseGmailLabels(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	r := csv.NewReader(strings.NewReader(raw))
	r.TrimLeadingSpace = true
	record, err := r.Read()
	if err != nil {
		// Fallback: split on comma.
		parts := strings.Split(raw, ",")
		var result []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	}
	var result []string
	for _, field := range record {
		field = strings.TrimSpace(field)
		if field != "" {
			result = append(result, field)
		}
	}
	return result
}

// parseDate parses an RFC 2822 date string.
func parseDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}

	// Try common formats.
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 -0700 (MST)",
		"2 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 MST",
		time.RFC822Z,
		time.RFC822,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

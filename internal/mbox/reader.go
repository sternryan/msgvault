// Package mbox provides an MBOX file reader and a gmail.API client for
// importing Google Takeout email exports into msgvault.
package mbox

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
)

// fromLineRe matches the MBOX "From " separator line.
var fromLineRe = regexp.MustCompile(`^From \S+`)

// escapedFromRe matches mboxrd-escaped ">*From " lines in message bodies.
var escapedFromRe = regexp.MustCompile(`^>+(From )`)

// RawEntry represents a single message from an MBOX file.
type RawEntry struct {
	Offset int64  // Byte offset in file where this message starts
	Raw    []byte // Complete raw message bytes (headers + body)
}

// Reader reads messages from an MBOX file.
type Reader struct {
	file   *os.File
	reader *bufio.Reader
	offset int64
	// pending holds the next "From " line already read by the previous Next() call.
	pending []byte
}

// NewReader opens an MBOX file for reading.
func NewReader(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open mbox: %w", err)
	}
	return &Reader{
		file:   f,
		reader: bufio.NewReaderSize(f, 256*1024),
		offset: 0,
	}, nil
}

// SeekTo positions the reader at the given byte offset for resumption.
func (r *Reader) SeekTo(offset int64) error {
	if _, err := r.file.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek mbox: %w", err)
	}
	r.reader.Reset(r.file)
	r.offset = offset
	r.pending = nil
	return nil
}

// Next returns the next message, or io.EOF when done.
func (r *Reader) Next() (*RawEntry, error) {
	// Find the "From " line that starts this message.
	var fromLine []byte
	if r.pending != nil {
		fromLine = r.pending
		r.pending = nil
	} else {
		for {
			line, err := r.readLine()
			if err != nil {
				return nil, err // io.EOF or real error
			}
			if isFromLine(line) {
				fromLine = line
				break
			}
			// Skip blank lines between messages or before the first message.
		}
	}

	// Record the offset where this message's "From " line begins.
	// The offset was advanced past the "From " line already, so we
	// compute the start as current offset minus the line length.
	entryOffset := r.offset - int64(len(fromLine))

	// Read lines until the next "From " separator or EOF.
	var buf bytes.Buffer
	for {
		line, err := r.readLine()
		if err == io.EOF {
			// End of file — flush current message.
			break
		}
		if err != nil {
			return nil, err
		}
		if isFromLine(line) {
			// This line belongs to the next message.
			r.pending = line
			break
		}
		// Unescape mboxrd: remove one leading '>' from lines like ">From ", ">>From ", etc.
		line = unescapeFromLine(line)
		buf.Write(line)
	}

	raw := buf.Bytes()
	// Trim trailing blank lines from the message body.
	raw = bytes.TrimRight(raw, "\r\n")
	if len(raw) > 0 {
		raw = append(raw, '\n')
	}

	return &RawEntry{
		Offset: entryOffset,
		Raw:    raw,
	}, nil
}

// Close closes the underlying file.
func (r *Reader) Close() error {
	return r.file.Close()
}

// CountMessages counts messages in an MBOX file by counting "From " separator lines.
func CountMessages(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open mbox: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 1024*1024)
	var count int64
	for scanner.Scan() {
		if isFromLine(scanner.Bytes()) {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan mbox: %w", err)
	}
	return count, nil
}

// readLine reads a full line including the newline character(s) and updates the offset.
func (r *Reader) readLine() ([]byte, error) {
	var full []byte
	for {
		line, isPrefix, err := r.reader.ReadLine()
		if err != nil {
			return nil, err
		}
		full = append(full, line...)
		if !isPrefix {
			break
		}
	}
	// Append newline since ReadLine strips it.
	full = append(full, '\n')
	r.offset += int64(len(full))
	return full, nil
}

// isFromLine checks if a line is an mbox "From " separator.
func isFromLine(line []byte) bool {
	return fromLineRe.Match(line)
}

// unescapeFromLine removes one leading '>' from mboxrd-escaped "From " lines.
// A line like ">From " becomes "From ", ">>From " becomes ">From ", etc.
func unescapeFromLine(line []byte) []byte {
	if len(line) > 1 && line[0] == '>' && escapedFromRe.Match(line) {
		return line[1:]
	}
	return line
}

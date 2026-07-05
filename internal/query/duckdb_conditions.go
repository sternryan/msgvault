package query

import (
	"fmt"
	"strings"

	"github.com/wesm/msgvault/internal/search"
)

// escapeILIKE escapes ILIKE wildcard characters (% and _) in user input.
func escapeILIKE(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\") // Escape backslash first
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// buildWhereClause builds WHERE conditions for Parquet queries.
// Column references use msg. prefix to be explicit since aggregate queries join multiple CTEs.
// buildAggregateSearchConditions builds SQL conditions for a search query in aggregate views.
// Returns conditions and args that can be appended to existing conditions.
// buildAggregateSearchConditions builds WHERE conditions for aggregate search.
// keyColumns are SQL expressions for the grouping dimension that text terms
// should filter on (e.g. "p.email_address", "p.display_name"). When nil,
// text terms search subject + sender (the default for Senders/Time views).
func (e *DuckDBEngine) buildAggregateSearchConditions(searchQuery string, keyColumns ...string) ([]string, []interface{}) {
	if searchQuery == "" {
		return nil, nil
	}

	var conditions []string
	var args []interface{}

	q := search.Parse(searchQuery)

	// Text terms: always search subject + sender, plus the view's grouping
	// key columns when provided (e.g., label name in Labels view).
	// Uses ILIKE for performance on Parquet scans.
	for _, term := range q.TextTerms {
		termPattern := "%" + escapeILIKE(term) + "%"
		var parts []string
		parts = append(parts, `msg.subject ILIKE ? ESCAPE '\'`)
		args = append(args, termPattern)
		parts = append(parts, `COALESCE(msg.snippet, '') ILIKE ? ESCAPE '\'`)
		args = append(args, termPattern)
		parts = append(parts, `EXISTS (
			SELECT 1 FROM mr mr_search
			JOIN p p_search ON p_search.id = mr_search.participant_id
			WHERE mr_search.message_id = msg.id
			  AND mr_search.recipient_type = 'from'
			  AND (p_search.email_address ILIKE ? ESCAPE '\' OR COALESCE(p_search.display_name, '') ILIKE ? ESCAPE '\')
		)`)
		args = append(args, termPattern, termPattern)
		for _, col := range keyColumns {
			parts = append(parts, col+` ILIKE ? ESCAPE '\'`)
			args = append(args, termPattern)
		}
		conditions = append(conditions, "("+strings.Join(parts, " OR ")+")")
	}

	// Append non-text filters (from:, to:, subject:, label:, has:, dates, sizes).
	nonTextConds, nonTextArgs := e.buildNonTextSearchConditions(q, keyColumns...)
	conditions = append(conditions, nonTextConds...)
	args = append(args, nonTextArgs...)

	return conditions, args
}

// buildNonTextSearchConditions builds WHERE conditions for the non-text
// portion of a parsed search query (from:, to:, subject:, label:, has:,
// date/size filters). Extracted from buildAggregateSearchConditions so
// callers that handle text terms themselves (e.g. buildStatsSearchConditions)
// can append non-text filters without having to compute how many args
// the text-term portion produced.
func (e *DuckDBEngine) buildNonTextSearchConditions(q *search.Query, keyColumns ...string) ([]string, []interface{}) {
	var conditions []string
	var args []interface{}

	// from: filter - match sender email
	for _, from := range q.FromAddrs {
		fromPattern := "%" + escapeILIKE(from) + "%"
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM mr mr_from
			JOIN p p_from ON p_from.id = mr_from.participant_id
			WHERE mr_from.message_id = msg.id
			  AND mr_from.recipient_type = 'from'
			  AND p_from.email_address ILIKE ? ESCAPE '\'
		)`)
		args = append(args, fromPattern)
	}

	// to: filter - match recipient email (to or cc, consistent with SearchFast)
	for _, to := range q.ToAddrs {
		toPattern := "%" + escapeILIKE(to) + "%"
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM mr mr_to
			JOIN p p_to ON p_to.id = mr_to.participant_id
			WHERE mr_to.message_id = msg.id
			  AND mr_to.recipient_type IN ('to', 'cc', 'bcc')
			  AND p_to.email_address ILIKE ? ESCAPE '\'
		)`)
		args = append(args, toPattern)
	}

	// subject: filter
	for _, subj := range q.SubjectTerms {
		subjPattern := "%" + escapeILIKE(subj) + "%"
		conditions = append(conditions, "msg.subject ILIKE ? ESCAPE '\\'")
		args = append(args, subjPattern)
	}

	// label: filter - case-insensitive substring match.
	// In the Labels aggregate view (keyColumns includes the label column),
	// filter the grouping column directly so only matching labels appear
	// in results — not all labels from matching messages.
	labelKeyCol := ""
	for _, col := range keyColumns {
		if strings.HasSuffix(col, ".name") &&
			strings.HasPrefix(col, "lbl") {
			labelKeyCol = col
			break
		}
	}
	if labelKeyCol != "" && len(q.Labels) > 0 {
		// Labels view: filter the grouped label column directly.
		// Use OR so label:arrow label:inbox shows both matching labels.
		var labelParts []string
		for _, label := range q.Labels {
			labelParts = append(labelParts, labelKeyCol+` ILIKE ? ESCAPE '\'`)
			args = append(args, "%"+escapeILIKE(label)+"%")
		}
		conditions = append(conditions, "("+strings.Join(labelParts, " OR ")+")")
	} else {
		// Non-label views: use EXISTS to filter messages by label.
		for _, label := range q.Labels {
			conditions = append(conditions, `EXISTS (
				SELECT 1 FROM ml ml_label
				JOIN lbl l_label ON l_label.id = ml_label.label_id
				WHERE ml_label.message_id = msg.id
				  AND l_label.name ILIKE ? ESCAPE '\'
			)`)
			args = append(args, "%"+escapeILIKE(label)+"%")
		}
	}

	// has:attachment filter
	if q.HasAttachment != nil && *q.HasAttachment {
		conditions = append(conditions, "msg.has_attachments = 1")
	}

	// Date filters from search query
	if q.AfterDate != nil {
		conditions = append(conditions, "msg.sent_at >= CAST(? AS TIMESTAMP)")
		args = append(args, q.AfterDate.Format("2006-01-02 15:04:05"))
	}
	if q.BeforeDate != nil {
		conditions = append(conditions, "msg.sent_at < CAST(? AS TIMESTAMP)")
		args = append(args, q.BeforeDate.Format("2006-01-02 15:04:05"))
	}

	// Size filters
	if q.LargerThan != nil {
		conditions = append(conditions, "msg.size_estimate > ?")
		args = append(args, *q.LargerThan)
	}
	if q.SmallerThan != nil {
		conditions = append(conditions, "msg.size_estimate < ?")
		args = append(args, *q.SmallerThan)
	}

	return conditions, args
}

// buildWhereClause builds WHERE conditions for aggregate queries.
// buildStatsSearchConditions builds search conditions for GetTotalStats.
// For 1:N views (Recipients, RecipientNames, Labels), text terms filter via
// EXISTS subqueries on the grouping dimension so stats match visible rows.
// For 1:1 views, falls back to the default subject+sender search.
func (e *DuckDBEngine) buildStatsSearchConditions(searchQuery string, groupBy ViewType) ([]string, []interface{}) {
	if searchQuery == "" {
		return nil, nil
	}

	q := search.Parse(searchQuery)

	var conditions []string
	var args []interface{}

	// Text terms — use EXISTS for 1:N views since the stats query has no
	// participant/label joins.
	for _, term := range q.TextTerms {
		termPattern := "%" + escapeILIKE(term) + "%"
		switch groupBy {
		case ViewRecipients, ViewRecipientNames:
			conditions = append(conditions, `EXISTS (
				SELECT 1 FROM mr mr_rs
				JOIN p p_rs ON p_rs.id = mr_rs.participant_id
				WHERE mr_rs.message_id = msg.id
				  AND mr_rs.recipient_type IN ('to', 'cc', 'bcc')
				  AND (p_rs.email_address ILIKE ? ESCAPE '\' OR p_rs.display_name ILIKE ? ESCAPE '\')
			)`)
			args = append(args, termPattern, termPattern)
		case ViewLabels:
			conditions = append(conditions, `EXISTS (
				SELECT 1 FROM ml ml_rs
				JOIN lbl lbl_rs ON lbl_rs.id = ml_rs.label_id
				WHERE ml_rs.message_id = msg.id
				  AND lbl_rs.name ILIKE ? ESCAPE '\'
			)`)
			args = append(args, termPattern)
		default:
			// Default: search subject, snippet, and sender
			conditions = append(conditions, `(
				msg.subject ILIKE ? ESCAPE '\' OR
				COALESCE(msg.snippet, '') ILIKE ? ESCAPE '\' OR
				EXISTS (
					SELECT 1 FROM mr mr_search
					JOIN p p_search ON p_search.id = mr_search.participant_id
					WHERE mr_search.message_id = msg.id
					  AND mr_search.recipient_type = 'from'
					  AND (p_search.email_address ILIKE ? ESCAPE '\' OR p_search.display_name ILIKE ? ESCAPE '\')
				)
			)`)
			args = append(args, termPattern, termPattern, termPattern, termPattern)
		}
	}

	// Non-text filters (from:, to:, subject:, label:, etc.) are the same
	// regardless of view — delegate to the non-text helper directly so we
	// don't have to track how many args the text-term portion emits.
	nonTextConds, nonTextArgs := e.buildNonTextSearchConditions(q)
	conditions = append(conditions, nonTextConds...)
	args = append(args, nonTextArgs...)

	return conditions, args
}

// keyColumns are passed through to buildAggregateSearchConditions to control
// which columns text search terms filter on.
func (e *DuckDBEngine) buildWhereClause(opts AggregateOptions, keyColumns ...string) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	// Exclude text messages from email-mode queries.
	// message_type IS NULL and '' handle old data without the column.
	conditions = append(conditions, "(msg.message_type = 'email' OR msg.message_type IS NULL OR msg.message_type = '')")

	if opts.SourceID != nil {
		conditions = append(conditions, "msg.source_id = ?")
		args = append(args, *opts.SourceID)
	}

	if opts.After != nil {
		conditions = append(conditions, "msg.sent_at >= CAST(? AS TIMESTAMP)")
		args = append(args, opts.After.Format("2006-01-02 15:04:05"))
	}

	if opts.Before != nil {
		conditions = append(conditions, "msg.sent_at < CAST(? AS TIMESTAMP)")
		args = append(args, opts.Before.Format("2006-01-02 15:04:05"))
	}

	if opts.WithAttachmentsOnly {
		conditions = append(conditions, "msg.has_attachments = 1")
	}
	if opts.HideDeletedFromSource {
		conditions = append(conditions, "msg.deleted_from_source_at IS NULL")
	}

	// Text search filter for aggregates - filter on view's key columns
	searchConds, searchArgs := e.buildAggregateSearchConditions(opts.SearchQuery, keyColumns...)
	conditions = append(conditions, searchConds...)
	args = append(args, searchArgs...)

	if len(conditions) == 0 {
		return "1=1", args
	}
	return strings.Join(conditions, " AND "), args
}

// buildFilterConditions builds WHERE conditions from a MessageFilter.
// Uses EXISTS subqueries for join-based filters (sender, recipient, label),
// which become semi-joins and avoid duplicates without needing DISTINCT.
func (e *DuckDBEngine) buildFilterConditions(filter MessageFilter) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	// Exclude text messages from email-mode queries.
	// message_type IS NULL and '' handle old data without the column.
	conditions = append(conditions, "(msg.message_type = 'email' OR msg.message_type IS NULL OR msg.message_type = '')")

	if filter.SourceID != nil {
		conditions = append(conditions, "msg.source_id = ?")
		args = append(args, *filter.SourceID)
	}

	if filter.ConversationID != nil {
		conditions = append(conditions, "msg.conversation_id = ?")
		args = append(args, *filter.ConversationID)
	}

	if filter.After != nil {
		conditions = append(conditions, "msg.sent_at >= CAST(? AS TIMESTAMP)")
		args = append(args, filter.After.Format("2006-01-02 15:04:05"))
	}

	if filter.Before != nil {
		conditions = append(conditions, "msg.sent_at < CAST(? AS TIMESTAMP)")
		args = append(args, filter.Before.Format("2006-01-02 15:04:05"))
	}

	if filter.WithAttachmentsOnly {
		conditions = append(conditions, "msg.has_attachments = true")
	}
	if filter.HideDeletedFromSource {
		conditions = append(conditions, "msg.deleted_from_source_at IS NULL")
	}

	// Sender filter - check both message_recipients (email) and direct sender_id (WhatsApp/chat)
	// Also checks phone_number for phone-based lookups (e.g., from:+447...)
	if filter.Sender != "" {
		conditions = append(conditions, `(EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type = 'from'
			  AND (p.email_address = ? OR p.phone_number = ?)
		) OR EXISTS (
			SELECT 1 FROM p
			WHERE p.id = msg.sender_id
			  AND (p.email_address = ? OR p.phone_number = ?)
		))`)
		args = append(args, filter.Sender, filter.Sender, filter.Sender, filter.Sender)
	} else if filter.MatchesEmpty(ViewSenders) {
		// A message has an "empty sender" only if it has no from-recipient AND no direct sender_id.
		conditions = append(conditions, `(NOT EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type = 'from'
			  AND (
			    (p.email_address IS NOT NULL AND p.email_address != '') OR
			    (p.phone_number IS NOT NULL AND p.phone_number != '')
			  )
		) AND msg.sender_id IS NULL)`)
	}

	// Sender name filter - check both message_recipients (email) and direct sender_id (WhatsApp/chat)
	if filter.SenderName != "" {
		conditions = append(conditions, `(EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type = 'from'
			  AND COALESCE(NULLIF(TRIM(p.display_name), ''), p.email_address) = ?
		) OR EXISTS (
			SELECT 1 FROM p
			WHERE p.id = msg.sender_id
			  AND COALESCE(NULLIF(TRIM(p.display_name), ''), p.email_address) = ?
		))`)
		args = append(args, filter.SenderName, filter.SenderName)
	} else if filter.MatchesEmpty(ViewSenderNames) {
		// A message has an "empty sender name" only if it has no from-recipient name AND no direct sender_id with a name.
		conditions = append(conditions, `(NOT EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type = 'from'
			  AND COALESCE(NULLIF(TRIM(p.display_name), ''), p.email_address) IS NOT NULL
		) AND NOT EXISTS (
			SELECT 1 FROM p
			WHERE p.id = msg.sender_id
			  AND COALESCE(NULLIF(TRIM(p.display_name), ''), p.email_address) IS NOT NULL
		))`)
	}

	// Recipient filter - use EXISTS subquery (becomes semi-join)
	if filter.Recipient != "" {
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type IN ('to', 'cc', 'bcc')
			  AND p.email_address = ?
		)`)
		args = append(args, filter.Recipient)
	} else if filter.MatchesEmpty(ViewRecipients) {
		conditions = append(conditions, "NOT EXISTS (SELECT 1 FROM mr WHERE mr.message_id = msg.id AND mr.recipient_type IN ('to', 'cc', 'bcc'))")
	}

	// Recipient name filter - use EXISTS subquery (becomes semi-join)
	if filter.RecipientName != "" {
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type IN ('to', 'cc', 'bcc')
			  AND COALESCE(NULLIF(TRIM(p.display_name), ''), p.email_address) = ?
		)`)
		args = append(args, filter.RecipientName)
	} else if filter.MatchesEmpty(ViewRecipientNames) {
		conditions = append(conditions, `NOT EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type IN ('to', 'cc', 'bcc')
			  AND COALESCE(NULLIF(TRIM(p.display_name), ''), p.email_address) IS NOT NULL
		)`)
	}

	// Domain filter - use EXISTS subquery (becomes semi-join)
	if filter.Domain != "" {
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type = 'from'
			  AND p.domain = ?
		)`)
		args = append(args, filter.Domain)
	} else if filter.MatchesEmpty(ViewDomains) {
		conditions = append(conditions, `NOT EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type = 'from'
			  AND p.domain IS NOT NULL
			  AND p.domain != ''
		)`)
	}

	// Label filter - case-insensitive EXISTS subquery (becomes semi-join)
	if filter.Label != "" {
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM ml
			JOIN lbl ON lbl.id = ml.label_id
			WHERE ml.message_id = msg.id
			  AND lbl.name ILIKE ? ESCAPE '\'
		)`)
		args = append(args, escapeILIKE(filter.Label))
	} else if filter.MatchesEmpty(ViewLabels) {
		conditions = append(conditions, "NOT EXISTS (SELECT 1 FROM ml WHERE ml.message_id = msg.id)")
	}

	// Time period filter
	if filter.TimeRange.Period != "" {
		granularity := inferTimeGranularity(filter.TimeRange.Granularity, filter.TimeRange.Period)
		conditions = append(conditions, fmt.Sprintf("%s = ?", timeExpr(granularity)))
		args = append(args, filter.TimeRange.Period)
	}

	if len(conditions) == 0 {
		return "1=1", args
	}
	return strings.Join(conditions, " AND "), args
}

// buildSearchConditions builds WHERE conditions for search queries.
// Shared by SearchFast and SearchFastCount.
// Note: These conditions reference msg and ms (msg_sender) CTEs.
func (e *DuckDBEngine) buildSearchConditions(q *search.Query, filter MessageFilter) ([]string, []interface{}) {
	var conditions []string
	var args []interface{}

	// Restrict to email messages only; NULL and '' handle pre-message_type data.
	conditions = append(conditions, emailOnlyFilterMsg)

	// Apply basic filter conditions (ignoring join flags for search - we handle those differently)
	if filter.SourceID != nil {
		conditions = append(conditions, "msg.source_id = ?")
		args = append(args, *filter.SourceID)
	}
	if filter.After != nil {
		conditions = append(conditions, "msg.sent_at >= CAST(? AS TIMESTAMP)")
		args = append(args, filter.After.Format("2006-01-02 15:04:05"))
	}
	if filter.Before != nil {
		conditions = append(conditions, "msg.sent_at < CAST(? AS TIMESTAMP)")
		args = append(args, filter.Before.Format("2006-01-02 15:04:05"))
	}
	if filter.WithAttachmentsOnly {
		conditions = append(conditions, "msg.has_attachments = true")
	}
	if filter.HideDeletedFromSource {
		conditions = append(conditions, "msg.deleted_from_source_at IS NULL")
	}
	// Sender filter - check both message_recipients (email/phone) and direct sender_id (WhatsApp/chat)
	if filter.Sender != "" {
		conditions = append(conditions, `(EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type = 'from'
			  AND (p.email_address = ? OR p.phone_number = ?)
		) OR EXISTS (
			SELECT 1 FROM p
			WHERE p.id = msg.sender_id
			  AND (p.email_address = ? OR p.phone_number = ?)
		))`)
		args = append(args, filter.Sender, filter.Sender, filter.Sender, filter.Sender)
	}
	if filter.Domain != "" {
		conditions = append(conditions, "ms.from_email ILIKE ?")
		args = append(args, "%@"+filter.Domain)
	}
	// Recipient filter - use EXISTS subquery for drill-down context (checks email and phone)
	if filter.Recipient != "" {
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM mr
			JOIN p ON p.id = mr.participant_id
			WHERE mr.message_id = msg.id
			  AND mr.recipient_type IN ('to', 'cc', 'bcc')
			  AND (p.email_address = ? OR p.phone_number = ?)
		)`)
		args = append(args, filter.Recipient, filter.Recipient)
	}
	// Label filter - use EXISTS subquery for drill-down context
	if filter.Label != "" {
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM ml
			JOIN lbl ON lbl.id = ml.label_id
			WHERE ml.message_id = msg.id
			  AND lbl.name ILIKE ? ESCAPE '\'
		)`)
		args = append(args, escapeILIKE(filter.Label))
	}
	if filter.TimeRange.Period != "" {
		granularity := inferTimeGranularity(filter.TimeRange.Granularity, filter.TimeRange.Period)
		conditions = append(conditions, fmt.Sprintf("%s = ?", timeExpr(granularity)))
		args = append(args, filter.TimeRange.Period)
	}

	// Text search terms - search subject, snippet, and from fields (fast path).
	// Uses ILIKE for performance on Parquet scans.
	if len(q.TextTerms) > 0 {
		for _, term := range q.TextTerms {
			termPattern := "%" + escapeILIKE(term) + "%"
			conditions = append(conditions, `(
				msg.subject ILIKE ? ESCAPE '\' OR
				COALESCE(msg.snippet, '') ILIKE ? ESCAPE '\' OR
				COALESCE(ms.from_email, ds.from_email, '') ILIKE ? ESCAPE '\' OR
				COALESCE(ms.from_name, ds.from_name, '') ILIKE ? ESCAPE '\' OR
				COALESCE(ms.from_phone, ds.from_phone, '') ILIKE ? ESCAPE '\'
			)`)
			args = append(args, termPattern, termPattern, termPattern, termPattern, termPattern)
		}
	}

	// From filter - check email, phone, display name via message_recipients and direct sender_id
	if len(q.FromAddrs) > 0 {
		for _, addr := range q.FromAddrs {
			pattern := "%" + escapeILIKE(addr) + "%"
			conditions = append(conditions, `(EXISTS (
				SELECT 1 FROM mr
				JOIN p ON p.id = mr.participant_id
				WHERE mr.message_id = msg.id
				  AND mr.recipient_type = 'from'
				  AND (p.email_address ILIKE ? ESCAPE '\' OR p.phone_number ILIKE ? ESCAPE '\' OR p.display_name ILIKE ? ESCAPE '\')
			) OR EXISTS (
				SELECT 1 FROM p
				WHERE p.id = msg.sender_id
				  AND (p.email_address ILIKE ? ESCAPE '\' OR p.phone_number ILIKE ? ESCAPE '\' OR p.display_name ILIKE ? ESCAPE '\')
			))`)
			args = append(args, pattern, pattern, pattern, pattern, pattern, pattern)
		}
	}

	// To filter - use EXISTS subquery to check recipients (email and phone)
	if len(q.ToAddrs) > 0 {
		for _, addr := range q.ToAddrs {
			pattern := "%" + escapeILIKE(addr) + "%"
			conditions = append(conditions, `EXISTS (
				SELECT 1 FROM mr
				JOIN p ON p.id = mr.participant_id
				WHERE mr.message_id = msg.id AND mr.recipient_type IN ('to', 'cc', 'bcc')
				AND (p.email_address ILIKE ? ESCAPE '\' OR p.phone_number ILIKE ? ESCAPE '\')
			)`)
			args = append(args, pattern, pattern)
		}
	}

	// Subject filter
	if len(q.SubjectTerms) > 0 {
		for _, term := range q.SubjectTerms {
			conditions = append(conditions, "msg.subject ILIKE ? ESCAPE '\\'")
			args = append(args, "%"+escapeILIKE(term)+"%")
		}
	}

	// Label filter - case-insensitive substring match
	if len(q.Labels) > 0 {
		for _, label := range q.Labels {
			conditions = append(conditions, `EXISTS (
				SELECT 1 FROM ml
				JOIN lbl ON lbl.id = ml.label_id
				WHERE ml.message_id = msg.id AND lbl.name ILIKE ? ESCAPE '\'
			)`)
			args = append(args, "%"+escapeILIKE(label)+"%")
		}
	}

	// Has attachment filter
	if q.HasAttachment != nil && *q.HasAttachment {
		conditions = append(conditions, "msg.has_attachments = 1")
	}

	// Date range filters
	if q.AfterDate != nil {
		conditions = append(conditions, "msg.sent_at >= CAST(? AS TIMESTAMP)")
		args = append(args, q.AfterDate.Format("2006-01-02 15:04:05"))
	}
	if q.BeforeDate != nil {
		conditions = append(conditions, "msg.sent_at < CAST(? AS TIMESTAMP)")
		args = append(args, q.BeforeDate.Format("2006-01-02 15:04:05"))
	}

	// Size filters
	if q.LargerThan != nil {
		conditions = append(conditions, "msg.size_estimate > ?")
		args = append(args, *q.LargerThan)
	}
	if q.SmallerThan != nil {
		conditions = append(conditions, "msg.size_estimate < ?")
		args = append(args, *q.SmallerThan)
	}

	// Account filter
	if q.AccountID != nil {
		conditions = append(conditions, "msg.source_id = ?")
		args = append(args, *q.AccountID)
	}

	// Default conditions if none specified
	if len(conditions) == 0 {
		conditions = append(conditions, "1=1")
	}

	return conditions, args
}

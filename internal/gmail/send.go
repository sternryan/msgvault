package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Send POSTs an RFC822-encoded message to the Gmail API and returns the
// resulting Gmail message ID. Requires the gmail.send OAuth scope on the
// active token; the caller is responsible for verifying scope availability
// before invoking Send (see oauth.Manager.HasScope).
//
// Per Phase 14 TRIAGE-06: this is the only network egress path of the
// triage pipeline. Body is opaque RFC822 bytes — Send does NOT parse,
// validate, or otherwise inspect them. Callers MUST sanitize headers
// (\r, \n) BEFORE passing them in (see internal/digest.BuildRFC822).
func (c *Client) Send(ctx context.Context, rfc822 []byte) (string, error) {
	if len(rfc822) == 0 {
		return "", fmt.Errorf("send: empty message")
	}
	body := struct {
		Raw string `json:"raw"`
	}{
		Raw: base64.URLEncoding.EncodeToString(rfc822),
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal send body: %w", err)
	}
	path := fmt.Sprintf("/users/%s/messages/send", c.userID)
	data, err := c.request(ctx, OpMessagesSend, "POST", path, bodyBytes)
	if err != nil {
		return "", fmt.Errorf("gmail send: %w", err)
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse send response: %w", err)
	}
	return resp.ID, nil
}

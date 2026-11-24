// Package whitelist implements SLACK_MCP_CHANNEL_WHITELIST, a read-side allow
// list of Slack conversation IDs. It is a fork addition, not part of upstream
// slack-mcp-server.
package whitelist

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// EnvVar holds the comma-separated conversation IDs that may be read.
const EnvVar = "SLACK_MCP_CHANNEL_WHITELIST"

// conversationIDPattern matches channel (C…), private channel or group DM (G…)
// and DM (D…) IDs.
var conversationIDPattern = regexp.MustCompile(`^[CGD][A-Z0-9]{8,}$`)

// Whitelist is an immutable set of conversation IDs. A nil *Whitelist means no
// whitelist is configured and every conversation is allowed (upstream behaviour).
type Whitelist struct {
	ids map[string]struct{}
}

// FromEnv reads EnvVar. Unset returns nil (no whitelist). Set but empty, or any
// malformed entry, is an error: an empty value would otherwise silently allow
// every conversation.
func FromEnv() (*Whitelist, error) {
	raw, ok := os.LookupEnv(EnvVar)
	if !ok {
		return nil, nil
	}
	return Parse(raw)
}

// Parse parses a comma-separated list of conversation IDs. Whitespace around
// entries is ignored. Names are not accepted because channels can be renamed.
func Parse(raw string) (*Whitelist, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("is set but empty; unset it to allow all conversations")
	}

	ids := make(map[string]struct{})
	for i, entry := range strings.Split(raw, ",") {
		id := strings.TrimSpace(entry)
		if id == "" {
			return nil, fmt.Errorf("entry %d is empty (stray comma or unset variable?)", i+1)
		}
		if !conversationIDPattern.MatchString(id) {
			return nil, fmt.Errorf("entry %d (%q) is not a conversation ID: expected an uppercase ID starting with C, G or D; names are not accepted", i+1, id)
		}
		ids[id] = struct{}{}
	}
	return &Whitelist{ids: ids}, nil
}

// Allows reports whether the conversation ID may be read. It matches exactly,
// so only the configured IDs themselves are allowed.
func (w *Whitelist) Allows(id string) bool {
	if w == nil {
		return true
	}
	_, ok := w.ids[id]
	return ok
}

// Len returns the number of whitelisted conversations (0 when not configured).
func (w *Whitelist) Len() int {
	if w == nil {
		return 0
	}
	return len(w.ids)
}

package handler

// Fork addition: SLACK_MCP_CHANNEL_WHITELIST enforcement.

import (
	"errors"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/whitelist"
	"go.uber.org/zap"
)

// errConversationNotWhitelisted deliberately names neither the conversation
// nor any of its content.
var errConversationNotWhitelisted = errors.New("access denied: conversation is not in " + whitelist.EnvVar)

// checkChannelWhitelist must be called with the conversation ID after any
// #name / @user has been resolved.
func (ch *ConversationsHandler) checkChannelWhitelist(channelID string) error {
	if ch.apiProvider.ChannelWhitelist().Allows(channelID) {
		return nil
	}
	ch.logger.Warn("Denied read of conversation not in channel whitelist", zap.String("channel", channelID))
	return errConversationNotWhitelisted
}

func filterChannelsByWhitelist(channels []provider.Channel, w *whitelist.Whitelist) []provider.Channel {
	if w == nil {
		return channels
	}
	var result []provider.Channel
	for _, c := range channels {
		if w.Allows(c.ID) {
			result = append(result, c)
		}
	}
	return result
}

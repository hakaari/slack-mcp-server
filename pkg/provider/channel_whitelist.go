package provider

// Fork addition: SLACK_MCP_CHANNEL_WHITELIST support.

import (
	"github.com/korotovsky/slack-mcp-server/pkg/whitelist"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// SetChannelWhitelist installs the read-side channel whitelist. Call it once at
// startup, before the server handles requests. nil means no whitelist.
func (ap *ApiProvider) SetChannelWhitelist(w *whitelist.Whitelist) {
	ap.channelWhitelist = w
}

// ChannelWhitelist returns the configured whitelist; nil allows every conversation.
func (ap *ApiProvider) ChannelWhitelist() *whitelist.Whitelist {
	return ap.channelWhitelist
}

// NewForTesting returns a provider backed by client whose users and channels
// caches are pre-populated and ready. It lets unit tests in other packages
// exercise handlers without a Slack connection.
func NewForTesting(client SlackAPI, users *UsersCache, channels *ChannelsCache) *ApiProvider {
	ap := &ApiProvider{
		transport:   "stdio",
		client:      client,
		logger:      zap.NewNop(),
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}
	ap.usersSnapshot.Store(users)
	ap.usersReady.Store(true)
	ap.channelsSnapshot.Store(channels)
	ap.channelsReady.Store(true)
	return ap
}

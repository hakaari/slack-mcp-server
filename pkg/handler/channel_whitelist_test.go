package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/whitelist"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	wlGeneral = "C0GENERAL1" // whitelisted public channel
	wlPrivate = "G0PRIVATE1" // whitelisted private channel
	wlDMAlice = "D0ALICE001" // whitelisted DM
	wlSecret  = "C0SECRET01" // not whitelisted
	wlDMBob   = "D0BOB00001" // not whitelisted DM

	wlUserAlice = "U0ALICE001"
	wlUserBob   = "U0BOB00001"

	testWhitelist = wlGeneral + ", " + wlPrivate + ", " + wlDMAlice
	testMessage   = "message text from the conversation"
)

// whitelistMockSlack records which conversations were requested from Slack.
// The embedded nil interface makes any call not implemented below panic, which
// flags an unexpected Slack request.
type whitelistMockSlack struct {
	provider.SlackAPI
	historyCalls []string
	repliesCalls []string
	forUser      []slack.Channel
}

func (m *whitelistMockSlack) AuthTest() (*slack.AuthTestResponse, error) {
	return &slack.AuthTestResponse{URL: "https://example.slack.com/"}, nil
}

func (m *whitelistMockSlack) GetConversationHistoryContext(_ context.Context, p *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error) {
	m.historyCalls = append(m.historyCalls, p.ChannelID)
	return &slack.GetConversationHistoryResponse{Messages: []slack.Message{testSlackMessage()}}, nil
}

func (m *whitelistMockSlack) GetConversationRepliesContext(_ context.Context, p *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
	m.repliesCalls = append(m.repliesCalls, p.ChannelID)
	return []slack.Message{testSlackMessage()}, false, "", nil
}

// GetConversationsContext backs the forced cache refresh that runs when a
// #name / @user is not in the cache.
func (m *whitelistMockSlack) GetConversationsContext(context.Context, *slack.GetConversationsParameters) ([]slack.Channel, string, error) {
	return nil, "", errors.New("refresh failed")
}

func (m *whitelistMockSlack) GetConversationsForUserContext(context.Context, *slack.GetConversationsForUserParameters) ([]slack.Channel, string, error) {
	return m.forUser, "", nil
}

func testSlackMessage() slack.Message {
	return slack.Message{Msg: slack.Msg{Text: testMessage, Timestamp: "1700000000.000100", User: wlUserAlice}}
}

func newWhitelistTestProvider(t *testing.T, raw string) (*whitelistMockSlack, *provider.ApiProvider) {
	t.Helper()

	users := &provider.UsersCache{
		Users: map[string]slack.User{
			wlUserAlice: {ID: wlUserAlice, Name: "alice"},
			wlUserBob:   {ID: wlUserBob, Name: "bob"},
		},
		UsersInv: map[string]string{"alice": wlUserAlice, "bob": wlUserBob},
	}
	channels := &provider.ChannelsCache{Channels: map[string]provider.Channel{}, ChannelsInv: map[string]string{}}
	for _, c := range []provider.Channel{
		{ID: wlGeneral, Name: "#general", Topic: "general topic", MemberCount: 10},
		{ID: wlPrivate, Name: "#private", IsPrivate: true, MemberCount: 4},
		{ID: wlDMAlice, Name: "@alice", IsIM: true, User: wlUserAlice},
		{ID: wlSecret, Name: "#secret", Topic: "secret topic", MemberCount: 3},
		{ID: wlDMBob, Name: "@bob", IsIM: true, User: wlUserBob},
	} {
		channels.Channels[c.ID] = c
		channels.ChannelsInv[c.Name] = c.ID
	}

	mock := &whitelistMockSlack{}
	p := provider.NewForTesting(mock, users, channels)
	if raw != "" {
		w, err := whitelist.Parse(raw)
		require.NoError(t, err)
		p.SetChannelWhitelist(w)
	}
	return mock, p
}

func toolResultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, res)
	require.Len(t, res.Content, 1)
	tc, ok := res.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected text content, got %T", res.Content[0])
	return tc.Text
}

type conversationTool struct {
	name    string
	handler func(*ConversationsHandler, context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	calls   func(*whitelistMockSlack) []string
}

var conversationTools = []conversationTool{
	{
		name:    "conversations_history",
		handler: (*ConversationsHandler).ConversationsHistoryHandler,
		calls:   func(m *whitelistMockSlack) []string { return m.historyCalls },
	},
	{
		name:    "conversations_replies",
		handler: (*ConversationsHandler).ConversationsRepliesHandler,
		calls:   func(m *whitelistMockSlack) []string { return m.repliesCalls },
	},
}

func conversationRequest(channel string) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"channel_id": channel,
		"thread_ts":  "1700000000.000100", // ignored by history
	}
	return req
}

func TestUnitChannelWhitelistHistoryAndReplies(t *testing.T) {
	cases := []struct {
		name    string
		channel string
		wantID  string // conversation sent to Slack; empty means denied
		wantErr string // for denials
	}{
		{"whitelisted channel ID", wlGeneral, wlGeneral, ""},
		{"whitelisted #name", "#general", wlGeneral, ""},
		{"whitelisted private channel", wlPrivate, wlPrivate, ""},
		{"whitelisted DM ID", wlDMAlice, wlDMAlice, ""},
		{"whitelisted @user DM", "@alice", wlDMAlice, ""},

		{"non-whitelisted channel ID", wlSecret, "", errConversationNotWhitelisted.Error()},
		{"non-whitelisted #name", "#secret", "", errConversationNotWhitelisted.Error()},
		{"non-whitelisted DM ID", wlDMBob, "", errConversationNotWhitelisted.Error()},
		{"non-whitelisted @user DM", "@bob", "", errConversationNotWhitelisted.Error()},
		{"lowercase whitelisted ID", strings.ToLower(wlGeneral), "", errConversationNotWhitelisted.Error()},
		{"padded whitelisted ID", " " + wlGeneral, "", errConversationNotWhitelisted.Error()},
		{"user ID instead of DM ID", wlUserAlice, "", errConversationNotWhitelisted.Error()},
		{"bare channel name", "general", "", errConversationNotWhitelisted.Error()},

		{"unresolvable #name", "#does-not-exist", "", "not found"},
		{"unresolvable @user", "@nobody", "", "not found"},
	}

	for _, tool := range conversationTools {
		for _, tc := range cases {
			t.Run(tool.name+"/"+tc.name, func(t *testing.T) {
				mock, p := newWhitelistTestProvider(t, testWhitelist)
				h := NewConversationsHandler(p, zap.NewNop())

				res, err := tool.handler(h, context.Background(), conversationRequest(tc.channel))

				if tc.wantID == "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tc.wantErr)
					assert.NotContains(t, err.Error(), testMessage)
					assert.Nil(t, res)
					assert.Empty(t, tool.calls(mock), "Slack must not be called for a denied conversation")
					return
				}

				require.NoError(t, err)
				assert.Equal(t, []string{tc.wantID}, tool.calls(mock))
				assert.Contains(t, toolResultText(t, res), testMessage)
			})
		}
	}
}

func TestUnitChannelWhitelistUnsetAllowsAll(t *testing.T) {
	for _, tool := range conversationTools {
		t.Run(tool.name, func(t *testing.T) {
			mock, p := newWhitelistTestProvider(t, "")
			h := NewConversationsHandler(p, zap.NewNop())

			_, err := tool.handler(h, context.Background(), conversationRequest("#secret"))
			require.NoError(t, err)
			assert.Equal(t, []string{wlSecret}, tool.calls(mock))
		})
	}
}

func TestUnitChannelWhitelistChannelsList(t *testing.T) {
	_, p := newWhitelistTestProvider(t, testWhitelist)
	h := NewChannelsHandler(p, zap.NewNop())

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"channel_types": "public_channel,private_channel,im,mpim"}
	res, err := h.ChannelsHandler(context.Background(), req)
	require.NoError(t, err)
	text := toolResultText(t, res)
	for _, id := range []string{wlGeneral, wlPrivate, wlDMAlice} {
		assert.Contains(t, text, id)
	}
	for _, s := range []string{wlSecret, wlDMBob, "#secret", "secret topic", "@bob"} {
		assert.NotContains(t, text, s)
	}

	t.Run("query cannot surface a non-whitelisted channel", func(t *testing.T) {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"channel_types": "public_channel,private_channel,im,mpim",
			"query":         "secret",
			"query_targets": "name,topic,purpose",
		}
		res, err := h.ChannelsHandler(context.Background(), req)
		require.NoError(t, err)
		text := toolResultText(t, res)
		assert.NotContains(t, text, wlSecret)
		assert.NotContains(t, text, "secret topic")
	})
}

func TestUnitChannelWhitelistChannelsMe(t *testing.T) {
	mock, p := newWhitelistTestProvider(t, testWhitelist)
	mock.forUser = []slack.Channel{
		{GroupConversation: slack.GroupConversation{Conversation: slack.Conversation{ID: wlSecret}, Name: "secret"}},
		{GroupConversation: slack.GroupConversation{Conversation: slack.Conversation{ID: wlDMBob, IsIM: true, User: wlUserBob}}},
		{GroupConversation: slack.GroupConversation{Conversation: slack.Conversation{ID: wlGeneral}, Name: "general"}},
		{GroupConversation: slack.GroupConversation{Conversation: slack.Conversation{ID: wlDMAlice, IsIM: true, User: wlUserAlice}}},
	}
	h := NewChannelsHandler(p, zap.NewNop())

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"channel_types": "public_channel,private_channel,im,mpim"}
	res, err := h.ChannelsMeHandler(context.Background(), req)
	require.NoError(t, err)
	text := toolResultText(t, res)
	assert.Contains(t, text, wlGeneral)
	assert.Contains(t, text, wlDMAlice)
	assert.NotContains(t, text, wlSecret)
	assert.NotContains(t, text, wlDMBob)

	t.Run("limit counts only whitelisted conversations", func(t *testing.T) {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{"channel_types": "public_channel", "limit": 1}
		res, err := h.ChannelsMeHandler(context.Background(), req)
		require.NoError(t, err)
		text := toolResultText(t, res)
		assert.Contains(t, text, wlGeneral)
		assert.NotContains(t, text, wlSecret)
	})
}

func TestUnitChannelWhitelistChannelsResource(t *testing.T) {
	_, p := newWhitelistTestProvider(t, testWhitelist)
	h := NewChannelsHandler(p, zap.NewNop())

	contents, err := h.ChannelsResource(context.Background(), mcp.ReadResourceRequest{})
	require.NoError(t, err)
	require.Len(t, contents, 1)
	trc, ok := contents[0].(mcp.TextResourceContents)
	require.True(t, ok, "expected text resource, got %T", contents[0])
	for _, id := range []string{wlGeneral, wlPrivate, wlDMAlice} {
		assert.Contains(t, trc.Text, id)
	}
	for _, s := range []string{wlSecret, wlDMBob, "secret topic"} {
		assert.NotContains(t, trc.Text, s)
	}
}

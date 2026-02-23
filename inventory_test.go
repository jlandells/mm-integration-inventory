package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// --- Mock client ---

type mockClient struct {
	incomingWebhooks map[int][]*model.IncomingWebhook // page -> hooks
	outgoingWebhooks map[int][]*model.OutgoingWebhook
	bots             map[int][]*model.Bot
	oauthApps        map[int][]*model.OAuthApp
	commands         map[string][]*model.Command // teamID -> commands
	users            map[string]*model.User
	teams            map[string]*model.Team
	teamsByName      map[string]*model.Team
	allTeams         map[int][]*model.Team
	channels         map[string]*model.Channel

	// Simulate 403 on specific fetch methods
	forbidBots      bool
	forbidOAuthApps bool

	// Track calls for cache verification
	getUserCalls int
	getTeamCalls int
}

func newMockClient() *mockClient {
	return &mockClient{
		incomingWebhooks: make(map[int][]*model.IncomingWebhook),
		outgoingWebhooks: make(map[int][]*model.OutgoingWebhook),
		bots:             make(map[int][]*model.Bot),
		oauthApps:        make(map[int][]*model.OAuthApp),
		commands:         make(map[string][]*model.Command),
		users:            make(map[string]*model.User),
		teams:            make(map[string]*model.Team),
		teamsByName:      make(map[string]*model.Team),
		allTeams:         make(map[int][]*model.Team),
		channels:         make(map[string]*model.Channel),
	}
}

func (m *mockClient) GetIncomingWebhooks(page, perPage int) ([]*model.IncomingWebhook, error) {
	if hooks, ok := m.incomingWebhooks[page]; ok {
		return hooks, nil
	}
	return nil, nil
}

func (m *mockClient) GetOutgoingWebhooks(page, perPage int) ([]*model.OutgoingWebhook, error) {
	if hooks, ok := m.outgoingWebhooks[page]; ok {
		return hooks, nil
	}
	return nil, nil
}

func (m *mockClient) GetBots(page, perPage int) ([]*model.Bot, error) {
	if m.forbidBots {
		return nil, &APIError{Message: "error: permission denied while fetching bots", StatusCode: http.StatusForbidden}
	}
	if bots, ok := m.bots[page]; ok {
		return bots, nil
	}
	return nil, nil
}

func (m *mockClient) GetOAuthApps(page, perPage int) ([]*model.OAuthApp, error) {
	if m.forbidOAuthApps {
		return nil, &APIError{Message: "error: permission denied while fetching OAuth apps", StatusCode: http.StatusForbidden}
	}
	if apps, ok := m.oauthApps[page]; ok {
		return apps, nil
	}
	return nil, nil
}

func (m *mockClient) GetCommands(teamID string) ([]*model.Command, error) {
	if cmds, ok := m.commands[teamID]; ok {
		return cmds, nil
	}
	return nil, nil
}

func (m *mockClient) GetUser(userID string) (*model.User, error) {
	m.getUserCalls++
	if user, ok := m.users[userID]; ok {
		return user, nil
	}
	return nil, &APIError{Message: fmt.Sprintf("user %s not found", userID), StatusCode: http.StatusNotFound}
}

func (m *mockClient) GetTeam(teamID string) (*model.Team, error) {
	m.getTeamCalls++
	if team, ok := m.teams[teamID]; ok {
		return team, nil
	}
	return nil, &APIError{Message: fmt.Sprintf("team %s not found", teamID), StatusCode: http.StatusNotFound}
}

func (m *mockClient) GetTeamByName(name string) (*model.Team, error) {
	if team, ok := m.teamsByName[name]; ok {
		return team, nil
	}
	return nil, &APIError{Message: fmt.Sprintf("team %q not found", name), StatusCode: http.StatusNotFound}
}

func (m *mockClient) GetChannel(channelID string) (*model.Channel, error) {
	if ch, ok := m.channels[channelID]; ok {
		return ch, nil
	}
	return nil, &APIError{Message: fmt.Sprintf("channel %s not found", channelID), StatusCode: http.StatusNotFound}
}

func (m *mockClient) GetAllTeams(page, perPage int) ([]*model.Team, error) {
	if teams, ok := m.allTeams[page]; ok {
		return teams, nil
	}
	return nil, nil
}

// --- Fixture helpers ---

func fixtureTeam(id, name, displayName string) *model.Team {
	return &model.Team{
		Id:          id,
		Name:        name,
		DisplayName: displayName,
	}
}

func fixtureUser(id, username, first, last string, deleteAt int64) *model.User {
	return &model.User{
		Id:        id,
		Username:  username,
		FirstName: first,
		LastName:  last,
		DeleteAt:  deleteAt,
	}
}

func fixtureChannel(id, name, displayName string) *model.Channel {
	return &model.Channel{
		Id:          id,
		Name:        name,
		DisplayName: displayName,
	}
}

// --- Tests ---

func TestOrphanDetection(t *testing.T) {
	tests := []struct {
		name           string
		user           *model.User
		userExists     bool
		expectStatus   CreatorStatus
		expectOrphaned bool
	}{
		{
			name:           "active user",
			user:           fixtureUser("u1", "alice", "Alice", "Johnson", 0),
			userExists:     true,
			expectStatus:   StatusActive,
			expectOrphaned: false,
		},
		{
			name:           "deactivated user (DeleteAt > 0)",
			user:           fixtureUser("u2", "bob.smith", "Bob", "Smith", 1700000000000),
			userExists:     true,
			expectStatus:   StatusDeactivated,
			expectOrphaned: true,
		},
		{
			name:           "deleted user (404)",
			user:           nil,
			userExists:     false,
			expectStatus:   StatusDeleted,
			expectOrphaned: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMockClient()
			if tt.userExists {
				mc.users[tt.user.Id] = tt.user
			}

			fetcher := NewInventoryFetcher(mc, false)
			ig := &Integration{}

			userID := "u_missing"
			if tt.user != nil {
				userID = tt.user.Id
			}
			fetcher.resolveCreator(userID, ig)

			if ig.CreatorStatus != tt.expectStatus {
				t.Errorf("CreatorStatus = %q, want %q", ig.CreatorStatus, tt.expectStatus)
			}
			if ig.Orphaned != tt.expectOrphaned {
				t.Errorf("Orphaned = %v, want %v", ig.Orphaned, tt.expectOrphaned)
			}
		})
	}
}

func TestOrphanDetectionEmptyUserID(t *testing.T) {
	mc := newMockClient()
	fetcher := NewInventoryFetcher(mc, false)
	ig := &Integration{}

	fetcher.resolveCreator("", ig)

	if ig.CreatorStatus != StatusUnknown {
		t.Errorf("CreatorStatus = %q, want %q", ig.CreatorStatus, StatusUnknown)
	}
	if !ig.Orphaned {
		t.Error("Orphaned = false, want true for empty userID")
	}
}

func TestUserCaching(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)

	fetcher := NewInventoryFetcher(mc, false)

	// First call should hit the API
	ig1 := &Integration{}
	fetcher.resolveCreator("u1", ig1)

	// Second call should hit cache
	ig2 := &Integration{}
	fetcher.resolveCreator("u1", ig2)

	if mc.getUserCalls != 1 {
		t.Errorf("GetUser called %d times, want 1 (second call should be cached)", mc.getUserCalls)
	}
	if ig1.CreatorUsername != ig2.CreatorUsername {
		t.Errorf("Cached result differs: %q vs %q", ig1.CreatorUsername, ig2.CreatorUsername)
	}
}

func TestTeamCaching(t *testing.T) {
	mc := newMockClient()
	mc.teams["t1"] = fixtureTeam("t1", "engineering", "Engineering")

	fetcher := NewInventoryFetcher(mc, false)

	ig1 := &Integration{}
	fetcher.resolveTeam("t1", ig1)

	ig2 := &Integration{}
	fetcher.resolveTeam("t1", ig2)

	if mc.getTeamCalls != 1 {
		t.Errorf("GetTeam called %d times, want 1", mc.getTeamCalls)
	}
	if ig1.Team != "Engineering" || ig2.Team != "Engineering" {
		t.Errorf("Team resolution failed: %q, %q", ig1.Team, ig2.Team)
	}
}

func TestPerTypeMapping(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)
	mc.teams["t1"] = fixtureTeam("t1", "engineering", "Engineering")
	mc.channels["c1"] = fixtureChannel("c1", "dev-ops", "Dev Ops")

	created := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC).UnixMilli()

	t.Run("incoming webhook uses UserId for creator", func(t *testing.T) {
		mc.incomingWebhooks[0] = []*model.IncomingWebhook{
			{Id: "iw1", DisplayName: "CI Webhook", UserId: "u1", TeamId: "t1", ChannelId: "c1", Description: "Build notifications", CreateAt: created},
		}
		fetcher := NewInventoryFetcher(mc, false)
		items, err := fetcher.fetchIncomingWebhooks("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
		if items[0].CreatorUsername != "alice" {
			t.Errorf("CreatorUsername = %q, want %q", items[0].CreatorUsername, "alice")
		}
		if items[0].Type != TypeIncomingWebhook {
			t.Errorf("Type = %q, want %q", items[0].Type, TypeIncomingWebhook)
		}
	})

	t.Run("outgoing webhook uses CreatorId", func(t *testing.T) {
		mc.outgoingWebhooks[0] = []*model.OutgoingWebhook{
			{Id: "ow1", DisplayName: "Alert Hook", CreatorId: "u1", TeamId: "t1", ChannelId: "c1", Description: "Alerts", CreateAt: created},
		}
		fetcher := NewInventoryFetcher(mc, false)
		items, err := fetcher.fetchOutgoingWebhooks("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
		if items[0].CreatorUsername != "alice" {
			t.Errorf("CreatorUsername = %q, want %q", items[0].CreatorUsername, "alice")
		}
	})

	t.Run("bot uses OwnerId not UserId for creator", func(t *testing.T) {
		mc.bots[0] = []*model.Bot{
			{UserId: "bot-user-1", DisplayName: "Deploy Bot", OwnerId: "u1", Description: "Handles deploys", CreateAt: created},
		}
		fetcher := NewInventoryFetcher(mc, false)
		items, err := fetcher.fetchBots()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
		if items[0].CreatorUsername != "alice" {
			t.Errorf("CreatorUsername = %q, want %q (should resolve OwnerId, not UserId)", items[0].CreatorUsername, "alice")
		}
		if items[0].ID != "bot-user-1" {
			t.Errorf("ID = %q, want %q", items[0].ID, "bot-user-1")
		}
		if items[0].Team != "All Teams" {
			t.Errorf("Team = %q, want %q", items[0].Team, "All Teams")
		}
	})

	t.Run("oauth app uses CreatorId", func(t *testing.T) {
		mc.oauthApps[0] = []*model.OAuthApp{
			{Id: "oa1", Name: "JIRA Integration", CreatorId: "u1", Description: "JIRA sync", CreateAt: created},
		}
		fetcher := NewInventoryFetcher(mc, false)
		items, err := fetcher.fetchOAuthApps()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
		if items[0].CreatorUsername != "alice" {
			t.Errorf("CreatorUsername = %q, want %q", items[0].CreatorUsername, "alice")
		}
		if items[0].Team != "All Teams" {
			t.Errorf("Team = %q, want %q", items[0].Team, "All Teams")
		}
	})

	t.Run("slash command uses CreatorId", func(t *testing.T) {
		mc.allTeams[0] = []*model.Team{fixtureTeam("t1", "engineering", "Engineering")}
		mc.commands["t1"] = []*model.Command{
			{Id: "cmd1", DisplayName: "Deploy", Trigger: "deploy", CreatorId: "u1", TeamId: "t1", Description: "Deploy to prod", CreateAt: created},
		}
		fetcher := NewInventoryFetcher(mc, false)
		items, err := fetcher.fetchSlashCommands("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
		if items[0].CreatorUsername != "alice" {
			t.Errorf("CreatorUsername = %q, want %q", items[0].CreatorUsername, "alice")
		}
	})
}

func TestSlashCommandFallbackToTrigger(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)
	mc.teams["t1"] = fixtureTeam("t1", "engineering", "Engineering")
	mc.allTeams[0] = []*model.Team{fixtureTeam("t1", "engineering", "Engineering")}
	mc.commands["t1"] = []*model.Command{
		{Id: "cmd1", DisplayName: "", Trigger: "standup", CreatorId: "u1", TeamId: "t1", CreateAt: 1710000000000},
	}

	fetcher := NewInventoryFetcher(mc, false)
	items, err := fetcher.fetchSlashCommands("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items[0].Name != "/standup" {
		t.Errorf("Name = %q, want %q (should fall back to /trigger)", items[0].Name, "/standup")
	}
}

func TestTeamScoping(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)
	mc.teams["t1"] = fixtureTeam("t1", "engineering", "Engineering")
	mc.teams["t2"] = fixtureTeam("t2", "marketing", "Marketing")
	mc.teamsByName["engineering"] = fixtureTeam("t1", "engineering", "Engineering")
	mc.channels["c1"] = fixtureChannel("c1", "dev-ops", "Dev Ops")
	mc.channels["c2"] = fixtureChannel("c2", "campaigns", "Campaigns")

	created := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC).UnixMilli()

	// Webhooks in both teams
	mc.incomingWebhooks[0] = []*model.IncomingWebhook{
		{Id: "iw1", DisplayName: "Eng Hook", UserId: "u1", TeamId: "t1", ChannelId: "c1", CreateAt: created},
		{Id: "iw2", DisplayName: "Mktg Hook", UserId: "u1", TeamId: "t2", ChannelId: "c2", CreateAt: created},
	}

	// Bots are instance-wide
	mc.bots[0] = []*model.Bot{
		{UserId: "bot1", DisplayName: "Global Bot", OwnerId: "u1", CreateAt: created},
	}

	// Commands per team
	mc.commands["t1"] = []*model.Command{
		{Id: "cmd1", DisplayName: "Eng Cmd", Trigger: "eng", CreatorId: "u1", TeamId: "t1", CreateAt: created},
	}

	fetcher := NewInventoryFetcher(mc, false)
	result, err := fetcher.FetchInventory("engineering", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should include: 1 incoming webhook (eng only), bot (instance-wide), 1 slash command
	// Should exclude: marketing webhook
	typeCount := make(map[IntegrationType]int)
	for _, ig := range result.Integrations {
		typeCount[ig.Type]++
		if ig.Type == TypeIncomingWebhook && ig.Name == "Mktg Hook" {
			t.Error("Marketing webhook should be excluded when filtering by engineering team")
		}
	}

	if typeCount[TypeIncomingWebhook] != 1 {
		t.Errorf("incoming webhooks = %d, want 1", typeCount[TypeIncomingWebhook])
	}
	if typeCount[TypeBot] != 1 {
		t.Errorf("bots = %d, want 1 (bots are instance-wide, always included)", typeCount[TypeBot])
	}
}

func TestTeamScopingUnknownTeam(t *testing.T) {
	mc := newMockClient()
	fetcher := NewInventoryFetcher(mc, false)
	_, err := fetcher.FetchInventory("nonexistent", "", false)
	if err == nil {
		t.Fatal("expected error for unknown team")
	}
	if _, ok := err.(*ConfigError); !ok {
		t.Errorf("expected ConfigError, got %T", err)
	}
}

func TestTypeFiltering(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)
	mc.teams["t1"] = fixtureTeam("t1", "engineering", "Engineering")
	mc.channels["c1"] = fixtureChannel("c1", "dev-ops", "Dev Ops")

	created := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC).UnixMilli()
	mc.incomingWebhooks[0] = []*model.IncomingWebhook{
		{Id: "iw1", DisplayName: "Eng Hook", UserId: "u1", TeamId: "t1", ChannelId: "c1", CreateAt: created},
	}
	mc.bots[0] = []*model.Bot{
		{UserId: "bot1", DisplayName: "Global Bot", OwnerId: "u1", CreateAt: created},
	}

	fetcher := NewInventoryFetcher(mc, false)
	result, err := fetcher.FetchInventory("", TypeIncomingWebhook, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Integrations) != 1 {
		t.Fatalf("got %d integrations, want 1", len(result.Integrations))
	}
	if result.Integrations[0].Type != TypeIncomingWebhook {
		t.Errorf("Type = %q, want %q", result.Integrations[0].Type, TypeIncomingWebhook)
	}
}

func TestForbiddenTypeSkippedWhenNoFilter(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)
	mc.teams["t1"] = fixtureTeam("t1", "engineering", "Engineering")
	mc.channels["c1"] = fixtureChannel("c1", "dev-ops", "Dev Ops")
	mc.allTeams[0] = []*model.Team{fixtureTeam("t1", "engineering", "Engineering")}

	created := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC).UnixMilli()
	mc.incomingWebhooks[0] = []*model.IncomingWebhook{
		{Id: "iw1", DisplayName: "CI Webhook", UserId: "u1", TeamId: "t1", ChannelId: "c1", CreateAt: created},
	}
	// Bots and OAuth apps return 403 (feature disabled)
	mc.forbidBots = true
	mc.forbidOAuthApps = true

	fetcher := NewInventoryFetcher(mc, false)
	result, err := fetcher.FetchInventory("", "", false)

	// Should succeed with a partial warning, not fail
	if err != nil {
		if _, ok := err.(*PartialError); !ok {
			t.Fatalf("expected nil or PartialError, got %T: %v", err, err)
		}
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Should still contain the webhook that succeeded
	found := false
	for _, ig := range result.Integrations {
		if ig.Type == TypeIncomingWebhook {
			found = true
		}
		if ig.Type == TypeBot || ig.Type == TypeOAuthApp {
			t.Errorf("should not contain %s when that type returned 403", ig.Type)
		}
	}
	if !found {
		t.Error("incoming webhook should still be present")
	}
}

func TestForbiddenTypeFatalWhenExplicitFilter(t *testing.T) {
	mc := newMockClient()
	mc.forbidBots = true

	fetcher := NewInventoryFetcher(mc, false)
	_, err := fetcher.FetchInventory("", TypeBot, false)

	if err == nil {
		t.Fatal("expected error when explicitly filtered type returns 403")
	}
	if !isForbidden(err) {
		t.Errorf("expected forbidden error, got %T: %v", err, err)
	}
}

func TestOrphanedOnlyFilter(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)
	mc.users["u2"] = fixtureUser("u2", "bob.smith", "Bob", "Smith", 1700000000000)
	mc.teams["t1"] = fixtureTeam("t1", "engineering", "Engineering")
	mc.channels["c1"] = fixtureChannel("c1", "dev-ops", "Dev Ops")

	created := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC).UnixMilli()
	mc.incomingWebhooks[0] = []*model.IncomingWebhook{
		{Id: "iw1", DisplayName: "Active Hook", UserId: "u1", TeamId: "t1", ChannelId: "c1", CreateAt: created},
		{Id: "iw2", DisplayName: "Orphaned Hook", UserId: "u2", TeamId: "t1", ChannelId: "c1", CreateAt: created},
	}

	fetcher := NewInventoryFetcher(mc, false)
	result, err := fetcher.FetchInventory("", TypeIncomingWebhook, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Integrations) != 1 {
		t.Fatalf("got %d integrations, want 1 (orphaned only)", len(result.Integrations))
	}
	if result.Integrations[0].Name != "Orphaned Hook" {
		t.Errorf("Name = %q, want %q", result.Integrations[0].Name, "Orphaned Hook")
	}
}

func TestPaginationSinglePage(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)
	mc.teams["t1"] = fixtureTeam("t1", "engineering", "Engineering")
	mc.channels["c1"] = fixtureChannel("c1", "dev-ops", "Dev Ops")

	mc.incomingWebhooks[0] = []*model.IncomingWebhook{
		{Id: "iw1", DisplayName: "Hook 1", UserId: "u1", TeamId: "t1", ChannelId: "c1", CreateAt: 1710000000000},
	}
	// No page 1 → single page

	fetcher := NewInventoryFetcher(mc, false)
	items, err := fetcher.fetchIncomingWebhooks("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("got %d items, want 1", len(items))
	}
}

func TestPaginationMultiplePages(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)

	// Simulate exactly perPage items on page 0 (boundary — must fetch next page)
	page0Bots := make([]*model.Bot, perPage)
	for i := range page0Bots {
		page0Bots[i] = &model.Bot{
			UserId:      fmt.Sprintf("bot-p0-%d", i),
			DisplayName: fmt.Sprintf("Bot %d", i),
			OwnerId:     "u1",
			CreateAt:    1710000000000,
		}
	}
	mc.bots[0] = page0Bots

	// Page 1 has fewer than perPage → last page
	mc.bots[1] = []*model.Bot{
		{UserId: "bot-p1-0", DisplayName: "Last Bot", OwnerId: "u1", CreateAt: 1710000000000},
	}

	fetcher := NewInventoryFetcher(mc, false)
	items, err := fetcher.fetchBots()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != perPage+1 {
		t.Errorf("got %d items, want %d", len(items), perPage+1)
	}
}

func TestPaginationEmptyResult(t *testing.T) {
	mc := newMockClient()
	// No data at all

	fetcher := NewInventoryFetcher(mc, false)
	items, err := fetcher.fetchBots()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("got %d items, want 0", len(items))
	}
}

func TestSlashCommandDedup(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)
	mc.teams["t1"] = fixtureTeam("t1", "engineering", "Engineering")
	mc.teams["t2"] = fixtureTeam("t2", "marketing", "Marketing")
	mc.allTeams[0] = []*model.Team{
		fixtureTeam("t1", "engineering", "Engineering"),
		fixtureTeam("t2", "marketing", "Marketing"),
	}

	// Same command ID in both teams — should be deduped
	sharedCmd := &model.Command{
		Id: "cmd1", DisplayName: "Deploy", Trigger: "deploy",
		CreatorId: "u1", TeamId: "t1", CreateAt: 1710000000000,
	}
	mc.commands["t1"] = []*model.Command{sharedCmd}
	mc.commands["t2"] = []*model.Command{sharedCmd}

	fetcher := NewInventoryFetcher(mc, false)
	items, err := fetcher.fetchSlashCommands("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("got %d items, want 1 (should deduplicate)", len(items))
	}
}

func TestPartialFailureDoesNotAbort(t *testing.T) {
	mc := newMockClient()
	// User u1 exists, u2 does not, u3 returns non-404 error
	mc.users["u1"] = fixtureUser("u1", "alice", "Alice", "Johnson", 0)
	// u2 will return 404 (deleted) — handled gracefully
	mc.teams["t1"] = fixtureTeam("t1", "engineering", "Engineering")
	mc.channels["c1"] = fixtureChannel("c1", "dev-ops", "Dev Ops")

	mc.incomingWebhooks[0] = []*model.IncomingWebhook{
		{Id: "iw1", DisplayName: "Good Hook", UserId: "u1", TeamId: "t1", ChannelId: "c1", CreateAt: 1710000000000},
		{Id: "iw2", DisplayName: "Orphan Hook", UserId: "u2", TeamId: "t1", ChannelId: "c1", CreateAt: 1710000000000},
	}

	fetcher := NewInventoryFetcher(mc, false)
	result, err := fetcher.FetchInventory("", TypeIncomingWebhook, false)
	// Should not return fatal error — deleted users are handled
	if err != nil {
		t.Fatalf("unexpected error: %v (deleted user should be handled gracefully)", err)
	}

	if len(result.Integrations) != 2 {
		t.Errorf("got %d integrations, want 2 (both should be present)", len(result.Integrations))
	}
}

func TestSummaryBuild(t *testing.T) {
	integrations := []Integration{
		{Type: TypeIncomingWebhook, CreatorStatus: StatusActive, Orphaned: false},
		{Type: TypeIncomingWebhook, CreatorStatus: StatusDeactivated, Orphaned: true},
		{Type: TypeBot, CreatorStatus: StatusActive, Orphaned: false},
		{Type: TypeBot, CreatorStatus: StatusDeleted, Orphaned: true},
	}

	summary := buildSummary(integrations, "engineering", TypeIncomingWebhook, true)

	if summary.Total != 4 {
		t.Errorf("Total = %d, want 4", summary.Total)
	}
	if summary.Orphaned != 2 {
		t.Errorf("Orphaned = %d, want 2", summary.Orphaned)
	}
	if summary.ByType[string(TypeIncomingWebhook)] != 2 {
		t.Errorf("ByType[incoming_webhook] = %d, want 2", summary.ByType[string(TypeIncomingWebhook)])
	}
	if summary.TeamFilter != "engineering" {
		t.Errorf("TeamFilter = %q, want %q", summary.TeamFilter, "engineering")
	}
	if summary.TypeFilter != string(TypeIncomingWebhook) {
		t.Errorf("TypeFilter = %q, want %q", summary.TypeFilter, string(TypeIncomingWebhook))
	}
	if !summary.OrphanedOnlyFilter {
		t.Error("OrphanedOnlyFilter = false, want true")
	}
}

func TestParseIntegrationType(t *testing.T) {
	tests := []struct {
		input    string
		expected IntegrationType
		wantErr  bool
	}{
		{"incoming", TypeIncomingWebhook, false},
		{"outgoing", TypeOutgoingWebhook, false},
		{"bot", TypeBot, false},
		{"oauth", TypeOAuthApp, false},
		{"slash", TypeSlashCommand, false},
		{"INCOMING", TypeIncomingWebhook, false},
		{" Bot ", TypeBot, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseIntegrationType(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMillisToTime(t *testing.T) {
	tests := []struct {
		name   string
		millis int64
		want   time.Time
	}{
		{"zero returns zero time", 0, time.Time{}},
		{"1710000000000 converts correctly", 1710000000000, time.Date(2024, 3, 9, 16, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := millisToTime(tt.millis)
			if !got.Equal(tt.want) {
				t.Errorf("millisToTime(%d) = %v, want %v", tt.millis, got, tt.want)
			}
		})
	}
}

func TestResolveTeamEmptyID(t *testing.T) {
	mc := newMockClient()
	fetcher := NewInventoryFetcher(mc, false)
	ig := &Integration{}
	fetcher.resolveTeam("", ig)
	if ig.Team != "All Teams" {
		t.Errorf("Team = %q, want %q for empty teamID", ig.Team, "All Teams")
	}
}

func TestResolveChannelEmptyID(t *testing.T) {
	mc := newMockClient()
	fetcher := NewInventoryFetcher(mc, false)
	ig := &Integration{}
	fetcher.resolveChannel("", ig)
	if ig.Channel != "" {
		t.Errorf("Channel = %q, want empty string for empty channelID", ig.Channel)
	}
}

func TestCreatorDisplayNameFallback(t *testing.T) {
	mc := newMockClient()
	mc.users["u1"] = &model.User{
		Id:       "u1",
		Username: "alice",
		// No FirstName or LastName
	}

	fetcher := NewInventoryFetcher(mc, false)
	ig := &Integration{}
	fetcher.resolveCreator("u1", ig)

	if ig.CreatorDisplayName != "alice" {
		t.Errorf("CreatorDisplayName = %q, want %q (should fall back to username)", ig.CreatorDisplayName, "alice")
	}
}

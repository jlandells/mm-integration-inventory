package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// IntegrationType identifies the kind of integration.
type IntegrationType string

const (
	TypeIncomingWebhook IntegrationType = "incoming_webhook"
	TypeOutgoingWebhook IntegrationType = "outgoing_webhook"
	TypeBot             IntegrationType = "bot"
	TypeOAuthApp        IntegrationType = "oauth_app"
	TypeSlashCommand    IntegrationType = "slash_command"
)

// AllTypes lists every integration type in display order.
var AllTypes = []IntegrationType{
	TypeIncomingWebhook,
	TypeOutgoingWebhook,
	TypeBot,
	TypeOAuthApp,
	TypeSlashCommand,
}

// TypeDisplayName returns a human-readable name for a type.
func TypeDisplayName(t IntegrationType) string {
	switch t {
	case TypeIncomingWebhook:
		return "Incoming Webhooks"
	case TypeOutgoingWebhook:
		return "Outgoing Webhooks"
	case TypeBot:
		return "Bot Accounts"
	case TypeOAuthApp:
		return "OAuth 2.0 Applications"
	case TypeSlashCommand:
		return "Slash Commands"
	default:
		return string(t)
	}
}

// ParseIntegrationType maps a user-facing short name to an IntegrationType.
func ParseIntegrationType(s string) (IntegrationType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "incoming":
		return TypeIncomingWebhook, nil
	case "outgoing":
		return TypeOutgoingWebhook, nil
	case "bot":
		return TypeBot, nil
	case "oauth":
		return TypeOAuthApp, nil
	case "slash":
		return TypeSlashCommand, nil
	default:
		return "", fmt.Errorf("unknown integration type %q. Valid types: incoming, outgoing, bot, oauth, slash", s)
	}
}

// CreatorStatus describes whether the integration creator's account is active.
type CreatorStatus string

const (
	StatusActive      CreatorStatus = "active"
	StatusDeactivated CreatorStatus = "deactivated"
	StatusDeleted     CreatorStatus = "deleted"
	StatusUnknown     CreatorStatus = "unknown"
)

// Integration is the unified record for all integration types.
type Integration struct {
	Type               IntegrationType `json:"type"`
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	CreatorUsername     string          `json:"creator_username"`
	CreatorDisplayName string          `json:"creator_display_name"`
	CreatorStatus      CreatorStatus   `json:"creator_status"`
	Team               string          `json:"team"`
	Channel            string          `json:"channel"`
	Description        string          `json:"description"`
	CreatedAt          time.Time       `json:"created_at"`
	Orphaned           bool            `json:"orphaned"`
}

// InventorySummary holds aggregate counts for the result envelope.
type InventorySummary struct {
	Total              int            `json:"total"`
	Orphaned           int            `json:"orphaned"`
	ByType             map[string]int `json:"by_type"`
	ByCreatorStatus    map[string]int `json:"by_creator_status"`
	TeamFilter         string         `json:"team_filter,omitempty"`
	TypeFilter         string         `json:"type_filter,omitempty"`
	OrphanedOnlyFilter bool           `json:"orphaned_only_filter"`
}

// InventoryResult is the top-level result envelope.
type InventoryResult struct {
	Summary      InventorySummary `json:"summary"`
	Integrations []Integration    `json:"integrations"`
}

// UserInfo caches resolved user details.
type UserInfo struct {
	Username    string
	DisplayName string
	Status      CreatorStatus
}

// InventoryFetcher holds the client and caches needed to build the inventory.
type InventoryFetcher struct {
	client   MattermostClient
	verbose  bool
	warnings int

	userCache    map[string]*UserInfo
	teamCache    map[string]*model.Team
	channelCache map[string]*model.Channel
}

// NewInventoryFetcher creates a new fetcher with empty caches.
func NewInventoryFetcher(client MattermostClient, verbose bool) *InventoryFetcher {
	return &InventoryFetcher{
		client:       client,
		verbose:      verbose,
		userCache:    make(map[string]*UserInfo),
		teamCache:    make(map[string]*model.Team),
		channelCache: make(map[string]*model.Channel),
	}
}

const perPage = 200

// FetchInventory gathers all integrations, applies filters, and returns the result.
func (f *InventoryFetcher) FetchInventory(teamFilter string, typeFilter IntegrationType, orphanedOnly bool) (*InventoryResult, error) {
	var filterTeamID string
	if teamFilter != "" {
		team, err := f.client.GetTeamByName(teamFilter)
		if err != nil {
			if isNotFound(err) {
				return nil, &ConfigError{Message: fmt.Sprintf("error: team %q not found. Please check the name and try again.", teamFilter)}
			}
			return nil, err
		}
		filterTeamID = team.Id
		f.teamCache[team.Id] = team
	}

	types := AllTypes
	if typeFilter != "" {
		types = []IntegrationType{typeFilter}
	}

	var integrations []Integration

	for _, t := range types {
		var items []Integration
		var err error

		switch t {
		case TypeIncomingWebhook:
			items, err = f.fetchIncomingWebhooks(filterTeamID)
		case TypeOutgoingWebhook:
			items, err = f.fetchOutgoingWebhooks(filterTeamID)
		case TypeBot:
			items, err = f.fetchBots()
		case TypeOAuthApp:
			items, err = f.fetchOAuthApps()
		case TypeSlashCommand:
			items, err = f.fetchSlashCommands(filterTeamID)
		}

		if err != nil {
			// If a specific type returns 403, the feature is likely disabled on
			// the instance.  Skip it with a warning unless the user explicitly
			// asked for that type — in which case it is a real error.
			if isForbidden(err) && typeFilter == "" {
				fmt.Fprintf(os.Stderr, "warning: skipping %s — access denied (feature may be disabled on this instance).\n", TypeDisplayName(t))
				f.warnings++
				continue
			}
			return nil, err
		}
		integrations = append(integrations, items...)
	}

	if orphanedOnly {
		filtered := integrations[:0]
		for _, ig := range integrations {
			if ig.Orphaned {
				filtered = append(filtered, ig)
			}
		}
		integrations = filtered
	}

	summary := buildSummary(integrations, teamFilter, typeFilter, orphanedOnly)

	var partialErr error
	if f.warnings > 0 {
		partialErr = &PartialError{
			Message:   "some creator lookups failed",
			FailCount: f.warnings,
		}
	}

	result := &InventoryResult{
		Summary:      summary,
		Integrations: integrations,
	}

	if partialErr != nil {
		return result, partialErr
	}
	return result, nil
}

func buildSummary(integrations []Integration, teamFilter string, typeFilter IntegrationType, orphanedOnly bool) InventorySummary {
	byType := make(map[string]int)
	byStatus := make(map[string]int)
	orphanCount := 0

	for _, ig := range integrations {
		byType[string(ig.Type)]++
		byStatus[string(ig.CreatorStatus)]++
		if ig.Orphaned {
			orphanCount++
		}
	}

	s := InventorySummary{
		Total:              len(integrations),
		Orphaned:           orphanCount,
		ByType:             byType,
		ByCreatorStatus:    byStatus,
		OrphanedOnlyFilter: orphanedOnly,
	}
	if teamFilter != "" {
		s.TeamFilter = teamFilter
	}
	if typeFilter != "" {
		s.TypeFilter = string(typeFilter)
	}
	return s
}

// --- Per-type fetch functions ---

func (f *InventoryFetcher) fetchIncomingWebhooks(filterTeamID string) ([]Integration, error) {
	var result []Integration
	page := 0
	for {
		hooks, err := f.client.GetIncomingWebhooks(page, perPage)
		if err != nil {
			return nil, err
		}
		for _, h := range hooks {
			if filterTeamID != "" && h.TeamId != filterTeamID {
				continue
			}
			ig := Integration{
				Type:        TypeIncomingWebhook,
				ID:          h.Id,
				Name:        h.DisplayName,
				Description: h.Description,
				CreatedAt:   millisToTime(h.CreateAt),
			}
			f.resolveCreator(h.UserId, &ig)
			f.resolveTeam(h.TeamId, &ig)
			f.resolveChannel(h.ChannelId, &ig)
			result = append(result, ig)
		}
		if len(hooks) < perPage {
			break
		}
		page++
	}
	return result, nil
}

func (f *InventoryFetcher) fetchOutgoingWebhooks(filterTeamID string) ([]Integration, error) {
	var result []Integration
	page := 0
	for {
		hooks, err := f.client.GetOutgoingWebhooks(page, perPage)
		if err != nil {
			return nil, err
		}
		for _, h := range hooks {
			if filterTeamID != "" && h.TeamId != filterTeamID {
				continue
			}
			ig := Integration{
				Type:        TypeOutgoingWebhook,
				ID:          h.Id,
				Name:        h.DisplayName,
				Description: h.Description,
				CreatedAt:   millisToTime(h.CreateAt),
			}
			f.resolveCreator(h.CreatorId, &ig)
			f.resolveTeam(h.TeamId, &ig)
			f.resolveChannel(h.ChannelId, &ig)
			result = append(result, ig)
		}
		if len(hooks) < perPage {
			break
		}
		page++
	}
	return result, nil
}

func (f *InventoryFetcher) fetchBots() ([]Integration, error) {
	var result []Integration
	page := 0
	for {
		bots, err := f.client.GetBots(page, perPage)
		if err != nil {
			return nil, err
		}
		for _, b := range bots {
			ig := Integration{
				Type:        TypeBot,
				ID:          b.UserId,
				Name:        b.DisplayName,
				Description: b.Description,
				Team:        "All Teams",
				CreatedAt:   millisToTime(b.CreateAt),
			}
			// OwnerId is the human who created the bot, not UserId (the bot's own account)
			f.resolveCreator(b.OwnerId, &ig)
			result = append(result, ig)
		}
		if len(bots) < perPage {
			break
		}
		page++
	}
	return result, nil
}

func (f *InventoryFetcher) fetchOAuthApps() ([]Integration, error) {
	var result []Integration
	page := 0
	for {
		apps, err := f.client.GetOAuthApps(page, perPage)
		if err != nil {
			return nil, err
		}
		for _, a := range apps {
			ig := Integration{
				Type:        TypeOAuthApp,
				ID:          a.Id,
				Name:        a.Name,
				Description: a.Description,
				Team:        "All Teams",
				CreatedAt:   millisToTime(a.CreateAt),
			}
			f.resolveCreator(a.CreatorId, &ig)
			result = append(result, ig)
		}
		if len(apps) < perPage {
			break
		}
		page++
	}
	return result, nil
}

func (f *InventoryFetcher) fetchSlashCommands(filterTeamID string) ([]Integration, error) {
	// Slash commands endpoint requires a team_id and doesn't paginate.
	// Strategy: if we have a specific team, use it; otherwise fetch all teams.
	var teamIDs []string
	if filterTeamID != "" {
		teamIDs = []string{filterTeamID}
	} else {
		teams, err := f.getAllTeams()
		if err != nil {
			return nil, err
		}
		for _, t := range teams {
			teamIDs = append(teamIDs, t.Id)
		}
	}

	seen := make(map[string]bool)
	var result []Integration

	for _, tid := range teamIDs {
		cmds, err := f.client.GetCommands(tid)
		if err != nil {
			if f.verbose {
				fmt.Fprintf(os.Stderr, "warning: failed to fetch commands for team %s: %v\n", tid, err)
			}
			f.warnings++
			continue
		}
		for _, c := range cmds {
			if seen[c.Id] {
				continue
			}
			seen[c.Id] = true

			name := c.DisplayName
			if name == "" {
				name = "/" + c.Trigger
			}

			ig := Integration{
				Type:        TypeSlashCommand,
				ID:          c.Id,
				Name:        name,
				Description: c.Description,
				CreatedAt:   millisToTime(c.CreateAt),
			}
			f.resolveCreator(c.CreatorId, &ig)
			f.resolveTeam(c.TeamId, &ig)
			result = append(result, ig)
		}
	}
	return result, nil
}

// --- Resolution helpers ---

func (f *InventoryFetcher) resolveCreator(userID string, ig *Integration) {
	if userID == "" {
		ig.CreatorUsername = ""
		ig.CreatorDisplayName = ""
		ig.CreatorStatus = StatusUnknown
		ig.Orphaned = true
		return
	}

	if info, ok := f.userCache[userID]; ok {
		ig.CreatorUsername = info.Username
		ig.CreatorDisplayName = info.DisplayName
		ig.CreatorStatus = info.Status
		ig.Orphaned = info.Status != StatusActive
		return
	}

	user, err := f.client.GetUser(userID)
	if err != nil {
		if isNotFound(err) {
			info := &UserInfo{Username: "(deleted)", DisplayName: "(deleted)", Status: StatusDeleted}
			f.userCache[userID] = info
			ig.CreatorUsername = info.Username
			ig.CreatorDisplayName = info.DisplayName
			ig.CreatorStatus = StatusDeleted
			ig.Orphaned = true
			return
		}
		if f.verbose {
			fmt.Fprintf(os.Stderr, "warning: failed to look up user %s: %v\n", userID, err)
		}
		f.warnings++
		ig.CreatorUsername = userID
		ig.CreatorDisplayName = ""
		ig.CreatorStatus = StatusUnknown
		ig.Orphaned = true
		return
	}

	status := StatusActive
	if user.DeleteAt > 0 {
		status = StatusDeactivated
	}

	displayName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if displayName == "" {
		displayName = user.Username
	}

	info := &UserInfo{
		Username:    user.Username,
		DisplayName: displayName,
		Status:      status,
	}
	f.userCache[userID] = info

	ig.CreatorUsername = info.Username
	ig.CreatorDisplayName = info.DisplayName
	ig.CreatorStatus = info.Status
	ig.Orphaned = status != StatusActive
}

func (f *InventoryFetcher) resolveTeam(teamID string, ig *Integration) {
	if teamID == "" {
		ig.Team = "All Teams"
		return
	}

	if team, ok := f.teamCache[teamID]; ok {
		ig.Team = team.DisplayName
		return
	}

	team, err := f.client.GetTeam(teamID)
	if err != nil {
		if f.verbose {
			fmt.Fprintf(os.Stderr, "warning: failed to look up team %s: %v\n", teamID, err)
		}
		f.warnings++
		ig.Team = teamID
		return
	}

	f.teamCache[teamID] = team
	ig.Team = team.DisplayName
}

func (f *InventoryFetcher) resolveChannel(channelID string, ig *Integration) {
	if channelID == "" {
		return
	}

	if ch, ok := f.channelCache[channelID]; ok {
		ig.Channel = ch.DisplayName
		return
	}

	ch, err := f.client.GetChannel(channelID)
	if err != nil {
		if f.verbose {
			fmt.Fprintf(os.Stderr, "warning: failed to look up channel %s: %v\n", channelID, err)
		}
		f.warnings++
		ig.Channel = channelID
		return
	}

	f.channelCache[channelID] = ch
	ig.Channel = ch.DisplayName
}

func (f *InventoryFetcher) getAllTeams() ([]*model.Team, error) {
	var all []*model.Team
	page := 0
	for {
		teams, err := f.client.GetAllTeams(page, perPage)
		if err != nil {
			return nil, err
		}
		all = append(all, teams...)
		if len(teams) < perPage {
			break
		}
		page++
	}
	return all, nil
}

// --- Utilities ---

func millisToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func isNotFound(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == http.StatusNotFound
	}
	return false
}

func isForbidden(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == http.StatusForbidden
	}
	return false
}

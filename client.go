package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"golang.org/x/term"
)

// MattermostClient abstracts the Mattermost API so business logic can be tested
// with a mock implementation.
type MattermostClient interface {
	GetIncomingWebhooks(page, perPage int) ([]*model.IncomingWebhook, error)
	GetOutgoingWebhooks(page, perPage int) ([]*model.OutgoingWebhook, error)
	GetBots(page, perPage int) ([]*model.Bot, error)
	GetOAuthApps(page, perPage int) ([]*model.OAuthApp, error)
	GetCommands(teamID string) ([]*model.Command, error)
	GetUser(userID string) (*model.User, error)
	GetTeam(teamID string) (*model.Team, error)
	GetTeamByName(name string) (*model.Team, error)
	GetChannel(channelID string) (*model.Channel, error)
	GetAllTeams(page, perPage int) ([]*model.Team, error)
}

// realClient wraps model.Client4 to implement MattermostClient.
type realClient struct {
	api *model.Client4
}

func (c *realClient) GetIncomingWebhooks(page, perPage int) ([]*model.IncomingWebhook, error) {
	ctx := context.Background()
	hooks, resp, err := c.api.GetIncomingWebhooks(ctx, page, perPage, "")
	if err != nil {
		return nil, wrapAPIError("fetching incoming webhooks", resp, err)
	}
	return hooks, nil
}

func (c *realClient) GetOutgoingWebhooks(page, perPage int) ([]*model.OutgoingWebhook, error) {
	ctx := context.Background()
	hooks, resp, err := c.api.GetOutgoingWebhooks(ctx, page, perPage, "")
	if err != nil {
		return nil, wrapAPIError("fetching outgoing webhooks", resp, err)
	}
	return hooks, nil
}

func (c *realClient) GetBots(page, perPage int) ([]*model.Bot, error) {
	ctx := context.Background()
	bots, resp, err := c.api.GetBots(ctx, page, perPage, "")
	if err != nil {
		return nil, wrapAPIError("fetching bots", resp, err)
	}
	return bots, nil
}

func (c *realClient) GetOAuthApps(page, perPage int) ([]*model.OAuthApp, error) {
	ctx := context.Background()
	apps, resp, err := c.api.GetOAuthApps(ctx, page, perPage)
	if err != nil {
		return nil, wrapAPIError("fetching OAuth apps", resp, err)
	}
	return apps, nil
}

func (c *realClient) GetCommands(teamID string) ([]*model.Command, error) {
	ctx := context.Background()
	cmds, resp, err := c.api.ListCommands(ctx, teamID, true)
	if err != nil {
		return nil, wrapAPIError("fetching slash commands", resp, err)
	}
	return cmds, nil
}

func (c *realClient) GetUser(userID string) (*model.User, error) {
	ctx := context.Background()
	user, resp, err := c.api.GetUser(ctx, userID, "")
	if err != nil {
		return nil, wrapAPIError("fetching user", resp, err)
	}
	return user, nil
}

func (c *realClient) GetTeam(teamID string) (*model.Team, error) {
	ctx := context.Background()
	team, resp, err := c.api.GetTeam(ctx, teamID, "")
	if err != nil {
		return nil, wrapAPIError("fetching team", resp, err)
	}
	return team, nil
}

func (c *realClient) GetTeamByName(name string) (*model.Team, error) {
	ctx := context.Background()
	team, resp, err := c.api.GetTeamByName(ctx, name, "")
	if err != nil {
		return nil, wrapAPIError(fmt.Sprintf("looking up team %q", name), resp, err)
	}
	return team, nil
}

func (c *realClient) GetChannel(channelID string) (*model.Channel, error) {
	ctx := context.Background()
	ch, resp, err := c.api.GetChannel(ctx, channelID)
	if err != nil {
		return nil, wrapAPIError("fetching channel", resp, err)
	}
	return ch, nil
}

func (c *realClient) GetAllTeams(page, perPage int) ([]*model.Team, error) {
	ctx := context.Background()
	teams, resp, err := c.api.GetAllTeams(ctx, "", page, perPage)
	if err != nil {
		return nil, wrapAPIError("fetching teams", resp, err)
	}
	return teams, nil
}

// wrapAPIError translates a Mattermost API error into a typed error with a
// user-friendly message.
func wrapAPIError(action string, resp *model.Response, err error) error {
	if resp == nil {
		return &APIError{Message: fmt.Sprintf("error %s: %v", action, err)}
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return &ConfigError{Message: "error: authentication failed. Check your token or credentials."}
	case http.StatusForbidden:
		return &APIError{Message: fmt.Sprintf("error: permission denied while %s. This may require a System Administrator account or the feature may be disabled.", action), StatusCode: resp.StatusCode}
	case http.StatusNotFound:
		return &APIError{Message: fmt.Sprintf("error %s: not found", action), StatusCode: resp.StatusCode}
	default:
		if resp.StatusCode >= 500 {
			return &APIError{
				Message:    fmt.Sprintf("error: the Mattermost server returned an unexpected error while %s. Check server logs for details.", action),
				StatusCode: resp.StatusCode,
			}
		}
		return &APIError{
			Message:    fmt.Sprintf("error %s: %v", action, err),
			StatusCode: resp.StatusCode,
		}
	}
}

// newClient creates a MattermostClient by authenticating against the given server.
func newClient(serverURL, token, username string, verbose bool) (MattermostClient, error) {
	url := strings.TrimRight(serverURL, "/")
	api := model.NewAPIv4Client(url)

	if token != "" {
		api.SetToken(token)
		if verbose {
			fmt.Fprintln(os.Stderr, "Authenticating with Personal Access Token...")
		}
		return &realClient{api: api}, nil
	}

	if username != "" {
		password, err := obtainPassword()
		if err != nil {
			return nil, &ConfigError{Message: fmt.Sprintf("error: %v", err)}
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Authenticating as %s...\n", username)
		}
		ctx := context.Background()
		_, resp, err := api.Login(ctx, username, password)
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusUnauthorized {
				return nil, &ConfigError{Message: "error: authentication failed. Check your username and password."}
			}
			return nil, &ConfigError{Message: fmt.Sprintf("error: login failed: %v", err)}
		}
		return &realClient{api: api}, nil
	}

	return nil, &ConfigError{
		Message: "error: authentication required. Use --token (or MM_TOKEN) for token auth, or --username (or MM_USERNAME) for password auth.",
	}
}

// obtainPassword gets the password via interactive prompt (if TTY) or MM_PASSWORD env var.
func obtainPassword() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Password: ")
		passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr) // move to next line after input
		if err != nil {
			return "", fmt.Errorf("unable to read password: %w", err)
		}
		return string(passwordBytes), nil
	}

	password := os.Getenv("MM_PASSWORD")
	if password == "" {
		return "", fmt.Errorf("password required. Set MM_PASSWORD for non-interactive use or run interactively for a prompt")
	}
	return password, nil
}

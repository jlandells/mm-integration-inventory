# mm-integration-inventory

A command-line tool that produces a complete inventory of all integrations configured on a Mattermost instance, including incoming webhooks, outgoing webhooks, bot accounts, OAuth 2.0 applications, and slash commands. For each integration it reports the creator, creation date, scope, and clearly flags any "orphaned" integrations whose creator account has been deactivated or deleted.

## Why You'd Use It

Over time, Mattermost instances accumulate integrations created by users across many teams. When those users leave the organisation, their integrations remain active but become orphaned — nobody owns them, nobody knows what they do, and they may represent a security risk. The Mattermost UI has per-team views for some integration types, but there is no consolidated, instance-wide inventory with ownership context.

This tool fills that gap. It is especially useful during security audits, staff off-boarding reviews, and SOC 2 or ISO 27001 assessments where evidence of integration governance is required.

## Installation

Download the pre-built binary for your platform from the [Releases](https://github.com/jlandells/mm-integration-inventory/releases) page. No other steps are required — the binary is self-contained with no external dependencies.

| Platform       | Filename                                   |
|----------------|--------------------------------------------|
| Linux (amd64)  | `mm-integration-inventory-linux-amd64`     |
| macOS (amd64)  | `mm-integration-inventory-darwin-amd64`    |
| macOS (arm64)  | `mm-integration-inventory-darwin-arm64`    |
| Windows (amd64)| `mm-integration-inventory-windows-amd64.exe` |

On Linux and macOS, make the binary executable after downloading:

```bash
chmod +x mm-integration-inventory-*
```

## Authentication

The tool requires a **System Administrator** account to access the instance-wide integration endpoints.

### Option 1 — Personal Access Token (recommended)

Generate a Personal Access Token in **System Console > Integrations > Bot Accounts** (or your user profile's **Security** tab) and pass it via the `--token` flag or `MM_TOKEN` environment variable.

```bash
mm-integration-inventory --url https://mattermost.example.com --token your-token-here
```

> **Note:** Personal Access Tokens may be disabled on some instances. If your instance does not support them, use username/password authentication instead.

### Option 2 — Username and Password

Pass your username via the `--username` flag or `MM_USERNAME` environment variable. The tool will prompt for your password interactively (input is hidden). For non-interactive or automation scenarios, set the `MM_PASSWORD` environment variable.

```bash
mm-integration-inventory --url https://mattermost.example.com --username admin
Password: ********
```

> **Note:** There is no `--password` flag. Passwords passed as CLI arguments appear in shell history and process listings, which is a security risk.

## Usage

```
mm-integration-inventory [flags]
```

### Flag Reference

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--url` | `MM_URL` | *(required)* | Mattermost server URL |
| `--token` | `MM_TOKEN` | | Personal Access Token |
| `--username` | `MM_USERNAME` | | Username for password auth |
| `--team` | | | Scope report to a single named team |
| `--orphaned-only` | | `false` | Show only orphaned integrations |
| `--type` | | *(all)* | Filter by type: `incoming`, `outgoing`, `bot`, `oauth`, `slash` |
| `--format` | | `table` | Output format: `table`, `csv`, `json` |
| `--output` | | *(stdout)* | Write output to a file |
| `--verbose` / `-v` | | `false` | Verbose logging to stderr |
| `--version` | | | Print version and exit |

## Examples

### Basic run with token auth

```bash
mm-integration-inventory --url https://mattermost.example.com --token your-token-here
```

### Basic run with username/password

```bash
mm-integration-inventory --url https://mattermost.example.com --username admin
Password: ********
```

### Using environment variables

```bash
export MM_URL=https://mattermost.example.com
export MM_TOKEN=your-token-here
mm-integration-inventory
```

### Write output to a file

```bash
mm-integration-inventory --url https://mattermost.example.com --token your-token --format csv --output inventory.csv
```

### Show only orphaned integrations

```bash
mm-integration-inventory --url https://mattermost.example.com --token your-token --orphaned-only
```

### Filter by integration type

```bash
mm-integration-inventory --url https://mattermost.example.com --token your-token --type bot
```

### Scope to a specific team

```bash
mm-integration-inventory --url https://mattermost.example.com --token your-token --team Engineering
```

Note: Bot accounts and OAuth apps are instance-wide and are always included regardless of team scoping.

### JSON output piped to jq

```bash
mm-integration-inventory --url https://mattermost.example.com --token your-token --format json | jq '.integrations[] | select(.orphaned == true)'
```

## Output Formats

### Table (default)

Human-readable, aligned columns grouped by integration type:

```
=== Incoming Webhooks (2) ===

NAME              CREATOR       CREATOR STATUS    TEAM          CHANNEL    CREATED
CI Webhook        alice         Active            Engineering   Dev Ops    2024-03-15
Alerts Webhook    bob.smith     Deactivated ⚠     Engineering   Alerts     2023-11-01

=== Bot Accounts (1) ===

NAME              CREATOR       CREATOR STATUS    TEAM          CHANNEL    CREATED
Deploy Bot        alice         Active            All Teams     N/A        2024-01-20

--- Summary ---
Total integrations: 3
Orphaned:           1

By type:
  Incoming Webhooks:      2
  Bot Accounts:           1
```

### CSV

```csv
type,name,creator_username,creator_display_name,creator_status,team,channel,description,created_at,orphaned
incoming_webhook,CI Webhook,alice,Alice Johnson,active,Engineering,Dev Ops,Build notifications,2024-03-15T10:00:00Z,false
incoming_webhook,Alerts Webhook,bob.smith,Bob Smith,deactivated,Engineering,Alerts,Alert notifications,2023-11-01T09:00:00Z,true
bot,Deploy Bot,alice,Alice Johnson,active,All Teams,,Handles deploys,2024-01-20T14:30:00Z,false
```

### JSON

```json
{
  "summary": {
    "total": 3,
    "orphaned": 1,
    "by_type": {
      "incoming_webhook": 2,
      "bot": 1
    },
    "by_creator_status": {
      "active": 2,
      "deactivated": 1
    },
    "orphaned_only_filter": false
  },
  "integrations": [
    {
      "type": "incoming_webhook",
      "id": "iw1",
      "name": "CI Webhook",
      "creator_username": "alice",
      "creator_display_name": "Alice Johnson",
      "creator_status": "active",
      "team": "Engineering",
      "channel": "Dev Ops",
      "description": "Build notifications",
      "created_at": "2024-03-15T10:00:00Z",
      "orphaned": false
    }
  ]
}
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Configuration error — missing URL, invalid auth, unknown team name, invalid type |
| 2 | API error — connection failure, unexpected server response |
| 3 | Partial failure — completed but some per-item lookups failed |
| 4 | Output error — unable to write output file (falls back to stdout) |

## Limitations

- This tool is **read-only**. It does not delete, disable, or modify any integrations.
- It does not re-assign ownership of orphaned integrations.
- It does not test whether webhooks or slash commands are actually functional.
- Plugin-based integrations are not included. Use [mm-plugin-audit](https://github.com/jlandells/mm-plugin-audit) for plugin reporting.
- The slash commands API endpoint requires a team ID and does not paginate. The tool works around this by iterating over all teams and deduplicating, which may be slow on instances with a very large number of teams.
- The account used must have System Administrator role. Non-admin tokens will receive a 403 error.

## Integration Testing

To test against a local Mattermost instance:

1. Start a local Mattermost server (e.g. via Docker)
2. Create a System Administrator account and generate a Personal Access Token
3. Create some integrations (webhooks, bots, slash commands) across multiple teams
4. Deactivate one of the creator accounts to test orphan detection
5. Run the tool:

```bash
./mm-integration-inventory --url http://localhost:8065 --token your-token --verbose
```

Verify:
- All integration types are listed
- `--orphaned-only` correctly filters to orphaned integrations
- `--type bot` correctly filters to bots only
- `--team <name>` scopes webhooks/commands but includes bots/OAuth apps
- `--format csv --output test.csv` produces a valid CSV file
- `--format json | jq .` produces valid JSON with a `summary` object

## Contributing

We welcome contributions from the community! Whether it's a bug report, a feature suggestion,
or a pull request, your input is valuable to us. Please feel free to contribute in the
following ways:
- **Issues and Pull Requests**: For specific questions, issues, or suggestions for improvements,
  open an issue or a pull request in this repository.
- **Mattermost Community**: Join the discussion in the
  [Integrations and Apps](https://community.mattermost.com/core/channels/integrations) channel
  on the Mattermost Community server.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contact

For questions, feedback, or contributions regarding this project, please use the following methods:
- **Issues and Pull Requests**: For specific questions, issues, or suggestions for improvements,
  feel free to open an issue or a pull request in this repository.
- **Mattermost Community**: Join us in the Mattermost Community server, where we discuss all
  things related to extending Mattermost. You can find me in the channel
  [Integrations and Apps](https://community.mattermost.com/core/channels/integrations).
- **Social Media**: Follow and message me on Twitter, where I'm
  [@jlandells](https://twitter.com/jlandells).

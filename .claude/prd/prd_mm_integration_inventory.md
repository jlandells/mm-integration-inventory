# PRD: mm-integration-inventory — Mattermost Integration Inventory

**Version:** 1.0  
**Status:** Ready for Development  
**Language:** Go  
**Binary Name:** `mm-integration-inventory`

---

## 1. Overview

`mm-integration-inventory` is a standalone command-line utility that produces a complete inventory of all integrations configured on a Mattermost instance. This includes incoming webhooks, outgoing webhooks, bot accounts, OAuth 2.0 applications, and slash commands. For each integration, it reports the creator, creation date, scope, and flags any integrations whose creator account has since been deactivated or deleted ("orphaned" integrations).

---

## 2. Background & Problem Statement

Over time, Mattermost instances accumulate integrations created by a variety of users across teams. When those users leave an organisation, their integrations remain active but become "orphaned" — nobody owns them, nobody knows what they do, and they may represent a security risk. The Mattermost UI has per-team views for some integration types, but there is no consolidated, instance-wide view of all integrations with ownership and activity context.

This gap is particularly problematic during security audits, staff off-boarding reviews, and SOC 2 or ISO 27001 assessments, where evidence of integration governance is often required.

---

## 3. Goals

- Enumerate all integrations across all types and all teams on a Mattermost instance
- Report creator and creation date for each integration
- Flag integrations whose creator account is deactivated or deleted
- Provide a scoped view (by team) for team-level reviews
- Produce output suitable for audit documentation (CSV, JSON) or human review (table)

---

## 4. Non-Goals

- This tool does not delete, disable, or modify any integrations — it is read-only
- This tool does not re-assign ownership of orphaned integrations
- This tool does not test whether webhooks or slash commands are functional
- This tool does not report on plugin integrations (see `mm-plugin-audit` for that)

---

## 5. Target Users

Mattermost System Administrators, Security Officers, and IT Governance teams responsible for maintaining an accurate record of third-party integrations and their ownership.

---

## 6. User Stories

- As a System Administrator, I want to see all integrations on my instance in one place so that I can understand what is connected and by whom.
- As a Security Officer, I want to identify integrations whose creator has left the organisation so that I can review and remediate them.
- As a Team Administrator, I want to scope the report to my team so that I can review only the integrations relevant to me.
- As a System Administrator, I want a CSV export so that I can include it in our annual security review.

---

## 7. Functional Requirements

### 7.1 Integration Types

The tool MUST enumerate all of the following integration types:

| Type | Mattermost Term | API Resource |
|------|----------------|--------------|
| Incoming Webhooks | `incoming_webhooks` | `/api/v4/hooks/incoming` |
| Outgoing Webhooks | `outgoing_webhooks` | `/api/v4/hooks/outgoing` |
| Bot Accounts | `bots` | `/api/v4/bots` |
| OAuth 2.0 Applications | `oauth_apps` | `/api/v4/oauth/apps` |
| Slash Commands | `commands` | `/api/v4/commands` |

### 7.2 Per-Integration Data

For each integration, the tool MUST report:

- Integration type (one of the five types above)
- Integration name / display name
- Creator username and display name
- Creator account status: `Active`, `Deactivated`, or `Deleted`
- Creation date (ISO 8601)
- Team scope (team display name, or `All Teams` if instance-wide)
- Channel scope (channel display name, or `N/A` if not channel-specific)
- Description (if set)

### 7.3 Orphan Detection

- An integration is considered **orphaned** if its creator's user account is deactivated or no longer exists in the system
- The `--orphaned-only` flag MUST filter the output to show only orphaned integrations
- In all output modes, orphaned integrations must be clearly marked

### 7.4 Team Scoping

- When `--team TEAM_NAME` is specified, the tool MUST resolve the team name to an ID via the API
- Only integrations scoped to that team should be included
- Bot accounts and OAuth apps are instance-wide and should always be included regardless of team scoping, with a note that they are instance-wide resources
- If the team name cannot be resolved, the tool MUST exit with a clear error message

### 7.5 Pagination

All API calls that return lists MUST be paginated. The tool must not assume all results fit in a single response.

---

## 8. CLI Specification

### Usage

```
mm-integration-inventory [flags]
```

### Connection Flags (required)

| Flag | Environment Variable | Description |
|------|----------------------|-------------|
| `--url URL` | `MM_URL` | Mattermost server URL, e.g. `https://mattermost.example.com` |

### Authentication Flags

| Flag | Environment Variable | Description |
|------|----------------------|-------------|
| `--token TOKEN` | `MM_TOKEN` | Personal Access Token (preferred) |
| `--username USERNAME` | `MM_USERNAME` | Username for password-based auth |
| *(no flag)* | `MM_PASSWORD` | Password (env var only — never a CLI flag) |

Authentication resolution order:
1. `--token` / `MM_TOKEN`
2. `--username` + interactive password prompt (if terminal is interactive)
3. `--username` + `MM_PASSWORD` environment variable

### Operational Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--team TEAM_NAME` | *(none — all teams)* | Scope report to a single named team |
| `--orphaned-only` | `false` | Show only integrations with deactivated/deleted creators |
| `--type TYPE` | *(all types)* | Filter by type: `incoming`, `outgoing`, `bot`, `oauth`, `slash` |
| `--format table\|csv\|json` | `table` | Output format |
| `--output FILE` | *(stdout)* | Write output to a file |
| `--verbose` / `-v` | `false` | Enable verbose logging to stderr |

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Configuration error (missing URL, invalid auth, unknown team name) |
| `2` | API error (connection failure, unexpected response) |
| `3` | Output error (unable to write file) |

---

## 9. Output Specification

### 9.1 Table Format

Grouped by integration type, with a header for each group:

```
=== Incoming Webhooks (3) ===
NAME            | CREATOR       | CREATOR STATUS | TEAM        | CHANNEL   | CREATED
my-ci-webhook   | john.smith    | Active         | Engineering | dev-ops   | 2023-04-12
alerts-webhook  | sarah.jones   | Deactivated ⚠  | Engineering | alerts    | 2022-11-01
...
```

The `⚠` marker (or equivalent) should draw attention to orphaned integrations in table view.

### 9.2 CSV Format

One row per integration. Columns:

```
type, name, creator_username, creator_display_name, creator_status, team, channel, description, created_at, orphaned
```

- `type` — one of: `incoming_webhook`, `outgoing_webhook`, `bot`, `oauth_app`, `slash_command`
- `creator_status` — one of: `active`, `deactivated`, `deleted`
- `orphaned` — `true` or `false`

### 9.3 JSON Format

Array of integration objects:

```json
[
  {
    "type": "incoming_webhook",
    "name": "my-ci-webhook",
    "creator_username": "john.smith",
    "creator_display_name": "John Smith",
    "creator_status": "active",
    "team": "Engineering",
    "channel": "dev-ops",
    "description": "Posts build notifications",
    "created_at": "2023-04-12T09:15:00Z",
    "orphaned": false
  }
]
```

---

## 10. Authentication Detail

The token or user account used MUST have **System Administrator** role. Without this, the API endpoints for listing instance-wide integrations will return 403.

Password handling follows the same pattern as all tools in this family:
- Interactive terminal: prompt with echo suppressed via `golang.org/x/term`
- Non-interactive: use `MM_PASSWORD` environment variable
- Never accept password as a CLI flag

---

## 11. API Endpoints Used

| Endpoint | Purpose |
|----------|---------|
| `GET /api/v4/hooks/incoming?page=N&per_page=200` | List incoming webhooks |
| `GET /api/v4/hooks/outgoing?page=N&per_page=200` | List outgoing webhooks |
| `GET /api/v4/bots?page=N&per_page=200` | List bot accounts |
| `GET /api/v4/oauth/apps?page=N&per_page=200` | List OAuth 2.0 applications |
| `GET /api/v4/commands?custom_only=true` | List custom slash commands |
| `GET /api/v4/users/{user_id}` | Look up creator account status |
| `GET /api/v4/teams/name/{team_name}` | Resolve team name to ID |

---

## 12. Error Handling

- Missing `--url` / `MM_URL`: exit code 1 with clear message
- Authentication failure (401): exit code 1 with clear message
- Permission denied (403): exit code 1, message should explicitly state that a System Administrator account is required
- Unknown team name: exit code 1 with clear message
- Unexpected API error: print status and body to stderr, exit code 2
- Individual record failures mid-run: log to stderr as warnings, skip, continue, and report skip count in summary

---

## 13. Testing Requirements

- Unit tests for orphan detection logic
- Unit tests for each integration type's data mapping
- Unit tests for CSV and JSON output formatting
- Mock API responses for each integration type endpoint

---

## 14. Out of Scope

- Deleting or modifying any integrations
- Re-assigning ownership of orphaned integrations
- Checking whether webhook URLs are reachable
- Reporting on plugin-based integrations

---

## 15. Acceptance Criteria

- [ ] Running with valid credentials lists all integration types across all teams
- [ ] `--orphaned-only` returns only integrations with deactivated/deleted creators
- [ ] `--type incoming` returns only incoming webhooks
- [ ] `--team Engineering` returns only integrations scoped to the Engineering team, plus instance-wide integrations
- [ ] `--format csv --output integrations.csv` produces a valid CSV
- [ ] `--format json` produces valid, `jq`-parseable JSON
- [ ] A non-System-Admin token produces a clear 403 error message
- [ ] All errors to stderr, all data to stdout
- [ ] Binary runs on Linux (amd64), macOS (arm64 and amd64), and Windows (amd64) without dependencies
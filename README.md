# GitHub User Offboarding for Okteto

When an employee leaves your organization and is removed from GitHub, their Okteto account stays active — taking up a seat and leaving their namespaces running. This workflow closes that gap by automatically reconciling the two systems every night: any Okteto user whose GitHub login is no longer part of your organization is removed, along with all namespaces they owned.

This is also an example of how you can use the [Okteto API](https://www.okteto.com/docs/admin/okteto-api/) to integrate Okteto with your organization's existing tooling and workflows. The same approach can be adapted to sync users from other identity sources, enforce policies on namespaces, or automate any other lifecycle event that your team already handles in GitHub.

> **Community project** — This is a community effort and is offered as-is, without warranties or guarantees of support. Use it as a starting point and adapt it to your needs.

## How it works

1. Fetch all current members of the GitHub organization via the GitHub API.
2. Fetch all users registered in the Okteto instance via the Okteto API (`GET /api/v0/users`).
3. Any Okteto user whose login is not found in the GitHub org is deleted (`DELETE /api/v0/users/{id}`), which also removes all namespaces they owned.

Okteto admin users are skipped by default to prevent accidental self-lockout.

## Setup

### 1. GitHub repository secrets

| Name | Description |
|------|-------------|
| `GH_TOKEN` | GitHub Personal Access Token with **`read:org`** scope |
| `OKTETO_TOKEN` | Okteto Admin Access Token (Admin → Admin Access Tokens in the Okteto dashboard) |

### 2. GitHub repository variables

| Name | Example | Description |
|------|---------|-------------|
| `GITHUB_ORG` | `my-company` | GitHub organization name |
| `OKTETO_URL` | `https://okteto.example.com` | Base URL of your Okteto instance |

Go to **Settings → Secrets and variables → Actions** to add them.

### 3. Enable the workflow

The workflow file is at `.github/workflows/offboard-users.yml`. It runs automatically every night at 02:00 UTC once the repository is pushed to GitHub.

## Manual execution

Trigger the workflow from **Actions → Offboard GitHub Users from Okteto → Run workflow**.

The manual trigger defaults to **dry-run mode** so you can preview the list of users that would be removed before committing to a live run.

## Running locally

```bash
docker build -t offboard .

# Dry run (no deletions)
docker run --rm \
  -e GH_TOKEN=ghp_... \
  -e GITHUB_ORG=my-company \
  -e OKTETO_TOKEN=... \
  -e OKTETO_URL=https://okteto.example.com \
  -e DRY_RUN=true \
  offboard

# Live run
docker run --rm \
  -e GH_TOKEN=ghp_... \
  -e GITHUB_ORG=my-company \
  -e OKTETO_TOKEN=... \
  -e OKTETO_URL=https://okteto.example.com \
  offboard
```

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GH_TOKEN` | Yes | GitHub PAT with `read:org` scope |
| `GITHUB_ORG` | Yes | GitHub organization name |
| `OKTETO_TOKEN` | Yes | Okteto admin API token |
| `OKTETO_URL` | Yes | Okteto instance base URL |
| `DRY_RUN` | No | Set to `true` to log removals without deleting |
| `INCLUDE_ADMINS` | No | Set to `true` to also process Okteto admin users |

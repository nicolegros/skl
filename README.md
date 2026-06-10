# skills

A CLI tool to manage agent skill installations from GitHub repositories.

## Install

```bash
go install github.com/nicolaslegros/skills@latest
```

Or build from source:

```bash
make install
```

## Configuration

Config lives at `$XDG_CONFIG_HOME/skills/config.json` (defaults to `~/.config/skills/config.json`).

On Windows: `%APPDATA%/skills/config.json`.

```json
{
  "directories": ["~/.skills"]
}
```

The `directories` array lists where skills get installed. A default config is created automatically on first use.

## Usage

### Install a skill

```bash
# From a single-skill repo
skills install owner/repo

# From a subdirectory in a multi-skill repo
skills install owner/repo grill-me

# Full GitHub URL works too
skills install https://github.com/owner/repo grill-me

# Pin to a specific ref (branch, tag, or commit)
skills install owner/repo --ref v1.0.0

# Install all skills from a repo
skills install owner/repo --all
```

### Update skills

```bash
# Update all installed skills
skills update

# Update a specific skill
skills update grill-me
```

Pinned skills will warn before updating and remain pinned to the new SHA.

### Remove a skill

```bash
skills remove grill-me
```

### List installed skills

```bash
skills list
```

## Authentication

For private repositories, set the `GITHUB_TOKEN` environment variable:

```bash
export GITHUB_TOKEN=ghp_xxxxxxxxxxxx
skills install owner/private-repo
```

## Lock file

Installed skills are tracked in `~/.config/skills/skills-lock.json`. This file records the source repo, subdirectory path, resolved commit SHA, and whether the skill is pinned.

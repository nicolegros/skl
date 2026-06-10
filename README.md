# skl

A CLI tool to manage agent skill installations from GitHub repositories.

## Install

```bash
go install github.com/nicolegros/skl@latest
```

Or build from source:

```bash
make install
```

## Configuration

Config lives at `$XDG_CONFIG_HOME/skl/config.yaml` (defaults to `~/.config/skl/config.yaml`).

On Windows: `%APPDATA%/skl/config.yaml`.

```yaml
# Directories where skills are installed
directories:
  - ~/.kiro/skills       # Kiro
  - ~/.copilot/skills    # Copilot
  - ~/.claude/skills     # Claude
  - ~/.agents/skills     # Codex
  - ~/.pi/agent/skills   # Pi
```

The `directories` list specifies where skills get installed. A default config is created automatically on first use. Remove any entries for providers you don't use.

## Usage

### Install a skill

```bash
# From a single-skill repo
skl install owner/repo

# From a subdirectory in a multi-skill repo
skl install owner/repo grill-me

# Full GitHub URL works too
skl install https://github.com/owner/repo grill-me

# Pin to a specific ref (branch, tag, or commit)
skl install owner/repo --ref v1.0.0

# Install all skills from a repo
skl install owner/repo --all
```

### Update skills

```bash
# Update all installed skills
skl update

# Update a specific skill
skl update grill-me
```

Pinned skills will warn before updating and remain pinned to the new SHA.

### Remove a skill

```bash
skl remove grill-me
```

### List installed skills

```bash
skl list
```

## Authentication

For private repositories, set the `GITHUB_TOKEN` environment variable:

```bash
export GITHUB_TOKEN=ghp_xxxxxxxxxxxx
skl install owner/private-repo
```

## Lock file

Installed skills are tracked in `~/.config/skl/skl.lock`. This file records the source repo, subdirectory path, resolved commit SHA, and whether the skill is pinned.

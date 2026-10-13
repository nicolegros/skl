# skl

Install, update, and remove AI agent skills from GitHub repositories, across multiple providers at once.

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

```yaml
# Directories where skills are installed
directories:
  - ~/.kiro/skills       # Kiro
  - ~/.copilot/skills    # Copilot
  - ~/.claude/skills     # Claude
  - ~/.agents/skills     # Codex
  - ~/.pi/agent/skills   # Pi
```

The `directories` list specifies where skills get installed.
A default config is created automatically on first use.
Remove any entries for providers you don't use.
A directory may not sit inside the skill store.

## How skills are stored

Each skill is stored once under `$XDG_DATA_HOME/skl/skills/<name>/` (defaults to `~/.local/share/skl/skills/<name>/`).
Every configured directory gets a symlink `<dir>/<name>` pointing at that store copy with an absolute path, so all providers see the same files.
Editing a skill through any of its links edits the single store copy.

Every `install`, `update`, and `remove` first reconciles the lock file with disk.
It fetches any store copy that is missing at the locked commit, creates missing links, and removes `skl`'s own links from directories you dropped from the config.
`skl install` with no arguments only reconciles.
`skl` never deletes a real directory or a symlink it did not create unless you choose to overwrite it.

### Migrating from copied installs

Earlier versions copied each skill into every directory.
The first `install`, `update`, or `remove` after upgrading fetches each skill into the store at its locked commit and replaces each copy with a link.
Copies identical to upstream are replaced silently.
A copy you edited is reported as a conflict and goes through the prompt below.
If you skip it, `skl` leaves it in place and does not ask again; `skl list` keeps showing it as unlinked.

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

# Install under a different name
skl install owner/repo grill-me --as interview-me

# Install all skills from a repo
skl install owner/repo --all

# Force overwrite local modifications without prompting
skl install owner/repo --force
```

The `--as` flag installs the skill under a custom name. The skill's `SKILL.md` frontmatter `name:` field and internal path references are patched to match. `--as` and `--all` are mutually exclusive.

### Update skills

```bash
# Update all installed skills
skl update

# Update a specific skill
skl update grill-me

# Force overwrite local modifications without prompting
skl update --force
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

Installed skills are tracked in `~/.config/skl/skl.lock`.
This file records the source repo, subdirectory path, resolved commit SHA, whether the skill is pinned, the alias (if installed with `--as`), SHA-256 checksums of each file in the store copy, and `links`, the directories currently holding a link to it.

## Conflicts

Before overwriting anything, `skl` checks for:

- a store copy whose files no longer match the lock checksums (you edited the skill),
- a real directory at `<dir>/<name>` whose content differs from the store,
- a symlink at `<dir>/<name>` pointing somewhere other than the store.

When it finds any, it prompts once per skill:

- **[d]iff** - show a unified diff against upstream at the locked commit (or the symlink target)
- **[b]ackup & overwrite** - back up, then overwrite
- **[o]verwrite** - overwrite without backup
- **[s]kip** - leave it unchanged

Use `--force` on `install` or `update` to skip this prompt and overwrite immediately.

Backups go to `$XDG_DATA_HOME/skl/backups/<name>/<timestamp>/`, followed by the full original path.
For example, a backup of `~/.claude/skills/review` lands at `~/.local/share/skl/backups/review/2025-01-31T09-30-00/Users/you/.claude/skills/review/`.
For a symlink, the contents of its target are backed up.
Backups are never pruned.

`skl list` appends `(unlinked: <dir>, ...)` to any skill missing its link in a configured directory.

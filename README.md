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

`skl` reads `$XDG_CONFIG_HOME/skl/skl.yaml` (defaults to `~/.config/skl/skl.yaml`).
It lists where skills are linked and which skills you want.

```yaml
# Directories where skills are installed
directories:
  - ~/.kiro/skills       # Kiro
  - ~/.copilot/skills    # Copilot
  - ~/.claude/skills     # Claude
  - ~/.agents/skills     # Codex
  - ~/.pi/agent/skills   # Pi
skills:
  axi: kunchenguid/axi
  grill-me: mattpocock/skills/grill-me
  interview-me: mattpocock/skills/grill-me@v1.0.0
```

The `directories` list specifies where skills get linked.
Remove any entries for providers you don't use.
A directory may not sit inside the skill store.

Each `skills` entry maps the installed name to a source written `owner/repo[/path][@ref]`.
The first two segments are the GitHub repository and the rest is the skill's directory inside it.
`@ref` is a branch, tag, or commit SHA; without it, `skl` follows the default branch.
The installed name may differ from the directory name upstream, as `interview-me` does above.
The skill's `SKILL.md` frontmatter `name:` field and internal path references are then patched to match.

A default `skl.yaml` with `skills: {}` is created on first use.
Edit it by hand or through `skl install` and `skl remove`.
When `skl` writes the file, it only inserts, replaces, or deletes `skills` entries, keeping new entries sorted, and leaves your comments and `directories` untouched.

Every command validates the whole file first.
Invalid YAML, a source that does not parse, or an empty name stops the command with the line number, before anything on disk changes.

### Migrating from config.yaml

Earlier versions kept `directories` in `config.yaml` and the installed skills only in the lock file.
The first command after upgrading writes `skl.yaml` from the text of `config.yaml`, comments included, plus one `skills` entry per lock entry, then deletes `config.yaml`.
A skill installed with `--ref` keeps that ref in its entry.
Nothing else changes on disk, so the next `skl install` has nothing to do.

## How skills are stored

Each skill is stored once under `$XDG_DATA_HOME/skl/skills/<name>/` (defaults to `~/.local/share/skl/skills/<name>/`).
Every configured directory gets a symlink `<dir>/<name>` pointing at that store copy with an absolute path, so all providers see the same files.
Editing a skill through any of its links edits the single store copy.

Every `install`, `update`, and `remove` makes disk match `skl.yaml`:

- a skill listed but not installed is fetched at its ref, stored, and linked,
- a skill whose repo, path, or ref changed in `skl.yaml` is reinstalled from the new source,
- an installed skill no longer listed is removed: its links, its store copy, and its lock entry,
- every other skill stays at its locked commit; a missing store copy is fetched again and missing links are created.

`skl` also removes its own links from directories you dropped from `directories`.
If one skill fails to fetch, the error is reported as `error: <name>: <source>: <reason>`, the others still proceed, and the command exits non-zero.
A failed skill is not recorded, so the next run tries again.
`skl` never deletes a real directory or a symlink it did not create unless you choose to overwrite it.

### Migrating from copied installs

Earlier versions copied each skill into every directory.
The first `install`, `update`, or `remove` after upgrading fetches each skill into the store at its locked commit and replaces each copy with a link.
Copies identical to upstream are replaced silently.
A copy you edited is reported as a conflict and goes through the prompt below.
If you skip it, `skl` leaves it in place and does not ask again; `skl list` keeps showing it as unlinked.

## Usage

### Install skills

```bash
# Install everything skl.yaml lists, e.g. on a new machine
skl install

# From a single-skill repo
skl install owner/repo

# From a subdirectory in a multi-skill repo
skl install owner/repo/grill-me
skl install owner/repo grill-me

# Full GitHub URLs work too, including links to a directory
skl install https://github.com/owner/repo/tree/main/grill-me

# At a specific ref (branch, tag, or commit)
skl install owner/repo@v1.0.0
skl install owner/repo --ref v1.0.0

# Install under a different name
skl install owner/repo/grill-me --as interview-me

# Install all skills from a repo, one skl.yaml entry each
skl install owner/repo --all

# Force overwrite local modifications without prompting
skl install owner/repo --force
```

`skl install <source>` installs the skill and adds its entry to `skl.yaml`, written in the normalized `owner/repo[/path][@ref]` form.
The entry is only written once the skill is installed.
If the name is already listed with the same repo and path, the entry's ref is replaced and the skill reinstalled.
If it is listed from a different repo or path, `skl` refuses; pick another name with `--as`, or edit `skl.yaml` to switch the source.
`--as` and `--all` are mutually exclusive.

### Update skills

```bash
# Update all installed skills
skl update

# Update a specific skill
skl update grill-me

# Force overwrite local modifications without prompting
skl update --force
```

`skl.yaml` holds the ref you asked for and the lock file holds the commit it resolved to.
`skl install` always uses the locked commit, so every machine with the same lock file gets the same files.
`skl update` resolves each ref again and moves the lock to the new commit.
A skill without a ref follows the default branch, and a branch ref follows that branch.
A tag or commit SHA resolves to the same commit, so those skills stay where they are.
`skl update` never edits `skl.yaml`; to move a skill to another tag, change its ref in `skl.yaml` or run `skl install` with the new ref.

### Remove a skill

```bash
skl remove grill-me
```

This deletes the entry from `skl.yaml` and removes the skill.
Deleting the entry by hand and running `skl install` does the same.

### List skills

```bash
skl list
```

Each row shows the name, the source as written in `skl.yaml`, and the short commit when installed.
A row may end with:

- `(not installed)` for an entry `skl install` has not installed yet,
- `(changed)` for an entry whose repo, path, or ref differs from what is installed,
- `(not in skl.yaml, removed on next install)` for an installed skill with no entry,
- `(unlinked: <dir>, ...)` for configured directories missing the skill's link.

## Authentication

For private repositories, set the `GITHUB_TOKEN` environment variable:

```bash
export GITHUB_TOKEN=ghp_xxxxxxxxxxxx
skl install owner/private-repo
```

## Lock file

Installed skills are tracked in `~/.config/skl/skl.lock`.
This file records, for each skill, the source repo and subdirectory path, the declared ref from `skl.yaml`, the commit SHA it resolved to, the upstream name when installed under another name, SHA-256 checksums of each file in the store copy, and `links`, the directories currently holding a link to it.

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

Removing a skill whose store copy you edited goes through the same prompt; skipping keeps the skill installed and asks again on the next run.
Use `--force` on `install` or `update` to skip this prompt and overwrite immediately.

Backups go to `$XDG_DATA_HOME/skl/backups/<name>/<timestamp>/`, followed by the full original path.
For example, a backup of `~/.claude/skills/review` lands at `~/.local/share/skl/backups/review/2025-01-31T09-30-00/Users/you/.claude/skills/review/`.
For a symlink, the contents of its target are backed up.
Backups are never pruned.

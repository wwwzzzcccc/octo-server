---
type: Task
title: "Task: group-avatar-name-no-text"
description: Default group avatar no longer renders the group name as text; it is the two-person icon unless a custom avatar_text is set. Reverses the is_named name-text render rule (global, existing groups included).
tags: [group, avatar, render, cache, product-pivot]
timestamp: 2026-06-29T12:00:00Z
# --- octospec extension fields ---
slug: group-avatar-name-no-text
upstream: group-chat-avatar-gen (product re-pivot, 2026-06-29)
supersedes: group-avatar-icon-default (S2 is_named name-text render rule)
source: self
---

# Task: group-avatar-name-no-text

> Server-side follow-up to the "group chat avatar" redesign. Product re-pivot
> (2026-06-29, same day as `group-avatar-icon-default`): the group **name** must
> never be used as avatar text. The default avatar is the two-person icon unless
> the user explicitly set a custom `avatar_text`.

## Goal
One render-rule change: collapse the avatar text-selection priority from three
tiers to two.

- **Before (S2, `group-avatar-icon-default`):** custom `avatar_text` >
  named-group name (`is_named=1` → `GroupNameText` first-2 glyphs) > two-person
  icon.
- **After (this task):** custom `avatar_text` > two-person icon. The group name
  is *never* an avatar-text source — not for user-named groups, not for
  member-concat auto names. `is_named` is no longer read by the render path.

Decision (user, 2026-06-29): **global** — existing user-named groups also flip to
the icon (no grandfather). Reason: product considers name-text avatars (e.g.
`我靠` → 「我靠」) undesirable; consistency over preserving existing renders.
Existing groups converge on the icon within the avatar `max-age` (≤5 min) via the
content-derived ETag (see below); no migration/backfill needed.

`is_named` column + lifecycle (create/rename/event/AddGroup writes, `GroupResp`
exposure) are **kept dormant** — column not dropped, no new migration. Cheapest,
fully reversible change, and keeps the PR #500 P1 clobber-fix (`UpdateInviteTx`)
and its tests intact. Only the render path stops consuming the flag.

## Background
- The is_named name-text rule shipped in PR #500 (`group-avatar-icon-default`).
  The render decision lives in `writeGroupDefaultAvatar` (`modules/group/api.go`).
- The web side (octo-web) is already aligned: the create/edit preview renders the
  two-person icon when `avatar_text` is empty, regardless of name. This server
  change makes the rendered PNG match that preview.

## Load-bearing list
- **`writeGroupDefaultAvatar` text-selection (`modules/group/api.go`).** Removed
  the `else if groupInfo.IsNamed == 1 { text = GroupNameText(name) }` branch. Now:
  `avatar_text != "" → GroupText(avatar_text)`, else `text == ""` → icon. The name
  is never read for avatar text.
- **ETag / cache identity unchanged in mechanism, self-converging on the flip.**
  ETag is CRC32 over factors (mode-version + group_no + color + text). Custom-text
  groups use `group-name-v4` (with text); everyone else uses `group-icon-v3` (no
  text). Existing named groups go from `group-name-v4 + <name>` to
  `group-icon-v3` (no text) → the factor string changes → ETag changes on its own
  → clients revalidate (≤ `max-age=300`) to the icon. **No render-version bump**
  (`RenderIcon`/`RenderGroup` pixels are untouched; only *which* groups land in
  which mode changed).
- **`is_named` kept dormant.** Column, migration `20260629000001`, and all
  lifecycle writes (`CreateGroup`, `UpdateGroupInfo` rename, `event.go`
  system/org/dept, `Service.AddGroup`) are unchanged; `GroupResp.is_named` still
  exposed (additive, now informational only). Render path no longer reads it.
- **`avatarrender.GroupNameText`** is no longer called from the handler. It
  remains an exported, unit-tested function in `pkg/avatarrender` (Go does not
  flag exported-unused); left in place for potential reuse, not removed.
- **Comments + swagger updated** across `modules/group/{api.go,db.go,service.go,
  const.go}` and `swagger/api.yaml` to the new rule ("空=双人图标，群名不作为头像
  文字来源"); `is_named` docs note it is recorded but not consumed by rendering.
- **Disbanded/nonexistent 404 guard** (`api.go` `avatarGet`) unchanged and still
  justified: rendering a disbanded group would still leak a custom `avatar_text`
  (if set) and expose the disbanded-vs-never-existed enumeration surface; the
  comment was corrected (it previously said "renders the name").

## Out of scope
- Any change to custom `avatar_text`/`avatar_color` create/update APIs,
  validation, upload path, palette endpoint/values, or the `is_named` lifecycle
  writes.
- Dropping the `is_named` column (kept dormant; reversible).
- Render version bump (intentionally none — see load-bearing list).
- octo-web (already aligned with the icon-default preview).

## Acceptance
- `writeGroupDefaultAvatar` with no custom `avatar_text`: any group (is_named=1 or
  0, named or auto, existing or new) → `RenderIcon`. Custom color still honored.
  Custom `avatar_text` always overrides → `RenderGroup(avatar_text)`.
- Tests: `TestGroupAvatarGetNamedRendersIcon` (was `...RendersNameText`) and
  `TestGroupAvatarGetNamedCustomColorRendersIcon` (was `...RendersNameText`) now
  assert the icon; auto-named/empty/custom-text/uploaded/404/disband + palette +
  is_named-lifecycle (`TestCreateGroup_*`, `TestUpdateGroupInfo_RenameMarksIsNamed`,
  `TestRenameThenInviteUpdate_KeepsIsNamed`, `TestAddGroup_SetsIsNamed`)
  regressions still pass.
- `go build ./...`, `go vet`, `golangci-lint`, `make i18n-lint` all green.
  (Endpoint tests needing MySQL/Redis/WuKongIM run in CI.)

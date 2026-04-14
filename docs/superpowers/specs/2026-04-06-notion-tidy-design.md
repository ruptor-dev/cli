# `/notion-tidy` — Notion Workspace Organizer Skill

**Date**: 2026-04-06
**Type**: Claude Code slash command (manual skill)
**Location**: `~/.claude/skills/notion-tidy/SKILL.md`

## Purpose

Find orphan pages in the user's Notion workspace (pages not nested under any known container) and interactively suggest where to move them.

## Workspace Structure

The user organizes Notion around top-level container pages that hold child pages:

- **Areas** — Ongoing responsibilities (Career, Finance, Hobbies, Philosophy)
- **Projects** — Active work with deadlines (Homelab, ElPoshoX, SaaS Opportunities)

Pages that exist outside these containers are considered "orphans" and need to be filed or grouped.

## Design

### Phase 1: Discover Containers

1. Run multiple `notion-search` queries with varied terms to surface workspace pages broadly. The Notion MCP `notion-search` tool does not support pagination or enumeration of all pages, so multiple targeted queries are needed:
   - Search by known container names ("Areas", "Projects")
   - Search by known child names ("Career", "Finance", "Hobbies", "Philosophy", "Homelab")
   - Search generic terms that might surface unknown pages ("notes", "docs", "resources", "admin", "personal")
   - Search single common letters/words to catch miscellaneous pages
2. Deduplicate results by page ID.
3. For each discovered page, call `notion-fetch` to read its content and ancestor path. The `ancestor-path` field in the response determines whether a page is top-level (empty ancestor path) or nested under another page. This is the only way to determine hierarchy — `notion-search` results do not include ancestry.
4. Build the organized set:
   - **Containers**: top-level pages that have child pages listed in their content.
   - **Organized pages**: all children of containers.
   - Note: empty top-level pages (no children) are NOT containers — they are orphan candidates.

### Phase 2: Find Orphans

From all discovered pages, filter for orphans:
- **Is not** a container root (Areas, Projects, or any other page with children)
- **Is not** a child of any container (not in the organized set)
- **Is not** a database (only loose pages qualify — filter by `type: "page"` from search results)

Result: list of orphan page IDs + titles.

### Phase 3: Classify

For each orphan:
1. Content is already fetched from Phase 1 step 3 — reuse it, no extra API calls needed.
2. Dynamically build the classification map by reading each container's content (also already fetched). Extract the container's description and child page names/descriptions to infer themes. This way, if the user adds "Areas/Health" in the future, the skill picks it up automatically without code changes.
3. Use LLM judgment (your own reasoning) to match orphan content against container themes. Assign a confidence level:
   - **High**: content clearly matches one container
   - **Medium**: content partially matches, or could fit multiple containers
   - **Low**: no clear match
4. If 2+ orphans share a theme that doesn't fit existing containers, suggest creating a new container. Use LLM judgment to name the proposed container based on the shared theme.

### Phase 4: Interactive Report

Present all results in a single report first, then ask for confirmation.

**Report format:**
```
X orphan pages found:

Move to existing container:
  1. PageName → Areas/Finance (high confidence)
  2. PageName → Areas/Hobbies (high confidence)

Suggest new container "Resources" (under workspace root):
  3. PageName + PageName → new container (medium confidence)

Unclear:
  4. PageName — no clear match, please advise
```

**Confirmation flow** via `AskUserQuestion`:
- Present the full report, then ask: "Which moves do you approve? (e.g., '1,2,3', 'all', or 'none'). For any you want to redirect, say '4 → Areas/Career'."
- Single interaction — no per-item prompting.

**Execution:**
- For new containers: ask the user whether to create under Areas or Projects (or workspace root). Create the page via `notion-create-pages`, then move orphans into it.
- `notion-move-pages` accepts a list of page URLs/IDs and a target parent page URL/ID. Process confirmed moves one at a time to isolate failures.

**Completion summary:**
```
Done: moved 3, skipped 1, failed 0.
```

## Constraints

- **Manual only**: Invoked via `/notion-tidy`, never scheduled. Moving pages is a destructive action that requires human judgment.
- **Read-heavy, write-cautious**: Fetches and analyzes freely, but only moves pages after explicit confirmation.
- **Search coverage**: Notion search is not exhaustive. The skill runs multiple varied queries to maximize coverage but may miss pages with unusual or empty titles. The report should note: "Note: Notion search may not surface all pages. Run again if you suspect missing orphans."
- **Scope**: Only reorganizes pages, not standalone databases.
- **No memory between runs**: Each invocation starts fresh. Skipped pages will appear again on the next run. This is intentional — the user's intent may change between runs.
- **Idempotency**: Running twice after a successful run produces "0 orphan pages found." Partial runs are safe — each move is independent, and no page can end up in an inconsistent state (Notion moves are atomic re-parents).

## Tools Used

- `notion-search` — discover pages across workspace
- `notion-fetch` — read page content, ancestor path, and child pages
- `notion-move-pages` — relocate confirmed orphans (takes page URLs + target parent URL)
- `notion-create-pages` — create new container pages at a specified parent
- `AskUserQuestion` — batch confirm moves interactively

## Error Handling

- If `notion-search` returns no results for a query, skip and continue with other queries.
- If `notion-fetch` fails for a page (deleted, permission), skip that page and note it in the report.
- If `notion-move-pages` fails for a page, report the error and continue with remaining moves.
- Never move a page without user confirmation.

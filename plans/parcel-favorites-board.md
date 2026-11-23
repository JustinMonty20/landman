# Private parcel favorites board

## Status and scope

Architecture only. This PR does not implement storage, routes, UI, or reminder delivery. Unchecked acceptance checks describe future work, not passed verification.

The operator approved one favorites list and one private Kanban board per user. A Kanban board groups parcel cards into columns that represent lead states. Users define their own states. There are no separate named lists, multiple boards, or list memberships.

This plan defines the private workflow required by [V1](v1.md) and [the browser application](browser-parcel-ui.md). For this workflow, their references to saved lists mean this single favorites collection. Preserve their other commitments, including saved-parcel tax requests and personal SMS reminder delivery. This plan does not implement those requests or remove their release gates.

## Existing code and proposed seam

`internal/auth/auth.go` supplies Google sign-in, approved-email access, database-backed sessions, and authenticated account identity. `app_users.id` is the stable local identity. Google subject identifies the external account; email is not the ownership key. Account-owned parcel detail preferences already exist. Shared county parcels use `parcels.id`.

Add one concrete `internal/parcelcrm` module. CRM means customer relationship management; here it means the buyer's private parcel tracking workflow. Its public operations own transactions, ownership checks, revisions, and activity creation. Keep SQL and private data rules there. Do not add a forwarding-only storage module or a swappable store interface with one implementation.

Reuse session validation from auth through a narrow authenticated-handler seam. Resolve the account once and pass its typed identity into CRM operations. Do not move board behavior into auth. Existing preference handlers can remain unchanged. Proposed private tax observations can later reference the same user and parcel pair without placing tax request logic inside board operations.

Use the existing Go API and Svelte application. Do not add another service, identity provider, or browser application.

## Domain contract

### Favorites and states

- One user has one implicit board. Do not create a board selector or board table.
- One user and parcel pair has at most one private tracking record. Its active favorite appears as exactly one card in exactly one state.
- Create a default `New` state on first board use. Initialization must be safe under concurrent requests. The user can rename it and select a different default.
- Users can create, rename, reorder, and delete custom states. Do not impose a fixed product count. Page state reads and use request-size and rate limits to bound work.
- State names must be nonempty after trimming and unique per user without case differences. Use stable state IDs; names and order can change.
- Deleting an occupied state requires a destination state owned by the same user. Move affected records and record their state changes in one transaction. The default state also requires a replacement. Keep at least one state.
- Quick actions and board moves invoke the same state-change operation. Do not infer lead state from owner labels, county updates, or elapsed time.
- Use stable card ordering by favorite creation time and ID within each state initially. Manual card ordering within a state is outside this plan.

### Exclusion with a reason

Preserve the browser plan's explicit exclude-lead action. Exclusion is a separate private flag with a required nonempty reason, not a reserved state. It leaves the favorite in its user-selected state. Show an Excluded label and the reason on the card and in parcel details. Show excluded favorites by default; provide an explicit hide-excluded control. Record exclusion and restoration in activity. Restoration clears the active flag and reason but retains their history. Exclusion cancels pending reminders in the same transaction; restoration does not reactivate them. Reject new reminders while excluded. Users can still edit notes and change state. Removing or permanently deleting a favorite remains a separate action.

### Notes and activity

Each user and parcel pair has its own scratchpad. Use dated plain-text entries. Allow entry creation, editing, and deletion. Show created and edited times. Render text safely; do not accept executable HTML. Moving states does not change notes. Different users have independent notes for the same parcel.

Keep a separate activity history for favorite changes, state changes, reminder actions, and dated manual contact activity. Users can record a handwritten note sent, a response received, or a visit. Store both the reported occurrence time and the server creation time for manual activity. Correct manual activity through an explicit edit or deletion. System state-change records are not editable through manual-activity operations. Do not copy note bodies into system activity or logs. Deleting a note removes its content; activity may retain the fact that a note was deleted.

### Removal and permanent deletion

Removing a favorite hides its card and cancels pending reminders atomically. Retain its notes, state, and history. Reject new reminders while the favorite is inactive. Explain this behavior in the confirmation. Favoriting again restores the private record and its retained state; canceled reminders stay canceled.

Provide a separate confirmed action to permanently delete the user's tracking record, notes, activity, and reminders. Do not delete shared county parcels or other users' records. A later favorite starts a new private record. Account deletion must remove all account-owned CRM data; account self-service deletion is not added by this plan.

Source refreshes preserve private records. Use a restrictive foreign key to shared parcels, not cascading deletion of private work. Existing parcel refresh code must be checked for compatibility before migration. If a source parcel becomes unavailable, retain the private record and show unavailable source evidence. Do not silently attach it to another parcel after a split or merge.

## Storage design

Use a new migration at the next free version at implementation time. Do not reserve a version here or edit applied migrations.

| Table | Required data and constraints |
| --- | --- |
| `user_lead_states` | User ID, state UUID, name, normalized name, sort position, timestamps, revision. Unique user/name and user/state keys. |
| `user_crm_settings` | One row per user; default state with a same-user composite foreign key. |
| `user_parcels` | User ID, parcel UUID, state ID, favorite flag, exclusion flag and reason, favorite creation time, timestamps, revision. Require a nonempty reason exactly when excluded. Unique user/parcel. Same-user state foreign key. Restrictive parcel reference. |
| `user_parcel_notes` | Entry UUID, user/parcel reference, body, timestamps, revision. |
| `user_parcel_activity` | Entry UUID, user/parcel reference, typed activity, occurrence time, creation time, limited structured evidence, revision for editable manual entries. Preserve state-name snapshots for historical display after rename or deletion. |
| `user_parcel_reminders` | Reminder UUID, user/parcel reference, task text, due instant, input timezone, status, timestamps, revision. |
| `user_reminder_deliveries` | Reminder version, channel, delivery state, attempts, next attempt, provider reference. Unique logical notification key. No credentials or message bodies in logs. |

Cascade private child deletion from the user's tracking record and account. Enforce same-user relationships with composite foreign keys, not application checks alone. Index board reads by user, favorite flag, state, favorite creation time, and parcel ID. Index reminder queues by user/status/due time and worker eligibility. Keep private text out of public parcel tables, source extras, map tiles, and shared caches.

Use revision checks on edits. Return a conflict for stale writes; do not silently replace another tab's changes. Append activity in the same transaction as the change. Make favorite creation idempotent. Use an operation token for retried note, activity, and reminder creation to prevent duplicates.

## Private HTTP operations

All proposed routes are under `/api/me`. The server derives ownership from the session. A request cannot select another user. Apply existing exact-origin protection to mutations, strict JSON validation, and bounded request bodies. Return 404 for missing or other-user records without exposing ownership. Use bounded cursor pages rather than full-board downloads.

| Route group | Operations |
| --- | --- |
| `/crm/board` | Read states, counts, and default state; read independently paged cards per state. |
| `/crm/states` | Create state; rename, reorder, or delete with explicit reassignment. |
| `/crm/settings` | Change default state. |
| `/favorites/{parcel_id}` | Read private tracking summary; idempotently favorite; remove favorite; permanently delete private tracking through a separate confirmed operation. |
| `/favorites/{parcel_id}/state` | Change state with expected revision. |
| `/favorites/{parcel_id}/exclusion` | Exclude with a required reason or restore, with expected revision. |
| `/favorites/{parcel_id}/notes` | Page, create, edit, and delete scratchpad entries. |
| `/favorites/{parcel_id}/activity` | Page history and manage dated manual entries. |
| `/favorites/{parcel_id}/reminders` | Create, edit, complete, snooze, reschedule, and cancel personal tasks. |
| `/crm/reminders` | Page upcoming and overdue personal tasks across parcels. |

Exact method and payload specifications belong to implementation review. Every mutation must define retry and conflict behavior before coding. Shared parcel reads remain separate from these private reads. Saving or viewing a favorite must not request live county tax data.

## Reminder delivery and open decisions

### Approved SMS requirements

- Run production on AWS. The exact AWS runtime is not yet selected.
- Use AWS SMS delivery to the user's own verified US phone number. Do not support other destination countries in the first release. A `+1` prefix alone does not establish a US destination.
- Let the user enter reminder text and select a date and time for a favorite parcel.
- Send once per scheduled reminder. Successful delivery does not complete the task. Do not repeat automatically until completion. Explicit snooze or reschedule creates a new scheduled version.
- Send only the user's text. Do not append a parcel identifier, link, application name, or other text to the reminder body. Carrier-required sender and opt-out behavior must be checked before release; do not silently change this content contract.
- Limit the body to one SMS segment using the encoding of the complete message. GSM-7 supports up to 160 septets; extension characters consume two septets. Unicode SMS generally supports 70 UTF-16 code units; supplementary characters consume two units. Verify the selected AWS product's encoding behavior before implementation acceptance. Do not use a universal character-count limit, truncate text, or silently send multiple segments. Show remaining capacity in the editor and apply the same validation on the server.
- Target submission to AWS within 60 seconds of the due instant under normal operation. Measure due-to-provider-acceptance time. This is not a guarantee of carrier delivery time. After an outage, submit eligible unsent reminders no more than one hour after their due instant. After that window, mark delivery missed, show it to the user, and allow explicit rescheduling. Keep the task open. This recovery rule does not permit automatic repeats of accepted messages.

The operator selected SMS-only reminders for V1. Email reminder delivery and email reminder preferences are deferred. The in-app task queue remains required. This explicitly replaces the earlier V1 email/SMS scope.

Store due times as UTC instants and retain an IANA timezone identifier for local display. Require explicit offset selection for ambiguous daylight-saving times; reject nonexistent local times. Initial reminders are one-time tasks. Recurrence is outside scope.

Task completion and delivery success are different states. A delivered notification does not complete a task. Snooze and reschedule replace the due instant and invalidate older pending delivery versions. Cancellation, removal from favorites, and permanent deletion prevent new delivery claims. Recheck status and version before provider submission. Explain that an already accepted provider request cannot be recalled.

Use durable delivery records with no automatic retries after a provider submission attempt. If AWS explicitly rejects a send, show a failed status and require an explicit user action for another send. Do not retry temporary service errors automatically. Do not automatically retry a submission when provider acceptance is uncertain. Mark its delivery state as uncertain and let the user explicitly authorize another send after a duplicate warning. Keep these submission outcomes separate from recovery of work that was never submitted. Only never-submitted work is eligible for automatic outage recovery within the one-hour window. Any new send must still pass current task, destination, and scheduling checks. Claim work transactionally. Recover expired claims after interruption. Use provider idempotency where supported; do not promise exactly-once external delivery when provider acceptance is uncertain. Keep failures visible. Notifications go only to the buyer's verified destination, never an owner.

Before delivery implementation, select the exact AWS runtime and SMS product, sender registration, verified destination process, consent and opt-out behavior, and final sender-access requirements. The operator selected no automatic retries for definite failures or uncertain acceptance. AWS hosting, US-only SMS, single-segment user text, a 60-second normal submission target, and a one-hour outage recovery window are approved. Do not infer deployment from local tools. Do not add deployment artifacts until those decisions are made. An in-app queue can be delivered first, but it does not satisfy V1 SMS delivery commitments.

Saved-parcel tax observation retention and integration remain governed by [the tax plan](tax-data-ingest.md). Before permanent deletion ships, define how it removes the user's private tax observations. Do not leave private child data orphaned or claim that this plan delivers the tax action.

### SMS acceptance checks

- [ ] Verify US destination checks, account ownership, consent, phone verification, opt-out handling, and sender registration with the selected AWS product.
- [ ] Test one-segment validation at encoding limits, GSM-7 extension characters, Unicode, and supplementary characters. Verify exact user text and no automatic additions.
- [ ] Use a controlled clock and a local delivery test double to verify the 60-second submission target, one-hour recovery cutoff, missed delivery status, and no automatic repeat after provider acceptance. Live AWS acceptance is a separate opt-in check.

## UI contract

Add a Favorites board view to the current application. Keep discovery and the board separate views of the same parcel identities. Each card shows parcel identity, state, next reminder, and note preview. Selecting it opens the existing parcel detail context with private notes and actions. Keep source facts distinct from user content.

Support drag moves and a keyboard-accessible Change state action. Show pending changes and save failures. Restore the confirmed state after a failed optimistic move. Handle revision conflicts explicitly. Allow state management without drag controls. Preserve the existing map and panel layout when discovery is active.

Page cards per column and load note history on demand. Do not fetch full parcel geometry or every note for every card. Offer accessible empty, loading, error, unavailable-parcel, and overdue states. Do not use color as the only state cue.

## Implementation order and acceptance

1. Implement schema, account isolation, default initialization, and the concrete CRM module. Verify on a disposable migrated PostGIS database.
2. Implement favorite, state, note, and activity operations with retry and concurrency tests.
3. Implement board and parcel quick actions. Verify both change the same record.
4. Implement reminder tasks and the in-app queue. Add external delivery only after the open decisions are resolved.
5. Integrate with saved-parcel tax actions under their separate contract. Complete V1 workflow checks before release.

- [ ] Two users can favorite the same parcel without sharing notes, states, activity, or reminders. Test guessed IDs for every read and mutation.
- [ ] Concurrent first use creates one default state and settings record. Repeated favorite requests create one card.
- [ ] Create, rename, reorder, and delete states. Test occupied/default deletion, foreign-user destinations, concurrent moves, and stale revisions.
- [ ] Board moves and parcel quick actions agree after reload. Note edits and state changes preserve unrelated data.
- [ ] Remove and restore a favorite; retain notes and cancel pending reminders. Test permanent deletion and account cascade without shared-parcel deletion.
- [ ] Verify parcel refresh, missing-source evidence, and restrictive deletion behavior with the canonical import path.
- [ ] Test reminder due times, daylight-saving cases, snooze, reschedule, completion, cancellation, restart recovery, duplicate creation, delivery retries, and cancellation/submission races.
- [ ] Verify no private text enters shared responses, tiles, caches, or logs. Test safe rendering and exact-origin mutation rejection.
- [ ] Use seeded two-account fixtures for routine tests. Do not require live county sources, operator files, or full-county imports.
- [ ] Run focused Go tests, database integration tests, UI checks/unit tests/build, and the repository CI equivalents selected under [review verification](../docs/review-verification.md).
- [ ] After UI changes, use `/landman-ui-evidence` for desktop/tablet board actions, keyboard moves, notes, state management, removal, reminders, and two-account isolation. Give reviewers artifact paths, actions, expected/observed results, and evidence limits.
- [ ] Verify exclusion reasons, visible excluded cards, reminder cancellation, rejected reminder creation for excluded or inactive favorites, restoration without reminder reactivation, and independence from custom states. Verify that dated manual contact records do not send owner messages.
- [ ] Before implementation acceptance, agree numeric latency and response-size budgets. Measure board pages and writes with at least 1,000 favorites, 50 states, and two simultaneous users. This fixture is a test baseline, not a product limit. Measure larger state counts before claiming unlimited practical scale.

Select independent review checks for ownership/database integrity and UI/reminder behavior at implementation time. This architecture-only PR needs document consistency, link, naming, and independent design review. It does not require application builds or browser evidence because it changes no application behavior.

## Exclusions

No multiple lists, multiple boards, shared/team boards, owner-facing delivery, generated messages, mailing exports, campaigns, recurring tasks, or automated lead-state inference. Actual buyer contact and handwritten note creation remain outside the application. Users can record those manual actions and their dates inside the application. Estate/trust label work and current checkout UI changes are separate work.

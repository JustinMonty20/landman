# Application V1: land discovery and lead tracking

## Product direction

Help a vacant-land buyer find suitable parcels, understand the available evidence, and keep track of his own handwritten notes to owners and subsequent conversations. Start with Union County, North Carolina.

The application provides parcel intelligence, a personal scratchpad, a lead state machine, and follow-up notifications. The buyer writes and sends handwritten notes outside the application. The application does not generate, prepare, or deliver outreach.

The core workflow is:

**Discover parcels → review evidence → save leads → record handwritten-note activity → track responses → remember follow-ups.**

## Current foundation

The repository has Union GIS fetching and normalization plus parcel and sales storage code. The executable currently streams likely-vacant parcel IDs; durable full-county ingestion, owner labels, tax ingestion, soil screening, scheduled jobs, and the user-facing workflow are not all implemented.

This document defines application-level scope and sequencing, not implementation authorization. Existing source plans remain implementation inputs, not evidence of shipped features.

## Parcel discovery and review

Provide a map and synchronized results list with parcel and owner search. Initial filters cover acreage, likely-vacant status, assessed land value, mailing state, and owner entity-name indicators where supported by the source.

Zoning/jurisdiction, road proximity, and current effective flood evidence are committed V1 features under [parcel-qualification-layers.md](parcel-qualification-layers.md). Add qualification filters for mapped zoning jurisdiction/code, road distance within the supported search radius, and mapped special flood hazard area overlap, with explicit stale/unknown states.

Parcel details show:

- Parcel identity, boundaries, acreage, and available land/building facts.
- Current source-reported owner and mailing information.
- Available sale history with source and date context.
- Parcel-linked tax evidence when available.
- Mapped zoning and jurisdiction, including split/conflicting assignments and a route to planning verification.
- Nearby mapped roads and parcel-to-centerline distance, without implying frontage or legal access.
- Current effective flood-zone overlap, affected acreage, coverage status, and panel/effective-date context; no flood-free certification.
- Source dates, retrieval dates, freshness, and missing or uncertain information.

Likely vacant is not verified vacant; assessed value is not market value; out-of-state mailing is not proof of residence; entity-name indicators are not verified legal ownership classifications. None establishes distress or willingness to sell. Do not introduce an opaque motivated-seller score.

## Required qualification evidence

Ship all three qualification groups in the interactive map, synchronized list, and parcel detail view. Ingestion alone is not completion. A buyer can inspect the contributing geometry, original classifications, dates, freshness, and missing/conflicting evidence before saving or excluding a lead. Qualification findings do not automatically reject parcels, change owner labels, advance lead states, or trigger contact.

Early development milestones may exercise personal lead tracking before these groups are ready, but the completed V1 release must include all three. This commitment does not make soil screening a prerequisite for the initial application release or relax the full soil-feature release contract. Utilities and the other previously considered specialist overlays remain outside this scope. The exclusions of mailing preparation/export, message generation/delivery, and campaign automation remain unchanged.

## Saved leads and personal scratchpad

Users can save parcels to named lists, record selection reasons, and exclude unsuitable parcels. Each saved lead has a scratchpad for the buyer's own notes, observations, handwritten-note dates, responses, and next steps.

Keep a dated activity history so the buyer can see what he recorded and when. Saving a parcel or adding a scratchpad entry never implies that a note was sent. The buyer explicitly records that action.

Saved lists, scratchpad entries, and activity history survive source refreshes and stay associated with stable parcel identities.

Primary actions are **Save to list**, **Add scratchpad note**, **Change status**, **Set follow-up**, and **Exclude**.

## Lead state machine

Use explicit states and user-driven transitions rather than inferring progress from source data or reminder delivery.

| State | Meaning | Normal next states |
| --- | --- | --- |
| Saved | Parcel is being considered | Ready, closed |
| Ready | Buyer has decided to contact the owner himself | Note sent, closed |
| Note sent | Buyer recorded sending a handwritten note | Replied, closed |
| Replied | Buyer recorded a response or ongoing conversation | Closed |
| Closed | Buyer is no longer pursuing the lead | Saved, through explicit reopening |

Record the date of each transition. Allow explicit corrections with history retained. Additional handwritten notes or conversations are activity entries, not new states. A closed lead may carry a reason such as not interested, unsuitable, or completed.

Follow-up due is a reminder condition, not a lead state: a lead can be awaiting a reply or already in conversation when a reminder becomes due.

A do-not-contact flag is independent of progress. It remains visible and blocks new follow-up reminders until explicitly cleared; reopening a lead does not clear it.

## Follow-up notifications

The buyer can set an individual follow-up reminder against a saved lead with a due date/time, timezone, and optional description of what he wants to remember. He can reschedule, snooze, cancel, or complete it.

Reminders appear in an in-app due/overdue queue and notify the buyer on his chosen schedule by SMS to his own phone number, email to his own address, or both. Each reminder uses his selected delivery channels and verified destinations; he can manage those destinations and opt out of either channel. These are personal reminders only. No notification is sent to a property owner, no message is drafted, and no handwritten note is treated as sent automatically.

Required behavior:

- Retries do not produce duplicate notifications for the same reminder occurrence.
- Failed SMS or email delivery does not hide the reminder from the due queue; delivery status is tracked separately for each selected channel.
- Completion or cancellation prevents subsequent delivery for that occurrence.
- Closing a lead or marking it do-not-contact cancels its pending follow-up reminders.
- A known ownership change pauses affected reminders and asks the buyer to review the lead. It does not erase history or automatically replace the person he was tracking.
- Reminder delivery or completion does not automatically change the lead's progress state.
- Upcoming reminders and notification preferences are visible and editable.

There are no campaign schedules, automatic sequences, or initial-outreach preparation workflows.

## Tax evidence and land suitability

Include narrowly scoped, parcel-linked delinquency evidence with tax year, reported balance, publication date when available, retrieval date, and a route to current verification. A published balance is not a current payoff quote; absence from an imported file does not prove that no tax is owed.

The existing tax plan's approximately twice-yearly refresh does not support a promise of timely foreclosure alerts. Resolve source publication cadence, freshness, and verification before offering those alerts.

Soil screening can help buyers reject unsuitable parcels, but is not a prerequisite for the first usable discovery and tracking release. If included, meet the existing soil plan's use-specific screening and evidence requirements. Do not substitute a soil map or a buildable badge. Soil does not establish access, zoning compliance, utilities, or septic approval.

## Data reliability and user trust

Use repeatable source imports, stable parcel identities, visible freshness, and operational failure reporting. Preserve personal notes, status history, and list membership independently of source refreshes.

Unknown or conflicting source information stays visible. Source updates are not necessarily newly occurring events; a GIS ownership update must not be described as a newly recorded deed.

Before user launch, provide authenticated access and isolate each user's lists, scratchpad entries, activity history, and reminders. Respect source data licenses and protect private user notes.

## Delivery sequence and existing plans

| Stage | Outcome | Existing inputs |
| --- | --- | --- |
| Durable parcel foundation | Stable parcels and geometry with repeatable refreshes | Parcel persistence in [soil-layer.md](soil-layer.md); [source-jobs-runner-scheduler.md](source-jobs-runner-scheduler.md) |
| Discovery and qualification | Map/list, parcel evidence, required zoning/jurisdiction, road proximity and current flood groups, filters, saved lists | [parcel-owner-labels.md](parcel-owner-labels.md); [parcel-qualification-layers.md](parcel-qualification-layers.md); new application work |
| Personal lead tracking | Scratchpad, explicit state transitions, activity history, follow-up queue, and scheduled personal SMS/email reminders | New application workflow work |
| Evidence enrichment | Narrow tax evidence; complete soil screening when ready | [tax-data-ingest.md](tax-data-ingest.md); [soil-layer.md](soil-layer.md) |

User access and isolation are launch prerequisites. This sequence does not change the soil release contract or tax refresh policy; implementation-plan changes require explicit reconciliation.

## Excluded product scope

The application does not provide AI-written outreach, letter generation, mailing-list export/preparation, mailing fulfillment, campaign automation, automatic initial reach-outs, owner-directed email/SMS/calls, or a full CRM. Handwritten notes remain the buyer's external, manual activity. These are exclusions, not planned later outreach features.

## Deferred data features

- Commercial deed and mortgage monitoring; consider only if ownership reconciliation proves valuable, not as the primary lead-discovery feature.
- Probate, estate, or executor alerts until suitable source access and reliable parcel linkage are verified. The older [docket monitoring todo](../todo/docket_monitor_notifications.md) is not a V1 commitment.
- Unspecified scraping. [scraper-enrichment.md](scraper-enrichment.md) still requires a confirmed source and field contract.
- Purchased contact enrichment, user-defined source jobs, and nationwide coverage.
- Saved-search change notifications until refresh behavior and newly-matching semantics are established.

Source investigations remain in [the parcel actions diary](../.agents/diary/parcel-actions.md).

## Pilot completion checks

- The buyer can discover Union County parcels, inspect evidence, and save leads.
- All three committed qualification groups work in the map, list, and detail view, with source dates, coverage, uncertainty, and evidence-based filters. Zoning is not permitted-use approval; road proximity is not legal access; no mapped flood overlap is not a flood-free guarantee.
- Qualification source failures/refreshes preserve saved work and remain visible; current flood retrieval and coverage are verified rather than silently replaced by historical county mapping.
- He can add scratchpad notes, explicitly record a handwritten note as sent, record responses, and inspect dated activity history.
- Lead states follow the defined transitions, with explicit corrections and reopening recorded in history.
- He can schedule, snooze, reschedule, cancel, and complete follow-up reminders at the correct local time.
- Due reminders appear in-app and notify him through his selected SMS and/or email channels at the scheduled time. Verified destinations, channel opt-outs, independent delivery status, retries without duplicate delivery, and cancellation behavior are verified.
- Closed or do-not-contact leads have no pending follow-up reminders; known owner changes require review before paused reminders resume.
- No reminder sends an owner-facing message or automatically records a handwritten note as sent.
- Source freshness and uncertainty are visible; one user's private workflow data is inaccessible to another user.
- Pilot feedback measures time to find suitable leads, source errors, usefulness of the scratchpad and states, missed follow-ups, and repeat use. Set commercial success targets with the buyer rather than deriving them from ingestion volume.

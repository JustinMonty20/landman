# Independent Union schedules

- Out-of-scope decision. Keep soil-4's historical polygon and overlap rows. New overlap writes are restricted to current immutable set members, and screening still uses fresh intersections rather than historical joins. Deletion and a current legacy-overlap read API are outside this change.
- Skipped refactor. Keep the existing generic scheduler calendar behavior, including library-level interval support. The executable accepts only explicit calendar expressions for its two source schedules.
- Out-of-scope decision. Do not add a persistent dispatch queue or crash-time backfill. Commands and scheduler logs expose failed downstream work; manual screening retry and the next successful source run are the recovery paths.
- Scope tangent. The generic scheduler still passes caller-supplied schedules directly to robfig cron, whose timezone parser can panic on missing literal-space separators. This change normalizes and validates both executable schedules before database access, so the Union command does not expose that existing library-level input bug. Broader scheduler input hardening is deferred.

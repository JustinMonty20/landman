[Out-of-scope decision] Keep the whole-scan parcel transaction. Parcel parts can occur on different pages, so page commits could publish incomplete geometry. Persistent staging and restart support require a separate design; this fix removes the repeated parcel lookup without changing publication behavior.

[Out-of-scope decision] Do not expand the verification helper's permitted container names. Another verification task owns its reserved port and container. This task uses a separate container, direct SQL checks, and comparisons of stored parcel and sale values.

[Scope tangent worth pursuing later] The retry used 41 seconds for 59 statistics updates. A different interval could reduce that cost, but it needs separate cold-import and failed-import retry measurements. Keep the 2,000-part interval for this fix.

[Scope tangent worth pursuing later] PR #47 changed the CLI arguments after the verification recipe was written. The recipe still says that extra arguments are ignored. This fix changes the timeout verdict, not the full CLI recipe.

[Out-of-scope decision] Keep the whole-scan parcel transaction. Parcel parts can occur on different pages, so page commits could publish incomplete geometry. Persistent staging and restart support require a separate design; this fix removes the repeated parcel lookup without changing publication behavior.

[Out-of-scope decision] Do not expand the verification helper's permitted container names. Another verification task owns its reserved port and container. This task uses a separate container, direct SQL checks, and comparisons of stored parcel and sale values.

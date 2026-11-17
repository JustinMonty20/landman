# Union tax job

[Out-of-scope decision] Keep the four-file job under `internal/connector/union/taxjob/`, not in the parent Union package. `taxstore` imports Union row types, so a parent-package import of `taxstore` would create a Go import cycle. Do not refactor the tax-store row contracts for this job.

# Union tax job

[Out-of-scope decision] Keep the four-file job under `internal/connector/union/taxjob/`, not in the parent Union package. `taxstore` imports Union row types, so a parent-package import of `taxstore` would create a Go import cycle. Do not refactor the tax-store row contracts for this job.

[Scope tangent] `SaveForeclosure` upserts a numbered case and adds its current PIN links, but it does not delete old links if that case later changes PINs. The tax-6 retry check now detects missing current links; removal of old links needs a separate case-reconciliation change and tests.

[Out-of-scope decision] The operator accepted the early disabled-job guard, race test with invalid database and missing files, and control-flow inspection as proof that the disabled job makes no county file request. Do not add request or file-open counters solely for this path; they would add test-only dependency wiring to code that returns before dependencies exist.

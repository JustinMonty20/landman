[Out-of-scope decision] The operator chose one repository PR. Add a project check for agent profiles and child commands; do not change Pi launch hooks in this PR. The repository check cannot block every child launch.

[Out-of-scope decision] Add Go CI jobs, but leave branch-protection settings unchanged. The operator must choose when to require the new status checks on `main`; code in this PR cannot set that policy.

[Scope tangent] A full PostgreSQL-tagged suite spent more than 350 seconds in soil measurement tests that wait on the one-request-per-second USDA limiter. Keep this CI change on the root, parcelstore, and taxstore database packages; decide how to run the soil tagged suite in separate work. The stopped run is not a pass and does not prove a soil defect.

# V1 plan priorities

- **Unrelated bug.** The merged parcel API and county qualification PRs both add Goose migrations numbered 20 and 21. A Goose status command with a dummy database address reproduced a duplicate-version-20 panic during migration collection, before database access. SQL-file `validate` alone passed and did not detect this collision. Record migration reconciliation, a duplicate-version check, and fresh-database verification as the first correctness task. Do not rename or rewrite possibly applied migrations during this planning task.

- **Out-of-scope decision.** The operator confirmed that migration repair is already handled elsewhere. Exclude its repair instructions from the plan index. Keep only plan scopes and priorities, with real-data browser integration first in the unassigned feature queue.

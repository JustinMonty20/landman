# Migration version conflict

- [Out-of-scope decision] Do not rewrite Goose history for development databases that used qualification versions 20 and 21. This fix preserves the applied API sequence and supports fresh databases and API-21 upgrades. Other histories require a separate backup and schema review.

# Thread retention

The Engine keeps thread history indefinitely by default. Each Engine serves one
company. An administrator can set **Thread retention** under
**Settings → Engine**. `0` keeps threads indefinitely; values from 1 to 36500
specify the number of days to retain terminal threads. The setting is stored
with the company and takes effect without restarting the Engine. In a split
deployment, the writer process performs cleanup.

The writer checks at startup and every minute. Each pass deletes at most 1,000
threads in batches of 100. It selects only completed, cancelled, closed, or
failed threads whose terminal state was archived before the configured period.
For threads archived before this field existed, the Engine uses `updated_at`.
Active threads remain. Deleting a thread also
removes its archived activities, access grants, references, step states,
validation results, notifications, substeps, and Valkey keys. A compact
tombstone prevents delayed archival events from restoring deleted history.

Turning retention off stops future deletions; it does not restore threads that
were already removed. PostgreSQL may reuse freed pages before the database file
shrinks, so `pg_database_size` may not fall immediately after a sweep.

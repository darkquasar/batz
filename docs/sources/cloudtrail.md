# Local CloudTrail evidence and identity

The `local_raw` route accepts uncompressed native CloudTrail JSON with a `Records`
array. A flat directory selects regular `.json` files. The current compact mapping
covers S3 GetObject by an IAMUser; other events remain queryable in staging with
unsupported coverage. Native JSONL from other sources needs its own pipeline.

Collection fingerprints each selected path and its exact SHA-256 contents.
Changed versions are additional evidence. Replay creates fresh acquired records;
retry uses the already committed evidence. See [runs](../usage/runs.md).

```text
+------------------+  every observation  +------------------+  variants  +------------------+
| Source snapshots | ==================> | Staging identity | =========> | Normalized query |
+------------------+                     +------------------+            +------------------+
```

Staging adds the following nested keys:

| Key | CloudTrail policy |
| --- | --- |
| `batz.source_event_id` | Batz's acquired-record identifier, unique across batches and record positions. It is separate from the vendor's `event.id`. |
| `batz.logical_event_id` | SHA-256 of the JSON tuple `[binding_id, "aws-cloudtrail", recipientAccountId, eventID]`, when the account and event ID are nonempty strings. |
| `batz.norm_event_variant_id` | SHA-256 of the JSON tuple `[logical_event_id, canonical_payload]`; without logical identity, the acquired-record ID replaces the logical ID. |

Missing, blank or malformed event/account identity leaves logical identity null.
Records still normalize when their supported semantic fields are valid. Their
original fields remain in the complete source payload. Missing account scope
never falls back to the actor's account or `sharedEventID`.

Identical payloads under one logical identity produce one normalized variant with
all origins. Distinct payloads remain separate rows with `batz.conflict=true`.
Another account or binding stays separate. Without logical identity, each
observation remains separate. A merged row's top-level `batz.source_event_id` is
null; all acquired-record IDs remain in `batz.origins[].source_event_id`.

```sql
SELECT batz.source_event_id, batz.logical_event_id, batz.norm_event_variant_id
FROM demo.source_staging;

SELECT event.id, batz.logical_event_id, batz.norm_event_variant_id,
       batz.conflict, batz.origins, batz.source_payload
FROM demo.api_activity;
```

CloudTrail's [record definition](https://docs.aws.amazon.com/awscloudtrail/latest/userguide/cloudtrail-event-reference-record-contents.html)
documents `eventID`, recipient-account scope and addenda. `UPDATED_DATA` names an
original event and updated fields. This MVP preserves addenda in full; automatic
patching/supersession is not inferred from arrival order. Source-authoritative
correction application remains dependent on a separately accepted rule. An
addendum with a new event ID currently retains that identity; it does not silently
overwrite an earlier event.

The ordinary run report counts input acquisition and event reconciliation
separately. All published revisions retain exact committed membership, original
timestamp text/offset, supplied precision and missing occurrence time.

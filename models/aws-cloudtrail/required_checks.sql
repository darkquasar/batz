MODEL (name cloudtrail.required_checks, kind FULL);

-- Pipeline-owned semantics. These checks are mandatory production evidence;
-- independent SQLMesh fixtures separately check the mapping and this oracle.
SELECT
 (SELECT count(*) FROM cloudtrail.api_activity a
  JOIN cloudtrail.source_staging s ON a.batz.origins[1].source_event_id=s.observation_id
  WHERE a.event.action IS DISTINCT FROM 'object-read'
     OR a.event.kind IS DISTINCT FROM 'event'
     OR a.ecs.version IS DISTINCT FROM '9.5.0'
     OR a.user.id IS DISTINCT FROM json_extract_string(s.payload,'$.userIdentity.arn')
     OR a."@timestamp" IS DISTINCT FROM s.occurrence_time
     OR a.batz.time.original IS DISTINCT FROM s.original_time
     OR a.batz.time.fraction_digits IS DISTINCT FROM s.fraction_digits
     OR a.event.outcome IS DISTINCT FROM CASE WHEN json_exists(s.payload,'$.errorCode') THEN 'failure' ELSE 'success' END
 )::BIGINT AS assertion_failures,
 (SELECT count(*) FROM cloudtrail.source_staging WHERE mapping_status='unavailable')::BIGINT AS capability_failures;

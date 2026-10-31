MODEL (name probe.required_checks, kind FULL);
SELECT (SELECT count(*) FROM probe.findings WHERE event.action IS DISTINCT FROM 'observed')::BIGINT AS assertion_failures,
 0::BIGINT AS capability_failures;

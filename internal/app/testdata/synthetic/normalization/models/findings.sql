MODEL (name probe.findings, kind FULL);
SELECT
 cast(json_extract_string(s.payload,'$.observed_at') AS TIMESTAMP_NS) AS "@timestamp",
 {'version':'9.5.0'} AS ecs,
 {'kind':'event','action':'observed','id':json_extract_string(s.payload,'$.id')} AS event,
 {'id':json_extract_string(s.payload,'$.subject')} AS "user",
 {'origins':[{'observation_id':s.observation_id,'parsed_revision':s.parsed_revision,'file':s.parsed_file,'row':s.parsed_row}],
  'source_payload':s.payload,'input_revision':c.input_revision,'input_digest':c.input_digest,
  'attempt_id':c.attempt_id,'evaluation_nonce':c.evaluation_nonce} AS batz
FROM probe.records s CROSS JOIN batz_input.build_context c;

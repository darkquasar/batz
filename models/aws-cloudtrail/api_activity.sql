MODEL (name cloudtrail.api_activity, kind FULL);

WITH variants AS (
 SELECT batz.norm_event_variant_id AS variant_id,
  list({'source_event_id':observation_id,'parsed_revision':parsed_revision,
        'file':parsed_file,'row':parsed_row} ORDER BY observation_id) AS origins,
  count(*) AS observation_count
 FROM cloudtrail.source_staging
 WHERE mapping_status='mapped'
 GROUP BY batz.norm_event_variant_id
), ranked AS (
 SELECT *,
  row_number() OVER (PARTITION BY batz.norm_event_variant_id ORDER BY observation_id) AS variant_row,
  count(DISTINCT batz.norm_event_variant_id) OVER
   (PARTITION BY coalesce(batz.logical_event_id,observation_id))>1 AS conflict
 FROM cloudtrail.source_staging
 WHERE mapping_status='mapped'
)
SELECT
 s.occurrence_time AS "@timestamp",
 {'version':'9.5.0'} AS ecs,
 {'kind':'event','category':['web'],'type':['access'],'action':'object-read',
  'outcome':CASE WHEN json_exists(s.payload,'$.errorCode') THEN 'failure' ELSE 'success' END,
  'provider':'s3.amazonaws.com','id':CASE WHEN json_type(s.payload,'$.eventID')='VARCHAR' THEN json_extract_string(s.payload,'$.eventID') ELSE NULL::VARCHAR END} AS event,
 {'provider':'aws','region':json_extract_string(s.payload,'$.awsRegion'),
  'account':{'id':CASE WHEN json_type(s.payload,'$.recipientAccountId')='VARCHAR' THEN json_extract_string(s.payload,'$.recipientAccountId') ELSE NULL::VARCHAR END}} AS cloud,
 {'id':json_extract_string(s.payload,'$.userIdentity.arn')} AS "user",
 {'ip':CASE WHEN regexp_full_match(s.caller,'([0-9]{1,3}[.]){3}[0-9]{1,3}')
     AND list_bool_and(list_transform(string_split(s.caller,'.'),x -> try_cast(x AS INTEGER) BETWEEN 0 AND 255))
    THEN s.caller ELSE NULL END} AS source,
 {'source_event_id':CASE WHEN v.observation_count=1 THEN s.batz.source_event_id ELSE NULL::VARCHAR END,
  'logical_event_id':s.batz.logical_event_id,
  'norm_event_variant_id':s.batz.norm_event_variant_id,
  'conflict':s.conflict,
  'actor_account_id':json_extract_string(s.payload,'$.userIdentity.accountId'),
  'target':{'bucket':json_extract_string(s.payload,'$.requestParameters.bucketName'),'key':json_extract_string(s.payload,'$.requestParameters.key')},
  'time':{'original':s.original_time,'fraction_digits':s.fraction_digits,'offset':s.original_offset,'uncertainty':'unspecified'},
  'profile':'batz-investigation/v0alpha1',
  'origins':v.origins,
  'source_payload':s.payload,
  'input_revision':c.input_revision,'input_digest':c.input_digest,'attempt_id':c.attempt_id,
  'evaluation_nonce':c.evaluation_nonce} AS batz
FROM ranked s
JOIN variants v ON v.variant_id=s.batz.norm_event_variant_id
CROSS JOIN batz_input.build_context c
WHERE s.variant_row=1;

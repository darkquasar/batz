MODEL (name cloudtrail.api_activity, kind FULL);

SELECT
 s.occurrence_time AS "@timestamp",
 {'version':'9.5.0'} AS ecs,
 {'kind':'event','category':['web'],'type':['access'],'action':'object-read',
  'outcome':CASE WHEN json_exists(s.payload,'$.errorCode') THEN 'failure' ELSE 'success' END,
  'provider':'s3.amazonaws.com','id':json_extract_string(s.payload,'$.eventID')} AS event,
 {'provider':'aws','region':json_extract_string(s.payload,'$.awsRegion'),
  'account':{'id':json_extract_string(s.payload,'$.recipientAccountId')}} AS cloud,
 {'id':json_extract_string(s.payload,'$.userIdentity.arn')} AS "user",
 {'ip':CASE WHEN regexp_full_match(s.caller,'([0-9]{1,3}[.]){3}[0-9]{1,3}')
     AND list_bool_and(list_transform(string_split(s.caller,'.'),x -> try_cast(x AS INTEGER) BETWEEN 0 AND 255))
    THEN s.caller ELSE NULL END} AS source,
 {'actor_account_id':json_extract_string(s.payload,'$.userIdentity.accountId'),
  'target':{'bucket':json_extract_string(s.payload,'$.requestParameters.bucketName'),'key':json_extract_string(s.payload,'$.requestParameters.key')},
  'time':{'original':s.original_time,'fraction_digits':s.fraction_digits,'offset':s.original_offset,'uncertainty':'unspecified'},
  'profile':'batz-investigation/v0alpha1',
  'origins':[{'observation_id':s.observation_id,'parsed_revision':s.parsed_revision,'file':s.parsed_file,'row':s.parsed_row}],
  'source_payload':s.payload,
  'input_revision':c.input_revision,'input_digest':c.input_digest,'attempt_id':c.attempt_id,
  'evaluation_nonce':c.evaluation_nonce} AS batz
FROM cloudtrail.source_staging s
CROSS JOIN batz_input.build_context c
WHERE s.mapping_status='mapped';

MODEL (name cloudtrail.source_staging, kind FULL);

WITH source AS (
 SELECT *,
   json_extract_string(payload, '$.eventTime') AS original_time,
   json_extract_string(payload, '$.sourceIPAddress') AS caller,
   json_extract_string(payload, '$.eventSource') = 's3.amazonaws.com'
     AND json_extract_string(payload, '$.eventName') = 'GetObject'
     AND json_extract_string(payload, '$.eventType') = 'AwsApiCall'
     AND json_extract_string(payload, '$.userIdentity.type') = 'IAMUser' AS supported
 FROM batz_input.parsed
), times AS (
 SELECT *,
   regexp_extract(original_time, '(Z|[+-][0-9]{2}:[0-9]{2})$', 1) AS original_offset,
   length(regexp_extract(original_time, '[.]([0-9]+)', 1))::INTEGER AS fraction_digits,
   try_cast(regexp_replace(original_time, '(Z|[+-][0-9]{2}:[0-9]{2})$', '') AS TIMESTAMP_NS) AS local_instant
 FROM source
), classified AS (
 SELECT *,
   CASE
     WHEN NOT coalesce(supported, false) THEN 'unsupported'
     WHEN original_time IS NOT NULL
       AND regexp_full_match(original_time, '[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})')
       AND try_cast(regexp_replace(original_time, '(Z|[+-][0-9]{2}:[0-9]{2})$', '') AS TIMESTAMP) IS NOT NULL
       AND (original_offset='Z' OR (try_cast(substr(original_offset,2,2) AS INTEGER)<=23 AND try_cast(substr(original_offset,5,2) AS INTEGER)<=59))
       AND (fraction_digits>9 OR local_instant IS NULL
         OR abs(epoch_ns(local_instant)::HUGEINT - CASE WHEN original_offset='Z' THEN 0 ELSE
           (CASE WHEN substr(original_offset,1,1)='-' THEN -1 ELSE 1 END) *
           (cast(substr(original_offset,2,2) AS HUGEINT)*60+cast(substr(original_offset,5,2) AS HUGEINT))*60000000000 END)>=9223372036854775807)
       THEN 'unavailable'
     WHEN original_time IS NOT NULL AND (
       NOT regexp_full_match(original_time, '[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2})')
       OR local_instant IS NULL
       OR (original_offset != 'Z' AND (try_cast(substr(original_offset,2,2) AS INTEGER)>23 OR try_cast(substr(original_offset,5,2) AS INTEGER)>59))
     ) THEN 'rejected'
     WHEN coalesce(json_type(payload,'$.eventID'),'NULL') NOT IN ('VARCHAR','NULL')
       OR coalesce(json_type(payload,'$.awsRegion'),'NULL') NOT IN ('VARCHAR','NULL')
       OR coalesce(json_type(payload,'$.userIdentity.accountId'),'NULL') NOT IN ('VARCHAR','NULL')
       OR coalesce(json_type(payload,'$.sourceIPAddress'),'NULL') NOT IN ('VARCHAR','NULL')
       OR coalesce(json_type(payload,'$.errorCode'),'NULL') NOT IN ('VARCHAR','NULL')
       OR coalesce(json_type(payload,'$.userIdentity.arn'),'NULL') NOT IN ('VARCHAR','NULL')
       OR coalesce(json_type(payload,'$.recipientAccountId'),'NULL') NOT IN ('VARCHAR','NULL')
       OR coalesce(json_type(payload,'$.requestParameters.bucketName'),'NULL') NOT IN ('VARCHAR','NULL')
       OR coalesce(json_type(payload,'$.requestParameters.key'),'NULL') NOT IN ('VARCHAR','NULL')
     THEN 'rejected'
     ELSE 'mapped'
   END AS mapping_status
 FROM times
)
SELECT *,
 CASE WHEN mapping_status='mapped' AND original_time IS NOT NULL THEN
  make_timestamp_ns(epoch_ns(local_instant) -
   CASE WHEN original_offset='Z' THEN 0 ELSE
    (CASE WHEN substr(original_offset,1,1)='-' THEN -1 ELSE 1 END) *
    (cast(substr(original_offset,2,2) AS BIGINT)*60+cast(substr(original_offset,5,2) AS BIGINT))*60000000000
   END)
 ELSE NULL::TIMESTAMP_NS END AS occurrence_time,
 CASE WHEN mapping_status='rejected' THEN 'invalid-supported-field' ELSE NULL END AS mapping_reject_reason
FROM classified;

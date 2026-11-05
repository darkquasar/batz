MODEL (name probe.records, kind FULL);
SELECT *, {'source_event_id':observation_id,'logical_event_id':NULL::VARCHAR,
 'norm_event_variant_id':sha256(to_json([observation_id,payload]))} AS batz, coalesce(json_extract_string(payload,'$.disposition'),'mapped') AS mapping_status,
 CASE WHEN json_extract_string(payload,'$.disposition')='rejected' THEN 'invented-reject' ELSE NULL::VARCHAR END AS mapping_reject_reason
FROM batz_input.parsed;

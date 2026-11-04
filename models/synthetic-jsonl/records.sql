MODEL (name probe.records, kind FULL);
SELECT *, {'source_event_id':observation_id,'logical_event_id':NULL::VARCHAR,
 'norm_event_variant_id':sha256(to_json([observation_id,payload]))} AS batz, 'mapped' AS mapping_status, NULL::VARCHAR AS mapping_reject_reason
FROM batz_input.parsed;

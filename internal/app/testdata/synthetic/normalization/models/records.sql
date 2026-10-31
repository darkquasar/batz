MODEL (name probe.records, kind FULL);
SELECT *, 'mapped' AS mapping_status, NULL::VARCHAR AS mapping_reject_reason
FROM batz_input.parsed;

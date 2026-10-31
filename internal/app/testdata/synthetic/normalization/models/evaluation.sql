MODEL (name probe.evaluation, kind FULL);

SELECT c.input_revision, c.input_digest, c.attempt_id, c.evaluation_nonce,
 count(s.observation_id)::BIGINT AS observations,
 count(s.observation_id) FILTER (WHERE s.mapping_status='mapped')::BIGINT AS mapped,
 count(s.observation_id) FILTER (WHERE s.mapping_status='unsupported')::BIGINT AS unsupported,
 count(s.observation_id) FILTER (WHERE s.mapping_status='rejected')::BIGINT AS rejected
FROM batz_input.build_context c
LEFT JOIN probe.records s ON TRUE
GROUP BY c.input_revision,c.input_digest,c.attempt_id,c.evaluation_nonce;

# Synthetic local CloudTrail fixture

api-read.json is adapted from the synthetic native Records example in the
pipeline specification. It is invented test data, contains no incident evidence
or credentials, and makes no claim to cover general CloudTrail behavior.
The unmapped_example field exercises preservation of an unknown source field.
The timestamp has second precision; future nano probes must be labelled synthetic
stack probes separately from this representative input.

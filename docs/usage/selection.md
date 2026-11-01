# Select a pipeline and its nonsecret context

Run a definition directly, select a saved binding, or explicitly select one
binding in a case. A workspace is always explicit or comes from that document.
An omitted route selects the sole declared route; multiple routes need an
explicit selection for execution.

```sh
runtime/.local/batz run --binding /path/binding.yaml --project . --workspace /tmp/case
runtime/.local/batz run --case /path/case.yaml --binding-id cloudtrail --project .
runtime/.local/batz explain pipelines/aws-cloudtrail/pipeline.yaml --binding /path/binding.yaml --format json
```

A saved binding is a versioned closed document:

```yaml
apiVersion: batz/v0alpha1
kind: PipelineBinding
id: cloudtrail
workspace: /tmp/case
definition: repo://pipelines/aws-cloudtrail/pipeline.yaml
version: 1
route: local_raw
input:
  path: repo://pipelines/aws-cloudtrail/fixtures/api-read.json
```

A Case has apiVersion, kind: Case, id, optional workspace and a pipelines list
of binding entries (without their envelope fields). --binding and --case are
mutually exclusive. A supplied positional definition must match the binding.
No nearby case or ancestor workspace is discovered. Omitted standalone IDs are
generated; a supplied stable ID addresses that binding's state. Multi-binding case
scheduling remains S8.

Resolution order, lowest to highest: route defaults, project/workspace.yaml, exact
selected workspace/workspace.yaml, selected document shared sections, binding-local
values, CLI overrides. Only storage, normalization, credentials and scoped
pipeline_defaults.<source>.<route> are shared. Maps merge; lists/scalars replace;
null deletes; a blank YAML value fails. Credential provider descriptors replace
atomically. These descriptors are static context; explanation never looks up a
secret. Provider execution and endpoint authorization remain S5.

CLI paths are caller-relative. YAML paths are relative to their original declaring
file; repo:// uses the selected project and workspace:// uses the selected
workspace. Without a workspace, workspace:// remains symbolic. Tilde expansion is
limited to declared path fields. Reports retain each winning nonsecret value and
its original source. Lists use JSON syntax on the CLI, for example
--input 'prefixes=["a/","b/"]'; timestamps retain their supplied RFC3339 text.
An unquoted YAML timestamp also retains its original fraction and offset.

The first executable native adapter remains the local path-only CloudTrail tracer.
Selections requiring unqualified storage/providers/native parameter forms fail
explicitly rather than silently ignoring those choices. --interactive opts into
prompts for missing workspace/required inputs only; supplied values are preserved.

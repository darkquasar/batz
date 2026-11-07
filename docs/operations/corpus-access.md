# Corpus credential access

Explicit corpus preparation uses the Batz vault in 1Password and a dedicated
macOS Keychain entry. Routine native/model tests still use prepared local bytes
without credentials or network access. This setup supplies credentials; it does
not implement a remote production route or sample acquisition.

```text
+----------------------+  token =>  +-----------------------+
| macOS login Keychain | =========> | op [Batz vault only]  |
+----------------------+            +-----------------------+
                                              | R2 credentials =>
                                              v
                                    +-----------------------+
                                    | Explicit command      |
                                    +-----------------------+
```

The generic-password service is `batz-corpus-op-service-account`. Its value is the
existing Batz service-account token, not the Cloudflare access key. Provision or
rotate that entry using normal 1Password desktop authentication and macOS
Keychain access. Keep its vault access restricted to Batz. This is independent
of the `specs` repository's archive account and bucket.

The root `corpus.op.env` is ignored by Git. It contains nonsecret bucket/endpoint
metadata and `op://Batz/...` references for access keys. Copy
[corpus.op.env.template](../../corpus.op.env.template), which contains only the
expected variable names with empty values, to create the local file. A bucket name can also be a
1Password reference when the item has that field. The endpoint is the HTTPS S3
service origin, without a bucket suffix. Resolved access keys and the
service-account token never belong in this file.

With Python 3 and the 1Password CLI installed, verify scope and resolve references
without displaying their values:

```sh
python3 -I ci/corpus-credentials.py check
```

Run an explicitly selected preparation command with:

```text
python3 -I ci/corpus-credentials.py -- COMMAND ARG...
```

The command receives `BATZ_CORPUS_R2_BUCKET`, `BATZ_CORPUS_R2_ENDPOINT`,
`BATZ_CORPUS_R2_ACCESS_KEY_ID` and `BATZ_CORPUS_R2_SECRET_ACCESS_KEY`. Consumers must
use those variables explicitly; these are not another project's archive settings
or an implicit AWS profile. The launcher removes the service-account token before
starting the consumer. Normal `op run` output masking remains enabled, but
consumers must still avoid logging credentials or dumping their environment.

On macOS the launcher reads the named Keychain entry. CI can instead supply
`OP_SERVICE_ACCOUNT_TOKEN` through its secret store. Either token must expose
exactly the Batz vault: broader or unrelated accounts are refused. Missing tokens
fail without falling back to unrestricted desktop access. Connect credentials
are removed from the `op` environment so they cannot override service-account
authentication. Keychain and vault-scope lookup diagnostics are suppressed rather
than risking secret values in errors. Command execution retains normal `op run`
output masking; failures remain visible through a nonzero exit.

Set `BATZ_CORPUS_OP_ENV` to an explicit reference-file path when using a separate
checkout. The child command's exit status is preserved. `check` proves vault
scope and reference resolution; it does not itself contact R2, download logs,
verify source hashes or qualify normalization. The initial local setup separately
checks an authenticated bucket HEAD without listing or printing log contents.

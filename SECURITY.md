# Security policy

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability or credential exposure.
Report privately to `yisshiki39@gmail.com` with the affected component, impact,
reproduction steps, and a safe contact method. Do not include real user data or
working production credentials.

We aim to acknowledge security reports within 2 business days, triage critical
issues within 24 hours, and provide a remediation/mitigation plan after validation.

## Supported code

`development` is the active pre-release branch. Production deployments must use a
reviewed commit promoted from the current development line. Old test builds and
unmaintained branches are not supported.

## Credential handling

Server secrets, signing keys, database URLs, service-account JSON, and tokens must
remain in `/Users/yota/Projects/Secrets/Ohey` or protected provider/CI stores. If a
credential appears in source or logs, revoke it first, then investigate and rotate
dependent systems. Deleting it from the latest commit alone is not sufficient.

# Security policy — pre-release draft

No supported public versions yet. The security model and release gates are specified in `docs/07-SECURITY_PRIVACY.md`. The product is intended to handle sensitive conversations and project data; assume any real-world capture/export can contain sensitive information and exercise caution. Testing should use local synthetic fixtures with explicit permission.

## Reporting a vulnerability

Before public release, maintainers must configure a private GitHub security advisory channel or dedicated private security contact, test the reporting workflow, and publish supported versions and response expectations. Contact and response SLA are not yet established. Until configured, notify the project owner privately; do not disclose secrets, customer data, usable access credentials or exploit details in public issues. Provide affected version/commit, OS, scope, minimal safe reproduction, severity/impact and mitigation if known. Never test against other users or repositories without authorization.

## Security release checklist

- Authenticated loopback API, cross-project isolation, explicit capture and external-data consent, redaction/exclusion tests and documented limitations.
- No credentials in logs/test artifacts, no automatic escalations in reviewers, no mandatory upload or telemetry.
- Dependency/security/license inventory, signed release checks, upgrade rollback, verified archive/path safety, encrypted restore tests for sync module.
- High/critical unresolved flaws block affected feature release; publish limitations for disabled or unsupported features. Follow coordinated disclosure and do not make unverifiable safety claims.

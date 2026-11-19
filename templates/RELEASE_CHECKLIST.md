# Release qualification — [version]

- [ ] Owner approved release scope, product name, LICENSE, attribution and NOTICE.
- [ ] Build artifacts contain only intended public files, no `_private/`, transcripts, secrets or local paths.
- [ ] Supported agent/OS/version matrix derived from actual tests.
- [ ] Relevant acceptance matrix passes, with evidence and limitations for disabled features.
- [ ] Actual resource/recall measurements and regression report published honestly.
- [ ] No unresolved high/critical vulnerabilities in enabled features; dependency scans, SBOM and fuzz/security tests complete.
- [ ] Signed/checksummed builds, install/update/rollback/uninstall and recoverability validated.
- [ ] Consent and privacy controls, export/delete semantics and offline degradation documented.
- [ ] Cloud feature only enabled after restore/encryption/security gates, with provider limitations disclosed.
- [ ] Quickstart, troubleshooting, release notes, contributor policy, security contact and support windows ready.
- [ ] Release CI protected, actual artifacts smoke tested, no credentials in untrusted PR CI.
- [ ] Explicit approval before GitHub publish, marketplace/package registry submission or external distribution.

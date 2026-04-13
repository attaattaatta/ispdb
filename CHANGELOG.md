# Changelog

## 0.1.3-beta

- fixed local MySQL fallback for servers that reject `START TRANSACTION READ ONLY WITH CONSISTENT SNAPSHOT` by adding a compatible consistent-snapshot fallback sequence
- fixed local MySQL backup export so empty strings are written as empty strings instead of `NULL`
- fixed `users` export scope to include `db users`
- fixed CSV and JSON exports to omit synthetic `Total` rows
- added regression tests for command defaults, SQL backup literals, and export behavior

## 0.1.2-beta

- replaced hardcoded IP defaults with current local server IPv4 for local command generation and kept destination IP rewrite for `--dest`
- replaced default DNS nameservers in generated commands with `ns1.example.com. ns2.example.com.`
- added read-only source access for SQLite and SQL dump reading, plus read-only consistent snapshot loading for local MySQL
- added first-run local database backup with reuse markers in `/root/.ispdb` and backup files in `/root/support/<date>/`
- improved startup output to show `DB backup:` before source details
- added GitHub Actions and GitLab CI build pipelines for Ubuntu, Alpine, and UBI9

# Changelog

## 0.3.2-beta

- renamed the `FTP users` section to `ftp users`
- fixed duplicated `db users` output for `--list all` while keeping `db users` available in both `--list users` and `--list databases`
- regrouped help examples into documented sections for quick start, export, remote migration, logging, and bulk operations
- clarified changelog coverage for the recently added package sync logic, Docker retry flow, and current SSL/self-signed handling during remote command rewriting

## 0.3.1-beta

- renamed command export header to `commands to run at remote server:` and commented all console command group titles for safe copy/paste
- renamed generated web command group to `web sites`
- stopped generating empty package groups and skipped console package commands for groups absent in the source package list
- removed invalid `limit_php_cgi_version=native` from generated `user.edit` commands
- enabled Docker package command generation when the destination panel edition is still unknown
- added destination retries for `user.edit` invalid CGI version errors and Docker-backed `db.server.edit` retries after automatic Docker install
- treated known `db.server.edit` invalid `version` responses as already-existing servers on destination side
- added grouped package synchronization commands for `web`, `email`, `dns`, `ftp`, `mysql`, `altphp`, and `others`
- kept remote site command rewriting on `--dest` pinned to `selfsigned` certificates unless a later certificate-specific flow overrides it

## 0.3.0-beta

- added `-v, --version`
- extended `--dest` CLI with `--overwrite`, `--no-delete-packages`, `--copy-configs`, and `--no-change-ip-addresses`
- updated help examples to use `/path/to/mysqldump/ispmgr.sql`
- documented and preserved `--clean` behavior for single-column exports

## 0.2.2-beta

- fixed `--clean` console mode to suppress banner and source metadata (`DB backup/DB/DB format/privkey`) so only requested database rows are printed

## 0.2.1-beta

- renamed SSH destination port flag to `-p, --port` and updated help/examples

## 0.2.0-beta

- added `--clean` mode for single-column text output without borders, titles, and totals
- added `--port` option for SSH destination connections
- added `--commands` alias for `--list commands`
- added automatic help output after parse errors when no inline hint is already present

## 0.1.7-beta

- fixed MySQL backup serialization for `time.Time` values so dump imports no longer fail on timezone-suffixed datetime literals

## 0.1.6-beta

- fixed MySQL backup importability by wrapping the generated SQL dump with `SET FOREIGN_KEY_CHECKS=0/1`

## 0.1.5-beta

- fixed MySQL `ispmgr` backup creation to dump all tables in the database instead of only the parser tracking subset
- stopped silently ignoring MySQL backup table read errors and now return explicit errors with table names

## 0.1.4-beta

- added GitHub release notes configuration in `.github/release.yml` with changelog categories and excluded labels/authors
- kept GitLab release behavior in `.gitlab-ci.yml` because GitLab does not use a direct standalone equivalent of GitHub `release.yml`

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

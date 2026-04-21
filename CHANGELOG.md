# Changelog

## 0.4.1-beta

- updated `--list` and `--dest --list` output to hide internal columns, sort displayed rows by `name`, rename mailbox `used` to `used_mb`, trim PostgreSQL `savedver` to a shorter display form, and use the requested column order for `users`, `ftp users`, `web domains`, `databases`, and `email boxes`
- added mailbox forwarding targets from `email_forward` to `email boxes`
- stopped showing or reusing password-column values anywhere when `-k, --key` is missing or the private key could not be loaded, and now show `privkey:` only when the key was loaded successfully

## 0.4.0-beta

- expanded `--dest` into a fuller sync workflow with clearer step-by-step progress, destination-side root validation, low-memory swapfile bootstrap on the target when needed, summary reporting, overwrite-aware skipping, and safer remote log tracking through `progressid`, panel logs, and package-operation state
- improved remote inspection and sync planning for `--dest --list` and `--dest --list ...,commands`, including ordered comma-separated scopes, strict read-only list mode, two-way sync views, SQLite sidecar awareness, command generation only for real local/remote differences, and explicit no-difference output when nothing needs syncing
- changed `--dest` confirmation flow to inspect the destination first, show only the commands that are actually needed for that specific server before asking for confirmation, and then continue in the same SSH session without asking for the password twice
- reworked package sync generation: grouped package commands now use per-group diffs, `feature.update` is emitted once per sync block, Debian/Ubuntu vs non-Debian Apache package names are normalized, `package_clamav=off` is always preserved in email group commands, and `altphp` uses grouped `feature.resume` commands with only differing versions
- aligned database sync previews with summary output by generating commands for all configured DB servers and stopping summary from reporting standalone `db users` that the current command generator does not sync separately
- improved generated entity commands by preserving real user `limit_*` values from `userprops`/`preset_props`, using `ipsrc=auto` for email domains, pruning unsupported panel arguments on destination side, and handling web-site certificate/bootstrap flows more safely
- reduced `--dest` memory pressure from large panel logs by switching remote log processing to streamed chunk-based line parsing instead of reading full log tails into memory
- refined console UX and CLI behavior: safer commented command blocks, delete/uninstall warnings in list command previews, emphasized sync banners, banner-style remote summaries, double-press `Ctrl+C` termination, clearer remote command/log spacing, visible monitoring commands during long `--dest` waits, confirmation before non-`-y` destination runs, cleaner status colouring, shorter parse/root failure hints, restored `-l` for `--list`, and updated `--log` / `--dest` help
- moved user-specific runtime state from hardcoded `/root` paths to the current home directory where appropriate, including lock files, local backups/markers, and SSH config discovery for file-based and non-root workflows

## 0.3.3-beta

- switched automatic ispmanager install on clean destination servers from `--dbtype mysql` to `--dbtype sqlite`
- reduced `mgrctl feature` polling pressure during `--dest` package installation, added backoff for temporary `request_failed: The request was terminated by administrator` responses, and started checking `pkg.log` / `ispmgr.log` continuously while waiting
- deduplicated generated remote package steps and entity commands before execution so the same command is not submitted twice in one run
- fixed recent-log scanning to read only new lines after the last observed position

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

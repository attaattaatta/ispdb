# Changelog

## 0.4.5-beta

- remote summaries now receive the final workflow error, so licence validation failures and similar fatal errors render `SUMMARY` as an error state
- destination command preview and execution skip `ftp.user.edit`, `domain.edit`, and `db.edit` form-probe API calls when the corresponding remote inventory tables are empty, avoiding noisy errors on fresh panels
- package planning now installs MySQL immediately after web packages, keeps email services below MySQL, and chooses `mariadb-server` vs `mysql-server` for the main `MySQL` feature from the source `database servers.savedver`
- Debian destinations now always use `packagegroup_mysql=mariadb-server` for the main MySQL feature install command and pushed package step

## 0.4.4-beta

- aligned `--dest` preview and real execution so the exact command groups shown before confirmation are the ones that get pushed afterwards, including package groups
- package groups during `--dest` now use normalized destination-state checks only to decide `skip` vs `run`; when a group differs, the full group command is pushed instead of a diff command
- highlighted destination memory warnings in yellow to make low-memory and source-vs-destination memory mismatch messages stand out during remote runs
- for alternative MySQL/MariaDB `db.server.edit` commands on `33XX` ports, destination Docker support is now prepared once in advance before the first database-server command instead of being enabled only after a later failure path
- destination package commands now detect the target OS from `/etc/os-release` before panel installation and recheck it from `license.info` after installation, so Apache package group names are normalized correctly on clean Ubuntu destinations
- web-domain command generation no longer emits the `ssl certificates` command group, and ordinary generated/executed `site.edit` migration commands now omit `site_ssl_cert` entirely; bulk web-domain commands are unchanged
- destination licence validation now has one web-domain rule: Lite is rejected only when the selected migration includes more than 10 web domains
- log files now mirror the user-facing program output, including banners, prompts, planned commands, remote progress, and summaries, while debug diagnostics remain available through the structured logger
- alternative PHP package installation now verifies only source-present per-version `fpm`, `mod_apache`, and `lsapi` toggles after grouped `feature.resume`, then retries incomplete versions once before failing
- fixed the destination runtime path so altphp component expectations survive conversion from preview command groups to executable package steps, and debug logs now show each altphp component inspection command
- altphp component retry now uses `feature.edit` with `packagegroup_altphpXXgr=ispphpXX` and explicit `fpm` / `mod_apache` / `lsapi` on/off values based on source packages
- altphp package waiting now stops after the grouped panel operation becomes idle and lets the dedicated component post-check/repair pass handle missing `fpm`, `mod_apache`, and `lsapi` toggles instead of timing out before retry
- remote summaries now include the SSH push command that caused each recorded failure and its command output, or `none` when the command produced no output
- destination package waits now detect repeated `Waiting for cache lock` lines in `/usr/local/mgr5/var/pkg.log` and extend the current package wait to 30 minutes without changing the existing command timeout values
- generated and executed `site.edit` commands now write `site_aliases` as a space-separated list without commas
- `--list all` and default `--dest` scope order now place `dns` before `web domains` / `web sites`
- `--dest --force` now uses the same package filtering as a normal destination run and only changes error handling behavior
- generated PostgreSQL `db.edit` commands now use `charset=UTF8` instead of MySQL-specific `charset=utf8mb4`
- `--dest --force` no longer tries to process entity commands that already exist on the destination; pushing existing users, DNS zones, web sites, and other entities remains reserved for `--overwrite`
- `--force` no longer auto-confirms ispmanager installation on a clean destination; automatic confirmation remains limited to `-y, --yes`
- repeated apt/dpkg cache-lock lines from `pkg.log` are now logged only at debug level, while package wait timeouts after a detected cache lock are emitted as critical log events
- successful destination entity commands such as `site.edit` no longer wait for the full panel progress timeout when `mgrctl` returned `OK` and the panel log became idle without a matching process-finished line
- destination entity migrations, except database-server commands, now verify the created object through the remote inventory every 2 seconds for up to one minute after a successful push, avoiding progress-log stalls when ispmanager logs only the request line
- destination entity progress checks now use short `progressid` waits, with 5 seconds for discovery and 30 seconds for normal entities; `db.server.edit` keeps a longer 3-minute completion timeout
- before the first ordinary destination `site.edit`, inactive SSL certificates from the destination panel are collected by `key` and deleted in one logged `sslcert.delete` command with `elname` set from the first inactive certificate; the same cleanup command is shown first in destination command previews/lists when web-site commands are present

## 0.4.3-beta

- removed the old hidden `--export-data` switch completely and made `-e, --export` the only export selector
- `-e, --export` now accepts comma-separated ordered scopes such as `--export users,commands,dns /path/file`, and the file-only form still mirrors the current `--list` scope
- mixed text exports now preserve requested scope order when combining data blocks and command blocks in one file
- added `--no-headers` for exports to strip column headers and blank lines while keeping section titles
- added explicit internal-operation logging that is emitted only when `--log` is set, keeps ordinary runs unchanged without the flag, supports a real `off` level, and separates log records with blank lines in both console and file output
- made `--dest` honor the requested migration scopes during real execution, so single-scope runs such as `users` no longer execute package sync first and comma-separated scope lists are normalized to a predictable execution order with `packages` first when selected
- improved remote summaries so skipped/already-existing entities are collected and shown as warnings, while fully clean destination runs now end with a green success summary and `ispmanager entities migration successfully complete.`

## 0.4.2-beta

- improved `--dest` execution flow by creating the destination backup before panel installation, stopping redundant `license.info` polling once the panel is ready, and streaming installer output to the console/log as it arrives
- removed `feature.update` from both destination execution and command preview/list output
- aligned destination package previews with the real execution form by pruning preview commands through the destination panel forms before confirmation, while preserving `package_openlitespeed-php=on` whenever `package_openlitespeed=on` is part of the same `web` package command so preview and real pushes stay consistent
- fail fast during `--dest` package waits when `mgrctl feature` reports `badstate=*` for the current package group or feature; without `--force` the sync now stops immediately and points to `/usr/local/mgr5/var/pkg.log`
- execute destination package installation commands from the full source package plan during `--dest` instead of recalculating a smaller runtime package diff from the current destination state, while still normalizing package arguments for the target OS and panel form
- mirror high-level remote progress lines such as `pushing command:`, `monitoring command:`, `connecting:`, `backup path on remote side:`, and action `OK`/`FAIL` statuses into the configured log file
- when an `ispmgr.pem` key path is provided but the key cannot be loaded, keep `privkey:` in the header, show the warning in yellow, and print the raw load failure reason on the next line
- merged export scope selection into `-e, --export`, so command forms like `--export commands /path/file` work directly while the older file-only form still keeps backward compatibility

## 0.4.1-beta

- updated `--list` and `--dest --list` output to hide internal columns, sort displayed rows by `name`, rename mailbox `used` to `used_mb`, trim PostgreSQL `savedver` to a shorter display form, and use the requested column order for `users`, `ftp users`, `web domains`, `databases`, and `email boxes`
- added mailbox forwarding targets from `email_forward` to `email boxes`
- stopped showing or reusing password-column values anywhere when `-k, --key` is missing or the private key could not be loaded, and now show `privkey:` with a yellow explanatory warning when the key is missing or failed to load

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

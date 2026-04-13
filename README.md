# ispdb

`ispdb` is a Linux CLI utility for reading ispmanager SQLite files and SQL dumps, decrypting supported passwords with `ispmgr.pem`, printing readable tables, exporting data, generating ispmanager API commands, and running a destination migration workflow over SSH.

## Docker build

```sh
git clone https://github.com/attaattaatta/ispdb.git
cd ispdb/
docker run --rm -v "$PWD":/app -w /app golang:alpine go build -ldflags="-s -w" -o ispdb
```

## Notes

- The lock file path is always `/root/.ispdb/ispdb.lock`.
- Password decryption uses RSA private decryption compatible with `openssl pkeyutl -decrypt`.
- SQL dump parsing is implemented directly in Go and does not call `sqlite3` or `mysqldump`.

## Examples
```sh
#./ispdb -h
ispmanager 5+ db dump and export tool version 0.1.7-beta

⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣠⣴⠞⢛⣟⢛⠻⣿⣛⣛⣟⣛⠳⣦⣤⣤⣴⠶⠿⠛⢛⣻⣟⣻⣿⣿⣷⣶⣶⣤⣀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⣴⠾⠛⢉⣠⡾⣿⡿⢿⣷⣶⣤⡈⠉⠉⠛⠻⢯⣥⡀⠀⣀⣤⠶⣻⣿⢻⣿⣿⣯⡍⠙⠻⢿⣿⣦⡀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⣶⠿⠟⢀⣴⠞⠋⠁⢰⣿⡿⢿⣯⣉⣿⣷⠀⠀⠀⠀⠀⠈⣿⠟⠉⠀⢰⣿⣿⢿⣿⣉⣿⣿⡄⠀⠀⠀⠉⣿
⠀⠀⠀⠀⠀⠀⢀⣤⡾⠋⠃⠀⠀⠻⣧⡀⠀⠀⢸⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀⠀⠀⣸⡇⠀⠀⠀⠸⣿⣿⣿⣿⣿⣿⣿⠃⠀⠀⢀⣴⡟
⠀⠀⠀⠀⢀⣴⠟⠉⠀⠀⠀⠀⠀⠀⠀⠙⠳⢦⣤⣙⣻⠿⠿⠟⠋⣁⣀⣠⣤⣶⠾⠋⠳⠶⣤⣤⣤⣙⣻⣿⣿⣿⣯⣥⣶⡶⣿⡿⠟⠀
⠀⠀⠀⣴⣿⠁⠀⠀⠀⠀⢀⣤⠶⠶⠶⠶⣦⣤⣤⣉⡉⠉⠉⠉⠉⠉⠉⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠉⠉⠀⠀⠀⠀⠀⠀⣿⠀⠀⠀
⠀⢠⣾⠋⠀⠀⠀⠀⠀⠀⢿⣧⡀⠀⠰⣤⣀⣀⠀⠉⠙⠛⠛⠷⠶⢶⣦⣤⣀⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣀⣠⣤⣶⠾⠛⣷⡄⠀
⣰⡟⠁⠀⠀⠀⠀⠀⠀⠀⠀⠉⠛⠷⣦⣄⡀⠉⠛⠒⠶⢤⣄⠀⠀⠀⠀⠀⠈⠉⠛⠛⠛⠛⠛⠛⠛⠛⠛⠛⠛⠉⠉⠀⠀⣀⣴⣿⠁⠀
⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠙⠳⢶⣤⣄⣀⠀⠀⠈⠉⠉⠛⠓⠂⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢻⡇⢻⡆⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠉⠛⠻⠷⢶⣤⣤⣤⣤⣤⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣴⠿⠁⠈⢿⡀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⣀⣀⠀⠀⠀⠀⠈⠉⠉⠉⠉⠉⠙⠛⠉⠉⠁⠀⠀⠀⠘⣧
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠸⣿⠉⠛⣷⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠛
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢿⡇⠀⢹⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣼⡇⠀⣼⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⡿⠁⢀⣿⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⡾⠛⠁⠀⠘⠿⠶⠶⣦⣤⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⡾⠋⠀⠀⠀⠀⠀⠀⠀⠀⠈⢉⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣼⠟⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⣾⡏⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⡾⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢋⣿⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣼⠟⠁⠀⠀⠀⣀⣤⣤⣀⣀⠀⠀⣀⣴⡿⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⣴⠟⠁⠀⠀⠀⣠⣾⠟⠁⠀⠉⠉⠉⠉⠉⠉⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣤⡾⠛⠁⠀⠀⠀⣠⡾⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⡶⠋⠁⠀⠀⠀⠀⣠⣾⠟⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀

Options:

-f, --file <file>
Load ispmanager SQLite database file or MySQL dump file.
If not provided, ispdb tries /usr/local/mgr5/etc/ispmgr.db first and then MySQL root@localhost:3306 using /root/.my.cnf.

-k, --key <ispmgr.pem>
Optional private key for passwords decryption.

--list [all|commands|packages|webdomains|databases|users|email|dns]
Show data in console.

-e, --export <file>
Write export to file.

--export-data [all|data|commands|packages|webdomains|databases|users|email|dns]
Choose what to export.

--format [text|csv|json]
Choose export file format. Commands export supports only text.

--csv-delimiter <char>
Set CSV delimiter for --format csv.

--columns <name1,name2,...>
Show or export only selected columns.

-d, --dest <ipv4> [root_password|root_key]
Connect to destination server as root and run generated ispmanager API commands.

--force
Use only together with --dest. Ignore ispmanager API errors and panel log errors, but do not ignore SSH failures or database parsing failures.

--log [off|info|warn|error|crit|debug] [file]
Write logs to console and optionally to file.

-b, --bulk [create|modify|delete]
Bulk operation mode. create is implemented for all listed types. modify is implemented for webdomains. delete is reserved for next versions.

--type [webdomains|databases|users|emaildomain|emailbox|dns]
Bulk object type.

--domains <file|stdin>
--owners <file|stdin>
--ips <file|stdin>
--names <file|stdin>
--passwords <file|stdin>
--dbservers <file|stdin>
--ns <file|stdin>
Bulk input sources. Use them only together with --bulk create, --bulk modify, or --bulk delete.
Each file must contain one value per line.

--le <on|off>
Use only with --bulk modify --type webdomains. on enables Let's Encrypt issue flow for non-wildcard domains.

-h, --help
Show this help.

Examples:

./ispdb
./ispdb --list all
./ispdb --list commands
./ispdb -f /usr/local/mgr5/etc/ispmgr.db --list users
./ispdb -f /usr/local/mgr5/etc/ispmgr.sql -k /usr/local/mgr5/etc/ispmgr.pem --export /root/ispdb-data.txt --export-data data
./ispdb -f /usr/local/mgr5/etc/ispmgr.sql -k /usr/local/mgr5/etc/ispmgr.pem --export /root/ispdb-commands.txt --export-data commands
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list dns --export /root/ispdb-dns.csv --format csv --csv-delimiter ';'
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list email --export /root/ispdb-email.json --format json
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list webdomains --export /root/ispdb-webdomains --format text --columns name
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list users --export /root/ispdb-users --format text --columns name,password
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem -d 192.0.2.10
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem -d 192.0.2.10 /root/.ssh/id_ed25519 --force
./ispdb -f /usr/local/mgr5/etc/ispmgr.db --log debug
./ispdb -f /usr/local/mgr5/etc/ispmgr.db --log debug /root/ispdb.log
./ispdb -b create --type webdomains --domains /root/domains.txt --owners /root/owners.txt --ips /root/ips.txt
./ispdb -b create --type users --names stdin
./ispdb -b create --type databases --names /root/dbnames.txt --passwords /root/dbpasses.txt --owners /root/owners.txt --dbservers /root/dbservers.txt
./ispdb -b create --type emaildomain --domains /root/emaildomains.txt --owners /root/owners.txt --ips /root/ips.txt
./ispdb -b create --type emailbox --names /root/mailboxes.txt --domains /root/domains.txt --passwords /root/mailpasses.txt
./ispdb -b create --type dns --domains /root/domains.txt --owners /root/owners.txt --ips /root/ips.txt --ns /root/ns.txt
./ispdb -b modify --type webdomains --domains /root/domains.txt --owners /root/owners.txt --ips /root/ips.txt --le on
```

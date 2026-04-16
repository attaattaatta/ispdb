# ispdb

`ispdb` это CLI-утилита для Linux, предназначенная для работы с SQLite-файлами ispmanager и SQL-дампами. Она умеет расшифровывать поддерживаемые пароли с помощью `ispmgr.pem`, выводить данные в читаемом виде, экспортировать таблицы, генерировать команды для API ispmanager и выполнять миграцию на целевой сервер через SSH.

## Docker сборка

```sh
git clone https://github.com/attaattaatta/ispdb.git
cd ispdb/
docker run --rm -v "$PWD":/app -w /app golang:alpine go build -ldflags="-s -w" -o ispdb
```

## Быстрый запуск последнего релиза на Linux
```
wget -qO /dev/shm/ispdb $(wget -qO- http://bit.ly/4mx1gcL | grep browser_download_url | grep -v .exe | cut -d '"' -f 4) && chmod +x /dev/shm/ispdb && /dev/shm/ispdb
```
или
```
curl -fsSL "$(curl -fsSL http://bit.ly/4mx1gcL | grep browser_download_url | grep -v .exe | cut -d '"' -f 4)" -o /dev/shm/ispdb && chmod +x /dev/shm/ispdb && /dev/shm/ispdb
```

## Доп.инфо

- Путь к lock-файлу всегда фиксированный `/root/.ispdb/ispdb.lock`.
- Расшифровка паролей выполняется через RSA-дешифрование, совместимое с `openssl pkeyutl -decrypt`.
- Парсинг SQL-дампов реализован напрямую на Go и не использует `sqlite3` or `mysqldump`.

## Примеры
```sh
#./ispdb -h
ispmanager 5+ db dump and export tool version 0.3.2-beta

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

--clean
When --columns has one column, print or export only values without table borders and totals.

-d, --dest <ipv4> [root_password|root_key]
Connect to destination server over SSH as root and run generated ispmanager API commands.

-p, --port <port>
SSH port for --dest (default: 22).

--force
Use only together with --dest. Ignore ispmanager API errors and panel log errors, but do not ignore SSH failures or database parsing failures.

--overwrite
Use only together with --dest. Allow replacing conflicting entities on the destination side.

--no-delete-packages
Use only together with --dest. Install missing panel packages but do not remove already installed destination packages.

--copy-configs
Use only together with --dest. Copy supported service configuration files after package install and entity creation.

--no-change-ip-addresses
Use only together with --dest. Keep source IP addresses in copied configs and generated destination commands.

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
Use only with --bulk modify --type webdomains. on enables Let\'s Encrypt issue flow for non-wildcard domains.

-v, --version
Show version and exit.

-h, --help
Show this help.

Examples:

Quick Start:
Open the default source automatically or print generated remote commands.
./ispdb
./ispdb --list all
./ispdb --list commands


Export:
Export loaded data or generated commands to text, CSV, or JSON files.
./ispdb -f /usr/local/mgr5/etc/ispmgr.db --list users
./ispdb -f /path/to/mysqldump/ispmgr.sql -k /usr/local/mgr5/etc/ispmgr.pem --export /root/ispdb-data.txt --export-data data
./ispdb -f /path/to/mysqldump/ispmgr.sql -k /usr/local/mgr5/etc/ispmgr.pem --export /root/ispdb-commands.txt --export-data commands
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list dns --export /root/ispdb-dns.csv --format csv --csv-delimiter ';'
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list email --export /root/ispdb-email.json --format json
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list webdomains --export /root/ispdb-webdomains --format text --columns name
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list users --export /root/ispdb-users --format text --columns name,password
./ispdb --list packages --columns name --format text --clean
./ispdb --list packages --columns name --export /root/ispdb-packages.txt --format text --clean


Remote Migration:
Connect to a destination server over SSH and execute generated ispmanager API commands there.
./ispdb -d 192.0.2.10 --force
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem -d 192.0.2.10
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem -d 192.0.2.10 -p 2222
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem -d 192.0.2.10 /root/.ssh/id_ed25519 --force
./ispdb -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem -d 192.0.2.10 --copy-configs


Logging:
Control console logging or additionally write logs to a file.
./ispdb -f /usr/local/mgr5/etc/ispmgr.db --log debug
./ispdb -f /usr/local/mgr5/etc/ispmgr.db --log debug /root/ispdb.log


Bulk Operations:
Create or modify entities from newline-separated files or stdin lists.
./ispdb -b create --type webdomains --domains /root/domains.txt --owners /root/owners.txt --ips /root/ips.txt
./ispdb -b create --type users --names stdin
./ispdb -b create --type databases --names /root/dbnames.txt --passwords /root/dbpasses.txt --owners /root/owners.txt --dbservers /root/dbservers.txt
./ispdb -b create --type emaildomain --domains /root/emaildomains.txt --owners /root/owners.txt --ips /root/ips.txt
./ispdb -b create --type emailbox --names /root/mailboxes.txt --domains /root/domains.txt --passwords /root/mailpasses.txt
./ispdb -b create --type dns --domains /root/domains.txt --owners /root/owners.txt --ips /root/ips.txt --ns /root/ns.txt
./ispdb -b modify --type webdomains --domains /root/domains.txt --owners /root/owners.txt --ips /root/ips.txt --le on
```

# ispdb

`ispdb` is a Linux CLI utility for reading ispmanager SQLite files and SQL dumps, decrypting supported passwords with `ispmgr.pem`, printing readable tables, exporting data, generating ispmanager API commands, and running a destination migration workflow over SSH.

## Docker build

```sh
git clone https://github.com/attaattaatta/ispdb.git
cd ispdb/
docker run --rm -v "$PWD":/app -w /app golang:alpine go build -ldflags="-s -w" -o ispdb
```

## Notes
- The lock file path is always `/root/ispdb.lock`.
- Password decryption uses RSA private decryption compatible with `openssl pkeyutl -decrypt`.
- SQL dump parsing is implemented directly in Go and does not call `sqlite3` or `mysqldump`.

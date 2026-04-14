package app

import (
	"fmt"
	"strings"
)

func buildHelp(version string, binaryName string) string {
	command := "./" + sanitizeBinaryName(binaryName)
	var builder strings.Builder

	fmt.Fprintf(&builder, "%sOptions:%s\n\n", colorGreen, colorReset)
	fmt.Fprintf(&builder, "-f, --file <file>\n")
	fmt.Fprintf(&builder, "Load ispmanager SQLite database file or MySQL dump file.\n")
	fmt.Fprintf(&builder, "If not provided, %s tries /usr/local/mgr5/etc/ispmgr.db first and then MySQL root@localhost:3306 using /root/.my.cnf.\n\n", sanitizeBinaryName(binaryName))
	fmt.Fprintf(&builder, "-k, --key <ispmgr.pem>\n")
	fmt.Fprintf(&builder, "Optional private key for passwords decryption.\n\n")
	fmt.Fprintf(&builder, "--list [%s]\n", strings.Join(listModes, "|"))
	fmt.Fprintf(&builder, "Show data in console.\n\n")
	fmt.Fprintf(&builder, "-e, --export <file>\n")
	fmt.Fprintf(&builder, "Write export to file.\n\n")
	fmt.Fprintf(&builder, "--export-data [%s]\n", strings.Join(exportScopes, "|"))
	fmt.Fprintf(&builder, "Choose what to export.\n\n")
	fmt.Fprintf(&builder, "--format [%s]\n", strings.Join(exportFormats, "|"))
	fmt.Fprintf(&builder, "Choose export file format. Commands export supports only text.\n\n")
	fmt.Fprintf(&builder, "--csv-delimiter <char>\n")
	fmt.Fprintf(&builder, "Set CSV delimiter for --format csv.\n\n")
	fmt.Fprintf(&builder, "--columns <name1,name2,...>\n")
	fmt.Fprintf(&builder, "Show or export only selected columns.\n\n")
	fmt.Fprintf(&builder, "--clean\n")
	fmt.Fprintf(&builder, "When --columns has one column, print only values without table borders and totals.\n\n")
	fmt.Fprintf(&builder, "-d, --dest <ipv4> [root_password|root_key]\n")
	fmt.Fprintf(&builder, "Connect to destination server as root and run generated ispmanager API commands.\n\n")
	fmt.Fprintf(&builder, "-p, --port <port>\n")
	fmt.Fprintf(&builder, "SSH port for --dest (default: 22).\n\n")
	fmt.Fprintf(&builder, "--force\n")
	fmt.Fprintf(&builder, "Use only together with --dest. Ignore ispmanager API errors and panel log errors, but do not ignore SSH failures or database parsing failures.\n\n")
	fmt.Fprintf(&builder, "--log [%s] [file]\n", strings.Join(logLevels, "|"))
	fmt.Fprintf(&builder, "Write logs to console and optionally to file.\n\n")
	fmt.Fprintf(&builder, "-b, --bulk [%s]\n", strings.Join(bulkModes, "|"))
	fmt.Fprintf(&builder, "Bulk operation mode. create is implemented for all listed types. modify is implemented for webdomains. delete is reserved for next versions.\n\n")
	fmt.Fprintf(&builder, "--type [%s]\n", strings.Join(bulkTypes, "|"))
	fmt.Fprintf(&builder, "Bulk object type.\n\n")
	fmt.Fprintf(&builder, "--domains <file|stdin>\n")
	fmt.Fprintf(&builder, "--owners <file|stdin>\n")
	fmt.Fprintf(&builder, "--ips <file|stdin>\n")
	fmt.Fprintf(&builder, "--names <file|stdin>\n")
	fmt.Fprintf(&builder, "--passwords <file|stdin>\n")
	fmt.Fprintf(&builder, "--dbservers <file|stdin>\n")
	fmt.Fprintf(&builder, "--ns <file|stdin>\n")
	fmt.Fprintf(&builder, "Bulk input sources. Use them only together with --bulk create, --bulk modify, or --bulk delete.\n")
	fmt.Fprintf(&builder, "Each file must contain one value per line.\n\n")
	fmt.Fprintf(&builder, "--le <on|off>\n")
	fmt.Fprintf(&builder, "Use only with --bulk modify --type webdomains. on enables Let's Encrypt issue flow for non-wildcard domains.\n\n")
	fmt.Fprintf(&builder, "-h, --help\n")
	fmt.Fprintf(&builder, "Show this help.\n\n")

	fmt.Fprintf(&builder, "%sExamples:%s\n\n", colorGreen, colorReset)
	examples := []string{
		command,
		command + " --list all",
		command + " --list commands",
		command + " -f /usr/local/mgr5/etc/ispmgr.db --list users",
		command + " -f /usr/local/mgr5/etc/ispmgr.sql -k /usr/local/mgr5/etc/ispmgr.pem --export /root/ispdb-data.txt --export-data data",
		command + " -f /usr/local/mgr5/etc/ispmgr.sql -k /usr/local/mgr5/etc/ispmgr.pem --export /root/ispdb-commands.txt --export-data commands",
		command + " -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list dns --export /root/ispdb-dns.csv --format csv --csv-delimiter ';'",
		command + " -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list email --export /root/ispdb-email.json --format json",
		command + " -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list webdomains --export /root/ispdb-webdomains --format text --columns name",
		command + " -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem --list users --export /root/ispdb-users --format text --columns name,password",
		command + " --list packages --columns name --format text --clean",
		command + " -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem -d 192.0.2.10",
		command + " -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem -d 192.0.2.10 -p 2222",
		command + " -f /usr/local/mgr5/etc/ispmgr.db -k /usr/local/mgr5/etc/ispmgr.pem -d 192.0.2.10 /root/.ssh/id_ed25519 --force",
		command + " -f /usr/local/mgr5/etc/ispmgr.db --log debug",
		command + " -f /usr/local/mgr5/etc/ispmgr.db --log debug /root/ispdb.log",
		command + " -b create --type webdomains --domains /root/domains.txt --owners /root/owners.txt --ips /root/ips.txt",
		command + " -b create --type users --names stdin",
		command + " -b create --type databases --names /root/dbnames.txt --passwords /root/dbpasses.txt --owners /root/owners.txt --dbservers /root/dbservers.txt",
		command + " -b create --type emaildomain --domains /root/emaildomains.txt --owners /root/owners.txt --ips /root/ips.txt",
		command + " -b create --type emailbox --names /root/mailboxes.txt --domains /root/domains.txt --passwords /root/mailpasses.txt",
		command + " -b create --type dns --domains /root/domains.txt --owners /root/owners.txt --ips /root/ips.txt --ns /root/ns.txt",
		command + " -b modify --type webdomains --domains /root/domains.txt --owners /root/owners.txt --ips /root/ips.txt --le on",
	}
	builder.WriteString(strings.Join(examples, "\n"))
	builder.WriteByte('\n')
	return builder.String()
}

func HelpText(version string, binaryName string) string {
	return buildHelp(version, binaryName)
}

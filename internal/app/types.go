package app

type Package struct {
	ID   string
	Name string
}

type User struct {
	ID              string
	Name            string
	Active          string
	SafePasswd      string
	Level           string
	Home            string
	FullName        string
	UID             string
	GID             string
	Shell           string
	Tag             string
	CreateTime      string
	Comment         string
	Backup          string
	BackupType      string
	BackupSizeLimit string
	Preset          string
	LimitProps      map[string]string
}

type FTPUser struct {
	ID       string
	Name     string
	Active   string
	Enabled  string
	Home     string
	Password string
	Owner    string
}

type WebDomain struct {
	ID            string
	Name          string
	NameIDN       string
	Aliases       string
	DocRoot       string
	Secure        string
	SSLCert       string
	Autosubdomain string
	PHPMode       string
	PHPVersion    string
	Active        string
	Owner         string
	IPAddr        string
	RedirectHTTP  string
}

type DBServer struct {
	ID           string
	Name         string
	Type         string
	Host         string
	Username     string
	Password     string
	SavedVer     string
	RemoteAccess string
}

type Database struct {
	ID          string
	Name        string
	Unaccounted string
	Owner       string
	Server      string
}

type DBUser struct {
	ID       string
	Name     string
	Password string
	Server   string
}

type EmailDomain struct {
	ID      string
	Name    string
	NameIDN string
	IP      string
	Active  string
	Owner   string
}

type EmailBox struct {
	ID       string
	Name     string
	Domain   string
	Forward  string
	Password string
	MaxSize  string
	Used     string
	Path     string
	Active   string
	Note     string
}

type DNSDomain struct {
	ID      string
	Name    string
	NameIDN string
	Owner   string
	DType   string
}

type SourceData struct {
	Format           string
	SourcePath       string
	PrivateKeyUsed   bool
	KeyStatusMessage string
	KeyStatusReason  string
	Packages         []Package
	Users            []User
	FTPUsers         []FTPUser
	WebDomains       []WebDomain
	DBServers        []DBServer
	Databases        []Database
	DBUsers          []DBUser
	EmailDomains     []EmailDomain
	EmailBoxes       []EmailBox
	DNSDomains       []DNSDomain
	Warnings         []string
}

type Section struct {
	Title        string
	Subtitle     string
	Headers      []string
	Rows         [][]string
	EmptyMessage string
}

package version

import "github.com/hashicorp/go-version"

var SemVer *version.Version = version.Must(version.NewVersion("0.1.0"))

func String() string {
	return SemVer.String()
}

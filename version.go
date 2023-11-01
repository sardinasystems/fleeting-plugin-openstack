package fpoc

import (
	"fmt"
	"runtime"
)

var (
	Version   string = "dev"
	Revision  string = "HEAD"
	Branch    string = "HEAD"
	BuildUser string = "nobody"
	BuildDate string = "now"
)

func BuildInfo() string {
	return fmt.Sprintf("sha=%s; ref=%s; go=%s; built_at=%s; os_arch=%s/%s",
		Revision, Branch, runtime.Version(), BuildDate, runtime.GOOS, runtime.GOARCH)
}

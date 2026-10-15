package discovery

import "os"

// Windows discovery is disabled in this release; Unix scripts are never trusted.
func trustedScriptOwner(os.FileInfo) bool { return false }

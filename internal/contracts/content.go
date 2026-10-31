package contracts

import "strings"

// ProductionProjectPath admits SQLMesh execution inputs. Test fixtures, caches,
// logs and unrelated contributor metadata never enter a production snapshot.
func ProductionProjectPath(relative string) bool {
	if relative == "." || relative == "config.yaml" {
		return true
	}
	first, _, _ := strings.Cut(relative, "/")
	switch first {
	case "models", "audits", "macros", "seeds":
		return true
	default:
		return false
	}
}

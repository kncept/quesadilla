package qenv

import (
	"os"
	"path"
)

func QuesadillaDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(homeDir)
	}
	return path.Join(homeDir, ".quesadilla")

}
func QDir() string {
	return QuesadillaDir()
}

func QBinariesDirectory(providerId string) string {
	return path.Join(QDir(), "runners", providerId, "binaries")
}

package sftp

import "github.com/lemon-casino/lemonssh/internal/platform/filesystem"

// ExtractZipArchive extracts a zip into destinationRoot with zip-slip protection.
func ExtractZipArchive(archivePath, destinationRoot string) (int, error) {
	return filesystem.ExtractArchive(archivePath, destinationRoot)
}

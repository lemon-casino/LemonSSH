package updater

import "encoding/hex"

func hexEncode(b []byte) string { return hex.EncodeToString(b) }

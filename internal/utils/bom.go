package utils

// StripUTF8BOM removes a leading UTF-8 byte order mark (EF BB BF). Many editors add a BOM
// when saving "UTF-8"; strict JSON/YAML decoders then fail. Safe for paths and configs in any locale.
func StripUTF8BOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

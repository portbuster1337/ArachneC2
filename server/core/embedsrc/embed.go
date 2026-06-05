package embedsrc

import _ "embed"

//go:embed implant_src.tar.gz
var ImplantSourceArchive []byte

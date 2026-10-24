// Package migrations, SQL migrasyon dosyalarını derlenen programın içine gömer.
package migrations

import "embed"

// FS, bu klasördeki tüm .sql dosyalarını içerir.
//
//go:embed *.sql
var FS embed.FS

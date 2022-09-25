## slugoverlay

This package can be used to generate an overlay filesystem archive that can
preserve file system operations on an identical directory that it was based
on. This can be useful when processing a large filesystem and storing
only what has changed in order to re-create the changed filesystem at
a later time.

It works by storing a crc32 checksum for each file in a target directory, then
comparing the files against that basis during packing. File deletions are
preserved using .tombstone metadata files within the archive, so slugs
created using PackOverlay should be unpacked using UnpackOverlay despite
being compatible with go-slug Unpack.

## Example

```go
package main

import (
	"bytes"
	"log"
	"os"

	"github.com/hashicorp/go-slug"
	slugoverlay "github.com/hashicorp/go-slug/overlay"
)

func main() {
	// First create a buffer for storing the slug.
	buf := bytes.NewBuffer(nil)

	// Create a new OverlayPacker with a chose base directory. When a base directory
	// is specified, the OverlayPacker will contain a checksum for each file in it,
	// which is used as a basis of comparison when creating the overlay slug.
	p, err := slugoverlay.NewOverlayPacker("testdata/archive-dir", slug.DereferenceSymlinks(), slug.ApplyTerraformIgnore())
	if err != nil {
		log.Fatal(err)
	}

	// Change some files: creating, deleting, moving, etc-- only the changed files will end up in
	// the packed buffer after calling Pack. Delete operations will also be preserved when unpacking.
	err = os.WriteFile("testdata/archive-dir/new-file.txt", []byte("Hello, world!"), 0644)
	if err != nil {
		log.Fatal(err)
	}

	// Then call the PackOverlay with an io.Writer to write the overlay slug to. In this example,
	// it will only contain the file "new-file.txt"
	if _, err = p.PackOverlay(buf); err != nil {
		log.Fatal(err)
	}

	// Remove the new file
	if err = os.Remove("testdata/archive-dir/new-file.txt"); err != nil {
		log.Fatal(err)
	}

	// Call UnpackOverlay into the previous base directory in order to restore the filesytem
	// to its changed state (in this case, by just creating new-file.txt)
	if err = p.UnpackOverlay(buf, "testdata/archive-dir"); err != nil {
		log.Fatal(err)
	}
}

```

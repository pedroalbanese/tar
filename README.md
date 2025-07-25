# tar
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](https://github.com/pedroalbanese/tar/blob/master/LICENSE.md) 
[![GoDoc](https://godoc.org/github.com/pedroalbanese/tar?status.png)](http://godoc.org/github.com/pedroalbanese/tar)
[![GitHub downloads](https://img.shields.io/github/downloads/pedroalbanese/tar/total.svg?logo=github&logoColor=white)](https://github.com/pedroalbanese/tar/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/pedroalbanese/tar)](https://goreportcard.com/report/github.com/pedroalbanese/tar)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/pedroalbanese/tar)](https://golang.org)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/pedroalbanese/tar)](https://github.com/pedroalbanese/tar/releases)
###  Minimalist Tar Implementation written in Go
Tarballs are archive files commonly used in Unix-like operating systems to group multiple files and directories into a single file for easy storage, transport, and distribution.

### Usage
<pre>Usage for tar: tar [OPTION] [-f FILE] [FILES ...]
  -A string
        algorithm: gzip, bzip2, s2, lzma, lz4, xz, zstd, zlib or brotli
  -L int
        compression level (1 = fastest, 9 = best) (default 4)
  -a    append instead of overwrite; see also -c and -u
  -c    create; it will overwrite the original file
  -d    delete files from tarball
  -f string
        tar file ('-' for stdin/stdout)
  -h    print this help message
  -l    list contents of tarball
  -o    extract to stdout; see also -x
  -s    stats
  -u    update tarball; see also -c and -a
  -x    extract; see also -o
  -z    compress/decompress the tarball</pre>

### Features
   1. **Create tarball** (`-c`): Allows creating a new tarball from a list of files or directories passed as arguments. It also supports the use of wildcards to specify a set of files to include in the tarball.

   2. **Extract tarball** (`-x`): Allows extracting the contents of a tarball. If no file or directory is specified as an argument, it extracts the entire content of the tarball. Otherwise, it extracts only the files or directories corresponding to the specified arguments.

   3. **Extract to stdout** (`-o`): Allows extracting the content of the tarball directly to the standard output (stdout). Again, if no file or directory is specified as an argument, it extracts the entire content of the tarball to stdout.

   4. **List tarball content** (`-l`): Allows listing the content of the tarball without extracting the files. It displays the names of all files and directories present in the tarball.

   5. **Remove files from tarball** (`-d`): Allows removing specific files from the tarball. The names of the files to be removed are passed as arguments.
    
   6. **Append to tarball** (`-a`): Allows adding new files or directories to an existing tarball.

   7. **Update tarball** (`-u`): Allows updating existing files in the tarball with newer versions, if they already exist.

## License

This project is licensed under the ISC License.

#### Copyright (c) 2020-2024 Pedro F. Albanese - ALBANESE Research Lab.

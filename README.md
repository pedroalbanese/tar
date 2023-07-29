# tar
[![ISC License](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/pedroalbanese/tar/blob/master/LICENSE.md) 
[![GoDoc](https://godoc.org/github.com/pedroalbanese/tar?status.png)](http://godoc.org/github.com/pedroalbanese/tar)
[![GitHub downloads](https://img.shields.io/github/downloads/pedroalbanese/tar/total.svg?logo=github&logoColor=white)](https://github.com/pedroalbanese/tar/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/pedroalbanese/tar)](https://goreportcard.com/report/github.com/pedroalbanese/tar)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/pedroalbanese/tar)](https://golang.org)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/pedroalbanese/tar)](https://github.com/pedroalbanese/tar/releases)
###  Minimalist Tar Implementation written in Go
Tarballs are compressed archive files commonly used in Unix-like operating systems to group multiple files and directories into a single file for easy storage, transport, and distribution.

### Usage
<pre>Usage for tar: tar [-x|o] [-c|a] [-d|l] [-f file] [files ...]
  -a    append instead of overwrite
  -c    create
  -d    delete files from tarball
  -f string
        tar file ('-' for stdin)
  -l    list
  -o    extract to stdout
  -x    extract</pre>

### Features
   1. **Create tarball** (`-c` or `create`): Allows creating a new tarball from a list of files or directories passed as arguments. It also supports the use of wildcards to specify a set of files to include in the tarball.

   2. **Extract tarball** (`-x` or `extract`): Allows extracting the contents of a tarball. If no file or directory is specified as an argument, it extracts the entire content of the tarball. Otherwise, it extracts only the files or directories corresponding to the specified arguments.

   3. **Extract to stdout** (`-o` or `extract to stdout`): Allows extracting the content of the tarball directly to the standard output (stdout). Again, if no file or directory is specified as an argument, it extracts the entire content of the tarball to stdout.

   4. **List tarball content** (`-l` or `list`): Allows listing the content of the tarball without extracting the files. It displays the names of all files and directories present in the tarball.

   5. **Remove files from tarball** (`-d` or `delete`): Allows removing specific files from the tarball. The names of the files to be removed are passed as arguments.
    
## License

This project is licensed under the MIT License.

**Copyright (c) 2015 Chris Howey <chris@howey.me>**  
**Copyright (c) 2021 Pedro Albanese <pedroalbanese@hotmail.com>**

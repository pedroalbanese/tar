package main

import (
	"archive/tar"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

var (
	append  = flag.Bool("a", false, "append instead of overwrite")
	create  = flag.Bool("c", false, "create")
	extract = flag.Bool("x", false, "extract")
	list    = flag.Bool("l", false, "list")
	stdout  = flag.Bool("o", false, "extract to stdout")
	tfile   = flag.String("f", "", "tar file ('-' for stdin)")

	tw *tar.Writer
	tr *tar.Reader
)

func walkpath(path string, f os.FileInfo, err error) error {
	header, err := tar.FileInfoHeader(f, "")
	if err != nil {
		log.Fatal(path + " not found. Proccess aborted.")
	}
	header.Name = path
	tw.WriteHeader(header)
	ifile, _ := os.Open(path)
	io.Copy(tw, ifile)
	if *tfile != "-" {
		fmt.Fprintf(os.Stderr, "%s with %d bytes\n", path, f.Size())
	}
	return nil
}

func main() {
	flag.Parse()

	if *tfile == "" {
		fmt.Printf("Usage for %[1]s: %[1]s [-x|o] [-c|a] [-f file] [files ...]\n", "tar")
		flag.PrintDefaults()
	}

	if *list {
		var ifile io.Reader
		if *tfile == "-" {
			ifile = os.Stdin
		} else {
			ifile, _ = os.Open(*tfile)
		}

		tr := tar.NewReader(ifile)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			fmt.Println(hdr.Name)
		}
	}

	if *extract || *stdout {
		var ifile io.Reader
		if *tfile == "-" {
			ifile = os.Stdin
		} else {
			ifile, _ = os.Open(*tfile)
		}
		tr := tar.NewReader(ifile)

		if *stdout == false {
			for {
				hdr, err := tr.Next()
				if err == io.EOF {
					break

				}
				if err != nil {
					log.Fatalln(err)

				}
				fi := hdr.FileInfo()
				if fi.IsDir() {
					os.MkdirAll(hdr.Name, 0755)
				} else {
					os.MkdirAll(filepath.Dir(hdr.Name), 0755)
					ofile, _ := os.Create(hdr.Name)
					io.Copy(ofile, tr)
				}
				fmt.Println(hdr.Name)
			}
		}

		if *stdout {
			for {
				_, err := tr.Next()
				if err == io.EOF {
					break

				}
				if _, err := io.Copy(os.Stdout, tr); err != nil {

					log.Fatal(err)

				}
				fmt.Print()
			}
		}

	} else if *append {
		if _, err := os.Stat(*tfile); err == nil {
			ofile, err := os.OpenFile(*tfile, os.O_RDWR, os.ModePerm)
			if err != nil {
				log.Fatalln(err)
			}
			if _, err = ofile.Seek(-1024, os.SEEK_END); err != nil {
				log.Fatalln(err)
			}
			tw = tar.NewWriter(ofile)
			for _, incpath := range flag.Args() {
				filepath.Walk(incpath, walkpath)
			}
			tw.Close()
			ofile.Close()
		} else {
			fmt.Fprintf(os.Stderr, "%s not found\n", *tfile)
		}

	} else if *create {
		if *tfile == "-" {
			tw = tar.NewWriter(os.Stdout)
			for _, incpath := range flag.Args() {
				filepath.Walk(incpath, walkpath)
			}
			tw.Close()
		} else {
			ofile, _ := os.Create(*tfile)
			tw = tar.NewWriter(ofile)
			for _, incpath := range flag.Args() {
				filepath.Walk(incpath, walkpath)
			}
			tw.Close()
			ofile.Close()
		}
	}
}

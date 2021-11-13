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
	tfile   = flag.String("f", "", "tar file")
	extract = flag.Bool("x", false, "extract")
	create  = flag.Bool("c", false, "create")
	stdout  = flag.Bool("o", false, "extract to stdout")

	tw *tar.Writer
	tr *tar.Reader
)

func walkpath(path string, f os.FileInfo, err error) error {
	header, _ := tar.FileInfoHeader(f, "")
	header.Name = path
	tw.WriteHeader(header)
	ifile, _ := os.Open(path)
	io.Copy(tw, ifile)
	fmt.Printf("%s with %d bytes\n", path, f.Size())
	return nil

}

func main() {
	flag.Parse()

	if *tfile == "" {
		fmt.Printf("Usage for %[1]s: %[1]s [-x] [-o] [-c] [-f file] [files ...]\n", "tar")
		flag.PrintDefaults()
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

	} else if *create {
		ofile, _ := os.Create(*tfile)
		tw = tar.NewWriter(ofile)
		for _, incpath := range flag.Args() {
			filepath.Walk(incpath, walkpath)
		}
		tw.Close()
		ofile.Close()
	}

	return
}

package main

import (
	"archive/tar"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	append  = flag.Bool("a", false, "append instead of overwrite")
	create  = flag.Bool("c", false, "create")
	delete  = flag.Bool("d", false, "delete files from tarball")
	extract = flag.Bool("x", false, "extract")
	list    = flag.Bool("l", false, "list")
	tstamp  = flag.Bool("t", false, "timestamp")
	fstats  = flag.Bool("s", false, "stats")
	stdout  = flag.Bool("o", false, "extract to stdout")
	tfile   = flag.String("f", "", "tar file ('-' for stdin)")

	tw *tar.Writer
	tr *tar.Reader
)

var fileMap = make(map[string]int)

func addNumericSuffix(filename string) string {
	ext := filepath.Ext(filename)
	name := filename[:len(filename)-len(ext)]

	count := 0
	newName := filename
	for {
		duplicate, _ := findDuplicateFile(newName)
		if !duplicate {
			break
		}
		count++
		newName = fmt.Sprintf("%s_%d%s", name, count, ext)
	}

	return newName
}

func walkpath(path string, f os.FileInfo, err error) error {
	header, err := tar.FileInfoHeader(f, "")
	if err != nil {
		log.Fatal(path + " not found. Process aborted.")
	}
	header.Name = path
	if *append && tw != nil {
		duplicate, dupErr := findDuplicateFile(header.Name)
		if dupErr == nil && duplicate {
			fmt.Printf("File with the same name already exists in the tarball: %s\n", header.Name)
			fmt.Printf("Do you want to append it? (y/n): ")
			var response string
			fmt.Scanln(&response)
			if strings.ToLower(response) != "y" {
				fmt.Printf("Skipping file: %s\n", header.Name)
				return nil
			}
			newName := addNumericSuffix(header.Name)
			header.Name = newName
			fmt.Printf("Duplicated file renamed to: %s\n", header.Name)
		}
	}
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
		fmt.Printf("Usage for %[1]s: %[1]s [-x|o] [-c|a] [-d|l] [-f file] [files ...]\n", "tar")
		flag.PrintDefaults()
	}

	if *fstats {
		err := stats(*tfile)
		if err != nil {
			log.Fatalf("Error while getting statistics: %s", err)
		}
	}

	if *list || *tstamp {
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
			if err != nil {
				log.Fatalln(err)
			}
			fileSize := float64(hdr.Size)

			size := "bytes"
			sizeValue := fileSize
			if fileSize >= 1024.0 {
				size = "KB"
				sizeValue = fileSize / 1024.0
			}
			if fileSize >= 1024.0*1024.0 {
				size = "MB"
				sizeValue = fileSize / (1024.0 * 1024.0)
			}
			if fileSize >= 1024.0*1024.0*1024.0 {
				size = "GB"
				sizeValue = fileSize / (1024.0 * 1024.0 * 1024.0)
			}

			sizeFormat := "%.2f %s"
			if sizeValue == float64(int64(sizeValue)) {
				sizeFormat = "%.0f %s"
			}

			if *tstamp {
				modTime := hdr.ModTime.Format("2006-01-02 15:04:05")
				fmt.Printf("%s %s %s ("+sizeFormat+")\n", hdr.FileInfo().Mode(), modTime, hdr.Name, sizeValue, size)
			} else {
				fmt.Printf("%s %s ("+sizeFormat+")\n", hdr.FileInfo().Mode(), hdr.Name, sizeValue, size)
			}
		}
	}

	if *delete {
		err := deleteFromTarball(*tfile, flag.Args())
		if err != nil {
			log.Fatalf("Error deleting files from tarball: %s", err)
		}
	}

	if (*extract || *stdout) && len(flag.Args()) > 0 {
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
			if err != nil {
				log.Fatalln(err)
			}

			for _, arg := range flag.Args() {
				matched, err := filepath.Match(arg, hdr.Name)
				if err != nil {
					log.Fatal(err)
				}

				if matched {
					if *stdout {
						if _, err := io.Copy(os.Stdout, tr); err != nil {
							log.Fatal(err)
						}
					} else {
						destPath := hdr.Name
						if strings.HasSuffix(arg, "/") {
							destPath = path.Join(arg, path.Base(hdr.Name))
						}

						fi := hdr.FileInfo()
						if fi.IsDir() {
							os.MkdirAll(destPath, 0755)
						} else {
							os.MkdirAll(filepath.Dir(destPath), 0755)
							ofile, _ := os.Create(destPath)
							io.Copy(ofile, tr)
							ofile.Close()
						}
						fmt.Println(destPath)
					}
				}
			}
		}

		return
	}

	if (*extract || *stdout) && len(flag.Args()) == 0 {
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
				files, err := filepath.Glob(incpath)
				if err != nil {
					fmt.Println("Error getting files matching pattern:", err)
					return
				}
				for _, file := range files {
					filepath.Walk(file, walkpath)
				}
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
				files, err := filepath.Glob(incpath)
				if err != nil {
					fmt.Println("Error getting files matching pattern:", err)
					return
				}
				for _, file := range files {
					filepath.Walk(file, walkpath)
				}
			}
			tw.Close()
		} else {
			ofile, _ := os.Create(*tfile)
			tw = tar.NewWriter(ofile)
			for _, incpath := range flag.Args() {
				files, err := filepath.Glob(incpath)
				if err != nil {
					fmt.Println("Error getting files matching pattern:", err)
					return
				}
				for _, file := range files {
					filepath.Walk(file, walkpath)
				}
			}
			tw.Close()
			ofile.Close()
		}
	}

}

func findDuplicateFile(filename string) (bool, error) {
	tfile, err := os.Open(*tfile)
	if err != nil {
		return false, err
	}
	defer tfile.Close()

	tr := tar.NewReader(tfile)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
		if header.Name == filename {
			return true, nil
		}
		ext := filepath.Ext(filename)
		name := filename[:len(filename)-len(ext)]
		if strings.HasPrefix(header.Name, name+"_") {
			suffix := header.Name[len(name)+1 : len(header.Name)-len(ext)]
			if _, err := strconv.Atoi(suffix); err == nil {
				return true, nil
			}
		}
	}

	return false, nil
}

func stats(tarballPath string) error {
	tarballFile, err := os.Open(tarballPath)
	if err != nil {
		return fmt.Errorf("Error opening the tarball file: %s", err)
	}
	defer tarballFile.Close()

	tr := tar.NewReader(tarballFile)

	var totalSize int64
	fileCount := 0
	dirCount := 0
	symlinkCount := 0
	otherCount := 0

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("Error reading the tarball header: %s", err)
		}

		switch header.Typeflag {
		case tar.TypeReg:
			fileCount++
			totalSize += header.Size
		case tar.TypeDir:
			dirCount++
		case tar.TypeSymlink:
			symlinkCount++
		default:
			otherCount++
		}
	}

	fmt.Printf("Statistics for tarball: %s\n", tarballPath)
	fmt.Printf("Total files: %d\n", fileCount)
	fmt.Printf("Total directories: %d\n", dirCount)
	fmt.Printf("Total symbolic links: %d\n", symlinkCount)
	fmt.Printf("Total other entries: %d\n", otherCount)
	fmt.Printf("Total size: %d bytes\n", totalSize)

	return nil
}

func deleteFromTarball(tarballPath string, filesToDelete []string) error {
	tarballFile, err := os.OpenFile(tarballPath, os.O_RDWR, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Error opening the Tarball file: %s", err)
	}
	defer tarballFile.Close()

	tarballData, err := ioutil.ReadAll(tarballFile)
	if err != nil {
		return fmt.Errorf("Error reading the content of the tarball: %s", err)
	}
	if err := tarballFile.Close(); err != nil {
		return fmt.Errorf("Error closing the tarball file: %s", err)
	}

	newTarballFile, err := os.Create(tarballPath)
	if err != nil {
		return fmt.Errorf("Error creating the new tarball file: %s", err)
	}
	defer newTarballFile.Close()

	tw := tar.NewWriter(newTarballFile)
	tr := tar.NewReader(bytes.NewReader(tarballData))

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("Error reading the tarball header: %s", err)
		}
		deleteFile := false
		for _, fileToDelete := range filesToDelete {
			matched, err := filepath.Match(fileToDelete, header.Name)
			if err != nil {
				return fmt.Errorf("Error matching wildcard pattern: %s", err)
			}
			if matched {
				deleteFile = true
				break
			}
		}
		if !deleteFile {
			if err := tw.WriteHeader(header); err != nil {
				return fmt.Errorf("Error writing the file header to the new tarball: %s", err)
			}
			if _, err := io.Copy(tw, tr); err != nil {
				return fmt.Errorf("Error copying the file content to the new tarball: %s", err)
			}
		}
	}
	for _, dirToDelete := range filesToDelete {
		if strings.HasSuffix(dirToDelete, "/") {
			dirToDelete = strings.TrimSuffix(dirToDelete, "/")
		}

		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("Error reading the tarball header: %s", err)
			}
			if strings.HasPrefix(header.Name, dirToDelete+"/") {
				continue
			}
			if err := tw.WriteHeader(header); err != nil {
				return fmt.Errorf("Error writing the file header to the new tarball: %s", err)
			}
			if _, err := io.Copy(tw, tr); err != nil {
				return fmt.Errorf("Error copying the file content to the new tarball: %s", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("Error closing the tarball writer: %s", err)
	}
	return nil
}

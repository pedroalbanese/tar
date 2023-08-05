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
	"sort"
	"strconv"
	"strings"
)

var (
	appendf = flag.Bool("a", false, "append instead of overwrite; see also -c and -u")
	create  = flag.Bool("c", false, "create; it will overwrite the original file")
	delete  = flag.Bool("d", false, "delete files from tarball")
	extract = flag.Bool("x", false, "extract; see also -o")
	fstats  = flag.Bool("s", false, "stats")
	list    = flag.Bool("l", false, "list contents of tarball")
	stdout  = flag.Bool("o", false, "extract to stdout; see also -x")
	tfile   = flag.String("f", "", "tar file ('-' for stdin/stdout)")
	update  = flag.Bool("u", false, "update tarball; see also -c and -a")

	tw *tar.Writer
	tr *tar.Reader
)

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
	if *appendf && tw != nil {
		if f.IsDir() {
			header.Name = strings.TrimSuffix(path, "/")
		} else {
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
		err := stats(*tfile, *tfile == "-")
		if err != nil {
			log.Fatalf("Error while getting statistics: %s", err)
		}
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
			modTime := hdr.ModTime.Format("2006-01-02 15:04:05")
			fmt.Printf("%s %s %s ("+sizeFormat+")\n", hdr.FileInfo().Mode(), modTime, hdr.Name, sizeValue, size)
		}
	}

	if *delete {
		err := deleteFromTarball(*tfile, flag.Args())
		if err != nil {
			log.Fatalf("Error deleting files from tarball: %s", err)
		}
	}

	if *update {
		err := updateTarball(*tfile, flag.Args())
		if err != nil {
			log.Fatalf("Error updating tarball: %s", err)
		}
		return
	}

	if (*extract || *stdout) && len(flag.Args()) > 0 {
		var ifile io.Reader
		if *tfile == "-" {
			ifile = os.Stdin
		} else {
			var err error
			ifile, err = os.Open(*tfile)
			if err != nil {
				log.Fatalln(err)
			}
		}
		tr := tar.NewReader(ifile)

		for _, arg := range flag.Args() {
			dirs, err := filepath.Glob(arg)
			if err != nil {
				log.Fatal(err)
			}
			if len(dirs) == 0 {
				dirs = append(dirs, arg)
			}
			for _, dir := range dirs {
				dirsToExtract := make(map[string]struct{})
				dirsToExtract[dir] = struct{}{}

				for {
					hdr, err := tr.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						log.Fatalln(err)
					}

					for dir := range dirsToExtract {
						matched, err := filepath.Match(dir, hdr.Name)
						if err != nil {
							log.Fatal(err)
						}
						if matched {
							if hdr.FileInfo().IsDir() {
								err := extractDir(tr, hdr.Name, *stdout)
								if err != nil {
									log.Fatal(err)
								}
							} else {
								if *stdout {
									if _, err := io.Copy(os.Stdout, tr); err != nil {
										log.Fatal(err)
									}
								} else {
									destPath := hdr.Name
									if strings.HasSuffix(dir, "/") {
										destPath = path.Join(dir, path.Base(hdr.Name))
									}
									fi := hdr.FileInfo()
									if fi.IsDir() {
										if err := os.MkdirAll(destPath, fi.Mode()); err != nil {
											log.Fatal(err)
										}
									} else {
										if err := os.MkdirAll(filepath.Dir(destPath), fi.Mode()); err != nil {
											log.Fatal(err)
										}
										ofile, err := os.Create(destPath)
										if err != nil {
											log.Fatal(err)
										}
										if _, err := io.Copy(ofile, tr); err != nil {
											log.Fatal(err)
										}
										ofile.Close()
										if err := os.Chmod(destPath, fi.Mode()); err != nil {
											log.Fatal(err)
										}
									}
									fmt.Println(destPath)
								}
							}
						}
					}
				}
				_, err = ifile.(io.Seeker).Seek(0, io.SeekStart)
				if err != nil {
					log.Fatal(err)
				}
				tr = tar.NewReader(ifile)
			}
		}
		return
	}

	if (*extract || *stdout) && len(flag.Args()) == 0 {
		var ifile io.Reader
		if *tfile == "-" {
			ifile = os.Stdin
		} else {
			var err error
			ifile, err = os.Open(*tfile)
			if err != nil {
				log.Fatalln(err)
			}
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
					if err := os.MkdirAll(hdr.Name, fi.Mode()); err != nil {
						log.Fatalf("Error creating directory: %s", err)
					}
				} else {
					if err := os.MkdirAll(filepath.Dir(hdr.Name), fi.Mode()); err != nil {
						log.Fatalf("Error creating directory: %s", err)
					}
					ofile, err := os.Create(hdr.Name)
					if err != nil {
						log.Fatalf("Error creating file: %s", err)
					}
					if _, err := io.Copy(ofile, tr); err != nil {
						log.Fatal(err)
					}
					ofile.Close()

					if err := os.Chmod(hdr.Name, fi.Mode()); err != nil {
						log.Fatalf("Error setting permissions: %s", err)
					}

					fmt.Println(hdr.Name)
				}
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
			}
		}

	} else if *appendf {
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
		if err := reorganizeTarball(*tfile); err != nil {
			fmt.Println("Error:", err)
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

func stats(tarballPath string, stdinInput bool) error {
	var tarballFile io.ReadCloser
	var err error

	if stdinInput {
		tarballFile = os.Stdin
	} else {
		tarballFile, err = os.Open(tarballPath)
		if err != nil {
			return fmt.Errorf("Error opening the tarball file: %s", err)
		}
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

	size := "bytes"
	sizeValue := float64(totalSize)

	if sizeValue >= 1024.0 {
		size = "KB"
		sizeValue = sizeValue / 1024.0
	}
	if sizeValue >= 1024.0 {
		size = "MB"
		sizeValue = sizeValue / 1024.0
	}
	if sizeValue >= 1024.0 {
		size = "GB"
		sizeValue = sizeValue / 1024.0
	}
	sizeFormat := "%.2f %s"
	if sizeValue == float64(int64(sizeValue)) {
		sizeFormat = "%.0f %s"
	}

	fmt.Printf("Statistics for tarball : %s\n", tarballPath)
	fmt.Printf("Total files            : %d\n", fileCount)
	fmt.Printf("Total directories      : %d\n", dirCount)
	fmt.Printf("Total symbolic links   : %d\n", symlinkCount)
	fmt.Printf("Total other entries    : %d\n", otherCount)
	fmt.Printf("Total size             : "+sizeFormat+"\n", sizeValue, size)

	return nil
}

func extractDir(tr *tar.Reader, dirPath string, toStdout bool) error {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("Error reading the tarball header: %s", err)
		}

		if strings.HasPrefix(hdr.Name, dirPath) {
			destPath := hdr.Name
			if strings.HasSuffix(dirPath, "/") {
				destPath = path.Join(dirPath, path.Base(hdr.Name))
			}

			fi := hdr.FileInfo()
			if fi.IsDir() && !toStdout {
				if err := os.MkdirAll(destPath, fi.Mode()); err != nil {
					return fmt.Errorf("Error creating directory: %s", err)
				}
			} else if toStdout {
				if _, err := io.Copy(os.Stdout, tr); err != nil {
					return err
				}
			} else {
				if err := os.MkdirAll(filepath.Dir(destPath), fi.Mode()); err != nil {
					return fmt.Errorf("Error creating directory: %s", err)
				}
				ofile, err := os.Create(destPath)
				if err != nil {
					return fmt.Errorf("Error creating file: %s", err)
				}
				if _, err := io.Copy(ofile, tr); err != nil {
					ofile.Close()
					return err
				}
				ofile.Close()

				if err := os.Chmod(destPath, fi.Mode()); err != nil {
					return fmt.Errorf("Error setting permissions: %s", err)
				}

				if !toStdout {
					fmt.Println(destPath)
				}
			}
		}
	}
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

func updateTarball(tarballPath string, filesToAdd []string) error {
	tarballFile, err := os.OpenFile(tarballPath, os.O_RDWR, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Error opening the tarball file: %s", err)
	}
	defer tarballFile.Close()

	var updatedTarballData bytes.Buffer
	tw := tar.NewWriter(&updatedTarballData)
	tr := tar.NewReader(tarballFile)

	existingFiles := make(map[string]bool)

	for _, fileToAdd := range filesToAdd {
		files, err := filepath.Glob(fileToAdd)
		if err != nil {
			fmt.Println("Error getting files matching pattern:", err)
			continue
		}
		for _, file := range files {
			filepath.Walk(file, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return fmt.Errorf("Error accessing file %s: %s", path, err)
				}

				header, err := tar.FileInfoHeader(info, "")
				if err != nil {
					return fmt.Errorf("Error creating tar header for %s: %s", path, err)
				}
				header.Name = path

				if existingFiles[path] {
					return nil
				} else {
					existingFiles[path] = true
				}

				if err := tw.WriteHeader(header); err != nil {
					return fmt.Errorf("Error writing the file header to the updated tarball: %s", err)
				}
				if info.IsDir() {
					return nil
				}
				fileToCopy, err := os.Open(path)
				if err != nil {
					return fmt.Errorf("Error opening the file %s: %s", path, err)
				}
				defer fileToCopy.Close()
				if _, err := io.Copy(tw, fileToCopy); err != nil {
					return fmt.Errorf("Error copying the file content to the updated tarball: %s", err)
				}
				fmt.Printf("Updated file: %s (%d bytes)\n", path, info.Size())

				return nil
			})
		}
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("Error reading from the original tarball: %s", err)
		}
		if !existingFiles[header.Name] {
			if err := tw.WriteHeader(header); err != nil {
				return fmt.Errorf("Error writing the file header to the updated tarball: %s", err)
			}
			if _, err := io.Copy(tw, tr); err != nil {
				return fmt.Errorf("Error copying the file content to the updated tarball: %s", err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("Error closing the tarball writer: %s", err)
	}
	if err := tarballFile.Truncate(0); err != nil {
		return fmt.Errorf("Error truncating the tarball file: %s", err)
	}
	if _, err := tarballFile.Seek(0, 0); err != nil {
		return fmt.Errorf("Error seeking to the beginning of the tarball file: %s", err)
	}
	if _, err := updatedTarballData.WriteTo(tarballFile); err != nil {
		return fmt.Errorf("Error writing the updated tarball data to the original tarball file: %s", err)
	}
	if err := reorganizeTarball(tarballPath); err != nil {
		fmt.Println("Error:", err)
	}
	return nil
}

type FileEntry struct {
	Header  *tar.Header
	Content []byte
}

func reorganizeTarball(tarballPath string) error {
	tarballFile, err := os.OpenFile(tarballPath, os.O_RDWR, os.ModePerm)
	if err != nil {
		return fmt.Errorf("Error opening the tarball file: %s", err)
	}
	defer tarballFile.Close()

	fileData := make(map[string]*FileEntry)
	tr := tar.NewReader(tarballFile)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("Error reading from the original tarball: %s", err)
		}

		var fileContent []byte
		if !header.FileInfo().IsDir() {
			fileContent, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("Error reading file content from the original tarball: %s", err)
			}
		}

		fileData[header.Name] = &FileEntry{
			Header:  header,
			Content: fileContent,
		}
	}

	var updatedTarballData bytes.Buffer
	tw := tar.NewWriter(&updatedTarballData)

	sortedFileNames := sortedKeys(fileData)

	for _, fileName := range sortedFileNames {
		fileEntry := fileData[fileName]
		header := &tar.Header{
			Name:       fileName,
			Mode:       fileEntry.Header.Mode,
			Uid:        fileEntry.Header.Uid,
			Gid:        fileEntry.Header.Gid,
			Uname:      fileEntry.Header.Uname,
			Gname:      fileEntry.Header.Gname,
			ModTime:    fileEntry.Header.ModTime,
			AccessTime: fileEntry.Header.AccessTime,
			ChangeTime: fileEntry.Header.ChangeTime,
		}
		if fileEntry.Header.FileInfo().IsDir() {
			header.Typeflag = tar.TypeDir
			if err := tw.WriteHeader(header); err != nil {
				return fmt.Errorf("Error writing the directory header to the updated tarball: %s", err)
			}
			continue
		}

		header.Size = int64(len(fileEntry.Content))
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("Error writing the file header to the updated tarball: %s", err)
		}
		if _, err := tw.Write(fileEntry.Content); err != nil {
			return fmt.Errorf("Error writing the file content to the updated tarball: %s", err)
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("Error closing the tarball writer: %s", err)
	}

	if err := tarballFile.Truncate(0); err != nil {
		return fmt.Errorf("Error truncating the tarball file: %s", err)
	}

	if _, err := tarballFile.Seek(0, 0); err != nil {
		return fmt.Errorf("Error seeking to the beginning of the tarball file: %s", err)
	}

	if _, err := updatedTarballData.WriteTo(tarballFile); err != nil {
		return fmt.Errorf("Error writing the updated tarball data to the original tarball file: %s", err)
	}
	return nil
}

func sortedKeys(m map[string]*FileEntry) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		partsI := strings.Split(keys[i], string(os.PathSeparator))
		partsJ := strings.Split(keys[j], string(os.PathSeparator))
		for k := 0; k < len(partsI) && k < len(partsJ); k++ {
			if partsI[k] == partsJ[k] {
				continue
			}
			return partsI[k] < partsJ[k]
		}
		return len(partsI) < len(partsJ)
	})
	return keys
}

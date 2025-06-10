package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/s2"
	"github.com/klauspost/compress/zstd"
	"github.com/pedroalbanese/brotli"
	"github.com/pedroalbanese/lzma"
	"github.com/pedroalbanese/xz"
	"github.com/pierrec/lz4/v4"
	"rsc.io/getopt"
)

var (
	algorithm = flag.String("A", "", "algorithm: gzip, bzip2, s2, lzma, lz4, xz, zstd, zlib or brotli")
	appendf   = flag.Bool("a", false, "append instead of overwrite; see also -c and -u")
	create    = flag.Bool("c", false, "create; it will overwrite the original file")
	delete    = flag.Bool("d", false, "delete files from tarball")
	extract   = flag.Bool("x", false, "extract; see also -o")
	fstats    = flag.Bool("s", false, "stats")
	help      = flag.Bool("h", false, "print this help message")
	level     = flag.Int("L", 4, "compression level (1 = fastest, 9 = best)")
	list      = flag.Bool("l", false, "list contents of tarball")
	stdout    = flag.Bool("o", false, "extract to stdout; see also -x")
	tfile     = flag.String("f", "", "tar file ('-' for stdin/stdout)")
	update    = flag.Bool("u", false, "update tarball; see also -c and -a")
	compress  = flag.Bool("z", false, "compress/decompress the tarball")

	tw *tar.Writer
	tr *tar.Reader
	cw io.WriteCloser
	cr io.Reader
)

func wrapCompressionWriter(w io.Writer, level, cores *int) (io.WriteCloser, error) {
	var (
		writer io.WriteCloser
		err    error
	)

	switch *algorithm {
	case "gzip":
		writer, err = gzip.NewWriterLevel(w, *level)
	case "zlib":
		writer, err = zlib.NewWriterLevel(w, *level)
	case "bzip2":
		writer, err = bzip2.NewWriter(w, &bzip2.WriterConfig{Level: *level})
	case "s2":
		switch {
		case *level <= 3:
			writer = s2.NewWriter(w, s2.WriterBetterCompression())
		case *level >= 7:
			writer = s2.NewWriter(w, s2.WriterBestCompression())
		default:
			writer = s2.NewWriter(w)
		}
	case "zstd":
		if *cores == 0 {
			*cores = 32
			
		}
		writer, err = zstd.NewWriter(w,
			zstd.WithEncoderLevel(zstd.EncoderLevel(*level)),
			zstd.WithEncoderConcurrency(*cores),
		)
	case "lzma":
		writer = lzma.NewWriterLevel(w, *level)
	case "lz4":
		var lvl lz4.CompressionLevel
		switch *level {
		case 0:
			lvl = lz4.Fast
		case 1:
			lvl = lz4.Level1
		case 2:
			lvl = lz4.Level2
		case 3:
			lvl = lz4.Level3
		case 4:
			lvl = lz4.Level4
		case 5:
			lvl = lz4.Level5
		case 6:
			lvl = lz4.Level6
		case 7:
			lvl = lz4.Level7
		case 8:
			lvl = lz4.Level8
		case 9:
			lvl = lz4.Level9
		default:
			lvl = lz4.Fast
		}
		zw := lz4.NewWriter(w)
		options := []lz4.Option{
			lz4.CompressionLevelOption(lvl),
			lz4.ConcurrencyOption(*cores),
		}
		if err = zw.Apply(options...); err != nil {
			return nil, err
		}
		writer = zw
	case "xz":
		writer, err = xz.NewWriter(w)
	default:
		writer = brotli.NewWriterLevel(w, *level)
	}

	return writer, err
}

func wrapCompressionReader(r io.Reader) (io.Reader, error) {
	switch *algorithm {
	case "gzip":
		return gzip.NewReader(r)
	case "zlib":
		return zlib.NewReader(r)
	case "brotli":
		return brotli.NewReader(r), nil
	case "bzip2":
		reader, err := bzip2.NewReader(r, &bzip2.ReaderConfig{})
		return reader, err
	case "xz":
		reader, err := xz.NewReader(r)
		return reader, err
	case "lzma":
		reader := lzma.NewReader(r)
		return reader, nil
	case "lz4":
		reader := lz4.NewReader(r)
		return reader, nil
	case "zstd":
		reader, err := zstd.NewReader(r)
		return reader, err
	case "s2":
		reader := s2.NewReader(r)
		return reader, nil
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", *algorithm)
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (n nopWriteCloser) Close() error {
	return nil
}

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
	if *help {
		getopt.PrintDefaults()
	}
	/*
		// Alias short flags with their long counterparts.
		getopt.Aliases(
			"A", "algorithm",
			"a", "append",
			"c", "create",
			"d", "delete",
			"x", "extract",
			"s", "stats",
			"l", "list",
			"o", "stdout",
			"u", "update",
			"z", "compress",
			"h", "help",
		)
	*/
	getopt.Parse()

	if *algorithm == "" && *tfile != "" && *tfile != "-" {
		ext := strings.ToLower(filepath.Ext(*tfile))
		switch ext {
		case ".gz", ".tgz":
			*algorithm = "gzip"
		case ".zz", ".zlib":
			*algorithm = "zlib"
		case ".br":
			*algorithm = "brotli"
		case ".bz2":
			*algorithm = "bzip2"
		case ".xz":
			*algorithm = "xz"
		case ".lzma":
			*algorithm = "lzma"
		case ".lz4":
			*algorithm = "lz4"
		case ".zst", ".zstd":
			*algorithm = "zstd"
		case ".s2":
			*algorithm = "s2"
		default:
			*algorithm = "none"
		}
		
		if *algorithm != "none" {
			*compress = true
		}
	}
	
	if len(os.Args) == 1 {
		fmt.Printf("Usage for %[1]s: %[1]s [OPTION] [-f FILE] [FILES ...]\n", "tar")
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

		if *compress {
			cr, err := wrapCompressionReader(ifile)
			if err != nil {
				log.Fatalf("Compression error: %v", err)
			}
			ifile = cr
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

		if *compress {
			cr, err := wrapCompressionReader(ifile)
			if err != nil {
				log.Fatalf("Compression error: %v", err)
			}
			ifile = cr
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
				if seeker, ok := ifile.(io.Seeker); ok {
					_, err = seeker.Seek(0, io.SeekStart)
					if err != nil {
						log.Fatal(err)
					}
				} else {
					log.Fatal("Cannot seek on input (possibly compressed stream)")
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

		if *compress {
			cr, err := wrapCompressionReader(ifile)
			if err != nil {
				log.Fatalf("Compression error: %v", err)
			}
			ifile = cr
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
			defer ofile.Close()

			var writer io.Writer = ofile
			if *compress {
				err := appendToCompressedTarball(*tfile, flag.Args())
				if err != nil {
					log.Fatalf("Error appending to compressed tarball: %v", err)
				}
				return
			}

			if _, err = ofile.Seek(-1024, io.SeekEnd); err != nil {
				log.Fatalln(err)
			}
			tw = tar.NewWriter(writer)

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
			if err := reorganizeTarball(*tfile); err != nil {
				fmt.Println("Error:", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "%s not found\n", *tfile)
		}

	} else if *create {
		var writer io.Writer
		if *tfile == "-" {
			writer = os.Stdout
		} else {
			ofile, err := os.Create(*tfile)
			if err != nil {
				log.Fatalf("Error creating file: %v", err)
			}
			defer ofile.Close()
			writer = ofile
		}

		if *compress {
			level := *level
			cores := 0
			cw, err := wrapCompressionWriter(writer, &level, &cores)
			if err != nil {
				log.Fatalf("Compression error: %v", err)
			}
			defer cw.Close()
			tw = tar.NewWriter(cw)
		} else {
			tw = tar.NewWriter(writer)
		}

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
		if cw != nil {
			cw.Close()
		}
	}
}

func appendToCompressedTarball(tarballPath string, filesToAdd []string) error {
	originalFile, err := os.Open(tarballPath)
	if err != nil {
		return fmt.Errorf("error opening existing tarball: %v", err)
	}
	defer originalFile.Close()

	cr, err := wrapCompressionReader(originalFile)
	if err != nil {
		return fmt.Errorf("compression reader error: %v", err)
	}
	tr := tar.NewReader(cr)

	var buffer bytes.Buffer
	level := *level
	cores := 0
	cw, err := wrapCompressionWriter(&buffer, &level, &cores)
	if err != nil {
		return fmt.Errorf("compression writer error: %v", err)
	}
	tw := tar.NewWriter(cw)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading original tarball: %v", err)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("error writing header: %v", err)
		}
		if _, err := io.Copy(tw, tr); err != nil {
			return fmt.Errorf("error copying file data: %v", err)
		}
	}

	for _, pattern := range filesToAdd {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("error matching pattern %s: %v", pattern, err)
		}
		for _, match := range matches {
			err := filepath.Walk(match, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				header, err := tar.FileInfoHeader(info, "")
				if err != nil {
					return fmt.Errorf("error creating header for %s: %v", path, err)
				}

				relPath, _ := filepath.Rel(".", path)
				header.Name = filepath.ToSlash(relPath)

				if err := tw.WriteHeader(header); err != nil {
					return fmt.Errorf("error writing header for %s: %v", path, err)
				}

				if !info.IsDir() {
					file, err := os.Open(path)
					if err != nil {
						return fmt.Errorf("error opening file %s: %v", path, err)
					}
					defer file.Close()

					if _, err := io.Copy(tw, file); err != nil {
						return fmt.Errorf("error writing file %s: %v", path, err)
					}
				}

				fmt.Printf("Appended: %s (%d bytes)\n", header.Name, info.Size())
				return nil
			})

			if err != nil {
				return fmt.Errorf("error processing %s: %v", match, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("error closing tar writer: %v", err)
	}
	if err := cw.Close(); err != nil {
		return fmt.Errorf("error closing compression writer: %v", err)
	}

	if err := originalFile.Close(); err != nil {
		return fmt.Errorf("error closing original tarball before overwrite: %v", err)
	}
	if err := os.WriteFile(tarballPath, buffer.Bytes(), 0644); err != nil {
		return fmt.Errorf("error writing updated tarball: %v", err)
	}

	if err := reorganizeTarball(*tfile); err != nil {
		fmt.Println("Error:", err)
	}

	return nil
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

	var tr *tar.Reader
	if *compress {
		cr, err := wrapCompressionReader(tarballFile)
		if err != nil {
			log.Fatalf("Compression error: %v", err)
		}
		tr = tar.NewReader(cr)
	} else {
		tr = tar.NewReader(tarballFile)
	}

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
		return fmt.Errorf("error opening the tarball file: %s", err)
	}
	defer tarballFile.Close()

	var tr *tar.Reader
	if *compress {
		cr, err := wrapCompressionReader(tarballFile)
		if err != nil {
			return fmt.Errorf("error creating compression reader: %v", err)
		}
		tr = tar.NewReader(cr)
	} else {
		tr = tar.NewReader(tarballFile)
	}

	fileData := make(map[string]*FileEntry)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading from the tarball: %s", err)
		}

		shouldDelete := false
		for _, pattern := range filesToDelete {
			matched, _ := filepath.Match(pattern, header.Name)
			if matched || strings.HasPrefix(header.Name, strings.TrimSuffix(pattern, "/")+"/") {
				shouldDelete = true
				break
			}
		}

		if shouldDelete {
			fmt.Printf("Deleted: %s\n", header.Name)
			continue
		}

		var content []byte
		if !header.FileInfo().IsDir() {
			content, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("error reading content for %s: %s", header.Name, err)
			}
		}

		fileData[header.Name] = &FileEntry{
			Header:  header,
			Content: content,
		}
	}

	var buffer bytes.Buffer
	var cw io.WriteCloser
	var tw *tar.Writer

	if *compress {
		level := *level
		cores := 0
		cw, err = wrapCompressionWriter(&buffer, &level, &cores)
		if err != nil {
			return fmt.Errorf("error creating compression writer: %v", err)
		}
		tw = tar.NewWriter(cw)
	} else {
		tw = tar.NewWriter(&buffer)
	}

	sortedFileNames := sortedKeys(fileData)

	for _, name := range sortedFileNames {
		entry := fileData[name]
		header := entry.Header

		if !header.FileInfo().IsDir() {
			header.Size = int64(len(entry.Content))
		}

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("error writing header: %s", err)
		}
		if !header.FileInfo().IsDir() {
			if _, err := tw.Write(entry.Content); err != nil {
				return fmt.Errorf("error writing content for %s: %s", name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("error closing tar writer: %s", err)
	}
	if cw != nil {
		if err := cw.Close(); err != nil {
			return fmt.Errorf("error closing compression writer: %s", err)
		}
	}

	if err := tarballFile.Truncate(0); err != nil {
		return fmt.Errorf("error truncating tarball file: %s", err)
	}
	if _, err := tarballFile.Seek(0, 0); err != nil {
		return fmt.Errorf("error seeking to start of tarball file: %s", err)
	}
	if _, err := buffer.WriteTo(tarballFile); err != nil {
		return fmt.Errorf("error writing updated tarball: %s", err)
	}

	return nil
}

func updateTarball(tarballPath string, filesToAdd []string) error {
	originalFile, err := os.OpenFile(tarballPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("Error opening original tarball: %s", err)
	}
	defer originalFile.Close()

	originalInfo, err := os.Stat(tarballPath)
	if err != nil {
		return fmt.Errorf("Error stating original tarball: %s", err)
	}
	originalMode := originalInfo.Mode()

	var tr *tar.Reader
	if *compress {
		cr, err := wrapCompressionReader(originalFile)
		if err != nil {
			return fmt.Errorf("Error initializing compression reader: %v", err)
		}
		tr = tar.NewReader(cr)
	} else {
		tr = tar.NewReader(originalFile)
	}

	existingFiles := make(map[string]*FileEntry)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("Error reading tarball: %s", err)
		}

		content := []byte{}
		if !hdr.FileInfo().IsDir() {
			content, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("Error reading file content: %s", err)
			}
		}

		existingFiles[hdr.Name] = &FileEntry{
			Header:  hdr,
			Content: content,
		}
	}

	for _, pattern := range filesToAdd {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("Error matching pattern %s: %s", pattern, err)
		}

		for _, match := range matches {
			err := filepath.Walk(match, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				header, err := tar.FileInfoHeader(info, "")
				if err != nil {
					return fmt.Errorf("Error creating header for %s: %s", path, err)
				}

				relPath, _ := filepath.Rel(".", path)
				header.Name = filepath.ToSlash(relPath)

				var content []byte
				if !info.IsDir() {
					file, err := os.Open(path)
					if err != nil {
						return fmt.Errorf("Error opening file %s: %s", path, err)
					}
					defer file.Close()

					content, err = io.ReadAll(file)
					if err != nil {
						return fmt.Errorf("Error reading file %s: %s", path, err)
					}
				}

				existingFiles[header.Name] = &FileEntry{
					Header:  header,
					Content: content,
				}

				fmt.Printf("Added or updated: %s (%d bytes)\n", header.Name, len(content))
				return nil
			})

			if err != nil {
				return fmt.Errorf("Error walking path %s: %s", match, err)
			}
		}
	}

	var updatedBuffer bytes.Buffer
	var tw *tar.Writer
	var compressionWriter io.WriteCloser

	if *compress {
		level := *level
		cores := 0
		cw, err := wrapCompressionWriter(&updatedBuffer, &level, &cores)
		if err != nil {
			return fmt.Errorf("Error initializing compression writer: %v", err)
		}
		defer cw.Close()
		compressionWriter = cw
		tw = tar.NewWriter(cw)
	} else {
		tw = tar.NewWriter(&updatedBuffer)
	}
	defer tw.Close()

	sortedFileNames := sortedKeys(existingFiles)
	for _, name := range sortedFileNames {
		entry := existingFiles[name]
		header := entry.Header
		if !header.FileInfo().IsDir() {
			header.Size = int64(len(entry.Content))
		}
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("Error writing header for %s: %s", name, err)
		}
		if !header.FileInfo().IsDir() {
			if _, err := tw.Write(entry.Content); err != nil {
				return fmt.Errorf("Error writing content for %s: %s", name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("Error closing tar writer: %s", err)
	}
	if compressionWriter != nil {
		if err := compressionWriter.Close(); err != nil {
			return fmt.Errorf("Error closing compression writer: %s", err)
		}
	}

	if err := originalFile.Truncate(0); err != nil {
		return fmt.Errorf("Error truncating tarball: %s", err)
	}

	if _, err := originalFile.Seek(0, 0); err != nil {
		return fmt.Errorf("Error seeking to beginning: %s", err)
	}

	if _, err := updatedBuffer.WriteTo(originalFile); err != nil {
		return fmt.Errorf("Error writing updated tarball: %s", err)
	}

	if err := os.Chmod(tarballPath, originalMode); err != nil {
		return fmt.Errorf("Error restoring permissions: %s", err)
	}

	return nil
}

type FileEntry struct {
	Header  *tar.Header
	Content []byte
}

func reorganizeTarball(tarballPath string) error {
	originalFile, err := os.OpenFile(tarballPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("Error opening tarball: %s", err)
	}
	defer originalFile.Close()

	originalInfo, err := os.Stat(tarballPath)
	if err != nil {
		return fmt.Errorf("Error stating original tarball: %s", err)
	}
	originalMode := originalInfo.Mode()

	var tarReader *tar.Reader

	if *compress {
		cr, err := wrapCompressionReader(originalFile)
		if err != nil {
			return fmt.Errorf("Error initializing compression reader: %v", err)
		}
		tarReader = tar.NewReader(cr)
	} else {
		tarReader = tar.NewReader(originalFile)
	}

	fileData := make(map[string]*FileEntry)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("Error reading tar entry: %s", err)
		}

		var content []byte
		if !header.FileInfo().IsDir() {
			content, err = io.ReadAll(tarReader)
			if err != nil {
				return fmt.Errorf("Error reading content for %s: %s", header.Name, err)
			}
		}

		fileData[header.Name] = &FileEntry{
			Header:  header,
			Content: content,
		}
	}

	var updatedBuffer bytes.Buffer
	var tw *tar.Writer
	var compressionWriter io.WriteCloser

	if *compress {
		level := *level
		cores := 0
		cw, err := wrapCompressionWriter(&updatedBuffer, &level, &cores)
		if err != nil {
			return fmt.Errorf("Error initializing compression writer: %v", err)
		}
		defer cw.Close()
		compressionWriter = cw
		tw = tar.NewWriter(cw)
	} else {
		tw = tar.NewWriter(&updatedBuffer)
	}
	defer tw.Close()

	sortedFileNames := sortedKeys(fileData)
	for _, name := range sortedFileNames {
		entry := fileData[name]
		header := entry.Header

		if !header.FileInfo().IsDir() {
			header.Size = int64(len(entry.Content))
		}

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("Error writing header for %s: %s", name, err)
		}

		if !header.FileInfo().IsDir() {
			if _, err := tw.Write(entry.Content); err != nil {
				return fmt.Errorf("Error writing content for %s: %s", name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("Error closing tar writer: %s", err)
	}
	if compressionWriter != nil {
		if err := compressionWriter.Close(); err != nil {
			return fmt.Errorf("Error closing compression writer: %s", err)
		}
	}

	if err := originalFile.Truncate(0); err != nil {
		return fmt.Errorf("Error truncating original tarball: %s", err)
	}

	if _, err := originalFile.Seek(0, 0); err != nil {
		return fmt.Errorf("Error seeking to beginning of file: %s", err)
	}

	if _, err := updatedBuffer.WriteTo(originalFile); err != nil {
		return fmt.Errorf("Error writing updated tarball: %s", err)
	}

	if err := os.Chmod(tarballPath, originalMode); err != nil {
		return fmt.Errorf("Error restoring permissions: %s", err)
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

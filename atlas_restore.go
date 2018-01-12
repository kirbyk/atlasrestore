package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type Config struct {
	DbPath               string `yaml:"dbPath"`
	Port                 int    `yaml:"port"`
	Ssl                  bool   `yaml:"ssl"`
	CertFile             string `yaml:"certFile"`
	KeyFile              string `yaml:"keyFile"`
	AuthKey              string `yaml:"authKey"`
	Filename             string `yaml:"filename"`
	TarDirectory         string `yaml:"tarDirectory"`
	GzipCompressionLevel int    `yaml:"gzipCompressionLevel"`
	DownloadMode         bool   `yaml:"downloadMode"`
	CentralUrl           string `yaml:"centralUrl"`
	JobId                string `yaml:"jobId"`
}

func addToTar(tw *tar.Writer, dbPath, tarParentDir, relPath string) error {
	fullPath := path.Join(dbPath, relPath)
	file, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(fileInfo, file.Name())
	if err != nil {
		return err
	}

	tarPath := path.Join(tarParentDir, relPath)
	header.Name = tarPath
	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	log.Printf("Streaming %s as %s, size %d\n", fullPath, tarPath, fileInfo.Size())
	if _, err := io.Copy(tw, file); err != nil {
		return err
	}

	log.Printf("Finished streaming %s\n", fullPath)
	return nil
}

// files in the channel are returned as relPaths (without the leading ./)
// from the given parent
func listFiles(parent, relDirPath string) (<-chan string, error) {
	dirPath := path.Join(parent, relDirPath)
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	ch := make(chan string)
	go func() {
		defer close(ch)
		n := len(files)
		for i := 0; i < n; i++ {
			child := files[i]
			relChildPath := path.Join(relDirPath, child.Name())
			if !child.IsDir() {
				ch <- relChildPath
			} else {
				files, err := listFiles(parent, relChildPath)
				if err != nil {
					panic(err)
				}
				for f := range files {
					ch <- f
				}
			}
		}
	}()

	return ch, nil
}

func writeTarGz(dbDir, tarParentDir string, gzipCompressionLevel int, w io.Writer) error {
	gw, err := gzip.NewWriterLevel(w, gzipCompressionLevel)
	if err != nil {
		return err
	}
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	iter, err := listFiles(dbDir, ".")
	if err != nil {
		return err
	}
	for relPath := range iter {
		if err := addToTar(tw, dbDir, tarParentDir, relPath); err != nil {
			log.Printf("Error writing file %s. Err: %v\n", relPath, err)
			return err
		}
	}

	return nil
}

func main() {
	var configFilename = flag.String("config", "", "configuration filename")
	flag.Parse()
	if *configFilename == "" {
		log.Printf("Please provide the configuration file\n")
		os.Exit(1)
	}

	config, err := parseConfig(*configFilename)
	if err != nil {
		log.Printf("Error parsing all required settings: %v\n", err)
		os.Exit(1)
	}

	log.Printf("Atlas Restore Server. Version %s Hash %s", VersionStr, GitCommitId)

	stats := NewStatsTracker(
		config.CentralUrl,
		config.JobId,
		config.AuthKey,
		config.DownloadMode)

	http.HandleFunc(strings.Join([]string{"", config.AuthKey, config.Filename}, "/"), func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Sending files from %s as %s.\n", config.DbPath, config.Filename)
		stats.InProgress()

		if err := writeTarGz(config.DbPath, config.TarDirectory, config.GzipCompressionLevel, w); err != nil {
			log.Printf("Error generating tar gz %s: %v\n", config.Filename, err)
			stats.Failed()
			return
		}
		stats.Succeeded()
		log.Printf("Done serving %s\n", config.Filename)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Disallowed route: %q", html.EscapeString(r.URL.Path))
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "Access denied\n")
	})

	if config.Ssl {
		log.Printf("Starting https server with config %v\n", config)
		log.Fatal(http.ListenAndServeTLS(strings.Join([]string{":", strconv.Itoa(config.Port)}, ""), config.CertFile, config.KeyFile, nil))
	} else {
		log.Printf("Starting http server with config %v\n", config)
		log.Fatal(http.ListenAndServe(strings.Join([]string{":", strconv.Itoa(config.Port)}, ""), nil))
	}
}

func parseConfig(file string) (*Config, error) {
	var config Config
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("Error reading the settings file. Filename: %v Err: %v", file, err)
	}

	err = yaml.Unmarshal(content, &config)
	if err != nil {
		return nil, err
	}

	if config.DbPath == "" {
		return nil, errors.New("Requires dbPath")
	}

	if config.Port == 0 {
		return nil, errors.New("Requires port")
	}

	if config.AuthKey == "" {
		return nil, errors.New("Requires authKey")
	}

	if config.Ssl && (config.CertFile == "" || config.KeyFile == "") {
		return nil, errors.New("Requires certFile and keyFile when ssl is enabled")
	}

	if config.Filename == "" {
		return nil, errors.New("Requires output filename")
	}

	if config.TarDirectory == "" {
		return nil, errors.New("Requires tar directory")
	}

	if config.GzipCompressionLevel < gzip.DefaultCompression || config.GzipCompressionLevel > gzip.BestCompression {
		return nil, fmt.Errorf("gzipCompressionLevel has to be in valid range. Expected: [%v, %v] Got: %v",
			gzip.DefaultCompression, gzip.BestCompression, config.GzipCompressionLevel)
	}

	return &config, nil
}

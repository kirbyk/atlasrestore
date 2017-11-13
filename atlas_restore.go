package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"html"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DbPath               string `yaml:"dbPath"`
	Port                 int    `yaml:"port"`
	Ssl                  bool   `yaml:"ssl"`
	CertFile             string `yaml:"certFile"`
	KeyFile              string `yaml:"keyFile"`
	AuthKey              string `yaml:"authKey"`
	Filename             string `yaml:"filename"`
	GzipCompressionLevel int    `yaml:"gzipCompressionLevel"`
}

func addToTar(tw *tar.Writer, path string) error {
	file, err := os.Open(path)
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

	header.Name = path
	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	log.Printf("Streaming %s, size %d\n", path, fileInfo.Size())
	if _, err := io.Copy(tw, file); err != nil {
		return err
	}

	log.Printf("Finished streaming %s\n", path)
	return nil
}

func listFiles(parent string) (<-chan string, error) {
	files, err := ioutil.ReadDir(parent)
	if err != nil {
		return nil, err
	}

	ch := make(chan string)
	go func() {
		defer close(ch)
		n := len(files)
		for i := 0; i < n; i++ {
			child := files[i]
			path := parent + string(os.PathSeparator) + child.Name()
			if !child.IsDir() {
				ch <- path
			} else {
				files, err := listFiles(path)
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

func writeTarGz(inputDir string, gzipCompressionLevel int, w io.Writer) error {
	gw, err := gzip.NewWriterLevel(w, gzipCompressionLevel)
	if err != nil {
		return err
	}
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	iter, err := listFiles(inputDir)
	if err != nil {
		return err
	}
	for f := range iter {
		if err := addToTar(tw, f); err != nil {
			log.Printf("Error writing file %s. Err: %v\n", f, err)
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
	http.HandleFunc(strings.Join([]string{"", config.AuthKey, config.Filename}, "/"), func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Sending files from %s as %s.\n", config.DbPath, config.Filename)
		if err := writeTarGz(config.DbPath, config.GzipCompressionLevel, w); err != nil {
			log.Printf("Error generating tar gz %s: %v\n", config.Filename, err)
			return
		}
		log.Printf("Done serving %s\n", config.Filename)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Disallowed route: %q", html.EscapeString(r.URL.Path))
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "Access denied\n")
	})
	if config.Ssl {
		log.Printf("Starting https server on port %d\n", config.Port)
		log.Fatal(http.ListenAndServeTLS(strings.Join([]string{":", strconv.Itoa(config.Port)}, ""), config.CertFile, config.KeyFile, nil))
	} else {
		log.Printf("Starting http server on port %d\n", config.Port)
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

	if config.GzipCompressionLevel < gzip.DefaultCompression || config.GzipCompressionLevel > gzip.BestCompression {
		return nil, fmt.Errorf("gzipCompressionLevel has to be in valid range. Expected: [%v, %v] Got: %v",
			gzip.DefaultCompression, gzip.BestCompression, config.GzipCompressionLevel)
	}

	return &config, nil
}

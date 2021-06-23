package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/iancoleman/strcase"
	"gopkg.in/yaml.v2"
)

const (
	jsonFileExt = ".json"
	yamlFileExt = ".yaml"
	ymlFileExt  = ".yml"
	tomlFileExt = ".toml"
	csvFileExt  = ".csv"
	zipFileExt  = ".zip"
)

type localizationFile map[string]string

type TmplValues struct {
	Timestamp     time.Time
	Keys          map[string]string
	Localizations map[string]string
	Package       string
	ListKeys      map[string][]string
}

const (
	defaultOutputDir = "localizations"
)

var (
	input  = flag.String("input", "", "input localizations folder")
	output = flag.String("output", "", "where to output the generated package")

	errFlagInputNotSet = errors.New("the flag -input must be set")
	needRemovePaths    = make([]string, 0)
)

func main() {
	flag.Parse()

	if err := run(input, output); err != nil {
		log.Fatal(err.Error())
	}

	if len(needRemovePaths) > 0 {
		removeZipFile(needRemovePaths)
	}
}

func run(in, out *string) error {
	inputDir, outputDir, err := parseFlags(in, out)
	if err != nil {
		return err
	}

	files, err := getLocalizationFiles(inputDir)
	if err != nil {
		return err
	}

	localizations, keys, err := generateLocalizations(files)
	if err != nil {
		return err
	}

	return generateFile(outputDir, keys, localizations)
}

func generateLocalizations(files []string) (map[string]string, []string, error) {
	localizations := map[string]string{}
	keyMap := make(map[string]struct{})
	for _, file := range files {
		newLocalizations, keys, err := getLocalizationsFromFile(file)
		if err != nil {
			return nil, nil, err
		}
		for key, value := range newLocalizations {
			localizations[key] = value
		}

		for _, v := range keys {
			keyMap[v] = struct{}{}
		}
	}

	keys := make([]string, 0)

	for k := range keyMap {
		keys = append(keys, k)
	}

	sort.SliceStable(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	return localizations, keys, nil
}

func getLocalizationFiles(dir string) ([]string, error) {
	var files []string

	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		ext := filepath.Ext(path)
		if !info.IsDir() && ext == zipFileExt {
			return Unzip(path, dir)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		ext := filepath.Ext(path)
		if !info.IsDir() && (ext == jsonFileExt || ext == yamlFileExt) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func generateFile(output string, keys []string, localizations map[string]string) error {
	dir := output
	parent := output
	if strings.Contains(output, string(filepath.Separator)) {
		parent = filepath.Base(dir)
	}

	err := os.MkdirAll(output, 0700)
	if err != nil {
		return err
	}

	f, err := os.Create(fmt.Sprintf("%v/%v.go", dir, parent))
	if err != nil {
		return err
	}

	keyMap := make(map[string]string)

	listKeyMap := make(map[string][]string)
	r, _ := regexp.Compile(`^(.+)\.\d+$`)

	for _, v := range keys {
		k := strcase.ToCamel(v)
		keyMap[k] = v
		matchs := r.FindStringSubmatch(v)
		if len(matchs) == 2 {
			listKey := strcase.ToCamel(matchs[1])
			listKeyMap[listKey] = append(listKeyMap[listKey], k)
		}
	}

	if len(listKeyMap) > 0 {
		for k := range listKeyMap {
			sort.SliceStable(listKeyMap[k], func(i, j int) bool {
				a, _ := strconv.Atoi(strings.TrimLeft(listKeyMap[k][i], k))
				b, _ := strconv.Atoi(strings.TrimLeft(listKeyMap[k][j], k))
				return a < b
			})
		}

		listUtilFp, err := os.Create(fmt.Sprintf("%v/%v.go", dir, "localizations_util"))
		if err != nil {
			return err
		}
		err = packageTemplateUtil.Execute(listUtilFp, TmplValues{
			Timestamp: time.Now(),
			Package:   parent,
			ListKeys:  listKeyMap,
		})
		if err != nil {
			return err
		}
	}

	return packageTemplate.Execute(f, TmplValues{
		Timestamp:     time.Now(),
		Keys:          keyMap,
		Localizations: localizations,
		Package:       parent,
		ListKeys:      listKeyMap,
	})
}

func getLocalizationsFromFile(file string) (map[string]string, []string, error) {
	newLocalizations := map[string]string{}

	openFile, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}

	byteValue, err := ioutil.ReadAll(openFile)
	if err != nil {
		return nil, nil, err
	}

	localizationFile := localizationFile{}
	ext := filepath.Ext(file)
	switch ext {
	case jsonFileExt:
		err = json.Unmarshal(byteValue, &localizationFile)
	case yamlFileExt, ymlFileExt:
		err = yaml.Unmarshal(byteValue, &localizationFile)
	case tomlFileExt:
		_, err = toml.Decode(string(byteValue), &localizationFile)
	case csvFileExt:
		err = parseCSV(byteValue, &localizationFile)
	default:
		return nil, nil, nil
	}

	if err != nil {
		return nil, nil, err
	}

	slicePath := getSlicePath(file)

	keys := make([]string, 0, len(localizationFile))
	var keyPrefix string

	if len(slicePath) > 1 {
		keyPrefix = strings.Join(slicePath[1:], ".")
	}

	for key, value := range localizationFile {
		newLocalizations[strings.Join(append(slicePath, key), ".")] = value
		tmpKey := key
		if keyPrefix != "" {
			tmpKey = keyPrefix + "." + key
		}
		keys = append(keys, tmpKey)
	}

	return newLocalizations, keys, nil
}

func parseCSV(value []byte, l *localizationFile) error {
	r := csv.NewReader(bytes.NewReader(value))
	localizations := localizationFile{}
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		localizations[record[0]] = record[1]
	}
	*l = localizations
	return nil
}

func getSlicePath(file string) []string {
	dir, file := filepath.Split(file)

	paths := strings.Replace(dir, *input, "", -1)
	pathSlice := strings.Split(paths, string(filepath.Separator))

	var strs []string
	for _, part := range pathSlice {
		part := strings.TrimSpace(part)
		part = strings.Trim(part, "/")
		if part != "" {
			strs = append(strs, part)
		}
	}

	strs = append(strs, strings.Replace(file, filepath.Ext(file), "", -1))

	if length := len(strs); length > 1 {
		keyPath := make([]string, 0, length)
		keyPath = append(keyPath, strs[length-1])
		keyPath = append(keyPath, strs[:length-1]...)
		strs = keyPath
	}

	return strs
}

func parseFlags(input *string, output *string) (string, string, error) {
	var inputDir, outputDir string

	if *input == "" {
		return "", "", errFlagInputNotSet
	}
	if *output == "" {
		outputDir = defaultOutputDir
	} else {
		outputDir = *output
	}

	inputDir = *input

	return inputDir, outputDir, nil
}

// Unzip 解压zip
func Unzip(archive, target string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}

	for _, file := range reader.File {

		if file.FileInfo().IsDir() {
			continue
		}

		fileReader, err := file.Open()
		if err != nil {
			return err
		}
		defer fileReader.Close()

		_, fileName := filepath.Split(file.Name)

		unzipPath := filepath.Join(target, fileName)

		needRemovePaths = append(needRemovePaths, unzipPath)

		targetFile, err := os.OpenFile(unzipPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}
		defer targetFile.Close()

		if _, err := io.Copy(targetFile, fileReader); err != nil {
			return err
		}
	}

	return nil
}

func removeZipFile(paths []string) {
	for _, v := range paths {
		_ = os.RemoveAll(v)
	}
}

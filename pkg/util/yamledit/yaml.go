package yamledit

import (
	"bufio"
	"github.com/pkg/errors"
	"gopkg.in/mikefarah/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
)

type YamlEdit struct {
	WriteInPlace    bool
	SkipNotExisting bool
}

func (y *YamlEdit) ModifyProperty(filePath string, yamlPath string, value string) error {
	yaml.DefaultMapType = reflect.TypeOf(yaml.MapSlice{})
	return y.readAndUpdate(os.Stdout, filePath, func(dataBucket dataBucketType, currentIndex int) (dataBucketType, error) {
		return y.updatedChildValue(dataBucket, y.parsePath(yamlPath), value), nil
	})
}

func (y *YamlEdit) updatedChildValue(child dataBucketType, remainingPaths []string, value dataBucketType) dataBucketType {
	if len(remainingPaths) == 0 {
		return value
	}

	_, nextIndexErr := strconv.ParseInt(remainingPaths[0], 10, 64)
	if nextIndexErr != nil && remainingPaths[0] != "+" {
		// must be a map
		return y.writeMap(child, remainingPaths, value)
	}

	// must be an array
	return y.writeArray(child, remainingPaths, value)
}

func (y *YamlEdit) entryInSlice(context yaml.MapSlice, key interface{}) *yaml.MapItem {
	for idx := range context {
		var entry = &context[idx]
		if entry.Key == key {
			return entry
		}
	}
	return nil
}

func (y *YamlEdit) writeArray(context interface{}, paths []string, value interface{}) []interface{} {
	array, _ := y.getArray(context)

	if len(paths) == 0 {
		return array
	}

	rawIndex := paths[0]
	var index int64
	// the append array indicator
	if rawIndex == "+" {
		index = int64(len(array))
	} else {
		index, _ = strconv.ParseInt(rawIndex, 10, 64) // nolint
		// writeArray is only called by updatedChildValue which handles parsing the
		// index, as such this renders this dead code.
	}

	for index >= int64(len(array)) {
		array = append(array, nil)
	}
	currentChild := array[index]

	remainingPaths := paths[1:]
	array[index] = y.updatedChildValue(currentChild, remainingPaths, value)
	return array
}

func (y *YamlEdit) getMapSlice(context interface{}) yaml.MapSlice {
	var mapSlice yaml.MapSlice
	switch context.(type) {
	case yaml.MapSlice:
		mapSlice = context.(yaml.MapSlice)
	default:
		mapSlice = make(yaml.MapSlice, 0)
	}
	return mapSlice
}

func (y *YamlEdit) getArray(context interface{}) (array []interface{}, ok bool) {
	switch context.(type) {
	case []interface{}:
		array = context.([]interface{})
		ok = true
	default:
		array = make([]interface{}, 0)
		ok = false
	}
	return
}

func (y *YamlEdit) writeMap(context dataBucketType, paths []string, value dataBucketType) yaml.MapSlice {
	mapSlice := y.getMapSlice(context)

	if len(paths) == 0 {
		return mapSlice
	}

	child := y.entryInSlice(mapSlice, paths[0])
	newField := false
	if child == nil {
		newField = true
		if !y.SkipNotExisting {
			newChild := yaml.MapItem{Key: paths[0]}
			mapSlice = append(mapSlice, newChild)
			child = y.entryInSlice(mapSlice, paths[0])
		}
	}

	if !y.SkipNotExisting || !newField {
		remainingPaths := paths[1:]
		child.Value = y.updatedChildValue(child.Value, remainingPaths, value)
	}
	return mapSlice
}

func (y *YamlEdit) safelyRenameFile(from string, to string) {
	if renameError := os.Rename(from, to); renameError != nil {
		// can't do this rename when running in docker to a file targeted in a mounted volume,
		// so gracefully degrade to copying the entire contents.
		if copyError := y.copyFileContents(from, to); copyError != nil {
			panic(copyError)
		}
	}
}

func (y *YamlEdit) readAndUpdate(stdOut io.Writer, inputFile string, updateData updateDataFn) error {
	var destination io.Writer
	var destinationName string
	if y.WriteInPlace {
		info, err := os.Stat(inputFile)
		if err != nil {
			return err
		}
		tempFile, err := ioutil.TempFile("", "temp")
		if err != nil {
			return err
		}
		destinationName = tempFile.Name()
		err = os.Chmod(destinationName, info.Mode())
		if err != nil {
			return err
		}
		destination = tempFile
		defer func() {
			y.safelyCloseFile(tempFile)
			y.safelyRenameFile(tempFile.Name(), inputFile)
		}()
	} else {
		var writer = bufio.NewWriter(stdOut)
		destination = writer
		destinationName = "Stdout"
		defer y.safelyFlush(writer)
	}
	var encoder = yaml.NewEncoder(destination)
	return y.readStream(inputFile, y.mapYamlDecoder(updateData, encoder))
}

func (y *YamlEdit) safelyFlush(writer *bufio.Writer) {
	if err := writer.Flush(); err != nil {
		panic(err)
	}

}
func (y *YamlEdit) safelyCloseFile(file *os.File) {
	err := file.Close()
	if err != nil {
		panic(err)
	}
}

type yamlDecoderFn func(*yaml.Decoder) error

func (y *YamlEdit) readStream(filename string, yamlDecoder yamlDecoderFn) error {
	if filename == "" {
		return errors.New("Must provide filename")
	}

	var stream io.Reader
	if filename == "-" {
		stream = bufio.NewReader(os.Stdin)
	} else {
		file, err := os.Open(filename) // nolint gosec
		if err != nil {
			return err
		}
		defer y.safelyCloseFile(file)
		stream = file
	}
	return yamlDecoder(yaml.NewDecoder(stream))
}

func (y *YamlEdit) readData(filename string, indexToRead int, parsedData interface{}) error {
	return y.readStream(filename, func(decoder *yaml.Decoder) error {
		for currentIndex := 0; currentIndex < indexToRead; currentIndex++ {
			errorSkipping := decoder.Decode(parsedData)
			if errorSkipping != nil {
				return errors.Wrapf(errorSkipping, "Error processing document at index %v, %v", currentIndex, errorSkipping)
			}
		}
		return decoder.Decode(parsedData)
	})
}

func (y *YamlEdit) parsePath(path string) []string {
	return y.parsePathAccum([]string{}, path)
}

func (y *YamlEdit) parsePathAccum(paths []string, remaining string) []string {
	head, tail := y.nextYamlPath(remaining)
	if tail == "" {
		return append(paths, head)
	}
	return y.parsePathAccum(append(paths, head), tail)
}

func (y *YamlEdit) nextYamlPath(path string) (pathElement string, remaining string) {
	switch path[0] {
	case '[':
		// e.g [0].blah.cat -> we need to return "0" and "blah.cat"
		return y.search(path[1:], []uint8{']'}, true)
	case '"':
		// e.g "a.b".blah.cat -> we need to return "a.b" and "blah.cat"
		return y.search(path[1:], []uint8{'"'}, true)
	default:
		// e.g "a.blah.cat" -> return "a" and "blah.cat"
		return y.search(path[0:], []uint8{'.', '['}, false)
	}
}

func (y *YamlEdit) search(path string, matchingChars []uint8, skipNext bool) (pathElement string, remaining string) {
	for i := 0; i < len(path); i++ {
		var char = path[i]
		if y.contains(matchingChars, char) {
			var remainingStart = i + 1
			if skipNext {
				remainingStart = remainingStart + 1
			} else if !skipNext && char != '.' {
				remainingStart = i
			}
			if remainingStart > len(path) {
				remainingStart = len(path)
			}
			return path[0:i], path[remainingStart:]
		}
	}
	return path, ""
}

func (y *YamlEdit) contains(matchingChars []uint8, candidate uint8) bool {
	for _, a := range matchingChars {
		if a == candidate {
			return true
		}
	}
	return false
}

type dataBucketType interface{}
type updateDataFn func(dataBucket dataBucketType, currentIndex int) (dataBucketType, error)

func (y *YamlEdit) mapYamlDecoder(updateData updateDataFn, encoder *yaml.Encoder) yamlDecoderFn {
	return func(decoder *yaml.Decoder) error {
		var dataBucket dataBucketType
		var errorReading error
		var errorWriting error
		var errorUpdating error
		var currentIndex = 0

		for {
			errorReading = decoder.Decode(&dataBucket)

			if errorReading == io.EOF {
				return nil
			} else if errorReading != nil {
				return errors.Wrapf(errorReading, "Error reading document at index %v, %v", currentIndex, errorReading)
			}
			dataBucket, errorUpdating = updateData(dataBucket, currentIndex)
			if errorUpdating != nil {
				return errors.Wrapf(errorUpdating, "Error updating document at index %v", currentIndex)
			}

			errorWriting = encoder.Encode(dataBucket)

			if errorWriting != nil {
				return errors.Wrapf(errorWriting, "Error writing document at index %v, %v", currentIndex, errorWriting)
			}
			currentIndex = currentIndex + 1
		}
	}
}

func (y *YamlEdit) copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src) // nolint gosec
	if err != nil {
		return err
	}
	defer y.safelyCloseFile(in)
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer y.safelyCloseFile(out)
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

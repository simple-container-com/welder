package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/simple-container-com/welder/pkg/util"

	"gopkg.in/yaml.v2"
)

const (
	envTag     = "env"
	defaultTag = "default"
	yamlTag    = "yaml"
)

type Config interface {
	SetConfigFilePath(path string)
	GetConfigFilePath() string
	Init() error
}

// ReadConfig reads config from console based on provided one
func ReadConfig(defaultConfig Config, reader util.ConsoleReader) Config {
	return setValuesTo(defaultConfig, func(val reflect.Value, field reflect.StructField) string {
		if len(val.String()) == 0 && val.CanSet() {
			fieldName := field.Name
			fmt.Printf("Enter %s [%s]: ", fieldName, val.String())
			var text string
			if fieldName == "Password" {
				text, _ = reader.ReadPassword()
			} else {
				text, _ = reader.ReadLine()
			}
			if len(text) > 0 && text != "\n" {
				return text
			}
		}
		return ""
	})
}

// AddDefaults sets default values into fields not defined in raw config
func AddDefaults(rawConfig map[string]interface{}, newConfig Config) Config {
	return setValuesTo(newConfig, func(val reflect.Value, field reflect.StructField) string {
		yamlTagName := getYamlFieldName(field)
		defaultValue := field.Tag.Get(defaultTag)
		origValue := ""
		if isStringType(val) {
			origValue = val.String()
		} else if isIntType(val) {
			intVal := int(val.Int())
			if intVal != 0 {
				origValue = strconv.Itoa(intVal)
			}
		} else if isFloatType(val) {
			floatVal := val.Float()
			if floatVal != 0.0 {
				origValue = strconv.FormatFloat(floatVal, 'g', -1, 64)
			}
		} else if isBoolType(val) {
			boolVal := val.Bool()
			if boolVal {
				origValue = strconv.FormatBool(boolVal)
			}
		}
		if (rawConfig[yamlTagName] == nil || yamlTagName == "-") && origValue == "" {
			return defaultValue
		}
		return origValue
	})
}

// AddEnv processes environment variables to configuration
func AddEnv(obj Config) Config {
	return setValuesTo(obj, func(val reflect.Value, field reflect.StructField) string {
		envVarName := field.Tag.Get(envTag)
		return os.Getenv(envVarName)
	})
}

type getValueFunc func(val reflect.Value, field reflect.StructField) string

func setValuesTo(obj Config, valueFunc getValueFunc) Config {
	objVal, objType := inspect(obj)
	res := setValues(objVal, objType, valueFunc)
	return res.Addr().Interface().(Config)
}

func inspect(obj interface{}) (reflect.Value, reflect.Type) {
	objVal := reflect.ValueOf(obj)
	objType := reflect.TypeOf(obj)
	if objType.Kind() == reflect.Ptr {
		objType = objType.Elem()
	}
	if objVal.Kind() == reflect.Ptr {
		objVal = objVal.Elem()
	}
	return objVal, objType
}

// setValue sets values from env (if any)
func setValues(value reflect.Value, valueType reflect.Type, valueFunc getValueFunc) reflect.Value {
	if valueType.Kind() == reflect.Ptr {
		valueType = valueType.Elem()
	}
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}
	for i := 0; i < value.NumField(); i++ {
		fieldValue := value.Field(i)
		fieldType := valueType.Field(i)

		if isStructType(fieldValue) && fieldValue.CanSet() {
			fieldValue.Set(setValues(fieldValue, fieldType.Type, valueFunc))
		} else {
			newValue := valueFunc(fieldValue, fieldType)
			if newValue != "" {
				setFieldValue(fieldValue, newValue)
			}
		}
	}
	return value
}

func getYamlFieldName(fieldType reflect.StructField) string {
	return strings.Split(fieldType.Tag.Get(yamlTag), ",")[0]
}

// DefaultConfig Returns default version of Config file
func DefaultConfig(cfgObj Config) Config {
	return AddDefaults(map[string]interface{}{}, cfgObj)
}

func setFieldValue(field reflect.Value, valueString string) {
	if !field.CanSet() {
		return
	}
	if isStringType(field) {
		field.Set(reflect.ValueOf(valueString))
	} else if isIntType(field) {
		if intVal, e := strconv.Atoi(valueString); e != nil {
			fmt.Println(fmt.Sprintf("WARN: '%s' is not integer", valueString))
		} else {
			switch field.Kind() {
			case reflect.Int64:
				field.Set(reflect.ValueOf(int64(intVal)))
			case reflect.Int32:
				field.Set(reflect.ValueOf(int32(intVal)))
			case reflect.Int16:
				field.Set(reflect.ValueOf(int16(intVal)))
			case reflect.Int8:
				field.Set(reflect.ValueOf(int8(intVal)))
			default:
				field.Set(reflect.ValueOf(int(intVal)))
			}
		}
	} else if isFloatType(field) {
		if floatVal, e := strconv.ParseFloat(valueString, 64); e != nil {
			fmt.Println(fmt.Sprintf("WARN: '%s' is not float", valueString))
		} else {
			field.Set(reflect.ValueOf(floatVal))
		}
	} else if isBoolType(field) {
		field.Set(reflect.ValueOf(valueString == "true"))
	}
}

// ReadConfigFile Reads config file from yaml file
func ReadConfigFile(filePath string, readConfig Config) (Config, map[string]interface{}, error) {
	rawConfig := make(map[string]interface{})
	if fileBytes, err := ioutil.ReadFile(filePath); err == nil {
		err = yaml.Unmarshal(fileBytes, readConfig)
		if err != nil {
			return readConfig, rawConfig, err
		}
		err = yaml.Unmarshal(fileBytes, &rawConfig)
		if err != nil {
			return readConfig, rawConfig, err
		}
	}
	readConfig.SetConfigFilePath(filePath)
	return readConfig, rawConfig, nil
}

// Writes config file to yaml file
func WriteConfigFile(filePath string, cfg Config) error {
	if fileBytes, err := yaml.Marshal(cfg); err != nil {
		return err
	} else if err := ioutil.WriteFile(filePath, fileBytes, os.ModePerm); err != nil {
		return err
	}
	return nil
}

// Reads config file from yaml safely and adds defaults from env or default tags
func Init(filePath string, cfgObj Config, reader util.ConsoleReader) Config {
	config, rawConfig, err := ReadConfigFile(filePath, cfgObj)
	if err != nil {
		panic(err)
	}
	cfg := ReadConfig(AddEnv(AddDefaults(rawConfig, config)).(Config), reader)
	if err := cfg.Init(); err != nil {
		panic(err)
	}
	return cfg
}

func isStructType(field reflect.Value) bool {
	return field.Kind() == reflect.Struct || field.Kind() == reflect.Interface
}

func isStringType(field reflect.Value) bool {
	return field.Kind() == reflect.String
}

func isIntType(field reflect.Value) bool {
	return field.Kind() == reflect.Int ||
		field.Kind() == reflect.Int8 ||
		field.Kind() == reflect.Int16 ||
		field.Kind() == reflect.Int32 ||
		field.Kind() == reflect.Int64
}

func isFloatType(field reflect.Value) bool {
	return field.Kind() == reflect.Float32 ||
		field.Kind() == reflect.Float64
}

func isBoolType(field reflect.Value) bool {
	return field.Kind() == reflect.Bool
}

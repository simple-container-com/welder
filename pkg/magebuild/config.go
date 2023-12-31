package magebuild

import (
	"github.com/simple-container-com/welder/pkg/config"
	"github.com/simple-container-com/welder/pkg/util"
)

// Reads config file from yaml safely and adds defaults from env or default tags
func Init(filePath string, reader util.ConsoleReader) *GoBuildContext {
	return config.Init(filePath, &GoBuildContext{}, reader).(*GoBuildContext)
}

// Reads config file from yaml safely and adds defaults from env or default tags
func InitDefault() *GoBuildContext {
	return config.Init("./build.yaml", &GoBuildContext{}, util.DefaultConsoleReader).(*GoBuildContext)
}

// Package version — версия приложения.
package version

// Version задаётся при сборке:
//
//	go build -ldflags "-X kintion/server/internal/version.Version=0.1.0" ./server/cmd/kintion
var Version = "dev"

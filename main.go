package main

import (
	"flag"
	"io"
	"log"
	"os"

	"detour/common"
	"detour/local"
	"detour/logger"
	"detour/server"
)

var (
	password string
	listen   string
	remotes  string
	proto    string
	debug    bool
)

func main() {
	if len(os.Args) < 2 {
		logger.Error.Println("run with server/local subcommand")
		os.Exit(1)
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	switch os.Args[1] {
	case "server":
		ser := flag.NewFlagSet("server", flag.ExitOnError)
		ser.StringVar(&password, "p", "password", "password for authentication")
		ser.StringVar(&listen, "l", "tcp://0.0.0.0:3811", "address to listen on")
		ser.BoolVar(&debug, "d", false, "print debug log")

		ser.Parse(os.Args[2:])

		if !debug {
			logger.Debug.SetOutput(io.Discard)
		}

		s := server.NewServer(&common.ServerConfig{
			Listen:   listen,
			Password: password,
		})
		s.RunServer()
	case "local":
		cli := flag.NewFlagSet("local", flag.ExitOnError)
		cli.StringVar(&remotes, "r", "ws://localhost:3811/ws", "remote server(s) to connect, seperated by comma")
		cli.StringVar(&password, "p", "password", "password for authentication")
		cli.StringVar(&listen, "l", "tcp://0.0.0.0:3810", "address to listen on")
		cli.StringVar(&proto, "t", "socks5", "target protocol to use")
		cli.BoolVar(&debug, "d", false, "print debug log")

		cli.Parse(os.Args[2:])

		if !debug {
			logger.Debug.SetOutput(io.Discard)
		}

		c := local.NewLocal(&common.LocalConfig{
			Listen:   listen,
			Remotes:  remotes,
			Password: password,
			Proto:    proto,
		})
		c.RunLocal()
	default:
		logger.Error.Println("only server/local subcommands are allowed")
		os.Exit(1)
	}
}

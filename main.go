package main

import (
	"flag"
	"io"
	"log"
	"os"

	"github.com/observerss/detour2/common"
	"github.com/observerss/detour2/deploy"
	"github.com/observerss/detour2/local"
	"github.com/observerss/detour2/logger"
	"github.com/observerss/detour2/server"
)

var (
	password     string
	listen       string
	remotes      string
	proto        string
	debug        bool
	mode         string
	key          string
	secret       string
	accountId    string
	region       string
	serviceName  string
	functionName string
	triggerName  string
	image        string
	publicPort   int
	remove       bool
)

func main() {
	if len(os.Args) < 2 {
		logger.Error.Fatal("run with 'server'/'local'/'deploy' subcommand")
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
	case "deploy":
		cli := flag.NewFlagSet("deploy", flag.ExitOnError)
		cli.StringVar(&mode, "m", "", "deploy 'server' or 'local'")
		cli.StringVar(&key, "k", "", "aliyun access key id")
		cli.StringVar(&secret, "s", "", "aliyun access key secret")
		cli.StringVar(&accountId, "a", "", "aliyun main account id")
		cli.StringVar(&region, "r", "cn-hongkong", "aliyun region")
		cli.StringVar(&serviceName, "sn", "api2", "aliyun fc service name")
		cli.StringVar(&functionName, "fn", "dt2", "aliyun fc function name")
		cli.StringVar(&triggerName, "tn", "ws2", "aliyun fc trigger name")
		cli.StringVar(&password, "p", "password", "password for authentication")
		cli.StringVar(&image, "i", "registry-vpc.cn-hongkong.aliyuncs.com/hjcrocks/detour2", "aliyun container registry uri")
		cli.IntVar(&publicPort, "pp", 3810, "public port to use")
		cli.BoolVar(&remove, "remove", false, "remove all fc trigger/function/service")

		cli.Parse(os.Args[2:])

		if mode == "" {
			cli.Usage()
			logger.Error.Fatal("mode cannot be empty")
		}

		if key == "" || secret == "" || accountId == "" {
			cli.Usage()
			logger.Error.Fatal("aliyun credentials cannot be empty")
		}

		conf := &common.DeployConfig{
			AccessKeyId:     key,
			AccessKeySecret: secret,
			AccountId:       accountId,
			Region:          region,
			ServiceName:     serviceName,
			FunctionName:    functionName,
			TriggerName:     triggerName,
			Password:        password,
			Image:           image,
			PublicPort:      publicPort,
			Remove:          remove,
		}
		switch mode {
		case "server":
			err := deploy.DeployServer(conf)
			if err != nil {
				logger.Error.Fatal(err)
			}
		case "local":
			err := deploy.DeployLocal(conf)
			if err != nil {
				logger.Error.Fatal(err)
			}
		default:
			logger.Error.Fatal("method should be either 'server' or 'local'")
		}

	default:
		logger.Error.Fatal("only 'server'/'local'/'deploy' subcommands are allowed")
	}
}

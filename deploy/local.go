package deploy

import (
	"bufio"
	"detour/common"
	"detour/logger"
	"fmt"
	"log"
	"os/exec"
	"strconv"
)

const CONTAINER_NAME = "detour2-deploy-client"

func DeployLocal(conf *common.DeployConfig) {
	logger.Info.Println("deploy on local...")

	fc, err := NewClient(conf)
	if err != nil {
		logger.Error.Fatal(err)
	}

	wsurl, err := fc.GetWebsocketURL()
	if err != nil {
		logger.Error.Fatal(err)
	}
	log.Println("remote wsurl", wsurl)

	cmd := exec.Command("docker", "rm", "-f", CONTAINER_NAME)
	err = RunCommand(cmd)
	if err != nil {
		logger.Error.Fatal(err)
	}

	cmd = exec.Command("docker", "run", "-d",
		"--name", CONTAINER_NAME, "--restart", "always", "-p"+strconv.Itoa(conf.PublicPort)+":3810",
		conf.Image, "./detour", "local", "-p", conf.Password, "-r", wsurl, "-l", "tcp://0.0.0.0:3810",
	)
	err = RunCommand(cmd)
	if err != nil {
		logger.Error.Fatal(err)
	}

	logger.Info.Println("deploy ok.")
}

func RunCommand(cmd *exec.Cmd) error {
	fmt.Println("-------run command-------\n", cmd.String())
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	cmd.Start()
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			m := scanner.Text()
			fmt.Println(m)
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			m := scanner.Text()
			fmt.Println(m)
		}
	}()
	err := cmd.Wait()
	fmt.Println("")
	return err
}

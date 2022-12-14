package deploy

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/observerss/detour2/common"
	"github.com/observerss/detour2/logger"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const CONTAINER_NAME = "detour2-deploy-local"

func DeployLocal(conf *common.DeployConfig) error {
	logger.Info.Println("deploy on local...")

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}

	fc, err := NewClient(conf)
	if err != nil {
		return err
	}

	wsurl, err := fc.GetWebsocketURL()
	if err != nil {
		return err
	}
	log.Println("remote wsurl", wsurl)

	// cmd := exec.Command("docker", "rm", "-f", CONTAINER_NAME)
	// err = RunCommand(cmd)
	_ = cli.ContainerRemove(context.Background(), CONTAINER_NAME, types.ContainerRemoveOptions{Force: true})

	// cmd = exec.Command("docker", "run", "-d",
	// 	"--name", CONTAINER_NAME, "--restart", "always", "-p"+strconv.Itoa(conf.PublicPort)+":3810",
	// 	strings.ReplaceAll(conf.Image, "-vpc", ""), "./detour", "local", "-p", conf.Password, "-r", wsurl, "-l", "tcp://0.0.0.0:3810",
	// )
	// err = RunCommand(cmd)
	resp, err := cli.ContainerCreate(
		context.Background(),
		&container.Config{
			Image: strings.ReplaceAll(conf.Image, "-vpc", ""),
			ExposedPorts: nat.PortSet{
				nat.Port("3810/tcp"): {},
			},
			Cmd: []string{"./detour", "local", "-p", conf.Password, "-r", wsurl, "-l", "tcp://0.0.0.0:3810"},
		},
		&container.HostConfig{
			RestartPolicy: container.RestartPolicy{Name: "always"},
			PortBindings: nat.PortMap{
				nat.Port("3810/tcp"): []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(conf.PublicPort)}},
			},
		},
		nil,
		nil,
		CONTAINER_NAME)
	if err != nil {
		return err
	}

	err = cli.ContainerStart(context.Background(), resp.ID, types.ContainerStartOptions{})
	if err != nil {
		return err
	}

	logger.Info.Println("deploy ok.")
	return nil
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
